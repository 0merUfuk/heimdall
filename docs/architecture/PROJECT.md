# Heimdall — Project Overview

**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

## Identity

| Field | Value |
|-------|-------|
| **Name** | Heimdall |
| **Meaning** | Norse god — the all-hearing guardian of the Bifrost bridge, possessing extraordinary hearing (could hear grass growing) |
| **Repository** | `github.com/0merUfuk/heimdall` |
| **License** | MIT (tentative — revisit at distribution phase) |
| **Language** | Go (orchestrator) + Swift (macOS audio capture helper) |
| **Distribution** | Open-source, Homebrew tap, GoReleaser cross-platform builds |

### Naming Decision

Alternatives considered:

| Name | Meaning | Why Not Chosen |
|------|---------|----------------|
| hearlog | Hear + Log | Good but generic |
| scriba | Latin for "scribe" | Elegant but less intuitive |
| vaultear | Vault (Obsidian) + Ear | Compound word, could be misread |
| echopad | Echo + Pad | Some existing GitHub repos |
| memnos | Greek "memory" | Pronunciation unclear |

**Heimdall** was chosen by the project owner. Perfect alignment with the product's purpose — an all-hearing guardian that listens to everything in your meetings.

---

## What Heimdall Is

A CLI-first, open-source meeting companion that:

1. **Captures audio** from all meeting participants (system audio + microphone)
2. **Transcribes in real-time** with speaker diarization (who said what)
3. **Analyzes via Claude** after the meeting (summary, action items, decisions)
4. **Writes to Obsidian** as structured meeting notes with frontmatter, wikilinks, and tags

---

## What Heimdall Is NOT

- **Not a GUI app** — no Electron, no menu bar icon, no window. Obsidian IS the GUI.
- **Not a SaaS product** — runs locally, user owns all data, pays API costs directly.
- **Not a meeting bot** — does not join calls. Captures audio from the system output device.
- **Not kernel-level** — entirely user-space, standard macOS permissions model.

---

## App vs CLI — Decision Record

**Decision**: CLI tool. Explicitly not a desktop application.

**Rationale**:
- Target audience lives in the terminal (engineers, Obsidian power users)
- The user runs Claude Code inside Obsidian's integrated terminal — CLI is native there
- Long-running process model (like `docker compose up`, not `ls`)
- Obsidian serves as the GUI for viewing output
- A web dashboard may be added post-v1.0 as a separate presentation layer
- Avoids GUI state management complexity entirely

**Terminal behavior during recording**:
```
heimdall v0.1.0 — recording "Sprint Planning"
Audio: system ✓  mic ✓  | STT: deepgram (connected) | 00:12:34

[00:00:12] Speaker 1: Alright, let's start with the sprint review.
[00:00:18] Speaker 2: Sure. I finished the auth migration yesterday.
[00:00:24] Speaker 1: Great. Any blockers on the API gateway?
[00:00:31] Speaker 3: Yes, we're waiting on the cert renewal...
[00:01:05] Speaker 1: Let's make that a priority for today.
                                                          ▌
Press q or Ctrl+C to stop recording
```

---

## System-Level Access — Permission Model

**Heimdall requires zero kernel-level access.** All permissions are user-space, granted through macOS TCC (Transparency, Consent, and Control).

| Permission | Level | Grant Method | Purpose |
|-----------|-------|-------------|---------|
| Microphone | User (TCC) | macOS auto-prompts on first use | Capture user's voice |
| Screen Recording | User (TCC) | Manual grant in System Settings | Core Audio Taps (audio only, no screen content) |
| Network | User | Automatic (no prompt) | Outbound HTTPS/WSS to Deepgram + Claude APIs |
| File System | User | Automatic | Write to Obsidian vault + `~/.heimdall/` config |

**Explicitly NOT required:**
- No kernel extensions (unlike BlackHole/Soundflower)
- No virtual audio drivers
- No root/sudo access
- No packet capture or network sniffing
- No accessibility permissions
- No SUID/SGID binaries

> **Note on "Screen Recording"**: The macOS permission is misleadingly named. Heimdall captures system audio only. Apple bundles system audio capture under Screen Recording permissions because Core Audio Taps is part of the screen capture framework. This is documented and explained to users during setup.

---

## Web Dashboard — Deferred

**Decision**: CLI-first for v1.0. Web dashboard is deferred to post-v1.0.

**Rationale from the project owner**: "CLI + Web Dashboard would be immense. But we could first start with CLI and after finishing the tests and make sure it's working go into making a Dashboard since it's not mandatory. More like a distribution concern."

**What the dashboard would provide (future)**:
- Visual meeting history and search
- Speaker analytics across meetings
- Action item tracking
- Meeting insights over time

**Why it's straightforward later**: The dashboard is a presentation layer on top of the same data (JSON files, Obsidian markdown, meeting metadata). The CLI establishes the data model; the dashboard just renders it differently.

---

## Development Approach — The-Matrix Ecosystem

Heimdall is built entirely using the-matrix autonomous agent ecosystem. Minimal handwritten code.

**The-matrix tools involved:**

| Tool | Role in Heimdall Development |
|------|------------------------------|
| `neo init` | Provision the `.claude/` agent ecosystem for the heimdall repo |
| `oracle research` | Synthesize Go audio/WebSocket/streaming best practices as knowledge docs |
| `morpheus loop` | Autonomous development loop (implement → test → review) |
| Developer agents | Write all Go + Swift source code |
| Tester agents | Write and run all tests |
| Reviewer agents | Quality gate every PR |
| Security agents | OWASP + ASI audit before merge |

**Workflow**: The user (Omer Ufuk, Head Engineer) architects the system and makes design decisions. The-matrix manager agent orchestrates the full development pipeline. This is the ultimate dogfood — a real product built entirely by the autonomous ecosystem.

**Development principles**:
- Almost no handwritten code — everything goes through the agent pipeline
- The user reviews architecture, makes decisions, approves PRs
- Agents write code, tests, and review each other's work
- The-matrix CLI tools provision the development infrastructure

---

## Target Audience

**"The meeting companion for engineers who use Obsidian."**

Primary users:
- Developers and engineering managers
- Technical PMs and knowledge workers
- People who use Obsidian as their second brain
- People who run Claude Code in the terminal
- Attend 5-15 meetings per week
- Care about data privacy and tooling control
- Want structured, searchable meeting records

This is NOT a replacement for Granola or Otter for non-technical users. It is the tool that the Obsidian + CLI + Claude audience didn't know they needed.

---

## Competitive Positioning

| | Heimdall | Otter.ai | Granola | Meetily |
|--|---------|----------|---------|---------|
| Type | CLI | SaaS | Desktop app | Open source |
| Price | ~$24/mo API costs | $16.99/mo | $18/mo | Free (local) |
| Privacy | Local + API (user's keys) | Cloud (data concerns) | Device-native | 100% local |
| Obsidian | Native integration | No | MCP only | No |
| CLI | Yes | No | No | No |
| Open Source | Yes (MIT) | No | No | Yes |
| Speaker Recognition | LLM-contextual (v1), voice enrollment (v0.3) | Yes | Yes | Yes (pyannote) |
| Distribution | `brew install` | App/web | App | Docker/Python |

**Unique value**: The only tool at the intersection of CLI-native + Obsidian-first + Claude-powered + open-source.
