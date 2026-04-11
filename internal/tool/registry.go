package tool

import (
	"errors"
	"fmt"
)

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
