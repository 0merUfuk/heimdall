**Version**: 2.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-31
**Authors:** Omer Ufuk

---

# Heimdall -- Known Issues

> Full vulnerability assessment (28 findings): `docs/architecture/ASSESSMENT.md`
> Grill report (20-agent audit): `docs/GRILL_REPORT.md`

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
| Speaker ID resets on WebSocket reconnection | Channel index used as speaker ID in multichannel mode (AD-008) |
| No local ASR fallback in v1.0 | Planned for v2.0 (Whisper.cpp) |
| System audio requires macOS 14.2+ | Doctor validates version, clear error message |
| Deepgram Turkish code-switching not supported | `--keywords` flag for English tech terms in Turkish meetings |
| LLM may hallucinate action items | Anti-hallucination prompt engineering (V-013) |
| Transcript content as prompt injection vector | Delimiter wrapping + sanitized --participants/--keywords (V-014) |
| Stereo billing doubles Deepgram cost | Documented in README cost table |

---

## Accepted Technical Debt

| Item | Severity | Notes |
|------|----------|-------|
| `ring_buffer.go` exists but is unused | Low | Kept as reusable type for future V-005 reconnection buffering |
| `go.mod` says `go 1.25.6` (doesn't exist) | Low | Works with current toolchain, cosmetic issue |
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
