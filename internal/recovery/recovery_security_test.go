package recovery

import (
	"os"
	"testing"
	"time"
)

// TestNewRecoveryWriter_DirectoryPermissions_M002 verifies that the recovery
// directory is created with 0700 permissions (owner rwx only), not 0755.
// Security fix M-002: transcript data in recovery files is sensitive and
// the directory must not be group/world readable or traversable.
func TestNewRecoveryWriter_DirectoryPermissions_M002(t *testing.T) {
	// Override HOME so we don't touch the real ~/.heimdall directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	startTime := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	rw, err := NewRecoveryWriter("perm dir test", startTime)
	if err != nil {
		t.Fatalf("NewRecoveryWriter: unexpected error: %v", err)
	}

	info, err := os.Stat(rw.dir)
	if err != nil {
		t.Fatalf("Stat recovery dir: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("recovery directory permissions: got %04o, want 0700 — transcript data must not be world-readable (M-002)", perm)
	}
}

// TestNewRecoveryWriter_DirectoryPermissions_ExistingDir verifies that if the
// recovery directory already exists with correct permissions those are preserved,
// and if the directory was created with wrong permissions the next writer
// creation corrects them (or at minimum does not widen them).
func TestNewRecoveryWriter_DirectoryPermissions_ExistingDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create the directory first (simulating a previous writer run).
	startTime := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	rw1, err := NewRecoveryWriter("first run", startTime)
	if err != nil {
		t.Fatalf("first NewRecoveryWriter: %v", err)
	}

	// Create a second writer — it must not change the permissions to be wider.
	startTime2 := time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)
	_, err = NewRecoveryWriter("second run", startTime2)
	if err != nil {
		t.Fatalf("second NewRecoveryWriter: %v", err)
	}

	info, err := os.Stat(rw1.dir)
	if err != nil {
		t.Fatalf("Stat recovery dir: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("recovery directory permissions after second writer: got %04o, want 0700 (M-002)", perm)
	}
}

// TestFlush_FilePermissions_AfterRename verifies that after the atomic rename
// the final recovery file retains 0600 permissions.
// This is the existing L-001 atomicWrite Chmod check: Chmod is called before
// the rename so the final file must be 0600, not the OS umask default.
func TestFlush_FilePermissions_AfterRename(t *testing.T) {
	rw := newTestWriter(t, "chmod after rename")
	rw.AddSegment(testSegment(0, "sensitive content", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: unexpected error: %v", err)
	}

	info, err := os.Stat(rw.FilePath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("recovery file permissions after Flush+rename: got %04o, want 0600 (L-001)", perm)
	}
}

// TestFlush_MultipleFlushes_PermissionsStable verifies that repeated Flush calls
// (which rename a new temp file each time) maintain 0600 on the final file.
func TestFlush_MultipleFlushes_PermissionsStable(t *testing.T) {
	rw := newTestWriter(t, "multi flush perm")

	for i := range 3 {
		rw.AddSegment(testSegment(0, "segment", 0))
		if err := rw.Flush(); err != nil {
			t.Fatalf("Flush[%d]: unexpected error: %v", i, err)
		}

		info, err := os.Stat(rw.FilePath())
		if err != nil {
			t.Fatalf("Stat after Flush[%d]: %v", i, err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("Flush[%d] permissions: got %04o, want 0600 (L-001)", i, perm)
		}
	}
}
