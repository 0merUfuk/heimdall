# Changelog

## v1.0.0 (Unreleased)

### Features
- Live meeting recording with dual-channel audio capture (system + microphone)
- Real-time transcription via Deepgram Nova-3 with speaker diarization
- Post-meeting analysis via Claude with structured output
- Obsidian vault integration with YAML frontmatter and wikilinks
- Turkish and multi-language support
- Crash recovery with automatic transcript preservation
- `heimdall record` — full pipeline recording command
- `heimdall doctor` — prerequisite checker
- `heimdall list` — past meeting browser
- `heimdall recover` — scan and re-analyze orphaned recovery transcripts
- `heimdall analyze --file <path>` — re-analyze a specific transcript file
- `heimdall config init/get/set` — configuration management commands
- `heimdall version` — version info

### Security
- Config file permissions hardened from 0644 to 0600 (owner read/write only)
- Prompt injection sanitization for `--participants` and `--keywords` flags
- Directory permissions tightened from 0755 to 0700 for config and recovery directories
- Path traversal containment in Obsidian vault output writer
- Explicit file permissions (0600) in atomic recovery writes

### Architecture
- 6-stage pipeline: Capture → Mix → Transcribe → Accumulate → Analyze → Render
- Go + Swift dual-binary (heimdall + heimdall-audio)
- Provider abstraction: swappable AudioSource, Transcriber, Analyzer, Writer interfaces
- V-001: Deepgram WebSocket proactive reconnection at 55 minutes
- V-002: Swift subprocess crash detection within 2 seconds with auto-restart
- V-005: Network disruption recovery with exponential backoff
- V-006: Crash recovery via periodic atomic writes
- V-009: Claude API retry with raw transcript fallback
