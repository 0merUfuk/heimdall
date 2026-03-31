**Version**: 3.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-31
**Authors:** Omer Ufuk

---

# Heimdall -- Next Steps

> Original execution plan: `docs/MASTER_PLAN.md` (19/19 complete)
> New strategic plan: `docs/STRATEGY_V2.md` (post-grill)

---

## v1.0 — COMPLETE

All 19 MASTER_PLAN subtasks executed. Security review passed (PR #6). Final review passed (PR #7). 31-bug sweep merged (PR #8).

## Immediate (this week)

| Task | Status |
|------|--------|
| Smoke test with real voices (docs/MANUAL_TESTING.md) | PENDING |
| Tag v1.0.0 release | PENDING (after smoke test) |
| Create Homebrew tap repo | NOT STARTED |

## Phase 2 (from STRATEGY_V2.md)

| Phase | Goal | Timeline |
|-------|------|----------|
| 2A: Ship & Fix | Tag v1.0.0, Homebrew tap | Week 1 |
| 2B: Local Transcription | Whisper.cpp default, zero API keys | Weeks 2-3 |
| 2C: MCP Server | Meeting data queryable by Claude Desktop | Weeks 3-4 |
| 2D: Launch | Obsidian community, README rewrite, first 50 users | Week 5 |

## Post-v2 Roadmap

See `docs/STRATEGY_V2.md` Phase 3+:
- Local LLM via Ollama (OpenAI-compatible adapter)
- Cross-meeting intelligence (vault-as-memory)
- Meeting type templates (standup, 1:1, planning)
- Calendar integration (iCal)
