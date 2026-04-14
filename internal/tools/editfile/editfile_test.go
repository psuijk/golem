package editfile_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/tools/editfile"
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("fixture write: %v", err)
	}
	return path
}

func buildInput(path, oldStr, newStr string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"path":%q,"old_string":%q,"new_string":%q}`,
		path, oldStr, newStr,
	))
}

func TestEditHappyPath(t *testing.T) {
	path := writeFixture(t, "hello world")

	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput(path, "world", "Go"))

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false; text = %q", result.Text)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello Go" {
		t.Errorf("file contents = %q, want %q", string(data), "hello Go")
	}
}

func TestEditDeletion(t *testing.T) {
	path := writeFixture(t, "keep this remove this")

	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput(path, " remove this", ""))

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false; text = %q", result.Text)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "keep this" {
		t.Errorf("file contents = %q, want %q", string(data), "keep this")
	}
}

func TestEditOldStringNotFound(t *testing.T) {
	path := writeFixture(t, "hello")

	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput(path, "goodbye", "farewell"))

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "not found") {
		t.Errorf("Text = %q, want to contain 'not found'", result.Text)
	}
}

func TestEditOldStringNotUnique(t *testing.T) {
	path := writeFixture(t, "foo bar foo")

	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput(path, "foo", "baz"))

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "must be unique") {
		t.Errorf("Text = %q, want to contain 'must be unique'", result.Text)
	}
}

func TestEditMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.txt")

	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput(path, "x", "y"))

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
		t.Errorf("Text = %q, want to contain 'stat'", result.Text)
	}
}

func TestEditEmptyOldString(t *testing.T) {
	et := editfile.New(1 << 20)
	result, err := et.Execute(context.Background(), buildInput("unused.txt", "", "anything"))

	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "non-empty") {
		t.Errorf("Text = %q, want to contain 'non-empty'", result.Text)
	}
}

func TestEditMalformedJSON(t *testing.T) {
	et := editfile.New(1 << 20)
	input := json.RawMessage(`{this is not valid json`)

	result, err := et.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute returned nil error for malformed JSON, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "parse editfile input") {
		t.Errorf("err = %v, want to contain 'parse editfile input'", err)
	}
}
