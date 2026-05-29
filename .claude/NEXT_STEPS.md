**Version**: 3.1
**Created**: 2026-03-28
**Last Updated**: 2026-05-29
**Authors:** Omer Ufuk

---

# Heimdall -- Next Steps

> Original execution plan: `docs/MASTER_PLAN.md` (18/19 complete)
> Live work tracker: `tasks/todo.md` (canonical for the v0.1.0 finishing push)
> Strategic plan: `docs/STRATEGY_V2.md` (post-grill)

---

## Status (2026-05-29)

Development resumed 2026-05-29 after a dormant stretch. The 2026-05-16 v0.1.0 tag target was missed. Since the last NEXT_STEPS refresh, the Soniox transcriber landed as a Phase 2 spike (PR #26 implementation + flag + doctor + factory, PR #27 five correctness fixes) — Deepgram remains the default. A v0.1.0 finishing push is now underway, tracked in `tasks/todo.md`.

**Release blocker**: `internal/transcriber` `TestReconnection_*` intermittently hangs under `-race`, so `make test` is not reliably green. Fix is in-flight on branch `fix/transcriber-reconnect-hang` (see KNOWN_ISSUES). v0.1.0 should not be tagged until this is resolved and the suite passes cleanly.

## v1.0 — FEATURE-COMPLETE (18/19 subtasks)

All 18 completed MASTER_PLAN subtasks executed. Security review passed (PR #6). Final review passed (PR #7). Bug sweep merged (PR #8). Subsequent refinements: docs refresh (PR #9), config validation (PR #10), mono+diarize switch (PR #11, see ID-001), guardrails (PR #12), meeting profiles + config UX (PR #14), Soniox transcriber spike (PRs #26, #27).

## Immediate

| Task | Status |
|------|--------|
| Resolve `TestReconnection_*` race hang (`fix/transcriber-reconnect-hang`) | IN PROGRESS — blocks tag |
| Get `make test` reliably green under `-race` | BLOCKED on the above |
| Smoke test with real voices (docs/MANUAL_TESTING.md) | PENDING |
| Tag v0.1.0 release | PENDING (after green tests + smoke test; original 2026-05-16 target missed) |
| Create Homebrew tap repo | NOT STARTED |

## Phase 2 (from STRATEGY_V2.md)

| Phase | Goal | Timeline |
|-------|------|----------|
| 2A: Ship & Fix | Tag v0.1.0, Homebrew tap | Week 1 |
| 2B: Local Transcription | Whisper.cpp default, zero API keys | Weeks 2-3 |
| 2C: MCP Server | Meeting data queryable by Claude Desktop | Weeks 3-4 |
| 2D: Launch | Obsidian community, README rewrite, first 50 users | Week 5 |

## Post-v2 Roadmap

See `docs/STRATEGY_V2.md` Phase 3+:
- Local LLM via Ollama (OpenAI-compatible adapter)
- Cross-meeting intelligence (vault-as-memory)
- Meeting type templates (standup, 1:1, planning)
- Calendar integration (iCal)
