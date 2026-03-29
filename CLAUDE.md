**Version**: 1.1
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

# Heimdall — Claude Code Instructions

Heimdall is a CLI meeting companion: captures audio, transcribes with speaker diarization, analyzes via Claude, and writes structured notes to an Obsidian vault. Go + Swift dual-binary architecture.

> **Architecture docs**: `docs/architecture/` contains all pipeline design, decisions, MVP spec, roadmap, and vulnerability assessment.

---

## Tool Inventory

| Binary | Language | Role |
|--------|----------|------|
| `heimdall` | Go | CLI orchestrator — config, record, analyze, output |
| `heimdall-audio` | Swift | Thin audio capture helper — Core Audio Taps, PCM to stdout |

---

## Before Working Here

1. Read `docs/MASTER_PLAN.md` — **19-subtask execution plan with dependencies and acceptance criteria**
2. Read `docs/architecture/PIPELINE.md` — 6-stage pipeline architecture
3. Read `docs/architecture/DECISIONS.md` — all architectural decisions (AD-001 to AD-010)
4. Read `docs/architecture/MVP.md` — v1.0 specification, commands, config, templates
5. Read `docs/architecture/ASSESSMENT.md` — 28 known vulnerabilities and mitigations
6. Read `docs/architecture/ROADMAP.md` — phased roadmap from spike to v4.0
7. Read `.claude/SERVICE_CONTEXT.md` — current implementation state
8. Read `.claude/NEXT_STEPS.md` — prioritized work items

---

## Directory Structure

```
heimdall/
├── cmd/heimdall/              # CLI commands (record, doctor, list, config, recover, version)
├── internal/
│   ├── heimdall/              # Shared types (AudioFrame, Segment, MeetingNote)
│   ├── audio/                 # AudioSource interface + MicrophoneSource + SystemAudioSource
│   ├── transcriber/           # Transcriber interface + DeepgramTranscriber (WebSocket)
│   ├── analyzer/              # Analyzer interface + ClaudeAnalyzer (Anthropic API)
│   ├── mixer/                 # Resample 48->16kHz, stereo interleave (L=system, R=mic)
│   ├── output/                # Writer interface + ObsidianWriter (Go templates)
│   ├── config/                # Config loading, validation, env var resolution
│   ├── recovery/              # Crash recovery (atomic temp files every 30s)
│   └── session/               # MeetingSession orchestrator (wires stages 1-4)
├── audio-helper/              # Swift audio capture binary (Core Audio Taps)
├── templates/                 # Go embed templates for Obsidian output
├── docs/
│   └── architecture/          # All design docs (7 files, 2900+ lines)
├── .claude/                   # Agent ecosystem (10 agents, 16 skills, 4 rules)
├── go.mod
├── Makefile
└── CLAUDE.md
```

---

## Key Commands

```bash
make build          # Build heimdall binary to bin/
make test           # Run all tests with race detection
make clean          # Remove binaries
make doctor         # Check prerequisites (Go, macOS version, Swift)
```

---

## Provider Interfaces (The 3 Boundaries)

```go
// internal/audio — swappable per platform
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
    SampleRate() int
    Channels() int
}

// internal/transcriber — swappable per STT provider
type Transcriber interface {
    Connect(ctx context.Context, opts TranscribeOpts) error
    Send(frame AudioFrame) error
    Receive() <-chan Segment
    Close() error
}

// internal/analyzer — swappable per LLM
type Analyzer interface {
    Summarize(ctx context.Context, segments []Segment, opts AnalyzeOpts) (*MeetingNote, error)
}
```

---

## Pipeline Stages (Quick Reference)

```
1. CAPTURE    → Core Audio Taps (Swift) + malgo mic (Go)        [No LLM]
2. MIX        → Resample, convert, interleave stereo             [No LLM]
3. TRANSCRIBE → Deepgram Nova-3 WebSocket + diarization          [No LLM]
4. ACCUMULATE → In-memory segments + terminal display             [No LLM]
5. ANALYZE    → Claude API (post-meeting) — the ONLY LLM stage  [LLM]
6. RENDER     → Go templates → Obsidian vault markdown           [No LLM]
```

---

## Critical v1.0 Constraints

- **V-001**: Deepgram WebSocket has 60-minute timeout — must implement proactive reconnection
- **V-002**: Swift subprocess crashes must be detected within 2 seconds and auto-restarted
- **V-003**: Screen Recording permission must be checked before recording starts
- **V-005**: Network disruption requires ring buffer + reconnection logic
- **V-006**: Crash recovery via temp file writes every 30 seconds
- **V-009**: Claude API failures must fall back to raw transcript output

> Full vulnerability list: `docs/architecture/ASSESSMENT.md`

---

## Core Principles

- **Pipeline clarity**: Each stage has one job. No LLM in the audio path. No audio processing in the output path.
- **Provider abstraction**: Every external dependency sits behind an interface.
- **Graceful degradation**: If Claude fails, write raw transcript. If Deepgram fails, save raw audio. Never lose the meeting.
- **Obsidian-native**: Output is vanilla markdown with YAML frontmatter. No plugin required.

---

## Agent Ecosystem

10 agents in `.claude/agents/` for coordinated autonomous development:

| Agent | Model | Role |
|-------|-------|------|
| `manager` | opus | Orchestrator — spawns agents, manages handoffs, creates PRs |
| `developer` | opus | Senior Go engineer — implements pipeline stages in isolated worktree |
| `tester` | sonnet | QA engineer — writes tests, validates implementations |
| `reviewer` | sonnet | Adversarial reviewer — read-only, 3-pass quality gate |
| `strategist` | opus | Product strategy, technology decisions, competitive research |
| `security-reviewer` | sonnet | OWASP + ASI security audits — read-only |
| `product-lead` | opus | CEO perspective — product health, competitive landscape, priorities |
| `tech-lead` | opus | CTO perspective — codebase health, architecture drift, dependencies |
| `growth-lead` | opus | CMO perspective — adoption channels, community, content strategy |
| `architect` | opus | Ecosystem evolution — creates/evolves agents, skills, rules |

**Full pipeline**: `claude --agent manager` → reads MASTER_PLAN.md → spawns developer → tester → reviewer → creates PR.

---

## Skills

| Skill | Purpose |
|-------|---------|
| `/audit` | Ecosystem integrity check |
| `/commit` | Conventional commit with scope |
| `/continue` | Resume from last session |
| `/doublecheck` | Multi-agent adversarial verification |
| `/fix` | Auto-fix from reviewer findings |
| `/issue` | Create GitHub issue from finding |
| `/dep-audit` | Dependency vulnerability check |
| `/owasp-review` | Security review (OWASP + ASI) |
| `/secret-scan` | Scan for leaked credentials |
| `/security-scan` | Full security analysis |
| `/strategy-weekly` | Weekly tactical brief (git activity, test health, priorities) |
| `/strategy-monthly` | Monthly deep review (product + tech + growth leads in parallel) |
| `/session-learn` | Capture session findings, evolve ecosystem |
| `/provision` | Create/update agents, skills, rules |
| `/release` | Full release workflow (GoReleaser, tag, publish) |
| `/pipeline-health` | Check all 6 pipeline stages health |

---

## Execution Plan

The master execution plan lives at `docs/MASTER_PLAN.md`. It contains 19 subtasks across 5 phases with a dependency graph. The manager agent reads this file at session start and continues from the first unchecked item.

**Phase summary:**
- **Phase 0**: Foundation + Spike (types, interfaces, mic capture, Swift helper, mixer, Deepgram, end-to-end)
- **Phase 1A**: Config system + doctor command
- **Phase 1B**: Claude analyzer + Obsidian renderer + crash recovery
- **Phase 1C**: Full record command (wire all 6 pipeline stages)
- **Phase 1D**: Integration tests + security review
- **Phase 1E**: Distribution + release (GoReleaser, Homebrew, README)
