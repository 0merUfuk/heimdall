# Heimdall — Project Grill Report

**Date**: 2026-03-29
**Method**: 20 parallel AI agents (4 reviewers, 11 strategists, 5 code auditors)
**Scope**: Product viability, code quality, competitive landscape, architecture, distribution, privacy, i18n, market trends

---

## Executive Summary

Heimdall is a well-engineered product that has not been shipped to a single user. The architecture is sound (A-), the code quality is high (A), but the competitive position is weak (D+) and distribution is nonexistent (F). The project has a priorities inversion: more energy went into meta-infrastructure (10 agents, 16 skills, 7 architecture docs) than into getting the tool into someone's hands.

**The one niche where heimdall wins**: Turkish-language meetings → structured Obsidian notes with speaker diarization, action items, and custom templates → personal knowledge vault. No competitor serves this intersection today.

---

## Scorecard

| Category | Grade | Summary |
|----------|-------|---------|
| Engineering Quality | **A** | 270 tests, 4 deps, atomic writes, graceful degradation |
| Architecture | **A-** | 6-stage pipeline justified, Go+Swift correct, interfaces earned their cost |
| Code Correctness | **C+** | Close() deadlocks at 55min, ring buffer dead code, Err() unconsumed |
| Documentation Honesty | **C** | doctor lies about macOS check, README model claim wrong, ~72% repo is non-product |
| Product-Market Fit | **D+** | Apple Notes free, Granola $14/mo at $1.5B, heimdall $23/mo at 20 meetings |
| Distribution | **F** | Zero users, no binary release, no Homebrew, 10-step install |
| Competitive Position | **D** | Granola same approach + $192M funding, Meetily 10.8K stars + free |
| Process Proportionality | **D** | 5,465 LOC product vs 7,571 LOC meta-infrastructure |

---

## Critical Bugs

| Bug | Impact | Location |
|-----|--------|----------|
| `Close()` deadlocks after reconnection | Meetings >55min hang on Ctrl+C | `deepgram.go:258 + :546` |
| Ring buffer is dead code | V-005 "30s buffer" written but never read | `mixer.go:331`, `session.go:248` |
| `SystemAudioSource.Err()` never consumed | Permission denied = silent mic-only recording | `system.go:541` |
| `doctor` doesn't validate macOS version | "Ready to record" on macOS 14.0, Swift crashes | `doctor.go:33-41` |
| `note.Platform` never set in live path | Blank `platform:` in every Obsidian note | `record.go:191-193` |
| `--app` flag silently ignored | Accepted but never wired to SystemAudioSource | `record.go:52,78` |
| Turkish filename chars stripped | `[^a-z0-9-]+` kills ç, ğ, ı, ö, ş, ü | `renderer.go:69` |
| `AnalyzeOpts.Language` never wired | Claude gets no language signal for Turkish meetings | `record.go:176-180` |
| No recording consent warning | STRATEGY.md prescribes R4 warning, code has none | `record.go` |
| `recover` processes all files without confirmation | No "y/N" prompt, costs money per file | `recover.go:85-92` |

---

## Pricing Correction

**AD-002 is wrong.** Deepgram bills stereo/multichannel at 2x the mono rate.

| Metric | AD-002 Claimed | Actual |
|--------|---------------|--------|
| Deepgram cost/hr | $0.58 | **$1.16** (stereo + diarization) |
| Total cost/hr | $0.60 | **$1.18** |
| Monthly @ 20 meetings | ~$12 | **~$23.60** |
| Free credit hours | ~345 | **~172** |

Heimdall is **more expensive than Granola** ($14/mo) at moderate usage.

---

## Competitive Landscape

### Direct Threats

| Competitor | What They Do | Why They Matter |
|------------|-------------|-----------------|
| **Granola** ($1.5B) | Same Core Audio Taps approach, native app, MCP server, $14/mo | Validates the market but outclasses on UX/distribution |
| **Apple Notes** (free) | On-device transcription + summarization, macOS Sequoia | Kills casual English use case; no Turkish, no diarization |
| **Meetily** (10.8K⭐) | 100% local, Tauri GUI, Whisper + Ollama, free | Owns "privacy-first open-source meeting AI" |
| **Talat** (launched Mar 24) | Core Audio Taps, local Whisper, local LLM, Obsidian export, MCP server | Almost exactly heimdall but GUI, shipping NOW |

### Heimdall's Unique Position

No competitor serves: **CLI-first + Obsidian-native + Turkish + no-bot + open-source + provider-swappable**

---

## Architecture Assessment

### What's Genuinely Good
- 4 direct dependencies for audio + WebSocket + CLI + YAML — remarkably lean
- Go + Swift dual binary — the only correct choice (Core Audio Taps require Swift)
- 6-stage pipeline maps to real concerns with different failure modes
- Raw WebSocket over Deepgram SDK — correct for timestamp/reconnection control
- Provider interfaces (`AudioSource`, `Transcriber`, `Analyzer`) — not YAGNI, tests use them
- Crash recovery with atomic writes every 30s — correct defensive engineering
- Graceful degradation chain actually implemented, not just documented

### What Should Change
- Default to local transcription (Whisper), cloud as opt-in
- Add MCP server (currently roadmap v4.0, should be v2.0)
- Wire the ring buffer to the transcriber (currently dead code)
- Replace 1s polling reconnectTimer with single-shot timer
- Fix `record.go` to read Claude API key from config, not just env var

---

## Performance Profile

| Metric | Value | Concern? |
|--------|-------|----------|
| RAM (1hr) | ~14 MB | No |
| CPU | <2% single core | No |
| Network upstream | 512 kbit/s (220 MB/hr) | Yes on bad WiFi |
| Goroutines | ~13 during recording | No |
| Recovery file | ~293 KB/hr | No |
| Ring buffer | 1.87 MB fixed ceiling | No (but dead code) |

---

## Meta-Engineering ROI

```
Product source code:      5,465 lines  (20%)
Test code:                9,318 lines  (35%)
Meta (.claude/ ecosystem): 7,571 lines  (28%)
Architecture docs:        3,642 lines  (13%)
User-facing docs:         1,032 lines  (4%)
```

- 5 of 10 agents used, 5 never invoked
- 3 of 16 skills used, 13 never invoked
- 3 of 7 architecture docs load-bearing
- Knowledge base (3,670 lines) encodes patterns the LLM already knows

**Verdict**: Core ecosystem (developer, reviewer, tester + /audit, /doublecheck + ASSESSMENT.md, DECISIONS.md) was a force multiplier. The expansion (domain-lead agents, growth skills, knowledge base) is engineering theater.

---

## Turkish/i18n Readiness

| Dimension | Status | Rating |
|-----------|--------|--------|
| Transcription (pure Turkish) | Works with `--language tr` | 7/10 |
| Transcription (TR+EN code-switch) | Deepgram `multi` excludes Turkish | **3/10** |
| Claude analysis language | Language field never wired to prompt | **4/10** |
| Filename handling | Turkish chars stripped by regex | **2/10** |
| YAML/Markdown content | UTF-8 works correctly | 9/10 |

---

## Privacy/Compliance Gaps

| Issue | Status |
|-------|--------|
| Raw audio → Deepgram US servers | No EU endpoint option in config |
| Transcript → Anthropic (7-day retention) | Not documented anywhere |
| Deepgram `mip_opt_out` parameter | Not passed (should be hardcoded true) |
| Recording consent warning | STRATEGY.md prescribes it, code doesn't implement it |
| Data flow documentation | Missing from README |
| growth-lead.md claims "audio never leaves machine" | Factually incorrect |

---

## STT Strategy Assessment

| Provider | Turkish Streaming | Diarization | Cost/hr | Verdict |
|----------|------------------|-------------|---------|---------|
| Deepgram Nova-3 | Yes | Yes | $1.16 | Current. Only provider with all three |
| AssemblyAI U2 | Batch only | Yes | $0.57 | Blocked for Turkish streaming |
| OpenAI GPT-4o | Yes | Batch only | $0.36 | Blocked for streaming diarization |
| Whisper.cpp (local) | Chunked | Dual-channel only | $0 | Best v2.0 default |
| Groq Whisper | Batch only | No | $0.02 | Best for re-transcription |

---

## LLM Strategy Assessment

Claude Haiku 4.5 is correct for v1.0. Cost is $0.02/meeting — negligible. The highest-leverage next step is an OpenAI-compatible `Analyzer` adapter that unlocks GPT-4o-mini, Ollama (local), vLLM, and all compatible endpoints through one implementation.

---

## Distribution Strategy

| Channel | Priority | Expected Impact |
|---------|----------|----------------|
| r/ObsidianMD (~250K members) | **#1** | High — active demand for this exact tool |
| Obsidian Forum "Share & Showcase" | **#2** | High — engaged community |
| Deepgram DevRel partnership | **#3** | Medium — free, targeted exposure |
| Hacker News "Show HN" | **#4** | Low-medium — meeting tools underperform on HN |
| Homebrew tap | Required | Reduces install friction from 10 steps to 3 |

**Realistic ceiling**: 200-500 stars without local mode, 1-2K with local + MCP + Obsidian community traction.

---

## Market Timing

- **6-month window** before Meetily adds CLI/MCP or Read.ai targets developers
- **Apple FoundationModels** ships fall 2026 — free on-device summarization
- **MCP meeting data** is forming as a category NOW (Read.ai launched March 2026)
- **Granola's $1.5B** validates the market but moves upmarket to enterprise

---

## Sources

20 agents collectively cited 100+ sources including:
- TechCrunch (Granola $125M, Talat launch)
- Deepgram documentation and pricing
- Anthropic API pricing and data policies
- GitHub repositories (Meetily, Screenpipe, WhisperLive, MeetBalls, Nojoin)
- Apple Developer documentation (SpeechAnalyzer, FoundationModels)
- Market reports (Verified Market Reports, Technavio, PLAUD)
- Provider benchmarks (Artificial Analysis, Soniox)
- Privacy regulations (GDPR, BIPA, EU AI Act)
