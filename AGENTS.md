**Version**: 1.3
**Created**: 2026-03-28
**Last Updated**: 2026-09-19
**Authors:** Omer Ufuk

---

# Heimdall — Codex Instructions

Heimdall is a CLI meeting companion: captures audio, transcribes with speaker diarization, analyzes via Claude, and writes structured notes to an Obsidian vault. Go + Swift dual-binary architecture.

> **Architecture docs**: `docs/architecture/` contains all pipeline design, decisions, MVP spec, roadmap, and vulnerability assessment.
>
> **This file is the Codex twin of `CLAUDE.md`.** Project state is shared, not duplicated: Codex reads the same `.claude/SERVICE_CONTEXT.md`, `.claude/NEXT_STEPS.md`, `.claude/KNOWN_ISSUES.md`, and `.claude/DECISIONS.md` as Claude Code. Only the agent definitions have a Codex form (`.codex/agents/*.toml`). When you change one of `CLAUDE.md`/`AGENTS.md`, update the other.

---

## Tool Inventory

| Binary | Language | Role |
|--------|----------|------|
| `heimdall` | Go | CLI orchestrator — config, record, analyze, output |
| `heimdall-audio` | Swift | Thin audio capture helper — Core Audio Taps, PCM to stdout |

---

## Before Working Here

1. Read `docs/MASTER_PLAN.md` — **19-subtask execution plan with dependencies and acceptance criteria**
2. Read `docs/STRATEGY_V2.md` — current strategy and roadmap pointer (supersedes `docs/architecture/STRATEGY.md`)
3. Read `docs/architecture/PIPELINE.md` — 6-stage pipeline architecture
4. Read `docs/architecture/DECISIONS.md` — all architectural decisions (AD-001 to AD-010)
5. Read `docs/architecture/MVP.md` — v1.0 specification, commands, config, templates
6. Read `docs/architecture/ASSESSMENT.md` — 28 known vulnerabilities and mitigations
7. Read `docs/architecture/ROADMAP.md` — phased roadmap from spike to v4.0
8. Read `.claude/SERVICE_CONTEXT.md` — current implementation state
9. Read `.claude/NEXT_STEPS.md` — prioritized work items
10. Read `.claude/rules/*.md` — **Codex does not auto-load these; Claude Code does.** They are binding here too: `pipeline-rules.md` (stage separation, channel convention, degradation chain), `audio-safety.md` (never block on audio channels, goroutine lifecycle, temp-file safety), `session-protocol.md`, `go-net-http-services.md`

---

## Directory Structure

```
heimdall/
├── cmd/heimdall/              # CLI commands (record, doctor, list, config, recover, version)
├── internal/
│   ├── heimdall/              # Shared types (AudioFrame, Segment, MeetingNote)
│   ├── audio/                 # AudioSource interface + MicrophoneSource + SystemAudioSource
│   ├── transcriber/           # Transcriber interface + DeepgramTranscriber (WebSocket)
│   ├── analyzer/              # Analyzer interface + 4 backends: ClaudeAnalyzer (API), ClaudeCodeAnalyzer, OllamaAnalyzer (on-device), CodexAnalyzer
│   ├── mixer/                 # Resample 48->16kHz, stereo interleave (L=system, R=mic) — downmixed to mono in session.go (ID-001)
│   ├── output/                # Writer interface + ObsidianWriter (Go templates)
│   ├── config/                # Config loading, validation, env var resolution
│   ├── recovery/              # Crash recovery (atomic temp files every 30s)
│   └── session/               # MeetingSession orchestrator (wires stages 1-4)
├── audio-helper/              # Swift audio capture binary (Core Audio Taps)
├── scripts/cloud-setup.sh     # Linux cloud-container setup (Codex cloud, Claude Code on the web)
├── templates/                 # Go embed templates for Obsidian output
├── docs/
│   └── architecture/          # All design docs (7 files, 2900+ lines)
├── .claude/                   # Shared project state + Claude agent ecosystem (10 agents, 18 skills, 4 rules)
├── .codex/                    # Codex: config.toml (project model defaults) + agents/*.toml (10 roles) + handoff skill
├── go.mod
├── Makefile
├── CLAUDE.md                  # Claude Code twin of this file
└── AGENTS.md
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

## Cloud Environments (Claude Code on the web, Codex cloud)

Linux containers: `scripts/cloud-setup.sh` installs the Go version go.mod requires, a C compiler (cgo is needed for malgo), and pre-builds everything. **Codex cloud**: in the environment's settings, set the **Setup script** to `scripts/cloud-setup.sh` (it pre-builds everything, so the agent phase works without internet). Claude Code on the web runs the same script automatically via the SessionStart hook in `.claude/settings.json` (a no-op locally). In a container you can build, vet, lint, and run `go test ./... -race` (the full suite passes on Linux); you cannot capture audio, build the Swift helper, run whisper.cpp, or reach a local Ollama. See `.claude/DECISIONS.md` ID-013.

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
3. TRANSCRIBE → Deepgram Nova-3 WebSocket (mono + diarize, ID-001) | Soniox | whisper: capture-only, whisper.cpp after the meeting (ID-014) [No LLM]
4. ACCUMULATE → In-memory segments + terminal display             [No LLM]
5. ANALYZE    → Claude API | claude CLI | Ollama (on-device) | codex CLI — post-meeting, the ONLY LLM stage [LLM]
6. RENDER     → Go templates → Obsidian vault markdown           [No LLM]
```

---

## Critical v1.0 Constraints

- **V-001**: Deepgram WebSocket has 60-minute timeout — must implement proactive reconnection
- **V-002**: Swift subprocess crashes must be detected within 2 seconds and auto-restarted
- **V-003**: Screen Recording permission must be checked before recording starts
- **V-005**: Network disruption requires ring buffer + reconnection logic
- **V-006**: Crash recovery via temp file writes every 30 seconds
- **V-009**: Analysis failures (any backend) must fall back to raw transcript output — and keep the recovery transcript for a retry (ID-011)

> Full vulnerability list: `docs/architecture/ASSESSMENT.md`

---

## Core Principles

- **Pipeline clarity**: Each stage has one job. No LLM in the audio path. No audio processing in the output path.
- **Provider abstraction**: Every external dependency sits behind an interface.
- **Graceful degradation**: If analysis fails, write raw transcript. If Deepgram fails, save raw audio. Never lose the meeting. Never fall back from on-device analysis to a cloud backend on the user's behalf (ID-011).
- **Obsidian-native**: Output is vanilla markdown with YAML frontmatter. No plugin required.

---

## Agent Ecosystem

10 roles in `.codex/agents/*.toml`, mirroring `.claude/agents/*.md` (the Claude definitions are the source; keep them in sync):

| Agent | Codex model / effort | Sandbox | Claude twin | Role |
|-------|----------------------|---------|-------------|------|
| `developer` | gpt-5.6-terra / medium | default | opus | Senior Go engineer — implements pipeline stages |
| `reviewer` | gpt-5.6-terra / medium | read-only | sonnet | Adversarial reviewer — 3-pass quality gate |
| `security-reviewer` | gpt-5.6-terra / medium | read-only | sonnet | OWASP + ASI security audits |
| `tech-lead` | gpt-5.6-terra / medium | read-only | opus | CTO perspective — codebase health, architecture drift |
| `strategist` | gpt-5.6-terra / medium | read-only | opus | Product strategy, technology decisions |
| `architect` | gpt-5.6-terra / medium | default | opus | Ecosystem evolution — `.claude/` + `.codex/` |
| `manager` | gpt-5.6-luna / medium | default | opus | Orchestrator — spawns agents, manages handoffs, creates PRs |
| `tester` | gpt-5.6-luna / medium | default | sonnet | QA engineer — writes tests, validates implementations |
| `product-lead` | gpt-5.6-luna / medium | default | opus | CEO perspective — product health, priorities |
| `growth-lead` | gpt-5.6-luna / medium | default | opus | CMO perspective — adoption channels, community |

### Codex model policy (cost)

Use the cheapest model tier that reliably handles the task — the same rule that makes `claude-haiku-4-5` the default analysis model.

- **Main session**: `.codex/config.toml` sets `gpt-5.6-terra` at `medium` for this repo only (verified: Codex loads project-scoped config). This overrides a global high/ultra default here; your global `~/.codex/config.toml` still applies everywhere else.
- **Subagents**: `gpt-5.6-terra` (balanced coding tier) for roles whose mistakes ship bugs or bad decisions; `gpt-5.6-luna` ("fast and affordable" tier) for bounded, procedural roles.
- **Escalate per task, never by default**: `codex -m gpt-6-astra` or `-c model_reasoning_effort="high"` for a genuinely hard problem, on the command line.
- **Product analysis** (`heimdall ... --analyzer codex`): `gpt-5.6-luna` at `low` effort, independent of this file (the analyzer ignores user/project Codex config on purpose).
- Model slugs come from Codex's model catalog as of 2026-09-19; update them here and in `.codex/` when Codex retires one. See `.claude/DECISIONS.md` ID-012.

**Full pipeline**: ask Codex to act as the `manager` agent → reads `docs/MASTER_PLAN.md` → spawns developer → tester → reviewer → creates PR.

---

## Skills

Codex-native: `.codex/skills/handoff/SKILL.md` (`/handoff` via `.codex/commands/handoff.md`) — package the session into a verified handoff + resume summary.

The other workflows are Claude Code skills in `.claude/skills/<name>/SKILL.md` (`audit`, `commit`, `continue`, `doublecheck`, `fix`, `issue`, `dep-audit`, `owasp-review`, `secret-scan`, `security-scan`, `sprint`, `strategy-weekly`, `strategy-monthly`, `session-learn`, `provision`, `release`, `pipeline-health`). They are plain markdown procedures: when a task matches one, read its `SKILL.md` and follow it.

---

## Execution Plan

The master execution plan lives at `docs/MASTER_PLAN.md`. It contains 19 subtasks across 6 phases (Phase 0 + 1A/1B/1C/1D/1E) with a dependency graph. The manager agent reads this file at session start and continues from the first unchecked item.

**Phase summary:**
- **Phase 0**: Foundation + Spike (types, interfaces, mic capture, Swift helper, mixer, Deepgram, end-to-end)
- **Phase 1A**: Config system + doctor command
- **Phase 1B**: Claude analyzer + Obsidian renderer + crash recovery
- **Phase 1C**: Full record command (wire all 6 pipeline stages)
- **Phase 1D**: Integration tests + security review
- **Phase 1E**: Distribution + release (GoReleaser, Homebrew, README)
