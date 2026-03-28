**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

# Heimdall — Next Steps

> Full execution plan with dependencies and acceptance criteria: `docs/MASTER_PLAN.md`

---

## Phase 0: Foundation + Spike

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 0.1 | Provision .claude/ ecosystem | — | COMPLETE |
| 0.2 | Oracle knowledge synthesis | 0.1 | COMPLETE |
| 0.3 | Core type definitions (AudioFrame, Segment, MeetingNote, interfaces) | 0.1 | NOT STARTED |
| 0.4 | Microphone capture (malgo AudioSource implementation) | 0.3 | NOT STARTED |
| 0.5 | Swift audio helper (Core Audio Taps → PCM stdout) | 0.3 | NOT STARTED |
| 0.6 | System audio source (Go subprocess wrapper for Swift helper) | 0.5 | NOT STARTED |
| 0.7 | Audio mixer (resample 48→16kHz, interleave L=system R=mic) | 0.4, 0.6 | NOT STARTED |
| 0.8 | Deepgram streaming transcriber (WebSocket + V-001 reconnection) | 0.3 | NOT STARTED |
| 0.9 | End-to-end spike (cmd/heimdall/main.go + record command, stages 1-4) | 0.4-0.8 | NOT STARTED |

> **HC-4**: After 0.9, user validates transcript quality on a real meeting. Kill criteria: <80% accuracy.

---

## Phase 1A: Config + Doctor

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 1A.1 | Config system (config.yaml, wizard, env var API keys) | Phase 0 | NOT STARTED |
| 1A.2 | Doctor command (V-003, V-019, V-020 permission checks) | 1A.1 | NOT STARTED |

---

## Phase 1B: Claude Analysis + Obsidian Output

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 1B.1 | Claude analyzer (structured output, V-009 fallback, V-013/V-014 prompt safety) | 0.3 | NOT STARTED |
| 1B.2 | Obsidian renderer (Go templates, YAML frontmatter, V-017 collision prevention) | 0.3 | NOT STARTED |
| 1B.3 | Crash recovery (temp file every 30s, `heimdall recover` command, V-006) | 0.3 | NOT STARTED |

---

## Phase 1C: Full Record Command

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 1C.1 | Meeting session orchestrator — wire all 6 stages, terminal display, shutdown | 0.9, 1A.1, 1B.1-1B.3 | NOT STARTED |
| 1C.2 | List + version commands | 1B.2 | NOT STARTED |

---

## Phase 1D: Testing + Security

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 1D.1 | Integration tests (session, reconnection, crash recovery, race detection) | 1C.1 | NOT STARTED |
| 1D.2 | Security review (API keys, subprocess injection, file paths, TLS, permissions) | 1D.1 | NOT STARTED |

---

## Phase 1E: Distribution + Release

| ID | Task | Depends On | Status |
|----|------|-----------|--------|
| 1E.1 | Distribution (GoReleaser, Homebrew tap, LICENSE, README, CHANGELOG) | 1D.2 | NOT STARTED |
| 1E.2 | Final review + v1.0.0 release | 1E.1 | NOT STARTED |

---

## Parallelization Opportunities

Within a session, these can run simultaneously after their dependencies are met:

- **After 0.3**: 0.4 (mic) + 0.5 (swift) + 0.8 (deepgram) in parallel
- **After Phase 0**: 1A.1 (config) + 1B.1 (claude) + 1B.2 (output) + 1B.3 (recovery) in parallel
- **Within each task**: developer → tester → reviewer is always sequential
