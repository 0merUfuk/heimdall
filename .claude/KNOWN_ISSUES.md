**Version**: 2.2
**Created**: 2026-03-28
**Last Updated**: 2026-05-29
**Authors:** Omer Ufuk

---

# Heimdall -- Known Issues

> Full vulnerability assessment (28 findings): `docs/architecture/ASSESSMENT.md`
> Grill report (20-agent audit): `docs/GRILL_REPORT.md`

---

## Open

No open blockers for v0.1.0. (Remaining pre-tag gates are owner actions: real-voice smoke test + macOS signing decision — see NEXT_STEPS.)

---

## Resolved in v0.1.0 release-readiness PR (2026-05-29)

- **`TestReconnection_*` hang / `Close()` deadlock under `-race`** — root-caused as a real production deadlock, not a test flake: `Close()` could hang on `wg.Wait()` if it ran during a reconnect `dial()` window, because the reconnect re-acquired `d.mu`, resurrected `d.conn`, and spawned a `readLoop` whose socket was never closed. Fixed by re-checking `d.closed` after `dial()` under `d.mu` and moving `wg.Add` into that locked section (clean happens-before with `Close()`'s `d.closed = true`). `TestReconnection_SendDuringReconnect` re-enabled. Verified 15/15 green under `-race`; full `make test` now green across all 11 packages.
- **8 callable stdlib CVEs** — resolved by bumping the toolchain to `go1.25.10`; `govulncheck` reports 0 callable vulnerabilities.

---

## Resolved in PR #8 (31-bug sweep)

All critical and warning bugs identified in the comprehensive audit have been fixed. See `CHANGELOG.md` for the full list.

Key fixes:
- Root cause of empty transcription (diarize+multichannel conflict)
- Deepgram error visibility (was completely silent)
- System audio permission denied now surfaces warning
- 5-second Ctrl+C delay eliminated
- Ring buffer dead code removed
- Recovery file lifecycle fixed

---

## Known Limitations (Accept + Document)

These are documented trade-offs, not bugs:

| Limitation | Mitigation |
|-----------|-----------|
| Speaker ID resets on WebSocket reconnection | Diarize mode uses Deepgram-assigned speaker IDs per connection; IDs may differ across reconnections. ID-001 documents the mono+diarize runtime path. |
| No local ASR fallback in v1.0 | Planned for v2.0 (Whisper.cpp) |
| System audio requires macOS 14.2+ | Doctor validates version, clear error message |
| Deepgram Turkish code-switching not supported | `--keywords` flag now flows through to Deepgram's keyword-boost parameter on the WebSocket URL (per PR #16), improving recognition of English tech terms in Turkish meetings. Does not fully resolve TR+EN code-switching. |
| LLM may hallucinate action items | Anti-hallucination prompt engineering (V-013) |
| Transcript content as prompt injection vector | Delimiter wrapping + sanitized --participants/--keywords (V-014) |

---

## Accepted Technical Debt

| Item | Severity | Notes |
|------|----------|-------|
| `ring_buffer.go` exists but is unused | Low | Kept as reusable type for future V-005 reconnection buffering |
| Config CLI uses simple text prompts, not TUI | Low | charmbracelet/huh TUI deferred |
| Audio package test coverage ~62% | Low | Hardware-dependent code hard to unit test |
| Claude analyzer uses raw HTTP, not official SDK | Low | Better testability via httptest |
| gorilla/websocket in maintenance mode | Low | Stable, consider migrating to coder/websocket in v2+ |

---

## Accepted gosec Findings

| Rule | Finding | Rationale |
|------|---------|-----------|
| G101 | Env var templates in defaults.go | False positive: `${DEEPGRAM_API_KEY}` is a template |
| G115 | Integer overflow in microphone.go/resample.go | Values are constants or intentional PCM conversions |
| G104 | Unhandled Close/Remove in error paths | Standard Go cleanup pattern |
