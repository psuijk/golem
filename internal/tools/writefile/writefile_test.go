package writefile_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/tools/writefile"
)

func TestExecuteHappyPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")

	wt := writefile.New()
	input := json.RawMessage(fmt.Sprintf(`{"path":%q, "content":"hello this is a test"}`, path))
	result, err := wt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello this is a test" {
		t.Errorf("file contents = %q, want %q", string(data), "hello this is a test")
	}
}

func TestOverwriteExisting(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")

	if err := os.WriteFile(path, []byte("this shouldn't exist"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	wt := writefile.New()
	input := json.RawMessage(fmt.Sprintf(`{"path":%q, "content":"hello this is a test"}`, path))
	result, err := wt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello this is a test" {
		t.Errorf("file contents = %q, want %q", string(data), "hello this is a test")
	}
}

func TestMissingParent(t *testing.T) {
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "foo.txt")
	wt := writefile.New()

	input := json.RawMessage(fmt.Sprintf(`{"path":%q, "content":"hello this is a test"}`, path))
	result, err := wt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "write") {
		t.Errorf("Text = %q, want to contain 'write'", result.Text)
	}
}

func TestExecuteMalformedJSON(t *testing.T) {
	wt := writefile.New()
	input := json.RawMessage(`{this is not valid json`)

	result, err := wt.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute returned nil error for malformed JSON, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "parse writefile input") {
		t.Errorf("err = %v, want to contain 'parse writefile input'", err)
	}
}
