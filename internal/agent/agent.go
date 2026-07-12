package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/sandbox"
	"github.com/psuijk/golem/internal/tool"
)

// ErrToolNotFound is returned when a tool call references a name that
// is not in the agent's tool registry.
var ErrToolNotFound = errors.New("tool not found")

// ModelResolver maps a model ID to the provider that serves it.
type ModelResolver interface {
	Resolve(modelID string) (llm.Provider, error)
}

// Config provides the inputs for constructing an Agent. Resolver and
// Store are required. MaxTurns and MaxSteps default to 10 if unset.
// Config is read once during New() and not stored — the agent unpacks
// what it needs into its own fields.
type Config struct {
	Resolver          ModelResolver
	Tools             []tool.Interface
	Boundaries        *sandbox.Boundaries
	Permissions       []string
	MaxTurns          int
	MaxSteps          int
	Store             *conversation.Store
	SystemPrompt      string
	OnPermissionGrant func(key string) error // persist "don't ask again"
}

// toolSet groups all tool-related runtime state: the tool registry for
// lookup and execution, the LLM-facing definitions, filesystem
// boundaries, and the approval set. Built from Config in New().
type toolSet struct {
	registry     map[string]tool.Interface
	defs         []llm.ToolDefinition
	bounds       *sandbox.Boundaries
	autoApproved map[string]bool
	onApproval   func(key string) error
}

// Agent orchestrates the LLM-tool loop. It is safe to call Run
// multiple times; the store accumulates history across calls.
type Agent struct {
	resolver     ModelResolver       // maps model IDs to providers
	store        *conversation.Store // conversation history, persists across Run calls
	systemPrompt string              // injected into every LLM request
	maxTurns     int                 // max LLM round-trips per Run
	maxSteps     int                 // max total tool executions per Run
	tools        toolSet             // tool registry, definitions, boundaries, approvals
}

// New validates the config, builds the tool registry and LLM
// definitions from Config.Tools, and returns a ready-to-use Agent.
func New(cfg Config) (*Agent, error) {

	if cfg.Resolver == nil {
		return nil, errors.New("agent: resolver is required")
	}

	if cfg.Store == nil {
		return nil, errors.New("agent: store is required")
	}

	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 10
	}

	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = 10
	}

	toolsMap := make(map[string]tool.Interface, len(cfg.Tools))
	toolDefs := make([]llm.ToolDefinition, 0, len(cfg.Tools))
	for _, t := range cfg.Tools {
		toolsMap[t.Name()] = t
		toolDefs = append(
			toolDefs,
			llm.ToolDefinition{Name: t.Name(), Description: t.Description(), Schema: t.Schema()},
		)
	}

	if cfg.OnPermissionGrant == nil {
		cfg.OnPermissionGrant = func(key string) error { return nil }
	}

	ts := toolSet{
		registry:     toolsMap,
		defs:         toolDefs,
		bounds:       cfg.Boundaries,
		autoApproved: make(map[string]bool, len(cfg.Permissions)),
		onApproval:   cfg.OnPermissionGrant,
	}

	for _, p := range cfg.Permissions {
		ts.autoApproved[p] = true
	}

	return &Agent{
		resolver:     cfg.Resolver,
		store:        cfg.Store,
		systemPrompt: cfg.SystemPrompt,
		maxTurns:     cfg.MaxTurns,
		maxSteps:     cfg.MaxSteps,
		tools:        ts,
	}, nil
}

// Run kicks off the agent loop: user message → LLM → tools → LLM → ... → done.
// Returns an event channel the caller consumes for UI.
func (a *Agent) Run(ctx context.Context, modelID string, userMessage string) <-chan event.Event {
	out := make(chan event.Event)

	go func() {
		defer close(out)
		defer func() { out <- event.TurnCompletedEvent{} }()

		a.store.Append(conversation.UserMessage{Text: userMessage})

		p, err := a.resolver.Resolve(modelID)
		if err != nil {
			out <- event.ErrorEvent{Err: err}
			return
		}

		turn := 0
		step := 0
		for {
			if ctx.Err() != nil {
				return
			}

			if turn >= a.maxTurns {
				return
			}

			reqParams := llm.RequestParams{
				Messages:        buildMessages(a.store.Messages()),
				SystemPrompt:    a.systemPrompt,
				ToolDefinitions: a.tools.defs,
				Model:           modelID,
			}

			stCh, err := p.Stream(ctx, reqParams)
			if err != nil {
				out <- event.ErrorEvent{Err: err}
				return
			}

			var assMsg conversation.AssistantMessage

			for ev := range stCh {
				switch e := ev.(type) {
				case llm.TextDelta:
					assMsg.Text += e.Text
					out <- event.TextDeltaEvent{Text: e.Text}
				case llm.ThinkingDelta:
					out <- event.ThinkingDeltaEvent{Text: e.Text}
				case llm.ToolUseEvent:
					assMsg.ToolCalls = append(
						assMsg.ToolCalls,
						conversation.ToolCall{ID: e.ID, Name: e.Name, Input: e.Input},
					)
				case llm.MessageStop:
					out <- event.UsageEvent{InputTokens: e.Usage.InputTokens, OutputTokens: e.Usage.OutputTokens}
				case llm.ErrorEvent:
					out <- event.ErrorEvent{Err: e.Err}
					return
				}
			}

			a.store.Append(assMsg)

			if len(assMsg.ToolCalls) == 0 {
				return
			}

			for i, call := range assMsg.ToolCalls {
				if ctx.Err() != nil {
					return
				}

				if step >= a.maxSteps {
					return
				}

				denied := a.dispatch(ctx, out, call.ID, call.Name, call.Input)
				if denied {
					remaining := assMsg.ToolCalls[i+1:]
					for _, next := range remaining {
						a.store.Append(
							conversation.ToolResultMessage{
								ToolCallID: next.ID,
								Content:    "skipped — turn stopped",
								IsError:    true,
							},
						)
					}
					return
				}
				step++
			}

			turn++
		}
	}()
	return out
}

// buildMessages converts conversation history into the LLM wire format.
func buildMessages(msgs []conversation.Message) []llm.Message {
	var llmMsgs []llm.Message

	for _, msg := range msgs {
		switch m := msg.(type) {
		case conversation.UserMessage:
			llmMsgs = append(llmMsgs, llm.Message{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: m.Text}}})
		case conversation.AssistantMessage:
			var content []llm.Content
			if m.Text != "" {
				content = append(content, llm.TextContent{Text: m.Text})
			}
			for _, tc := range m.ToolCalls {
				content = append(content, llm.ToolUseContent{ID: tc.ID, Name: tc.Name, Input: tc.Input})
			}
			llmMsgs = append(llmMsgs, llm.Message{Role: llm.RoleAssistant, Content: content})
		case conversation.ToolResultMessage:
			llmMsgs = append(llmMsgs, llm.Message{Role: llm.RoleUser, Content: []llm.Content{llm.ToolResultContent{ToolCallID: m.ToolCallID, Content: m.Content, IsError: m.IsError}}})
		}
	}

	return llmMsgs
}

// dispatch handles a single tool call: checks approval, enforces
// boundaries, and executes the tool. Results are recorded in the
// store and emitted as events via the deferred cleanup.
func (a *Agent) dispatch(
	ctx context.Context,
	out chan event.Event,
	id string,
	name string,
	input json.RawMessage,
) (denied bool) {
	var content string
	var isError bool
	defer func() {
		out <- event.ToolCallCompletedEvent{Name: name, Text: content, IsError: isError}
		a.store.Append(
			conversation.ToolResultMessage{ToolCallID: id, Content: content, IsError: isError},
		)
	}()

	t, ok := a.tools.registry[name]
	if !ok {
		content = ErrToolNotFound.Error()
		isError = true
		return
	}

	permKey, err := t.PermissionFromInput(input)
	if err != nil {
		content = fmt.Sprintf("dispatch %q: %s", name, err)
		isError = true
		return
	}

	if !a.tools.autoApproved[permKey] {
		reCh := make(chan event.ApprovalResponse)
		out <- event.ToolApprovalEvent{Name: name, Input: input, Response: reCh}
		switch <-reCh {
		case event.Deny:
			content = "tool use rejected"
			isError = true
			denied = true
			return
		case event.ApproveOnce:
			//valid, do nothing
		case event.ApproveAlways:
			if err := a.tools.onApproval(permKey); err != nil {
				content = fmt.Sprintf("dispatch %q: %s", name, err)
				isError = true
				return
			}
			a.tools.autoApproved[permKey] = true
		}

	}

	out <- event.ToolCallStartedEvent{Name: name, Input: input}

	if a.tools.bounds != nil {
		pv, ok := t.(sandbox.PathValidator)
		if ok {
			path, op, err := pv.PathFromInput(input)
			if err != nil {
				content = fmt.Sprintf("dispatch %q: %s", name, err)
				isError = true
				return
			}
			if err := a.tools.bounds.ValidatePath(path, op); err != nil {
				content = fmt.Sprintf("dispatch %q: %s", name, err)
				isError = true
				return
			}
		}
	}
	result, err := t.Execute(ctx, input)
	if err != nil {
		content = fmt.Sprintf("dispatching %q: %s", name, err)
		isError = true
		return
	}

	content = result.Text
	return
}
