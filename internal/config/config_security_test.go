package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSave_FilePermissions_C001 verifies that config files are written with
// 0600 permissions (owner read/write only), not 0644.
// Security fix C-001: config files contain API key references and should not
// be world-readable.
func TestSave_FilePermissions_C001(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config file permissions: got %04o, want 0600 — config files must not be world-readable (C-001)", perm)
	}
}

// TestSave_DirectoryPermissions_M001 verifies that the config directory is
// created with 0700 permissions (owner rwx only), not 0755.
// Security fix M-001: the config directory holds sensitive configuration and
// must not be world-readable or traversable by other users.
func TestSave_DirectoryPermissions_M001(t *testing.T) {
	// Use a fresh temp dir as the "home" so we don't touch real ~/.heimdall.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Build a path that exercises the MkdirAll in Save.
	dir := filepath.Join(tmpHome, ".heimdall")
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat directory: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("config directory permissions: got %04o, want 0700 — config directory must not be group/world accessible (M-001)", perm)
	}
}

// TestSave_NestedDirectoryPermissions_M001 verifies that nested config directories
// (multiple levels deep) are also created with 0700 permissions.
func TestSave_NestedDirectoryPermissions_M001(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Nested path: the leaf dir must be 0700.
	dir := filepath.Join(tmpHome, ".heimdall", "subdir")
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat nested directory: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("nested config directory permissions: got %04o, want 0700 (M-001)", perm)
	}
}

// TestSave_FilePermissions_OverwritePreservesMode verifies that overwriting an
// existing config file also produces 0600 permissions (not inherited from old file).
func TestSave_FilePermissions_OverwritePreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write once.
	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// Mutate and write again to simulate an overwrite.
	cfg.Deepgram.Model = "nova-2"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("overwritten config file permissions: got %04o, want 0600 (C-001)", perm)
	}
}
