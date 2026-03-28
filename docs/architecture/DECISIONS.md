# Heimdall — Architectural Decision Records

**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

## Decision Log

| ID | Decision | Status | Date |
|----|---------|--------|------|
| AD-001 | Go + Swift hybrid architecture | Accepted | 2026-03-28 |
| AD-002 | Deepgram as primary STT provider | Accepted | 2026-03-28 |
| AD-003 | Provider abstraction layer (3 interfaces) | Accepted | 2026-03-28 |
| AD-004 | Post-meeting Claude processing (not real-time) | Accepted | 2026-03-28 |
| AD-005 | File-based Obsidian integration (no plugin) | Accepted | 2026-03-28 |
| AD-006 | MIT License (tentative — revisit at distribution) | Proposed | 2026-03-28 |
| AD-007 | Dual-channel stereo audio (L=system, R=mic) | Accepted | 2026-03-28 |
| AD-008 | LLM-based contextual speaker identification for MVP | Accepted | 2026-03-28 |
| AD-009 | CLI-first, web dashboard deferred to post-v1.0 | Accepted | 2026-03-28 |
| AD-010 | macOS 14.2+ minimum (Core Audio Taps requirement) | Accepted | 2026-03-28 |

---

## AD-001: Go + Swift Hybrid Architecture

**Status**: Accepted
**Date**: 2026-03-28

**Context**: macOS system audio capture requires Apple's Core Audio Taps API (Swift/ObjC only). The CLI tool, API integrations, and Obsidian output are best served by Go.

**Decision**: Go orchestrator binary (`heimdall`) + thin Swift audio helper binary (`heimdall-audio`) communicating via stdout pipe.

**Alternatives considered**:
- Pure Go with CGo bridge to CoreAudio: Complex, fragile, Core Audio Taps API not well-documented for C
- Pure Swift: Loses Go distribution story (no GoReleaser, no Homebrew tap synergy, no the-matrix patterns)
- Rust: Would require learning new ecosystem, no synergy with the-matrix
- Node.js with native addon: Too heavy, not distributable as single binary

**Consequences**: Two binaries to distribute. CI must build Swift for macOS targets. Linux version does not need the Swift helper (PulseAudio/PipeWire capture directly in Go).

---

## AD-002: Deepgram as Primary STT Provider

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Need real-time streaming STT with speaker diarization, official Go SDK, and reasonable pricing.

**Decision**: Deepgram Nova-3 with diarization add-on ($0.58/hr all-in).

**Key factors**:
- Official Go SDK with WebSocket streaming support
- $200 free credit (~345 hours of meetings, ~7 months of heavy use)
- Same rate for streaming and batch (no premium for real-time)
- Sub-300ms latency

**Fallback**: AssemblyAI (185hr free/month, Go SDK, excellent DER ~10.1%).

---

## AD-003: Provider Abstraction Layer (3 Interfaces)

**Status**: Accepted
**Date**: 2026-03-28

**Context**: STT provider lock-in is a risk. Users may prefer different providers for cost, accuracy, or privacy.

**Decision**: Three Go interfaces defining the abstraction boundaries:

```go
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
    SampleRate() int
    Channels() int
}

type Transcriber interface {
    Connect(ctx context.Context, opts TranscribeOpts) error
    Send(frame AudioFrame) error
    Receive() <-chan Segment
    Close() error
}

type Analyzer interface {
    Summarize(ctx context.Context, segments []Segment, opts AnalyzeOpts) (*MeetingNote, error)
}
```

**Consequences**: Slightly more upfront work, but enables AssemblyAI, Gladia, Whisper, and alternative LLM backends later.

---

## AD-004: Post-Meeting Claude Processing

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Two approaches: (a) send transcript to Claude after meeting ends, or (b) stream to Claude during meeting for live summaries.

**Decision**: Post-meeting processing for v1.0. Full transcript sent as a single prompt after meeting ends.

**Rationale**:
- Simpler architecture (no concurrent LLM streaming alongside STT streaming)
- Better summary quality (Claude sees full context, not fragments)
- Lower cost (one API call vs many)
- Live transcription already provides real-time value; summary can wait 10-30 seconds

**Consequences**: 10-30 second processing delay after meeting ends. Acceptable.

---

## AD-005: File-Based Obsidian Integration

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Obsidian integration can be done via (a) writing markdown files to vault, (b) building an Obsidian plugin, or (c) using Obsidian CLI.

**Decision**: Direct file writes to vault directory. Obsidian's file watcher detects new files automatically.

**Rationale**:
- Zero dependencies, works with any Obsidian version
- No plugin maintenance burden
- Plugin can be added later without breaking file-based approach

---

## AD-006: MIT License (Tentative)

**Status**: Proposed
**Date**: 2026-03-28

**Context**: Need an open-source license. Will revisit at distribution time.

**Decision**: MIT License (tentative).

**Note**: Final license decision deferred to distribution phase. MIT is the working assumption.

---

## AD-007: Dual-Channel Stereo Audio

**Status**: Accepted
**Date**: 2026-03-28

**Context**: System audio and microphone are two separate audio sources. They can be mixed into mono, sent as stereo, or sent as separate streams.

**Decision**: Interleave into stereo (L=system audio, R=microphone) and send to Deepgram with `multichannel=true`.

**Rationale**:
- Deepgram processes each channel independently, improving diarization accuracy
- Pre-separates local user from remote participants
- Reduces echo/duplication when user uses speakers instead of headphones
- Single WebSocket connection (not 2x cost)

**Consequences**: Audio mixer must handle resampling, format conversion, and interleaving.

---

## AD-008: LLM-Based Contextual Speaker Identification

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Speaker diarization labels speakers as "Speaker 0", "Speaker 1". Mapping these to real names requires either (a) voice enrollment + neural network matching, or (b) LLM reading conversational cues.

**Decision**: Use Claude's contextual understanding for MVP. In most meetings, participants address each other by name. Claude can read the transcript and produce a speaker map.

**Accuracy**: ~80% of speakers identified from context alone.

**Future**: ECAPA-TDNN voice enrollment for v0.3+ (the remaining 20% where names aren't spoken).

**Rationale**:
- Zero additional ML infrastructure for MVP
- No voice enrollment ceremony for users
- Free (included in the summarization API call)
- Good enough for v1.0

---

## AD-009: CLI-First, Dashboard Deferred

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Product could be CLI-only or CLI + web dashboard.

**Decision**: CLI-first for v1.0. Web dashboard is a presentation layer on top of the same data — deferred to post-v1.0.

**Rationale**:
- CLI is the core product for the target audience
- Obsidian IS the GUI for viewing outputs
- Dashboard is a distribution/adoption concern, not a core concern
- Same underlying data/APIs power both interfaces

---

## AD-010: macOS 14.2+ Minimum

**Status**: Accepted
**Date**: 2026-03-28

**Context**: Core Audio Taps API requires macOS 14.2+ (December 2023). Older macOS versions would require virtual audio drivers (BlackHole/Soundflower) or kernel extensions.

**Decision**: Require macOS 14.2+ for system audio capture.

**Rationale**:
- Core Audio Taps is Apple's official, sanctioned API
- No kernel extensions or virtual audio drivers needed
- By 2026, the vast majority of macOS users are on 14.2+
- Clean, future-proof approach
