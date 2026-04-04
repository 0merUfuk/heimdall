# Heimdall — Execution Plan v3

**Date**: 2026-04-02
**Supersedes**: STRATEGY_V2.md Phase Plan (timeline updated, priorities adjusted)
**Status**: Active

---

## Where We Are (Ground Truth)

### What's Working
- Audio capture: system + mic ✅ (tested in real 67-min Turkish meeting)
- Deepgram Turkish transcription ✅ (real meeting, readable output)
- Real-time terminal display ✅
- Crash recovery ✅
- Config system ✅ (TRYPIX vault configured)
- 31 bugs fixed, 32 failure paths guarded ✅

### What's NEVER Worked
- **Claude analysis** — user has Claude subscription, not API key. Zero end-to-end runs.
- **Obsidian note output** — no meeting note has ever been written to TRYPIX vault.
- **Multi-speaker diarization** — mono+diarize implemented (PR #11) but untested with real voices.

### Current Environment
```
DEEPGRAM_API_KEY:  set (in .zshrc, worked during meeting)
ANTHROPIC_API_KEY: NOT set (user has subscription, not API)
Ollama:            NOT installed
Whisper.cpp:       NOT installed
Vault:             /Users/omerufuk/Documents/TRYPIX/10-Meetings/
Existing folders:  1-on-1s/, Daily/
Config language:   en (should be tr for Turkish meetings)
```

---

## The Plan

### Milestone 0: First End-to-End Success (THIS WEEK)

**Goal**: See a real meeting note appear in your TRYPIX Obsidian vault. Everything else is secondary.

#### Your Actions (30 minutes)

1. **Merge PR #12** (guardrails)

2. **Get Anthropic API key** (5 minutes, free credits):
   - Go to https://console.anthropic.com/
   - Sign up (separate from Claude subscription)
   - Create an API key
   - New accounts get $5 free credit (~250 meeting analyses)
   ```bash
   # Add to ~/.zshrc
   export ANTHROPIC_API_KEY=sk-ant-...
   source ~/.zshrc
   ```

3. **Fix your config language** (your meetings are in Turkish):
   ```bash
   ./bin/heimdall config set deepgram.language tr
   ```

4. **Rebuild and test** in your next meeting:
   ```bash
   make build
   ./bin/heimdall record --title "Daily TRYPIX" --language tr --participants "Omer,Merve,Cenker,Arda"
   ```

5. **After Ctrl+C**, you should see:
   ```
   Generating meeting summary via Claude...
   Meeting note saved: /Users/omerufuk/Documents/TRYPIX/10-Meetings/2026-04-02/daily-trypix.md
   ```

6. **Open in Obsidian** — verify the note looks good:
   - Summary in Turkish?
   - Action items extracted?
   - Speaker names identified?
   - Wikilinks `[[Omer]]`, `[[Merve]]` working?
   - Multiple Speaker IDs (not all Speaker 0)?

**Exit criteria**: One real meeting note in your TRYPIX vault that you'd actually reference later.

#### My Actions (parallel)

- Start implementing Ollama analyzer (Phase 2B prep)
- Prepare `--no-analyze` flag for transcript-only mode

---

### Milestone 1: Daily Driver (Week 1-2)

**Goal**: You use heimdall for every TRYPIX meeting. Fix whatever breaks.

#### Your Actions
- Record every meeting with heimdall
- Report what's wrong, what's missing, what's annoying
- Build the habit: terminal before meeting, Ctrl+C after

#### My Actions
| Task | Effort | Deliverable |
|------|--------|-------------|
| Implement `OllamaAnalyzer` (OpenAI-compatible) | 2-3 days | `--analyzer ollama` flag, works with any local LLM |
| Add `--no-analyze` flag | 30 min | Transcript-only mode, no API key needed |
| Fix whatever your daily use reveals | Ongoing | Bug fixes as needed |
| Tag v1.0.0-beta | 1 hr | GitHub pre-release |

**Exit criteria**: You've used heimdall for 5+ meetings. You know what works and what doesn't.

---

### Milestone 2: Local-First (Weeks 2-4)

**Goal**: `heimdall record` works with zero API keys. $0/meeting.

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| `WhisperTranscriber` implementation | P0 | 3-5 days | whisper.cpp subprocess, same pattern as Swift helper |
| `--provider local/deepgram` flag | P0 | 1 hr | Provider selection at record time |
| `transcriber.provider` in config | P0 | 1 hr | Default: `local` when whisper available, `deepgram` otherwise |
| `heimdall setup` command | P0 | 2 hr | Downloads whisper.cpp binary + model (~1.5GB) |
| `OllamaAnalyzer` (if not done in M1) | P0 | 2-3 days | OpenAI-compatible adapter → Ollama, GPT-4o-mini, vLLM |
| `analyzer.provider` in config | P0 | 1 hr | `ollama` (default local) / `anthropic` (cloud) |
| Auto-detect available providers | P1 | 2 hr | Check if whisper/ollama installed, fall back gracefully |
| Batch transcription fallback | P1 | 4 hr | If live Whisper too slow, transcribe after meeting |

**Provider matrix after this milestone:**

| | Transcription | Analysis | Cost/hr |
|---|---|---|---|
| **Full local** | Whisper.cpp | Ollama | $0.00 |
| **Hybrid** | Whisper.cpp | Claude API | $0.02 |
| **Full cloud** | Deepgram | Claude API | $1.18 |

**Exit criteria**: `make build && heimdall record --title "Test"` works on a fresh Mac with zero API keys.

---

### Milestone 3: MCP Server (Weeks 4-5)

**Goal**: "Hey Claude, what did we decide about the redesign in yesterday's TRYPIX daily?"

| Task | Priority | Effort | Details |
|------|----------|--------|---------|
| `heimdall mcp` command (stdio) | P0 | 2-3 days | Go MCP SDK, reads from Obsidian vault |
| `search_meetings(query, since)` | P0 | 1 day | Full-text search over meeting notes |
| `get_meeting(date, title)` | P0 | 4 hr | Return specific note |
| `list_meetings(since, limit)` | P0 | 4 hr | Recent meetings list |
| `get_action_items(assignee)` | P1 | 1 day | Aggregate from all notes |
| Claude Desktop config | P0 | 1 hr | Add to `claude_desktop_config.json` |

**Why this matters**: This is what makes heimdall more than a transcriber. Your meeting notes become queryable context for Claude, Cursor, and every MCP-compatible tool. No competitor in the CLI space does this.

**Exit criteria**: Ask Claude Desktop "what are my open action items from TRYPIX meetings this week?" and get a real answer from your vault.

---

### Milestone 4: Ship to the World (Weeks 5-6)

**Goal**: First 50 external users.

| Task | Owner | Effort | Details |
|------|-------|--------|---------|
| Tag v1.0.0 final release | Me | 1 hr | GoReleaser, GitHub release with binaries |
| Create Homebrew tap | Me | 2 hr | `brew tap 0merUfuk/tap && brew install heimdall` |
| README rewrite | Me | 1 day | Demo GIF (VHS/asciinema), Obsidian screenshot, comparison table |
| Obsidian Forum post | You | 1 hr | "Share & Showcase" — screenshot of note, 3-step workflow |
| r/ObsidianMD post | You | 1 hr | "I built a CLI that records meetings into my Obsidian vault" |
| Deepgram DevRel outreach | You | 1 hr | Email: "open-source meeting tool using Nova-3" |
| GitHub topics/tags | Me | 15 min | obsidian, meeting-notes, cli, macos, transcription |

**Exit criteria**: 50+ GitHub stars, 10+ real users reporting issues/feedback.

---

### Milestone 5: Intelligence (Weeks 7-10)

**Goal**: The features that make heimdall irreplaceable.

| Task | Priority | Effort |
|------|----------|--------|
| Cross-meeting context (feed last 5 summaries to Claude) | P1 | 3-5 days |
| Meeting type templates (standup, 1:1, planning, retro) | P2 | 2 days |
| `heimdall search <query>` CLI | P2 | 1 day |
| Multichannel + per-channel diarize (upgrade from mono) | P2 | 2 days |
| Calendar integration (iCal → auto-populate participants) | P3 | 3 days |
| Vault-as-memory ("recurring unresolved items") | P3 | 5 days |

---

## What We're NOT Doing

| Item | Why Not |
|------|---------|
| Web dashboard | Granola's territory |
| Windows/Linux | macOS Core Audio Taps is the differentiator |
| Team collaboration | Enterprise is Otter/Fireflies, not our market |
| Voice enrollment (ECAPA-TDNN) | LLM speaker ID from context is good enough |
| GUI/menu bar app | CLI is the identity |

---

## Success Metrics

| Timeframe | Metric |
|-----------|--------|
| This week | First real meeting note in TRYPIX vault |
| Week 2 | Using heimdall daily for all TRYPIX meetings |
| Week 4 | Zero-API-key mode working (local Whisper + Ollama) |
| Week 5 | MCP server live, Claude Desktop querying your meetings |
| Week 6 | v1.0.0 released, posted to Obsidian community |
| Week 8 | 50+ GitHub stars, 10+ external users |

---

## Kill Criteria (Be Honest)

**Pause if** (at week 4):
- You don't use heimdall for your own TRYPIX meetings
- Turkish Deepgram quality is below 70% accuracy
- Zero organic interest after Obsidian community post

**Pivot to portfolio piece if** (at week 8):
- Fewer than 20 users
- Meetily adds Obsidian export
- Apple ships Turkish transcription + diarization

---

## The Competitive Window

- **Talat** launched March 24 with MCP + local + Obsidian — we need MCP by Week 5
- **Meetily** at 10.8K stars — if they add CLI mode, our niche narrows
- **Apple FoundationModels** ships fall 2026 — basic pipeline becomes free on-device
- **Read.ai** launched MCP March 2026 — "meeting data for coding agents" category is forming

**Clock is ticking. Let's cook.**
