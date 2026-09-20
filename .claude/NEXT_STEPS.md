**Version**: 5.1
**Created**: 2026-03-28
**Last Updated**: 2026-09-19
**Authors:** Omer Ufuk

---

# Heimdall -- Next Steps

> Original execution plan: `docs/MASTER_PLAN.md`
> Strategic plan: `docs/STRATEGY_V2.md` (post-grill) + `docs/PRODUCTIZATION.md` v3
> Evaluation methodology: `docs/EVALUATION.md`
> Release process: `docs/RELEASING.md`
> Current implementation state: `.claude/SERVICE_CONTEXT.md`

---

## Status (2026-09-14)

**v0.1.0 is tagged and released**: [github.com/0merUfuk/heimdall/releases/tag/v0.1.0](https://github.com/0merUfuk/heimdall/releases/tag/v0.1.0). 11 PRs (#29-#41) merged into `main` over 2026-09-13/14, closing the CI break, adding a second Analyzer backend, a real eval system, local Whisper transcription, an MCP server, cost/latency observability, a lint/gofmt CI gate, a real (previously broken) release pipeline, and tag-triggered release automation. Full contents: `.claude/SERVICE_CONTEXT.md`.

## In review: local/offline analyzer + Codex + cloud (branch `claude/heimdall-offline-analyzer-502957`)

Built and verified locally; see `.claude/SERVICE_CONTEXT.md` and `.claude/DECISIONS.md` ID-011..013. Remaining before/after merge:

| Task | Owner | Why |
|------|-------|-----|
| Review + merge the branch | Owner | Nothing merged yet |
| ~~`heimdall eval --analyzer codex`~~ | Done 2026-09-20 | 6/7, 6/7, 5/7 live; see `docs/EVALUATION.md` |
| Live meeting test of `record --transcriber whisper --analyzer ollama` | Owner present in a real meeting; needs macOS Microphone + Screen & System Audio Recording permission for the app that runs it | The one thing synthetic audio cannot validate |
| Set the Codex cloud environment's Setup script to `scripts/cloud-setup.sh` | Owner (ChatGPT web UI) | Cannot be configured from the repo |
| Map-reduce for meetings longer than the local context window | Only if real meetings overflow | Deliberately deferred (ID-011) |

## Immediate (owner actions -- nothing left here is a code-readiness gap)

| Task | Status |
|------|--------|
| Create the `homebrew-heimdall` tap repo (empty, public) + add a `HOMEBREW_TAP_TOKEN` repo secret (PAT, `contents:write` on that repo) | **Not started, owner action** -- `docs/RELEASING.md` has the exact steps. Until done, the release workflow correctly skips the Homebrew push rather than failing (verified: v0.1.0's release succeeded cleanly without it) |
| Real-voice smoke test (`docs/MANUAL_TESTING.md`) | Still pending, still an owner action -- requires a live meeting. A synthetic 2-speaker pipeline test (real `say`+`ffmpeg`-generated audio through real `whisper-cli` + real `claude` CLI) was run this session and verified transcription accuracy and graceful degradation; a genuine live meeting is the one thing that can't be substituted |
| Real `heimdall eval` run against a live model | Pending -- confirmed this session that the suite correctly detects and fails on degraded/fallback output rather than false-passing (0/7 with a clear, correct reason), but no sandbox credential was available to get an actual passing baseline. Needs `ANTHROPIC_API_KEY` or a logged-in `claude` CLI |
| macOS code-signing decision (cert+notarize vs. unsigned+`xattr` workaround) | Still pending, still an owner decision -- `docs/RELEASING.md` documents the current unsigned state and the `xattr` postflight hook as the interim fix |

## Recently closed (this round, PRs #29-#41 -- see SERVICE_CONTEXT.md for detail)

- CI break (2 months red) fixed
- `claude-code` analyzer backend (use an existing Claude subscription, no separate API key)
- Real eval system (`heimdall eval`, `internal/eval`)
- `--save-audio` wired up + local Whisper transcription (`heimdall transcribe`)
- `go vet`/`gofmt -l`/`golangci-lint` wired into CI
- GoReleaser universal-binary bug fixed (release builds had never actually succeeded before this)
- MCP server (`heimdall mcp`) -- STRATEGY_V2 Phase 2C, PRODUCTIZATION.md v3's stated moat
- Cost/latency observability for the Analyze stage
- Tag-triggered release automation + Homebrew tap config (`.goreleaser.yml`, `.github/workflows/release.yml`)
- v0.1.0 tagged and released

## Phase 2 (from STRATEGY_V2.md) -- status against the plan

| Phase | Goal | Status |
|-------|------|--------|
| 2A: Ship & Fix | Tag v0.1.0, Homebrew tap | v0.1.0 shipped; tap config exists, tap repo/token creation is the one remaining owner step |
| 2B: Local Transcription | Whisper.cpp, zero API keys | Done (`heimdall transcribe`, a separate batch command rather than a `record --transcriber whisper` streaming path -- see DECISIONS.md ID-007 for why) |
| 2C: MCP Server | Meeting data queryable by Claude Desktop | Done (`heimdall mcp`) |
| 2D: Launch | Obsidian community, README rewrite, first 50 users | Not started -- v0.1.0 just shipped, no public launch push yet |

## Post-v2 Roadmap

See `docs/STRATEGY_V2.md` Phase 3+ and `docs/PRODUCTIZATION.md` v3:
- Local LLM via Ollama -- **built** on this branch (`--analyzer ollama`), via Ollama's native API rather than the OpenAI-compatible adapter originally planned (ID-011 explains why)
- Cross-meeting intelligence (vault-as-memory) -- not started; PRODUCTIZATION.md v3 scopes this as a Pro (paid) feature
- Meeting type templates (standup, 1:1, planning) -- not started; also scoped as Pro
- Calendar integration (iCal) -- not started; also scoped as Pro
- Ecosystem hygiene per STRATEGY_V2 "What We're Cutting" (deleting product-lead/tech-lead/growth-lead/architect agents, most skills) -- still not executed; deliberately left as an owner conversation, not a unilateral action, since a `.codex`/`.hermes` multi-runtime scaffold added since suggests the ecosystem's scope may now be intentional
- `total_cost_usd` wiring for the `claude-code` analyzer path -- its own `claude -p --output-format json` envelope already reports this; not yet piped through to the same log line the API backend uses (see `.claude/DECISIONS.md` ID-010's Consequences)
- ~18 stray `oracle/cycle-*` git tags from earlier agent work -- harmless (GoReleaser ignores non-semver tags for versioning) but clutter `git describe`/tag listings; left for an owner decision, not deleted unilaterally
