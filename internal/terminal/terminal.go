// Package terminal provides the interactive TUI for golem, built on
// Bubble Tea. It owns the input loop, renders agent events as they
// stream in, and handles slash commands.
package terminal

import (
	"context"
	"fmt"
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
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
)

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

// Run starts the terminal UI. It blocks until the user exits.
func Run(cfg Config) error {
	if cfg.Agent == nil {
		return fmt.Errorf("terminal: agent is required")
	}

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

			// Slash commands are handled locally, not sent to the agent.
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}

			m.output.WriteString(promptStyle.Render("You: ") + input + "\n\n")
			m.waiting = true

			ctx, cancel := context.WithCancel(context.Background())
			m.cancelRun = cancel
			m.eventCh = m.agent.Run(ctx, m.modelID, input)
			return m, waitForEvent(m.eventCh)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		return m, nil

	case agentEventMsg:
		m.handleAgentEvent(msg.event)
		return m, waitForEvent(m.eventCh)

	case agentDoneMsg:
		m.waiting = false
		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}
		m.eventCh = nil
		m.output.WriteString("\n")
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// View renders the current TUI state to the terminal.
func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := headerStyle.Render("golem") + dimStyle.Render(" ("+m.modelID+")")
	output := m.output.String()

	// Reserve space: header (1) + gap (1) + input gap (1) + prompt (1) + textarea (3) + padding (1).
	outputHeight := m.height - 8
	if outputHeight < 1 {
		outputHeight = 1
	}

	lines := strings.Split(output, "\n")
	if len(lines) > outputHeight {
		lines = lines[len(lines)-outputHeight:]
	}
	visible := strings.Join(lines, "\n")

	return fmt.Sprintf("%s\n\n%s\n\n> %s", header, visible, m.textarea.View())
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

// waitForEvent returns a Bubble Tea command that reads the next event
// from the agent's channel. When the channel closes, it returns
// agentDoneMsg to signal the run is complete.
func waitForEvent(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		if _, done := ev.(event.TurnCompletedEvent); done {
			return agentDoneMsg{}
		}
		return agentEventMsg{event: ev}
	}
}
