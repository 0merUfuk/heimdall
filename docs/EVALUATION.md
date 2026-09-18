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

## Measured results: local models (2026-09-19)

The first runs of this suite against a real model. Machine: Apple M4 Pro, 24 GB unified memory, Ollama 0.20.2, `--analyzer ollama`, temperature 0. Every configuration was run 3 times; at temperature 0 the three reports were byte-identical, so the pass rates below are deterministic, not averages.

### Golden fixtures (`heimdall eval --analyzer ollama`)

| Model | Fixtures passed | Valid JSON on first attempt | Warm latency per fixture | Failing fixtures |
|---|---|---|---|---|
| **`qwen3:14b`** (default) | **5/7** | 21/21 | 6-27 s | `tr-standup`: owners written as "Speaker N" instead of the required "Unknown Speaker N". `tr-en-code-switch` (`--language multi`): Turkish meeting summarized in English |
| `qwen2.5:7b` (rejected) | 0/7 | 21/21 | -- | "Speaker N" naming on every fixture, 0 of 5 decisions on `en-dense-coverage`, English summaries of Turkish meetings (measured before the language-name prompt fix) |

- Schema-constrained decoding (Ollama `format`) made JSON validity a non-issue: 42/42 valid on the first attempt across both models, no retries, no fallback notes.
- Naming the output language in the prompt ("Turkish (tr)" instead of "tr") moved `tr-standup`'s summary from English to Turkish on `qwen3:14b`. Two added instructions for `multi` changed nothing and were not kept.
- `qwen3:14b` passes everything in English: coverage (including 5 decisions + 5 action items on `en-dense-coverage`), both anti-hallucination fixtures, prompt-injection resistance, and meeting type.

### Long meetings (single pass through `heimdall analyze`)

Synthetic transcripts at realistic speech density (150 words/min in English), with action items and a decision planted at ~3%, 50%, and 97% of the meeting. Recall is scored only on the analysis part of the note, not the embedded raw transcript.

| Transcript | Input tokens | Context | Latency | Planted items recovered |
|---|---|---|---|---|
| English, 60 min (9,085 words) | 15,458 | 32K | 2 m 14 s | 3/4 -- **missed the mid-meeting action item** |
| Turkish, 60 min (`--language tr`) | 21,946 | 32K | 3 m 40 s | 4/4, summary in Turkish |
| English, 120 min | ~45,000 (estimated) | -- | refused, nothing sent | -- (exceeds the 32K default and the model's 40,960 maximum) |

- Action-item recall on 60-minute meetings: **5/6 (83%)**, decisions 2/2. The miss is the classic "lost in the middle" pattern of long-context models; it is the strongest argument for map-reduce if real meetings show the same.
- `qwen3:14b` with a 32K context occupies 13.8 GiB of unified memory while loaded.
- Two-hour meetings do not fit `qwen3:14b` at all. The analyzer refuses them up front with the exact token counts instead of silently truncating (ID-011); use a cloud backend for those until map-reduce exists.

### Offline end to end

`say` + `ffmpeg` produced a real two-speaker stereo WAV; `heimdall transcribe` (whisper.cpp, base model) then `heimdall analyze` (`claude.analyzer: ollama` from config) ran inside a macOS sandbox profile that denies every non-loopback network connection, with no API keys in the environment. Result: 7 correctly speaker-separated segments, the decision and the action item (owner + deadline) extracted correctly, note written to the vault, recovery transcript removed after success.

### Not yet measured

- **Cloud baseline** (`claude-haiku-4-5`): no credential was available during this run (no `ANTHROPIC_API_KEY`, `claude` CLI not logged in, and the `claude-code` backend's `--bare` auth issue in `.claude/KNOWN_ISSUES.md`). Run `heimdall eval --analyzer api` to fill in the comparison row.
- **`--analyzer codex`**: blocked by a Codex usage limit during the session.
- **Real meetings**: all numbers above are from synthetic transcripts.

## Known limitation

> Superseded in part: see "Measured results: local models" above for the first real runs. The cloud baseline is still unmeasured.

This was built in a sandbox with no logged-in `claude` CLI and, at authoring time, no `ANTHROPIC_API_KEY` configured -- so while every check function, the runner's orchestration, and the judge's HTTP contract are unit-tested against scripted/mock responses (`internal/eval/*_test.go`), the suite has not yet been run end-to-end against a real model. Run `heimdall eval` (and `heimdall eval --judge`) once real credentials are available, and treat the first real run's pass/fail as the actual quality baseline -- not this document's design intent.
