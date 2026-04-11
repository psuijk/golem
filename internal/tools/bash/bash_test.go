package bash_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/psuijk/golem/internal/tools/bash"
)

func TestExecuteHappyPath(t *testing.T) {
	bt := bash.New(5 * time.Second)
	input := json.RawMessage(`{"command": "echo hello"}`)

	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	if !strings.Contains(result.Text, "hello") {
		t.Errorf("Text = %q, want to contain %q", result.Text, "hello")
	}
}

func TestExecuteNonZeroExit(t *testing.T) {
	bt := bash.New(5 * time.Second)
	input := json.RawMessage(`{"command": "sh -c 'echo out; echo err >&2; exit 7'"}`)

	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "out") {
		t.Errorf("Text = %q, want to contain %q", result.Text, "out")
	}
	if !strings.Contains(result.Text, "err") {
		t.Errorf("Text = %q, want to contain %q", result.Text, "err")
	}
}

func TestExecuteMalformedJSON(t *testing.T) {
	bt := bash.New(5 * time.Second)
	input := json.RawMessage(`{this is not valid json`)

	result, err := bt.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("Execute returned nil error for malformed JSON, want error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "parse bash input") {
		t.Errorf("err = %v, want to contain 'parse bash input'", err)
	}
}

func TestExecuteNegativeTimeout(t *testing.T) {
	bt := bash.New(5 * time.Second)
	input := json.RawMessage(`{"command": "echo should not run", "timeout_seconds": -1}`)

	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil, want non-nil")
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if !strings.Contains(result.Text, "timeout_seconds") {
		t.Errorf("Text = %q, want to mention timeout_seconds", result.Text)
	}
	if strings.Contains(result.Text, "should not run") {
		t.Errorf("Text = %q, command should not have executed", result.Text)
	}
}

func TestExecutePerCallTimeoutOverride(t *testing.T) {
	bt := bash.New(60 * time.Second)
	input := json.RawMessage(`{"command": "sleep 5", "timeout_seconds": 1}`)

	start := time.Now()
	result, err := bt.Execute(context.Background(), input)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("Execute took %v, expected per-call timeout (~1s) to apply", elapsed)
	}
	if err == nil && (result == nil || !result.IsError) {
		t.Errorf("expected timeout failure, got result=%+v err=%v", result, err)
	}
}
