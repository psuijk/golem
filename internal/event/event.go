package event

import "encoding/json"

// Event is the marker interface for all event types emitted by the agent
// loop into its event channel. The unexported isEvent method restricts
// implementation to types defined in this package.
type Event interface {
	isEvent()
}

// TextDeltaEvent is emitted for each chunk of text streamed from the LLM.
// Consumers concatenate these to build the full assistant response.
type TextDeltaEvent struct {
	Text string
}

func (TextDeltaEvent) isEvent() {}

// ToolCallStartedEvent is emitted immediately before the loop dispatches
// a tool call. Name identifies the tool; Input is the raw JSON arguments.
type ToolCallStartedEvent struct {
	Name  string
	Input json.RawMessage
}

func (ToolCallStartedEvent) isEvent() {}

// ToolCallCompletedEvent is emitted after a tool call returns. Text
// holds the tool's output. IsError is true when the tool ran but the
// operation failed (e.g. file not found) or when the dispatch itself
// failed (e.g. unknown tool).
type ToolCallCompletedEvent struct {
	Name    string
	Text    string
	IsError bool
}

func (ToolCallCompletedEvent) isEvent() {}

// TurnCompletedEvent signals that the loop has finished a turn. It is
// always the last event emitted before the channel closes, regardless
// of whether the turn ended normally, hit the step limit, or was
// cancelled via context.
type TurnCompletedEvent struct{}

func (TurnCompletedEvent) isEvent() {}

// UsageEvent is emitted after each LLM call completes, reporting token
// consumption for that call. Consumers can aggregate across turns for
// total cost tracking.
type UsageEvent struct {
	InputTokens  int
	OutputTokens int
}

func (UsageEvent) isEvent() {}

// ErrorEvent is emitted when a non-recoverable error occurs, such as a
// provider failure or stream error. The agent will close the channel
// shortly after emitting this event.
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) isEvent() {}
