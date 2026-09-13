package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/eval"
)

var (
	evalAnalyzer string
	evalJudge    bool
	evalJSON     bool
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run the meeting-analysis quality suite against golden transcripts",
	Long: `Runs every golden fixture in internal/eval through the selected
--analyzer and checks the output for coverage (decisions/action items
weren't dropped), anti-hallucination (nothing invented when the transcript
has nothing to extract, no names untraceable to the transcript), prompt-
injection resistance, and multilingual consistency.

These are deterministic checks -- free, fast, no live LLM calls beyond the
analysis itself. Add --judge to additionally score faithfulness and
coverage via LLM-as-judge (an extra API call per fixture, using
ANTHROPIC_API_KEY regardless of --analyzer; costs real tokens).

Exits non-zero if any fixture fails its deterministic checks. Judge scores
are informative only and never affect the exit code.`,
	Example: `  heimdall eval                          # deterministic checks only, default (api) analyzer
  heimdall eval --analyzer claude-code    # same checks, via a local Claude Code login
  heimdall eval --judge                   # also score faithfulness/coverage via LLM-as-judge
  heimdall eval --json                    # machine-readable output for CI`,
	RunE: runEval,
}

func init() {
	evalCmd.Flags().StringVar(&evalAnalyzer, "analyzer", "api", "meeting-analysis backend under test: api (default) or claude-code")
	evalCmd.Flags().BoolVar(&evalJudge, "judge", false, "also score faithfulness/coverage via LLM-as-judge (extra API call per fixture, needs ANTHROPIC_API_KEY)")
	evalCmd.Flags().BoolVar(&evalJSON, "json", false, "print machine-readable JSON instead of a human-readable report")
	rootCmd.AddCommand(evalCmd)
}

func runEval(cmd *cobra.Command, args []string) error {
	switch evalAnalyzer {
	case "", analyzer.ProviderAPI, analyzer.ProviderClaudeCode:
	default:
		return fmt.Errorf("--analyzer %q is not valid: use %q (default) or %q",
			evalAnalyzer, analyzer.ProviderAPI, analyzer.ProviderClaudeCode)
	}

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	a, err := analyzer.NewFromName(evalAnalyzer, anthropicKey)
	if err != nil {
		return err
	}

	opts := eval.RunOptions{}
	if evalJudge {
		if anthropicKey == "" {
			return fmt.Errorf("--judge requires ANTHROPIC_API_KEY (the judge always uses the API directly, regardless of --analyzer)")
		}
		opts.Judge = eval.NewAPIJudge(anthropicKey, "").Judge
	}

	if !evalJSON {
		label := "the Anthropic API"
		if evalAnalyzer == analyzer.ProviderClaudeCode {
			label = "a local Claude Code login"
		}
		fmt.Printf("heimdall eval -- running %d fixtures via %s%s...\n\n", len(eval.Fixtures()), label, judgeSuffix(evalJudge))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	report := eval.Run(ctx, a, eval.Fixtures(), opts)

	if evalJSON {
		return printEvalJSON(report)
	}
	printEvalReport(report)

	if !report.Passed() {
		return fmt.Errorf("eval failed")
	}
	return nil
}

func judgeSuffix(judge bool) string {
	if judge {
		return " (with LLM-as-judge scoring)"
	}
	return ""
}

func printEvalReport(report eval.Report) {
	for _, r := range report.Results {
		status := "PASS"
		if !r.Passed() {
			status = "FAIL"
		}
		fmt.Printf("[%s] %s -- %s\n", status, r.Fixture.ID, r.Fixture.Description)

		if r.Err != nil {
			fmt.Printf("      error: %v\n", r.Err)
			continue
		}
		for _, c := range r.Checks {
			if c.Passed {
				continue // only print failures inline; a clean run stays short
			}
			fmt.Printf("      FAIL %s: %s\n", c.Name, c.Detail)
		}
		if r.Judge != nil {
			fmt.Printf("      judge: faithfulness=%d/10 coverage=%d/10 (%s)\n", r.Judge.Faithfulness, r.Judge.Coverage, r.Judge.Latency.Round(time.Millisecond))
			for _, h := range r.Judge.Hallucinations {
				fmt.Printf("        hallucination: %s\n", h)
			}
			for _, m := range r.Judge.Missed {
				fmt.Printf("        missed: %s\n", m)
			}
		}
	}

	passed, total := report.Counts()
	fmt.Printf("\n%d/%d fixtures passed.\n", passed, total)
}

// evalJSONReport is a JSON-serializable projection of eval.Report --
// eval.FixtureResult carries a raw `error`, which does not marshal
// meaningfully on its own, so the CLI layer flattens it to a string here
// rather than teaching the eval package about JSON output concerns.
type evalJSONReport struct {
	Passed  bool                  `json:"passed"`
	Total   int                   `json:"total"`
	Fixture []evalJSONFixtureItem `json:"fixtures"`
}

type evalJSONFixtureItem struct {
	ID     string             `json:"id"`
	Passed bool               `json:"passed"`
	Error  string             `json:"error,omitempty"`
	Checks []eval.CheckResult `json:"checks,omitempty"`
	Judge  *eval.JudgeResult  `json:"judge,omitempty"`
}

func printEvalJSON(report eval.Report) error {
	out := evalJSONReport{Passed: report.Passed()}
	for _, r := range report.Results {
		item := evalJSONFixtureItem{ID: r.Fixture.ID, Passed: r.Passed(), Checks: r.Checks, Judge: r.Judge}
		if r.Err != nil {
			item.Error = r.Err.Error()
		}
		out.Fixture = append(out.Fixture, item)
	}
	out.Total = len(out.Fixture)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding eval report: %w", err)
	}
	if !out.Passed {
		return fmt.Errorf("eval failed")
	}
	return nil
}
