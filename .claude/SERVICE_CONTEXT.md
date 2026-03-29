**Version**: 2.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-29
**Authors:** Omer Ufuk

---

# Heimdall -- Service Context

## Current State

v1.0 implementation complete. All 19 MASTER_PLAN subtasks executed. Full 6-stage pipeline functional.

- **Status**: v1.0.0-dev -- feature complete, pre-release
- **Execution plan**: `docs/MASTER_PLAN.md` -- 19/19 subtasks executed
- **Build**: `make build` produces `bin/heimdall` (Go) + `bin/heimdall-audio` (Swift)
- **Tests**: 9 packages, all passing with `-race`

---

## Implemented Packages

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `internal/heimdall` | Shared types (AudioFrame, Segment, MeetingNote) | types.go |
| `internal/audio` | AudioSource interface + MicrophoneSource + SystemAudioSource | source.go, microphone.go, system.go |
| `internal/mixer` | Resample 48->16kHz, interleave stereo (L=system, R=mic) | mixer.go, resample.go, ring_buffer.go |
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
| `heimdall doctor` | Check prerequisites |
| `heimdall list` | List past meeting notes |
| `heimdall config init/get/set` | Configuration management |
| `heimdall recover` | Scan for orphaned recovery files |
| `heimdall analyze --file <path>` | Re-analyze a transcript |
| `heimdall version` | Print version info |

---

## Agent Ecosystem

| Agent | Role |
|-------|------|
| manager | Orchestrator -- spawns agents, manages handoffs |
| developer | Senior Go engineer -- implements code |
| tester | QA engineer -- writes tests |
| reviewer | Adversarial code reviewer -- read-only |
| strategist | Product strategy, research |
| security-reviewer | OWASP + ASI security audits |
| product-lead | CEO perspective -- product health, priorities |
| tech-lead | CTO perspective -- codebase health, architecture |
| growth-lead | CMO perspective -- adoption, community |
| architect | Ecosystem evolution -- agents, skills, rules |
