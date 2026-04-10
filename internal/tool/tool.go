package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(t Tool) error {
	name := t.Name()
	if name == "" {
		return errors.New("tool has empty name")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

func (r *Registry) get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Dispatcher routes tool calls to registered tools and returns their results.
type Dispatcher struct {
	registry *Registry
}

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
		return nil, fmt.Errorf("tool %q not found", name)
	}
	result, err := t.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("dispatching %q: %w", name, err)
	}

	return result, nil
}
