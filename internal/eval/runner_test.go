package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// scriptedAnalyzer is a mock analyzer.Analyzer for testing the Run
// orchestration without any live API/subprocess calls, matching the
// project's "tests use mocks, not real external services" convention
// (.claude/rules/go-net-http-services.md).
type scriptedAnalyzer struct {
	// respond, keyed by Fixture.ID (recovered from context via
	// segments[0].Text as a cheap correlation key in these tests), returns
	// the note/error to hand back. If nil, a default passing note is returned.
	respond func(segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error)
	calls   int
}

func (s *scriptedAnalyzer) Summarize(_ context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	s.calls++
	if s.respond != nil {
		return s.respond(segments, opts)
	}
	return &heimdall.MeetingNote{Summary: "ok"}, nil
}

func TestRun_AllFixturesPass(t *testing.T) {
	mock := &scriptedAnalyzer{
		respond: func(segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
			// A generous note that satisfies every fixture's minimums
			// regardless of which one is being processed, so this test
			// exercises orchestration (all fixtures run, results recorded),
			// not extraction quality (that's fixtures.go + checks_test.go's job).
			return &heimdall.MeetingNote{
				Summary: "Discussed the migration. ı ş ğ.", // satisfies Turkish LanguageMarkers too
				Decisions: []heimdall.Decision{
					{Description: "d1"}, {Description: "d2"}, {Description: "d3"}, {Description: "d4"}, {Description: "d5"},
				},
				ActionItems: []heimdall.ActionItem{
					{Task: "a1"}, {Task: "a2"}, {Task: "a3"}, {Task: "a4"}, {Task: "a5"},
				},
				MeetingType: "standup",
			}, nil
		},
	}

	report := Run(context.Background(), mock, Fixtures(), RunOptions{})

	if mock.calls != len(Fixtures()) {
		t.Errorf("expected %d Summarize calls, got %d", len(Fixtures()), mock.calls)
	}
	if len(report.Results) != len(Fixtures()) {
		t.Fatalf("expected %d results, got %d", len(Fixtures()), len(report.Results))
	}

	for _, r := range report.Results {
		// Not asserting report.Passed() here -- a couple of fixtures
		// (anti-hallucination ones) require EMPTY decisions/action items,
		// which this generous note deliberately violates. Just confirm the
		// plumbing produced a result with checks for every fixture.
		if r.Err != nil {
			t.Errorf("fixture %s: unexpected Summarize error: %v", r.Fixture.ID, r.Err)
		}
		if len(r.Checks) == 0 {
			t.Errorf("fixture %s: expected checks to have run", r.Fixture.ID)
		}
	}
}

func TestRun_SummarizeErrorRecordedPerFixture(t *testing.T) {
	wantErr := errors.New("boom")
	mock := &scriptedAnalyzer{
		respond: func(segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
			return nil, wantErr
		},
	}

	report := Run(context.Background(), mock, Fixtures()[:2], RunOptions{})

	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Err == nil {
			t.Errorf("fixture %s: expected an error to be recorded", r.Fixture.ID)
		}
		if r.Passed() {
			t.Errorf("fixture %s: a Summarize error must not count as passed", r.Fixture.ID)
		}
	}
	if report.Passed() {
		t.Error("Report.Passed() should be false when every fixture errored")
	}
}

func TestRun_OneBadFixtureDoesNotHideTheRest(t *testing.T) {
	calls := 0
	mock := &scriptedAnalyzer{
		respond: func(segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("first fixture fails")
			}
			return &heimdall.MeetingNote{Summary: "ok"}, nil
		},
	}

	report := Run(context.Background(), mock, Fixtures()[:3], RunOptions{})

	if len(report.Results) != 3 {
		t.Fatalf("expected all 3 fixtures to produce a result despite the first one erroring, got %d", len(report.Results))
	}
	if report.Results[0].Err == nil {
		t.Error("expected the first fixture to have recorded an error")
	}
	for i := 1; i < 3; i++ {
		if report.Results[i].Err != nil {
			t.Errorf("fixture %d should not have been affected by fixture 0's error, got: %v", i, report.Results[i].Err)
		}
	}
}

func TestRun_JudgeInvokedWhenSet(t *testing.T) {
	mock := &scriptedAnalyzer{}
	judgeCalls := 0
	judge := func(ctx context.Context, f Fixture, note *heimdall.MeetingNote) (*JudgeResult, error) {
		judgeCalls++
		return &JudgeResult{Faithfulness: 9, Coverage: 8}, nil
	}

	fixtures := Fixtures()[:2]
	report := Run(context.Background(), mock, fixtures, RunOptions{Judge: judge})

	if judgeCalls != len(fixtures) {
		t.Errorf("expected judge to be called %d times, got %d", len(fixtures), judgeCalls)
	}
	for _, r := range report.Results {
		if r.Judge == nil {
			t.Errorf("fixture %s: expected a Judge result", r.Fixture.ID)
		}
	}
}

func TestRun_JudgeNotInvokedWhenNil(t *testing.T) {
	mock := &scriptedAnalyzer{}
	report := Run(context.Background(), mock, Fixtures()[:2], RunOptions{})

	for _, r := range report.Results {
		if r.Judge != nil {
			t.Errorf("fixture %s: expected no Judge result when RunOptions.Judge is nil", r.Fixture.ID)
		}
	}
}

func TestRun_JudgeSkippedOnFallbackNote(t *testing.T) {
	mock := &scriptedAnalyzer{
		respond: func(segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
			return &heimdall.MeetingNote{Summary: "Analysis failed", IsFallback: true}, nil
		},
	}
	judgeCalls := 0
	judge := func(ctx context.Context, f Fixture, note *heimdall.MeetingNote) (*JudgeResult, error) {
		judgeCalls++
		return &JudgeResult{}, nil
	}

	Run(context.Background(), mock, Fixtures()[:1], RunOptions{Judge: judge})

	if judgeCalls != 0 {
		t.Error("judge should not be invoked for a fallback note -- there is nothing real to grade")
	}
}

func TestReport_CountsAndPassed(t *testing.T) {
	report := Report{Results: []FixtureResult{
		{Checks: []CheckResult{{Passed: true}}},
		{Checks: []CheckResult{{Passed: false}}},
		{Err: errors.New("x")},
	}}

	passed, total := report.Counts()
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if passed != 1 {
		t.Errorf("passed: got %d, want 1", passed)
	}
	if report.Passed() {
		t.Error("Report.Passed() should be false when any fixture failed")
	}
}
