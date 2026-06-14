package llm

import (
	"context"
	"encoding/json"
)

// ToolDefinition describes a tool for the LLM so it can decide when
// to invoke it. Schema is a JSON Schema for the tool's input.
type ToolDefinition struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// RequestParams holds everything needed for a single LLM call.
type RequestParams struct {
	Messages        []Message
	SystemPrompt    string
	ToolDefinitions []ToolDefinition
	Model           string
	Temperature     float32
	MaxTokens       int
}

// Provider abstracts an LLM backend. Stream sends a request and returns
// a channel of streaming events. The channel is closed when the response
// is complete.
type Provider interface {
	Stream(ctx context.Context, request RequestParams) (<-chan StreamEvent, error)
}
