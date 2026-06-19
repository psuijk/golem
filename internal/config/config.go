// Package config loads project-level settings from .golem/settings.json
// and .golem/settings.local.json, merging them into a single Settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/psuijk/golem/internal/sandbox"
)

// Settings holds the merged project configuration loaded from
// .golem/settings.json (team-shared) and .golem/settings.local.json
// (user-local, gitignored).
type Settings struct {
	Boundaries  []sandbox.PathRule `json:"boundaries"`
	Permissions []string           `json:"permissions"`
}

// Load reads settings from the .golem/ directory under the given path.
// Both files are optional — missing files are treated as empty. When
// both exist, they are merged: permissions are unioned and deduplicated;
// boundaries are merged by path with the more restrictive access level
// winning on conflicts.
func Load(dir string) (*Settings, error) {
	golemDir := filepath.Join(dir, ".golem")

	base, err := loadFile(filepath.Join(golemDir, "settings.json"))
	if err != nil {
		return nil, err
	}

	local, err := loadFile(filepath.Join(golemDir, "settings.local.json"))
	if err != nil {
		return nil, err
	}

	return merge(base, local), nil
}

// loadFile reads and parses a single settings file. Returns nil if
// the file does not exist.
func loadFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
	}
	return &s, nil
}

// merge combines base and local settings. Permissions are unioned and
// deduplicated. Boundaries are merged by path — when the same path
// appears in both, the more restrictive access level wins.
func merge(base, local *Settings) *Settings {
	if base == nil && local == nil {
		return &Settings{}
	}
	if base == nil {
		return local
	}
	if local == nil {
		return base
	}

	// Merge permissions: union + dedup.
	perms := append(base.Permissions, local.Permissions...)
	slices.Sort(perms)
	perms = slices.Compact(perms)

	// Merge boundaries: map by path, more restrictive wins.
	boundMap := make(map[string]sandbox.AccessLevel)
	for _, r := range base.Boundaries {
		boundMap[r.Path] = r.Access
	}
	for _, r := range local.Boundaries {
		existing, ok := boundMap[r.Path]
		if !ok || r.Access < existing {
			// Lower value = more restrictive (ReadOnly < ReadWrite).
			boundMap[r.Path] = r.Access
		}
	}

	bounds := make([]sandbox.PathRule, 0, len(boundMap))
	for path, access := range boundMap {
		bounds = append(bounds, sandbox.PathRule{Path: path, Access: access})
	}

	return &Settings{
		Boundaries:  bounds,
		Permissions: perms,
	}
}
