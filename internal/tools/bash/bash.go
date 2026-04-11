package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/psuijk/golem/internal/shellexec"
	"github.com/psuijk/golem/internal/tool"
)

const schemaJSON = `{
	"type": "object",
	"properties": {
		"command": {
			"type": "string",
			"description": "The shell command to run"
		},
		"timeout_seconds": {
			"type": "integer",
			"description": "Maximum time in seconds before the command is killed"
		}
	},
	"required": ["command"]
}`

type bashArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds *int   `json:"timeout_seconds"`
}

type Tool struct {
	defaultTimeout time.Duration
}

func New(defaultTimeout time.Duration) *Tool {
	return &Tool{defaultTimeout: defaultTimeout}
}

func (t *Tool) Name() string {
	return "bash"
}

func (t *Tool) Description() string {
	return "Run a shell command and return its stdout, stderr, and exit code."
}

func (t *Tool) Schema() json.RawMessage {
	return json.RawMessage(schemaJSON)
}

func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {

	var args bashArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parse bash input: %w", err)
	}

	timeout := t.defaultTimeout
	if args.TimeoutSeconds != nil {
		if *args.TimeoutSeconds <= 0 {
			return &tool.Result{
				Text:    fmt.Sprintf("invalid timeout_seconds: %d (must be > 0)", *args.TimeoutSeconds),
				IsError: true,
			}, nil
		}
		timeout = time.Duration(*args.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := shellexec.Run(ctx, "sh", "-c", args.Command)
	if err != nil {
		return nil, fmt.Errorf("bash tool encountered an error: %w", err)
	}

	text := result.Stdout + result.Stderr
	isErr := result.ExitCode != 0

	return &tool.Result{Text: text, IsError: isErr}, nil
}

var _ tool.Tool = (*Tool)(nil)
