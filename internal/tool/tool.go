package tool

import (
	"context"
	"encoding/json"
)

// Result holds the output of a tool execution. IsError distinguishes
// operational failures (e.g. file not found) from successful runs;
// Go errors from Execute indicate caller bugs (e.g. malformed input).
type Result struct {
	Text    string
	IsError bool
}

// Interface defines the contract for a tool. Name, Description, and
// Schema are used to build the LLM's tool definitions. Execute runs
// the tool with the given JSON input.
type Interface interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (*Result, error)
}
