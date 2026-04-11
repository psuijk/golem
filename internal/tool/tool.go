package tool

import (
	"context"
	"encoding/json"
)

type Result struct {
	Text    string
	IsError bool
}

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (*Result, error)
}
