package eval

import (
	"context"
	"time"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// perFixtureTimeout bounds a single fixture's Summarize call. Fixtures are
// short (a handful of segments), so this is generous headroom for a live
// API/subprocess call plus the existing analyzer retry/backoff, not a tight
// budget.
const perFixtureTimeout = 60 * time.Second

// RunOptions configures a Run.
type RunOptions struct {
	// Judge, when non-nil, is invoked after the deterministic checks for
	// every fixture that passed Summarize, to additionally score
	// faithfulness/coverage via LLM-as-judge. Nil skips judging entirely
	// (the default -- judging costs real API tokens and is opt-in only).
	Judge JudgeFunc
}

// Run executes every fixture in fixtures against a, running the
// deterministic checks (and, if opts.Judge is set, LLM-as-judge scoring) on
// each result. It never returns an error itself -- a fixture whose
// Summarize call fails is recorded in that FixtureResult.Err and still
// contributes to the Report so a single bad fixture doesn't hide the rest.
func Run(ctx context.Context, a analyzer.Analyzer, fixtures []Fixture, opts RunOptions) Report {
	report := Report{Results: make([]FixtureResult, 0, len(fixtures))}

	for _, f := range fixtures {
		result := FixtureResult{Fixture: f}

		callCtx, cancel := context.WithTimeout(ctx, perFixtureTimeout)
		note, err := a.Summarize(callCtx, f.Segments, heimdall.AnalyzeOpts{
			Language:     f.Language,
			Participants: f.Participants,
		})
		cancel()

		if err != nil {
			result.Err = err
			report.Results = append(report.Results, result)
			continue
		}

		result.Note = note
		result.Checks = runChecks(f, note)

		if opts.Judge != nil && !note.IsFallback {
			judgeCtx, judgeCancel := context.WithTimeout(ctx, perFixtureTimeout)
			if jr, jerr := opts.Judge(judgeCtx, f, note); jerr == nil {
				result.Judge = jr
			}
			// A judge error is non-fatal: deterministic checks already ran
			// and stand on their own. The report simply omits Judge for
			// this fixture; callers that print it handle a nil Judge.
			judgeCancel()
		}

		report.Results = append(report.Results, result)
	}

	return report
}
