// Package terminal provides the interactive TUI for golem, built on
// Bubble Tea. It owns the input loop, renders agent events as they
// stream in, and handles slash commands.
package terminal

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
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
	toolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	commandStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	approvalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	plainText     = lipgloss.NewStyle()
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
	runID int
	event event.Event
}

// agentDoneMsg signals that the agent's event channel has closed.
type agentDoneMsg struct{ runID int }

// Config holds the dependencies the terminal needs to run.
type Config struct {
	Agent        *agent.Agent
	ModelID      string
	FullThinking bool // show all thinking tokens instead of a rolling window
}

// pendingApproval holds state while waiting for the user to respond
// to a tool approval prompt.
type pendingApproval struct {
	name     string
	input    string
	response chan event.ApprovalResponse
}

// model holds all TUI state. It implements tea.Model (Init, Update, View).
type model struct {
	runID           int
	agent           *agent.Agent
	modelID         string
	lastInput       string
	textarea        textarea.Model
	inputQueue      []string
	turnStart       int
	block           *strings.Builder
	output          []string
	thinking        *strings.Builder // accumulates thinking tokens separately
	eventCh         <-chan event.Event
	cancelRun       context.CancelFunc
	approval        *pendingApproval // non-nil when waiting for user to approve a tool call
	fullThinking    bool
	waiting         bool
	quitting        bool
	width           int
	height          int
	totalInputToks  int
	totalOutputToks int
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
		agent:        cfg.Agent,
		modelID:      cfg.ModelID,
		textarea:     ta,
		output:       nil,
		block:        &strings.Builder{},
		thinking:     &strings.Builder{},
		fullThinking: cfg.FullThinking,
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
		// While an approval prompt is active, only handle approval keys.
		if m.approval != nil {
			return m.handleApprovalKey(msg)
		}

		switch msg.Type {
		case tea.KeyEsc:
			if m.cancelRun != nil {
				m.cancelActiveRun()
				m.output = m.output[:m.turnStart]
				m.block.Reset()
				m.thinking.Reset()
				m.textarea.SetValue(m.lastInput)
				m.updateTextareaStyle(m.lastInput)
				m.lastInput = ""
			}
			m.waiting = false
			return m, nil
		case tea.KeyCtrlC:
			m.cancelActiveRun()
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			input := strings.TrimSpace(m.textarea.Value())
			debugLog.Printf("ENTER: raw=%q trimmed=%q", m.textarea.Value(), input)
			if input == "" {
				return m, nil
			}

			m.textarea.Reset()
			m.updateTextareaStyle("")

			// Slash commands are handled locally, not sent to the agent.
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}

			if m.waiting {
				m.inputQueue = append(m.inputQueue, input)
				return m, nil
			}

			return m.startRun(input)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		return m, nil

	case agentEventMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		debugLog.Printf("EVENT: %T eventCh=%v", msg.event, m.eventCh)
		if u, ok := msg.event.(event.UsageEvent); ok {
			m.totalInputToks += u.InputTokens
			m.totalOutputToks += u.OutputTokens
		}
		// ToolApprovalEvent requires user input before the agent can
		// continue. Set the approval state and don't chain the next
		// waitForEvent — it resumes after the user responds.
		if a, ok := msg.event.(event.ToolApprovalEvent); ok {
			m.approval = &pendingApproval{
				name:     a.Name,
				input:    string(a.Input),
				response: a.Response,
			}
			return m, nil
		}
		m = m.handleAgentEvent(msg.event)
		return m, waitForEvent(m.eventCh, m.runID)

	case agentDoneMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		debugLog.Printf("DONE")
		m.waiting = false
		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}
		m.eventCh = nil
		m = m.flushBlock()
		if len(m.inputQueue) != 0 {
			next := m.inputQueue[0]
			m.inputQueue = m.inputQueue[1:]
			return m.startRun(next)
		}
		return m, nil
	}

	// Let the textarea handle the key, then update its style based
	// on whether the user is typing a slash command.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.updateTextareaStyle(m.textarea.Value())
	return m, cmd
}

func (m model) cancelActiveRun() {
	if m.cancelRun == nil {
		return
	}
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

func (m model) startRun(input string) (tea.Model, tea.Cmd) {
	// Mark where this turn's entries begin in output, so Esc can truncate
	// the whole turn back to here.
	m.turnStart = len(m.output)
	m.lastInput = input
	m.block.WriteString(promptStyle.Render("You: ") + input + "\n\n")
	m.waiting = true
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelRun = cancel
	m.eventCh = m.agent.Run(ctx, m.modelID, input)
	m.runID++
	debugLog.Printf("ENTER: started agent, eventCh=%v", m.eventCh)
	return m, waitForEvent(m.eventCh, m.runID)
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

	header := headerStyle.Render("golem") + dimStyle.Render(" ("+m.modelID+")") +
		dimStyle.Render(fmt.Sprintf("  [%d in / %d out]", m.totalInputToks, m.totalOutputToks))
	block := m.block.String()
	suggestions := m.renderSuggestions()
	thinkingWindow := ""
	if !m.fullThinking {
		thinkingWindow = m.renderThinkingWindow()
	}
	approval := m.renderApproval()

	// Reserve space: header (1) + gap (1) + input gap (1) + prompt (1) + textarea (3) + padding (1).
	reserved := 8
	if suggestions != "" {
		reserved += strings.Count(suggestions, "\n") + 1
	}
	if thinkingWindow != "" {
		reserved += strings.Count(thinkingWindow, "\n") + 1
	}
	if approval != "" {
		reserved += strings.Count(approval, "\n") + 1
	}
	outputHeight := max(m.height-reserved, 1)

	var lines []string
	for _, line := range m.output {
		lines = append(lines, strings.Split(line, "\n")...)
	}

	lines = append(lines, strings.Split(block, "\n")...)
	if len(lines) > outputHeight {
		lines = lines[len(lines)-outputHeight:]
	}
	visible := strings.Join(lines, "\n")

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	sb.WriteString(visible)
	if thinkingWindow != "" {
		sb.WriteString("\n")
		sb.WriteString(thinkingWindow)
	}
	if approval != "" {
		sb.WriteString("\n")
		sb.WriteString(approval)
	}
	sb.WriteString("\n\n> ")
	sb.WriteString(m.textarea.View())
	if suggestions != "" {
		sb.WriteString("\n")
		sb.WriteString(suggestions)
		debugLog.Printf("VIEW: suggestions=%q", suggestions)
	}
	return sb.String()
}

// handleApprovalKey processes a keypress while a tool approval prompt
// is active. Sends the user's decision on the approval's response
// channel and resumes event processing.
func (m model) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var response event.ApprovalResponse
	var label string

	switch msg.String() {
	case "y":
		response = event.ApproveOnce
		label = "approved"
	case "n", "esc":
		response = event.Deny
		label = "denied"
	case "a":
		response = event.ApproveAlways
		label = "approved (always)"
	default:
		return m, nil // ignore other keys
	}

	m.thinking.WriteString(dimStyle.Render("  → "+label) + "\n")
	m.approval.response <- response
	m.approval = nil
	return m, waitForEvent(m.eventCh, m.runID)
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
			m.block.WriteString(dimStyle.Render("Usage: /model <model-id>") + "\n")
			return m, nil
		}
		m.modelID = parts[1]
		m.block.WriteString(dimStyle.Render("Switched to model: "+m.modelID) + "\n")
		return m, nil
	default:
		m.block.WriteString(errorStyle.Render("Unknown command: "+cmd) + "\n")
		return m, nil
	}
}

// flushBlock appends the current block to output as a finished entry and
// resets it. It's a no-op when the block is empty, so a boundary that fires
// with nothing pending doesn't add a blank entry. Returns the updated model
// because output is a value field — the append is lost otherwise.
func (m model) flushBlock() model {
	if m.block.Len() == 0 {
		return m
	}
	m.output = append(m.output, m.block.String())
	m.block.Reset()
	return m
}

// handleAgentEvent updates the transcript for a single agent event and
// returns the updated model (needed so output appends persist).
func (m model) handleAgentEvent(ev event.Event) model {
	switch e := ev.(type) {
	case event.ThinkingDeltaEvent:
		if m.fullThinking {
			m.thinking.WriteString(dimStyle.Render(e.Text))
		} else {
			m.thinking.WriteString(e.Text)
		}
	case event.TextDeltaEvent:
		if !m.fullThinking && m.thinking.Len() > 0 {
			m.thinking.Reset()
		}
		m.block.WriteString(e.Text)
	case event.ToolCallStartedEvent:
		// A tool call ends the current text block; flush it, then start a
		// fresh block for the tool header.
		m = m.flushBlock()
		m.block.WriteString("\n" + toolStyle.Render("⟩ "+e.Name+" ") + dimStyle.Render(string(e.Input)) + "\n")
	case event.ToolCallCompletedEvent:
		if e.IsError {
			// Denied tools emit no ToolCallStartedEvent, so name the tool
			// here — otherwise the denial has no indication of which tool.
			m.block.WriteString(errorStyle.Render("✗ "+e.Name+"→"+e.Text) + "\n")
		} else {
			text := e.Text
			if len(text) > maxToolOutputLen {
				text = text[:maxToolOutputLen] + "..."
			}
			m.block.WriteString(dimStyle.Render(text) + "\n")
		}
		m = m.flushBlock()
	case event.UsageEvent:
		m.block.WriteString(dimStyle.Render(
			fmt.Sprintf("\n[%d in / %d out tokens]", e.InputTokens, e.OutputTokens),
		) + "\n")
	case event.ErrorEvent:
		m.block.WriteString(errorStyle.Render("Error: "+e.Err.Error()) + "\n")
		m = m.flushBlock()
	}
	return m
}

// renderApproval returns the pending tool-approval prompt, or "" if no
// approval is awaiting a decision. Rendered transiently by View — never
// persisted to the transcript, so it vanishes once resolved.
func (m model) renderApproval() string {
	if m.approval == nil {
		return ""
	}
	return approvalStyle.Render("⟩ "+m.approval.name+" ") + dimStyle.Render(m.approval.input) + "\n" +
		approvalStyle.Render("  [y] approve  [n] deny  [a] approve always")
}

// thinkingWindowLines is the number of rolling lines shown in the
// View when fullThinking is false.
const thinkingWindowLines = 2

// renderThinkingWindow returns the last thinkingWindowLines of the
// accumulated thinking text, or empty string if not thinking.
func (m model) renderThinkingWindow() string {
	if m.thinking.Len() == 0 {
		return ""
	}
	text := m.thinking.String()
	lines := strings.Split(text, "\n")
	if len(lines) > thinkingWindowLines {
		lines = lines[len(lines)-thinkingWindowLines:]
	}
	maxWidth := m.width - 4
	if maxWidth < 20 {
		maxWidth = 80
	}
	var sb strings.Builder
	sb.WriteString(dimStyle.Render("thinking...") + "\n")
	for _, line := range lines {
		if len(line) > maxWidth {
			line = line[:maxWidth]
		}
		sb.WriteString(dimStyle.Render(line))
		sb.WriteString("\n")
	}
	return sb.String()
}

// matchingCommands returns slash commands that match the given prefix,
// sorted by name length (shortest/closest match first).
func matchingCommands(prefix string) []slashCommand {
	var matches []slashCommand
	for _, c := range commands {
		if strings.HasPrefix(c.name, prefix) {
			matches = append(matches, c)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i].name) < len(matches[j].name)
	})
	return matches
}

// maxVisibleSuggestions is the maximum number of command suggestions
// shown below the input at once.
const maxVisibleSuggestions = 5

// renderSuggestions returns the rendered suggestion list, or an empty
// string if the input doesn't start with "/" or no commands match.
// Shows at most maxVisibleSuggestions, always padded to that height
// to prevent rendering artifacts.
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

	visible := matches
	if len(visible) > maxVisibleSuggestions {
		visible = visible[:maxVisibleSuggestions]
	}

	var sb strings.Builder
	for i, c := range visible {
		sb.WriteString(commandStyle.Render(c.name))
		sb.WriteString(dimStyle.Render("  " + c.desc))
		if i < len(visible)-1 {
			sb.WriteString("\n")
		}
	}
	// Always pad to maxVisibleSuggestions so the area stays stable.
	for i := len(visible); i < maxVisibleSuggestions; i++ {
		sb.WriteString("\n")
	}
	return sb.String()
}

// waitForEvent returns a Bubble Tea command that reads the next event
// from the agent's channel. When the channel closes, it returns
// agentDoneMsg to signal the run is complete.
func waitForEvent(ch <-chan event.Event, id int) tea.Cmd {
	return func() tea.Msg {
		debugLog.Printf("WAIT: blocking on channel %v", ch)
		ev, ok := <-ch
		debugLog.Printf("WAIT: read ok=%v type=%T", ok, ev)
		if !ok {
			return agentDoneMsg{runID: id}
		}
		if _, done := ev.(event.TurnCompletedEvent); done {
			return agentDoneMsg{runID: id}
		}
		return agentEventMsg{runID: id, event: ev}
	}
}
