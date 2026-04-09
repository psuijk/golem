package main

import (
	"context"
	"fmt"
	"os"

	"github.com/psuijk/golem/internal/shellexec"
)

func main() {

	ctx := context.Background()

	result, err := shellexec.Run(ctx, "echo", "hello", "world")

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("stdout: %q\n", result.Stdout)
	fmt.Printf("stderr: %q\n", result.Stderr)
	fmt.Printf("exit:   %d\n", result.ExitCode)
}
