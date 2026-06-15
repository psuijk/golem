// Package terminal provides the interactive TUI for golem, built on
// Bubble Tea. It owns the input loop, renders agent events as they
// stream in, and handles slash commands.
package terminal

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/event"
)

// maxToolOutputLen is the maximum number of characters of tool output
// shown inline. Longer output is truncated with an ellipsis.
const maxToolOutputLen = 200

// Styles for terminal rendering.
var (
	toolStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	promptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	commandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	plainText    = lipgloss.NewStyle()
)

// slashCommand describes a slash command for display in the suggestion list.
type slashCommand struct {
	name string
	desc string
}

// commands is the list of available slash commands.
var commands = []slashCommand{
	{name: "/exit", desc: "Exit golem"},
	{name: "/quit", desc: "Exit golem"},
	{name: "/model", desc: "Switch model (/model <model-id>)"},
}

// agentEventMsg wraps an agent event for delivery through Bubble Tea.
type agentEventMsg struct {
	event event.Event
}

// agentDoneMsg signals that the agent's event channel has closed.
type agentDoneMsg struct{}

// Config holds the dependencies the terminal needs to run.
type Config struct {
	Agent   *agent.Agent
	ModelID string
}

// model holds all TUI state. It implements tea.Model (Init, Update, View).
type model struct {
	agent     *agent.Agent
	modelID   string
	textarea  textarea.Model
	output    *strings.Builder
	eventCh   <-chan event.Event
	cancelRun context.CancelFunc
	waiting   bool
	quitting  bool
	width     int
	height    int
}

var debugLog *log.Logger

// Run starts the terminal UI. It blocks until the user exits.
func Run(cfg Config) error {
	if cfg.Agent == nil {
		return fmt.Errorf("terminal: agent is required")
	}

	f, _ := os.Create("terminal.log")
	debugLog = log.New(f, "", log.Ltime|log.Lmicroseconds)

	ta := textarea.New()
	ta.Placeholder = "Ask golem something..."
	plain := textarea.Style{
		Base:             lipgloss.NewStyle(),
		CursorLine:       lipgloss.NewStyle(),
		CursorLineNumber: lipgloss.NewStyle(),
		EndOfBuffer:      lipgloss.NewStyle(),
		LineNumber:       lipgloss.NewStyle(),
		Placeholder:      lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Prompt:           lipgloss.NewStyle(),
		Text:             lipgloss.NewStyle(),
	}
	ta.FocusedStyle = plain
	ta.BlurredStyle = plain
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()

	m := model{
		agent:    cfg.Agent,
		modelID:  cfg.ModelID,
		textarea: ta,
		output:   &strings.Builder{},
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init returns the initial command (cursor blink).
func (m model) Init() tea.Cmd {
	return textarea.Blink
}

// Update processes a single message and returns the updated model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	debugLog.Printf("UPDATE: msg=%T eventCh=%v", msg, m.eventCh)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.cancelRun != nil {
				m.cancelRun()
				// Drain the event channel so the agent goroutine
				// doesn't block on its deferred TurnCompletedEvent send.
				go func(ch <-chan event.Event) {
					if ch == nil {
						return
					}
					for range ch {
					}
				}(m.eventCh)
			}
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.waiting {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			m.textarea.Reset()
			m.updateTextareaStyle("")

			// Slash commands are handled locally, not sent to the agent.
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}

			m.output.WriteString(promptStyle.Render("You: ") + input + "\n\n")
			m.waiting = true

			ctx, cancel := context.WithCancel(context.Background())
			m.cancelRun = cancel
			m.eventCh = m.agent.Run(ctx, m.modelID, input)
			debugLog.Printf("ENTER: started agent, eventCh=%v", m.eventCh)
			return m, waitForEvent(m.eventCh)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		return m, nil

	case agentEventMsg:
		debugLog.Printf("EVENT: %T eventCh=%v", msg.event, m.eventCh)
		m.handleAgentEvent(msg.event)
		return m, waitForEvent(m.eventCh)

	case agentDoneMsg:
		debugLog.Printf("DONE")
		m.waiting = false
		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}
		m.eventCh = nil
		m.output.WriteString("\n")
		return m, nil
	}

	// Let the textarea handle the key, then update its style based
	// on whether the user is typing a slash command.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.updateTextareaStyle(m.textarea.Value())
	return m, cmd
}

// updateTextareaStyle sets the textarea text color based on whether
// the input looks like a slash command.
func (m *model) updateTextareaStyle(input string) {
	if strings.HasPrefix(input, "/") {
		m.textarea.FocusedStyle.Text = commandStyle
	} else {
		m.textarea.FocusedStyle.Text = plainText
	}
}

// View renders the current TUI state to the terminal.
func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := headerStyle.Render("golem") + dimStyle.Render(" ("+m.modelID+")")
	output := m.output.String()
	suggestions := m.renderSuggestions()

	// Reserve space: header (1) + gap (1) + input gap (1) + prompt (1) + textarea (3) + padding (1).
	reserved := 8
	if suggestions != "" {
		reserved += strings.Count(suggestions, "\n") + 1
	}
	outputHeight := m.height - reserved
	if outputHeight < 1 {
		outputHeight = 1
	}

	lines := strings.Split(output, "\n")
	if len(lines) > outputHeight {
		lines = lines[len(lines)-outputHeight:]
	}
	visible := strings.Join(lines, "\n")

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	sb.WriteString(visible)
	sb.WriteString("\n\n> ")
	sb.WriteString(m.textarea.View())
	if suggestions != "" {
		sb.WriteString("\n")
		sb.WriteString(suggestions)
	}
	return sb.String()
}

// handleCommand processes slash commands locally.
func (m model) handleCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/exit", "/quit":
		m.quitting = true
		return m, tea.Quit
	case "/model":
		if len(parts) < 2 {
			m.output.WriteString(dimStyle.Render("Usage: /model <model-id>") + "\n")
			return m, nil
		}
		m.modelID = parts[1]
		m.output.WriteString(dimStyle.Render("Switched to model: "+m.modelID) + "\n")
		return m, nil
	default:
		m.output.WriteString(errorStyle.Render("Unknown command: "+cmd) + "\n")
		return m, nil
	}
}

// handleAgentEvent writes the appropriate output for a single agent event.
func (m model) handleAgentEvent(ev event.Event) {
	switch e := ev.(type) {
	case event.ThinkingDeltaEvent:
		m.output.WriteString(dimStyle.Render(e.Text))
	case event.TextDeltaEvent:
		m.output.WriteString(e.Text)
	case event.ToolCallStartedEvent:
		m.output.WriteString("\n" + toolStyle.Render("⟩ "+e.Name) + " ")
	case event.ToolCallCompletedEvent:
		if e.IsError {
			m.output.WriteString(errorStyle.Render("✗ "+e.Text) + "\n")
		} else {
			text := e.Text
			if len(text) > maxToolOutputLen {
				text = text[:maxToolOutputLen] + "..."
			}
			m.output.WriteString(dimStyle.Render(text) + "\n")
		}
	case event.UsageEvent:
		m.output.WriteString(dimStyle.Render(
			fmt.Sprintf("\n[%d in / %d out tokens]", e.InputTokens, e.OutputTokens),
		) + "\n")
	case event.ErrorEvent:
		m.output.WriteString(errorStyle.Render("Error: "+e.Err.Error()) + "\n")
	}
}

// matchingCommands returns slash commands that match the given prefix.
func matchingCommands(prefix string) []slashCommand {
	var matches []slashCommand
	for _, c := range commands {
		if strings.HasPrefix(c.name, prefix) {
			matches = append(matches, c)
		}
	}
	return matches
}

// renderSuggestions returns the rendered suggestion list, or an empty
// string if the input doesn't start with "/" or no commands match.
func (m model) renderSuggestions() string {
	input := strings.TrimSpace(m.textarea.Value())
	if !strings.HasPrefix(input, "/") || input == "" {
		return ""
	}

	prefix := strings.Fields(input)[0]
	matches := matchingCommands(prefix)
	if len(matches) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, c := range matches {
		sb.WriteString(commandStyle.Render(c.name))
		sb.WriteString(dimStyle.Render("  " + c.desc))
		if i < len(matches)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// waitForEvent returns a Bubble Tea command that reads the next event
// from the agent's channel. When the channel closes, it returns
// agentDoneMsg to signal the run is complete.
func waitForEvent(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		debugLog.Printf("WAIT: blocking on channel %v", ch)
		ev, ok := <-ch
		debugLog.Printf("WAIT: read ok=%v type=%T", ok, ev)
		if !ok {
			return agentDoneMsg{}
		}
		if _, done := ev.(event.TurnCompletedEvent); done {
			return agentDoneMsg{}
		}
		return agentEventMsg{event: ev}
	}
}
