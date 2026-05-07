package llm

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Content interface{ isContent() }

type TextContent struct{ Text string }

func (TextContent) isContent() {}

type ToolUseContent struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolUseContent) isContent() {}

type ToolResultContent struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (ToolResultContent) isContent() {}

type Message struct {
	Role    Role
	Content []Content
}

type StreamEvent interface{ isStreamEvent() }

type TextDelta struct {
	Text string
}

func (TextDelta) isStreamEvent() {}

type ToolUseEvent struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolUseEvent) isStreamEvent() {}

type MessageStop struct {
	StopReason string
	Usage      Usage
}

func (MessageStop) isStreamEvent() {}

type ErrorEvent struct {
	Err error
}

func (ErrorEvent) isStreamEvent() {}

type Usage struct {
	InputTokens  int
	OutputTokens int
}
