package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeTool struct {
	name   string
	result *Result
	err    error
}

func (f fakeTool) Name() string {
	return f.name
}

func (f fakeTool) Description() string {
	return ""
}

func (f fakeTool) Schema() json.RawMessage {
	return nil
}

func (f fakeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	return f.result, f.err
}

var _ Tool = fakeTool{}

func TestDispatchHappyPath(t *testing.T) {
	registry := NewRegistry()
	registry.Register(fakeTool{
		name:   "echo",
		result: &Result{Text: "hello", IsError: false},
		err:    nil,
	})
	dispatcher := NewDispatcher(registry)

	result, err := dispatcher.Dispatch(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if result.Text != "hello" {
		t.Errorf("Text = %q, want %q", result.Text, "hello")
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
}

func TestDispatchToolNotFound(t *testing.T) {
	registry := NewRegistry()
	dispatcher := NewDispatcher(registry)

	result, err := dispatcher.Dispatch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("Dispatch returned nil error for unregistered tool, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
}

func TestDispatchExecuteError(t *testing.T) {
	registry := NewRegistry()
	registry.Register(fakeTool{
		name:   "broken",
		result: nil,
		err:    ErrToolNotFound,
	})
	dispatcher := NewDispatcher(registry)

	result, err := dispatcher.Dispatch(context.Background(), "broken", nil)
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v, want to mention tool name", err)
	}
}
