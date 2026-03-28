# AI Meeting Buddy -- Strategic Research Brief

**Date**: 2026-03-28
**Author**: Strategist Agent (research), Omer Ufuk (direction)
**Status**: Research Complete -- Ready for Decision

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Market Analysis](#2-market-analysis)
3. [Technical Architecture](#3-technical-architecture)
4. [Speaker Diarization Deep Dive](#4-speaker-diarization-deep-dive)
5. [Audio Capture Strategy](#5-audio-capture-strategy)
6. [Integration Design](#6-integration-design)
7. [MVP Definition](#7-mvp-definition)
8. [Phased Roadmap](#8-phased-roadmap)
9. [Cost Analysis](#9-cost-analysis)
10. [Risk Matrix](#10-risk-matrix)
11. [Distribution Plan](#11-distribution-plan)
12. [Naming Recommendations](#12-naming-recommendations)
13. [Technical Decisions](#13-technical-decisions)

---

## 1. Executive Summary

AI Meeting Buddy is a CLI-first, open-source meeting companion that captures audio from all meeting participants (system audio + microphone), performs speaker diarization to identify who is speaking, sends transcripts to Claude for intelligent summarization, and writes structured meeting notes directly into an Obsidian vault. It fills a gap no existing tool covers: the intersection of **CLI-native workflow**, **Obsidian-first output**, **Claude-powered analysis**, and **open-source transparency**. The commercial meeting assistant market is a $1.5B+ validated space (Granola alone is valued at $1.5B as of March 2026), but every existing tool is either a SaaS product with privacy concerns, a GUI-only application, or lacks deep integration with knowledge management tools like Obsidian. A Go CLI binary distributed via Homebrew, designed for developers and technical knowledge workers who already live in the terminal, has a clear positioning that no competitor occupies.

---

## 2. Market Analysis

### 2.1 Competitive Landscape

| Tool | Type | Pricing | Diarization | Privacy | CLI | Obsidian | Moat |
|------|------|---------|-------------|---------|-----|----------|------|
| **Otter.ai** | SaaS | Free (300 min/mo, 30-min cap), Pro $16.99/mo | Yes | Cloud (class action lawsuit over data training) | No | No | Real-time collab, search |
| **Fireflies.ai** | SaaS | Free (800 min storage, 20 credits/mo), Pro ~$10/mo | Yes | Cloud | No | No | CRM integrations |
| **Fathom** | SaaS + bot | Free (unlimited recording, 5 AI summaries/mo) | Yes | HIPAA/SOC2, but visible bot | No | No | Generous free tier |
| **Granola** | Desktop app | Free (25 meetings lifetime), $18/mo | Yes | Bot-free device capture | No | MCP integration | $1.5B valuation, device-native |
| **tl;dv** | SaaS + bot | Free tier, paid ~$18/mo | Yes | Cloud | No | No | Video clips |
| **Krisp** | Desktop | Free tier, Pro $8/mo | Limited | Local noise cancel | No | No | Noise cancellation |
| **Tactiq** | Chrome ext | Free tier, paid $8/mo | Basic | Browser only | No | No | Google Meet native |
| **Meetily** | Open source | Free | Yes (pyannote) | 100% local, Rust+Python | No | No | 10.7K stars, privacy |
| **MeetScribe** | Open source | Free | Yes (WhisperX) | Local | CLI | No | Simple, PDF output |

### 2.2 Market Gaps Identified

1. **No CLI-native tool exists.** Every competitor is a GUI desktop app, browser extension, or SaaS. Developers who live in the terminal (the Obsidian-in-terminal crowd) have zero options.

2. **No Obsidian-first integration.** Granola recently added MCP integration, but no tool writes native Obsidian markdown with proper frontmatter, wikilinks, tags, and vault-aware structure.

3. **No Claude-powered analysis.** Every tool uses GPT-4 or internal models. Claude's structured output, extended thinking, and 1M token context window (for processing very long meetings) are unused in this space.

4. **Open source + polished distribution is rare.** Meetily (10.7K stars) proves demand, but it requires Docker/Python setup. No Go binary with `brew install` simplicity.

5. **Speaker recognition (not just diarization) is absent from open-source.** Commercial tools do it; open-source tools label "Speaker 1, Speaker 2" but never "Omer, Sarah."

### 2.3 Positioning

**"The meeting companion for engineers who use Obsidian."**

Target: Developers, engineering managers, technical PMs, and knowledge workers who:
- Use Obsidian as their second brain
- Run Claude Code in the terminal
- Attend 5-15 meetings per week
- Care about data privacy and tooling control
- Want structured, searchable meeting records

This is NOT a replacement for Granola or Otter for non-technical users. It is the tool that the Obsidian + CLI + Claude audience didn't know they needed.

### 2.4 Market Validation Signals

- Granola: $192M total funding, $1.5B valuation (March 2026), 250% quarterly revenue growth
- Meetily: 10.7K GitHub stars in ~3 months (privacy-first open-source demand)
- MeetScribe: 284 stars in 2 weeks (fully local meeting transcription)
- WhisperX: 20.9K stars (speech processing + diarization demand)
- Obsidian CLI: Official launch, growing automation ecosystem
- The-matrix knowledge: oracle already synthesizes knowledge docs -- same pattern of "capture + structure + output"

---

## 3. Technical Architecture

### 3.1 Recommended Architecture: Hybrid (Cloud STT + Local Orchestration)

```
                         +-------------------+
                         |   Meeting Audio    |
                         | (Zoom/Meet/Teams)  |
                         +--------+----------+
                                  |
                    +-------------+-------------+
                    |                           |
          +---------v--------+       +----------v---------+
          | System Audio     |       | Microphone         |
          | (Core Audio Tap  |       | (PortAudio/malgo)  |
          | via Swift helper)|       |                    |
          +--------+---------+       +----------+---------+
                   |                            |
                   +------------+---------------+
                                |
                     +----------v-----------+
                     | Audio Mixer (Go)     |
                     | Merge to single      |
                     | PCM stream           |
                     +----------+-----------+
                                |
               +----------------+----------------+
               |                                 |
    +----------v-----------+          +----------v-----------+
    | Real-Time STT API    |          | Local WAV Recording  |
    | (Deepgram Streaming) |          | (Backup/offline)     |
    | + Speaker Diarization|          |                      |
    +----------+-----------+          +----------+-----------+
               |                                 |
    +----------v-----------+                     |
    | Diarized Transcript  |                     |
    | { speaker, text, ts }|                     |
    +----------+-----------+                     |
               |                                 |
    +----------v-----------+                     |
    | Speaker Recognition  |                     |
    | (Voice profile DB)   |<--------------------+
    | Map Speaker_1 -> Name|
    +----------+-----------+
               |
    +----------v-----------+
    | Claude API           |
    | - Summary            |
    | - Action items       |
    | - Key decisions      |
    | - Follow-ups         |
    +----------+-----------+
               |
    +----------v-----------+
    | Obsidian Writer      |
    | - Frontmatter        |
    | - Wikilinks          |
    | - Tags               |
    | - Template rendering |
    +----------+-----------+
               |
    +----------v-----------+
    | Obsidian Vault       |
    | meetings/2026-03-28/ |
    |  sprint-planning.md  |
    +----------------------+
```

### 3.2 Why Hybrid Over Full-Local or Full-Cloud

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| **Full Cloud** (all audio to cloud API) | Simplest, best accuracy, real-time | Privacy concerns, cost scales linearly, internet required | Good for MVP |
| **Hybrid** (local capture + cloud STT) | Best accuracy, privacy for audio storage, offline fallback | Still sends audio to cloud API | **Recommended** |
| **Full Local** (Whisper + pyannote locally) | Maximum privacy, zero API cost | Requires GPU, 5-10x slower than real-time on CPU, diarization accuracy drops ~20% vs cloud | Future option |

**Recommendation**: Start hybrid. Audio is captured and optionally stored locally (privacy). STT + diarization via cloud API (accuracy + speed). Claude processing is inherently cloud. Offer a `--local` flag for full offline mode in v0.3+.

### 3.3 Language Choice: Go Orchestrator + Swift Audio Helper

Go is the right choice for the orchestrator, CLI, Claude integration, Obsidian output, and distribution (single binary, Homebrew). But Go cannot access macOS Core Audio taps directly -- this requires a Swift helper binary.

**Architecture**:
- `meetbuddy` (Go binary): CLI, configuration, API clients, transcript processing, Claude integration, Obsidian output
- `meetbuddy-audio` (Swift binary): Thin audio capture layer using Core Audio taps API, outputs PCM to stdout
- Communication: Go spawns Swift binary as subprocess, reads PCM from stdout pipe
- Distribution: Both binaries in the Homebrew formula (or embed Swift binary as a resource)

This mirrors the audiotee project pattern (MIT-licensed Swift CLI that captures system audio to stdout) and could potentially vendor audiotee directly.

---

## 4. Speaker Diarization Deep Dive

### 4.1 Diarization vs Recognition

| Capability | What It Does | Accuracy (2026) | Example Output |
|------------|-------------|------------------|----------------|
| **Diarization** | Segments audio by speaker identity | DER 8-19% (85-95% accuracy) | "Speaker 1: Let's discuss the roadmap" |
| **Recognition** | Identifies known speakers by voice | EER 1.7-1.9% for enrolled speakers | "Omer: Let's discuss the roadmap" |
| **Enrollment** | Trains system on a specific voice | 10-30 seconds of speech needed | User provides voice sample |

To achieve the 95-99% accuracy target for distinguishing who is speaking:
- **Diarization alone**: 85-95% accuracy achievable with top-tier APIs (AssemblyAI, Deepgram, pyannoteAI Precision-2)
- **Diarization + Recognition**: 95-99% achievable by combining API diarization with local speaker embeddings for name mapping

### 4.2 API Comparison for Diarization

| Provider | Model | DER | Latency | Diarization Cost | Total Cost (STT + Diarization) | Go SDK | Notes |
|----------|-------|-----|---------|-----------------|-------------------------------|--------|-------|
| **Deepgram** | Nova-3 | Not published | Real-time (<300ms) | $0.002/min | $0.0097/min ($0.58/hr) | Official | $200 free credit, same rate for streaming |
| **AssemblyAI** | Universal-3 Pro | ~10% DER | Real-time (~300ms P50) | $0.02/hr | $0.23/hr (pre-recorded), $0.57/hr (streaming) | Go SDK | 185hr free (pre-recorded), 333hr free (streaming) |
| **pyannoteAI** | Precision-2 | ~8-10% DER | Batch | EUR 0.14/hr (diarization only) | EUR 0.25/hr (with STT orchestration) | No SDK (REST) | 150hr free trial, best DER |
| **Gladia** | Solaria-1 | ~12% DER | 103ms partials | Included | $0.55/hr (all features bundled) | No SDK (REST) | 10hr/mo free, all features included |
| **Speechmatics** | Flow | Not published | Real-time | Not published | Contact sales | No SDK (REST) | 480 free min/mo |
| **Rev.ai** | Standard | Not published | Real-time | ~$0.002-0.005/min | ~$0.005/min | No SDK (REST) | |

### 4.3 Recommendation: Deepgram for MVP

**Primary**: Deepgram Nova-3 with diarization add-on
- Official Go SDK with WebSocket streaming support
- $0.58/hour all-in (STT + diarization), cheapest for streaming
- $200 free credit (~345 hours of meetings)
- Same rate for real-time streaming and batch -- no premium for live
- Language-agnostic diarization trained on 100K+ speakers
- Sub-300ms latency

**Secondary/Fallback**: AssemblyAI
- Extensive free tier (185hr pre-recorded, 333hr streaming)
- Go SDK with WebSocket support
- Best-in-class DER (~10.1%) with recent improvements
- 30% better in noisy environments

**Future local option**: WhisperX (Whisper + pyannote 3.1)
- DER ~11-19% on standard benchmarks
- Fully offline, zero cost
- Requires Python runtime or Docker (not ideal for Go CLI)
- Could be offered as `meetbuddy --local` mode

### 4.4 Speaker Recognition (Voice Enrollment)

For mapping "Speaker 1" to "Omer", the approach is:

1. **Enrollment**: User provides a 10-30 second voice sample per person
   ```bash
   meetbuddy enroll --name "Omer" --audio ~/recordings/omer-sample.wav
   # Or: meetbuddy enroll --name "Omer" --record (records from mic)
   ```

2. **Embedding extraction**: Use ECAPA-TDNN or TitaNet (via ONNX Runtime in Go) to generate a 192-dim speaker embedding vector

3. **Matching**: During/after diarization, extract embeddings per speaker segment, compare cosine similarity against enrolled profiles

4. **Storage**: Speaker profiles stored locally in `~/.meetbuddy/voices/`

| Model | Architecture | EER | Embedding Dim | ONNX Available | Notes |
|-------|-------------|-----|---------------|----------------|-------|
| ECAPA-TDNN | TDNN + SE + Res2Net | 1.71% | 192 | Yes (SpeechBrain) | Most widely used |
| TitaNet-L | 1D DepthwiseConv + SE | 1.91% | 192 | Yes (NeMo) | NVIDIA, larger model |
| ReDimNet | CNN + Transformer | <1.5% | 192 | Limited | Newest, best robustness |

**Recommendation**: ECAPA-TDNN via ONNX Runtime. Mature, excellent accuracy, ONNX export available from SpeechBrain (HuggingFace), can run in Go via go-onnxruntime bindings. No Python dependency needed.

**MVP shortcut**: Skip voice recognition in v0.1. Let users manually label speakers after the meeting. Add voice enrollment in v0.2.

---

## 5. Audio Capture Strategy

### 5.1 macOS Audio Capture

macOS deliberately sandboxes audio between applications. There is no API to "hear what Zoom hears" without going through approved channels. Here are the options:

| Method | macOS Version | Requires | Permissions | Quality | Notes |
|--------|--------------|----------|-------------|---------|-------|
| **Core Audio Taps** | 14.2+ (Dec 2023) | Swift binary | Screen Recording permission | Excellent (native sample rate, 32-bit float) | Apple's official API, MIT-licensed audiotee exists |
| **ScreenCaptureKit** | 13+ | Swift/ObjC | Screen Recording permission | Good | Designed for screen recording, audio is secondary |
| **BlackHole** (virtual audio driver) | Any | User installs BlackHole | Audio settings change | Good | Requires manual audio routing setup |
| **Aggregate Device** | Any | Audio MIDI Setup | Manual configuration | Good | Complex setup, fragile |

**Recommendation**: Core Audio Taps via a Swift helper binary (or vendoring audiotee).

- Requires macOS 14.2+ (released Dec 2023) -- acceptable for a new tool in 2026
- No kernel extensions or virtual audio drivers needed
- MIT-licensed reference implementation exists (audiotee)
- One-time Screen Recording permission prompt
- Captures system audio from all processes (Zoom, Meet, Teams, browser) without joining the call

### 5.2 Microphone Capture

For the user's own voice (which system audio capture may miss or capture at lower quality):

| Library | Language | Backend | Status |
|---------|----------|---------|--------|
| **malgo** | Go | miniaudio (CoreAudio on macOS) | Active, well-maintained |
| **portaudio** | Go (CGo) | PortAudio | Stable, requires system lib |
| **microphone** (beep) | Go | PortAudio wrapper | Good for beep ecosystem |

**Recommendation**: malgo (github.com/gen2brain/malgo). Pure Go bindings to miniaudio, no external dependencies, cross-platform, handles microphone capture natively.

### 5.3 Dual Capture Architecture

```
+---------------------+     +----------------------+
| Swift Audio Helper  |     | Go Microphone        |
| (Core Audio Taps)   |     | (malgo/miniaudio)    |
| System audio stdout |     | User voice capture   |
+----------+----------+     +-----------+----------+
           |                            |
           v                            v
    +------+-------+            +-------+------+
    | PCM stream   |            | PCM stream   |
    | 32-bit float |            | 16-bit int   |
    | native rate  |            | 16kHz        |
    +--------------+            +--------------+
           |                            |
           +----------- + --------------+
                        |
              +---------v----------+
              | Audio Mixer (Go)   |
              | - Resample to 16kHz|
              | - Normalize formats|
              | - Dual-channel or  |
              |   mixed mono       |
              +---------+----------+
                        |
              +---------v----------+
              | Deepgram WebSocket |
              | Streaming STT      |
              | + Diarization      |
              +--------------------+
```

### 5.4 Cross-Platform Considerations

| Platform | System Audio | Mic | Complexity | MVP Priority |
|----------|-------------|-----|------------|-------------|
| **macOS** | Core Audio Taps (14.2+) | malgo | Medium | **Yes** |
| **Linux** | PulseAudio/PipeWire monitor | malgo | Low | Phase 2 |
| **Windows** | WASAPI loopback | malgo | Medium | Phase 3 |

macOS first. Linux is actually easier (PulseAudio exposes monitor devices natively). Windows requires WASAPI loopback capture. malgo handles mic capture on all three platforms.

---

## 6. Integration Design

### 6.1 Claude API Integration

**SDK**: Official anthropic-sdk-go (github.com/anthropics/anthropic-sdk-go)
- Actively maintained by Anthropic
- Supports streaming, structured output, tool use
- Full Go type safety

**Model Selection**:
- Default: Claude Haiku 4.5 ($1/$5 per M tokens) -- fast, cheap, good enough for meeting summaries
- Option: Claude Sonnet 4.6 ($3/$15 per M tokens) -- better for complex meetings with technical content
- User configurable via `--model` flag or config file

**Token Management for Long Meetings**:

| Meeting Duration | ~Word Count | ~Token Count | Model Context | Fits? |
|-----------------|-------------|-------------|---------------|-------|
| 30 min | 4,500 | ~6,000 | 200K (Haiku) | Yes |
| 1 hour | 9,000 | ~12,000 | 200K (Haiku) | Yes |
| 2 hours | 18,000 | ~24,000 | 200K (Haiku) | Yes |
| 4 hours | 36,000 | ~48,000 | 200K (Haiku) | Yes |
| All-day (8hr) | 72,000 | ~96,000 | 200K (Haiku) | Yes |

Even an 8-hour meeting transcript fits comfortably in Haiku's 200K context window. No chunking strategy needed for any realistic meeting length.

**Prompt Strategy**:

```
System: You are a meeting note analyst. You produce structured meeting notes
from diarized transcripts. Output must be valid markdown suitable for
Obsidian with YAML frontmatter.

User: Here is the diarized transcript of a meeting titled "{title}":

<transcript>
{diarized_transcript}
</transcript>

Produce meeting notes with these sections:
1. **Summary** (3-5 sentences)
2. **Key Decisions** (bullet list, each with who decided and context)
3. **Action Items** (table: task, owner, deadline if mentioned, priority)
4. **Discussion Topics** (organized by topic with key points)
5. **Follow-ups** (questions raised but not resolved)
6. **Participants** (list with role if identifiable)

Format rules:
- Use Obsidian wikilinks for people: [[{name}]]
- Tag action items with #action-item
- Tag decisions with #decision
- Include timestamps for key moments
```

**Structured Output**: Use Claude's JSON schema mode for programmatic extraction of action items, then render to markdown templates. This separates the intelligence (Claude) from the formatting (Go templates).

**Cost per meeting** (Haiku 4.5):
- 1-hour meeting: ~12K input tokens + ~2K output tokens = $0.022
- With prompt caching (subsequent meetings same day): ~$0.014

### 6.2 Obsidian Integration

**Approach**: File-based integration (write markdown directly to vault directory). No plugin needed for MVP.

**Meeting Note Template**:

```markdown
---
date: {{.Date}}
type: meeting
title: "{{.Title}}"
participants:
{{- range .Participants}}
  - "[[{{.Name}}]]"
{{- end}}
duration: {{.Duration}}
platform: {{.Platform}}
tags:
  - meeting
  - {{.MeetingType}}
---

# {{.Title}}

> Recorded on {{.Date}} | Duration: {{.Duration}} | Platform: {{.Platform}}

## Summary

{{.Summary}}

## Key Decisions

{{range .Decisions}}
- {{.Decision}} -- decided by [[{{.DecidedBy}}]] #decision
{{end}}

## Action Items

| Task | Owner | Deadline | Priority |
|------|-------|----------|----------|
{{range .ActionItems}}
| {{.Task}} | [[{{.Owner}}]] | {{.Deadline}} | {{.Priority}} |
{{end}}

## Discussion Topics

{{range .Topics}}
### {{.Title}}

{{.Content}}

{{end}}

## Follow-ups

{{range .Followups}}
- [ ] {{.Question}} (raised by [[{{.RaisedBy}}]])
{{end}}

## Raw Transcript

<details>
<summary>Click to expand full transcript</summary>

{{range .Segments}}
**{{.Speaker}}** ({{.Timestamp}}): {{.Text}}
{{end}}

</details>
```

**Vault Structure**:
```
vault/
├── meetings/
│   ├── 2026-03-28/
│   │   ├── sprint-planning.md
│   │   └── design-review.md
│   ├── 2026-03-27/
│   │   └── standup.md
│   └── templates/
│       └── meeting-note.md
├── people/
│   ├── Omer Ufuk.md          <-- wikilinked from meeting notes
│   └── Sarah Chen.md
└── .meetbuddy/
    └── config.yaml
```

**Advanced Obsidian Features (post-MVP)**:
- Obsidian CLI integration (`obsidian open` to jump to the new note)
- Obsidian URI scheme (`obsidian://open?vault=...&file=...`)
- Dataview-compatible frontmatter for querying meetings
- Automatic people page creation when a new participant is detected
- Daily note integration (append meeting summary to today's daily note)

---

## 7. MVP Definition

### 7.1 MVP Scope (v0.1.0)

**What it does**:
1. Captures system audio from any meeting app (macOS 14.2+ only)
2. Captures microphone audio simultaneously
3. Streams to Deepgram for real-time transcription + speaker diarization
4. After meeting ends, sends transcript to Claude for summarization
5. Writes structured meeting note to Obsidian vault directory
6. Interactive CLI with real-time transcript display

**What it does NOT do (yet)**:
- Speaker recognition (names) -- speakers labeled "Speaker 1", "Speaker 2"
- Cross-session voice learning
- Real-time summarization (summary comes after meeting ends)
- Plugin for Obsidian (just file writes)
- Non-macOS support
- Local/offline mode
- Meeting calendar integration

### 7.2 MVP Commands

```bash
# Record a meeting (interactive)
meetbuddy record --title "Sprint Planning"
# Starts capturing system + mic audio
# Shows live transcript in terminal
# Ctrl+C or 'q' to stop
# Processes transcript through Claude
# Writes to Obsidian vault

# Configure
meetbuddy config init
# Wizard: Deepgram API key, Claude API key, Obsidian vault path, output template

# List past meetings
meetbuddy list
# Shows recent meetings with dates, titles, durations

# View a meeting
meetbuddy view "Sprint Planning" --date 2026-03-28
# Opens the meeting note in Obsidian (via URI scheme)

# Status
meetbuddy status
# Shows API key validity, vault path, audio device info
```

### 7.3 MVP Config File (`~/.meetbuddy/config.yaml`)

```yaml
deepgram:
  api_key: ${DEEPGRAM_API_KEY}  # env var reference
  model: nova-3
  language: en

claude:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-haiku-4-5
  # model: claude-sonnet-4-6  # for complex meetings

obsidian:
  vault_path: ~/Documents/Obsidian/MyVault
  meetings_folder: meetings
  template: default  # or path to custom template
  daily_note_append: false

audio:
  system_audio: true
  microphone: true
  save_recording: false  # save WAV locally
  recording_path: ~/.meetbuddy/recordings/

output:
  include_transcript: true  # include raw transcript in note
  include_timestamps: true
  language: en
```

### 7.4 MVP Technical Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Audio capture | Core Audio Taps via Swift helper | Only reliable method on macOS 14.2+ without third-party drivers |
| STT + Diarization | Deepgram Nova-3 streaming | Best Go SDK, cheapest streaming, $200 free credit |
| LLM | Claude Haiku 4.5 via anthropic-sdk-go | $0.02/meeting, official Go SDK, structured output |
| Obsidian | Direct file write | Zero dependencies, works immediately, no plugin to maintain |
| Mic capture | malgo | No system dependencies, cross-platform ready |
| CLI framework | Cobra | Same as the-matrix, proven, excellent UX |
| Config | Viper | Same as the-matrix, env var support, YAML |

---

## 8. Phased Roadmap

### Phase 0: Spike (1 week)

**Goal**: Prove the audio capture + Deepgram streaming pipeline works end-to-end.

- [ ] Build Swift audio helper (fork/vendor audiotee)
- [ ] Go subprocess spawns Swift helper, reads PCM from stdout
- [ ] Stream PCM to Deepgram WebSocket, get back diarized text
- [ ] Print transcript to terminal
- **Kill criterion**: If Deepgram diarization quality on meeting audio is below 80% accuracy, re-evaluate API choice

### Phase 1: MVP (3-4 weeks) -- v0.1.0

**Goal**: Complete record-to-Obsidian pipeline.

- [ ] `meetbuddy record` command with live terminal display
- [ ] System audio + microphone dual capture
- [ ] Deepgram streaming STT + diarization
- [ ] Claude post-meeting summarization
- [ ] Obsidian vault markdown output with template
- [ ] `meetbuddy config init` wizard
- [ ] `meetbuddy status` diagnostic command
- [ ] Basic error handling and graceful shutdown (Ctrl+C)
- [ ] GitHub repo, MIT license, README

### Phase 2: Speaker Recognition + Polish (3-4 weeks) -- v0.2.0

**Goal**: Speakers get real names. Distribution begins.

- [ ] `meetbuddy enroll --name "Omer"` voice enrollment
- [ ] ECAPA-TDNN ONNX model for speaker embeddings
- [ ] Local voice profile storage (`~/.meetbuddy/voices/`)
- [ ] Automatic speaker-to-name mapping during meetings
- [ ] Homebrew tap distribution (GoReleaser)
- [ ] `meetbuddy list` and `meetbuddy view` commands
- [ ] Save recordings locally (optional, for re-processing)
- [ ] Custom Obsidian templates

### Phase 3: Intelligence + Cross-Platform (4-6 weeks) -- v0.3.0

**Goal**: The tool gets smarter and works on more platforms.

- [ ] Cross-meeting memory: Claude remembers context from past meetings
- [ ] Meeting type detection (standup, planning, 1:1, interview)
- [ ] Automatic action item tracking (which items are still open?)
- [ ] Linux support (PulseAudio/PipeWire monitor capture)
- [ ] `--local` mode: Whisper.cpp + pyannote (no cloud STT)
- [ ] Obsidian daily note integration
- [ ] Meeting calendar integration (Google Calendar, Outlook)
- [ ] People page auto-creation in Obsidian

### Phase 4: Enterprise + Community (ongoing) -- v0.4.0+

**Goal**: Sustainable open-source project.

- [ ] Windows support (WASAPI loopback)
- [ ] Real-time summarization (streaming Claude integration)
- [ ] Team features: shared meeting library
- [ ] Obsidian plugin for enhanced UX (sidebar, search)
- [ ] Alternative LLM backends (Ollama, GPT-4, Gemini)
- [ ] Alternative STT backends (AssemblyAI, Gladia, local Whisper)
- [ ] MCP server for external tool integration
- [ ] Webhook output for Slack/Discord/Notion

---

## 9. Cost Analysis

### 9.1 API Costs Per Meeting

| Component | 30-min meeting | 1-hour meeting | 2-hour meeting |
|-----------|---------------|----------------|----------------|
| Deepgram STT + Diarization | $0.29 | $0.58 | $1.16 |
| Claude Haiku (summary) | $0.011 | $0.022 | $0.044 |
| Claude Sonnet (if upgraded) | $0.048 | $0.096 | $0.192 |
| **Total (Haiku)** | **$0.30** | **$0.60** | **$1.20** |
| **Total (Sonnet)** | **$0.34** | **$0.68** | **$1.35** |

### 9.2 Monthly Cost Scenarios

| Usage Pattern | Meetings/Week | Hours/Week | Monthly Cost (Haiku) | Monthly Cost (Sonnet) |
|--------------|--------------|-----------|---------------------|----------------------|
| Light (IC engineer) | 5 | 5 | $12.00 | $13.60 |
| Medium (eng manager) | 10 | 10 | $24.00 | $27.20 |
| Heavy (PM/exec) | 15 | 15 | $36.00 | $40.80 |
| Extreme (all-day) | 20 | 20 | $48.00 | $54.40 |

### 9.3 Free Tier Coverage

| API | Free Amount | Equivalent Meetings (1hr each) |
|-----|-----------|-------------------------------|
| Deepgram | $200 credit | ~345 meetings |
| AssemblyAI | 185 hours/month | 185 meetings/month |
| Claude | Pay-per-use only | N/A |

**Deepgram's $200 free credit alone covers ~345 one-hour meetings.** That is approximately 7 months of heavy use (10 meetings/week) before any payment is needed.

### 9.4 Cost Comparison vs Competitors

| Tool | Monthly Cost (10 meetings/week) | What You Get |
|------|-------------------------------|-------------|
| **meetbuddy** | ~$24 (API costs) | Full control, Obsidian native, CLI, open source |
| Otter Pro | $16.99 | 6,000 min/mo, cloud storage, no Obsidian |
| Fireflies Pro | $10/mo | Cloud, CRM integrations, no Obsidian |
| Granola Individual | $18/mo | Desktop app, no CLI, limited Obsidian |
| Fathom Premium | $19/mo | Free recording, paid AI features |
| Meetily | $0 (local) | Requires Docker/Python, no Obsidian, basic UX |

meetbuddy at ~$24/month for a heavy user is competitive, with the advantage of full data ownership, open source, and Obsidian integration. For light users (5 meetings/week), it is $12/month.

### 9.5 Development Time Investment

| Phase | Estimated Effort | Timeline |
|-------|-----------------|----------|
| Phase 0 (Spike) | 1 week, solo | Week 1 |
| Phase 1 (MVP) | 3-4 weeks, solo | Weeks 2-5 |
| Phase 2 (Speaker Recognition) | 3-4 weeks | Weeks 6-9 |
| Phase 3 (Intelligence) | 4-6 weeks | Weeks 10-15 |
| **Total to v0.3** | **~15 weeks** | **~4 months** |

---

## 10. Risk Matrix

### Risk Assessment (Impact x Likelihood)

| # | Risk | Impact (1-5) | Likelihood (1-5) | Score | Mitigation |
|---|------|-------------|------------------|-------|------------|
| R1 | **macOS audio permissions break in future update** | 4 | 2 | 8 | Core Audio Taps is Apple's official API (not a hack). Low risk of removal. BlackHole fallback available. |
| R2 | **Diarization accuracy below 85% in real meetings** | 5 | 2 | 10 | Phase 0 spike validates this before committing. Fallback to AssemblyAI. Multi-API support planned. |
| R3 | **Deepgram pricing increases or free tier removed** | 3 | 2 | 6 | Abstract STT provider behind interface. AssemblyAI (185hr free/mo) as backup. Local Whisper as escape hatch. |
| R4 | **Legal risk: recording without consent** | 5 | 3 | 15 | Built-in consent warning on startup. Documentation on recording laws. Recommend users inform participants. Not a technical problem to solve. |
| R5 | **Scope creep into GUI territory** | 3 | 3 | 9 | Strict CLI-only mandate. Obsidian IS the GUI. No Electron/Tauri/React. |
| R6 | **Obsidian adds native meeting features** | 4 | 1 | 4 | Obsidian's team is small, focused on core editor. Even if they add it, CLI integration remains unique. |
| R7 | **Claude API latency for post-meeting processing** | 2 | 2 | 4 | Processing happens post-meeting (not blocking). Use streaming API for progress indication. |
| R8 | **Swift helper binary distribution complexity** | 3 | 3 | 9 | Package both binaries in Homebrew formula. Build Swift binary in CI. Alternatively, embed via CGo bridge. |
| R9 | **Competition from Granola** | 3 | 4 | 12 | Different audience (CLI developers vs general users). Granola is $18/mo closed-source; meetbuddy is open-source CLI. |
| R10 | **Low adoption due to niche positioning** | 4 | 3 | 12 | Niche is intentional -- Obsidian has 5M+ users, CLI/terminal users are a growing segment. Quality > breadth. |

### Top 3 Risks to Monitor

1. **R4 -- Legal/consent risk** (Score: 15). Meeting recording laws vary by jurisdiction. The tool MUST display a prominent consent warning and recommend users inform all participants. Include `--no-consent-warning` flag only after explicit acknowledgment.

2. **R9 -- Granola competition** (Score: 12). Granola is well-funded and expanding. Differentiation is critical: CLI-native, open-source, Obsidian-first, Claude-powered, speaker recognition across sessions.

3. **R10 -- Adoption** (Score: 12). The Obsidian + CLI + meeting recording intersection is narrow. Mitigation: the tool should work great even without Obsidian (just outputs markdown files) and even without the CLI framing (it is simply a meeting transcription tool).

---

## 11. Distribution Plan

### 11.1 Open Source Strategy

- **License**: MIT (same as audiotee, same as the-matrix)
- **Repository**: `github.com/omerufuk/meetbuddy` (or chosen name)
- **Monorepo**: Go binary + Swift audio helper in one repo
- **CI**: GitHub Actions (build Go + Swift, cross-compile, run tests)

### 11.2 Homebrew Distribution

```ruby
class Meetbuddy < Formula
  desc "CLI meeting companion with speaker diarization and Obsidian integration"
  homepage "https://github.com/omerufuk/meetbuddy"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/omerufuk/meetbuddy/releases/download/v0.1.0/meetbuddy_darwin_arm64.tar.gz"
      sha256 "..."
    else
      url "https://github.com/omerufuk/meetbuddy/releases/download/v0.1.0/meetbuddy_darwin_amd64.tar.gz"
      sha256 "..."
    end
  end

  def install
    bin.install "meetbuddy"
    bin.install "meetbuddy-audio"  # Swift audio helper
  end

  def caveats
    <<~EOS
      meetbuddy requires:
      - macOS 14.2+ (for system audio capture)
      - Deepgram API key (DEEPGRAM_API_KEY)
      - Anthropic API key (ANTHROPIC_API_KEY)

      Run `meetbuddy config init` to get started.
    EOS
  end
end
```

### 11.3 GoReleaser Configuration

Standard GoReleaser for the Go binary. The Swift audio helper needs a separate build step in CI:

```yaml
# .goreleaser.yml (simplified)
builds:
  - id: meetbuddy
    main: ./cmd/meetbuddy
    binary: meetbuddy
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -X main.version={{.Version}}

# Swift binary built separately in GitHub Actions before GoReleaser runs
```

### 11.4 macOS Code Signing

- **Without signing**: Users get Gatekeeper warning, must right-click > Open
- **With Apple Developer Program ($99/year)**: Signed + notarized, no warning
- **Recommendation**: Skip code signing for v0.1 (open-source norms). Add it if adoption warrants the $99/year investment.

### 11.5 Permissions Setup

On first run, the tool needs:
1. **Microphone permission**: macOS will prompt automatically when the Go binary accesses the mic via malgo
2. **Screen Recording permission** (for system audio): User must manually grant in System Settings > Privacy & Security > Screen Recording
3. The CLI should detect missing permissions and guide the user:
   ```
   meetbuddy requires Screen Recording permission to capture meeting audio.
   Please grant permission in:
     System Settings > Privacy & Security > Screen Recording
   Then restart meetbuddy.
   ```

### 11.6 Community Building

- README with GIF demo (terminal recording showing live transcript)
- "Awesome Obsidian" list submission
- Hacker News launch post (CLI tools do well there)
- r/ObsidianMD subreddit post
- YouTube demo (5-minute walkthrough)
- Discord or GitHub Discussions for community

---

## 12. Naming Recommendations

| # | Name | CLI Command | Domain/GitHub Availability | Meaning | Pros | Cons |
|---|------|------------|---------------------------|---------|------|------|
| 1 | **hearlog** | `hearlog` | GitHub: **AVAILABLE** (0 results) | Hear + Log | Short, typeable, descriptive, unique | Slightly generic |
| 2 | **vaultear** | `vaultear` | GitHub: **AVAILABLE** (0 results) | Vault (Obsidian) + Ear | Perfect Obsidian reference, unique | Compound word, might be misread |
| 3 | **scriba** | `scriba` | GitHub: **AVAILABLE** for meeting context (0 results) | Latin for "scribe/secretary" | Elegant, short, historical meaning | Latin may not be obvious |
| 4 | **echopad** | `echopad` | GitHub: semi-available (only 0-star repos) | Echo (audio) + Pad (notes) | Descriptive, modern feel | Some existing repos |
| 5 | **memnos** | `memnos` | GitHub: **AVAILABLE** (0 results for meeting context) | Greek "mnemos" (memory) | Unique, memorable, knowledge-related | Pronunciation unclear |

### Top Recommendation: **hearlog**

- 7 characters, easy to type: `hearlog record --title "Sprint"`
- Immediately communicates function: hearing + logging
- No GitHub conflicts
- Works as a noun ("check the hearlog") and a verb ("hearlog this meeting")
- Clean branding potential
- Domain likely available

### Runner-up: **scriba**

- 6 characters, elegant: `scriba record --title "Sprint"`
- Rich historical meaning (Roman scribe/secretary who took dictation)
- Sounds like "scriber" which is intuitively correct
- Unique in the tooling space

---

## 13. Technical Decisions

### TD-001: Go + Swift Hybrid Architecture

**Status**: Proposed
**Context**: macOS system audio capture requires Apple's Core Audio Taps API (Swift/ObjC only). The CLI tool, API integrations, and Obsidian output are best served by Go.
**Decision**: Go orchestrator binary + thin Swift audio helper binary communicating via stdout pipe.
**Alternatives considered**:
- Pure Go with CGo bridge to CoreAudio: Complex, fragile, CoreAudio taps API not well-documented for C
- Pure Swift: Loses Go distribution story (no GoReleaser, no Homebrew tap synergy)
- Rust (like Meetily): Would require learning new ecosystem, no synergy with the-matrix
- Node.js with native addon: Too heavy, not distributable as single binary
**Consequences**: Two binaries to distribute. CI must build Swift for macOS targets. Linux version does not need the Swift helper.

### TD-002: Deepgram as Primary STT Provider

**Status**: Proposed
**Context**: Need real-time streaming STT with speaker diarization, official Go SDK, and reasonable pricing.
**Decision**: Deepgram Nova-3 with diarization add-on.
**Alternatives considered**:
- AssemblyAI: Excellent accuracy, Go SDK, but more expensive for streaming ($0.57/hr vs $0.58/hr -- roughly equivalent, but Deepgram has $200 free credit)
- pyannoteAI: Best DER (~8%), but no Go SDK, batch-only (no real-time), requires separate STT
- Gladia: All-inclusive pricing ($0.55/hr), but no Go SDK
- Local Whisper + pyannote: Maximum privacy, but 5-10x slower, lower accuracy, requires Python/Docker
**Consequences**: Cloud dependency for STT. Must abstract provider behind interface for future swappability.

### TD-003: Provider Abstraction Layer

**Status**: Proposed
**Context**: STT provider lock-in is a risk. Users may prefer different providers for cost, accuracy, or privacy reasons.
**Decision**: Define a Go interface for the STT+diarization provider. Deepgram is the first implementation. Interface supports both streaming and batch modes.
```go
type STTProvider interface {
    StreamTranscribe(ctx context.Context, audio <-chan []byte, opts StreamOpts) (<-chan Segment, error)
    BatchTranscribe(ctx context.Context, audioPath string, opts BatchOpts) ([]Segment, error)
}

type Segment struct {
    Speaker   string
    Text      string
    Start     time.Duration
    End       time.Duration
    Confidence float64
}
```
**Consequences**: Slightly more upfront work, but enables AssemblyAI, Gladia, Whisper backends later.

### TD-004: Post-Meeting Claude Processing (Not Real-Time)

**Status**: Proposed
**Context**: Two approaches: (a) send transcript to Claude after the meeting ends, or (b) stream transcript to Claude during the meeting for live summaries.
**Decision**: Post-meeting processing for MVP. Full transcript sent to Claude as a single prompt after meeting ends.
**Rationale**:
- Simpler architecture (no concurrent Claude streaming)
- Better summary quality (Claude sees full context, not fragments)
- Lower cost (one API call vs many)
- Live transcription already provides real-time value; summary can wait 10-30 seconds
**Consequences**: 10-30 second processing delay after meeting ends. Acceptable tradeoff.

### TD-005: File-Based Obsidian Integration (No Plugin)

**Status**: Proposed
**Context**: Obsidian integration can be done via (a) writing markdown files directly to the vault, (b) building an Obsidian plugin, or (c) using Obsidian CLI.
**Decision**: Direct file writes to vault directory for MVP. Obsidian detects new files automatically.
**Rationale**:
- Zero dependencies -- works with any Obsidian version
- No plugin maintenance burden
- Obsidian's file watcher picks up new files instantly
- Obsidian CLI can be used to open notes afterward (`obsidian open ...`)
- Plugin can be added later for enhanced UX without breaking the file-based approach
**Consequences**: No real-time Obsidian integration during meeting. User sees note after meeting ends. This is acceptable -- you should not be reading notes during the meeting anyway.

### TD-006: MIT License

**Status**: Proposed
**Context**: Need to choose an open-source license.
**Decision**: MIT License.
**Rationale**:
- Maximum adoption (no copyleft concerns for companies)
- Same as audiotee (which we may vendor)
- Same as the-matrix ecosystem
- Same as most Go CLI tools (gh, lazygit, etc.)
**Consequences**: Anyone can fork, modify, or commercialize. This is intentional -- the tool's value is in the implementation and community, not the license.

### TD-007: Speaker Profile Storage Format

**Status**: Proposed
**Context**: Speaker recognition requires storing voice embeddings locally.
**Decision**: JSON files in `~/.meetbuddy/voices/{name}.json` containing speaker embedding vectors.
```json
{
  "name": "Omer Ufuk",
  "created": "2026-03-28T10:00:00Z",
  "updated": "2026-03-28T10:00:00Z",
  "embeddings": [
    {
      "source": "enrollment",
      "vector": [0.123, -0.456, ...],
      "duration_seconds": 15.2
    },
    {
      "source": "meeting-2026-03-28-sprint",
      "vector": [0.125, -0.452, ...],
      "duration_seconds": 342.1
    }
  ],
  "average_embedding": [0.124, -0.454, ...]
}
```
**Rationale**: Simple, human-readable, no database dependency. Embeddings are small (192 floats = ~1.5KB per embedding).
**Consequences**: No query capability over voice profiles. Fine for a CLI tool with <100 profiles.

---

## Appendix A: Key GitHub Projects Referenced

| Project | Stars | Language | Relevance |
|---------|-------|----------|-----------|
| [whisperX](https://github.com/m-bain/whisperX) | 20.9K | Python | ASR + diarization, local processing reference |
| [meetily](https://github.com/Zackriya-Solutions/meetily) | 10.7K | Rust | Privacy-first meeting tool, validates market demand |
| [transcriptionstream](https://github.com/transcriptionstream/transcriptionstream) | 923 | Python | Self-hosted transcription + diarization |
| [meetscribe](https://github.com/pretyflaco/meetscribe) | 284 | Python | Local meeting transcription CLI |
| [audiotee](https://github.com/makeusabrew/audiotee) | ~100 | Swift | Core Audio Taps reference implementation (MIT) |
| [deepgram-go-sdk](https://github.com/deepgram/deepgram-go-sdk) | ~100 | Go | Official Deepgram Go SDK |
| [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) | ~1K | Go | Official Anthropic Go SDK |
| [malgo](https://github.com/gen2brain/malgo) | ~200 | Go | miniaudio Go bindings for mic capture |
| [voicetag](https://github.com/Gr122lyBr/voicetag) | 44 | Python | Speaker identification with pyannote + resemblyzer |

---

## Appendix B: Recording Consent Legal Summary

| Jurisdiction | Type | Requirement |
|-------------|------|-------------|
| **Federal (US)** | One-party | One participant must consent |
| **38 US states** (incl. NY, TX) | One-party | You can record if you are a participant |
| **~13 US states** (incl. CA, FL, IL, MA, PA, WA) | All-party | All participants must consent |
| **EU (GDPR)** | Varies | Generally requires informing all parties, legitimate interest or consent |
| **UK** | One-party | Legal to record for personal use |
| **Turkey** | Consent required | Must inform participants |

**The tool should**:
1. Display consent warning on every `record` invocation
2. Provide a `--consent-acknowledged` flag for automation
3. Include a `RECORDING_NOTICE.md` template that users can share with participants
4. Never silently record -- always show a visible indicator that recording is active

---

## Appendix C: Full API Pricing Reference (March 2026)

### Deepgram
- Nova-3 (mono): $0.0077/min ($0.462/hr)
- Nova-3 (multi): $0.0092/min ($0.552/hr)
- Diarization add-on: $0.0020/min ($0.12/hr)
- Streaming: Same price as batch
- Free credit: $200 (no expiration)

### AssemblyAI
- Universal-3 Pro (pre-recorded): $0.21/hr
- Universal-3 Pro (streaming): $0.45/hr
- Diarization: $0.02/hr (pre-recorded), $0.12/hr (streaming)
- Free: 185hr/mo pre-recorded, 333hr/mo streaming

### pyannoteAI
- Developer: EUR 19/mo (125hr included)
- Starter: EUR 99/mo (825hr included)
- Precision-2 diarization: EUR 0.14/hr overage
- STT orchestration: EUR 0.25/hr overage
- Free trial: 150hr (1 month)

### Gladia
- Solaria-1: $0.55/hr (all features included)
- Pro plan: $0.612/hr
- Free: 10hr/mo

### Claude (Anthropic)
- Haiku 4.5: $0.25/$1.25 per M input/output tokens
- Sonnet 4.6: $3/$15 per M input/output tokens
- Opus 4.6: $5/$25 per M input/output tokens
- Batch API: 50% discount
- Prompt caching: 10% of input cost for hits

---

## Appendix D: Synergies with the-matrix

This project has natural synergies with the-matrix ecosystem, though it should be a separate repository:

1. **Distribution**: Same GoReleaser + Homebrew tap infrastructure
2. **CLI patterns**: Same Cobra + Viper stack, same CLI style (charmbracelet TUI)
3. **Build system**: Same Makefile patterns
4. **Agent ecosystem**: Could generate a `.claude/` ecosystem for the meetbuddy repo using neo
5. **Oracle**: Could use oracle to research speech processing best practices
6. **Brand**: "From the creators of the-matrix" adds credibility

The project should NOT be part of the-matrix monorepo. It is a different product with a different audience. Separate repo, separate releases, separate identity.
