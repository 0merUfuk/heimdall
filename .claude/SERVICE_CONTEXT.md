**Version**: 4.0
**Created**: 2026-03-28
**Last Updated**: 2026-09-13
**Authors:** Omer Ufuk

---

# Heimdall -- Service Context

## Current State (verified against code, not assumed from prior docs)

**On `main` right now**: feature-complete through the v0.1.0 scope (Deepgram + Soniox transcription, Claude API analysis, Obsidian output, crash recovery, consent, profiles), but **CI has been red since 2026-07-19** (3 govulncheck-flagged Go stdlib CVEs against the pinned go1.25.10 toolchain) and **no v0.1.0 tag has ever been cut**. The repo went dormant between 2026-07-19 and 2026-09-13 with only docs-only commits (productization strategy v1-v3, agent-runtime scaffolding) landing directly on `main`.

**In open PRs, pending merge** (built 2026-09-13, all green on `go test ./... -race -count=1`, blocked on owner merge action -- see below): four stacked PRs that fix the CI break and add substantial new capability. See "Open PRs" below for exact contents.

**Merge status**: this session could open PRs, push branches, and run every verification step, but **`gh pr merge` is blocked by the harness's own auto-mode permission classifier** -- an owner action (click merge on GitHub, or approve the CLI merge) is required to get any of this onto `main`. This is not a code-readiness gap; it is a deliberate tooling boundary this session did not attempt to route around.

- **Execution plan**: `docs/MASTER_PLAN.md` -- original 19-subtask plan, 18/19 complete (1E.2 final-release-tag still open)
- **Strategy**: `docs/STRATEGY_V2.md` (engineering roadmap) + `docs/PRODUCTIZATION.md` v3 (open-core + $49 one-time Pro, monetization/distribution research-backed)
- **Evaluation**: `docs/EVALUATION.md` (new, in PR #31) -- methodology for measuring Analyze-stage output quality
- **Build**: `make build` produces `bin/heimdall` (Go) + `bin/heimdall-audio` (Swift)
- **Tests on `main`**: currently pass locally (`go test ./... -race -count=1`, 11 packages) but CI is red on `govulncheck` -- see Known Issues
- **Tests across all 4 open PRs merged**: 14 packages, all green

---

## Open PRs (2026-09-13 session, chronological/stacked order)

Each PR is stacked on the previous (linear history), so they must merge in this order: #29 -> #30 -> #31 -> #32.

| PR | Branch | Contents |
|----|--------|----------|
| [#29](https://github.com/0merUfuk/heimdall/pull/29) | `fix/ci-toolchain-cve-bump` | Fixes the 2-month CI break: go1.25.10 -> 1.27.1 (go.mod + ci.yml), plus a README/CHANGELOG cleanup. Root cause: 3 stdlib CVEs disclosed against 1.25.10 after it was pinned; 1.25 is now outside Go's two-latest-majors security-backport window. |
| [#30](https://github.com/0merUfuk/heimdall/pull/30) | `feat/claude-code-analyzer` | New `ClaudeCodeAnalyzer` (`internal/analyzer/claudecode.go`): `--analyzer claude-code` shells out to a local, already-logged-in `claude` CLI instead of the Anthropic API -- no separate `ANTHROPIC_API_KEY` needed for users with an existing Claude subscription. Retry/fallback logic (V-009) extracted into a shared `summarizeWithRetry` so both backends degrade identically. Also fixed a real bug: `config.Validate()`'s Claude-model allowlist rejected the current real model (`claude-sonnet-5`) while accepting a fictitious one -- replaced with a shape check. |
| [#31](https://github.com/0merUfuk/heimdall/pull/31) | `feat/eval-system` | New `internal/eval` package + `heimdall eval` command: 7 golden transcripts, 10 deterministic checks (coverage, anti-hallucination, prompt-injection resistance, multilingual consistency), optional `--judge` LLM-as-judge scoring. First real measurement of Analyze-stage output quality this project has ever had -- previously only httptest-mocked plumbing tests existed. See `docs/EVALUATION.md`. |
| [#32](https://github.com/0merUfuk/heimdall/pull/32) | `feat/save-audio-recording` | Two coupled features: (1) `--save-audio` / `audio.save_recording` actually works now -- this config field existed since the MVP spec but was never wired to anything; new `internal/recording` WAV writer + `MeetingSession.OnAudioFrame` hook. (2) `heimdall transcribe` -- fully offline local transcription via a new `internal/localstt` package wrapping `whisper-cli` (`brew install whisper-cpp`) in batch mode, plus `heimdall model download <size>`. whisper.cpp's actual `--diarize` flag and `--output-json` schema were verified empirically (real binary, synthetic audio, real end-to-end smoke test) before writing the parser -- see `.claude/DECISIONS.md` ID-008. Also fixed a second real bug found in passing: `internal/recovery`'s filename sanitizer still stripped Turkish characters even though `internal/output`'s was fixed for this in the 31-bug sweep; consolidated into one shared `heimdall.SanitizeFilename`. |

New architectural decisions from this work: `.claude/DECISIONS.md` ID-005 through ID-008.

---

## Implemented Packages

| Package | Purpose | Status |
|---------|---------|--------|
| `internal/heimdall` | Shared types (AudioFrame, Segment, MeetingNote) + `SanitizeFilename` (shared, PR #32) | main + PR #32 |
| `internal/audio` | AudioSource interface + MicrophoneSource + SystemAudioSource | main |
| `internal/mixer` | Resample 48->16kHz, interleave stereo (L=system, R=mic) -- downmixed to mono by session.go before Deepgram (ID-001) | main |
| `internal/transcriber` | Transcriber interface (streaming) + DeepgramTranscriber + SonioxTranscriber + `NewFromName` factory | main |
| `internal/localstt` | **New (PR #32)**. Batch (non-streaming) local transcription via whisper-cli subprocess. Deliberately NOT a `Transcriber` implementation -- see DECISIONS.md ID-007 for why. | PR #32 |
| `internal/analyzer` | Analyzer interface + ClaudeAnalyzer (API) + **ClaudeCodeAnalyzer (PR #30, subprocess)** + `NewFromName` factory + shared `summarizeWithRetry` | main + PR #30 |
| `internal/eval` | **New (PR #31)**. Golden-fixture quality suite for the Analyze stage. | PR #31 |
| `internal/output` | Writer interface + ObsidianWriter (Go templates); filename sanitizer now delegates to `heimdall.SanitizeFilename` (PR #32) | main + PR #32 |
| `internal/recording` | **New (PR #32)**. WAV writer for `--save-audio`. | PR #32 |
| `internal/config` | YAML config, env var resolution, validation (loosened Claude-model check, PR #30; `claude.analyzer` field, PR #30) | main + PR #30 |
| `internal/consent` | First-run recording-consent banner | main |
| `internal/recovery` | Crash recovery (atomic writes every 30s); filename sanitizer fixed for Turkish (PR #32) | main + PR #32 |
| `internal/session` | MeetingSession orchestrator; new `OnAudioFrame` hook (PR #32) | main + PR #32 |
| `cmd/heimdall` | CLI commands -- see below | main + PRs #30-32 |

---

## CLI Commands

| Command | Purpose | Availability |
|---------|---------|--------------|
| `heimdall record` | Full pipeline recording (`--profile`, `--transcriber deepgram\|soniox`, `--analyzer api\|claude-code` [PR #30], `--save-audio` [PR #32]) | main + PRs |
| `heimdall doctor` | Check prerequisites (macOS version, API keys, audio permissions, vault path, claude CLI [PR #30], whisper-cli [PR #32]) | main + PRs |
| `heimdall list` | List past meeting notes | main |
| `heimdall config init/get/set/show/edit/path/add-profile/profiles` | Configuration management | main |
| `heimdall recover` | Scan for orphaned recovery files and re-analyze (`--analyzer` in PR #30) | main + PR #30 |
| `heimdall analyze --file <path>` | Re-analyze a specific recovery transcript JSON file (`--analyzer` in PR #30) | main + PR #30 |
| `heimdall transcribe --file <wav>` | **New (PR #32)**. Local offline transcription via Whisper. | PR #32 |
| `heimdall model download <size>` | **New (PR #32)**. Fetch a ggml Whisper model. | PR #32 |
| `heimdall eval` | **New (PR #31)**. Meeting-analysis quality suite. | PR #31 |
| `heimdall version` | Print version info | main |
