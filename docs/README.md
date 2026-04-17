# Heimdall — Documentation Index

**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

## Strategy & Execution

| Document | Purpose | Status |
|----------|---------|--------|
| [MASTER_PLAN.md](MASTER_PLAN.md) | 19-subtask execution plan with dependency graph and acceptance criteria | 18/19 complete |
| [STRATEGY_V2.md](STRATEGY_V2.md) | Post-grill strategic pivot — local-first, Obsidian-native, MCP-first (supersedes architecture/STRATEGY.md) | Current |
| [GRILL_REPORT.md](GRILL_REPORT.md) | 20-agent grill audit that produced STRATEGY_V2 | Historical |
| [MANUAL_TESTING.md](MANUAL_TESTING.md) | Step-by-step end-to-end manual test scenarios (7 scenarios + smoke test) | Current |

---

## Architecture Documents

| Document | Purpose | Status |
|----------|---------|--------|
| [PROJECT.md](architecture/PROJECT.md) | Project identity, naming, target audience, development approach, competitive positioning | Complete |
| [PIPELINE.md](architecture/PIPELINE.md) | Full 6-stage pipeline architecture with diagrams and technology map | Complete |
| [DECISIONS.md](architecture/DECISIONS.md) | All architectural decisions in ADR format (AD-001 through AD-010) | Complete |
| [MVP.md](architecture/MVP.md) | v1.0 specification — commands, config, templates, cost, audio design | Complete |
| [ROADMAP.md](architecture/ROADMAP.md) | Phased roadmap from spike through v4.0 with timeline estimates | Complete |
| [ASSESSMENT.md](architecture/ASSESSMENT.md) | Engineering vulnerability assessment — 28 findings with mitigations | Complete |
| [STRATEGY.md](architecture/STRATEGY.md) | Full strategic research — market analysis, competitive landscape, API pricing, technical feasibility | Historical (superseded by STRATEGY_V2.md) |

---

## Quick Reference

### Pipeline Stages

```
1. CAPTURE   → Core Audio Taps (Swift) + malgo mic (Go)
2. MIX       → Resample, convert, interleave stereo (L=system, R=mic)
3. TRANSCRIBE → Deepgram Nova-3 WebSocket streaming + diarization
4. ACCUMULATE → In-memory segments + terminal display + crash recovery
5. ANALYZE   → Claude API (post-meeting) — summary, speaker ID, action items
6. RENDER    → Go templates → Obsidian vault markdown
```

### Key Decisions

| ID | Decision |
|----|---------|
| AD-001 | Go + Swift dual-binary architecture |
| AD-002 | Deepgram as primary STT provider |
| AD-003 | Provider abstraction layer (3 interfaces) |
| AD-004 | Post-meeting Claude analysis (not real-time) |
| AD-005 | File-based Obsidian integration (no plugin) |
| AD-006 | MIT license (tentative) |
| AD-007 | Dual-channel stereo (L=system, R=mic) |
| AD-008 | LLM contextual speaker identification for MVP |
| AD-009 | CLI-first, dashboard deferred |
| AD-010 | macOS 14.2+ minimum |

### v1.0 Must-Fix Vulnerabilities

| ID | Title | Effort |
|----|-------|--------|
| V-001 | 60-min WebSocket reconnection | Large |
| V-002 | Swift subprocess crash detection | Medium |
| V-003 | Screen Recording permission UX | Small |
| V-005 | Network disruption recovery | Large (overlaps V-001) |
| V-006 | SIGKILL crash recovery | Medium |
| V-009 | Claude API retry + fallback | Medium |
| V-017 | File naming collision | Small |
| V-019 | macOS version check | Small |
| V-020 | Microphone permission check | Small |
