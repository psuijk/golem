package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrToolNotFound is returned when Dispatch is called with a name that
// is not registered. Callers can check for it with errors.Is.
var ErrToolNotFound = errors.New("tool not found")

// Dispatcher routes tool calls to registered tools and returns their results.
type Dispatcher struct {
	registry *Registry
}

// NewDispatcher returns a Dispatcher that routes tool calls through the given registry.
func NewDispatcher(r *Registry) *Dispatcher {
	return &Dispatcher{
		registry: r,
	}
}

// Dispatch looks up a tool by name and executes it with the given input.
// It returns an error if the tool is not registered or if execution fails.
func (d *Dispatcher) Dispatch(ctx context.Context, name string, input json.RawMessage) (*Result, error) {
	t, ok := d.registry.get(name)
	if !ok {
		return nil, fmt.Errorf("dispatching %q: %w", name, ErrToolNotFound)
	}
	result, err := t.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("dispatching %q: %w", name, err)
	}

	return result, nil
}
