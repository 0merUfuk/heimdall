package recovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// --- V-006 Crash Recovery Integration Tests ---

// TestRecoveryIntegration_CrashSimulation simulates a crash scenario:
//  1. Create a RecoveryWriter and add segments
//  2. Flush to disk (simulates periodic write during recording)
//  3. Do NOT call Cleanup (simulates crash — process killed before cleanup)
//  4. Use ListRecoveryFiles to find the orphaned recovery file
//  5. Load and verify segments are intact
func TestRecoveryIntegration_CrashSimulation(t *testing.T) {
	// Override HOME to isolate from real recovery directory.
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	startTime := time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC)

	// Step 1: Create writer and add segments (simulates recording session).
	rw, err := NewRecoveryWriter("Sprint Planning", startTime, "en")
	if err != nil {
		t.Fatalf("NewRecoveryWriter: %v", err)
	}

	segments := []heimdall.Segment{
		{
			Speaker:    0,
			Text:       "Let's start with the sprint review.",
			Start:      1 * time.Second,
			End:        4 * time.Second,
			Confidence: 0.97,
			Channel:    0,
			IsFinal:    true,
		},
		{
			Speaker:    1,
			Text:       "The auth migration is complete.",
			Start:      5 * time.Second,
			End:        8 * time.Second,
			Confidence: 0.95,
			Channel:    1,
			IsFinal:    true,
		},
		{
			Speaker:    0,
			Text:       "Great. What about the CI pipeline?",
			Start:      9 * time.Second,
			End:        12 * time.Second,
			Confidence: 0.93,
			Channel:    0,
			IsFinal:    true,
		},
	}

	for _, seg := range segments {
		rw.AddSegment(seg)
	}

	// Step 2: Flush to disk (this would happen every 30s during recording).
	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify file was written.
	if _, err := os.Stat(rw.FilePath()); os.IsNotExist(err) {
		t.Fatal("recovery file not found after Flush")
	}

	// Step 3: Simulate crash -- do NOT call rw.Cleanup(). The recovery file
	// remains on disk as an orphaned file.

	// Step 4: On "next launch", scan for orphaned recovery files.
	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 orphaned recovery file, got %d", len(files))
	}

	// Step 5: Verify the recovered data matches what was written.
	recovered := files[0]

	if recovered.Metadata.Title != "Sprint Planning" {
		t.Errorf("recovered title = %q; want %q", recovered.Metadata.Title, "Sprint Planning")
	}
	if !recovered.Metadata.StartTime.Equal(startTime) {
		t.Errorf("recovered start time = %v; want %v", recovered.Metadata.StartTime, startTime)
	}
	if len(recovered.Segments) != len(segments) {
		t.Fatalf("recovered %d segments; want %d", len(recovered.Segments), len(segments))
	}

	for i, seg := range recovered.Segments {
		if seg.Text != segments[i].Text {
			t.Errorf("segment[%d].Text = %q; want %q", i, seg.Text, segments[i].Text)
		}
		if seg.Speaker != segments[i].Speaker {
			t.Errorf("segment[%d].Speaker = %d; want %d", i, seg.Speaker, segments[i].Speaker)
		}
		if seg.Start != segments[i].Start {
			t.Errorf("segment[%d].Start = %v; want %v", i, seg.Start, segments[i].Start)
		}
		if seg.Confidence != segments[i].Confidence {
			t.Errorf("segment[%d].Confidence = %f; want %f", i, seg.Confidence, segments[i].Confidence)
		}
		if seg.IsFinal != segments[i].IsFinal {
			t.Errorf("segment[%d].IsFinal = %v; want %v", i, seg.IsFinal, segments[i].IsFinal)
		}
	}
}

// TestRecoveryIntegration_CleanShutdownDeletesFile verifies that a clean
// shutdown (Cleanup called) removes the recovery file, so it is NOT found
// by ListRecoveryFiles on the next launch.
func TestRecoveryIntegration_CleanShutdownDeletesFile(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	startTime := time.Date(2026, 3, 28, 15, 0, 0, 0, time.UTC)

	rw, err := NewRecoveryWriter("Clean Shutdown Meeting", startTime, "en")
	if err != nil {
		t.Fatalf("NewRecoveryWriter: %v", err)
	}

	rw.AddSegment(heimdall.Segment{
		Speaker: 0, Text: "Test segment", IsFinal: true,
	})

	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Clean shutdown: call Cleanup (simulates successful Obsidian write).
	if err := rw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// The file should not exist.
	if _, err := os.Stat(rw.FilePath()); !os.IsNotExist(err) {
		t.Error("recovery file should not exist after clean shutdown")
	}

	// ListRecoveryFiles should find nothing.
	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 orphaned files after clean shutdown, got %d", len(files))
	}
}

// TestRecoveryIntegration_PeriodicWriteWithCrash verifies the full periodic
// write lifecycle: Start writes periodically, segments added mid-recording
// are captured, and a crash leaves a recoverable file.
func TestRecoveryIntegration_PeriodicWriteWithCrash(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	startTime := time.Date(2026, 3, 28, 16, 0, 0, 0, time.UTC)

	rw, err := NewRecoveryWriterWithInterval("Periodic Test", startTime, "en", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewRecoveryWriterWithInterval: %v", err)
	}

	// Add initial segment before starting periodic writes.
	rw.AddSegment(heimdall.Segment{
		Speaker: 0, Text: "Before start", Start: 0, End: 2 * time.Second, IsFinal: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	rw.Start(ctx)

	// Wait for first periodic write.
	time.Sleep(100 * time.Millisecond)

	// Add more segments during recording.
	rw.AddSegment(heimdall.Segment{
		Speaker: 1, Text: "During recording", Start: 5 * time.Second, End: 8 * time.Second, IsFinal: true,
	})
	rw.AddSegment(heimdall.Segment{
		Speaker: 0, Text: "Also during recording", Start: 10 * time.Second, End: 13 * time.Second, IsFinal: true,
	})

	// Wait for another periodic write to capture the new segments.
	time.Sleep(100 * time.Millisecond)

	// Simulate crash: cancel context (stops periodic writes) but do NOT call Cleanup.
	cancel()
	time.Sleep(50 * time.Millisecond) // Let goroutine exit.

	// Recover: find and load the orphaned file.
	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 orphaned file, got %d", len(files))
	}

	recovered := files[0]
	if len(recovered.Segments) != 3 {
		t.Errorf("recovered %d segments; want 3", len(recovered.Segments))
	}

	// Verify segment content.
	texts := make([]string, len(recovered.Segments))
	for i, seg := range recovered.Segments {
		texts[i] = seg.Text
	}

	expected := []string{"Before start", "During recording", "Also during recording"}
	for i, want := range expected {
		if i >= len(texts) {
			t.Errorf("missing segment[%d] with text %q", i, want)
			continue
		}
		if texts[i] != want {
			t.Errorf("segment[%d].Text = %q; want %q", i, texts[i], want)
		}
	}
}

// TestRecoveryIntegration_MultipleOrphanedFiles verifies that ListRecoveryFiles
// correctly finds multiple orphaned recovery files from different crashed sessions.
func TestRecoveryIntegration_MultipleOrphanedFiles(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	dir := RecoveryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Simulate 3 different crashed sessions.
	sessions := []struct {
		title     string
		startTime time.Time
		segments  int
	}{
		{"Morning Standup", time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC), 5},
		{"Sprint Planning", time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC), 12},
		{"1-on-1 with Sarah", time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC), 8},
	}

	for _, sess := range sessions {
		rf := RecoveryFile{
			Metadata: RecoveryMetadata{
				Title:     sess.title,
				StartTime: sess.startTime,
				Platform:  "darwin",
			},
			Segments: make([]heimdall.Segment, sess.segments),
		}
		for i := range sess.segments {
			rf.Segments[i] = heimdall.Segment{
				Speaker: i % 3,
				Text:    "Segment text",
				Start:   time.Duration(i) * 5 * time.Second,
				End:     time.Duration(i)*5*time.Second + 3*time.Second,
				IsFinal: true,
			}
		}

		data, err := json.Marshal(rf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		filename := sess.startTime.Format("2006-01-02T15-04-05") + "-" + sanitizeTitle(sess.title) + ".json"
		if err := os.WriteFile(filepath.Join(dir, filename), data, 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// Recover all orphaned files.
	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 orphaned files, got %d", len(files))
	}

	// Verify each has the correct number of segments.
	segCountMap := make(map[string]int)
	for _, f := range files {
		segCountMap[f.Metadata.Title] = len(f.Segments)
	}

	for _, sess := range sessions {
		count, ok := segCountMap[sess.title]
		if !ok {
			t.Errorf("recovery file for %q not found", sess.title)
			continue
		}
		if count != sess.segments {
			t.Errorf("recovery file %q has %d segments; want %d", sess.title, count, sess.segments)
		}
	}
}

// TestRecoveryIntegration_CorruptFileSkipped verifies that corrupt recovery files
// are skipped when listing, without preventing other valid files from being loaded.
func TestRecoveryIntegration_CorruptFileSkipped(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	dir := RecoveryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a valid recovery file.
	validRF := RecoveryFile{
		Metadata: RecoveryMetadata{Title: "Valid Meeting", StartTime: time.Now()},
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Valid data", IsFinal: true},
		},
	}
	validData, _ := json.Marshal(validRF)
	os.WriteFile(filepath.Join(dir, "2026-03-28T10-00-00-valid.json"), validData, 0600)

	// Write a corrupt recovery file (invalid JSON).
	os.WriteFile(filepath.Join(dir, "2026-03-28T11-00-00-corrupt.json"), []byte("{invalid json"), 0600)

	// Write another valid recovery file.
	validRF2 := RecoveryFile{
		Metadata: RecoveryMetadata{Title: "Another Valid Meeting", StartTime: time.Now()},
		Segments: []heimdall.Segment{
			{Speaker: 1, Text: "Also valid", IsFinal: true},
		},
	}
	validData2, _ := json.Marshal(validRF2)
	os.WriteFile(filepath.Join(dir, "2026-03-28T12-00-00-valid2.json"), validData2, 0600)

	// ListRecoveryFiles should skip the corrupt file and return the 2 valid ones.
	files, err := ListRecoveryFiles()
	if err != nil {
		t.Fatalf("ListRecoveryFiles: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 valid recovery files (corrupt skipped), got %d", len(files))
	}
}

// TestRecoveryIntegration_LoadRecoveryFilePreservesAllFields verifies that all
// segment fields survive the write -> crash -> recovery cycle without data loss.
func TestRecoveryIntegration_LoadRecoveryFilePreservesAllFields(t *testing.T) {
	dir := t.TempDir()

	original := RecoveryFile{
		Metadata: RecoveryMetadata{
			Title:     "Field Preservation Test",
			StartTime: time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC),
			Language:  "tr",
			Platform:  "darwin",
		},
		Segments: []heimdall.Segment{
			{
				Speaker:    2,
				Text:       "Testing all fields",
				Start:      45 * time.Second,
				End:        48 * time.Second,
				Confidence: 0.87,
				Channel:    1,
				IsFinal:    true,
			},
			{
				Speaker:    0,
				Text:       "Interim result",
				Start:      50 * time.Second,
				End:        51 * time.Second,
				Confidence: 0.65,
				Channel:    0,
				IsFinal:    false,
			},
		},
	}

	// Write to disk.
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Load from disk.
	loaded, err := LoadRecoveryFile(path)
	if err != nil {
		t.Fatalf("LoadRecoveryFile: %v", err)
	}

	// Verify metadata.
	if loaded.Metadata.Title != original.Metadata.Title {
		t.Errorf("Title: got %q, want %q", loaded.Metadata.Title, original.Metadata.Title)
	}
	if loaded.Metadata.Language != original.Metadata.Language {
		t.Errorf("Language: got %q, want %q", loaded.Metadata.Language, original.Metadata.Language)
	}
	if loaded.Metadata.Platform != original.Metadata.Platform {
		t.Errorf("Platform: got %q, want %q", loaded.Metadata.Platform, original.Metadata.Platform)
	}
	if !loaded.Metadata.StartTime.Equal(original.Metadata.StartTime) {
		t.Errorf("StartTime: got %v, want %v", loaded.Metadata.StartTime, original.Metadata.StartTime)
	}

	// Verify segments.
	if len(loaded.Segments) != len(original.Segments) {
		t.Fatalf("Segments length: got %d, want %d", len(loaded.Segments), len(original.Segments))
	}

	for i, orig := range original.Segments {
		got := loaded.Segments[i]
		if got.Speaker != orig.Speaker {
			t.Errorf("segment[%d].Speaker: got %d, want %d", i, got.Speaker, orig.Speaker)
		}
		if got.Text != orig.Text {
			t.Errorf("segment[%d].Text: got %q, want %q", i, got.Text, orig.Text)
		}
		if got.Start != orig.Start {
			t.Errorf("segment[%d].Start: got %v, want %v", i, got.Start, orig.Start)
		}
		if got.End != orig.End {
			t.Errorf("segment[%d].End: got %v, want %v", i, got.End, orig.End)
		}
		if got.Confidence != orig.Confidence {
			t.Errorf("segment[%d].Confidence: got %f, want %f", i, got.Confidence, orig.Confidence)
		}
		if got.Channel != orig.Channel {
			t.Errorf("segment[%d].Channel: got %d, want %d", i, got.Channel, orig.Channel)
		}
		if got.IsFinal != orig.IsFinal {
			t.Errorf("segment[%d].IsFinal: got %v, want %v", i, got.IsFinal, orig.IsFinal)
		}
	}
}
