# heimdall

CLI meeting companion that captures audio, transcribes with speaker diarization, analyzes via Claude, and writes structured notes to an Obsidian vault.

Named after the Norse god who could hear grass growing.

## Features

- Captures system audio (remote participants) and microphone (your voice) simultaneously
- Real-time transcription with speaker diarization via Deepgram Nova-3 (default), with Soniox as an opt-in alternative (`--transcriber soniox`)
- Optional fully-offline path: `--save-audio` to capture raw audio, `heimdall transcribe` to transcribe it locally via Whisper -- no cloud, no API key, no per-meeting cost
- Post-meeting analysis via Claude (API key or a local Claude Code login -- `--analyzer claude-code`) -- summary, decisions, action items, speaker identification
- Writes Obsidian-native markdown with YAML frontmatter, wikilinks, and collapsible transcript
- `heimdall mcp` exposes your meeting history to Claude Desktop, Claude Code, Cursor, or any MCP client -- ask "what did we decide about the API migration?" and get an answer sourced from your vault
- Crash recovery -- periodic temp file saves, recover interrupted sessions
- Graceful degradation -- Claude fails? Raw transcript. Deepgram fails? Raw audio saved.

## Requirements

- macOS 14.2+ (required for Core Audio Taps system audio capture)
- Go 1.27+
- Deepgram API key ([get one free](https://console.deepgram.com/))
- For meeting analysis, one of:
  - an Anthropic API key (`--analyzer api`, the default), or
  - a local, logged-in [Claude Code](https://claude.com/claude-code) install (`--analyzer claude-code` -- reuses your existing Claude subscription, no separate API key)

## Quick Start

### Install from source

```bash
git clone https://github.com/0merUfuk/heimdall.git
cd heimdall
make build
```

### Configure

```bash
# Set API keys as environment variables (never stored in plaintext).
export DEEPGRAM_API_KEY=your_deepgram_key
export ANTHROPIC_API_KEY=your_anthropic_key

# Run the interactive configuration wizard.
bin/heimdall config init
```

### Record a meeting

```bash
bin/heimdall record --title "Sprint Planning"
```

Press Ctrl+C to stop recording. The meeting is analyzed via Claude and written to your Obsidian vault.

### Check prerequisites

```bash
bin/heimdall doctor
```

## Commands

### `heimdall record`

Record a meeting with live transcription.

```bash
heimdall record --title "Sprint Planning"
heimdall record --title "1:1 with Sarah" --participants "Sarah"
heimdall record --title "Meeting" --language tr
heimdall record --title "Mixed Meeting" --language multi
heimdall record --title "Meeting" --keywords "Kubernetes,gRPC"
heimdall record --title "Meeting" --analyzer claude-code
```

| Flag | Description |
|------|-------------|
| `--title` | Meeting title (required) |
| `--participants` | Comma-separated participant names (hints for speaker ID) |
| `--language` | Transcription language code (default: `en`, use `multi` for auto-detect) |
| `--keywords` | Comma-separated context keywords (Deepgram only; ignored under `--transcriber soniox`) |
| `--transcriber` | Transcription provider: `deepgram` (default) or `soniox`. `soniox` requires `SONIOX_API_KEY` |
| `--analyzer` | Meeting-analysis backend: `api` (default, needs `ANTHROPIC_API_KEY`) or `claude-code` (shells out to a local, logged-in `claude` CLI -- no API key needed) |
| `--profile` | Use a named meeting profile from config (loads title, language, participants, keywords); explicit flags override profile values |
| `--consent-acknowledged` | Acknowledge the recording-consent banner non-interactively (scripts/CI; does not persist to config) |
| `--save-audio` | Save the raw mixed audio (16kHz stereo WAV, L=system/R=mic) to `audio.recording_path`; does not persist to config. Graceful-degradation backup -- if the transcriber and analyzer both fail, the meeting audio is still on disk. Same effect as setting `audio.save_recording: true` |

#### Analysis backends

heimdall never bundles its own model access -- Stage 5 (analysis) always calls out to a Claude backend you provide:

- **`api`** (default): calls the Anthropic Messages API directly with `ANTHROPIC_API_KEY`. Billed per token; works anywhere, including CI/headless hosts.
- **`claude-code`**: runs `claude -p` against your local [Claude Code](https://claude.com/claude-code) install. If you already pay for a Claude subscription (Pro/Max/Team) and have run `claude /login` once, this needs no separate API key and no extra cost per meeting. Requires the `claude` CLI on `PATH` and an active login; `heimdall doctor` reports its availability.

Set a persistent default with `heimdall config set claude.analyzer claude-code` instead of passing `--analyzer` on every `record`/`recover`/`analyze` call.

### `heimdall doctor`

Check all prerequisites: macOS version, API keys, audio permissions, vault path.

The Soniox API key is checked as optional: `doctor` reports `[pass]` when `SONIOX_API_KEY` is set and `[info]` when it is not. Because Soniox is opt-in (`--transcriber soniox`), an unset key does not count as a failed check. The same pattern applies to Claude analysis: `doctor` passes as long as *either* `ANTHROPIC_API_KEY` or a working `claude` CLI is available, and only fails if neither is.

### `heimdall list`

List past meeting notes from the Obsidian vault.

```bash
heimdall list
heimdall list --since "2026-03-01"
```

### `heimdall config`

Manage configuration.

```bash
heimdall config init                              # Interactive setup
heimdall config get obsidian.vault_path           # Get a value
heimdall config set claude.model claude-sonnet-5   # Set a value
```

### `heimdall recover`

Scan the recovery directory for orphaned transcripts from crashed sessions and re-analyze them. Accepts `--analyzer claude-code` like `record`.

### `heimdall analyze`

Re-analyze a specific recovery transcript file.

```bash
heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json
heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json --analyzer claude-code
```

### `heimdall transcribe`

Transcribe a saved audio file locally via [Whisper](https://github.com/ggml-org/whisper.cpp) -- fully offline, no API key, no cloud, no per-meeting cost. Pairs with `--save-audio`: record without any transcription provider, transcribe later on your own schedule.

```bash
heimdall record --save-audio --transcriber deepgram   # or skip cloud STT entirely and just save audio
heimdall transcribe --file ~/.heimdall/recordings/2026-09-13T10-00-00-standup.wav
heimdall transcribe --file meeting.wav --model small --language tr
```

Requires the `whisper-cli` binary (`brew install whisper-cpp`) and a downloaded model (`heimdall model download base`). Always passes whisper.cpp's built-in `--diarize` (stereo-channel diarization), separating system audio (remote participants) from your microphone -- a real but coarse two-party split, not per-individual diarization like Deepgram/Soniox. Writes output in the same format as crash recovery, so `heimdall analyze --file <path>` picks it up directly.

### `heimdall model download <size>`

Downloads a Whisper model (`tiny`, `base`, `small`, `medium`, `large`) to `~/.heimdall/models/` for use with `heimdall transcribe`.

```bash
heimdall model download base
```

### `heimdall eval`

Run the meeting-analysis quality suite against seven golden transcripts, checking coverage, anti-hallucination, prompt-injection resistance, and multilingual consistency. Add `--judge` for LLM-as-judge faithfulness/coverage scoring. See [docs/EVALUATION.md](docs/EVALUATION.md).

```bash
heimdall eval
heimdall eval --analyzer claude-code
heimdall eval --judge --json
```

### `heimdall mcp`

Runs a local [MCP](https://modelcontextprotocol.io) server over stdio, exposing your Obsidian vault's meeting notes to any MCP client as three read-only tools:

| Tool | Purpose |
|------|---------|
| `list_meetings` | List past meetings, most recent first |
| `search_meetings` | Full-text search across all meeting notes |
| `get_meeting` | Get one meeting's full content (summary, decisions, action items, transcript) |

Runs entirely locally -- no network access, no data leaves your machine beyond what your MCP client itself does with the results. Add to Claude Desktop's config (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "heimdall": {
      "command": "heimdall",
      "args": ["mcp"]
    }
  }
}
```

Not a background daemon -- your MCP client starts and stops it as part of managing the connection.

### `heimdall version`

Print version and build information.

## Configuration

Configuration lives at `~/.heimdall/config.yaml`. API keys are stored as environment variable references.

```yaml
deepgram:
  api_key: ${DEEPGRAM_API_KEY}
  model: nova-3
  language: en

claude:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-haiku-4-5
  analyzer: api   # or "claude-code" to use a local Claude Code login instead of api_key

obsidian:
  vault_path: ~/Documents/Obsidian/MyVault
  meetings_folder: meetings
  template: default

audio:
  system_audio: true
  microphone: true
  save_recording: false            # true (or --save-audio) saves raw meeting audio as WAV
  recording_path: ~/.heimdall/recordings/

output:
  include_transcript: true
  include_timestamps: true
  language: en
```

## Architecture

heimdall uses a 6-stage pipeline. The LLM appears in exactly one stage (Stage 5), after the meeting ends.

```
Stage 1: CAPTURE     System audio (Swift/Core Audio Taps) + microphone (Go/malgo)
Stage 2: MIX         Resample 48kHz->16kHz, interleave stereo (L=system, R=mic)
Stage 3: TRANSCRIBE  Deepgram Nova-3 WebSocket (mono + diarize) -- identifies N speakers by voice fingerprinting
Stage 4: ACCUMULATE  In-memory segments + live terminal display
Stage 5: ANALYZE     Claude API (post-meeting) -- summary, decisions, action items
Stage 6: RENDER      Go templates -> Obsidian-native markdown
```

### Dual-binary architecture

| Binary | Language | Role |
|--------|----------|------|
| `heimdall` | Go | CLI orchestrator -- all pipeline stages, config, commands |
| `heimdall-audio` | Swift | Thin audio capture -- Core Audio Taps, PCM to stdout |

The Go binary spawns the Swift binary as a subprocess and reads PCM from its stdout pipe.

### Graceful degradation

```
Claude fails     -> Write raw transcript (no summary, no action items)
Deepgram fails   -> Save raw audio to recovery file
Audio fails      -> Clean exit with clear error message
Swift crashes    -> Detect within 2s, auto-restart (max 3 retries)
Network drops    -> Reconnect with exponential backoff (1s, 2s, 4s... max 30s)
Process killed   -> Recovery file written every 30s, recoverable
```

## Cost Per Meeting

heimdall sends mono audio with Deepgram diarization (ID-001). The cost table below uses stereo rates as a conservative ceiling; actual billing is roughly 50% lower at the mono rate.

| Duration | Deepgram (stereo + diarization) | Claude Haiku (summary) | Total |
|----------|--------------------------------|------------------------|-------|
| 30 min | $0.58 | $0.01 | ~$0.59 |
| 1 hour | $1.16 | $0.02 | ~$1.18 |
| 2 hours | $2.32 | $0.04 | ~$2.36 |

Deepgram offers $200 free credit -- enough for approximately 170 one-hour meetings.

## Project Structure

```
heimdall/
  cmd/heimdall/           CLI entry point (cobra commands)
  internal/
    audio/                AudioSource interface + mic/system implementations
    mixer/                Audio resampling, format conversion, stereo interleaving
    transcriber/          Transcriber interface + Deepgram and Soniox WebSocket implementations (NewFromName factory)
    analyzer/             Analyzer interface + Claude implementation
    output/               Obsidian template rendering + file writing
    config/               Config loading, validation, defaults
    recovery/             Crash recovery (temp files, re-analysis)
    session/              MeetingSession orchestrator (wires stages 1-4)
    heimdall/             Shared types (AudioFrame, Segment, MeetingNote)
  audio-helper/           Swift audio capture binary
  templates/              Embedded Go templates for Obsidian output
  docs/architecture/      Design docs (pipeline, decisions, MVP, vulnerabilities)
```

## Privacy and Data Flow

heimdall sends meeting data to two external services:

| Data | Destination | Retention | Training |
|------|-------------|-----------|----------|
| Raw audio (PCM) | Deepgram (US) via WebSocket | Zero after processing | No (mip_opt_out=true) |
| Meeting transcript | Anthropic (US) via HTTPS | 7 days (API policy) | Never (API data excluded) |
| Meeting notes | Your local Obsidian vault | You control | N/A |
| Recovery files | `~/.heimdall/recovery/` (local) | Until cleanup | N/A |
| Raw audio recording (opt-in, `--save-audio`) | `~/.heimdall/recordings/` (local) | Until you delete it | N/A |

heimdall does not store audio or transcripts on any server it controls. API keys are stored as environment variable references, never in plaintext config files.

**Recording consent**: Meeting recording may be subject to consent laws in your jurisdiction. In the US, 12+ states require all-party consent (see the [Justia 50-state survey](https://www.justia.com/50-state-surveys/recording-of-conversations-laws/) for the current state-by-state map). In the EU, participants must generally be informed. Please ensure all meeting participants are aware that recording is active.

See [PRIVACY.md](PRIVACY.md) for the full data-flow and biometric-data notice.

## License

MIT License. See [LICENSE](LICENSE) for details.
