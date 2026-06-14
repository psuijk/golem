package tool

import (
	"errors"
	"fmt"
)

// Registry holds the tools available for dispatch, keyed by name.
// It is not safe for concurrent use; register all tools at startup
// before any goroutines begin reading from it.
type Registry struct {
	tools map[string]Interface
}

// NewRegistry returns an empty Registry ready to accept tools via Register.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Interface),
	}
}

// Register adds a tool to the registry under the name returned by t.Name.
// It returns an error if the name is empty or if a tool with that name
// is already registered.
func (r *Registry) Register(t Interface) error {
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

// get looks up a tool by name. The second return value reports whether
// a tool with that name exists.
func (r *Registry) get(name string) (Interface, bool) {
	t, ok := r.tools[name]
	return t, ok
}
