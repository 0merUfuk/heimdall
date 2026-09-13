// Package eval measures the quality of Analyzer output against a golden
// dataset of synthetic meeting transcripts. It is the only place in heimdall
// that evaluates *what the LLM said*, as opposed to whether the pipeline
// plumbing works -- everything in internal/analyzer's own test suite uses
// canned/mocked responses (per .claude/rules/go-net-http-services.md and the
// project's "tests use mocks, not real external services" convention) and so
// can never catch a real regression in extraction accuracy, hallucination,
// or multilingual output quality. This package closes that gap.
//
// Two layers, by design:
//
//   - Deterministic checks (checks.go): pure functions over a Fixture and the
//     MeetingNote an Analyzer produced for it. Free, instant, fully unit-
//     tested with a scripted Analyzer -- no live API calls. These catch
//     coverage regressions (dropped decisions/action items), anti-
//     hallucination violations (invented items when the golden expects
//     none, invented names untraceable to the transcript), and prompt-
//     injection leakage (V-014).
//
//   - LLM-as-judge (judge.go): an optional, explicitly opt-in layer that
//     asks a Claude model to rate faithfulness and coverage on a 0-10 scale
//     given the transcript and the produced note. This costs real API
//     tokens and is never invoked by `go test` -- only by the `heimdall
//     eval --judge` CLI path, which a human runs deliberately (e.g. before
//     tagging a release).
//
// Run the full suite with: heimdall eval
package eval

import (
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Fixture is one golden test case: a synthetic meeting transcript plus the
// ground-truth expectations an Analyzer's output is checked against.
//
// Every Golden field is a *minimum bar* or a *forbidden pattern*, never an
// exact-match requirement -- LLM prose varies run to run even at
// temperature 0, so checks assert presence/absence/counts, never string
// equality against generated text.
type Fixture struct {
	ID          string
	Description string

	// Language is passed as heimdall.AnalyzeOpts.Language. "en" (or empty)
	// for English fixtures; a BCP-47-ish code otherwise (e.g. "tr").
	Language string

	// Participants seeds heimdall.AnalyzeOpts.Participants, mirroring how
	// `heimdall record --participants` hints speaker identification.
	Participants []string

	Segments []heimdall.Segment
	Golden   Golden
}

// Golden captures the ground-truth expectations for one Fixture.
type Golden struct {
	// MinDecisions / MinActionItems: the transcript explicitly describes at
	// least this many distinct decisions/action items; the analyzer must
	// not drop them below this floor (coverage).
	MinDecisions   int
	MinActionItems int

	// RequireEmptyDecisions / RequireEmptyActionItems: for fixtures with NO
	// decisions or action items actually present in the transcript, the
	// analyzer must return an empty slice, not invent one (V-013).
	RequireEmptyDecisions   bool
	RequireEmptyActionItems bool

	// ForbiddenStrings must not appear anywhere in the analyzer's output
	// text (summary, decision descriptions, action item tasks, topic
	// content, followup questions). Used for the prompt-injection-
	// resistance check (V-014): a decoy payload embedded in a transcript
	// segment that, if it leaked into the output verbatim, would prove the
	// model treated injected text as an instruction rather than as speech.
	ForbiddenStrings []string

	// ExpectedMeetingType, if non-empty, is checked against note.MeetingType.
	ExpectedMeetingType string

	// LanguageMarkers are substrings expected to appear in the summary when
	// Language is non-English -- a crude but cheap multilingual-consistency
	// signal (e.g. Turkish-specific words/diacritics). Leave empty for
	// English fixtures.
	LanguageMarkers []string

	// AllowedExtraNames lists names that may legitimately appear in the
	// output even though they are not spoken verbatim in any segment (e.g.
	// a participant hint that never actually speaks on-mic). Everything
	// else attributed to a person must be traceable to Segments text or to
	// an "Unknown Speaker N" placeholder -- see checkNoHallucinatedNames.
	AllowedExtraNames []string
}

// CheckResult is the outcome of one deterministic check against one
// Fixture's Analyzer output.
type CheckResult struct {
	Name   string
	Passed bool
	Detail string // human-readable explanation, always populated on failure
}

// FixtureResult is every check's outcome for one Fixture, plus the note that
// produced them (kept for --json output / debugging) and an optional Judge
// score when LLM-as-judge scoring was requested.
type FixtureResult struct {
	Fixture Fixture
	Note    *heimdall.MeetingNote
	Checks  []CheckResult
	Judge   *JudgeResult // nil unless judge scoring was requested and succeeded
	Err     error        // non-nil if Summarize itself failed unrecoverably
}

// Passed reports whether every deterministic check in this result passed.
// Judge scores do not gate Passed -- they are informative, not pass/fail,
// since LLM-as-judge scoring is inherently noisier than the deterministic
// checks and is reported separately.
func (r FixtureResult) Passed() bool {
	if r.Err != nil {
		return false
	}
	for _, c := range r.Checks {
		if !c.Passed {
			return false
		}
	}
	return true
}

// Report aggregates FixtureResults across a full eval run.
type Report struct {
	Results []FixtureResult
}

// Passed reports whether every fixture in the report passed every check.
func (r Report) Passed() bool {
	for _, res := range r.Results {
		if !res.Passed() {
			return false
		}
	}
	return true
}

// Counts returns (fixtures passed, fixtures total).
func (r Report) Counts() (passed, total int) {
	total = len(r.Results)
	for _, res := range r.Results {
		if res.Passed() {
			passed++
		}
	}
	return passed, total
}
