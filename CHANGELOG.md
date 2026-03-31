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

### Bug Fixes (31-bug sweep, PR #8)
- Fixed root cause of empty transcription: `diarize=true` and `multichannel=true` conflict
- Deepgram error responses now logged (were silently discarded)
- Config `ResolveEnvVars()` now called (env var paths were used literally)
- System audio permission denied now surfaces as a visible warning
- Recovery file no longer re-created after cleanup on clean exit
- `--language multi` correctly maps to Deepgram `detect_language=true`
- Removed dead `--app` flag (was accepted but never wired)
- Removed dead ring buffer code from mixer
- Fixed 5-second Ctrl+C shutdown delay (Close before cancel)
- Fixed `Close()` hang at 55-minute reconnection boundary
- Fixed concurrent write race in proactive reconnection
- keepAlive loop restarted after reconnection (was permanently lost)
- Channel index used as speaker ID in multichannel mode
- Turkish characters preserved in filenames (Unicode-aware regex)
- Claude analysis language-aware for non-English meetings
- Doctor now validates macOS >= 14.2 (was cosmetic check)
- Processed recovery files deleted after successful re-analysis
- Duration captured before shutdown (was inflated by stop time)
- Fallback stdout output when Obsidian writer fails
- `mip_opt_out=true` sent to Deepgram (privacy opt-out)

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
