# Heimdall — Evaluation

**Version**: 1.0
**Created**: 2026-09-13
**Authors:** Omer Ufuk (synthesized via autonomous engineering session)

---

Stage 5 (ANALYZE) is the only LLM stage in heimdall's pipeline, and until now it had zero measurement beyond `internal/analyzer`'s own unit tests -- which only prove the JSON-parsing plumbing works against *canned* responses (per `.claude/rules/go-net-http-services.md`'s "tests use mocks, not real external services" convention). Nothing measured whether a real model call on a real transcript actually produces a faithful, complete, non-hallucinated, correctly-localized meeting note. This document describes what closes that gap: `internal/eval` and the `heimdall eval` command.

## What is measured

| Dimension | Mechanism | Cost |
|---|---|---|
| **Coverage** -- did extraction drop a decision/action item the transcript explicitly contains? | Deterministic: `len(note.Decisions) >= Golden.MinDecisions` (and the action-item equivalent) | Free |
| **Anti-hallucination (empty case)** -- did the model invent a decision/action item for a transcript that has none? | Deterministic: golden fixtures with genuinely no decisions/action items assert an empty slice back | Free |
| **Anti-hallucination (names)** -- did the model attribute a decision/action item to a person who was never actually in the transcript? | Deterministic: every name in `speaker_map`, `decided_by`, `owner`, `raised_by` must be traceable to the fixture's own transcript text, its `Participants` hint, an explicit allow-list, or an "Unknown Speaker N" placeholder | Free |
| **Prompt-injection resistance (V-014)** | Deterministic: a fixture transcript carries a decoy payload instructing the model to output a specific marker string; the check fails if that marker leaks into any output field | Free |
| **Multilingual consistency** | Deterministic (crude but cheap): non-English fixtures assert the summary contains at least one language-specific marker substring (e.g. Turkish diacritics), catching a "language wiring" regression where output silently reverts to English | Free |
| **Faithfulness (nuanced)** -- is every claim in the output actually supported by the transcript, beyond what a name-traceability check alone catches? | LLM-as-judge (`--judge`): a stronger model (default `claude-sonnet-5`) scores 0-10 given the transcript and the output | Real API tokens |
| **Coverage (nuanced)** -- did the output miss something material that a simple item-count wouldn't catch? | LLM-as-judge (`--judge`), same call as faithfulness | Real API tokens |

The deterministic layer is the primary, always-free signal and is what fails the command (non-zero exit). The judge layer is informative supplementary detail, never a pass/fail gate -- LLM-as-judge scoring is itself noisy, and gating CI on it would trade a flaky, expensive signal for the free, deterministic one that already exists.

## Golden dataset

Seven fixtures in `internal/eval/fixtures.go`, each a synthetic transcript with hand-written ground truth:

| Fixture | Exercises |
|---|---|
| `en-standup-basic` | Baseline: one decision, two action items with owners/deadlines |
| `en-casual-no-decisions` | Anti-hallucination floor: pure small talk, nothing to extract |
| `tr-standup` | Turkish output (`--language tr`) |
| `en-prompt-injection` | V-014: decoy instruction embedded in a segment |
| `en-unknown-speakers` | No name ever spoken -- must not guess an identity |
| `en-dense-coverage` | Five decisions + five action items in one transcript -- coverage under length |
| `tr-en-code-switch` | Turkish meeting with mixed-in English technical terms |

Add a fixture whenever a real production failure is found -- a fixture reproducing a past bug is a regression test with teeth, the same discipline `internal/transcriber`'s reconnection tests already apply to V-001.

## Running it

```bash
heimdall eval                          # deterministic checks, default (api) analyzer -- needs ANTHROPIC_API_KEY
heimdall eval --analyzer claude-code   # same checks, via a local Claude Code login instead
heimdall eval --judge                  # + LLM-as-judge faithfulness/coverage scores (extra API call per fixture)
heimdall eval --json                   # machine-readable report for CI/scripting
make eval                              # builds heimdall, then runs the above
```

Exit code is non-zero if any fixture fails a deterministic check. `--judge` scores never affect the exit code.

## Why this design, not something heavier

- **No golden-output string matching.** LLM prose varies run to run even at the same settings; checks assert presence/absence/counts/traceability, never exact text equality against a "correct" summary. A brittle golden-string suite would fail on harmless rephrasing constantly and get disabled within a month.
- **Judge is opt-in, not wired into CI by default.** The public GitHub Actions workflow (`.github/workflows/ci.yml`) has no secrets configured, so a judge-gated CI run isn't possible without asking the maintainer to provision an API key as a repo secret -- a deliberate, human decision, not something to default into a public workflow silently. The deterministic layer needs no credentials at all when run with `--analyzer claude-code` and a local Claude Code login, or requires only the same `ANTHROPIC_API_KEY` a maintainer already needs to use heimdall itself.
- **Not a Python eval framework.** heimdall is a lean, 5-dependency Go binary by design (see `docs/GRILL_REPORT.md`'s "Meta-Engineering ROI" finding on process proportionality). Reaching for a separate Python eval stack (promptfoo, deepeval, ragas, ...) would add a second language, a second dependency tree, and a second CI runner just to grade output this package's own `heimdall` binary already knows how to parse. `internal/eval` is ~600 lines of plain Go, reuses the exact `analyzer.Analyzer` interface production code runs through, and ships in the same binary.

## Known limitation

This was built in a sandbox with no logged-in `claude` CLI and, at authoring time, no `ANTHROPIC_API_KEY` configured -- so while every check function, the runner's orchestration, and the judge's HTTP contract are unit-tested against scripted/mock responses (`internal/eval/*_test.go`), the suite has not yet been run end-to-end against a real model. Run `heimdall eval` (and `heimdall eval --judge`) once real credentials are available, and treat the first real run's pass/fail as the actual quality baseline -- not this document's design intent.
