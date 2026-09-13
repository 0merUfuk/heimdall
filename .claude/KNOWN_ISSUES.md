**Version**: 3.0
**Created**: 2026-03-28
**Last Updated**: 2026-09-13
**Authors:** Omer Ufuk

---

# Heimdall -- Known Issues

> Full vulnerability assessment (28 findings, historical): `docs/architecture/ASSESSMENT.md`
> Grill report (20-agent audit, historical): `docs/GRILL_REPORT.md`
> Live PR/branch state: `.claude/SERVICE_CONTEXT.md`

---

## Open

- **CI has been red on `main` since 2026-07-19** -- `govulncheck` flags 3 Go stdlib CVEs disclosed against go1.25.10 after it was pinned. Fix is ready in PR #29 (toolchain bump to 1.27.1) but not yet merged (see SERVICE_CONTEXT.md -- merge is an owner action, blocked by a harness permission classifier this session did not attempt to route around).
- **No v0.1.0 tag exists** despite the v1.0 feature set being complete since 2026-04-22ish. Pre-tag gates are owner actions: real-voice smoke test, macOS signing decision, and now also merging PRs #29-32 (which include the CI fix -- tagging red-CI code would be worse than not tagging).
- **`make lint` (`golangci-lint`) is never run in CI** -- only `make build`, `go test -race`, and `govulncheck` are wired into `.github/workflows/ci.yml`. Found 2026-09-13; not yet fixed. Pre-existing `gofmt` drift exists in `cmd/heimdall/helpers_test.go`, `cmd/heimdall/secrets.go`, `internal/mixer/mixer.go`, `internal/transcriber/soniox.go` -- none were touched by the 2026-09-13 PRs so they were left alone rather than scope-crept into unrelated changes.
- **Token usage is parsed but discarded** -- `ClaudeAnalyzer.callAPI` (`internal/analyzer/claude.go`) unmarshals `usage.input_tokens`/`usage.output_tokens` from every Anthropic API response but never logs, stores, or surfaces them. No cost/latency observability exists for the one LLM stage in the pipeline. Found 2026-09-13; not yet fixed.
- **`heimdall eval`'s quality baseline is still theoretical** -- the suite (PR #31) is unit-tested against scripted/mock analyzers but has never been run against a real model (the build sandbox had neither `ANTHROPIC_API_KEY` nor a logged-in `claude` CLI). The first real run is the actual baseline, not the design intent documented in `docs/EVALUATION.md`.

---

## Resolved 2026-09-13 (four PRs, not yet merged to main -- see SERVICE_CONTEXT.md)

- **CI toolchain CVEs** -- go1.25.10 -> 1.27.1 (PR #29).
- **No way to analyze meetings without a separate paid API key** -- `ClaudeCodeAnalyzer` / `--analyzer claude-code` reuses an existing local Claude Code login (PR #30).
- **Zero measurement of AI-output quality** -- `internal/eval` + `heimdall eval`: golden transcripts, coverage/anti-hallucination/prompt-injection/multilingual checks, optional LLM-as-judge (PR #31).
- **`audio.save_recording` / `--save-audio` was a documented no-op** -- config field existed since the MVP spec, nothing read it. Now wired via `internal/recording` + `MeetingSession.OnAudioFrame` (PR #32).
- **No local/offline transcription path** -- `heimdall transcribe` via `internal/localstt` (whisper-cli, batch mode) (PR #32). See "Known Limitations" below for what this does and doesn't do.
- **Turkish characters stripped from crash-recovery filenames** -- `internal/recovery`'s filename sanitizer had its own copy of the pre-V-017 ASCII-only regex, never updated when `internal/output`'s was fixed. Consolidated into one shared `heimdall.SanitizeFilename` (PR #32).
- **Stale Claude-model allowlist** -- `config.Validate()` rejected the real current model (`claude-sonnet-5`) while accepting a fictitious one (`claude-sonnet-4-6`) baked into a hardcoded enum. Replaced with a shape check (PR #30).
- **Docs index (`docs/README.md`) stale since 2026-04-18** -- missing links to `PRODUCTIZATION.md`, `MONETIZATION_RESEARCH.md`, `DISTRIBUTION_RESEARCH.md`, `EVALUATION.md`, all of which already existed unlinked or were newly added (PR #31).

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
