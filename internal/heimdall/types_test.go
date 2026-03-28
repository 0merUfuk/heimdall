package heimdall

import (
	"testing"
	"time"
)

// TestAudioFrame_MicrophoneValues validates construction with typical microphone values.
func TestAudioFrame_MicrophoneValues(t *testing.T) {
	data := make([]byte, 3200) // 100ms at 16kHz, mono, 16-bit = 3200 bytes
	frame := AudioFrame{
		Data:       data,
		SampleRate: 16000,
		Channels:   1,
		Timestamp:  0,
	}

	if frame.SampleRate != 16000 {
		t.Errorf("SampleRate: got %d, want 16000", frame.SampleRate)
	}
	if frame.Channels != 1 {
		t.Errorf("Channels: got %d, want 1", frame.Channels)
	}
	if len(frame.Data) != 3200 {
		t.Errorf("len(Data): got %d, want 3200", len(frame.Data))
	}
	if frame.Timestamp != 0 {
		t.Errorf("Timestamp: got %v, want 0", frame.Timestamp)
	}
}

// TestAudioFrame_SystemAudioValues validates construction with typical system audio values.
func TestAudioFrame_SystemAudioValues(t *testing.T) {
	// 100ms at 48kHz, stereo, 32-bit float = 48000 * 0.1 * 2ch * 4bytes = 38400 bytes
	data := make([]byte, 38400)
	frame := AudioFrame{
		Data:       data,
		SampleRate: 48000,
		Channels:   2,
		Timestamp:  500 * time.Millisecond,
	}

	if frame.SampleRate != 48000 {
		t.Errorf("SampleRate: got %d, want 48000", frame.SampleRate)
	}
	if frame.Channels != 2 {
		t.Errorf("Channels: got %d, want 2", frame.Channels)
	}
	if len(frame.Data) != 38400 {
		t.Errorf("len(Data): got %d, want 38400", len(frame.Data))
	}
	if frame.Timestamp != 500*time.Millisecond {
		t.Errorf("Timestamp: got %v, want 500ms", frame.Timestamp)
	}
}

// TestAudioFrame_TableDriven validates various AudioFrame configurations.
func TestAudioFrame_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		channels   int
		dataLen    int
		timestamp  time.Duration
	}{
		{
			name:       "microphone 16kHz mono",
			sampleRate: 16000,
			channels:   1,
			dataLen:    3200,
			timestamp:  0,
		},
		{
			name:       "system audio 48kHz stereo",
			sampleRate: 48000,
			channels:   2,
			dataLen:    38400,
			timestamp:  1 * time.Second,
		},
		{
			name:       "interleaved stereo after mixing",
			sampleRate: 16000,
			channels:   2,
			dataLen:    6400,
			timestamp:  2500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := AudioFrame{
				Data:       make([]byte, tt.dataLen),
				SampleRate: tt.sampleRate,
				Channels:   tt.channels,
				Timestamp:  tt.timestamp,
			}
			if frame.SampleRate != tt.sampleRate {
				t.Errorf("SampleRate: got %d, want %d", frame.SampleRate, tt.sampleRate)
			}
			if frame.Channels != tt.channels {
				t.Errorf("Channels: got %d, want %d", frame.Channels, tt.channels)
			}
			if len(frame.Data) != tt.dataLen {
				t.Errorf("len(Data): got %d, want %d", len(frame.Data), tt.dataLen)
			}
			if frame.Timestamp != tt.timestamp {
				t.Errorf("Timestamp: got %v, want %v", frame.Timestamp, tt.timestamp)
			}
		})
	}
}

// TestAudioFrame_ZeroValue validates that zero value is sensible (no panics, zero fields).
func TestAudioFrame_ZeroValue(t *testing.T) {
	var frame AudioFrame
	if frame.SampleRate != 0 {
		t.Errorf("zero SampleRate: got %d, want 0", frame.SampleRate)
	}
	if frame.Channels != 0 {
		t.Errorf("zero Channels: got %d, want 0", frame.Channels)
	}
	if frame.Data != nil {
		t.Errorf("zero Data: got non-nil, want nil")
	}
	if frame.Timestamp != 0 {
		t.Errorf("zero Timestamp: got %v, want 0", frame.Timestamp)
	}
}

// TestSegment_InterimResult validates construction of an interim transcription segment.
func TestSegment_InterimResult(t *testing.T) {
	seg := Segment{
		Speaker:    0,
		Text:       "Hello, this is a test",
		Start:      1 * time.Second,
		End:        3 * time.Second,
		Confidence: 0.85,
		Channel:    0,
		IsFinal:    false,
	}

	if seg.IsFinal {
		t.Error("expected IsFinal=false for interim segment")
	}
	if seg.Speaker != 0 {
		t.Errorf("Speaker: got %d, want 0", seg.Speaker)
	}
	if seg.Text != "Hello, this is a test" {
		t.Errorf("Text: got %q, want %q", seg.Text, "Hello, this is a test")
	}
	if seg.Channel != 0 {
		t.Errorf("Channel: got %d, want 0 (left/system)", seg.Channel)
	}
}

// TestSegment_FinalResult validates construction of a final transcription segment.
func TestSegment_FinalResult(t *testing.T) {
	seg := Segment{
		Speaker:    1,
		Text:       "This is a final transcription.",
		Start:      5 * time.Second,
		End:        8500 * time.Millisecond,
		Confidence: 0.97,
		Channel:    1,
		IsFinal:    true,
	}

	if !seg.IsFinal {
		t.Error("expected IsFinal=true for final segment")
	}
	if seg.Speaker != 1 {
		t.Errorf("Speaker: got %d, want 1", seg.Speaker)
	}
	if seg.Confidence != 0.97 {
		t.Errorf("Confidence: got %f, want 0.97", seg.Confidence)
	}
	if seg.Channel != 1 {
		t.Errorf("Channel: got %d, want 1 (right/mic)", seg.Channel)
	}
	duration := seg.End - seg.Start
	if duration != 3500*time.Millisecond {
		t.Errorf("segment duration: got %v, want 3500ms", duration)
	}
}

// TestSegment_ZeroValue validates that zero value Segment is sensible.
func TestSegment_ZeroValue(t *testing.T) {
	var seg Segment
	if seg.Speaker != 0 {
		t.Errorf("zero Speaker: got %d, want 0", seg.Speaker)
	}
	if seg.Text != "" {
		t.Errorf("zero Text: got %q, want empty string", seg.Text)
	}
	if seg.IsFinal {
		t.Error("zero IsFinal: got true, want false")
	}
	if seg.Confidence != 0 {
		t.Errorf("zero Confidence: got %f, want 0", seg.Confidence)
	}
}

// TestMeetingNote_FullyPopulated validates constructing a complete MeetingNote.
func TestMeetingNote_FullyPopulated(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	note := MeetingNote{
		Title:    "Q1 Sprint Planning",
		Date:     now,
		Duration: 45 * time.Minute,
		Summary:  "Team aligned on sprint goals and capacity.",
		Platform: "Zoom",
		Decisions: []Decision{
			{Description: "Use Redis for caching", DecidedBy: "Alice"},
			{Description: "Defer auth refactor to Q2", DecidedBy: "Bob"},
		},
		ActionItems: []ActionItem{
			{Task: "Set up Redis cluster", Owner: "Charlie", Deadline: "2026-04-04", Priority: "high"},
			{Task: "Write ADR for caching", Owner: "Alice", Deadline: "", Priority: "medium"},
		},
		Topics: []Topic{
			{Title: "Sprint capacity", Content: "Team at 80% capacity due to PTO."},
			{Title: "Tech debt", Content: "Auth refactor deferred."},
		},
		Followups: []Followup{
			{Question: "What is the Redis licensing cost?", RaisedBy: "Bob"},
		},
		SpeakerMap: map[int]string{
			0: "Alice",
			1: "Bob",
			2: "Charlie",
		},
		Segments: []Segment{
			{Speaker: 0, Text: "Let's start.", Start: 0, End: 2 * time.Second, Confidence: 0.99, IsFinal: true},
		},
	}

	if note.Title != "Q1 Sprint Planning" {
		t.Errorf("Title: got %q, want %q", note.Title, "Q1 Sprint Planning")
	}
	if note.Duration != 45*time.Minute {
		t.Errorf("Duration: got %v, want 45m", note.Duration)
	}
	if len(note.Decisions) != 2 {
		t.Errorf("Decisions: got %d, want 2", len(note.Decisions))
	}
	if len(note.ActionItems) != 2 {
		t.Errorf("ActionItems: got %d, want 2", len(note.ActionItems))
	}
	if len(note.Topics) != 2 {
		t.Errorf("Topics: got %d, want 2", len(note.Topics))
	}
	if len(note.Followups) != 1 {
		t.Errorf("Followups: got %d, want 1", len(note.Followups))
	}
	if len(note.SpeakerMap) != 3 {
		t.Errorf("SpeakerMap: got %d entries, want 3", len(note.SpeakerMap))
	}
	if len(note.Segments) != 1 {
		t.Errorf("Segments: got %d, want 1", len(note.Segments))
	}
	if note.Platform != "Zoom" {
		t.Errorf("Platform: got %q, want %q", note.Platform, "Zoom")
	}
}

// TestMeetingNote_SpeakerMap validates create, read, and update operations on SpeakerMap.
func TestMeetingNote_SpeakerMap(t *testing.T) {
	note := MeetingNote{
		SpeakerMap: make(map[int]string),
	}

	// Create
	note.SpeakerMap[0] = "Alice"
	note.SpeakerMap[1] = "Bob"
	if len(note.SpeakerMap) != 2 {
		t.Fatalf("after create: SpeakerMap len = %d, want 2", len(note.SpeakerMap))
	}

	// Read
	if got := note.SpeakerMap[0]; got != "Alice" {
		t.Errorf("SpeakerMap[0]: got %q, want Alice", got)
	}
	if got := note.SpeakerMap[1]; got != "Bob" {
		t.Errorf("SpeakerMap[1]: got %q, want Bob", got)
	}

	// Update
	note.SpeakerMap[0] = "Alice Smith"
	if got := note.SpeakerMap[0]; got != "Alice Smith" {
		t.Errorf("SpeakerMap[0] after update: got %q, want Alice Smith", got)
	}

	// Missing key returns zero value
	if got := note.SpeakerMap[99]; got != "" {
		t.Errorf("SpeakerMap[99] (missing): got %q, want empty string", got)
	}

	// Delete
	delete(note.SpeakerMap, 1)
	if _, ok := note.SpeakerMap[1]; ok {
		t.Error("SpeakerMap[1] should not exist after delete")
	}
}

// TestMeetingNote_ZeroValue validates that zero value MeetingNote has sensible defaults.
func TestMeetingNote_ZeroValue(t *testing.T) {
	var note MeetingNote
	if note.Title != "" {
		t.Errorf("zero Title: got %q, want empty string", note.Title)
	}
	if note.Summary != "" {
		t.Errorf("zero Summary: got %q, want empty string", note.Summary)
	}
	if note.Decisions != nil {
		t.Error("zero Decisions: got non-nil, want nil")
	}
	if note.ActionItems != nil {
		t.Error("zero ActionItems: got non-nil, want nil")
	}
	if note.Topics != nil {
		t.Error("zero Topics: got non-nil, want nil")
	}
	if note.Followups != nil {
		t.Error("zero Followups: got non-nil, want nil")
	}
	if note.SpeakerMap != nil {
		t.Error("zero SpeakerMap: got non-nil, want nil")
	}
	if note.Segments != nil {
		t.Error("zero Segments: got non-nil, want nil")
	}
	if !note.Date.IsZero() {
		t.Errorf("zero Date: got %v, want zero time", note.Date)
	}
}

// TestDecision validates field access on Decision.
func TestDecision(t *testing.T) {
	d := Decision{
		Description: "Migrate to PostgreSQL",
		DecidedBy:   "Engineering Lead",
	}
	if d.Description != "Migrate to PostgreSQL" {
		t.Errorf("Description: got %q, want %q", d.Description, "Migrate to PostgreSQL")
	}
	if d.DecidedBy != "Engineering Lead" {
		t.Errorf("DecidedBy: got %q, want %q", d.DecidedBy, "Engineering Lead")
	}
}

// TestActionItem validates field access and all priority levels.
func TestActionItem(t *testing.T) {
	tests := []struct {
		name     string
		priority string
	}{
		{"high priority", "high"},
		{"medium priority", "medium"},
		{"low priority", "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ActionItem{
				Task:     "Complete the review",
				Owner:    "Alice",
				Deadline: "2026-04-01",
				Priority: tt.priority,
			}
			if item.Priority != tt.priority {
				t.Errorf("Priority: got %q, want %q", item.Priority, tt.priority)
			}
			if item.Task == "" {
				t.Error("Task should not be empty")
			}
			if item.Owner == "" {
				t.Error("Owner should not be empty")
			}
		})
	}
}

// TestActionItem_EmptyDeadline validates that Deadline is optional (may be empty).
func TestActionItem_EmptyDeadline(t *testing.T) {
	item := ActionItem{
		Task:     "Write tests",
		Owner:    "Bob",
		Deadline: "",
		Priority: "low",
	}
	if item.Deadline != "" {
		t.Errorf("Deadline should be empty, got %q", item.Deadline)
	}
}

// TestTopic validates field access on Topic.
func TestTopic(t *testing.T) {
	topic := Topic{
		Title:   "Architecture Review",
		Content: "Discussed moving to microservices.",
	}
	if topic.Title != "Architecture Review" {
		t.Errorf("Title: got %q, want %q", topic.Title, "Architecture Review")
	}
	if topic.Content == "" {
		t.Error("Content should not be empty")
	}
}

// TestFollowup validates field access on Followup.
func TestFollowup(t *testing.T) {
	f := Followup{
		Question: "Will we need extra capacity?",
		RaisedBy: "Charlie",
	}
	if f.Question != "Will we need extra capacity?" {
		t.Errorf("Question: got %q, want %q", f.Question, "Will we need extra capacity?")
	}
	if f.RaisedBy != "Charlie" {
		t.Errorf("RaisedBy: got %q, want %q", f.RaisedBy, "Charlie")
	}
}

// TestTranscribeOpts validates typical construction.
func TestTranscribeOpts(t *testing.T) {
	opts := TranscribeOpts{
		Language:    "en",
		Model:       "nova-3",
		SampleRate:  16000,
		Channels:    2,
		Encoding:    "linear16",
		Diarize:     true,
		Punctuate:   true,
		SmartFormat: true,
		Keywords:    []string{"heimdall", "Deepgram", "Claude"},
	}

	if opts.Language != "en" {
		t.Errorf("Language: got %q, want en", opts.Language)
	}
	if opts.Model != "nova-3" {
		t.Errorf("Model: got %q, want nova-3", opts.Model)
	}
	if opts.SampleRate != 16000 {
		t.Errorf("SampleRate: got %d, want 16000", opts.SampleRate)
	}
	if opts.Channels != 2 {
		t.Errorf("Channels: got %d, want 2", opts.Channels)
	}
	if opts.Encoding != "linear16" {
		t.Errorf("Encoding: got %q, want linear16", opts.Encoding)
	}
	if !opts.Diarize {
		t.Error("Diarize: got false, want true")
	}
	if !opts.Punctuate {
		t.Error("Punctuate: got false, want true")
	}
	if !opts.SmartFormat {
		t.Error("SmartFormat: got false, want true")
	}
	if len(opts.Keywords) != 3 {
		t.Errorf("Keywords: got %d, want 3", len(opts.Keywords))
	}
}

// TestTranscribeOpts_ZeroValue validates zero value has safe defaults.
func TestTranscribeOpts_ZeroValue(t *testing.T) {
	var opts TranscribeOpts
	if opts.Diarize {
		t.Error("zero Diarize: got true, want false")
	}
	if opts.Punctuate {
		t.Error("zero Punctuate: got true, want false")
	}
	if opts.SmartFormat {
		t.Error("zero SmartFormat: got true, want false")
	}
	if opts.Keywords != nil {
		t.Error("zero Keywords: got non-nil, want nil")
	}
}

// TestAnalyzeOpts validates typical construction.
func TestAnalyzeOpts(t *testing.T) {
	opts := AnalyzeOpts{
		Model:        "claude-haiku-4-5",
		Participants: []string{"Alice", "Bob", "Charlie"},
		Keywords:     []string{"sprint", "Q1", "roadmap"},
		Language:     "en",
	}

	if opts.Model != "claude-haiku-4-5" {
		t.Errorf("Model: got %q, want claude-haiku-4-5", opts.Model)
	}
	if len(opts.Participants) != 3 {
		t.Errorf("Participants: got %d, want 3", len(opts.Participants))
	}
	if len(opts.Keywords) != 3 {
		t.Errorf("Keywords: got %d, want 3", len(opts.Keywords))
	}
	if opts.Language != "en" {
		t.Errorf("Language: got %q, want en", opts.Language)
	}
}

// TestAnalyzeOpts_ZeroValue validates zero value has safe defaults.
func TestAnalyzeOpts_ZeroValue(t *testing.T) {
	var opts AnalyzeOpts
	if opts.Model != "" {
		t.Errorf("zero Model: got %q, want empty string", opts.Model)
	}
	if opts.Participants != nil {
		t.Error("zero Participants: got non-nil, want nil")
	}
	if opts.Keywords != nil {
		t.Error("zero Keywords: got non-nil, want nil")
	}
}
