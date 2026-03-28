# Heimdall — Pipeline Architecture

**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

## Overview

Heimdall is a CLI-first meeting companion that captures audio, transcribes with speaker diarization, analyzes via LLM, and writes structured notes to an Obsidian vault. Named after the Norse god who could hear grass growing.

The pipeline has 6 stages. The LLM appears in exactly ONE stage (Stage 5), after the meeting ends. Everything before that is system APIs and specialized speech ML.

---

## Full Pipeline Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    STAGE 1: CAPTURE                         │
│                    (System-level APIs)                       │
│                                                             │
│  ┌──────────────────┐       ┌──────────────────┐           │
│  │ Swift Helper      │       │ Go (malgo)        │           │
│  │ Core Audio Taps   │       │ Microphone        │           │
│  │ System audio      │       │ Your voice        │           │
│  │ (what Zoom plays) │       │                   │           │
│  └────────┬─────────┘       └─────────┬─────────┘           │
│           │ PCM 48kHz/32-bit          │ PCM 16kHz/16-bit    │
│           │ float, stereo             │ mono                │
│           └───────────┬───────────────┘                     │
│                       ▼                                     │
│  No LLM. No ML. Pure system audio APIs.                     │
└───────────────────────┬─────────────────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────────────────┐
│                    STAGE 2: MIX                              │
│                    (Audio Processing — Go)                   │
│                                                             │
│  ┌──────────────────────────────────────┐                   │
│  │ Resample system audio 48kHz → 16kHz  │                   │
│  │ Convert 32-bit float → 16-bit int    │                   │
│  │ Interleave: L=system, R=mic          │                   │
│  │ Ring buffer (30-60s backpressure)     │                   │
│  └──────────────────┬───────────────────┘                   │
│                     │ Stereo PCM 16kHz/16-bit               │
│                                                             │
│  No LLM. No ML. Pure DSP math.                              │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    STAGE 3: TRANSCRIBE                       │
│                    (Specialized Speech ML — Deepgram Cloud)  │
│                                                             │
│  ┌──────────────────────────────────────┐                   │
│  │ WebSocket stream to Deepgram         │                   │
│  │                                      │                   │
│  │ Nova-3 model does TWO things:        │                   │
│  │  1. Speech-to-Text (what was said)   │                   │
│  │  2. Diarization (who said it)        │                   │
│  │                                      │                   │
│  │ Returns per-utterance:               │                   │
│  │  { speaker: 0, text: "...",          │                   │
│  │    start: 12.4s, end: 15.1s,         │                   │
│  │    confidence: 0.97 }                │                   │
│  └──────────────────┬───────────────────┘                   │
│                     │                                       │
│  No LLM. Deepgram is a purpose-built speech model.          │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    STAGE 4: ACCUMULATE                       │
│                    (Local State — Go)                        │
│                                                             │
│  ┌──────────────────────────────────────┐                   │
│  │ Segments arrive in real-time         │                   │
│  │                                      │                   │
│  │ In-memory: []Segment (growing list)  │                   │
│  │ Terminal:  live transcript display    │                   │
│  │ Disk:     temp .json (crash recovery)│                   │
│  │ Optional: raw .wav recording         │                   │
│  └──────────────────┬───────────────────┘                   │
│                     │                                       │
│  No LLM. Collecting and displaying data as it arrives.      │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │  ← USER HITS Ctrl+C (meeting ends)
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    STAGE 5: ANALYZE                          │
│               *** THIS IS WHERE THE LLM LIVES ***           │
│                    (Claude API — post-meeting only)          │
│                                                             │
│  Claude does THREE things:                                  │
│                                                             │
│  1. SPEAKER IDENTIFICATION (contextual)                     │
│     Reads conversational cues ("Hey Sarah", "Thanks Omer")  │
│     Maps Speaker_N → real names from transcript content     │
│                                                             │
│  2. MEETING INTELLIGENCE                                    │
│     Summary, decisions, action items, follow-ups, topics    │
│                                                             │
│  3. STRUCTURED OUTPUT                                       │
│     Returns JSON: { speaker_map, summary, decisions,        │
│                     action_items, topics, follow_ups }      │
│                                                             │
│  Runs ONCE, after meeting ends, for 10-30 seconds.          │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    STAGE 6: RENDER + WRITE                   │
│                    (Template Engine + File I/O — Go)         │
│                                                             │
│  Go text/template renders Obsidian-native markdown:         │
│  - YAML frontmatter (date, participants, tags)              │
│  - Wikilinks for people ([[Omer]])                          │
│  - Action items table                                       │
│  - Collapsible raw transcript                               │
│                                                             │
│  Writes to: ~/vault/meetings/YYYY-MM-DD/{title}.md          │
│                                                             │
│  No LLM. Pure template rendering + file write.              │
└─────────────────────────────────────────────────────────────┘
```

---

## Technology Map

| Stage | Technology | LLM? | ML? | Network? |
|-------|-----------|------|-----|----------|
| 1. Capture | Core Audio Taps (Swift) + malgo (Go) | No | No | No |
| 2. Mix | Go audio processing (resample, interleave) | No | No | No |
| 3. Transcribe | Deepgram Nova-3 via WebSocket | No | Yes (speech) | Yes |
| 4. Accumulate | Go in-memory state + terminal display | No | No | No |
| **5. Analyze** | **Claude API (Haiku 4.5 default)** | **Yes** | No | Yes |
| 6. Render | Go text/template + os.WriteFile | No | No | No |

---

## Dual-Channel Audio Strategy

System audio and microphone are sent as a **stereo stream** (L=system, R=mic) to Deepgram with `multichannel=true`. This gives the STT engine a structural hint: left channel = remote participants, right channel = local user.

Benefits:
- Better diarization accuracy (channels are pre-separated)
- Reduces echo/duplication when user uses speakers instead of headphones
- Deepgram processes each channel independently and merges results

---

## Dual-Binary Architecture

```
heimdall         (Go binary)    — CLI, config, API clients, templates, output
heimdall-audio   (Swift binary) — Thin audio capture, PCM to stdout
```

Communication: Go spawns Swift binary as subprocess, reads PCM from stdout pipe. On Linux, the Swift binary is not needed (PulseAudio/PipeWire capture is done directly in Go).

---

## LLM-Based Speaker Identification (MVP Approach)

Instead of voice enrollment + ECAPA-TDNN neural network, the MVP uses Claude's contextual understanding to identify speakers from transcript content:

```
Input:  [Speaker 0]: "Sarah, can you share the update?"
        [Speaker 1]: "Sure. The auth migration is done."
        [Speaker 2]: "Omer, what's the timeline?"
        [Speaker 0]: "Next Thursday."

Output: { "0": "Omer", "1": "Sarah", "2": "Unknown" }
```

This achieves ~80% speaker identification accuracy at zero additional cost. True voice recognition (ECAPA-TDNN + enrollment) is planned for v0.3.

---

## Shutdown Sequence

```
User hits Ctrl+C
  ├── 1. Signal Swift subprocess to stop → flush remaining audio
  ├── 2. Send final audio bytes to Deepgram → wait for is_final
  ├── 3. Close WebSocket cleanly
  ├── 4. Send complete transcript to Claude → wait for analysis
  ├── 5. Render template → write to Obsidian vault
  └── 6. Print summary → exit

Expected duration: 10-30 seconds
Progress shown in terminal at each step
```

---

## System Permissions Required (macOS)

| Permission | Level | Grant Method | Purpose |
|-----------|-------|-------------|---------|
| Microphone | User (TCC) | Auto-prompted by OS | Capture user voice |
| Screen Recording | User (TCC) | Manual in System Settings | Core Audio Taps (audio only) |
| Network | User | Automatic | Deepgram + Claude API |
| File System | User | Automatic | Vault write + config |

No kernel extensions. No root/sudo. No virtual audio drivers.
