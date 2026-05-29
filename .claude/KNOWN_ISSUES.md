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

| Issue | Severity | Status |
|-------|----------|--------|
| `internal/transcriber` `TestReconnection_*` intermittently hangs under `-race` | Blocker (test flake; blocks a green v0.1.0 test run) | In-flight fix on branch `fix/transcriber-reconnect-hang` (discovered 2026-05-29) |

The reconnection tests (`internal/transcriber/reconnection_test.go`) do not deterministically complete under the race detector — a run can hang rather than fail. `make test` is therefore not reliably green until the fix lands. The hang is in the test/transcriber interaction at the proactive-reconnection boundary (V-001), not in shipped record/analyze paths. Treat `make test` as not-yet-green when reporting v0.1.0 readiness.

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
