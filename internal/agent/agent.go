package agent

import (
	"context"
	"errors"

	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/tool"
)

// ModelResolver maps a model ID to the provider that serves it.
type ModelResolver interface {
	Resolve(modelID string) (llm.Provider, error)
}

// Config holds the dependencies and limits for an Agent. Resolver,
// Dispatcher, and Store are required; MaxTurns and MaxSteps default
// to 10 if unset.
type Config struct {
	Resolver     ModelResolver
	MaxTurns     int
	MaxSteps     int
	Dispatcher   *tool.Dispatcher
	Store        *conversation.Store
	SystemPrompt string
}

// Agent orchestrates the LLM-tool loop. It is safe to call Run
// multiple times on the same Agent; the Store accumulates history
// across calls.
type Agent struct {
	cfg   Config
	tools []llm.ToolDefinition
}

// New validates the config, builds tool definitions from the dispatcher,
// and returns a ready-to-use Agent.
func New(cfg Config) (*Agent, error) {

	if cfg.Resolver == nil {
		return nil, errors.New("agent: resolver is required")
	}

	if cfg.Dispatcher == nil {
		return nil, errors.New("agent: dispatcher is required")
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

	toolInterfaces := cfg.Dispatcher.Tools()
	tools := make([]llm.ToolDefinition, 0, len(toolInterfaces))
	for _, t := range toolInterfaces {
		tools = append(tools, llm.ToolDefinition{Name: t.Name(), Description: t.Description(), Schema: t.Schema()})
	}
	return &Agent{cfg: cfg, tools: tools}, nil
}

// Run kicks off the agent loop: user message → LLM → tools → LLM → ... → done.
// Returns an event channel the caller consumes for UI.
func (a *Agent) Run(ctx context.Context, modelID string, userMessage string) <-chan event.Event {
	out := make(chan event.Event)

	go func() {
		defer close(out)
		defer func() { out <- event.TurnCompletedEvent{} }()

		a.cfg.Store.Append(conversation.UserMessage{Text: userMessage})

		p, err := a.cfg.Resolver.Resolve(modelID)
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

			if turn >= a.cfg.MaxTurns {
				return
			}

			reqParams := llm.RequestParams{
				Messages:        buildMessages(a.cfg.Store.Messages()),
				SystemPrompt:    a.cfg.SystemPrompt,
				ToolDefinitions: a.tools,
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

			a.cfg.Store.Append(assMsg)

			if len(assMsg.ToolCalls) == 0 {
				return
			}

			for _, call := range assMsg.ToolCalls {
				if ctx.Err() != nil {
					return
				}

				if step >= a.cfg.MaxSteps {
					return
				}

				out <- event.ToolCallStartedEvent{Name: call.Name, Input: call.Input}
				result, err := a.cfg.Dispatcher.Dispatch(ctx, call.Name, call.Input)

				var content string
				var isError bool
				if err != nil {
					content = err.Error()
					isError = true
				} else {
					content = result.Text
					isError = result.IsError
				}
				a.cfg.Store.Append(
					conversation.ToolResultMessage{ToolCallID: call.ID, Content: content, IsError: isError},
				)

				out <- event.ToolCallCompletedEvent{Name: call.Name, Text: content, IsError: isError}
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
