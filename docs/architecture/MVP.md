# Heimdall — MVP Specification (v1.0.0)

**Version**: 1.1
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

## MVP Scope

### What v1.0 Does

1. Captures system audio from any meeting app (macOS 14.2+ only)
2. Captures microphone audio simultaneously
3. Streams dual-channel audio to Deepgram for real-time transcription + speaker diarization
4. Displays live transcript in terminal during meeting
5. After meeting ends, sends transcript to Claude for summarization + speaker identification
6. Writes structured meeting note to Obsidian vault directory
7. Crash recovery from unexpected termination

### What v1.0 Does NOT Do (Deferred)

- Speaker voice enrollment / recognition (speakers labeled by name from context only)
- Cross-session voice learning
- Real-time summarization (summary comes after meeting ends)
- Obsidian plugin (just file writes)
- Non-macOS support (Linux, Windows)
- Local/offline mode (Whisper)
- Meeting calendar integration
- Web dashboard
- Alternative LLM backends

---

## Commands

```bash
# Record a meeting (interactive, long-running)
heimdall record --title "Sprint Planning"
heimdall record --title "Team Sync" --participants "Omer,Sarah,Mike"
heimdall record --title "Sprint Planning" --keywords "Kubernetes,gRPC,CI/CD"
heimdall record --profile daily                                  # use a saved profile
heimdall record --language tr --title "Retro"                    # override STT language

# Configure (first-run wizard)
heimdall config init
# Interactive wizard: Deepgram API key, Claude API key, Obsidian vault path

# Show/set individual config values (keys are dotted paths into the YAML config)
heimdall config get obsidian.vault_path
heimdall config set claude.model claude-sonnet-4-6

# Diagnostics (validates all prerequisites)
heimdall doctor
# Checks: macOS version, Screen Recording permission, microphone permission,
#          API keys valid, vault path exists, audio devices available

# List past meetings
heimdall list
heimdall list --since "2026-03-01"
# Shows recent meetings: date, title, duration, speaker count

# Re-analyze a transcript (retry if Claude failed, or re-process with different model)
heimdall analyze --file ~/.heimdall/recovery/2026-03-28-sprint-planning.json
heimdall analyze --file ~/.heimdall/recovery/2026-03-28-sprint-planning.json --model claude-sonnet-4-6

# Recover from crash
heimdall recover
# Detects unprocessed transcripts from crashed sessions, offers to analyze + write

# Version
heimdall version
```

---

## Config File (`~/.heimdall/config.yaml`)

```yaml
deepgram:
  api_key: ${DEEPGRAM_API_KEY}     # env var reference — never stored in plaintext
  model: nova-3
  language: en

claude:
  api_key: ${ANTHROPIC_API_KEY}    # env var reference — never stored in plaintext
  model: claude-haiku-4-5
  # model: claude-sonnet-4-6       # for complex meetings

obsidian:
  vault_path: ~/Documents/Obsidian/MyVault
  meetings_folder: meetings
  template: default                 # or path to custom template
  daily_note_append: false

audio:
  system_audio: true
  microphone: true
  save_recording: false             # save raw WAV locally
  recording_path: ~/.heimdall/recordings/

output:
  include_transcript: true          # include raw transcript in note
  include_timestamps: true
  language: en

keywords: []                        # custom vocabulary for STT boost
  # - Kubernetes
  # - gRPC
  # - CI/CD
```

---

## Obsidian Meeting Note Template

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

---

## Obsidian Vault Structure

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
│   ├── Omer Ufuk.md              ← wikilinked from meeting notes
│   └── Sarah Chen.md
└── .heimdall/
    └── config.yaml               ← optional vault-level overrides
```

---

## Shutdown Sequence (Ctrl+C)

When the user ends the meeting:

```
^C received — finishing up...
  ✓ Audio capture stopped                    (< 1 second)
  ✓ Final transcript received (47 segments)  (1-3 seconds)
  ⠋ Generating meeting summary via Claude... (5-20 seconds)
  ✓ Summary complete
  ✓ Meeting note saved: ~/vault/meetings/2026-03-28/sprint-planning.md

Meeting recorded: 47m 23s | 4 speakers identified | 12 action items
```

**Shutdown steps in order:**
1. Signal Swift subprocess to stop → flushes remaining audio and exits
2. Send final audio bytes to Deepgram → wait for last `is_final` transcript segments
3. Close WebSocket cleanly
4. Send complete transcript to Claude → wait for structured analysis (retry 3x on failure)
5. Render Go template → write markdown to Obsidian vault
6. If Claude fails after all retries: write raw transcript to vault with "analysis pending" placeholder
7. Clean up temp recovery file
8. Print summary → exit

**Total shutdown time**: 10-30 seconds (dominated by Claude API call)

---

## Crash Recovery

If heimdall is force-killed (SIGKILL, battery death, terminal crash):

1. On next launch, `heimdall recover` checks `~/.heimdall/recovery/` for unprocessed transcripts
2. Recovery file contains: all segments with timestamps, speaker IDs, meeting start time, title
3. Recovery file is written every 30 seconds during recording (atomic write: temp → rename)
4. User is offered: "Found unprocessed transcript from 2026-03-28 14:30. Analyze and write to Obsidian? [Y/n]"
5. Maximum data loss on SIGKILL: ~30 seconds of transcript

---

## Provider Interfaces (Go)

Three abstraction boundaries for swappability:

```go
// Audio capture — swappable per platform
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
    SampleRate() int
    Channels() int
}

// Speech-to-text + diarization — swappable per provider
type Transcriber interface {
    Connect(ctx context.Context, opts TranscribeOpts) error
    Send(frame AudioFrame) error
    Receive() <-chan Segment
    Close() error
}

// Meeting intelligence — swappable per LLM
type Analyzer interface {
    Summarize(ctx context.Context, segments []Segment, opts AnalyzeOpts) (*MeetingNote, error)
}
```

**v1.0 implementations:**
- `AudioSource`: `SystemAudioSource` (macOS system audio via Swift subprocess), `MicrophoneSource` (microphone via malgo)
- `Transcriber`: `DeepgramTranscriber` (WebSocket streaming + diarization)
- `Analyzer`: `ClaudeAnalyzer` (Haiku 4.5 default, configurable)

---

## Cost Per Meeting (v1.0)

| Duration | Deepgram STT + Diarization | Claude Haiku Summary | Total |
|----------|---------------------------|---------------------|-------|
| 30 min | $0.29 | $0.011 | **$0.30** |
| 1 hour | $0.58 | $0.022 | **$0.60** |
| 2 hours | $1.16 | $0.044 | **$1.20** |

**Monthly estimates:**

| Usage | Meetings/Week | Monthly Cost |
|-------|--------------|-------------|
| Light (IC) | 5 | ~$12 |
| Medium (EM) | 10 | ~$24 |
| Heavy (PM) | 15 | ~$36 |

**Free tier coverage**: Deepgram's $200 credit covers ~345 one-hour meetings (~7 months of heavy use).

---

## Audio Design Details

### Dual-Channel Strategy (AD-007, superseded at the Deepgram boundary by ID-001)

System audio and microphone are interleaved into stereo inside the mixer:
- **Left channel**: System audio (remote participants — what Zoom/Meet/Teams plays)
- **Right channel**: Microphone (local user's voice)

This L=system / R=mic convention is load-bearing for mixer and any downstream per-source processing — downmix, save-audio (future), debugging — and must not be swapped.

Before the stream reaches Deepgram, `internal/session/session.go` downmixes the stereo frame to mono (averaging L+R). Deepgram receives **`channels=1` with `diarize=true`** — speakers are separated by voice fingerprint, not by channel assignment. This supports N-speaker meetings (the `multichannel` flag was dropped because it conflicts with `diarize` and caps identification at 2 speakers — one per channel; see ID-001 in `.claude/DECISIONS.md`).

### Audio Format Conversion

| Source | Format | Conversion |
|--------|--------|-----------|
| Core Audio Taps (system) | 48kHz, 32-bit float, stereo | → 16kHz, 16-bit int, mono (downmixed + resampled) |
| malgo (microphone) | 16kHz, 16-bit int, mono | → no conversion needed |
| Mixer output (internal) | 16kHz, 16-bit int, stereo (L=system, R=mic) | → downmixed to mono by `session.go` per ID-001 |
| Deepgram WebSocket (wire) | 16kHz, 16-bit int, mono + `diarize=true` | (final format sent to Deepgram) |

### Ring Buffer

30-60 second ring buffer between audio capture and Deepgram WebSocket:
- Absorbs network hiccups (WebSocket reconnection window)
- Prevents backpressure from blocking audio capture
- Audio is never dropped as long as reconnection completes within buffer duration

### Raw WAV Recording (Deferred)

Raw WAV recording was scoped for v1.0 but did not ship — the record command has no `save audio` flag today (see `cmd/heimdall/record.go`). The design remains a candidate for a future release:

- Target location: `~/.heimdall/recordings/YYYY-MM-DD-title.wav`
- Footprint: ~110MB per hour (16kHz stereo 16-bit)
- Stream directly to disk, not held in memory
- Use case: re-process with different STT provider, speaker enrollment training data, backup
