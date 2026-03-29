package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// TestSanitizeFilename_PathTraversal_M003 verifies that sanitizeFilename strips
// all path traversal sequences. The sanitized result must never contain directory
// separators or dot-dot segments that could escape the output directory.
func TestSanitizeFilename_PathTraversal_M003(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unix traversal", "../../../etc/passwd"},
		{"windows traversal", `..\..\windows\system32`},
		{"deep traversal", "../../../../root/.ssh/authorized_keys"},
		{"mixed slash traversal", "../meetings/../../../etc/hosts"},
		{"absolute unix path", "/etc/shadow"},
		{"absolute path with dir", "/var/log/auth.log"},
		{"null byte with traversal", "../etc/passwd\x00.md"},
		{"leading slash", "/vault-root/secret"},
		{"percent-encoded slash", "../../../etc%2fpasswd"},
		{"unicode lookalike slash", "..∕..∕etc∕passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)

			// The sanitized filename must not contain any path separators.
			if strings.ContainsAny(got, "/\\") {
				t.Errorf("sanitizeFilename(%q) = %q: contains path separator (M-003)", tt.input, got)
			}

			// The sanitized filename must not start with a dot (hidden files / relative paths).
			if strings.HasPrefix(got, ".") {
				t.Errorf("sanitizeFilename(%q) = %q: must not start with '.' (M-003)", tt.input, got)
			}

			// The sanitized filename must not be empty after sanitization.
			if got == "" {
				t.Errorf("sanitizeFilename(%q) returned empty string — should fallback to 'meeting'", tt.input)
			}

			// Verify the result is usable as a plain filename (no dir components).
			if filepath.Base(got) != got {
				t.Errorf("sanitizeFilename(%q) = %q is not a plain filename (contains directory component)", tt.input, got)
			}
		})
	}
}

// TestWrite_PathTraversal_Via_Title_M003 verifies that a meeting title containing
// path traversal sequences cannot escape the vault directory when passed to Write.
func TestWrite_PathTraversal_Via_Title_M003(t *testing.T) {
	w, vaultPath := newTestWriter(t)

	// A note with a title crafted to escape the output directory.
	note := &heimdall.MeetingNote{
		Title:    "../../../tmp/escape",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 10 * time.Minute,
		Platform: "Zoom",
	}

	path, err := w.Write(note)
	if err != nil {
		// An error is acceptable (reject the note) — what is NOT acceptable is
		// writing outside the vault.
		return
	}

	// Verify the output path is inside the vault.
	if !strings.HasPrefix(path, vaultPath) {
		t.Errorf("Write with traversal title wrote outside vault: path=%q, vault=%q (M-003)", path, vaultPath)
	}

	// Verify no file was created outside the vault.
	escapedPath := "/tmp/escape.md"
	if _, err := os.Stat(escapedPath); err == nil {
		t.Errorf("Write created file outside vault at %s (M-003)", escapedPath)
	}
}

// TestWrite_PathTraversal_Via_AbsoluteTitle_M003 verifies that an absolute path
// in the title cannot write outside the vault.
func TestWrite_PathTraversal_Via_AbsoluteTitle_M003(t *testing.T) {
	w, vaultPath := newTestWriter(t)

	note := &heimdall.MeetingNote{
		Title:    "/etc/cron.d/malicious",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 5 * time.Minute,
		Platform: "Zoom",
	}

	path, err := w.Write(note)
	if err != nil {
		return
	}

	if !strings.HasPrefix(path, vaultPath) {
		t.Errorf("Write with absolute path title wrote outside vault: path=%q, vault=%q (M-003)", path, vaultPath)
	}
}

// TestSanitizeFilename_TraversalResultIsPlainName_M003 verifies that after
// sanitizing a traversal input the result is always a plain filename with no
// directory component — it is safe to use in filepath.Join.
func TestSanitizeFilename_TraversalResultIsPlainName_M003(t *testing.T) {
	traversalInputs := []string{
		"../etc/passwd",
		"../../root/.bashrc",
		"/absolute/path",
		"./relative",
		"a/b/c",
		"meeting/../secret",
	}

	for _, input := range traversalInputs {
		got := sanitizeFilename(input)

		// filepath.Base of a plain name equals itself.
		if filepath.Base(got) != got {
			t.Errorf("sanitizeFilename(%q) = %q contains directory component (M-003)", input, got)
		}

		// Must not be empty.
		if got == "" {
			t.Errorf("sanitizeFilename(%q) is empty (M-003)", input)
		}
	}
}

// TestAtomicWrite_FilePermissions_L001 verifies that files written by atomicWrite
// have explicit 0644 permissions (not umask-dependent).
// Security fix L-001: explicit Chmod ensures consistent permissions across systems.
// Obsidian notes use 0644 (not 0600) so the user's editor and Obsidian.app can read them.
func TestAtomicWrite_FilePermissions_L001(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meeting-note.md")
	data := []byte("# Sprint Planning\n\nSensitive business information.")

	if err := atomicWrite(path, data); err != nil {
		t.Fatalf("atomicWrite: unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("atomicWrite file permissions: got %04o, want 0644 — meeting notes must be readable by Obsidian (L-001)", perm)
	}
}

// TestWrite_OutputFilePermissions_L001 verifies that the full Write pipeline
// (via ObsidianWriter) produces files with explicit 0644 permissions.
func TestWrite_OutputFilePermissions_L001(t *testing.T) {
	w, _ := newTestWriter(t)
	note := testNote()

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat output file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("Write output file permissions: got %04o, want 0644 (L-001)", perm)
	}
}

// TestAtomicWrite_OverwritePreservesPermissions_L001 verifies that overwriting
// an existing file via atomicWrite still produces 0644.
func TestAtomicWrite_OverwritePreservesPermissions_L001(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	// Write once.
	if err := atomicWrite(path, []byte("original content")); err != nil {
		t.Fatalf("first atomicWrite: %v", err)
	}

	// Overwrite.
	if err := atomicWrite(path, []byte("updated content")); err != nil {
		t.Fatalf("second atomicWrite: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("overwritten file permissions: got %04o, want 0644 (L-001)", perm)
	}
}

// TestSanitizeFilename_DotDotCollapsed_M003 is a targeted regression test
// verifying that the double-dot traversal pattern is eliminated.
func TestSanitizeFilename_DotDotCollapsed_M003(t *testing.T) {
	got := sanitizeFilename("../../../etc/passwd")

	if strings.Contains(got, "..") {
		t.Errorf("sanitizeFilename should strip '..', got %q (M-003)", got)
	}
	if strings.Contains(got, "/") {
		t.Errorf("sanitizeFilename should strip '/', got %q (M-003)", got)
	}
}
