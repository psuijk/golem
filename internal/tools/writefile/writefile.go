package writefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/psuijk/golem/internal/fsops"
	"github.com/psuijk/golem/internal/tool"
)

// schemaJSON describes the Execute input: two required strings - "path" and "content".
const schemaJSON = `{
  "type": "object",
  "properties": {
    "path": {"type": "string"},
    "content": {"type": "string"}
  },
  "required": ["path", "content"]
}`

// writeFileArgs is the unmarshaled shape of the Execute input payload.
type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Tool writes files to the local filesystem, unconditionally overwriting
// any existing file at the target path.
type Tool struct{}

// New returns a Tool ready to dispatch writefile calls.
func New() *Tool {
	return &Tool{}
}

// Name returns the tool's identifier used by the dispatcher.
func (t *Tool) Name() string {
	return "writefile"
}

// Description returns the human-readable description shown to the LLM.
func (t *Tool) Description() string {
	return "Write content to a file on the local filesystem, creating the file if it does not exist or overwriting it completely if it does. Takes two required arguments: \"path\" (string), which may be absolute or relative to the working directory, and \"content\" (string), the full contents to write. The parent directory must already exist; writefile will not create missing parent directories. Returns a short confirmation message on success. Fails with an error result if the path is empty, points to an existing directory, has a missing parent directory, or cannot be written due to permissions. This tool overwrites files unconditionally -- use the edit tool instead when modifying part of an existing file."
}

// Schema returns the JSON Schema describing Execute's input.
func (t *Tool) Schema() json.RawMessage {
	return json.RawMessage(schemaJSON)
}

// Execute writes args.Content to args.Path, overwriting any existing file.
// Malformed input returns a Go error (caller bug). Expected failures --
// missing parent directory, permission denied, target is a directory --
// return a non-nil *tool.Result with IsError set and a nil Go error.
// The 0644 permission is used only when creating a new file; existing
// files keep their current permissions.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var args writeFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parse writefile input: %w", err)
	}

	err := os.WriteFile(args.Path, []byte(args.Content), 0644)

	if err != nil {
		return &tool.Result{Text: fmt.Sprintf("write %s: %s", args.Path, err), IsError: true}, nil
	}

	return &tool.Result{Text: fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), IsError: false}, nil
}

func (t *Tool) PathFromInput(input json.RawMessage) (string, fsops.Operation, error) {
	var args writeFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return "", fsops.OpWrite, fmt.Errorf("parse writefile input: %w", err)
	}

	return args.Path, fsops.OpWrite, nil
}

var _ tool.Tool = (*Tool)(nil)
var _ fsops.PathValidator = (*Tool)(nil)
