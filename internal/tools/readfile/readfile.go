package readfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/psuijk/golem/internal/fsops"
	"github.com/psuijk/golem/internal/tool"
)

// schemaJSON describes the Execute input: a single required "path" string.
const schemaJSON = `{
  "type": "object",
  "properties": {
    "path": {"type": "string"}
  },
  "required": ["path"]
}`

// readFileArgs is the unmarshaled shape of the Execute input payload.
type readFileArgs struct {
	Path string `json:"path"`
}

// Tool reads files from the local filesystem, capped at maxSize bytes
// to prevent loading arbitrarily large files into memory.
type Tool struct {
	maxSize int64
}

// New returns a Tool that refuses to read files larger than maxSize bytes.
func New(maxSize int64) *Tool {
	return &Tool{maxSize: maxSize}
}

// Name returns the tool's identifier used by the dispatcher.
func (t *Tool) Name() string {
	return "readfile"
}

// Description returns the human-readable description shown to the LLM.
func (t *Tool) Description() string {
	return "Read the contents of a file from the local filesystem and return it as text. Takes a single required argument \"path\" (string), which may be absolute or relative to the working directory. Returns the file's contents as a UTF-8 string. Fails with an error result if the file does not exist, cannot be read due to permissions, or exceeds the configured maximum size. Binary files are not detected; reading one will return garbled text."
}

// Schema returns the JSON Schema describing Execute's input.
func (t *Tool) Schema() json.RawMessage {
	return json.RawMessage(schemaJSON)
}

// Execute reads the file at the given path and returns its contents as text.
// Malformed input returns a Go error (caller bug). Expected failures --
// missing file, unreadable file, oversized file -- return a non-nil
// *tool.Result with IsError set and a nil Go error.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {

	var args readFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parse readfile input: %w", err)
	}

	info, err := os.Stat(args.Path)

	if err != nil {
		return &tool.Result{Text: fmt.Sprintf("stat %s: %s", args.Path, err), IsError: true}, nil
	}

	if info.Size() > t.maxSize {
		return &tool.Result{Text: fmt.Sprintf("file %s is %d bytes, exceeds max %d", args.Path, info.Size(), t.maxSize), IsError: true}, nil
	}

	data, err := os.ReadFile(args.Path)

	if err != nil {
		return &tool.Result{
			Text:    fmt.Sprintf("read %s: %s", args.Path, err),
			IsError: true,
		}, nil
	}

	return &tool.Result{Text: string(data), IsError: false}, nil
}

func (t *Tool) PathFromInput(input json.RawMessage) (string, fsops.Operation, error) {
	var args readFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return "", fsops.OpRead, fmt.Errorf("parse readfile input: %w", err)
	}

	return args.Path, fsops.OpRead, nil
}

var _ tool.Tool = (*Tool)(nil)
var _ fsops.PathValidator = (*Tool)(nil)
