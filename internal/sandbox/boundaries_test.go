package sandbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/sandbox"
)

func TestReadUnderAllowedRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	if err := bounds.ValidatePath(path, sandbox.OpRead); err != nil {
		t.Errorf("expected read to be allowed, got: %v", err)
	}
}

func TestWriteUnderReadWriteRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	if err := bounds.ValidatePath(path, sandbox.OpWrite); err != nil {
		t.Errorf("expected write to be allowed, got: %v", err)
	}
}

func TestWriteUnderReadOnlyRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadOnly},
	})

	err := bounds.ValidatePath(path, sandbox.OpWrite)
	if err == nil {
		t.Fatal("expected write to be denied, got nil")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %v, want to contain 'read-only'", err)
	}
}

func TestPathOutsideAllRoots(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	path := filepath.Join(outsideDir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	err := bounds.ValidatePath(path, sandbox.OpRead)
	if err == nil {
		t.Fatal("expected path outside root to be denied, got nil")
	}
	if !strings.Contains(err.Error(), "not under any allowed root") {
		t.Errorf("error = %v, want to contain 'not under any allowed root'", err)
	}
}

func TestMostSpecificRuleWins(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "readonly_sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	path := filepath.Join(subdir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
		{Path: subdir, Access: sandbox.ReadOnly},
	})

	// Read should be allowed (read-only still permits reads)
	if err := bounds.ValidatePath(path, sandbox.OpRead); err != nil {
		t.Errorf("expected read to be allowed, got: %v", err)
	}

	// Write should be denied (subdirectory override is read-only)
	err := bounds.ValidatePath(path, sandbox.OpWrite)
	if err == nil {
		t.Fatal("expected write to subdirectory to be denied, got nil")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %v, want to contain 'read-only'", err)
	}
}

func TestSimilarDirectoryName(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	projectX := filepath.Join(dir, "projectX")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.MkdirAll(projectX, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	path := filepath.Join(projectX, "secret.txt")
	if err := os.WriteFile(path, []byte("secret"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: project, Access: sandbox.ReadWrite},
	})

	err := bounds.ValidatePath(path, sandbox.OpRead)
	if err == nil {
		t.Fatal("expected projectX to NOT match project rule, got nil")
	}
}

func TestSymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("secret"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Create a symlink inside the allowed root pointing outside
	link := filepath.Join(allowed, "sneaky_link")
	if err := os.Symlink(secretPath, link); err != nil {
		t.Fatalf("setup symlink: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: allowed, Access: sandbox.ReadWrite},
	})

	err := bounds.ValidatePath(link, sandbox.OpRead)
	if err == nil {
		t.Fatal("expected symlink escape to be denied, got nil")
	}
	if !strings.Contains(err.Error(), "not under any allowed root") {
		t.Errorf("error = %v, want to contain 'not under any allowed root'", err)
	}
}

func TestWriteNewFileParentExists(t *testing.T) {
	dir := t.TempDir()

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	// File doesn't exist but parent dir does.
	path := filepath.Join(dir, "newfile.txt")
	if err := bounds.ValidatePath(path, sandbox.OpWrite); err != nil {
		t.Errorf("expected write to be allowed, got: %v", err)
	}
}

func TestWriteNewFileParentNotExists(t *testing.T) {
	dir := t.TempDir()

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	// Neither file nor parent dir exist — multi-level walk.
	path := filepath.Join(dir, "subdir", "deep", "newfile.txt")
	if err := bounds.ValidatePath(path, sandbox.OpWrite); err != nil {
		t.Errorf("expected write to be allowed, got: %v", err)
	}
}

func TestWriteNewFileNestedOutsideRoot(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: allowed, Access: sandbox.ReadWrite},
	})

	// Non-existent nested path outside allowed root — walk should
	// resolve to an ancestor outside the boundary.
	path := filepath.Join(outside, "subdir", "deep", "newfile.txt")
	err := bounds.ValidatePath(path, sandbox.OpWrite)
	if err == nil {
		t.Fatal("expected path outside root to be denied, got nil")
	}
	if !strings.Contains(err.Error(), "not under any allowed root") {
		t.Errorf("error = %v, want to contain 'not under any allowed root'", err)
	}
}

func TestRelativePathResolution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	bounds := sandbox.NewBoundaries([]sandbox.PathRule{
		{Path: dir, Access: sandbox.ReadWrite},
	})

	// Change to the parent of the temp dir so we can use a relative path.
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldWd)
	os.Chdir(filepath.Dir(dir))

	relPath := filepath.Join(filepath.Base(dir), "file.txt")
	if err := bounds.ValidatePath(relPath, sandbox.OpRead); err != nil {
		t.Errorf("expected relative path to be allowed, got: %v", err)
	}
}
