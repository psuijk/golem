package editfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/psuijk/golem/internal/fsops"
	"github.com/psuijk/golem/internal/tool"
)

// schemaJSON describes the Execute input: three required strings --
// "path", "old_string", and "new_string". old_string must appear
// exactly once in the file at path; it will be replaced with new_string.
const schemaJSON = `{
  "type": "object",
  "properties": {
    "path": {"type": "string"},
    "old_string": {"type": "string"},
    "new_string": {"type": "string"}
  },
  "required": ["path", "old_string", "new_string"]
}`

// editFileArgs is the unmarshaled shape of the Execute input payload.
type editFileArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// Tool performs targeted string replacements inside existing files,
// capped at maxSize bytes to bound memory use on large files. Edits
// are rejected unless old_string appears exactly once in the target
// file, forcing the caller to supply enough context to disambiguate
// the edit location.
type Tool struct {
	maxSize int64
}

// New returns a Tool that refuses to edit files larger than maxSize bytes.
func New(maxSize int64) *Tool {
	return &Tool{maxSize: maxSize}
}

// Name returns the tool's identifier used by the dispatcher.
func (t *Tool) Name() string {
	return "editfile"
}

// Description returns the human-readable description shown to the LLM.
func (t *Tool) Description() string {
	return "Replace a unique substring inside an existing file. Takes three required arguments: \"path\" (string), the file to edit, which must already exist; \"old_string\" (string), the exact substring to find, which must appear exactly once in the file; and \"new_string\" (string), the replacement text, which may be empty to delete. Returns a short confirmation message on success. Fails with an error result if the file does not exist, exceeds the configured maximum size, old_string is empty, old_string is not found, or old_string appears more than once. The uniqueness requirement is a safety property: if an edit is ambiguous, supply more surrounding context in old_string until it uniquely identifies the intended location. This tool cannot create new files -- use the writefile tool for that -- and cannot perform pattern or regex matching."
}

// Schema returns the JSON Schema describing Execute's input.
func (t *Tool) Schema() json.RawMessage {
	return json.RawMessage(schemaJSON)
}

// Execute reads the file at args.Path, replaces the single occurrence of
// args.OldString with args.NewString, and writes the result back.
// Malformed input returns a Go error (caller bug). Expected failures --
// missing file, oversized file, empty old_string, old_string not found,
// old_string appearing more than once, write errors -- return a non-nil
// *tool.Result with IsError set and a nil Go error. Existing file
// permissions are preserved on write.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {

	var args editFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parse editfile input: %w", err)
	}

	if args.OldString == "" {
		return &tool.Result{Text: "old_string must be non-empty", IsError: true}, nil
	}

	info, err := os.Stat(args.Path)

	if err != nil {
		return &tool.Result{Text: fmt.Sprintf("stat %s: %s", args.Path, err), IsError: true}, nil
	}

	if info.Size() > t.maxSize {
		return &tool.Result{
			Text:    fmt.Sprintf("file %s is %d bytes, exceeds max %d", args.Path, info.Size(), t.maxSize),
			IsError: true,
		}, nil
	}

	data, err := os.ReadFile(args.Path)

	if err != nil {
		return &tool.Result{
			Text:    fmt.Sprintf("edit %s: %s", args.Path, err),
			IsError: true,
		}, nil
	}

	count := strings.Count(string(data), args.OldString)
	if count == 0 {
		return &tool.Result{Text: fmt.Sprintf("old_string not found in %s", args.Path), IsError: true}, nil
	}
	if count > 1 {
		return &tool.Result{
			Text:    fmt.Sprintf("found %d occurrences of old_string in %s, must be unique", count, args.Path),
			IsError: true,
		}, nil
	}

	newContent := strings.Replace(string(data), args.OldString, args.NewString, 1)

	err = os.WriteFile(args.Path, []byte(newContent), info.Mode())

	if err != nil {
		return &tool.Result{Text: fmt.Sprintf("write %s: %s", args.Path, err), IsError: true}, nil
	}

	return &tool.Result{Text: fmt.Sprintf("wrote %d bytes to %s", len(newContent), args.Path), IsError: false}, nil
}

func (t *Tool) PathFromInput(input json.RawMessage) (string, fsops.Operation, error) {
	var args editFileArgs

	if err := json.Unmarshal(input, &args); err != nil {
		return "", fsops.OpWrite, fmt.Errorf("parse editfile input: %w", err)
	}

	return args.Path, fsops.OpWrite, nil
}

// Compile-time assertion that *Tool satisfies the tool.Interface interface.
var _ tool.Interface = (*Tool)(nil)
var _ fsops.PathValidator = (*Tool)(nil)
