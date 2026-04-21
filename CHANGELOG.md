# Changelog

## v0.1.0 (Unreleased)

### Added
- First-run recording-consent banner with persistent acknowledgement, plus `--consent-acknowledged` flag for non-interactive scripts/CI (PR #24).
- `heimdall doctor` Screen Recording permission preflight via `audio-helper --check-permissions` (PR #24).
- PRIVACY.md with data-flow table and BIPA Illinois notice; SECURITY.md vulnerability disclosure policy; AD-011 ratifying Option A strategy (PR #23).
- `/sprint` skill for quick workplan status (PR #22).
- Meeting profiles feature -- define per-meeting-type defaults (language, keywords, participants, output formatting) and select with `--profile` (PR #14)
- `heimdall config init` -- interactive setup wizard (PR #14)
- `heimdall config show` -- print the resolved config (PR #14)
- `heimdall config edit` -- open the config file in `$EDITOR` (PR #14)
- `heimdall config path` -- print the config file path (PR #14)
- `heimdall config add-profile` -- add a new profile (PR #14)
- `heimdall config profiles` -- list defined profiles (PR #14)
- `--profile <name>` flag on `heimdall record` to select a profile at recording time (PR #14)

### Changed
- `Config.Save` now uses atomic temp+rename (V-006 pattern) to avoid partial writes on crash (PR #24).
- Transcription runtime switched from multichannel to mono + diarization (PR #11). Deepgram now receives `channels=1` with `diarize=true`; speaker IDs are assigned by voice fingerprint. The mixer still produces stereo internally; `session.go` downmixes to mono before handing off to the transcriber. See `.claude/DECISIONS.md` ID-001 (supersedes AD-007).
- Config validation errors surface clearly to the user instead of generic wrapped errors (PR #10).
- User-facing guardrails added on all failure paths (PR #12, follow-ups in PR #13) -- missing API keys, vault path not found, permission denied, and subprocess crashes all produce actionable error messages instead of stack traces.
- Documentation refresh -- architecture docs, reading lists, and cross-references updated to match the shipped codebase (PR #9).

### Security
- Mask API keys in `config set` / `config get` / `config show` output so credentials no longer echo to the terminal or shell history (SEC-01, SEC-02) (PR #21).
- Lock `mip_opt_out=true` invariant on the Deepgram WebSocket URL via test assertions so a regression cannot silently re-enable model-training retention (PR #21).

### Fixed
- Config validation -- invalid YAML, missing required fields, and bad type coercion now fail fast with specific line/field errors rather than silent defaults (PR #10).

## v0.1.0-rc baseline (2026-03-31)

> Historical snapshot: this content documents the v1.0-scope work completed through PR #8. Retained for reference; the v0.1.0 release aggregates this baseline with the PR #9-14 changes above.

### Features
- Live meeting recording with dual-channel audio capture (system + microphone)
- Real-time transcription via Deepgram Nova-3 with speaker diarization
- Post-meeting analysis via Claude with structured output
- Obsidian vault integration with YAML frontmatter and wikilinks
- Turkish and multi-language support
- Crash recovery with automatic transcript preservation
- `heimdall record` -- full pipeline recording command
- `heimdall doctor` -- prerequisite checker
- `heimdall list` -- past meeting browser
- `heimdall recover` -- scan and re-analyze orphaned recovery transcripts
- `heimdall analyze --file <path>` -- re-analyze a specific transcript file
- `heimdall config init` (pre-PR#14 basic wizard), `get`, `set` -- configuration management commands
- `heimdall version` -- version info

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
- 6-stage pipeline: Capture -> Mix -> Transcribe -> Accumulate -> Analyze -> Render
- Go + Swift dual-binary (heimdall + heimdall-audio)
- Provider abstraction: swappable AudioSource, Transcriber, Analyzer, Writer interfaces
- V-001: Deepgram WebSocket proactive reconnection at 55 minutes
- V-002: Swift subprocess crash detection within 2 seconds with auto-restart
- V-005: Network disruption recovery with exponential backoff
- V-006: Crash recovery via periodic atomic writes
- V-009: Claude API retry with raw transcript fallback
