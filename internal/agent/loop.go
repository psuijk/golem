package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/tool"
)

type ToolCall struct {
	Name  string
	Input json.RawMessage
}

type Config struct {
	MaxSteps   int
	Dispatcher *tool.Dispatcher
	Store      *conversation.Store
}

type Loop struct {
	Config Config
}

func New(cfg Config) *Loop {
	return &Loop{Config: cfg}
}

func (l *Loop) Run(ctx context.Context, calls []ToolCall) <-chan event.Event {
	out := make(chan event.Event)

	go func() {
		step := 0
		for _, call := range calls {
			if ctx.Err() != nil {
				break
			}

			if step >= l.Config.MaxSteps {
				break
			}

			out <- event.ToolCallStartedEvent{Name: call.Name, Input: call.Input}
			result, err := l.Config.Dispatcher.Dispatch(ctx, call.Name, call.Input)
			if l.Config.Store != nil {
				id := fmt.Sprintf("call_%d", step)
				l.Config.Store.Append(conversation.AssistantMessage{ToolCalls: []conversation.ToolCall{{ID: id, Name: call.Name, Input: call.Input}}})
				var content string
				var isError bool
				if err != nil {
					content = err.Error()
					isError = true
				} else {
					content = result.Text
					isError = result.IsError
				}
				l.Config.Store.Append(conversation.ToolResultMessage{ToolCallID: id, Content: content, IsError: isError})
			}
			out <- event.ToolCallCompletedEvent{Name: call.Name, Result: result, Err: err}
			step++
		}

		out <- event.TurnCompletedEvent{}

		close(out)
	}()

	return out
}
