**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-04-18
**Authors:** Omer Ufuk

---

# Heimdall — Architectural Decisions

> Architectural decisions (AD-001 to AD-010) are documented in `docs/architecture/DECISIONS.md`.
> This file tracks IMPLEMENTATION decisions made during development.

---

## Architecture Decision Index

| ID | Decision | Reference |
|----|----------|-----------|
| AD-001 | Go + Swift dual-binary architecture | `docs/architecture/DECISIONS.md` |
| AD-002 | Deepgram as primary STT provider | `docs/architecture/DECISIONS.md` |
| AD-003 | Provider abstraction layer (3 interfaces) | `docs/architecture/DECISIONS.md` |
| AD-004 | Post-meeting Claude analysis (not real-time) | `docs/architecture/DECISIONS.md` |
| AD-005 | File-based Obsidian integration (no plugin) | `docs/architecture/DECISIONS.md` |
| AD-006 | MIT license (tentative) | `docs/architecture/DECISIONS.md` |
| AD-007 | Dual-channel stereo (L=system, R=mic) | `docs/architecture/DECISIONS.md` |
| AD-008 | LLM contextual speaker identification for MVP | `docs/architecture/DECISIONS.md` |
| AD-009 | CLI-first, dashboard deferred | `docs/architecture/DECISIONS.md` |
| AD-010 | macOS 14.2+ minimum | `docs/architecture/DECISIONS.md` |

---

## Implementation Decisions

> Record implementation decisions here as development progresses. Use format:
>
> ### ID-{NNN}: {Title}
> **Date**: YYYY-MM-DD
> **Context**: Why this decision was needed.
> **Decision**: What was decided.
> **Consequences**: What changes.

### ID-001: Mono + Diarize over Stereo + Multichannel

**Date**: 2026-04-01 (PR #11, commit e24110d)
**Supersedes**: AD-007 (Dual-Channel Stereo Audio)

**Context**: AD-007 specified stereo (L=system, R=mic) with `multichannel=true` sent to Deepgram. This produced empty transcripts in real-voice testing because Deepgram's `multichannel=true` and `diarize=true` parameters conflict: with multichannel enabled, diarization is applied per channel and capped at the number of channels (2). For N-speaker meetings on a single remote channel, all remote voices collapsed into one "Speaker 0", defeating speaker separation.

**Decision**: The mixer still produces stereo (L=system, R=mic) internally for potential future use, but `internal/session/session.go` downmixes to mono before handing off to Deepgram. TranscribeOpts sends `channels=1`, `diarize=true`, no `multichannel`. Deepgram's diarization separates speakers by voice fingerprint instead of channel index, supporting N speakers on a single stream.

**Consequences**:
- Fixed the empty-transcript bug for real multi-speaker meetings
- Speaker IDs now come from Deepgram's voice fingerprinting (per-connection, not stable across reconnections — acknowledged limitation)
- Stereo billing savings: mono rate ($0.58/hr) instead of stereo ($1.16/hr) — see AD-002 amendment
- Simpler Deepgram URL construction

### ID-002: Raw HTTP Client for Anthropic (no official SDK)

**Date**: 2026-03-28 (initial implementation)

**Context**: MASTER_PLAN task 1B.1 specified `github.com/anthropics/anthropic-sdk-go` as the integration library.

**Decision**: `internal/analyzer/claude.go` uses `net/http` with hand-rolled JSON marshaling against the Anthropic REST endpoint (`https://api.anthropic.com`), setting `Anthropic-Version: 2023-06-01` manually.

**Rationale**:
- Better testability via `httptest` (mock HTTP responses directly)
- One fewer external dependency
- Anthropic's REST API is small and stable for the two endpoints we use (messages, models list)
- Official SDK adds surface area (async APIs, streaming iterators) we do not need

**Consequences**: Must track Anthropic-Version header manually when the API evolves. Acceptable given the narrow usage.

### ID-003: gorilla/websocket for Deepgram (no official SDK)

**Date**: 2026-03-28 (initial implementation)

**Context**: MASTER_PLAN task 0.8 specified `github.com/deepgram/deepgram-go-sdk` as the integration library.

**Decision**: `internal/transcriber/deepgram.go` uses `github.com/gorilla/websocket` directly, constructing the Deepgram WebSocket URL and JSON message frames by hand.

**Rationale**:
- Full control over reconnection logic (V-001 55-minute proactive reconnection, V-005 exponential backoff)
- Full control over the URL query parameter set (`diarize`, `keywords`, `mip_opt_out`) (`multichannel` intentionally omitted — see ID-001)
- Smaller dependency footprint
- Deepgram's WebSocket JSON protocol is stable and well-documented

**Trade-off**: gorilla/websocket is in maintenance mode. Tracked in `.claude/KNOWN_ISSUES.md` as acceptable tech debt; migration to `coder/websocket` is a post-v0.1 option.

### ID-004: Meeting Profiles in Config (PR #14)

**Date**: 2026-04-04 (PR #14, commit 1a67cbb)

**Context**: Users with recurring meeting types (daily standup, 1:1s, sprint planning) had to re-specify `--language`, `--participants`, `--keywords`, `--title` on every invocation.

**Decision**: Added a `profiles:` map to config.yaml. Each profile can set title, language, participants, and keywords. The `heimdall record --profile daily` flag applies the profile; explicit CLI flags override profile values. Two new subcommands: `heimdall config add-profile <name>` and `heimdall config profiles`.

**Consequences**: Zero-flag daily workflow for the common case. Config schema extended; existing configs without `profiles:` continue to work.

### ID-005: ClaudeCodeAnalyzer -- subprocess backend as a second Analyzer implementation

**Date**: 2026-09-13

**Context**: The only `Analyzer` implementation (`ClaudeAnalyzer`) requires `ANTHROPIC_API_KEY` and bills per token. A user who already pays for a Claude subscription (Pro/Max/Team) and has Claude Code installed has no way to reuse that access for meeting analysis -- they'd need a second, separate credential and a second bill.

**Decision**: Added `ClaudeCodeAnalyzer` (`internal/analyzer/claudecode.go`), selected via `--analyzer claude-code` (default remains `api`). It shells out to the user's own `claude` binary in non-interactive print mode: `claude -p --bare --restricted --permission-prompts none --output-format json --system-prompt <systemPrompt>`, piping the transcript over stdin (not argv, to avoid any risk of OS argument-length limits on long meetings) and parsing the `{"result": ..., "is_error": ...}` JSON envelope. `--bare` skips hook/CLAUDE.md/plugin discovery so a meeting transcript can't pick up unrelated project context; `--restricted` plus `--permission-prompts none` remove tool-execution surface entirely, since this is a pure text-in/JSON-out completion with no legitimate reason to invoke a tool. heimdall never sees or handles the user's Claude credential -- it only asks an already-authenticated local process to run once and exit.

The retry/backoff/parse/fallback orchestration (V-009) was extracted out of `ClaudeAnalyzer.Summarize` into a shared `summarizeWithRetry(ctx, segments, opts, callOnce)` helper so both backends share identical failure-degradation behavior; each backend only supplies its own `callOnce` closure (HTTP call vs. subprocess call). `buildUserPrompt`, `systemPrompt`, and the response parser (`parseAnalysisResponse`) were already backend-agnostic and needed no changes.

**Consequences**:
- `Analyzer` now has two implementations selectable via `analyzer.NewFromName(name, apiKey)`, mirroring `transcriber.NewFromName`'s provider-factory pattern.
- Model selection differs by design: the `api` backend falls back to the package `DefaultModel` when unset; `claude-code` leaves `--model` unset when `opts.Model == ""` so it defers to the user's own Claude Code default rather than forcing a specific model choice onto their already-configured setup.
- `config.Validate()`'s Claude-model check was loosened from an exact-enum allowlist (which had already gone stale twice in this project's history) to a `"claude-..."` shape check, and `claude.api_key` is only required when `claude.analyzer != "claude-code"`.
- `doctor` now reports `claude` CLI availability and only fails its Claude-analysis check when *neither* `ANTHROPIC_API_KEY` nor a working `claude` CLI is present.
- Not yet covered: a live end-to-end test against a real authenticated `claude` CLI (the sandbox this was built in has no logged-in session to test against) -- unit tests cover the subprocess contract via an injectable `commandRunner`, verified empirically against the real CLI's JSON envelope shape, but a first real run should be smoke-tested by a user with an active Claude Code login.
