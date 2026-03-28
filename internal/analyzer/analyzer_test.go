package analyzer

import (
	"context"
	"errors"
	"testing"
	"time"

	heimdall "github.com/0merUfuk/heimdall/internal/heimdall"
)

// mockAnalyzer is a minimal implementation of Analyzer used to verify the
// interface is implementable and that its contract can be exercised in tests.
type mockAnalyzer struct {
	// returnErr, if non-nil, is returned by Summarize.
	returnErr error
	// returnNote is the MeetingNote returned by Summarize when returnErr is nil.
	returnNote *heimdall.MeetingNote
	// capturedSegments stores the segments passed to the last Summarize call.
	capturedSegments []heimdall.Segment
	// capturedOpts stores the opts passed to the last Summarize call.
	capturedOpts heimdall.AnalyzeOpts
}

func (m *mockAnalyzer) Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	m.capturedSegments = segments
	m.capturedOpts = opts
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.returnNote, nil
}

// Compile-time assertion: mockAnalyzer must satisfy Analyzer.
var _ Analyzer = (*mockAnalyzer)(nil)

// newMockAnalyzerWithNote returns a mockAnalyzer that produces a non-nil MeetingNote.
func newMockAnalyzerWithNote(note *heimdall.MeetingNote) *mockAnalyzer {
	return &mockAnalyzer{returnNote: note}
}

// newMockAnalyzerWithError returns a mockAnalyzer that returns an error from Summarize.
func newMockAnalyzerWithError(err error) *mockAnalyzer {
	return &mockAnalyzer{returnErr: err}
}

// TestAnalyzer_InterfaceCompliance verifies that mockAnalyzer satisfies
// the Analyzer interface and that Summarize is callable.
func TestAnalyzer_InterfaceCompliance(t *testing.T) {
	note := &heimdall.MeetingNote{
		Title:   "Test Meeting",
		Summary: "A brief test summary.",
	}
	var a Analyzer = newMockAnalyzerWithNote(note)

	segments := []heimdall.Segment{
		{Speaker: 0, Text: "Hello.", IsFinal: true},
	}
	opts := heimdall.AnalyzeOpts{Model: "claude-haiku-4-5", Language: "en"}

	result, err := a.Summarize(context.Background(), segments, opts)
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Summarize: returned nil note")
	}
	if result.Title != note.Title {
		t.Errorf("MeetingNote.Title: got %q, want %q", result.Title, note.Title)
	}
}

// TestAnalyzer_SummarizeReturnsError verifies error propagation on failure.
func TestAnalyzer_SummarizeReturnsError(t *testing.T) {
	wantErr := errors.New("API timeout")
	var a Analyzer = newMockAnalyzerWithError(wantErr)

	result, err := a.Summarize(context.Background(), nil, heimdall.AnalyzeOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
}

// TestAnalyzer_SummarizeCapturesSegments verifies all provided segments are forwarded.
func TestAnalyzer_SummarizeCapturesSegments(t *testing.T) {
	mock := newMockAnalyzerWithNote(&heimdall.MeetingNote{})
	segments := []heimdall.Segment{
		{Speaker: 0, Text: "First segment.", IsFinal: true},
		{Speaker: 1, Text: "Second segment.", IsFinal: true},
		{Speaker: 0, Text: "Third segment.", IsFinal: false},
	}

	_, err := mock.Summarize(context.Background(), segments, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if len(mock.capturedSegments) != 3 {
		t.Errorf("capturedSegments: got %d, want 3", len(mock.capturedSegments))
	}
	if mock.capturedSegments[1].Text != "Second segment." {
		t.Errorf("capturedSegments[1].Text: got %q, want %q", mock.capturedSegments[1].Text, "Second segment.")
	}
}

// TestAnalyzer_SummarizeCapturesOpts verifies that opts are forwarded to the implementation.
func TestAnalyzer_SummarizeCapturesOpts(t *testing.T) {
	mock := newMockAnalyzerWithNote(&heimdall.MeetingNote{})
	opts := heimdall.AnalyzeOpts{
		Model:        "claude-haiku-4-5",
		Participants: []string{"Alice", "Bob"},
		Keywords:     []string{"sprint", "roadmap"},
		Language:     "en",
	}

	_, err := mock.Summarize(context.Background(), nil, opts)
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if mock.capturedOpts.Model != opts.Model {
		t.Errorf("capturedOpts.Model: got %q, want %q", mock.capturedOpts.Model, opts.Model)
	}
	if len(mock.capturedOpts.Participants) != 2 {
		t.Errorf("capturedOpts.Participants: got %d, want 2", len(mock.capturedOpts.Participants))
	}
}

// TestAnalyzer_SummarizeWithEmptySegments verifies Summarize handles empty input gracefully.
func TestAnalyzer_SummarizeWithEmptySegments(t *testing.T) {
	note := &heimdall.MeetingNote{Title: "Empty Meeting", Summary: "No transcript."}
	var a Analyzer = newMockAnalyzerWithNote(note)

	result, err := a.Summarize(context.Background(), []heimdall.Segment{}, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize with empty segments: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for empty segments")
	}
}

// TestAnalyzer_SummarizeWithNilSegments verifies Summarize handles nil slice input.
func TestAnalyzer_SummarizeWithNilSegments(t *testing.T) {
	note := &heimdall.MeetingNote{Title: "Nil Segments", Summary: "No transcript."}
	var a Analyzer = newMockAnalyzerWithNote(note)

	result, err := a.Summarize(context.Background(), nil, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize with nil segments: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for nil segments")
	}
}

// TestAnalyzer_SummarizeReturnsFullNote verifies a fully-populated MeetingNote is returned.
func TestAnalyzer_SummarizeReturnsFullNote(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	expected := &heimdall.MeetingNote{
		Title:    "Sprint Review",
		Date:     now,
		Duration: 60 * time.Minute,
		Summary:  "Team reviewed sprint outcomes.",
		Decisions: []heimdall.Decision{
			{Description: "Ship feature X", DecidedBy: "Alice"},
		},
		ActionItems: []heimdall.ActionItem{
			{Task: "Deploy to staging", Owner: "Bob", Priority: "high"},
		},
		Topics: []heimdall.Topic{
			{Title: "Feature X", Content: "Completed and reviewed."},
		},
		Followups: []heimdall.Followup{
			{Question: "When is production deploy?", RaisedBy: "Charlie"},
		},
		SpeakerMap: map[int]string{0: "Alice", 1: "Bob", 2: "Charlie"},
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Let's begin.", IsFinal: true},
		},
		Platform: "Google Meet",
	}

	var a Analyzer = newMockAnalyzerWithNote(expected)
	result, err := a.Summarize(context.Background(), expected.Segments, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}

	if result.Title != expected.Title {
		t.Errorf("Title: got %q, want %q", result.Title, expected.Title)
	}
	if result.Duration != expected.Duration {
		t.Errorf("Duration: got %v, want %v", result.Duration, expected.Duration)
	}
	if len(result.Decisions) != 1 {
		t.Errorf("Decisions: got %d, want 1", len(result.Decisions))
	}
	if len(result.ActionItems) != 1 {
		t.Errorf("ActionItems: got %d, want 1", len(result.ActionItems))
	}
	if len(result.SpeakerMap) != 3 {
		t.Errorf("SpeakerMap: got %d, want 3", len(result.SpeakerMap))
	}
	if result.Platform != "Google Meet" {
		t.Errorf("Platform: got %q, want Google Meet", result.Platform)
	}
}

// TestAnalyzer_GracefulDegradation verifies that on error a nil note is returned
// (not a partial note), allowing the caller to implement the fallback to raw transcript.
func TestAnalyzer_GracefulDegradation(t *testing.T) {
	var a Analyzer = newMockAnalyzerWithError(errors.New("claude API error"))

	segments := []heimdall.Segment{
		{Speaker: 0, Text: "Meeting content.", IsFinal: true},
	}

	result, err := a.Summarize(context.Background(), segments, heimdall.AnalyzeOpts{})
	if err == nil {
		t.Fatal("expected an error for graceful degradation test")
	}
	if result != nil {
		t.Errorf("on error, result should be nil; got %+v", result)
	}
}
