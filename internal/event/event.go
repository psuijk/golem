package event

import (
	"encoding/json"

	"github.com/psuijk/golem/internal/tool"
)

// Event is the marker interface for all event types emitted by the agent
// loop into its event channel. The unexported isEvent method restricts
// implementation to types defined in this package.
type Event interface {
	isEvent()
}

// ToolCallStartedEvent is emitted immediately before the loop dispatches
// a tool call. Name identifies the tool; Input is the raw JSON arguments.
type ToolCallStartedEvent struct {
	Name  string
	Input json.RawMessage
}

func (ToolCallStartedEvent) isEvent() {}

// ToolCallCompletedEvent is emitted after a tool call returns. Result
// holds the tool's output (nil if the tool returned a Go error). Err is
// non-nil only for caller bugs (e.g. malformed input), not for expected
// operational failures (which use Result.IsError).
type ToolCallCompletedEvent struct {
	Name   string
	Result *tool.Result
	Err    error
}

func (ToolCallCompletedEvent) isEvent() {}

// UserMessageEvent is emitted when a user message enters the conversation.
type UserMessageEvent struct {
	Text string
}

func (UserMessageEvent) isEvent() {}

// TurnCompletedEvent signals that the loop has finished a turn. It is
// always the last event emitted before the channel closes, regardless
// of whether the turn ended normally, hit the step limit, or was
// cancelled via context.
type TurnCompletedEvent struct{}

func (TurnCompletedEvent) isEvent() {}
