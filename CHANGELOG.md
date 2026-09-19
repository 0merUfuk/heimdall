# Changelog

## Unreleased

### Added
- `--analyzer ollama` -- fully on-device meeting analysis via a local [Ollama](https://ollama.com) server (`internal/analyzer/ollama.go`). With `heimdall transcribe`, the whole path from audio to Obsidian note is offline: no audio, transcript, or analysis leaves the machine. Default model `qwen3:14b`. Uses Ollama's **native** `/api/chat`, not its OpenAI/Anthropic-compatible endpoints: those cannot set the context window per request, and Ollama was measured silently truncating a 60-minute transcript to 4,096 tokens there while still returning valid, plausible JSON with early action items missing (`.claude/DECISIONS.md` ID-011). The context window is sized per transcript; a transcript that does not fit is refused with an actionable message, never truncated. Output is schema-constrained at decode time (`format`), reasoning is disabled for bounded extraction (`think:false`). New config section `ollama:` (`base_url`, `model`, `max_context`), all optional.
- `--analyzer codex` -- analysis via a local, logged-in `codex` CLI (`codex exec`), the Codex counterpart of `claude-code`. Defaults to the cheapest reliable tier, `gpt-5.6-luna` at low reasoning effort (`codex.model` to override). Runs isolated from the user's Codex config, project docs, and repository (`--ignore-user-config --strict-config`, empty private temp dir, read-only sandbox, ephemeral session), with the system prompt passed as `developer_instructions` (ID-012).
- `heimdall record --transcriber whisper` -- fully offline meeting capture: nothing is transcribed live and nothing goes over the network; the audio is saved and transcribed on the machine by whisper.cpp after the stop, then analyzed. With `--analyzer ollama` a meeting never leaves the Mac. `whisper-cli` and the model are checked before the meeting starts (`--whisper-model`, default `base`) (ID-014).
- `heimdall eval --model <name>` to evaluate a specific model on the selected backend.
- `heimdall doctor` reports the `codex` CLI and Ollama (reachable + default model pulled); the analysis check now passes if any of the four backends is usable.
- `config set` keys: `claude.analyzer`, `ollama.base_url`, `ollama.model`, `ollama.max_context`, `codex.model`.
- Codex developer setup: project `.codex/config.toml` (cost-efficient model defaults for this repo only) and per-role `model` / `model_reasoning_effort` / `sandbox_mode` in `.codex/agents/*.toml`; `AGENTS.md` rewritten against current reality (ID-012).
- Cloud environments: `scripts/cloud-setup.sh` for Codex cloud and Claude Code on the web (installs the go.mod Go version, a C compiler for cgo, pre-builds everything), wired into Claude Code on the web through a `.claude/settings.json` SessionStart hook. The full test suite passes on Linux under `-race` (ID-013).

### Changed
- The analysis prompt names the output language ("Turkish (tr)") instead of passing the bare code (measured on `qwen3:14b`: the bare code produced English summaries of Turkish meetings), and `--language multi` now asks for the meeting's main language (measured on `claude-haiku-4-5`: a code-switched Turkish meeting went from 0/3 to 3/3 Turkish summaries). The language value is now sanitized like `--participants`/`--keywords`.
- `recover`, `analyze`, and `eval` honor `claude.analyzer` from config when `--analyzer` is not passed (previously only `record` did, although README documented it for all of them).
- Analysis timeouts are per backend: API and `claude-code` keep 120s; `ollama` gets 20 minutes and `codex` 10 minutes. `eval`'s per-fixture budget scales the same way.
- `eval` passes the configured model for the selected backend (previously `claude-code` evals ignored `claude.model`).
- `analyzer.NewFromName` takes an `analyzer.Settings` struct instead of a bare API key.

### Fixed
- `--analyzer claude-code` could never use a Claude subscription login: it passed `--bare`, under which Claude Code reads only `ANTHROPIC_API_KEY`/`apiKeyHelper`, never OAuth. `--bare` is gone; the isolation it provided (no hooks, plugins, CLAUDE.md, auto-memory, MCP servers, or saved session in a call that carries a meeting transcript) is rebuilt from `--restricted`, `--strict-mcp-config`, `--no-session-persistence`, `--disable-slash-commands`, an empty temp working directory, and three `CLAUDE_CODE_DISABLE_*` variables -- verified against a live model with planted hooks and a planted CLAUDE.md (ID-015).
- A crash mid-recording left the `--save-audio` WAV with a header claiming zero bytes of audio (the size was only written on close); the header is now checkpointed every 30 seconds. A recording that fails to start no longer leaves an empty WAV behind.
- A failed analysis no longer deletes the only re-analyzable copy of the meeting: `record`, `recover`, and `analyze` used to delete the recovery transcript after writing a *fallback* (raw-transcript) note. The file is now kept and the exact retry command is printed. heimdall never falls back from a local backend to a cloud one on its own.
- `heimdall config set claude.analyzer ...`, documented in README, returned "unknown config key".
- `${VAR}` references in config stopped resolving at the first unset variable: without `DEEPGRAM_API_KEY` (the first field, and typically unset on the fully local path) every later reference, e.g. `ollama.base_url: ${...}`, was silently left unresolved. All resolvable fields are now resolved and every unset variable is reported.

## v0.1.0 (2026-09-14)

### Added
- `.github/workflows/release.yml` -- pushing a `v*` tag now runs GoReleaser on a macOS runner and publishes a GitHub Release, with archives, checksums, and an auto-generated changelog. `.goreleaser.yml` gained a `homebrew_casks` block that pushes an updated cask to a `homebrew-heimdall` tap on every release (PR #38). See `docs/RELEASING.md` for the one-time setup this depends on and the current code-signing gap.
- Analysis token usage and latency are now logged after every successful Claude API call (`internal/analyzer/claude.go`): `analyzer: model=... input_tokens=... output_tokens=... latency=...`. The response already carried this data (`apiResponse.Usage`) but it was parsed and silently discarded. Deliberately logs raw counts, not a computed dollar estimate -- this project's own history (AD-002) already shows a hardcoded price figure going stale and silently wrong once; README's "Cost Per Meeting" table remains the one place a $ estimate lives, kept current by hand rather than duplicated in code.
- `heimdall mcp` -- a local MCP (Model Context Protocol) server over stdio, exposing the Obsidian vault as three read-only tools (`list_meetings`, `search_meetings`, `get_meeting`) to Claude Desktop, Claude Code, Cursor, or any MCP client (STRATEGY_V2 Phase 2C; PRODUCTIZATION.md v3's stated core differentiator, the "Obsidian knowledge graph two-way bridge"). Built on the official `github.com/modelcontextprotocol/go-sdk`. New `internal/vault` package reads meeting notes back out of the vault -- the inverse of `internal/output`, which only writes them. Tool-level failures (vault not configured, bad input, nothing found) are reported via `CallToolResult.IsError` per the SDK's own documented contract, not as Go errors, so a calling model can see what went wrong and react -- verified with a real client connected over the SDK's in-memory transport, exercising the actual JSON-RPC protocol rather than only the handler functions in isolation. Also smoke-tested over the real stdio transport with a hand-built JSON-RPC request sequence, which caught two bugs neither the in-memory nor unit tests could: `heimdall mcp` originally refused to start without `DEEPGRAM_API_KEY`/`ANTHROPIC_API_KEY` set (it resolved the *entire* config instead of just `obsidian.vault_path`, which is all it ever reads), and a normal client disconnect (stdin closing) was reported as a fatal `Error: server is closing: EOF` with exit code 1 -- every ordinary end of an MCP session would have looked like a crash to a client's process supervision. Adds `github.com/modelcontextprotocol/go-sdk` as a new direct dependency (and its own transitive tree: jsonschema-go, segmentio/encoding, uritemplate, oauth2, x/sync, x/time) -- a real increase in this project's previously very lean dependency surface, taken deliberately: MCP's JSON-RPC/stdio protocol is intricate enough that hand-rolling it (as this project does for Deepgram's WebSocket protocol and the Anthropic REST API) would trade a one-time integration cost for an ongoing protocol-correctness liability, with no testability upside the way hand-rolling has for the simpler HTTP/WebSocket cases.
- CI now runs `go vet`, a `gofmt -l` check, and `golangci-lint` (with `errcheck` deliberately excluded -- see the workflow file's comment) on every push/PR. `make lint` mirrors the same three checks locally. Previously `make lint` existed as a Makefile target but nothing ever invoked it in CI.
- `heimdall transcribe` -- fully offline, local speech-to-text via a new `internal/localstt` package wrapping the `whisper-cli` binary (`brew install whisper-cpp`) in batch mode. Pairs with `--save-audio`: record without any cloud transcription provider, transcribe later on your own schedule, no API key, no per-meeting cost. Always passes whisper.cpp's built-in `--diarize` flag (verified empirically against 1.9.4 -- see `.claude/DECISIONS.md` ID-008), giving real (if coarse, two-party) system-audio-vs-microphone speaker separation for free. Writes output in the same crash-recovery JSON format `heimdall record` uses, so `heimdall analyze --file <path>` picks it up directly with zero new glue code. New `heimdall model download <size>` command fetches ggml models to `~/.heimdall/models/`. `heimdall doctor` reports `whisper-cli`/model availability (optional -- Deepgram/Soniox remain the default transcription path).
- `--save-audio` flag / `audio.save_recording` config now actually works. This config field has existed since the initial config schema (and is documented in README's example) but was never wired to anything -- `heimdall record` silently ignored it. Now writes the mixed meeting audio as a 16kHz stereo WAV (`internal/recording`) to `audio.recording_path` (default `~/.heimdall/recordings/`), tapped from the session pipeline pre-downmix via a new `MeetingSession.OnAudioFrame` hook. Graceful-degradation backup: if the transcriber and the analyzer both fail, the raw audio still survives.
- `heimdall eval` -- a meeting-analysis quality suite (`internal/eval`) that runs seven golden transcripts through the configured Analyzer and checks coverage (decisions/action items weren't dropped), anti-hallucination (nothing invented for a transcript with nothing to extract, no names untraceable to the transcript), prompt-injection resistance (V-014), and multilingual consistency (Turkish, TR+EN code-switching). `--judge` adds optional LLM-as-judge faithfulness/coverage scoring (extra API cost, never gates the exit code); `--json` for machine-readable output. See `docs/EVALUATION.md` for the full methodology and its one known limitation (not yet run against a live model -- see that doc).
- `ClaudeCodeAnalyzer` -- a second `Analyzer` backend that shells out to a local, already-authenticated `claude` CLI (`claude -p --bare --restricted --permission-prompts none`) instead of calling the Anthropic API directly. Select it with `--analyzer claude-code` on `record`/`recover`/`analyze`, or persist it via `claude.analyzer: claude-code` in config. Lets a user with an existing Claude subscription (Pro/Max/Team) analyze meetings without a separate, pay-per-token `ANTHROPIC_API_KEY`. `heimdall doctor` now reports whether the `claude` CLI is available and only fails the analysis check if *neither* backend is usable. Shares all retry/backoff/fallback policy (V-009) and the anti-hallucination + prompt-injection-mitigated system prompt with the existing API backend via a new `summarizeWithRetry` helper -- both backends degrade identically on failure.
- SonioxTranscriber scaffolding behind the existing `Transcriber` interface, with `SonioxConfig` (env-var resolution + secret masking) and the `transcriber.NewFromName(name, dgCfg, snxCfg)` provider factory. Deepgram remains the default; opt-in via the new `--transcriber soniox` flag on `heimdall record` once `SONIOX_API_KEY` is configured. `heimdall doctor` now reports Soniox API-key status (`[pass]` when set, `[info]` when unset — opt-in, so absence is not a failure). Validation spike (TR+EN WER + streaming latency measurement) is a separate manual operator task per AD-011 Phase 2 (PRs #26, #27).
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
- `make audio-helper-universal` (the release build path for the bundled Swift helper) silently shipped a **single-architecture binary instead of the intended universal arm64+x86_64 one**. It looked for the built binary at `audio-helper/.build/apple/Products/Release/heimdall-audio`, but the current Swift toolchain writes it to `.build/out/Products/Release/heimdall-audio` -- the `cp` step was failing outright (`No such file or directory`). Never caught because no release had ever been run end-to-end via GoReleaser until this was found doing exactly that. Fixed by locating the binary with `find` instead of a hardcoded path, with a second check that fails loudly if the result isn't actually a 2-architecture Mach-O (an earlier, looser version of the `find` pattern intermittently matched a single-arch intermediate object file instead of the real product -- caught by this same check, not by inspection). Verified with three consecutive clean rebuilds and a full local `goreleaser release --snapshot --clean` producing correct universal binaries in the resulting archives.
- Cleared the 60 pre-existing `golangci-lint` findings not already covered by an accepted-exception entry: 8 `staticcheck` issues (capitalized error strings, redundant type declarations, `WriteString(fmt.Sprintf(...))` instead of `fmt.Fprintf`) and 2 genuinely-dead constants in `internal/mixer` (`systemSamplesPerFrame`, `sourceReadTimeout` -- unlike `ring_buffer.go`, which stays as documented accepted debt, these had no corresponding logic anywhere and no rationale to keep). The remaining 50 findings were all `errcheck` on `Close()`/`Remove()` in cleanup paths -- the same already-accepted pattern as gosec G104, not new problems.
- gofmt drift across `cmd/heimdall/helpers_test.go`, `cmd/heimdall/secrets.go`, `internal/analyzer/claude.go`, `internal/mixer/mixer.go`, `internal/session/session.go`, `internal/session/session_integration_test.go`, `internal/transcriber/soniox.go`.
- `internal/recovery`'s crash-recovery filenames stripped Turkish (and every other non-ASCII) character from meeting titles via a `[^a-zA-Z0-9]+` regex, even though `internal/output`'s Obsidian-note filenames were fixed for this back in the 31-bug sweep (V-017: `[^\p{L}\p{N}-]+`). The two copies had drifted -- a Turkish meeting title produced a correct `.md` filename but a mangled `.json` recovery filename. Consolidated into one shared `heimdall.SanitizeFilename`; both packages now call it.
- `config.Validate()` rejected the real current flagship model (`claude-sonnet-5`) while accepting a fictitious one (`claude-sonnet-4-6`) that had drifted into the hardcoded allowlist. Replaced the exact-enum check with a `"claude-..."` shape check so the validator stops going stale every time Anthropic ships a new model name -- this project's own history shows that allowlist going wrong twice already.
- CI had been red on `main` since 2026-07-19: `govulncheck` flagged 3 Go stdlib CVEs (GO-2026-5856, GO-2026-5039, GO-2026-5037) disclosed against go1.25.10 after it was pinned. Bumped the toolchain to go1.25.10 -> 1.27.1 (go.mod + CI workflow); Go only backports security fixes to the latest two majors, so this moves onto current stable rather than chasing 1.25.x patches.
- Soniox: log dropped transcript segments (speaker, timestamp, truncated text) instead of discarding them silently when the output channel is full; the drop stays non-blocking but is now observable (PR #27).
- Soniox: scope the `--language` config fallback to the active provider so a `deepgram.language` config value no longer bleeds into a `--transcriber soniox` session (and vice versa) (PR #27).
- Soniox: disable `enable_language_identification` for monolingual sessions; the flag is only set for the multi/auto TR+EN code-switched path (PR #27).
- Soniox: close the segments channel when the server sends `finished:true` without a client `Close()`, so downstream `Receive()` consumers no longer block indefinitely (PR #27).
- Soniox: advance `timeOffset` on segment emission so post-reconnect token timestamps continue monotonically instead of colliding with the pre-reconnect transcript (PR #27).
- Race condition in `segmentProducingTranscriber` test mock that caused intermittent `send on closed channel` panics under `-race` (`internal/session/session_integration_test.go`).
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
