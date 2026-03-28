package output

import (
	"errors"
	"fmt"
	"testing"
	"time"

	heimdall "github.com/0merUfuk/heimdall/internal/heimdall"
)

// mockWriter is a minimal implementation of Writer used to verify the
// interface is implementable and that its contract can be exercised in tests.
type mockWriter struct {
	// returnErr, if non-nil, is returned by Write.
	returnErr error
	// returnPath is the file path returned by Write when returnErr is nil.
	returnPath string
	// capturedNote stores the note passed to the last Write call.
	capturedNote *heimdall.MeetingNote
	// writeCount tracks the number of Write calls.
	writeCount int
}

func (m *mockWriter) Write(note *heimdall.MeetingNote) (string, error) {
	m.capturedNote = note
	m.writeCount++
	if m.returnErr != nil {
		return "", m.returnErr
	}
	return m.returnPath, nil
}

// Compile-time assertion: mockWriter must satisfy Writer.
var _ Writer = (*mockWriter)(nil)

// newMockWriter returns a mockWriter that succeeds and returns the given path.
func newMockWriter(path string) *mockWriter {
	return &mockWriter{returnPath: path}
}

// newMockWriterWithError returns a mockWriter that returns an error from Write.
func newMockWriterWithError(err error) *mockWriter {
	return &mockWriter{returnErr: err}
}

// TestWriter_InterfaceCompliance verifies that mockWriter satisfies the Writer
// interface and that Write is callable.
func TestWriter_InterfaceCompliance(t *testing.T) {
	var w Writer = newMockWriter("/vault/2026-03-28-sprint-review.md")

	note := &heimdall.MeetingNote{
		Title: "Sprint Review",
	}

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	if path == "" {
		t.Error("Write: returned empty path on success")
	}
}

// TestWriter_WriteReturnsPath verifies the file path returned by Write.
func TestWriter_WriteReturnsPath(t *testing.T) {
	wantPath := "/Users/alice/vault/meetings/2026-03-28-sprint-review.md"
	var w Writer = newMockWriter(wantPath)

	got, err := w.Write(&heimdall.MeetingNote{Title: "Sprint Review"})
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	if got != wantPath {
		t.Errorf("Write path: got %q, want %q", got, wantPath)
	}
}

// TestWriter_WriteReturnsError verifies error propagation.
func TestWriter_WriteReturnsError(t *testing.T) {
	wantErr := errors.New("permission denied")
	var w Writer = newMockWriterWithError(wantErr)

	path, err := w.Write(&heimdall.MeetingNote{Title: "Sprint Review"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
	if path != "" {
		t.Errorf("on error, path should be empty; got %q", path)
	}
}

// TestWriter_WriteCapturesNote verifies the note is forwarded to the writer.
func TestWriter_WriteCapturesNote(t *testing.T) {
	mock := newMockWriter("/vault/test.md")

	want := &heimdall.MeetingNote{
		Title:    "Q1 Retrospective",
		Summary:  "Team discussed blockers and achievements.",
		Platform: "Zoom",
	}

	_, err := mock.Write(want)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	if mock.capturedNote == nil {
		t.Fatal("capturedNote is nil")
	}
	if mock.capturedNote.Title != want.Title {
		t.Errorf("capturedNote.Title: got %q, want %q", mock.capturedNote.Title, want.Title)
	}
	if mock.capturedNote.Platform != want.Platform {
		t.Errorf("capturedNote.Platform: got %q, want %q", mock.capturedNote.Platform, want.Platform)
	}
}

// TestWriter_WriteWithFullNote verifies Write handles a fully-populated MeetingNote.
func TestWriter_WriteWithFullNote(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	note := &heimdall.MeetingNote{
		Title:    "Architecture Decision",
		Date:     now,
		Duration: 30 * time.Minute,
		Summary:  "Agreed on microservices approach.",
		Decisions: []heimdall.Decision{
			{Description: "Use gRPC internally", DecidedBy: "Alice"},
		},
		ActionItems: []heimdall.ActionItem{
			{Task: "Draft service contracts", Owner: "Bob", Priority: "high"},
		},
		Topics: []heimdall.Topic{
			{Title: "Transport layer", Content: "Evaluated REST vs gRPC."},
		},
		Followups: []heimdall.Followup{
			{Question: "Performance benchmarks?", RaisedBy: "Charlie"},
		},
		SpeakerMap: map[int]string{0: "Alice", 1: "Bob", 2: "Charlie"},
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Let's decide on the transport layer.", IsFinal: true},
		},
		Platform: "Google Meet",
	}

	var w Writer = newMockWriter("/vault/2026-03-28-architecture-decision.md")
	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

// TestWriter_WriteWithNilNote verifies Write handles a nil note without panicking
// (the interface allows nil — implementations decide how to handle it).
func TestWriter_WriteWithNilNote(t *testing.T) {
	mock := newMockWriter("/vault/nil-note.md")

	// The mock implementation does not panic on nil; this test ensures the
	// interface is wide enough to accept nil (not a contract violation).
	_, err := mock.Write(nil)
	if err != nil {
		t.Fatalf("Write(nil): unexpected error from mock: %v", err)
	}
	if mock.capturedNote != nil {
		t.Error("capturedNote should be nil when nil was passed")
	}
}

// TestWriter_MultipleWrites verifies that Write can be called multiple times.
func TestWriter_MultipleWrites(t *testing.T) {
	mock := newMockWriter("/vault/meeting.md")

	for i := range 3 {
		note := &heimdall.MeetingNote{Title: fmt.Sprintf("Meeting %d", i)}
		_, err := mock.Write(note)
		if err != nil {
			t.Fatalf("Write[%d]: unexpected error: %v", i, err)
		}
	}

	if mock.writeCount != 3 {
		t.Errorf("writeCount: got %d, want 3", mock.writeCount)
	}
}

// TestWriter_PathVariants validates various Obsidian vault path patterns.
func TestWriter_PathVariants(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"vault root", "/Users/alice/vault/meeting.md"},
		{"nested directory", "/Users/alice/vault/meetings/2026/03/sprint-review.md"},
		{"with spaces", "/Users/alice/My Vault/Q1 Sprint Review.md"},
	}

	note := &heimdall.MeetingNote{Title: "Test Meeting"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w Writer = newMockWriter(tt.path)
			got, err := w.Write(note)
			if err != nil {
				t.Fatalf("Write: unexpected error: %v", err)
			}
			if got != tt.path {
				t.Errorf("path: got %q, want %q", got, tt.path)
			}
		})
	}
}
