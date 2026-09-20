**Version**: 4.1
**Created**: 2026-03-28
**Last Updated**: 2026-09-19
**Authors:** Omer Ufuk

---

# Heimdall -- Known Issues

> Full vulnerability assessment (28 findings, historical): `docs/architecture/ASSESSMENT.md`
> Grill report (20-agent audit, historical): `docs/GRILL_REPORT.md`
> Live PR/branch state: `.claude/SERVICE_CONTEXT.md`

---

## Open

- **Codex always loads the user's global `~/.codex/AGENTS.md` into `--analyzer codex` calls** -- verified 2026-09-19 with a canary: `project_doc_max_bytes=0` stops project docs only, and no config key or feature flag disables the global file (`instructions=""` was tested and does not; it strips Codex's own base prompt instead). Measured impact: ~3.5K extra input tokens per call; output stays schema-valid, and the event stream shows no tool calls. The only full fix is a separate `CODEX_HOME`, which would mean relocating the user's Codex credentials (token-refresh risk), so it is not done. Keep personal instructions out of that file, or use another backend, if that matters.
- **`qwen3:14b` fails 2 of 7 eval fixtures, both Turkish** (deterministic across runs at temperature 0): `tr-standup` attributes owners as "Speaker N" instead of the required "Unknown Speaker N" (instruction-following, not hallucination), and `tr-en-code-switch` (`--language multi`) summarizes a Turkish meeting with English technical terms in English. Naming the language in the prompt fixed the explicit `--language tr` case, and the `multi` instruction fixed code-switching for Haiku but not for `qwen3:14b`. For Turkish meetings, prefer `--language tr` over `multi` with the local backend, or a cloud backend. See `docs/EVALUATION.md`.
- **Whisper `small` is not enough for Turkish or jargon-heavy meetings** -- measured on a real 57-minute meeting: `small` produced a transcript the analyzer could extract nothing from (0 decisions, 0 action items), `medium` + `--language tr` produced 2 decisions and 4 action items from the same audio (ID-016). Defaults moved to `small`; use `--whisper-model medium` for those meetings.
- **Channel diarization collapses when the microphone hears the system output** -- if you are on speakers or leaky earbuds, both channels carry the same meeting audio at similar levels and whisper.cpp's energy-based `--diarize` attributes nearly everything to one speaker (measured: 221 of 224 segments). Use closed headphones, or a cloud transcriber for per-person speakers (ID-016).
- **Live capture needs macOS privacy permissions** -- Microphone and "Screen & System Audio Recording" for whichever app runs `heimdall record` (Terminal, or the app that launches it). Verified 2026-09-19: without them the helper reports `screen-recording-permission: denied` and the microphone open fails or blocks at a pending permission prompt. Only the user can grant them.
- **`--analyzer claude-code` subscription (OAuth) path not exercised end to end** -- the `--bare` auth bug is fixed (ID-015) and the backend passes 7/7 through the real binary, but with API-key auth: this machine's `claude` CLI was not logged in. First `claude /login` user confirms it.
- **Local recall vs. cloud baseline is 83% (5/6 planted action items)**, one item under the brief's 85% bar; the miss is mid-meeting in a 1-hour English transcript. See `docs/EVALUATION.md`.
- **Ollama only; LM Studio unsupported** -- deliberate (ID-011): OpenAI-compatible endpoints cannot set the context window per request.
- **Transcripts longer than the local context window are refused, not chunked** -- map-reduce deliberately deferred (ID-011). Default `ollama.max_context` 32768 covers ~1 hour of English speech; raise it (qwen3:14b's maximum is 40960) or use a cloud backend for longer meetings.
- **Codex model slugs will age** -- `gpt-5.6-luna`/`gpt-5.6-terra` come from Codex's local model catalog as of 2026-09-19; update `internal/analyzer/codex.go`, `.codex/config.toml`, `.codex/agents/*.toml`, and `AGENTS.md` together when Codex retires them.
- **The committed SessionStart hook auto-runs `scripts/cloud-setup.sh` in cloud sessions** (root, inside the container). Benign today and logged step by step, but changes to that script or `.claude/settings.json` must be reviewed like a CI workflow file (security review, 2026-09-20).
- **`scripts/cloud-setup.sh` repoints stale Go binaries** in the container (e.g. `/usr/local/go/bin/go` in `golang:1.24`) -- intentional and Linux-only, but surprising if someone runs it on a long-lived Linux workstation.

- **Homebrew tap repo/token not yet created** -- `.goreleaser.yml`'s `homebrew_casks` config and `.github/workflows/release.yml` are live and v0.1.0's release ran successfully, but `github.com/0merUfuk/homebrew-heimdall` doesn't exist yet and no `HOMEBREW_TAP_TOKEN` secret is set. The release workflow correctly no-ops this step rather than failing (see "Resolved" below) -- but the tap itself is still an owner action (`docs/RELEASING.md` has the steps).
- **No real live-meeting validation** -- everything through Whisper transcription + Claude analysis + Obsidian rendering was verified with a synthetic 2-speaker recording (real `say`+`ffmpeg` audio, real `whisper-cli`, real `claude` CLI graceful-degradation path) and a real downloaded-and-run release artifact, but an actual live meeting is the one thing that needs the owner physically present.
- **macOS code-signing decision not made** -- binaries are unsigned/unnotarized; `docs/RELEASING.md` documents the `xattr` postflight-hook workaround in the interim. Needs an Apple Developer account, which only the owner can provision.

---

## Resolved 2026-09-19 (branch `claude/heimdall-offline-analyzer-502957`)

- **`--analyzer codex` unverified** -- live: 6/7, 6/7, 5/7 on the eval, 21/21 valid JSON (`docs/EVALUATION.md`).
- **The Ollama client followed HTTP redirects** -- a redirecting service on the configured address could have received and forwarded the transcript (Go re-sends a POST body on 307/308) while the run was labeled on-device; redirects are now refused (found by an independent Codex review).
- **A failed seek after a WAV checkpoint would have overwritten recorded audio** -- the header is now patched with a positional write that never moves the append offset (same review).

- **`--analyzer claude-code` could never use a subscription login** -- `--bare` forbids OAuth/keychain auth; replaced by verified isolation flags (ID-015).
- **No cloud baseline for the Analyze stage** -- measured: Haiku 6/7 (api), 7/7 (claude-code); see `docs/EVALUATION.md`.
- **No way to capture a meeting fully offline** -- `record --transcriber whisper` (ID-014).
- **A crash mid-recording left a WAV whose header claimed zero bytes** -- 30 s header checkpoints (ID-014).

## Resolved 2026-09-13/14 (11 PRs, #29-#41, merged to `main`; v0.1.0 tagged and released)

- **CI toolchain CVEs, twice** -- go1.25.10 -> 1.27.1 (#29). New CVEs disclosed mid-session hit 5 *other* independently-branched PRs that hadn't inherited the fix; same bump applied directly to each.
- **No way to analyze meetings without a separate paid API key** -- `ClaudeCodeAnalyzer` / `--analyzer claude-code` reuses an existing local Claude Code login (#30).
- **Zero measurement of AI-output quality** -- `internal/eval` + `heimdall eval`: golden transcripts, coverage/anti-hallucination/prompt-injection/multilingual checks, optional LLM-as-judge (#31).
- **`audio.save_recording` / `--save-audio` was a documented no-op** -- now wired via `internal/recording` + `MeetingSession.OnAudioFrame` (#32).
- **No local/offline transcription path** -- `heimdall transcribe` via `internal/localstt` (#32).
- **Turkish characters stripped from crash-recovery filenames** -- consolidated into one shared `heimdall.SanitizeFilename` (#32).
- **Stale Claude-model allowlist** -- replaced hardcoded enum with a shape check (#30).
- **`make lint` never run in CI** -- `go vet`/`gofmt -l`/`golangci-lint` now wired into `.github/workflows/ci.yml`; pre-existing gofmt drift across 7 files cleared (#34).
- **GoReleaser release builds had never actually succeeded** -- universal-binary path was hardcoded wrong (`.build/apple/...` vs. the real `.build/out/...`); fixed with a `find`-based lookup plus a 2-architecture verification check (#35).
- **No MCP server** -- `heimdall mcp` exposes the vault as three tools via the official SDK; a hand-built stdio smoke test caught two real bugs (config over-resolution requiring unrelated API keys to start; a clean disconnect misreported as a crash) that unit tests alone hadn't (#36).
- **Token usage parsed but discarded** -- now logged with latency on every successful API call (#37).
- **No release automation, no Homebrew integration at all** -- tag-triggered `.github/workflows/release.yml` + `homebrew_casks` in `.goreleaser.yml` (#39; #38 had accidentally merged into its own base branch instead of `main` -- caught by checking out fresh `main` and finding the files missing, not by trusting the "merged" status).
- **`skip_upload` template would have failed the entire release** the first time anyone tagged before the Homebrew tap's one-time setup -- Go's `text/template` errors on a genuinely-missing map key rather than defaulting to empty; caught by testing this exact scenario locally *before* tagging v0.1.0, fixed with `index .Env "..."` (#40).
- **No v0.1.0 tag/release existed** -- tagged and released after the above; verified by downloading the actual published artifact and running it, not just checking the Actions log.
- **Docs index (`docs/README.md`) stale since 2026-04-18** -- missing links to `PRODUCTIZATION.md`, `MONETIZATION_RESEARCH.md`, `DISTRIBUTION_RESEARCH.md`, `EVALUATION.md` (#31).

---

## Resolved in v0.1.0 release-readiness PR (2026-05-29, merged)

- `TestReconnection_*` hang / `Close()` deadlock under `-race` -- root-caused as a real production deadlock (`Close()` could hang on `wg.Wait()` during a reconnect `dial()` window). Fixed by re-checking `d.closed` after `dial()` under `d.mu`.
- 8 callable stdlib CVEs -- resolved (at the time) by bumping to go1.25.10. New CVEs against that same version are the 2026-09-13 issue above -- a reminder that a toolchain pin needs periodic re-verification, not a one-time fix.

---

## Resolved in PR #8 (31-bug sweep, 2026-03-31)

See `CHANGELOG.md` for the full list. Key fixes: root cause of empty transcription (diarize+multichannel conflict), Deepgram error visibility, 5-second Ctrl+C delay, recovery file lifecycle.

---

## Known Limitations (Accept + Document)

These are documented trade-offs, not bugs:

| Limitation | Mitigation |
|-----------|-----------|
| Speaker ID resets on WebSocket reconnection (Deepgram/Soniox) | Diarize mode uses provider-assigned speaker IDs per connection; IDs may differ across reconnections. ID-001 documents the mono+diarize runtime path. |
| `heimdall transcribe` (local Whisper) only separates system-audio vs. microphone (2 parties), not per-individual speakers | whisper.cpp's built-in `--diarize` is channel-based, not voice-fingerprint-based like Deepgram/Soniox. Documented in README and the command's own `--help`. See DECISIONS.md ID-008. |
| System audio requires macOS 14.2+ | Doctor validates version, clear error message |
| Deepgram Turkish code-switching not fully supported | `--keywords` flows through to Deepgram's keyword-boost parameter, improving English-tech-term recognition in Turkish meetings, but doesn't fully resolve TR+EN code-switching |
| LLM may hallucinate action items | Anti-hallucination prompt engineering (V-013); now measurable via `heimdall eval`'s anti-hallucination checks (PR #31) |
| Transcript content as prompt injection vector | Delimiter wrapping + sanitized --participants/--keywords (V-014); now measurable via `heimdall eval`'s injection-resistance check (PR #31) |
| `ClaudeCodeAnalyzer` (PR #30) had never been run against a real `claude` CLI (superseded 2026-09-19: now 7/7 on the eval through the real binary, ID-015; an OAuth/subscription login is still unexercised) | Build sandbox had none. Unit-tested via an injectable subprocess runner against the real CLI's empirically-captured JSON contract; a live smoke test is an owner action. |

---

## Accepted Technical Debt

| Item | Severity | Notes |
|------|----------|-------|
| `ring_buffer.go` exists but is unused | Low | Kept as reusable type for future V-005 reconnection buffering |
| Config CLI uses simple text prompts, not TUI | Low | charmbracelet/huh TUI deferred |
| Audio package test coverage ~62% | Low | Hardware-dependent code hard to unit test |
| Claude analyzer uses raw HTTP, not official SDK | Low | Better testability via httptest; same rationale does NOT apply to `heimdall eval`'s judge, which is also raw HTTP for the same reason (see DECISIONS.md ID-006) |
| gorilla/websocket in maintenance mode | Low | Stable, consider migrating to coder/websocket in v2+ |
| `.claude/`, `.codex/`, `.hermes/` multi-runtime agent scaffolding is large relative to product code | Medium | Flagged by `docs/GRILL_REPORT.md`'s "Meta-Engineering ROI" finding (2026-03-29) and STRATEGY_V2's "What We're Cutting" section (2026-03-30) as likely over-scoped; never acted on, and the `.codex`/`.hermes` trees were added afterward (2026-07-21), suggesting the scope may be intentional now rather than leftover. Worth an owner conversation, not a unilateral deletion. |

---

## Accepted gosec Findings

| Rule | Finding | Rationale |
|------|---------|-----------|
| G101 | Env var templates in defaults.go | False positive: `${DEEPGRAM_API_KEY}` is a template |
| G115 | Integer overflow in microphone.go/resample.go | Values are constants or intentional PCM conversions |
| G104 | Unhandled Close/Remove in error paths | Standard Go cleanup pattern |

> Not re-run in this session (no `gosec` invocation) -- these are carried forward from the last verified pass. Re-verify before the next security review.
