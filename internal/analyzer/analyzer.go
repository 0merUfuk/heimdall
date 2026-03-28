// Package analyzer defines the Analyzer interface for meeting intelligence providers.
// The primary implementation is ClaudeAnalyzer (Anthropic API) which performs
// post-meeting analysis including speaker identification, summarization,
// decision extraction, and action item extraction.
package analyzer

import (
	"context"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Analyzer defines the interface for meeting intelligence providers.
// The primary implementation is ClaudeAnalyzer (Anthropic API).
type Analyzer interface {
	// Summarize analyzes a complete meeting transcript and returns structured notes.
	// This is called once after the meeting ends (post-meeting processing).
	// Returns a MeetingNote with speaker identification, summary, decisions, action items, etc.
	// On failure after retries, returns a partial MeetingNote with raw transcript only.
	Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error)
}
