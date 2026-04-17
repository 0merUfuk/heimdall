**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

# Pipeline Rules

> Auto-loaded when working in heimdall. These rules enforce the 6-stage pipeline architecture defined in `docs/architecture/PIPELINE.md`.

---

## Stage Separation

Each pipeline stage has ONE job. Never cross concerns:

| Stage | Does | Does NOT |
|-------|------|---------|
| 1. CAPTURE | Capture raw audio from hardware | Resample, transcribe, analyze |
| 2. MIX | Resample + interleave stereo | Capture audio, transcribe, analyze |
| 3. TRANSCRIBE | Stream audio → text segments | Capture audio, analyze, render |
| 4. ACCUMULATE | Buffer segments + display | Capture, mix, analyze, render |
| 5. ANALYZE | LLM summary (post-meeting only) | Capture, mix, transcribe in real-time |
| 6. RENDER | Templates → Obsidian markdown | Capture, analyze, transcribe |

**No LLM in the audio path** (stages 1-4). **No audio processing in the output path** (stages 5-6).

---

## Runtime Channel Mode (ID-001, supersedes AD-007)

- The mixer outputs stereo internally: **L = system audio, R = microphone**. Do not swap these — downstream code assumes the convention.
- The session (`internal/session/session.go`) **downmixes stereo to mono** before handing off to Deepgram. `TranscribeOpts` sends `channels=1`, `diarize=true`, no `multichannel`.
- Resampling: system audio 48kHz → 16kHz before mixing
- Bit depth: 32-bit float → 16-bit int before mixing
- Why mono: see `.claude/DECISIONS.md` ID-001. `multichannel=true` conflicted with `diarize=true` for N-speaker meetings.

---

## Graceful Degradation Chain

When a component fails, fall back to the next-best output. Never lose the meeting data:

```
Claude fails → Write raw transcript (no summary, no action items)
Deepgram fails → Save raw audio to recovery file
Audio capture fails → Clean exit with clear error message
Swift helper crashes → Detect within 2s, auto-restart (V-002)
Network drops → Buffer in ring buffer, reconnect with backoff (V-005)
```

---

## Provider Abstraction (AD-003)

Every external dependency sits behind an interface:

| Interface | Package | Implementations |
|-----------|---------|----------------|
| `AudioSource` | `internal/audio/` | `MicrophoneSource` (malgo), `SystemAudioSource` (Swift subprocess) |
| `Transcriber` | `internal/transcriber/` | `DeepgramTranscriber` (WebSocket) |
| `Analyzer` | `internal/analyzer/` | `ClaudeAnalyzer` (Anthropic API) |
| `OutputWriter` | `internal/output/` | `ObsidianWriter` (markdown templates) |

All tests must use mock implementations, not real external services.

---

## Deepgram WebSocket Rules

- Proactive reconnection at **55 minutes** (before the 60-min timeout, V-001)
- Open new connection BEFORE closing old one (seamless handoff)
- Realign timestamps on reconnection (new connection starts at 0)
- Re-map speaker IDs (new connection may assign different IDs)
- KeepAlive messages per Deepgram protocol
- Retry with exponential backoff: 1s, 2s, 4s, max 30s (V-005)
