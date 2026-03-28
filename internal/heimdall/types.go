// Package heimdall provides shared types used across all pipeline stages.
// These types define the data contracts between audio capture, transcription,
// analysis, and output rendering.
package heimdall

import (
	"fmt"
	"time"
)

// AudioFrame represents a chunk of raw PCM audio data from a capture source.
type AudioFrame struct {
	// Data contains raw PCM audio bytes.
	Data []byte

	// SampleRate is the sample rate in Hz (e.g., 16000, 48000).
	SampleRate int

	// Channels is the number of audio channels (1=mono, 2=stereo).
	Channels int

	// Timestamp is the offset from recording start.
	Timestamp time.Duration
}

// Segment represents a transcribed speech segment with speaker diarization.
type Segment struct {
	// Speaker is the speaker ID from diarization (0-based).
	Speaker int

	// Text is the transcribed text content.
	Text string

	// Start is the start time offset from recording start.
	Start time.Duration

	// End is the end time offset from recording start.
	End time.Duration

	// Confidence is the transcription confidence score (0.0-1.0).
	Confidence float64

	// Channel is the audio channel this segment came from (0=left/system, 1=right/mic).
	Channel int

	// IsFinal indicates whether this is a final (not interim) transcription result.
	IsFinal bool
}

// Timestamp returns a formatted timestamp string suitable for display (e.g., "00:01:12").
// This is a convenience method that formats the Start time for template rendering.
func (s Segment) Timestamp() string {
	total := int(s.Start.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// Participant represents a meeting participant identified from the transcript.
type Participant struct {
	// Name is the participant's display name.
	Name string
}

// MeetingNote represents the complete output of a meeting analysis.
type MeetingNote struct {
	// Title is the meeting title.
	Title string

	// Date is the meeting date/time.
	Date time.Time

	// Duration is the total meeting duration.
	Duration time.Duration

	// Summary is the LLM-generated meeting summary.
	Summary string

	// Decisions contains key decisions made during the meeting.
	Decisions []Decision

	// ActionItems contains action items extracted from the meeting.
	ActionItems []ActionItem

	// Topics contains discussion topics covered.
	Topics []Topic

	// Followups contains follow-up questions/items.
	Followups []Followup

	// SpeakerMap maps speaker IDs to identified names.
	SpeakerMap map[int]string

	// Segments contains the full transcript segments.
	Segments []Segment

	// Participants contains identified meeting participants.
	Participants []Participant

	// MeetingType categorizes the meeting (e.g., "standup", "1:1", "planning").
	MeetingType string

	// Platform is the meeting platform (e.g., "Zoom", "Google Meet").
	Platform string
}

// Decision represents a key decision made during the meeting.
type Decision struct {
	// Description is what was decided.
	Description string

	// DecidedBy is the person who made/announced the decision.
	DecidedBy string
}

// ActionItem represents a task extracted from the meeting.
type ActionItem struct {
	// Task is the description of the action item.
	Task string

	// Owner is the person responsible.
	Owner string

	// Deadline is the deadline if mentioned; may be empty.
	Deadline string

	// Priority is the priority level: "high", "medium", "low".
	Priority string
}

// Topic represents a discussion topic covered in the meeting.
type Topic struct {
	// Title is the topic title/heading.
	Title string

	// Content is the summary of discussion on this topic.
	Content string
}

// Followup represents a follow-up question or item raised during the meeting.
type Followup struct {
	// Question is the follow-up question or item.
	Question string

	// RaisedBy is the person who raised it.
	RaisedBy string
}

// TranscribeOpts contains configuration for the transcription provider.
type TranscribeOpts struct {
	// Language is the language code (e.g., "en").
	Language string

	// Model is the model name (e.g., "nova-3").
	Model string

	// SampleRate is the audio sample rate in Hz.
	SampleRate int

	// Channels is the number of audio channels.
	Channels int

	// Encoding is the audio encoding (e.g., "linear16").
	Encoding string

	// Diarize enables speaker diarization.
	Diarize bool

	// Punctuate enables auto-punctuation.
	Punctuate bool

	// SmartFormat enables smart formatting.
	SmartFormat bool

	// Keywords contains custom vocabulary boost words.
	Keywords []string
}

// AnalyzeOpts contains configuration for the LLM analyzer.
type AnalyzeOpts struct {
	// Model is the LLM model name (e.g., "claude-haiku-4-5").
	Model string

	// Participants contains known participant names (hints for speaker identification).
	Participants []string

	// Keywords contains meeting context keywords.
	Keywords []string

	// Language is the output language.
	Language string
}
