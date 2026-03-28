**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

# Heimdall — Known Issues

> Full vulnerability assessment (28 findings): `docs/architecture/ASSESSMENT.md`

---

## v1.0 Must-Fix Vulnerabilities

These must be addressed during implementation. Each is mapped to a MASTER_PLAN subtask.

| ID | Title | Severity | Effort | Addressed In |
|----|-------|----------|--------|-------------|
| V-001 | Deepgram WebSocket 60-min timeout — proactive reconnection at 55 min | Critical | Large | 0.8 |
| V-002 | Swift subprocess crash — detect within 2s, auto-restart with gap tracking | Critical | Medium | 0.6 |
| V-003 | Screen Recording permission — check before recording, clear guidance | Critical | Small | 1A.2 |
| V-005 | Network disruption — ring buffer + exponential backoff reconnection | High | Large (overlaps V-001) | 0.8 |
| V-006 | SIGKILL crash recovery — temp file writes every 30 seconds | High | Medium | 1B.3 |
| V-009 | Claude API retry + fallback to raw transcript on failure | High | Medium | 1B.1 |
| V-017 | File naming collision in vault — append suffix | Low | Small | 1B.2 |
| V-019 | macOS version check (>= 14.2) at startup | Low | Small | 1A.2 |
| V-020 | Microphone permission check before recording | Low | Small | 1A.2 |

---

## Known Limitations (Accept + Document)

These are documented trade-offs, not bugs:

| ID | Limitation | Mitigation |
|----|-----------|-----------|
| V-004 | Speaker ID resets on WebSocket reconnection | Re-map via LLM in post-meeting analysis (AD-008) |
| V-010 | No local ASR fallback | Document Deepgram dependency clearly |
| V-011 | System audio requires macOS 14.2+ | Document in README, `doctor` checks version |
| V-012 | Speaker identification ~80% accurate | Provide `--participants` hint flag |
| V-013 | LLM may hallucinate action items | Anti-hallucination prompt engineering |
| V-014 | Transcript content as prompt injection vector | Delimiter wrapping + system prompt guardrails |
| V-018 | No real-time editing of speaker names | Deferred to v2.0 |

---

## Implementation Issues

> Add issues discovered during development below.

(No implementation issues yet — development has not started.)
