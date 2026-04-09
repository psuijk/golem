package shellexec

import (
	"context"
	"testing"
)

func TestRunEcho(t *testing.T) {
	ctx := context.Background()
	result, err := Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunNonexistentBinary(t *testing.T) {
	ctx := context.Background()
	result, err := Run(ctx, "this-binary-definitely-does-not-exist-xyz")
	if err == nil {
		t.Fatal("Run returned nil error for nonexistent binary, want error")
	}
	if result != nil {
		t.Errorf("Run returned non-nil result for nonexistent binary: %+v", result)
	}
}

func TestRunNonzeroExit(t *testing.T) {
	ctx := context.Background()
	result, err := Run(ctx, "sh", "-c", "echo out; echo err >&2; exit 7")
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result.Stdout != "out\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "out\n")
	}
	if result.Stderr != "err\n" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "err\n")
	}
	if result.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", result.ExitCode)
	}
}
