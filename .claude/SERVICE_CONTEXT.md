**Version**: 3.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-31
**Authors:** Omer Ufuk

---

# Heimdall -- Service Context

## Current State

v1.0 implementation complete. 31-bug sweep merged (PR #8). Pipeline tested and verified. Ready for real-voice smoke testing and v1.0.0 tag.

- **Status**: v1.0.0-rc -- all bugs fixed, pending smoke test + release tag
- **Execution plan**: `docs/MASTER_PLAN.md` -- 19/19 subtasks complete
- **Strategy**: `docs/STRATEGY_V2.md` -- post-grill execution plan
- **Build**: `make build` produces `bin/heimdall` (Go) + `bin/heimdall-audio` (Swift)
- **Tests**: 9 packages, all passing with `-race`

---

## Key PRs (merged to main)

| PR | Description |
|----|-------------|
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
| `internal/mixer` | Resample 48->16kHz, interleave stereo (L=system, R=mic) | mixer.go, resample.go |
| `internal/transcriber` | Transcriber interface + DeepgramTranscriber (WebSocket) | transcriber.go, deepgram.go |
| `internal/analyzer` | Analyzer interface + ClaudeAnalyzer (Anthropic API) | analyzer.go, claude.go, prompts.go |
| `internal/output` | Writer interface + ObsidianWriter (Go templates) | writer.go, renderer.go |
| `internal/config` | YAML config, env var resolution, validation | config.go, defaults.go |
| `internal/recovery` | Crash recovery (atomic writes every 30s) | recovery.go |
| `internal/session` | MeetingSession orchestrator (wires stages 1-4) | session.go |
| `cmd/heimdall` | CLI commands (record, doctor, list, config, recover, version) | main.go, record.go, etc. |

---

## CLI Commands

| Command | Purpose |
|---------|---------|
| `heimdall record --title "..."` | Full pipeline recording |
| `heimdall doctor` | Check prerequisites (validates macOS >= 14.2) |
| `heimdall list` | List past meeting notes |
| `heimdall config init/get/set` | Configuration management |
| `heimdall recover` | Scan for orphaned recovery files |
| `heimdall analyze --file <path>` | Re-analyze a transcript |
| `heimdall version` | Print version info |
