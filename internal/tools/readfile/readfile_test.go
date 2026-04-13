package readfile_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/tools/readfile"
)

func TestExecuteHappyPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")

	if err := os.WriteFile(path, []byte("hello test"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rt := readfile.New(1 << 20)
	input := json.RawMessage(fmt.Sprintf(`{"path":%q}`, path))
	result, err := rt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	if result.Text != "hello test" {
		t.Errorf("Text = %q, want %q", result.Text, "hello test")
	}
}

func TestMissingFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")

	rt := readfile.New(1 << 20)
	input := json.RawMessage(fmt.Sprintf(`{"path":%q}`, path))
	result, err := rt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "stat") {
		t.Errorf("err = %v, want to contain 'stat'", result.Text)
	}
}

func TestExecuteMalformedJSON(t *testing.T) {
	rt := readfile.New(1 << 20)
	input := json.RawMessage(`{this is not valid json`)

	result, err := rt.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute returned nil error for malformed JSON, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "parse readfile input") {
		t.Errorf("err = %v, want to contain 'parse readfile input'", err)
	}
}

func TestOversizedFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")

	if err := os.WriteFile(path, []byte("this content is longer thatn 10 bytes"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rt := readfile.New(10)
	input := json.RawMessage(fmt.Sprintf(`{"path":%q}`, path))
	result, err := rt.Execute(ctx, input)

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "exceeds max") {
		t.Errorf("err = %v, want to contain 'exceeds max'", result.Text)
	}
}
