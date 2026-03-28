**Version**: 1.0
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

1. Read `docs/architecture/PIPELINE.md` — 6-stage pipeline architecture
2. Read `docs/architecture/DECISIONS.md` — all architectural decisions (AD-001 to AD-010)
3. Read `docs/architecture/MVP.md` — v1.0 specification, commands, config, templates
4. Read `docs/architecture/ASSESSMENT.md` — 28 known vulnerabilities and mitigations
5. Read `docs/architecture/ROADMAP.md` — phased roadmap from spike to v4.0

---

## Directory Structure

```
heimdall/
├── cmd/heimdall/              # CLI entry point (cobra)
├── internal/
│   ├── audio/                 # AudioSource interface + implementations
│   ├── transcriber/           # Transcriber interface + Deepgram implementation
│   ├── analyzer/              # Analyzer interface + Claude implementation
│   ├── mixer/                 # Audio mixing, resampling, interleaving
│   ├── output/                # Obsidian template rendering + file writing
│   ├── config/                # Config loading, validation, wizard
│   └── recovery/              # Crash recovery (temp files, re-analysis)
├── audio-helper/              # Swift audio capture binary (future)
├── templates/                 # Go embed templates for Obsidian output
├── docs/
│   └── architecture/          # All design docs (7 files, 2900+ lines)
├── .claude/                   # Agent ecosystem
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
