**Version**: 3.0
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

# Heimdall -- Service Context

## Current State

v1.0 feature-complete through PR #14 (profiles + config UX). Pipeline tested. 18 of 19 MASTER_PLAN subtasks complete — 1E.2 (final review + release) open.

- **Status**: v0.1.0 — 18/19 MASTER_PLAN subtasks complete, pending v0.1.0 release tag (see NEXT_STEPS)
- **Execution plan**: `docs/MASTER_PLAN.md` -- 18/19 subtasks complete (1E.2 — final review + release — pending)
- **Strategy**: `docs/STRATEGY_V2.md` -- post-grill execution plan
- **Build**: `make build` produces `bin/heimdall` (Go) + `bin/heimdall-audio` (Swift)
- **Tests**: 9 packages, all passing with `-race`

---

## Key PRs (merged to main)

| PR | Description |
|----|-------------|
| #14 | **feat**: meeting profiles + config UX — zero-flag daily workflow |
| #12 | **fix**: add user-facing guardrails for all failure paths |
| #11 | **fix**: switch from multichannel to mono+diarize for N-speaker meetings (see ID-001) |
| #10 | **fix**: add validation to config init and config set |
| #9 | **docs**: comprehensive documentation refresh — align all docs with code |
| #8 | **31-bug sweep** -- root cause fix (diarize+multichannel), error visibility, 25 more fixes |
| #7 | Final review -- data race fix, config-aware model, CHANGELOG |
| #6 | Security review -- file permissions, prompt sanitization, path traversal |
| #5 | Integration tests, LICENSE, distribution config |
| #1-4 | Phase 0-1 implementation (all pipeline stages) |

---

## Implemented Packages

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `internal/heimdall` | Shared types (AudioFrame, Segment, MeetingNote) | types.go |
| `internal/audio` | AudioSource interface + MicrophoneSource + SystemAudioSource | source.go, microphone.go, system.go |
| `internal/mixer` | Resample 48->16kHz, interleave stereo (L=system, R=mic) — downmixed to mono by session.go before Deepgram (see ID-001) | mixer.go, resample.go |
| `internal/transcriber` | Transcriber interface + DeepgramTranscriber (WebSocket) | transcriber.go, deepgram.go |
| `internal/analyzer` | Analyzer interface + ClaudeAnalyzer (Anthropic API) | analyzer.go, claude.go, prompts.go |
| `internal/output` | Writer interface + ObsidianWriter (Go templates) | writer.go, renderer.go |
| `internal/config` | YAML config, env var resolution, validation | config.go, defaults.go |
| `internal/recovery` | Crash recovery (atomic writes every 30s) | recovery.go |
| `internal/session` | MeetingSession orchestrator (wires stages 1-4) | session.go |
| `cmd/heimdall` | CLI commands: main.go, record.go, doctor.go, list.go, config.go, recover.go, version.go | 7 files |

---

## CLI Commands

| Command | Purpose |
|---------|---------|
| `heimdall record` | Full pipeline recording (now supports `--profile`) |
| `heimdall doctor` | Check prerequisites (validates macOS >= 14.2) |
| `heimdall list` | List past meeting notes |
| `heimdall config init` | Interactive first-run wizard |
| `heimdall config get <key>` | Read a dotted-path value (e.g. `obsidian.vault_path`) |
| `heimdall config set <key> <value>` | Write a value |
| `heimdall config show` | Print full config |
| `heimdall config edit` | Open config in $EDITOR |
| `heimdall config path` | Print config file path |
| `heimdall config add-profile <name>` | Define a meeting profile (PR #14) |
| `heimdall config profiles` | List available profiles |
| `heimdall recover` | Scan for orphaned recovery files |
| `heimdall version` | Print version info |
