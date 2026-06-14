package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/tool"
)

// mockProvider implements llm.Provider with a sequence of canned responses.
// Each call to Stream pops the next response off the list.
type mockProvider struct {
	responses [][]llm.StreamEvent
	callCount int
}

func (m *mockProvider) Stream(_ context.Context, _ llm.RequestParams) (<-chan llm.StreamEvent, error) {
	if m.callCount >= len(m.responses) {
		return nil, errors.New("no more responses")
	}
	events := m.responses[m.callCount]
	m.callCount++

	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		for _, e := range events {
			ch <- e
		}
	}()
	return ch, nil
}

// echoTool is a minimal tool that returns its input as text.
type echoTool struct{}

func (echoTool) Name() string                { return "echo" }
func (echoTool) Description() string          { return "echoes input" }
func (echoTool) Schema() json.RawMessage      { return json.RawMessage(`{}`) }
func (echoTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	return &tool.Result{Text: string(input)}, nil
}

func newDispatcher(tools ...tool.Interface) *tool.Dispatcher {
	d, err := tool.NewDispatcher(tools, nil)
	if err != nil {
		panic(err)
	}
	return d
}

// mockResolver returns a fixed provider for any model ID.
type mockResolver struct {
	provider llm.Provider
}

func (m *mockResolver) Resolve(modelID string) (llm.Provider, error) {
	return m.provider, nil
}

func newAgent(t *testing.T, provider llm.Provider, dispatcher *tool.Dispatcher, store *conversation.Store) *agent.Agent {
	t.Helper()
	a, err := agent.New(agent.Config{
		Resolver:   &mockResolver{provider: provider},
		Dispatcher: dispatcher,
		Store:      store,
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return a
}

func collect(ch <-chan event.Event) []event.Event {
	var events []event.Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestRunTextOnly(t *testing.T) {
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{
				llm.TextDelta{Text: "hello "},
				llm.TextDelta{Text: "world"},
				llm.MessageStop{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
			},
		},
	}

	a := newAgent(t, provider, newDispatcher(), conversation.New())
	events := collect(a.Run(context.Background(), "test-model", "hi"))

	// Expect: TextDelta, TextDelta, UsageEvent, TurnCompletedEvent
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}

	if td, ok := events[0].(event.TextDeltaEvent); !ok || td.Text != "hello " {
		t.Errorf("events[0] = %+v, want TextDeltaEvent{hello }", events[0])
	}
	if td, ok := events[1].(event.TextDeltaEvent); !ok || td.Text != "world" {
		t.Errorf("events[1] = %+v, want TextDeltaEvent{world}", events[1])
	}
	if u, ok := events[2].(event.UsageEvent); !ok || u.InputTokens != 10 || u.OutputTokens != 5 {
		t.Errorf("events[2] = %+v, want UsageEvent{10, 5}", events[2])
	}
	if _, ok := events[3].(event.TurnCompletedEvent); !ok {
		t.Errorf("events[3] = %T, want TurnCompletedEvent", events[3])
	}
}

func TestRunWithToolCalls(t *testing.T) {
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			// First LLM call: request a tool
			{
				llm.TextDelta{Text: "let me check"},
				llm.ToolUseEvent{ID: "call_1", Name: "echo", Input: json.RawMessage(`"ping"`)},
				llm.MessageStop{Usage: llm.Usage{InputTokens: 10, OutputTokens: 8}},
			},
			// Second LLM call: final text response
			{
				llm.TextDelta{Text: "done"},
				llm.MessageStop{Usage: llm.Usage{InputTokens: 20, OutputTokens: 3}},
			},
		},
	}

	a := newAgent(t, provider, newDispatcher(echoTool{}), conversation.New())
	events := collect(a.Run(context.Background(), "test-model", "do it"))

	// Expect: TextDelta("let me check"), UsageEvent, ToolCallStarted, ToolCallCompleted,
	//         TextDelta("done"), UsageEvent, TurnCompleted
	if len(events) != 7 {
		t.Fatalf("got %d events, want 7: %+v", len(events), events)
	}

	if _, ok := events[0].(event.TextDeltaEvent); !ok {
		t.Errorf("events[0] = %T, want TextDeltaEvent", events[0])
	}
	if _, ok := events[1].(event.UsageEvent); !ok {
		t.Errorf("events[1] = %T, want UsageEvent", events[1])
	}
	if s, ok := events[2].(event.ToolCallStartedEvent); !ok || s.Name != "echo" {
		t.Errorf("events[2] = %+v, want ToolCallStartedEvent{echo}", events[2])
	}
	if c, ok := events[3].(event.ToolCallCompletedEvent); !ok || c.Text != `"ping"` {
		t.Errorf("events[3] = %+v, want ToolCallCompletedEvent with text \"ping\"", events[3])
	}
	if _, ok := events[4].(event.TextDeltaEvent); !ok {
		t.Errorf("events[4] = %T, want TextDeltaEvent", events[4])
	}
	if _, ok := events[6].(event.TurnCompletedEvent); !ok {
		t.Errorf("events[6] = %T, want TurnCompletedEvent", events[6])
	}

	if provider.callCount != 2 {
		t.Errorf("provider called %d times, want 2", provider.callCount)
	}
}

func TestRunMaxTurns(t *testing.T) {
	// Provider always returns a tool call, so the agent would loop forever
	// without MaxTurns.
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{
				llm.ToolUseEvent{ID: "call_1", Name: "echo", Input: json.RawMessage(`"a"`)},
				llm.MessageStop{},
			},
			{
				llm.ToolUseEvent{ID: "call_2", Name: "echo", Input: json.RawMessage(`"b"`)},
				llm.MessageStop{},
			},
			{
				llm.TextDelta{Text: "should not reach"},
				llm.MessageStop{},
			},
		},
	}

	a, err := agent.New(agent.Config{
		MaxTurns:   2,
		Resolver:   &mockResolver{provider: provider},
		Dispatcher: newDispatcher(echoTool{}),
		Store:      conversation.New(),
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	events := collect(a.Run(context.Background(), "test-model", "go"))

	// Should have called the provider exactly 2 times
	if provider.callCount != 2 {
		t.Errorf("provider called %d times, want 2", provider.callCount)
	}

	// Last event should be TurnCompleted
	last := events[len(events)-1]
	if _, ok := last.(event.TurnCompletedEvent); !ok {
		t.Errorf("last event = %T, want TurnCompletedEvent", last)
	}
}

func TestRunMaxSteps(t *testing.T) {
	// Single LLM response with 3 tool calls, but MaxSteps is 2.
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{
				llm.ToolUseEvent{ID: "call_1", Name: "echo", Input: json.RawMessage(`"a"`)},
				llm.ToolUseEvent{ID: "call_2", Name: "echo", Input: json.RawMessage(`"b"`)},
				llm.ToolUseEvent{ID: "call_3", Name: "echo", Input: json.RawMessage(`"c"`)},
				llm.MessageStop{},
			},
		},
	}

	a, err := agent.New(agent.Config{
		MaxSteps:   2,
		Resolver:   &mockResolver{provider: provider},
		Dispatcher: newDispatcher(echoTool{}),
		Store:      conversation.New(),
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	events := collect(a.Run(context.Background(), "test-model", "go"))

	// Should have 2 tool started + 2 tool completed + usage + turn completed = 6
	var toolCompleted int
	for _, ev := range events {
		if _, ok := ev.(event.ToolCallCompletedEvent); ok {
			toolCompleted++
		}
	}
	if toolCompleted != 2 {
		t.Errorf("got %d tool completions, want 2", toolCompleted)
	}

	last := events[len(events)-1]
	if _, ok := last.(event.TurnCompletedEvent); !ok {
		t.Errorf("last event = %T, want TurnCompletedEvent", last)
	}
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{llm.TextDelta{Text: "should not reach"}, llm.MessageStop{}},
		},
	}

	a := newAgent(t, provider, newDispatcher(), conversation.New())
	events := collect(a.Run(ctx, "test-model", "hi"))

	// Only TurnCompletedEvent — provider should not be called
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if _, ok := events[0].(event.TurnCompletedEvent); !ok {
		t.Errorf("event = %T, want TurnCompletedEvent", events[0])
	}
	if provider.callCount != 0 {
		t.Errorf("provider called %d times, want 0", provider.callCount)
	}
}

func TestRunProviderError(t *testing.T) {
	provider := &mockProvider{} // no responses → returns error

	a := newAgent(t, provider, newDispatcher(), conversation.New())
	events := collect(a.Run(context.Background(), "test-model", "hi"))

	// Expect: ErrorEvent, TurnCompletedEvent
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if _, ok := events[0].(event.ErrorEvent); !ok {
		t.Errorf("events[0] = %T, want ErrorEvent", events[0])
	}
	if _, ok := events[1].(event.TurnCompletedEvent); !ok {
		t.Errorf("events[1] = %T, want TurnCompletedEvent", events[1])
	}
}

func TestRunStreamError(t *testing.T) {
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{
				llm.TextDelta{Text: "partial"},
				llm.ErrorEvent{Err: errors.New("stream broke")},
			},
		},
	}

	a := newAgent(t, provider, newDispatcher(), conversation.New())
	events := collect(a.Run(context.Background(), "test-model", "hi"))

	// Expect: TextDelta, ErrorEvent, TurnCompletedEvent
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if _, ok := events[0].(event.TextDeltaEvent); !ok {
		t.Errorf("events[0] = %T, want TextDeltaEvent", events[0])
	}
	if _, ok := events[1].(event.ErrorEvent); !ok {
		t.Errorf("events[1] = %T, want ErrorEvent", events[1])
	}
	if _, ok := events[2].(event.TurnCompletedEvent); !ok {
		t.Errorf("events[2] = %T, want TurnCompletedEvent", events[2])
	}
}

func TestRunStoreAccumulates(t *testing.T) {
	provider := &mockProvider{
		responses: [][]llm.StreamEvent{
			{
				llm.ToolUseEvent{ID: "call_1", Name: "echo", Input: json.RawMessage(`"first"`)},
				llm.MessageStop{},
			},
			{
				llm.TextDelta{Text: "all done"},
				llm.MessageStop{},
			},
		},
	}

	store := conversation.New()
	a := newAgent(t, provider, newDispatcher(echoTool{}), store)

	// Drain events
	collect(a.Run(context.Background(), "test-model", "go"))

	msgs := store.Messages()
	// UserMessage, AssistantMessage(tool call), ToolResultMessage, AssistantMessage(text)
	if len(msgs) != 4 {
		t.Fatalf("store has %d messages, want 4", len(msgs))
	}

	if _, ok := msgs[0].(conversation.UserMessage); !ok {
		t.Errorf("msgs[0] = %T, want UserMessage", msgs[0])
	}
	if am, ok := msgs[1].(conversation.AssistantMessage); !ok || len(am.ToolCalls) != 1 {
		t.Errorf("msgs[1] = %+v, want AssistantMessage with 1 tool call", msgs[1])
	}
	if _, ok := msgs[2].(conversation.ToolResultMessage); !ok {
		t.Errorf("msgs[2] = %T, want ToolResultMessage", msgs[2])
	}
	if am, ok := msgs[3].(conversation.AssistantMessage); !ok || am.Text != "all done" {
		t.Errorf("msgs[3] = %+v, want AssistantMessage{all done}", msgs[3])
	}
}
