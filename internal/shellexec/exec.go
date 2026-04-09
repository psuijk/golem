package shellexec

import (
	"context"
	"os/exec"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

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

	return &Result{
		Stdout:   string(output),
		Stderr:   string(stderr),
		ExitCode: cmd.ProcessState.ExitCode(),
	}, nil
}
