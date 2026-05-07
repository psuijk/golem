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

	d := tool.NewDispatcher(r)
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

	for evt := range events {
		switch e := evt.(type) {
		case event.ToolCallStartedEvent:
			t.Logf("[started]   %s(%s)\n", e.Name, e.Input)
		case event.ToolCallCompletedEvent:
			if e.Err != nil {
				t.Errorf("[error]     %s: %v\n", e.Name, e.Err)
			}
			t.Logf("[completed] %s -> %+v\n", e.Name, e.Result)
		case event.TurnCompletedEvent:
			t.Logf("[done]      turn completed")
			return
		}
	}
}

func TestLoopMaxSteps(t *testing.T) {
	ctx := context.Background()

	r := tool.NewRegistry()
	if err := r.Register(bash.New(5 * time.Second)); err != nil {
		t.Fatalf("register: %v", err)
	}

	d := tool.NewDispatcher(r)
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

	d := tool.NewDispatcher(r)
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
	d := tool.NewDispatcher(r)
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
