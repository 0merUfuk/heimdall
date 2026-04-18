# Heimdall — Phased Roadmap

**Version**: 1.1
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

## Overview

The roadmap progresses from a 1-week validation spike through a full-featured v1.0, then into speaker recognition and intelligence features. Each phase has a clear goal and kill criteria.

---

## Phase 0: Spike (1 week)

> **SHIPPED** (all deliverables complete as of 2026-04-18). This phase proved the end-to-end audio → transcript pipeline and gated everything downstream. See `docs/MASTER_PLAN.md` Progress Tracker for per-subtask status.

**Goal**: Prove the audio capture → Deepgram streaming pipeline works end-to-end.

**Deliverables**:
- [x] Swift audio helper captures system audio from a Zoom/Meet call
- [x] Go binary spawns Swift helper, reads PCM from stdout pipe
- [x] PCM streamed to Deepgram WebSocket, diarized transcript returned
- [x] Diarized transcript printed to terminal in real-time
- [x] Microphone capture via malgo working alongside system audio

**Kill criteria**:
- If Deepgram diarization quality on real meeting audio is below 80% accuracy → re-evaluate API choice
- If Core Audio Taps cannot reliably capture from common meeting apps → re-evaluate capture approach
- If dual-channel stereo interleaving causes issues with Deepgram → fall back to mono

**What this does NOT include**: Claude integration, Obsidian output, config wizard, crash recovery. Just the raw audio → transcript pipeline.

---

## Phase 1: MVP (3-4 weeks) — v1.0.0

> **SHIPPED** (18/19 subtasks complete as of 2026-04-18). The entire record-to-Obsidian pipeline is functional; distribution/Homebrew publishing is the only outstanding item (MASTER_PLAN subtask 1E.2). A few originally-scoped features were deferred to later phases (see struck-through items below) and moved to Phase 2+.

**Goal**: Complete record-to-Obsidian pipeline. A usable tool.

**Deliverables**:
- [x] `heimdall record` command with live terminal transcript display
- [x] System audio + microphone dual-channel capture
- [x] Deepgram streaming STT + diarization with 60-minute reconnection (V-001)
- [x] Network disruption reconnection + audio buffering (V-005)
- [x] Swift subprocess crash detection + restart (V-002)
- [x] Claude post-meeting summarization with structured output
- [x] LLM-based contextual speaker identification
- [x] Obsidian vault markdown output with Go templates
- [x] `heimdall config init` wizard
- [x] `heimdall doctor` diagnostic command (V-003, V-019, V-020)
- [ ] ~~`heimdall status` — show API key validity, audio devices, vault path~~ _(deferred — `heimdall doctor` covers this in practice; dedicated status command not implemented)_
- [x] `heimdall recover` — crash recovery from temp files (V-006)
- [x] `heimdall analyze` — standalone re-analysis command (V-009) _(separate top-level subcommand `heimdall analyze --file <path>`, registered independently on `rootCmd`; shares source file `cmd/heimdall/recover.go` with `heimdall recover` but has its own cobra.Command, flag set, and RunE)_
- [x] `--participants` flag for speaker identification hints (V-012)
- [x] `--keywords` flag for custom vocabulary (V-023)
- [ ] ~~`--save-audio` flag for optional WAV recording~~ _(deferred; config field `audio.save_recording` exists but is not yet wired to the record command — moved to Phase 2+)_
- [x] File naming collision prevention (V-017)
- [x] Vault path validation at startup (V-016)
- [x] Graceful shutdown sequence (Ctrl+C)
- [x] Claude API retry with fallback to raw transcript (V-009)
- [x] Anti-hallucination prompt engineering (V-013)
- [x] Prompt injection mitigation via structured output (V-014)
- [x] GitHub repo, MIT license, README
- [ ] Homebrew tap initial setup _(outstanding — MASTER_PLAN subtask 1E.2)_

---

## Phase 2: Speaker Recognition + Polish (3-4 weeks) — v2.0.0

**Goal**: Speakers get real names via voice enrollment. Distribution polished.

**Deliverables**:
- [ ] `heimdall enroll --name "Omer"` voice enrollment command
- [ ] ECAPA-TDNN ONNX model for speaker embedding extraction
- [ ] Local voice profile storage (`~/.heimdall/voices/`)
- [ ] Automatic speaker-to-name mapping during/after meetings
- [ ] Combined LLM + voice recognition for 95-99% accuracy
- [ ] `heimdall list` meeting history command
- [ ] `heimdall view` command (opens note in Obsidian via URI scheme)
- [ ] Custom Obsidian templates
- [ ] Audio device hot-swap detection (V-007)
- [ ] Process-specific audio capture (`--app Zoom`) refinement (V-004)
- [ ] Silence detection (V-021)
- [ ] GoReleaser + Homebrew tap polished distribution
- [ ] Comprehensive test suite

---

## Phase 3: Intelligence + Cross-Platform (4-6 weeks) — v3.0.0

**Goal**: The tool gets smarter and works on more platforms.

**Deliverables**:
- [ ] Cross-meeting memory: context from past meetings informs analysis
- [ ] Meeting type detection (standup, planning, 1:1, interview)
- [ ] Automatic action item tracking (which items from past meetings are still open?)
- [ ] Linux support (PulseAudio/PipeWire monitor capture)
- [ ] `--local` mode: Whisper.cpp for offline STT (no Deepgram)
- [ ] Obsidian daily note integration (append summary to today's note)
- [ ] Meeting calendar integration (Google Calendar, iCal)
- [ ] People page auto-creation in Obsidian
- [ ] AssemblyAI as alternative STT provider (Transcriber interface)
- [ ] Bluetooth latency compensation (V-008)
- [ ] Non-English language support improvement (V-022)

---

## Phase 4: Dashboard + Community (ongoing) — v4.0.0+

**Goal**: Web dashboard, sustainable open-source project.

**Deliverables**:
- [ ] Web dashboard (meeting history, search, analytics, action item tracking)
- [ ] Windows support (WASAPI loopback)
- [ ] Real-time summarization (streaming Claude during meeting)
- [ ] Team features: shared meeting library
- [ ] Obsidian plugin for enhanced UX (sidebar, search, live status)
- [ ] Alternative LLM backends (Ollama, GPT-4, Gemini)
- [ ] Alternative STT backends (Gladia, Speechmatics)
- [ ] MCP server for external tool integration
- [ ] Webhook output for Slack/Discord/Notion
- [ ] Software echo cancellation (V-018)

---

## Timeline Estimate

| Phase | Effort | Timeline | Cumulative |
|-------|--------|----------|-----------|
| Phase 0 (Spike) | 1 week | Week 1 | Week 1 |
| Phase 1 (MVP v1.0) | 3-4 weeks | Weeks 2-5 | ~5 weeks |
| Phase 2 (Recognition v2.0) | 3-4 weeks | Weeks 6-9 | ~9 weeks |
| Phase 3 (Intelligence v3.0) | 4-6 weeks | Weeks 10-15 | ~15 weeks |
| Phase 4 (Dashboard v4.0) | Ongoing | Week 16+ | Continuous |

**Total to feature-complete CLI**: ~15 weeks (~4 months)

---

## Version Mapping to Vulnerabilities

| Version | Vulnerabilities Addressed |
|---------|--------------------------|
| v1.0.0 | V-001, V-002, V-003, V-005, V-006, V-009, V-013, V-014, V-015, V-016, V-017, V-019, V-020, V-023, V-024 |
| v2.0.0 | V-004 (refined), V-007, V-012 (voice enrollment), V-021 |
| v3.0.0 | V-008, V-022, V-028 (AssemblyAI fallback) |
| v4.0.0 | V-018 (echo cancellation), V-025 (large file handling) |
