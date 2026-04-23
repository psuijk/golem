package conversation

import "encoding/json"

// Message is the marker interface for all message types stored in a
// conversation. The unexported isMessage method restricts implementation
// to types defined in this package.
type Message interface {
	isMessage()
}

// UserMessage represents a message from the human user.
type UserMessage struct {
	Text string
}

func (UserMessage) isMessage() {}

// ToolCall represents a single tool invocation requested by the assistant.
// ID links the call to its corresponding ToolResultMessage. Name identifies
// the tool; Input is the raw JSON arguments.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// AssistantMessage represents a response from the LLM. It may contain
// text, tool calls, or both. A nil ToolCalls slice means the assistant
// responded with text only and did not request any tool invocations.
type AssistantMessage struct {
	Text      string
	ToolCalls []ToolCall
}

func (AssistantMessage) isMessage() {}

// ToolResultMessage carries the output of a tool execution back into the
// conversation. ToolCallID links it to the AssistantMessage's ToolCall
// that triggered it. IsError mirrors tool.Result.IsError -- true means
// the tool ran but the operation failed (e.g. file not found), not that
// the tool itself broke.
type ToolResultMessage struct {
	ToolCallID string
	Content    string
	IsError    bool
}

func (ToolResultMessage) isMessage() {}

// Store holds the ordered sequence of messages in a conversation.
// It is append-only and in-memory -- no persistence, no compaction.
// The LLM adapter reads from it to build the API payload each turn.
type Store struct {
	messages []Message
}

// New returns an empty Store ready to accumulate messages.
func New() *Store {
	return &Store{}
}

// Append adds a message to the end of the conversation.
func (s *Store) Append(msg Message) {
	s.messages = append(s.messages, msg)
}

// Messages returns the full conversation history in order. The returned
// slice is the Store's internal state -- callers should not modify it.
func (s *Store) Messages() []Message {
	return s.messages
}
