package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/sandbox"
	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/readfile"
	"github.com/psuijk/golem/internal/tools/writefile"
)

type fakeTool struct {
	name   string
	result *tool.Result
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

func (f fakeTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	return f.result, f.err
}

var _ tool.Interface = fakeTool{}

func TestDispatchHappyPath(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(fakeTool{
		name:   "echo",
		result: &tool.Result{Text: "hello", IsError: false},
		err:    nil,
	})
	dispatcher := tool.NewDispatcher(registry, nil)

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
	registry := tool.NewRegistry()
	dispatcher := tool.NewDispatcher(registry, nil)

	result, err := dispatcher.Dispatch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("Dispatch returned nil error for unregistered tool, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
}

func TestDispatchExecuteError(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(fakeTool{
		name:   "broken",
		result: nil,
		err:    tool.ErrToolNotFound,
	})
	dispatcher := tool.NewDispatcher(registry, nil)

	result, err := dispatcher.Dispatch(context.Background(), "broken", nil)
	if !errors.Is(err, tool.ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v, want to mention tool name", err)
	}
}

func TestDispatchPolicyDeniesOutsidePath(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()

	outsidePath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsidePath, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	registry := tool.NewRegistry()
	registry.Register(readfile.New(1 << 20))

	policy := sandbox.NewPolicy([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})
	dispatcher := tool.NewDispatcher(registry, policy)

	input := json.RawMessage(fmt.Sprintf(`{"path":%q}`, outsidePath))
	result, err := dispatcher.Dispatch(context.Background(), "readfile", input)

	if err != nil {
		t.Fatalf("expected IsError result, got Go error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "not under any allowed root") {
		t.Errorf("Text = %q, want to contain 'not under any allowed root'", result.Text)
	}
}

func TestDispatchPolicyDeniesWriteToReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	registry := tool.NewRegistry()
	registry.Register(writefile.New())

	policy := sandbox.NewPolicy([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadOnly},
	})
	dispatcher := tool.NewDispatcher(registry, policy)

	input := json.RawMessage(fmt.Sprintf(`{"path":%q,"content":"overwrite"}`, path))
	result, err := dispatcher.Dispatch(context.Background(), "writefile", input)

	if err != nil {
		t.Fatalf("expected IsError result, got Go error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "read-only") {
		t.Errorf("Text = %q, want to contain 'read-only'", result.Text)
	}
}

func TestDispatchPolicyAllowsReadOnReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	registry := tool.NewRegistry()
	registry.Register(readfile.New(1 << 20))

	policy := sandbox.NewPolicy([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadOnly},
	})
	dispatcher := tool.NewDispatcher(registry, policy)

	input := json.RawMessage(fmt.Sprintf(`{"path":%q}`, path))
	result, err := dispatcher.Dispatch(context.Background(), "readfile", input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false; text = %q", result.Text)
	}
	if result.Text != "hello" {
		t.Errorf("Text = %q, want %q", result.Text, "hello")
	}
}

func TestDispatchNoPolicySkipsValidation(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(fakeTool{
		name:   "echo",
		result: &tool.Result{Text: "hello", IsError: false},
	})
	dispatcher := tool.NewDispatcher(registry, nil)

	result, err := dispatcher.Dispatch(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "hello" {
		t.Errorf("Text = %q, want %q", result.Text, "hello")
	}
}
