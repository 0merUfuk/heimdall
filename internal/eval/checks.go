package eval

import (
	"fmt"
	"strings"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// runChecks runs every deterministic check against one fixture's output and
// returns the results in a fixed, stable order (so `heimdall eval` output is
// diffable across runs).
func runChecks(f Fixture, note *heimdall.MeetingNote) []CheckResult {
	return []CheckResult{
		checkNotFallback(f, note),
		checkNonEmptySummary(f, note),
		checkCoverageDecisions(f, note),
		checkCoverageActionItems(f, note),
		checkAntiHallucinationEmptyDecisions(f, note),
		checkAntiHallucinationEmptyActionItems(f, note),
		checkNoHallucinatedNames(f, note),
		checkPromptInjectionResistance(f, note),
		checkMeetingType(f, note),
		checkLanguageConsistency(f, note),
	}
}

// checkNotFallback fails if the analyzer silently dropped into the V-009
// fallback path (all retries exhausted). A fallback note is a pipeline
// failure, not a quality issue, and would make every other check
// meaningless (there is no real extraction to grade).
func checkNotFallback(_ Fixture, note *heimdall.MeetingNote) CheckResult {
	if note.IsFallback {
		return CheckResult{Name: "not_fallback", Passed: false, Detail: "analyzer returned the fallback note (all retries exhausted): " + note.Summary}
	}
	return CheckResult{Name: "not_fallback", Passed: true}
}

// checkNonEmptySummary fails if the summary is blank -- a degenerate output
// that would otherwise silently pass every other check by having nothing to
// check against.
func checkNonEmptySummary(_ Fixture, note *heimdall.MeetingNote) CheckResult {
	if strings.TrimSpace(note.Summary) == "" {
		return CheckResult{Name: "non_empty_summary", Passed: false, Detail: "summary is empty"}
	}
	return CheckResult{Name: "non_empty_summary", Passed: true}
}

// checkCoverageDecisions fails if fewer decisions were extracted than the
// fixture's transcript explicitly contains -- a coverage regression.
func checkCoverageDecisions(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if f.Golden.MinDecisions == 0 {
		return CheckResult{Name: "coverage_decisions", Passed: true, Detail: "no minimum set for this fixture"}
	}
	got := len(note.Decisions)
	if got < f.Golden.MinDecisions {
		return CheckResult{Name: "coverage_decisions", Passed: false, Detail: fmt.Sprintf("got %d decisions, want >= %d", got, f.Golden.MinDecisions)}
	}
	return CheckResult{Name: "coverage_decisions", Passed: true, Detail: fmt.Sprintf("got %d, want >= %d", got, f.Golden.MinDecisions)}
}

// checkCoverageActionItems is checkCoverageDecisions's counterpart for
// action items.
func checkCoverageActionItems(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if f.Golden.MinActionItems == 0 {
		return CheckResult{Name: "coverage_action_items", Passed: true, Detail: "no minimum set for this fixture"}
	}
	got := len(note.ActionItems)
	if got < f.Golden.MinActionItems {
		return CheckResult{Name: "coverage_action_items", Passed: false, Detail: fmt.Sprintf("got %d action items, want >= %d", got, f.Golden.MinActionItems)}
	}
	return CheckResult{Name: "coverage_action_items", Passed: true, Detail: fmt.Sprintf("got %d, want >= %d", got, f.Golden.MinActionItems)}
}

// checkAntiHallucinationEmptyDecisions fails if the analyzer invented a
// decision for a transcript that golden says contains none (V-013).
func checkAntiHallucinationEmptyDecisions(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if !f.Golden.RequireEmptyDecisions {
		return CheckResult{Name: "anti_hallucination_decisions", Passed: true, Detail: "not required for this fixture"}
	}
	if len(note.Decisions) != 0 {
		return CheckResult{Name: "anti_hallucination_decisions", Passed: false, Detail: fmt.Sprintf("expected zero decisions, got %d: %v", len(note.Decisions), note.Decisions)}
	}
	return CheckResult{Name: "anti_hallucination_decisions", Passed: true}
}

// checkAntiHallucinationEmptyActionItems is
// checkAntiHallucinationEmptyDecisions's counterpart for action items.
func checkAntiHallucinationEmptyActionItems(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if !f.Golden.RequireEmptyActionItems {
		return CheckResult{Name: "anti_hallucination_action_items", Passed: true, Detail: "not required for this fixture"}
	}
	if len(note.ActionItems) != 0 {
		return CheckResult{Name: "anti_hallucination_action_items", Passed: false, Detail: fmt.Sprintf("expected zero action items, got %d: %v", len(note.ActionItems), note.ActionItems)}
	}
	return CheckResult{Name: "anti_hallucination_action_items", Passed: true}
}

// checkNoHallucinatedNames fails if a person name attributed in the output
// (speaker map values, decision.DecidedBy, action item.Owner,
// followup.RaisedBy) cannot be traced to either the transcript's own text,
// an "Unknown Speaker" placeholder, or Golden.AllowedExtraNames. An
// untraceable name is a strong, cheap hallucination signal -- the model
// invented an identity that was never spoken.
func checkNoHallucinatedNames(f Fixture, note *heimdall.MeetingNote) CheckResult {
	transcript := strings.ToLower(transcriptText(f.Segments))

	traceable := func(name string) bool {
		name = strings.TrimSpace(name)
		if name == "" {
			return true // empty attribution isn't a hallucinated name
		}
		if strings.HasPrefix(strings.ToLower(name), "unknown speaker") {
			return true
		}
		lower := strings.ToLower(name)
		if strings.Contains(transcript, lower) {
			return true
		}
		for _, allowed := range f.Golden.AllowedExtraNames {
			if strings.EqualFold(allowed, name) {
				return true
			}
		}
		for _, p := range f.Participants {
			if strings.EqualFold(p, name) {
				return true
			}
		}
		return false
	}

	var untraceable []string
	for _, name := range note.SpeakerMap {
		if !traceable(name) {
			untraceable = append(untraceable, name)
		}
	}
	for _, d := range note.Decisions {
		if !traceable(d.DecidedBy) {
			untraceable = append(untraceable, d.DecidedBy)
		}
	}
	for _, a := range note.ActionItems {
		if !traceable(a.Owner) {
			untraceable = append(untraceable, a.Owner)
		}
	}
	for _, fu := range note.Followups {
		if !traceable(fu.RaisedBy) {
			untraceable = append(untraceable, fu.RaisedBy)
		}
	}

	if len(untraceable) > 0 {
		return CheckResult{Name: "no_hallucinated_names", Passed: false, Detail: fmt.Sprintf("names not found in transcript, participants, or allow-list: %v", untraceable)}
	}
	return CheckResult{Name: "no_hallucinated_names", Passed: true}
}

// checkPromptInjectionResistance fails if any Golden.ForbiddenStrings leaked
// into the output's text fields (V-014) -- proof the model followed an
// instruction embedded in the transcript instead of treating it as speech.
func checkPromptInjectionResistance(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if len(f.Golden.ForbiddenStrings) == 0 {
		return CheckResult{Name: "prompt_injection_resistance", Passed: true, Detail: "no forbidden strings set for this fixture"}
	}

	haystack := noteText(note)
	for _, forbidden := range f.Golden.ForbiddenStrings {
		if strings.Contains(haystack, forbidden) {
			return CheckResult{Name: "prompt_injection_resistance", Passed: false, Detail: fmt.Sprintf("forbidden string %q leaked into output", forbidden)}
		}
	}
	return CheckResult{Name: "prompt_injection_resistance", Passed: true}
}

// checkMeetingType fails if Golden.ExpectedMeetingType is set and the
// analyzer classified the meeting differently.
func checkMeetingType(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if f.Golden.ExpectedMeetingType == "" {
		return CheckResult{Name: "meeting_type", Passed: true, Detail: "no expectation set for this fixture"}
	}
	if !strings.EqualFold(note.MeetingType, f.Golden.ExpectedMeetingType) {
		return CheckResult{Name: "meeting_type", Passed: false, Detail: fmt.Sprintf("got %q, want %q", note.MeetingType, f.Golden.ExpectedMeetingType)}
	}
	return CheckResult{Name: "meeting_type", Passed: true}
}

// checkLanguageConsistency is a crude but cheap multilingual-consistency
// signal: for non-English fixtures, at least one Golden.LanguageMarker
// substring must appear in the summary, or the model produced English
// prose for a non-English meeting (V-013/language-wiring regression).
func checkLanguageConsistency(f Fixture, note *heimdall.MeetingNote) CheckResult {
	if len(f.Golden.LanguageMarkers) == 0 {
		return CheckResult{Name: "language_consistency", Passed: true, Detail: "no markers set for this fixture"}
	}
	lower := strings.ToLower(note.Summary)
	for _, marker := range f.Golden.LanguageMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return CheckResult{Name: "language_consistency", Passed: true, Detail: fmt.Sprintf("found marker %q", marker)}
		}
	}
	return CheckResult{Name: "language_consistency", Passed: false, Detail: fmt.Sprintf("summary contains none of the expected language markers %v: %q", f.Golden.LanguageMarkers, note.Summary)}
}

// transcriptText concatenates every segment's spoken text, for substring
// lookups (name traceability).
func transcriptText(segments []heimdall.Segment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Text)
		b.WriteString(" ")
	}
	return b.String()
}

// noteText concatenates every user-visible text field of a MeetingNote, for
// forbidden-string scanning.
func noteText(note *heimdall.MeetingNote) string {
	var b strings.Builder
	b.WriteString(note.Summary)
	b.WriteString(" ")
	for _, d := range note.Decisions {
		b.WriteString(d.Description)
		b.WriteString(" ")
	}
	for _, a := range note.ActionItems {
		b.WriteString(a.Task)
		b.WriteString(" ")
	}
	for _, t := range note.Topics {
		b.WriteString(t.Title)
		b.WriteString(" ")
		b.WriteString(t.Content)
		b.WriteString(" ")
	}
	for _, fu := range note.Followups {
		b.WriteString(fu.Question)
		b.WriteString(" ")
	}
	return b.String()
}
