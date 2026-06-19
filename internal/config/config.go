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

	"github.com/gofrs/flock"
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

// AddPermission appends a permission key to settings.local.json,
// creating the file and .golem/ directory if they don't exist. If
// the key is already present, it's a no-op. Uses a file lock to
// prevent concurrent writes from multiple goroutines or processes.
func AddPermission(dir string, permKey string) error {
	golemDir := filepath.Join(dir, ".golem")
	if err := os.MkdirAll(golemDir, 0755); err != nil {
		return fmt.Errorf("adding permission: %w", err)
	}

	lockPath := filepath.Join(golemDir, "settings.local.json.lock")
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("adding permission: lock: %w", err)
	}
	defer fl.Unlock()

	path := filepath.Join(golemDir, "settings.local.json")

	settings, err := loadFile(path)
	if err != nil {
		return fmt.Errorf("adding permission: %w", err)
	}
	if settings == nil {
		settings = &Settings{Boundaries: make([]sandbox.PathRule, 0), Permissions: make([]string, 0)}
	}

	if slices.Contains(settings.Permissions, permKey) {
		return nil
	}
	settings.Permissions = append(settings.Permissions, permKey)

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("adding permission: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("adding permission: %w", err)
	}

	return nil
}
