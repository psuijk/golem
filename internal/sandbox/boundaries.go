package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathValidator is an optional interface that filesystem-aware tools
// implement. The dispatcher checks for it before executing a tool --
// if the tool implements PathValidator and Boundaries are configured,
// the dispatcher calls PathFromInput to extract the target path and
// operation, then validates against the Boundaries before allowing
// execution. Tools that don't touch the filesystem don't implement
// this and are unaffected.
type PathValidator interface {
	PathFromInput(input json.RawMessage) (string, Operation, error)
}

// AccessLevel controls what operations are permitted on paths
// matching a PathRule.
type AccessLevel int

const (
	// ReadOnly permits read operations but denies writes and edits.
	ReadOnly AccessLevel = iota
	// ReadWrite permits all filesystem operations.
	ReadWrite
)

// PathRule associates a directory path with an access level. Paths are
// resolved and normalized by NewBoundaries at construction time so that
// ValidatePath comparisons are consistent.
type PathRule struct {
	Path   string
	Access AccessLevel
}

// Boundaries holds an ordered set of path rules that control which
// directories an agent may read from or write to. This is a hard
// enforcement layer — it applies regardless of approval state or
// yolo mode. When multiple rules match a path, the most specific
// (longest prefix) wins, allowing broad roots to be narrowed by
// subdirectory overrides.
type Boundaries struct {
	PathRules []PathRule
}

// Operation represents the type of filesystem access being attempted.
type Operation int

const (
	// OpRead represents a read operation (e.g. readfile).
	OpRead Operation = iota
	// OpWrite represents a write or edit operation (e.g. writefile, editfile).
	OpWrite
)

// NewBoundaries creates Boundaries from the given rules. Each rule's
// path is resolved through symlinks and normalized with a trailing
// separator so that prefix matching respects directory boundaries
// (e.g. a rule for "/project" does not match "/projectX").
func NewBoundaries(rules []PathRule) *Boundaries {
	for i := range rules {
		resolved, err := filepath.EvalSymlinks(rules[i].Path)
		if err == nil {
			rules[i].Path = resolved
		}
		rules[i].Path = filepath.Clean(rules[i].Path) + string(filepath.Separator)
	}
	return &Boundaries{PathRules: rules}
}

// ValidatePath checks whether the given operation on the given path
// is permitted by the boundaries. The path is resolved through symlinks
// and matched against rules by longest prefix. Returns nil if the
// operation is allowed, or an error describing why it was denied.
func (b *Boundaries) ValidatePath(path string, op Operation) error {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		// File doesn't exist yet (e.g. writing a new file).
		// Resolve the parent directory instead.
		resolved, err = filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("boundary: resolve path %q: %w", path, err)
		}
		resolved = filepath.Join(resolved, filepath.Base(path))
	} else if err != nil {
		return fmt.Errorf("boundary: resolve path %q: %w", path, err)
	}

	resolved = filepath.Clean(resolved)

	var best *PathRule

	for i := range b.PathRules {
		rule := &b.PathRules[i]

		if strings.HasPrefix(resolved, rule.Path) &&
			(best == nil || len(rule.Path) > len(best.Path)) {
			best = rule
		}
	}

	if best == nil {
		return fmt.Errorf("boundary: path %q not under any allowed root", path)
	}

	if op == OpWrite && best.Access == ReadOnly {
		return fmt.Errorf("boundary: write denied, path %q is under read-only root %q", path, best.Path)
	}

	return nil
}
