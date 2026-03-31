# Heimdall — Strategy v2: Post-Grill Execution Plan

**Date**: 2026-03-30
**Input**: 20-agent grill report (2026-03-29)
**Status**: Proposed — awaiting approval
**Authors**: Omer Ufuk, Manager Agent (synthesis)

---

## One-Line Strategy

> Stop building infrastructure. Go local-first. Ship to the Obsidian community. Make meetings queryable by AI agents.

---

## Positioning Statement

*"The open-source, local-first meeting companion for developers who live in the terminal and think in Obsidian. Your audio, your keys, your vault."*

**Not**: "A cheaper Granola" (you're actually more expensive at $1.16/hr)
**Not**: "An open-source Otter.ai" (Meetily owns that with 10.8K stars)
**Is**: "The CLI meeting tool that writes to your knowledge graph and talks to your coding agent"

---

## Strategic Pillars

### Pillar 1: Local-First (Kill the API Key Barrier)
- Default to Whisper.cpp for transcription ($0/hr)
- Default to Ollama for analysis ($0/hr) with Claude as opt-in premium
- Deepgram becomes `--provider deepgram` for users who want cloud accuracy
- **Result**: `brew install heimdall && heimdall record` — zero API keys for first use

### Pillar 2: Obsidian-Native (Own the Knowledge Graph)
- Meeting notes are nodes in the user's knowledge graph, not files in a SaaS silo
- Wikilinks, YAML frontmatter, daily note integration, cross-meeting linking
- No competitor writes native Obsidian markdown. This is the moat.

### Pillar 3: MCP-Native (Make Meetings Queryable)
- `heimdall mcp` exposes meeting history to Claude Desktop, Cursor, and any MCP client
- "What did we decide about the API migration?" answered from your vault
- Talat and Read.ai just launched this — the category is forming NOW

### Pillar 4: Developer Ergonomics (CLI-First, Composable)
- Pipe-friendly, scriptable, `jq`-compatible JSON output
- `heimdall search`, `heimdall list --json`, `heimdall export`
- Integrates with developer workflows, not replaces them

---

## Phase Plan

### Phase 2A: Ship & Fix (Week 1) ← YOU ARE HERE

**Goal**: Tag v1.0, fix the bugs that would embarrass you on launch day.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| Fix `Close()` deadlock after reconnection | P0 | 2-4hr | Track all superseded connections in Close(), close them before wg.Wait() |
| Fix `SystemAudioSource.Err()` consumption | P0 | 1hr | Select on errCh in session.go, surface permission errors to terminal |
| Fix `doctor` macOS version validation | P0 | 30min | Parse major.minor, verify ≥14.2 |
| Add recording consent warning | P0 | 30min | Print warning + require `--consent-acknowledged` or interactive y/N |
| Fix `note.Platform` in live record path | P1 | 15min | Set from `--app` flag or "zoom"/"meet" default |
| Fix `--app` flag (remove or implement) | P1 | 30min | Remove the flag from CLI if not implemented; don't lie |
| Add `mip_opt_out=true` to Deepgram URL | P1 | 5min | One line in buildURL() |
| Fix Turkish filename regex | P1 | 15min | `[^a-z0-9-]+` → `[^\p{L}\p{N}-]+` |
| Wire `AnalyzeOpts.Language` in record.go | P1 | 15min | Pass `recordLanguage` to analyzeOpts |
| Make `buildUserPrompt` language-aware | P1 | 30min | When language != "en", instruct Claude on output language |
| Add data flow section to README | P1 | 30min | Document what goes to Deepgram/Anthropic |
| Correct AD-002 pricing (stereo 2x billing) | P1 | 15min | New ADR in DECISIONS.md |
| Tag v1.0.0, push release | P0 | 15min | GoReleaser, create GitHub release |
| Create Homebrew tap repo | P0 | 1hr | `homebrew-heimdall` repo, goreleaser integration |

**Exit criteria**: `brew install heimdall` works, all P0 bugs fixed, release tagged.

---

### Phase 2B: Local Transcription (Weeks 2-3)

**Goal**: Zero-cost mode. No API keys required for basic use.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| Implement `WhisperTranscriber` | P0 | 3-5 days | Behind existing `Transcriber` interface. Subprocess pattern (like Swift helper). whisper.cpp binary. |
| Add `--provider` flag to record | P0 | 1hr | `local` (default) / `deepgram` (cloud) |
| Add `transcriber.provider` to config | P0 | 1hr | Config-driven provider selection |
| Model download command | P1 | 2hr | `heimdall setup --model medium` downloads whisper.cpp + model |
| Dual-channel local diarization | P1 | 1 day | L=system (remote), R=mic (local user). Two-speaker separation for free. |
| Chunked streaming for live display | P2 | 2 days | VAD + 3-5s chunks for near-real-time with local Whisper |
| Fallback: batch after meeting | P1 | 4hr | If local streaming too slow, transcribe full audio post-meeting |

**Exit criteria**: `heimdall record --title "Test"` works with zero API keys on a fresh Mac.

---

### Phase 2C: MCP Server (Weeks 3-4)

**Goal**: Meeting data queryable by any AI agent.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| `heimdall mcp` command (stdio transport) | P0 | 2-3 days | Go MCP SDK, reads from Obsidian vault |
| `search_meetings(query, since, until)` | P0 | 1 day | Full-text search over vault markdown |
| `get_meeting(date, title)` | P0 | 4hr | Return specific note content |
| `list_meetings(since, limit)` | P0 | 4hr | List recent meeting notes |
| `get_action_items(assignee)` | P1 | 1 day | Parse action item tables from frontmatter/body |
| `get_decisions(since)` | P1 | 1 day | Parse decision sections |
| Claude Desktop integration test | P0 | 2hr | Verify "what did we decide about X?" works |

**Exit criteria**: Claude Desktop can answer questions about past meetings via MCP.

---

### Phase 2D: Launch (Week 5)

**Goal**: First 50 users.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| README rewrite — outcome-first, demo GIF | P0 | 1 day | VHS/asciinema recording, Obsidian screenshot, comparison table |
| Obsidian Forum "Share & Showcase" post | P0 | 1hr | Screenshot of rendered note, 3-step workflow |
| r/ObsidianMD post | P0 | 1hr | "I built a CLI that records meetings into my vault" |
| Obsidian Discord #share-showcase | P0 | 30min | Cross-post |
| Deepgram DevRel outreach | P1 | 1hr | Email: "open-source meeting tool using Nova-3 + diarization" |
| Hacker News "Show HN" | P2 | 1hr | Angle: "My meetings cost $0 and live in my Obsidian vault" |
| GitHub topics/tags | P0 | 15min | obsidian, meeting-notes, transcription, cli, macos, deepgram |

**Exit criteria**: 50+ GitHub stars, 10+ real users.

---

### Phase 3: Intelligence (Weeks 6-10)

**Goal**: Cross-meeting intelligence. The things Apple/Granola can't do.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| Local LLM via Ollama (`OpenAIAnalyzer`) | P0 | 2-3 days | Single adapter talks to Ollama, GPT-4o-mini, vLLM, etc. |
| `--no-analyze` flag | P1 | 30min | Skip LLM, output raw transcript only |
| Cross-meeting context | P1 | 3-5 days | Feed last 5 meeting summaries as context to Claude analysis |
| Meeting type templates | P2 | 2 days | Standup, 1:1, planning, retro — different extraction patterns |
| `heimdall search <query>` CLI | P2 | 1 day | Search across meeting notes from terminal |
| Calendar integration (iCal) | P3 | 3 days | Auto-populate participants from upcoming calendar events |
| Vault-as-memory | P3 | 5 days | Past meetings as context for analysis ("recurring unresolved items") |

---

### Phase 4: Platform (Weeks 11+) — Future

| Task | Notes |
|------|-------|
| AssemblyAI `Transcriber` | Better code-switching for Turkish |
| Groq batch re-transcription | Cheapest cloud option for `analyze --retranscribe` |
| Obsidian companion plugin | "Record Meeting" button in sidebar, status display |
| People pages in vault | Auto-generated `[[Person Name]]` pages with meeting history |
| Daily note integration | Append meeting summaries to today's daily note |
| Apple SpeechAnalyzer backend | Free on-device STT, add diarization + structure on top |

---

## What We're Cutting

These items from the v1.0 roadmap are **deprioritized or dropped**:

| Item | Previous Phase | Decision | Reason |
|------|---------------|----------|--------|
| Web dashboard | Phase 4 | **Drop** | Granola's territory. Wrong direction for CLI tool. |
| Windows support | Phase 4 | **Defer indefinitely** | Core Audio Taps are the differentiator. WASAPI dilutes identity. |
| Team/collaboration features | Phase 4 | **Drop** | Enterprise is Otter/Fireflies/Granola. Not our market. |
| Linux support | Phase 3 | **Defer** | macOS audience first. Revisit if demand signals appear. |
| Voice enrollment (ECAPA-TDNN) | Phase 2 | **Defer to Phase 4** | Nice-to-have, not essential. LLM speaker ID works. |
| Domain-lead agents | Current | **Delete** | Never used. product-lead, tech-lead, growth-lead, architect → remove. |
| Unused skills (10 of 16) | Current | **Delete** | /dep-audit, /strategy-weekly, /strategy-monthly, etc. never invoked. |
| Knowledge base (3,670 lines) | Current | **Delete** | Generic Go patterns the LLM already knows internally. |

---

## Success Metrics

### Month 1 (End of Phase 2B)
- [ ] v1.0.0 tagged and released
- [ ] `brew install heimdall` works
- [ ] All P0 bugs from grill report fixed
- [ ] Local Whisper mode functional (zero API keys needed)

### Month 2 (End of Phase 2D)
- [ ] MCP server functional with Claude Desktop
- [ ] README rewritten with demo GIF
- [ ] Posted to Obsidian community
- [ ] 50+ GitHub stars
- [ ] 10+ real users (tracked via GitHub issues/discussions)

### Month 3 (End of Phase 3)
- [ ] Local LLM option via Ollama
- [ ] Cross-meeting intelligence working
- [ ] Developer actually uses heimdall daily for own meetings
- [ ] Turkish meeting transcription validated with real meetings

---

## The Honest Kill Criteria

**Stop investing if** (at month 2):
- Turkish Deepgram quality is below 70% accuracy on real meetings
- Zero organic GitHub stars after Obsidian community post
- Developer (you) doesn't use heimdall for own meetings daily

**Pivot to portfolio piece if** (at month 3):
- Fewer than 20 active users
- Meetily adds Obsidian export + CLI mode
- Apple ships Turkish transcription + diarization

---

## Cost of Doing Nothing

The window is ~6 months before:
1. Meetily absorbs the "open-source meeting AI" mindshare completely
2. Apple FoundationModels (fall 2026) commoditizes capture + transcribe + summarize
3. Read.ai or Granola builds "meeting data for coding agents" before heimdall
4. Talat (launched this week) establishes the "local Mac meeting notes + MCP" category

**The code is ready. The market window is open. Ship.**

---

## References

- [Grill Report](./GRILL_REPORT.md) — full findings from 20-agent audit
- [MASTER_PLAN.md](./MASTER_PLAN.md) — original 19-subtask v1.0 plan (18/19 complete)
- [ROADMAP.md](./architecture/ROADMAP.md) — original phased roadmap (partially superseded)
- [STRATEGY.md](./architecture/STRATEGY.md) — original market research (pre-grill)
