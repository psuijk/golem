package llm

import (
	"context"
	"encoding/json"
)

type ToolDefinition struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type RequestParams struct {
	Messages        []Message
	SystemPrompt    string
	ToolDefinitions []ToolDefinition
	Model           string
	Temperature     float32
	MaxTokens       int
}

type Provider interface {
	Stream(ctx context.Context, request RequestParams) (<-chan StreamEvent, error)
}
