package conversation_test

import (
	"encoding/json"
	"testing"

	"github.com/psuijk/golem/internal/conversation"
)

func TestEmptyStore(t *testing.T) {
	s := conversation.New()
	msgs := s.Messages()

	if len(msgs) != 0 {
		t.Fatalf("messages should be 0 but was %d", len(msgs))
	}
}

func TestAppendAndRetrieve(t *testing.T) {
	s := conversation.New()
	s.Append(conversation.UserMessage{Text: "I am a user message"})
	s.Append(conversation.AssistantMessage{Text: "I am an assistant message"})
	s.Append(conversation.ToolResultMessage{ToolCallID: "5", Content: "I am a tool result message", IsError: false})

	msgs := s.Messages()

	if len(msgs) != 3 {
		t.Fatalf("messages should be 3 but was %d", len(msgs))
	}

	m0, ok := msgs[0].(conversation.UserMessage)
	if !ok {
		t.Fatalf("msgs[0] is %T, want UserMessage", msgs[0])
	}
	if m0.Text != "I am a user message" {
		t.Errorf("msgs[0].Text = %q, want %q", m0.Text, "I am a user message")
	}

	m1, ok := msgs[1].(conversation.AssistantMessage)
	if !ok {
		t.Fatalf("msgs[1] is %T, want AssistantMessage", msgs[1])
	}
	if m1.Text != "I am an assistant message" {
		t.Errorf("msgs[1].Text = %q, want %q", m1.Text, "I am an assistant message")
	}

	m2, ok := msgs[2].(conversation.ToolResultMessage)
	if !ok {
		t.Fatalf("msgs[2] is %T, want ToolResultMessage", msgs[2])
	}
	if m2.Content != "I am a tool result message" {
		t.Errorf("msgs[2].Content = %q, want %q", m2.Content, "I am a tool result message")
	}
	if m2.IsError != false {
		t.Errorf("msgs[2].IsError = %v, want false", m2.IsError)
	}
	if m2.ToolCallID != "5" {
		t.Errorf("msgs[2].ToolCallID = %q, want %q", m2.ToolCallID, "5")
	}
}

func TestAppendPreservesOrder(t *testing.T) {
	s := conversation.New()
	s.Append(conversation.UserMessage{Text: "first"})
	s.Append(conversation.UserMessage{Text: "second"})
	s.Append(conversation.UserMessage{Text: "third"})

	msgs := s.Messages()

	if len(msgs) != 3 {
		t.Fatalf("messages should be 3 but was %d", len(msgs))
	}

	expected := []string{"first", "second", "third"}
	for i, want := range expected {
		m, ok := msgs[i].(conversation.UserMessage)
		if !ok {
			t.Fatalf("msgs[%d] is %T, want UserMessage", i, msgs[i])
		}
		if m.Text != want {
			t.Errorf("msgs[%d].Text = %q, want %q", i, m.Text, want)
		}
	}
}

func TestAssistantMessageWithToolCalls(t *testing.T) {
	s := conversation.New()
	s.Append(conversation.AssistantMessage{
		Text: "I'll read that file",
		ToolCalls: []conversation.ToolCall{
			{ID: "tc_1", Name: "readfile", Input: json.RawMessage(`{"path":"foo.txt"}`)},
			{ID: "tc_2", Name: "bash", Input: json.RawMessage(`{"command":"echo hi"}`)},
		},
	})

	msgs := s.Messages()

	if len(msgs) != 1 {
		t.Fatalf("messages should be 1 but was %d", len(msgs))
	}

	m, ok := msgs[0].(conversation.AssistantMessage)
	if !ok {
		t.Fatalf("msgs[0] is %T, want AssistantMessage", msgs[0])
	}
	if m.Text != "I'll read that file" {
		t.Errorf("Text = %q, want %q", m.Text, "I'll read that file")
	}
	if len(m.ToolCalls) != 2 {
		t.Fatalf("ToolCalls length = %d, want 2", len(m.ToolCalls))
	}
	if m.ToolCalls[0].Name != "readfile" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", m.ToolCalls[0].Name, "readfile")
	}
	if m.ToolCalls[0].ID != "tc_1" {
		t.Errorf("ToolCalls[0].ID = %q, want %q", m.ToolCalls[0].ID, "tc_1")
	}
	if m.ToolCalls[1].Name != "bash" {
		t.Errorf("ToolCalls[1].Name = %q, want %q", m.ToolCalls[1].Name, "bash")
	}
}

func TestAssistantMessageWithoutToolCalls(t *testing.T) {
	s := conversation.New()
	s.Append(conversation.AssistantMessage{Text: "just text, no tools"})

	msgs := s.Messages()

	if len(msgs) != 1 {
		t.Fatalf("messages should be 1 but was %d", len(msgs))
	}

	m, ok := msgs[0].(conversation.AssistantMessage)
	if !ok {
		t.Fatalf("msgs[0] is %T, want AssistantMessage", msgs[0])
	}
	if m.Text != "just text, no tools" {
		t.Errorf("Text = %q, want %q", m.Text, "just text, no tools")
	}
	if len(m.ToolCalls) != 0 {
		t.Errorf("ToolCalls length = %d, want 0", len(m.ToolCalls))
	}
}
