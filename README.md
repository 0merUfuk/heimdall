# heimdall

CLI meeting companion that captures audio, transcribes with speaker diarization, analyzes via Claude, Codex, or a fully on-device model, and writes structured notes to an Obsidian vault.

Named after the Norse god who could hear grass growing.

## Features

- Captures system audio (remote participants) and microphone (your voice) simultaneously
- Real-time transcription with speaker diarization via Deepgram Nova-3 (default), with Soniox as an opt-in alternative (`--transcriber soniox`)
- Optional fully-offline path: `--save-audio` to capture raw audio, `heimdall transcribe` to transcribe it locally via Whisper -- no cloud, no API key, no per-meeting cost
- Post-meeting analysis -- summary, decisions, action items, speaker identification -- via your choice of backend: Claude (API key, or a local Claude Code login with `--analyzer claude-code`), Codex (a local Codex login with `--analyzer codex`), or **fully on-device** via [Ollama](https://ollama.com) (`--analyzer ollama` -- the transcript never leaves your Mac)
- Fully offline meetings: `heimdall record --transcriber whisper --analyzer ollama` captures the meeting, transcribes it on your Mac with whisper.cpp after you stop, and analyzes it with a local model -- no audio, transcript, or analysis leaves the machine, no API key, no account
- Writes Obsidian-native markdown with YAML frontmatter, wikilinks, and collapsible transcript
- `heimdall mcp` exposes your meeting history to Claude Desktop, Claude Code, Cursor, or any MCP client -- ask "what did we decide about the API migration?" and get an answer sourced from your vault
- Crash recovery -- periodic temp file saves, recover interrupted sessions
- Graceful degradation -- analysis fails? Raw transcript, and the transcript is kept for a retry. Deepgram fails? Raw audio saved.

## Requirements

- macOS 14.2+ (required for Core Audio Taps system audio capture)
- Go 1.27+
- Deepgram API key ([get one free](https://console.deepgram.com/))
- For meeting analysis, one of:
  - an Anthropic API key (`--analyzer api`, the default), or
  - a local, logged-in [Claude Code](https://claude.com/claude-code) install (`--analyzer claude-code` -- reuses your existing Claude subscription, no separate API key), or
  - a local, logged-in [Codex](https://developers.openai.com/codex) install (`--analyzer codex` -- reuses your existing ChatGPT/Codex plan), or
  - [Ollama](https://ollama.com) with `qwen3:14b` pulled (`--analyzer ollama` -- fully on-device; ~9 GB model, 16 GB+ unified memory recommended)

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
| `--transcriber` | Transcription provider: `deepgram` (default), `soniox` (requires `SONIOX_API_KEY`), or `whisper` -- offline: no live transcript; the audio is saved and transcribed on this machine by whisper.cpp after you stop (needs `brew install whisper-cpp` and `heimdall model download base`) |
| `--whisper-model` | Whisper model for `--transcriber whisper`: `tiny`, `base` (default), `small`, `medium`, `large`, or a path to a `.bin` file |
| `--analyzer` | Meeting-analysis backend: `api` (default, needs `ANTHROPIC_API_KEY`), `claude-code` (local, logged-in `claude` CLI), `codex` (local, logged-in `codex` CLI), or `ollama` (fully on-device). Defaults to `claude.analyzer` from config |
| `--profile` | Use a named meeting profile from config (loads title, language, participants, keywords); explicit flags override profile values |
| `--consent-acknowledged` | Acknowledge the recording-consent banner non-interactively (scripts/CI; does not persist to config) |
| `--save-audio` | Save the raw mixed audio (16kHz stereo WAV, L=system/R=mic) to `audio.recording_path`; does not persist to config. Graceful-degradation backup -- if the transcriber and analyzer both fail, the meeting audio is still on disk. Same effect as setting `audio.save_recording: true` |

#### Analysis backends

heimdall never bundles its own model access -- Stage 5 (analysis) calls out to a backend you provide. Every default is the cheapest model that reliably produces the analysis:

| Backend | Default model | Needs | Where the transcript goes |
|---|---|---|---|
| **`api`** (default) | `claude-haiku-4-5` | `ANTHROPIC_API_KEY` (billed per token) | Anthropic API |
| **`claude-code`** | your `claude` default, or `claude.model` | `claude` CLI + `claude /login` (Claude Pro/Max/Team) | Anthropic, via your Claude Code login |
| **`codex`** | `gpt-5.6-luna`, low reasoning effort | `codex` CLI + `codex login` (ChatGPT/Codex plan) | OpenAI, via your Codex login |
| **`ollama`** | `qwen3:14b` | [Ollama](https://ollama.com) running + `ollama pull qwen3:14b` | **Nowhere -- stays on this machine** |

Set a persistent default with `heimdall config set claude.analyzer <backend>` instead of passing `--analyzer` on every `record`/`recover`/`analyze`/`eval` call. `heimdall doctor` reports which backends are available.

If analysis fails after retries, heimdall writes the raw transcript as the note, **keeps the transcript file**, and prints the exact `heimdall analyze --file ... --analyzer ...` command to retry. It never switches to a cloud backend on its own -- if you chose `ollama` for privacy, a cloud retry only happens if you run it yourself.

#### Fully offline meetings

```bash
brew install whisper-cpp && heimdall model download base   # local transcription
brew install ollama && ollama serve && ollama pull qwen3:14b  # local analysis
heimdall record --transcriber whisper --analyzer ollama --title "Design review"
```

Nothing is transcribed live and nothing is sent over the network: the meeting audio is saved (checkpointed every 30 seconds, so a crash keeps everything captured up to then), transcribed by whisper.cpp when you stop, analyzed by the local model, and written to your vault. Whisper's speaker separation is by channel -- remote participants (system audio) vs. you (microphone) -- not per individual. Like any recording, it needs macOS Microphone and "Screen & System Audio Recording" permission for the app you run it from.

#### Fully on-device analysis (`--analyzer ollama`)

```bash
brew install ollama && ollama serve        # or install the Ollama app
ollama pull qwen3:14b                      # ~9.3 GB
heimdall analyze --file <transcript.json> --analyzer ollama
```

- Uses Ollama's native API and sizes the model's context window to each transcript. A transcript too long for the window is **refused, never truncated** (a silently truncated prompt produces confident but incomplete notes -- see `.claude/DECISIONS.md` ID-011). A 1-hour English meeting fits comfortably; raise `ollama.max_context` (default 32768 tokens) for longer meetings if you have the memory.
- Local analysis takes minutes, not seconds (measured on an M4 Pro, 24 GB: 6-27 s for a short meeting, ~2-4 minutes for a 1-hour meeting; the model uses ~14 GB of memory while loaded). 2-hour meetings exceed `qwen3:14b`'s context and are refused -- use a cloud backend for those.
- Quality: see the measured results in [docs/EVALUATION.md](docs/EVALUATION.md). For a high-stakes meeting you can always re-run it through a cloud backend explicitly.
- `ollama.base_url` defaults to `http://localhost:11434`. If you point it at another host, heimdall labels that run "REMOTE" -- the transcript then goes to that server.

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

Scan the recovery directory for orphaned transcripts from crashed sessions (or failed analyses) and re-analyze them. Accepts `--analyzer` like `record`.

### `heimdall analyze`

Re-analyze a specific recovery transcript file.

```bash
heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json
heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json --analyzer claude-code
heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json --analyzer ollama
```

### `heimdall transcribe`

Transcribe a saved audio file locally via [Whisper](https://github.com/ggml-org/whisper.cpp) -- fully offline, no API key, no cloud, no per-meeting cost. Works on any WAV, e.g. one saved by `heimdall record --save-audio`. (`record --transcriber whisper` does this automatically after the meeting.)

```bash
heimdall transcribe --file ~/.heimdall/recordings/2026-09-13T10-00-00-standup.wav
heimdall transcribe --file meeting.wav --model small --language tr

# Fully offline, end to end: local Whisper, then local analysis
heimdall transcribe --file meeting.wav && heimdall analyze --file <printed path> --analyzer ollama
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
heimdall eval --analyzer ollama                      # default qwen3:14b
heimdall eval --analyzer ollama --model qwen2.5:7b   # compare another model
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
  analyzer: api   # default backend: api, claude-code, codex, or ollama

# Optional -- only read by the matching backend; all fields have defaults.
ollama:
  base_url: http://localhost:11434
  model: qwen3:14b
  max_context: 32768   # tokens; larger fits longer meetings, uses more memory

codex:
  model: gpt-5.6-luna

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
                     | Soniox | whisper: nothing live, whisper.cpp runs on the saved audio after the meeting
Stage 4: ACCUMULATE  In-memory segments + live terminal display
Stage 5: ANALYZE     Claude API | claude CLI | codex CLI | Ollama (on-device) -- post-meeting summary, decisions, action items
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
Analysis fails   -> Write raw transcript (no summary), keep the transcript file for a retry
Deepgram fails   -> Save raw audio to recovery file
Audio fails      -> Clean exit with clear error message
Swift crashes    -> Detect within 2s, auto-restart (max 3 retries)
Network drops    -> Reconnect with exponential backoff (1s, 2s, 4s... max 30s)
Process killed   -> Recovery file written every 30s, recoverable
```

## Cost Per Meeting

**Free path**: `--transcriber whisper` + `--analyzer ollama` costs nothing per meeting -- whisper.cpp and the local model run on your Mac, and nothing is billed because nothing leaves it. The table below is the default cloud path.

heimdall sends mono audio with Deepgram diarization (ID-001). The cost table below uses stereo rates as a conservative ceiling; actual billing is roughly 50% lower at the mono rate.

| Duration | Deepgram (stereo + diarization) | Claude Haiku (summary) | Total |
|----------|--------------------------------|------------------------|-------|
| 30 min | $0.58 | $0.01 | ~$0.59 |
| 1 hour | $1.16 | $0.02 | ~$1.18 |
| 2 hours | $2.32 | $0.04 | ~$2.36 |

Deepgram offers $200 free credit -- enough for approximately 170 one-hour meetings.

`--analyzer claude-code` and `--analyzer codex` bill nothing per meeting either: they run against a Claude or ChatGPT/Codex plan you already pay for (they do consume that plan's limits -- a Codex call carries ~16K tokens of its own agent prompt, see `docs/EVALUATION.md`).

## Project Structure

```
heimdall/
  cmd/heimdall/           CLI entry point (cobra commands)
  internal/
    audio/                AudioSource interface + mic/system implementations
    mixer/                Audio resampling, format conversion, stereo interleaving
    transcriber/          Transcriber interface + Deepgram and Soniox WebSocket implementations (NewFromName factory)
    analyzer/             Analyzer interface + Claude API, Claude Code, Codex, and Ollama backends
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

By default heimdall sends meeting data to two external services. With `heimdall transcribe` + `--analyzer ollama`, it sends nothing anywhere:

| Data | Destination | Retention | Training |
|------|-------------|-----------|----------|
| Raw audio (PCM) | Deepgram (US) via WebSocket | Zero after processing | No (mip_opt_out=true) |
| Meeting transcript (`--analyzer api`, default) | Anthropic (US) via HTTPS | 7 days (API policy) | Never (API data excluded) |
| Meeting transcript (`--analyzer claude-code` / `codex`) | Anthropic / OpenAI, under your own Claude Code / Codex account | Per your plan's terms | Per your plan's settings |
| Meeting transcript (`--analyzer ollama`) | **Nowhere** -- analyzed on this machine | N/A | N/A |
| Meeting notes | Your local Obsidian vault | You control | N/A |
| Recovery files | `~/.heimdall/recovery/` (local) | Deleted after a successful meeting note; **kept** when analysis fails so you can retry -- delete them yourself if you will not | N/A |
| Raw audio recording (opt-in, `--save-audio`) | `~/.heimdall/recordings/` (local) | Until you delete it | N/A |

heimdall does not store audio or transcripts on any server it controls. API keys are stored as environment variable references, never in plaintext config files.

**Recording consent**: Meeting recording may be subject to consent laws in your jurisdiction. In the US, 12+ states require all-party consent (see the [Justia 50-state survey](https://www.justia.com/50-state-surveys/recording-of-conversations-laws/) for the current state-by-state map). In the EU, participants must generally be informed. Please ensure all meeting participants are aware that recording is active.

See [PRIVACY.md](PRIVACY.md) for the full data-flow and biometric-data notice.

## License

MIT License. See [LICENSE](LICENSE) for details.
