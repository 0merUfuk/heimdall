**Version**: 5.1
**Created**: 2026-03-28
**Last Updated**: 2026-09-19
**Authors:** Omer Ufuk

---

# Heimdall -- Service Context

## Current State (verified against code, not assumed from prior docs)

**v0.1.0 is tagged and released.** [github.com/0merUfuk/heimdall/releases/tag/v0.1.0](https://github.com/0merUfuk/heimdall/releases/tag/v0.1.0) -- real GitHub Release, real GoReleaser-built `heimdall_0.1.0_darwin_{amd64,arm64}.tar.gz` archives + `checksums.txt`, verified by downloading the published artifact and running it (`heimdall version` -> `0.1.0`, `go1.27.1`).

**How this session got there**: 11 PRs (#29-#41) were built, tested, and merged into `main` on 2026-09-13/14 -- CI toolchain fix, a second `Analyzer` backend (`claude-code`, no separate API key needed), a real eval system, local Whisper transcription + raw-audio saving, an MCP server exposing the Obsidian vault, cost/latency observability, a lint/gofmt CI gate, a real GoReleaser bug fix (release builds had never actually succeeded before), tag-triggered release automation + Homebrew tap config, and a version bump. Full merge/conflict-resolution/verification detail: git log on `main` between `d8e06f5` and `v0.1.0` and each PR's own description.

- **Execution plan**: `docs/MASTER_PLAN.md` -- original 19-subtask plan; distribution (1E) is now genuinely done, not just planned
- **Strategy**: `docs/STRATEGY_V2.md` (engineering roadmap) + `docs/PRODUCTIZATION.md` v3 (open-core + $49 one-time Pro, monetization/distribution research-backed)
- **Evaluation**: `docs/EVALUATION.md` -- methodology for measuring Analyze-stage output quality. `heimdall eval` runs and correctly fails closed (flags fallback output rather than false-passing) when no live Claude credential is present -- not yet run against a real live model; that's the actual quality baseline once someone with a working credential runs it.
- **Release**: `docs/RELEASING.md` -- the process, plus the one-time owner setup still outstanding (see Known Issues)
- **Build**: `make build` produces `bin/heimdall` (Go) + `bin/heimdall-audio` (Swift); `make audio-helper-universal` produces the release's universal (arm64+x86_64) Swift binary
- **Tests on `main`**: all packages green (`go test ./... -race -count=1`), CI green

---

## In Progress: Local/offline analyzer + Codex + cloud (branch `claude/heimdall-offline-analyzer-502957`, 2026-09-19)

Not merged yet. Four analysis backends instead of two, a Codex developer setup, and cloud-container support. Decisions and measured evidence: `.claude/DECISIONS.md` ID-011 (Ollama), ID-012 (Codex), ID-013 (cloud).

- `--analyzer ollama` (`internal/analyzer/ollama.go`) -- on-device analysis via Ollama's native API, context sized per transcript, over-long transcripts refused not truncated. Default `qwen3:14b`. `heimdall transcribe` + `analyze --analyzer ollama` is offline end to end.
- `--analyzer codex` (`internal/analyzer/codex.go`) -- `codex exec`, isolated, default `gpt-5.6-luna` at low effort. Verified live: 6/7, 6/7, 5/7 on the eval, 21/21 valid JSON (`docs/EVALUATION.md`).
- `record --transcriber whisper` -- fully offline meeting capture: audio saved (checkpointed every 30 s), transcribed by whisper.cpp after the stop (ID-014).
- `--analyzer claude-code` fixed for subscription logins (`--bare` removed, isolation rebuilt and verified, ID-015); 7/7 on the eval through the real binary.
- Cloud baseline measured: Haiku 6/7 (api) vs `qwen3:14b` 5/7 -- `docs/EVALUATION.md`.
- Fallback no longer deletes the recovery transcript; `claude.analyzer` from config honored by every analysis command; `config set claude.analyzer` works; the prompt names the output language.
- `.codex/config.toml` + per-role Codex models; `AGENTS.md` rewritten (the previous one pointed at nonexistent `.Codex/` paths).
- `scripts/cloud-setup.sh` + `.claude/settings.json` SessionStart hook; full test suite verified passing on Linux (ubuntu:24.04, golang:1.24 images).

## What Shipped in This Round (PRs #29-#41)

| PR | Contents |
|----|----------|
| [#29](https://github.com/0merUfuk/heimdall/pull/29) | Go toolchain 1.25.10 -> 1.27.1, fixing a CI break that had been red since 2026-07-19 (3 stdlib CVEs). |
| [#30](https://github.com/0merUfuk/heimdall/pull/30) | `ClaudeCodeAnalyzer` -- `--analyzer claude-code` shells out to a local `claude` CLI login instead of the Anthropic API. Fixed a real bug in passing: `config.Validate()`'s Claude-model allowlist rejected the real current model while accepting a fictitious one. |
| [#31](https://github.com/0merUfuk/heimdall/pull/31) | `internal/eval` + `heimdall eval` -- 7 golden transcripts, 10 deterministic checks, optional `--judge` LLM-as-judge. First real Analyze-stage quality measurement this project has had. |
| [#32](https://github.com/0merUfuk/heimdall/pull/32) | `--save-audio` actually works now (documented since MVP, never wired); `heimdall transcribe` -- fully offline local transcription via `whisper-cli`, verified empirically against the real binary before the parser was written (`.claude/DECISIONS.md` ID-008). Also fixed a real Turkish-filename sanitizer bug in `internal/recovery`. |
| [#33](https://github.com/0merUfuk/heimdall/pull/33) | Docs sync (SERVICE_CONTEXT/NEXT_STEPS/KNOWN_ISSUES) -- since superseded by this update. |
| [#34](https://github.com/0merUfuk/heimdall/pull/34) | `go vet` + `gofmt` + `golangci-lint` wired into CI (existed as a Makefile target, never actually run). |
| [#35](https://github.com/0merUfuk/heimdall/pull/35) | Fixed GoReleaser's universal-binary path -- it was wrong; no release had ever actually succeeded via GoReleaser before this. Verified with real `goreleaser release --snapshot` runs. |
| [#36](https://github.com/0merUfuk/heimdall/pull/36) | `heimdall mcp` -- MCP server exposing the vault (`list_meetings`, `search_meetings`, `get_meeting`) via the official `github.com/modelcontextprotocol/go-sdk`. Two real bugs caught by a hand-built stdio JSON-RPC smoke test that unit tests couldn't have caught. |
| [#37](https://github.com/0merUfuk/heimdall/pull/37) | Analyzer cost/latency logging -- token usage and latency were parsed from every API response and silently discarded until now. |
| [#38](https://github.com/0merUfuk/heimdall/pull/38)/[#39](https://github.com/0merUfuk/heimdall/pull/39) | Tag-triggered `.github/workflows/release.yml` + `homebrew_casks` in `.goreleaser.yml`. (#38 accidentally merged into its own base branch instead of `main` -- #39 carries the identical, re-verified content correctly targeted at `main`.) |
| [#40](https://github.com/0merUfuk/heimdall/pull/40) | Fixed a real bug found by testing the release path before actually tagging: `skip_upload`'s template errored outright (not just evaluated false) when `HOMEBREW_TAP_TOKEN` was unset, which would have failed the entire release the first time anyone tagged before doing the tap's one-time setup. |
| [#41](https://github.com/0merUfuk/heimdall/pull/41) | Version bump to 0.1.0 (Makefile + CHANGELOG), immediately preceding the tag. |

New architectural decisions: `.claude/DECISIONS.md` ID-005 through ID-010 (renumbered during merge where two branches independently claimed the same ID -- see that file's own notes on ID-009/ID-010 for why).

---

## Implemented Packages

| Package | Purpose |
|---------|---------|
| `internal/heimdall` | Shared types (AudioFrame, Segment, MeetingNote) + `SanitizeFilename` |
| `internal/audio` | AudioSource interface + MicrophoneSource + SystemAudioSource |
| `internal/mixer` | Resample 48->16kHz, interleave stereo (L=system, R=mic) -- downmixed to mono by session.go before Deepgram (ID-001) |
| `internal/transcriber` | Transcriber interface (streaming) + DeepgramTranscriber + SonioxTranscriber + `NewFromName` factory |
| `internal/localstt` | Batch (non-streaming) local transcription via whisper-cli subprocess. Deliberately NOT a `Transcriber` implementation -- see DECISIONS.md ID-007. |
| `internal/analyzer` | Analyzer interface + ClaudeAnalyzer (API, with cost/latency logging) + ClaudeCodeAnalyzer (subprocess) + OllamaAnalyzer (native Ollama API, on-device) + CodexAnalyzer (`codex exec` subprocess) + `NewFromName(name, Settings)` factory + shared `summarizeWithRetry` and `analysisJSONSchema` |
| `internal/eval` | Golden-fixture quality suite for the Analyze stage |
| `internal/vault` | Reads meeting notes back out of the Obsidian vault (the inverse of `internal/output`) -- powers `heimdall mcp` |
| `internal/mcpserver` | MCP server over stdio exposing `internal/vault` as three tools |
| `internal/output` | Writer interface + ObsidianWriter (Go templates) |
| `internal/recording` | WAV writer for `--save-audio` |
| `internal/config` | YAML config, env var resolution, validation |
| `internal/consent` | First-run recording-consent banner |
| `internal/recovery` | Crash recovery (atomic writes every 30s) |
| `internal/session` | MeetingSession orchestrator, including the `OnAudioFrame` tap |
| `cmd/heimdall` | CLI commands -- see below |

---

## CLI Commands

| Command | Purpose |
|---------|---------|
| `heimdall record` | Full pipeline recording (`--profile`, `--transcriber deepgram\|soniox\|whisper`, `--analyzer api\|claude-code\|ollama\|codex`, `--save-audio`) |
| `heimdall doctor` | Check prerequisites (macOS version, API keys, audio permissions, vault path, claude/codex CLIs, Ollama + model, whisper-cli) |
| `heimdall list` | List past meeting notes |
| `heimdall config init/get/set/show/edit/path/add-profile/profiles` | Configuration management |
| `heimdall recover` | Scan for orphaned recovery files and re-analyze |
| `heimdall analyze --file <path>` | Re-analyze a specific recovery transcript JSON file |
| `heimdall transcribe --file <wav>` | Local offline transcription via Whisper |
| `heimdall model download <size>` | Fetch a ggml Whisper model |
| `heimdall eval` | Meeting-analysis quality suite (`--analyzer`, `--model`, `--judge`, `--json`) |
| `heimdall mcp` | MCP server over stdio, exposing the vault to any MCP client |
| `heimdall version` | Print version info |
