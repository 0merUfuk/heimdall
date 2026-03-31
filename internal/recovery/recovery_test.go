package recovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// testSegment returns a Segment for testing purposes.
func testSegment(speaker int, text string, start time.Duration) heimdall.Segment {
	return heimdall.Segment{
		Speaker:    speaker,
		Text:       text,
		Start:      start,
		End:        start + 3*time.Second,
		Confidence: 0.95,
		Channel:    0,
		IsFinal:    true,
	}
}

// newTestWriter creates a RecoveryWriter that writes to a temp directory
// with a short interval for testing.
func newTestWriter(t *testing.T, title string) *RecoveryWriter {
	t.Helper()
	dir := t.TempDir()

	startTime := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	rw := &RecoveryWriter{
		dir:      dir,
		filename: "2026-03-28T10-00-00-" + sanitizeTitle(title) + ".json",
		metadata: RecoveryMetadata{
			Title:     title,
			StartTime: startTime,
			Platform:  "darwin",
		},
		interval: 50 * time.Millisecond,
	}
	return rw
}

// TestNewRecoveryWriter verifies that the constructor creates the recovery directory.
func TestNewRecoveryWriter(t *testing.T) {
	// Override home to temp dir to avoid touching real ~/.heimdall.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	defer os.Setenv("HOME", origHome)

	startTime := time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC)
	rw, err := NewRecoveryWriter("Sprint Planning", startTime)
	if err != nil {
		t.Fatalf("NewRecoveryWriter: unexpected error: %v", err)
	}

	if rw.filename == "" {
		t.Error("filename should not be empty")
	}
	if !strings.Contains(rw.filename, "sprint-planning") {
		t.Errorf("filename should contain sanitized title, got: %s", rw.filename)
	}
	if !strings.HasPrefix(rw.filename, "2026-03-28T14-30-00") {
		t.Errorf("filename should start with timestamp, got: %s", rw.filename)
	}

	// Verify directory was created.
	if _, err := os.Stat(rw.dir); os.IsNotExist(err) {
		t.Error("recovery directory was not created")
	}
}

// TestFlush_AtomicWrite verifies that Flush writes atomically (no temp files left behind).
func TestFlush_AtomicWrite(t *testing.T) {
	rw := newTestWriter(t, "atomic test")

	rw.AddSegment(testSegment(0, "Hello world", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: unexpected error: %v", err)
	}

	// Check the final file exists.
	finalPath := rw.FilePath()
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Error("recovery file does not exist after Flush")
	}

	// Check no temp files remain.
	entries, err := os.ReadDir(rw.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

// TestFlush_FilePermissions verifies that recovery files have 0600 permissions.
func TestFlush_FilePermissions(t *testing.T) {
	rw := newTestWriter(t, "perm test")
	rw.AddSegment(testSegment(0, "Secret transcript", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: unexpected error: %v", err)
	}

	info, err := os.Stat(rw.FilePath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %04o, want 0600", perm)
	}
}

// TestFlush_FileContent verifies that the recovery file contains correct JSON.
func TestFlush_FileContent(t *testing.T) {
	rw := newTestWriter(t, "content test")

	seg := testSegment(0, "First segment", 0)
	rw.AddSegment(seg)

	seg2 := testSegment(1, "Second segment", 5*time.Second)
	rw.AddSegment(seg2)

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: unexpected error: %v", err)
	}

	data, err := os.ReadFile(rw.FilePath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var rf RecoveryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if rf.Metadata.Title != "content test" {
		t.Errorf("Metadata.Title: got %q, want %q", rf.Metadata.Title, "content test")
	}
	if len(rf.Segments) != 2 {
		t.Fatalf("Segments length: got %d, want 2", len(rf.Segments))
	}
	if rf.Segments[0].Text != "First segment" {
		t.Errorf("Segments[0].Text: got %q, want %q", rf.Segments[0].Text, "First segment")
	}
	if rf.Segments[1].Speaker != 1 {
		t.Errorf("Segments[1].Speaker: got %d, want 1", rf.Segments[1].Speaker)
	}
}

// TestCleanup_DeletesFile verifies that Cleanup removes the recovery file.
func TestCleanup_DeletesFile(t *testing.T) {
	rw := newTestWriter(t, "cleanup test")
	rw.AddSegment(testSegment(0, "data", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// File should exist.
	if _, err := os.Stat(rw.FilePath()); os.IsNotExist(err) {
		t.Fatal("recovery file should exist before Cleanup")
	}

	if err := rw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: unexpected error: %v", err)
	}

	// File should be gone.
	if _, err := os.Stat(rw.FilePath()); !os.IsNotExist(err) {
		t.Error("recovery file should not exist after Cleanup")
	}
}

// TestCleanup_Idempotent verifies that calling Cleanup twice does not error.
func TestCleanup_Idempotent(t *testing.T) {
	rw := newTestWriter(t, "idempotent test")
	rw.AddSegment(testSegment(0, "data", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if err := rw.Cleanup(); err != nil {
		t.Fatalf("first Cleanup: %v", err)
	}
	if err := rw.Cleanup(); err != nil {
		t.Fatalf("second Cleanup (idempotent): %v", err)
	}
}

// TestAddSegment_ThreadSafe verifies that concurrent AddSegment calls do not race.
func TestAddSegment_ThreadSafe(t *testing.T) {
	rw := newTestWriter(t, "thread-safety")

	var wg sync.WaitGroup
	const goroutines = 10
	const segmentsPerGoroutine = 100

	for i := range goroutines {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for j := range segmentsPerGoroutine {
				seg := testSegment(gID, "text", time.Duration(j)*time.Second)
				rw.AddSegment(seg)
			}
		}(i)
	}

	wg.Wait()

	rw.mu.Lock()
	got := len(rw.segments)
	rw.mu.Unlock()

	want := goroutines * segmentsPerGoroutine
	if got != want {
		t.Errorf("segment count: got %d, want %d", got, want)
	}
}

// TestStart_PeriodicWrite verifies that Start triggers periodic writes.
func TestStart_PeriodicWrite(t *testing.T) {
	rw := newTestWriter(t, "periodic test")

	rw.AddSegment(testSegment(0, "before start", 0))

	ctx, cancel := context.WithCancel(context.Background())
	rw.Start(ctx)

	// Wait enough time for at least one tick (50ms interval).
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	// The file should have been written by the periodic loop.
	if _, err := os.Stat(rw.FilePath()); os.IsNotExist(err) {
		t.Error("recovery file should exist after periodic write")
	}

	rf, err := LoadRecoveryFile(rw.FilePath())
	if err != nil {
		t.Fatalf("LoadRecoveryFile: %v", err)
	}
	if len(rf.Segments) != 1 {
		t.Errorf("Segments length: got %d, want 1", len(rf.Segments))
	}
}

// TestStart_UpdatesWithNewSegments verifies that periodic writes include
// segments added after the writer started.
func TestStart_UpdatesWithNewSegments(t *testing.T) {
	rw := newTestWriter(t, "update test")

	rw.AddSegment(testSegment(0, "first", 0))

	ctx, cancel := context.WithCancel(context.Background())
	rw.Start(ctx)

	// Wait for first write.
	time.Sleep(80 * time.Millisecond)

	// Add more segments.
	rw.AddSegment(testSegment(1, "second", 5*time.Second))
	rw.AddSegment(testSegment(0, "third", 10*time.Second))

	// Wait for second write.
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	rf, err := LoadRecoveryFile(rw.FilePath())
	if err != nil {
		t.Fatalf("LoadRecoveryFile: %v", err)
	}
	if len(rf.Segments) != 3 {
		t.Errorf("Segments length: got %d, want 3", len(rf.Segments))
	}
}

// TestLoadRecoveryFile verifies loading a recovery file from disk.
func TestLoadRecoveryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-recovery.json")

	rf := RecoveryFile{
		Metadata: RecoveryMetadata{
			Title:     "Test Meeting",
			StartTime: time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC),
			Language:  "en",
			Platform:  "darwin",
		},
		Segments: []heimdall.Segment{
			testSegment(0, "Hello", 0),
			testSegment(1, "World", 3*time.Second),
		},
	}

	data, err := json.Marshal(rf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := LoadRecoveryFile(path)
	if err != nil {
		t.Fatalf("LoadRecoveryFile: unexpected error: %v", err)
	}

	if loaded.Metadata.Title != "Test Meeting" {
		t.Errorf("Metadata.Title: got %q, want %q", loaded.Metadata.Title, "Test Meeting")
	}
	if len(loaded.Segments) != 2 {
		t.Fatalf("Segments length: got %d, want 2", len(loaded.Segments))
	}
	if loaded.Segments[0].Text != "Hello" {
		t.Errorf("Segments[0].Text: got %q, want Hello", loaded.Segments[0].Text)
	}
}

// TestLoadRecoveryFile_InvalidJSON verifies error handling for corrupt files.
func TestLoadRecoveryFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadRecoveryFile(path)
	if err == nil {
		t.Fatal("LoadRecoveryFile: expected error for invalid JSON, got nil")
	}
}

// TestLoadRecoveryFile_NonExistent verifies error for missing file.
func TestLoadRecoveryFile_NonExistent(t *testing.T) {
	_, err := LoadRecoveryFile("/nonexistent/file.json")
	if err == nil {
		t.Fatal("LoadRecoveryFile: expected error for missing file, got nil")
	}
}

// TestListRecoveryFiles_FindsOrphaned verifies scanning the recovery directory.
func TestListRecoveryFiles_FindsOrphaned(t *testing.T) {
	// Override HOME to use a temp dir for RecoveryDir().
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	dir := RecoveryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write two recovery files.
	for _, name := range []string{"2026-03-28T10-00-00-meeting-1.json", "2026-03-28T11-00-00-meeting-2.json"} {
		rf := RecoveryFile{
			Metadata: RecoveryMetadata{Title: name, StartTime: time.Now()},
			Segments: []heimdall.Segment{testSegment(0, "data", 0)},
		}
		data, _ := json.Marshal(rf)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// Write a non-JSON file that should be skipped.
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not recovery"), 0600)

	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ListRecoveryFiles: got %d files, want 2", len(files))
	}
}

// TestListRecoveryFiles_EmptyDir verifies empty result for no recovery files.
func TestListRecoveryFiles_EmptyDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	dir := RecoveryDir()
	os.MkdirAll(dir, 0755)

	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("ListRecoveryFiles: got %d files, want 0", len(files))
	}
}

// TestListRecoveryFiles_MissingDir verifies no error when directory does not exist.
func TestListRecoveryFiles_MissingDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	defer os.Setenv("HOME", origHome)

	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("ListRecoveryFiles: got %v, want nil for missing dir", files)
	}
}

// TestSanitizeTitle verifies title sanitization for filenames.
func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Sprint Planning", "sprint-planning"},
		{"1:1 with Sarah", "1-1-with-sarah"},
		{"  spaces  ", "spaces"},
		{"UPPER-case", "upper-case"},
		{"special!@#$chars", "special-chars"},
		{"", "untitled"},
		{"---", "untitled"},
		{"multi   space   title", "multi-space-title"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTitle(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestFlush_OverwritesPreviousFile verifies that subsequent flushes overwrite
// the recovery file (not append).
func TestFlush_OverwritesPreviousFile(t *testing.T) {
	rw := newTestWriter(t, "overwrite test")

	rw.AddSegment(testSegment(0, "first", 0))
	if err := rw.Flush(); err != nil {
		t.Fatalf("first Flush: %v", err)
	}

	rw.AddSegment(testSegment(1, "second", 5*time.Second))
	if err := rw.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}

	rf, err := LoadRecoveryFile(rw.FilePath())
	if err != nil {
		t.Fatalf("LoadRecoveryFile: %v", err)
	}

	// Should contain both segments (not just the second).
	if len(rf.Segments) != 2 {
		t.Errorf("Segments length: got %d, want 2", len(rf.Segments))
	}
}

// TestRecoveryDir_ReturnsExpectedPath verifies the recovery directory path.
func TestRecoveryDir_ReturnsExpectedPath(t *testing.T) {
	dir := RecoveryDir()
	if !strings.HasSuffix(dir, filepath.Join(".heimdall", "recovery")) {
		t.Errorf("RecoveryDir: got %q, want suffix .heimdall/recovery", dir)
	}
}

// TestStopAfterCleanup_DoesNotRecreatFile verifies that calling Stop after
// Cleanup does not re-create the recovery file (Bug #5 fix).
func TestStopAfterCleanup_DoesNotRecreateFile(t *testing.T) {
	rw := newTestWriter(t, "stop-after-cleanup")
	rw.AddSegment(testSegment(0, "data", 0))

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// File should exist after flush.
	if _, err := os.Stat(rw.FilePath()); os.IsNotExist(err) {
		t.Fatal("recovery file should exist after Flush")
	}

	// Cleanup deletes the file and sets the cleaned flag.
	if err := rw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// File should be gone after cleanup.
	if _, err := os.Stat(rw.FilePath()); !os.IsNotExist(err) {
		t.Fatal("recovery file should not exist after Cleanup")
	}

	// Stop should NOT re-create the file.
	if err := rw.Stop(); err != nil {
		t.Fatalf("Stop after Cleanup: unexpected error: %v", err)
	}

	// File should still be gone.
	if _, err := os.Stat(rw.FilePath()); !os.IsNotExist(err) {
		t.Error("recovery file should NOT exist after Stop following Cleanup (Bug #5)")
	}
}

// TestFilePath verifies FilePath returns the full path.
func TestFilePath(t *testing.T) {
	rw := newTestWriter(t, "path test")
	path := rw.FilePath()

	if !strings.HasSuffix(path, rw.filename) {
		t.Errorf("FilePath: got %q, want suffix %q", path, rw.filename)
	}
	if !strings.HasPrefix(path, rw.dir) {
		t.Errorf("FilePath: got %q, want prefix %q", path, rw.dir)
	}
}
