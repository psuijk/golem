package event

import (
	"encoding/json"

	"github.com/psuijk/golem/internal/tool"
)

type Event interface {
	isEvent()
}

type ToolCallStartedEvent struct {
	Name  string
	Input json.RawMessage
}

func (ToolCallStartedEvent) isEvent() {}

type ToolCallCompletedEvent struct {
	Name   string
	Result *tool.Result
	Err    error
}

func (ToolCallCompletedEvent) isEvent() {}

type UserMessageEvent struct {
	Text string
}

func (UserMessageEvent) isEvent() {}

type TurnCompletedEvent struct{}

func (TurnCompletedEvent) isEvent() {}
