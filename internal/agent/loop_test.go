package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
	"github.com/psuijk/golem/internal/tools/readfile"
	"github.com/psuijk/golem/internal/tools/writefile"
)

func TestLoopHappyPath(t *testing.T) {
	ctx := context.Background()

	r := tool.NewRegistry()

	err := r.Register(bash.New(time.Second * 30))
	if err != nil {
		t.Fatalf("failed to register bash tool: %v", err)
	}
	err = r.Register(writefile.New())
	if err != nil {
		t.Fatalf("failed to register writefile tool: %v", err)
	}
	err = r.Register(readfile.New(1 << 20))
	if err != nil {
		t.Fatalf("failed to register readfile tool: %v", err)
	}

	d := tool.NewDispatcher(r, nil)
	store := conversation.New()
	cfg := agent.Config{MaxSteps: 10, Dispatcher: d, Store: store}

	a := agent.New(cfg)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	events := a.Run(ctx, []agent.ToolCall{
		{Name: "bash", Input: json.RawMessage(`{"command": "echo hello"}`)},
		{Name: "writefile", Input: json.RawMessage(fmt.Sprintf(`{"path":%q,"content":"hello from loop"}`, path))},
		{Name: "readfile", Input: json.RawMessage(fmt.Sprintf(`{"path":%q}`, path))},
	})

	var collected []event.Event
	for evt := range events {
		collected = append(collected, evt)
	}

	// 3 tool calls: started, completed x3 + turn completed = 7 events
	if len(collected) != 7 {
		t.Fatalf("got %d events, want 7", len(collected))
	}

	// Store should have 6 messages: 3 AssistantMessage + 3 ToolResultMessage
	msgs := store.Messages()
	if len(msgs) != 6 {
		t.Fatalf("store has %d messages, want 6", len(msgs))
	}

	// Verify alternating pattern: AssistantMessage, ToolResultMessage, ...
	for i := 0; i < 6; i += 2 {
		am, ok := msgs[i].(conversation.AssistantMessage)
		if !ok {
			t.Fatalf("msgs[%d] is %T, want AssistantMessage", i, msgs[i])
		}
		if len(am.ToolCalls) != 1 {
			t.Errorf("msgs[%d] ToolCalls length = %d, want 1", i, len(am.ToolCalls))
		}

		tr, ok := msgs[i+1].(conversation.ToolResultMessage)
		if !ok {
			t.Fatalf("msgs[%d] is %T, want ToolResultMessage", i+1, msgs[i+1])
		}
		if tr.Content == "" {
			t.Errorf("msgs[%d] Content is empty", i+1)
		}
		if tr.ToolCallID != am.ToolCalls[0].ID {
			t.Errorf("msgs[%d] ToolCallID = %q, want %q", i+1, tr.ToolCallID, am.ToolCalls[0].ID)
		}
	}

	// Spot-check first tool call is bash
	am0 := msgs[0].(conversation.AssistantMessage)
	if am0.ToolCalls[0].Name != "bash" {
		t.Errorf("first tool call Name = %q, want %q", am0.ToolCalls[0].Name, "bash")
	}
}

func TestLoopMaxSteps(t *testing.T) {
	ctx := context.Background()

	r := tool.NewRegistry()
	if err := r.Register(bash.New(5 * time.Second)); err != nil {
		t.Fatalf("register: %v", err)
	}

	d := tool.NewDispatcher(r, nil)
	a := agent.New(agent.Config{MaxSteps: 2, Dispatcher: d})

	events := a.Run(ctx, []agent.ToolCall{
		{Name: "bash", Input: json.RawMessage(`{"command": "echo one"}`)},
		{Name: "bash", Input: json.RawMessage(`{"command": "echo two"}`)},
		{Name: "bash", Input: json.RawMessage(`{"command": "echo three"}`)},
	})

	var collected []event.Event
	for evt := range events {
		collected = append(collected, evt)
	}

	// 2 calls dispatched (not 3): started, completed, started, completed, turn completed
	if len(collected) != 5 {
		t.Fatalf("got %d events, want 5", len(collected))
	}

	// last event should be TurnCompletedEvent
	if _, ok := collected[4].(event.TurnCompletedEvent); !ok {
		t.Errorf("last event = %T, want TurnCompletedEvent", collected[4])
	}
}

func TestLoopContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately, before Run even starts

	r := tool.NewRegistry()
	if err := r.Register(bash.New(5 * time.Second)); err != nil {
		t.Fatalf("register: %v", err)
	}

	d := tool.NewDispatcher(r, nil)
	a := agent.New(agent.Config{MaxSteps: 10, Dispatcher: d})

	events := a.Run(ctx, []agent.ToolCall{
		{Name: "bash", Input: json.RawMessage(`{"command": "echo should not run"}`)},
	})

	var collected []event.Event
	for evt := range events {
		collected = append(collected, evt)
	}

	// context was cancelled before any calls, so only TurnCompletedEvent
	if len(collected) != 1 {
		t.Fatalf("got %d events, want 1", len(collected))
	}
	if _, ok := collected[0].(event.TurnCompletedEvent); !ok {
		t.Errorf("event = %T, want TurnCompletedEvent", collected[0])
	}
}

func TestLoopEmptyCalls(t *testing.T) {
	ctx := context.Background()

	r := tool.NewRegistry()
	d := tool.NewDispatcher(r, nil)
	a := agent.New(agent.Config{MaxSteps: 10, Dispatcher: d})

	events := a.Run(ctx, []agent.ToolCall{})

	var collected []event.Event
	for evt := range events {
		collected = append(collected, evt)
	}

	// no calls, just TurnCompletedEvent
	if len(collected) != 1 {
		t.Fatalf("got %d events, want 1", len(collected))
	}
	if _, ok := collected[0].(event.TurnCompletedEvent); !ok {
		t.Errorf("event = %T, want TurnCompletedEvent", collected[0])
	}
}
