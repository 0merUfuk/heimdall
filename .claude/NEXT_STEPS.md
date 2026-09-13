**Version**: 4.0
**Created**: 2026-03-28
**Last Updated**: 2026-09-13
**Authors:** Omer Ufuk

---

# Heimdall -- Next Steps

> Original execution plan: `docs/MASTER_PLAN.md` (18/19 complete)
> Strategic plan: `docs/STRATEGY_V2.md` (post-grill) + `docs/PRODUCTIZATION.md` v3
> Evaluation methodology: `docs/EVALUATION.md`
> Current implementation state: `.claude/SERVICE_CONTEXT.md`

---

## Status (2026-09-13)

Repo was dormant 2026-07-19 to 2026-09-13 (docs-only commits). A single autonomous session on 2026-09-13 found CI had been silently red for two months, fixed it, and shipped four PRs (#29-32, stacked, all green) adding: the CI fix itself, a `claude-code` analyzer backend (use an existing Claude subscription instead of a separate API key), a real evaluation system for the Analyze stage (previously nonexistent), working `--save-audio`, and fully offline local transcription via Whisper. Full contents: `.claude/SERVICE_CONTEXT.md`.

**These PRs are not yet on `main`.** Merging is blocked by the harness's own permission classifier (`gh pr merge` denied); an owner needs to merge #29 -> #30 -> #31 -> #32 in order (they're stacked).

## Immediate (owner actions -- nothing here is a code-readiness gap)

| Task | Status |
|------|--------|
| Merge PR #29 (CI fix) | **BLOCKED on owner** -- `gh pr merge` denied by permission classifier |
| Merge PR #30, #31, #32 in order | **BLOCKED on owner**, same reason; each depends on the previous merging first (stacked branches) |
| Real-voice smoke test (docs/MANUAL_TESTING.md) | Still pending, still an owner action -- requires a live meeting, which no autonomous session can produce |
| macOS code-signing decision (cert+notarize vs unsigned+`xattr`) | Still pending, still an owner decision |
| Real `heimdall eval` run against a live model | Pending -- PR #31's suite has only been verified against scripted/mock analyzers; needs `ANTHROPIC_API_KEY` or a logged-in `claude` CLI to establish an actual quality baseline |
| Tag v0.1.0 release | Blocked on the above -- tagging current `main` would ship known-broken CI and miss 4 PRs of fixes/features; tag only after #29-32 merge |
| Create Homebrew tap repo | Not started -- `.goreleaser.yml` already exists and looks release-ready (bundles the Swift helper as a universal binary), but a tap needs an actual tagged release to point to first |

## Once #29-32 are merged (next session or same session, post-merge)

| Task | Priority | Notes |
|------|----------|-------|
| Tag v0.1.0, verify GoReleaser produces a working GitHub Release | P0 | `.goreleaser.yml` exists and was not touched this session; verify it still matches the current build (Go 1.27.1, Swift helper bundling) before tagging |
| Set up `homebrew-heimdall` tap | P0 | Blocked on the tag above |
| Run `heimdall eval --judge` against a real model, treat the result as the actual quality baseline | P0 | Both `docs/EVALUATION.md` and `.claude/DECISIONS.md` ID-006 flag this as unverified |
| gofmt cleanup on pre-existing drift + wire `go vet`/`gofmt -l`/`golangci-lint` into CI | P1 | `make lint` target exists (`golangci-lint`) but CI never runs it -- found this session, not yet fixed. Pre-existing gofmt drift in `cmd/heimdall/helpers_test.go`, `cmd/heimdall/secrets.go`, `internal/mixer/mixer.go`, `internal/transcriber/soniox.go` (none touched by PRs #29-32) |
| MCP server (`heimdall mcp`) | P1 | STRATEGY_V2 Phase 2C, PRODUCTIZATION.md v3's stated moat ("Obsidian knowledge graph two-way bridge"). Not started this session -- scoped out due to time, not difficulty |
| Cost/latency observability for the Analyze stage | P2 | `ClaudeAnalyzer.callAPI` already parses `usage.input_tokens`/`output_tokens` from the API response but discards them -- never logged, never surfaced. Explicitly requested in the owning goal ("cost/latency takibi") |
| Ecosystem hygiene per STRATEGY_V2 "What We're Cutting" | P3 | STRATEGY_V2 (2026-03-30) recommended deleting `product-lead`/`tech-lead`/`growth-lead`/`architect` agents, 13 of 16 skills, and the knowledge base as "engineering theater" outweighing product code. Never executed -- still all present as of 2026-09-13. Lower priority than shipping; a `.codex`/`.hermes` multi-runtime scaffold was also added since (2026-07-21) suggesting the ecosystem may now serve a broader purpose than originally scoped -- worth a conversation with the owner before deleting anything, not a unilateral autonomous action |

## Phase 2 (from STRATEGY_V2.md) -- status against the plan

| Phase | Goal | Status |
|-------|------|--------|
| 2A: Ship & Fix | Tag v0.1.0, Homebrew tap | Still not shipped as of 2026-09-13 (see above) |
| 2B: Local Transcription | Whisper.cpp, zero API keys | **Done in PR #32** (as `heimdall transcribe`, a separate batch command rather than a `record --transcriber whisper` streaming path -- see DECISIONS.md ID-007 for why) |
| 2C: MCP Server | Meeting data queryable by Claude Desktop | Not started |
| 2D: Launch | Obsidian community, README rewrite, first 50 users | Not started -- no users yet, nothing has shipped publicly |

## Post-v2 Roadmap

See `docs/STRATEGY_V2.md` Phase 3+ and `docs/PRODUCTIZATION.md` v3:
- Local LLM via Ollama (OpenAI-compatible adapter) -- not started
- Cross-meeting intelligence (vault-as-memory) -- not started; PRODUCTIZATION.md v3 scopes this as a Pro (paid) feature
- Meeting type templates (standup, 1:1, planning) -- not started; also scoped as Pro
- Calendar integration (iCal) -- not started; also scoped as Pro
