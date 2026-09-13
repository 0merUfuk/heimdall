**Version**: 4.0
**Created**: 2026-03-28
**Last Updated**: 2026-09-14
**Authors:** Omer Ufuk

---

# Heimdall -- Known Issues

> Full vulnerability assessment (28 findings, historical): `docs/architecture/ASSESSMENT.md`
> Grill report (20-agent audit, historical): `docs/GRILL_REPORT.md`
> Live PR/branch state: `.claude/SERVICE_CONTEXT.md`

---

## Open

- **Homebrew tap repo/token not yet created** -- `.goreleaser.yml`'s `homebrew_casks` config and `.github/workflows/release.yml` are live and v0.1.0's release ran successfully, but `github.com/0merUfuk/homebrew-heimdall` doesn't exist yet and no `HOMEBREW_TAP_TOKEN` secret is set. The release workflow correctly no-ops this step rather than failing (see "Resolved" below) -- but the tap itself is still an owner action (`docs/RELEASING.md` has the steps).
- **`heimdall eval`'s quality baseline is still theoretical** -- the suite correctly detects and fails closed on degraded/fallback output (verified: 0/7 with the right root cause reported) but has never been run against a real model -- no sandbox credential was available this round either. The first real run is the actual baseline, not the design intent documented in `docs/EVALUATION.md`.
- **No real live-meeting validation** -- everything through Whisper transcription + Claude analysis + Obsidian rendering was verified with a synthetic 2-speaker recording (real `say`+`ffmpeg` audio, real `whisper-cli`, real `claude` CLI graceful-degradation path) and a real downloaded-and-run release artifact, but an actual live meeting is the one thing that needs the owner physically present.
- **macOS code-signing decision not made** -- binaries are unsigned/unnotarized; `docs/RELEASING.md` documents the `xattr` postflight-hook workaround in the interim. Needs an Apple Developer account, which only the owner can provision.

---

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
| `ClaudeCodeAnalyzer` (PR #30) has never been run against a real logged-in `claude` CLI | Build sandbox had none. Unit-tested via an injectable subprocess runner against the real CLI's empirically-captured JSON contract; a live smoke test is an owner action. |

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
