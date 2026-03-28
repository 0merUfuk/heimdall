**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

# Heimdall — Service Context

## Current State

Project initialized with complete architecture documentation and agent ecosystem. No implementation code exists yet.

- **Status**: Pre-implementation — architecture phase complete, ready for Phase 0 execution
- **Version**: 0.1.0-dev (pre-release)
- **Execution plan**: `docs/MASTER_PLAN.md` — 19 subtasks, 5 phases, 0/19 complete

---

## Architecture References (MANDATORY READING)

| Document | Path | What It Contains |
|----------|------|-----------------|
| Master Plan | `docs/MASTER_PLAN.md` | 19-subtask execution plan with dependencies, acceptance criteria, human checkpoints |
| Pipeline Design | `docs/architecture/PIPELINE.md` | 6-stage pipeline with ASCII diagrams, technology map, shutdown sequence |
| Decisions | `docs/architecture/DECISIONS.md` | AD-001 to AD-010 — all architectural decisions in ADR format |
| MVP Spec | `docs/architecture/MVP.md` | Commands, config schema, Obsidian template, provider interfaces, cost analysis |
| Vulnerabilities | `docs/architecture/ASSESSMENT.md` | 28 findings (3 critical, 7 high, 10 medium, 8 low), 9 must-fix for v1.0 |
| Roadmap | `docs/architecture/ROADMAP.md` | Spike → v1.0 → v4.0 phased timeline |
| Strategy | `docs/architecture/STRATEGY.md` | Market analysis, competitor landscape, API pricing, naming decision |
| Project Identity | `docs/architecture/PROJECT.md` | Target audience, positioning, development approach |

---

## Pipeline Stages

```
1. CAPTURE    → Core Audio Taps (Swift) + malgo mic (Go)        [No LLM]
2. MIX        → Resample, convert, interleave stereo             [No LLM]
3. TRANSCRIBE → Deepgram Nova-3 WebSocket + diarization          [No LLM]
4. ACCUMULATE → In-memory segments + terminal display             [No LLM]
5. ANALYZE    → Claude API (post-meeting) — the ONLY LLM stage  [LLM]
6. RENDER     → Go templates → Obsidian vault markdown           [No LLM]
```

**Critical rule**: No LLM in stages 1-4. No audio processing in stages 5-6.

---

## Provider Interfaces (Not Yet Implemented)

| Interface | Package | Purpose |
|-----------|---------|---------|
| `AudioSource` | `internal/audio/` | Platform-swappable audio capture (mic via malgo, system via Swift subprocess) |
| `Transcriber` | `internal/transcriber/` | STT provider abstraction (Deepgram Nova-3 WebSocket) |
| `Analyzer` | `internal/analyzer/` | LLM abstraction (Claude post-meeting analysis) |
| `OutputWriter` | `internal/output/` | Vault writer (Obsidian markdown rendering) |

---

## Dual-Binary Architecture (AD-001)

| Binary | Language | Role |
|--------|----------|------|
| `heimdall` | Go | CLI orchestrator — all 6 pipeline stages, config, commands |
| `heimdall-audio` | Swift | Thin audio capture — Core Audio Taps API, PCM to stdout |

Go binary spawns Swift binary as subprocess. Communication via stdin/stdout pipes. Crash detection within 2 seconds (V-002).

---

## Key Constraints

| ID | Constraint | Severity |
|----|-----------|----------|
| V-001 | Deepgram WebSocket 60-min timeout — proactive reconnection at 55 min | Critical |
| V-002 | Swift subprocess crash — detect within 2s, auto-restart | Critical |
| V-003 | Screen Recording permission — check before recording | Critical |
| V-005 | Network disruption — ring buffer + reconnection | High |
| V-006 | SIGKILL crash recovery — temp file every 30s | High |
| V-009 | Claude API failure — fall back to raw transcript | High |

---

## What Exists on Disk

```
heimdall/
├── .claude/                   # Agent ecosystem (6 agents, 10 skills, 2 rules)
├── .gitignore
├── CLAUDE.md                  # Project entry point
├── Makefile                   # build, test, clean, doctor targets
├── go.mod                     # module github.com/0merUfuk/heimdall
├── docs/
│   ├── README.md              # Documentation index
│   ├── MASTER_PLAN.md         # 19-subtask execution plan
│   └── architecture/          # 7 architecture docs (2,900+ lines)
└── (no implementation code yet)
```
