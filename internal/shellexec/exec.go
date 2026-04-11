// Package shellexec runs external commands and captures their output.
package shellexec

import (
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

	output, err := cmd.Output()

	var stderr []byte
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return nil, err
		}
		stderr = exitErr.Stderr
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	return &Result{
		Stdout:   string(output),
		Stderr:   string(stderr),
		ExitCode: cmd.ProcessState.ExitCode(),
	}, nil
}
