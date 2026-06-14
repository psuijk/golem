// Package shellexec runs external commands and captures their output.
package shellexec

import (
	"bytes"
	"context"
	"os/exec"
)

// Result holds the captured output and exit code of an executed command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run executes the named program with the given arguments and returns its
// stdout, stderr, and exit code. If the context is cancelled before the
// command finishes, the context error is returned instead of a Result.
func Run(ctx context.Context, name string, args ...string) (*Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()

	if err != nil {
		// Context cancellation — command was killed, no useful result.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// If it's not an ExitError, the command couldn't run at all
		// (e.g. binary not found).
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	return &Result{
		Stdout:   string(output),
		Stderr:   stderr.String(),
		ExitCode: cmd.ProcessState.ExitCode(),
	}, nil
}
