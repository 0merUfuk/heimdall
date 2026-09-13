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

### ID-005: MCP server -- official SDK over hand-rolling, IsError over Go errors for tool-level failures

**Date**: 2026-09-13

**Context**: STRATEGY_V2 Phase 2C and `docs/PRODUCTIZATION.md` v3 both name an MCP server exposing the Obsidian vault as heimdall's core differentiator (the "knowledge graph two-way bridge" -- heimdall writes meeting notes in, this reads them back out for an agent to query). Two decisions needed: how to implement the MCP protocol itself, and how a `heimdall`-side package would read notes it did not itself just write (the vault may contain other, unrelated notes; heimdall's own notes must be identified reliably).

**Decision 1 -- use `github.com/modelcontextprotocol/go-sdk` rather than hand-rolling the protocol.** Every other external integration in this codebase (Deepgram, Soniox, the Anthropic API) hand-rolls its wire protocol deliberately, for testability via `httptest` and to avoid unnecessary dependency surface (see ID-002, ID-003). MCP does not fit that precedent: its JSON-RPC/stdio protocol, capability negotiation, and schema generation are intricate enough that a hand-rolled implementation would trade a one-time integration cost for an ongoing protocol-correctness liability, with no testability upside the way hand-rolling had for plain HTTP/WebSocket (the official SDK also ships its own in-memory transport for exactly this kind of testing -- see `internal/mcpserver/server_test.go`). This is a real, acknowledged increase in dependency surface (one new direct dependency, six new transitive ones) for a project whose "4 direct dependencies" was previously a specifically celebrated strength (`docs/GRILL_REPORT.md`) -- taken deliberately, not accidentally.

**Decision 2 -- tool-level errors are `CallToolResult{IsError: true}`, not Go `error` returns.** The SDK's own doc comment for `CallToolResult.IsError` states plainly that errors originating from the tool belong in `Content` with `IsError` set, not as an MCP protocol-level error response, "Otherwise, the LLM would not be able to see that an error occurred and self-correct." A Go `error` return becomes a protocol-level error reserved for things like "the tool doesn't exist." The first draft of this package got this backwards -- every validation failure (empty query, vault not configured, malformed date) returned a Go error, which would have made every one of those failures invisible to the calling model as anything other than an opaque RPC failure. Caught by reading the SDK's own source (`internal/.../go-mod/.../mcp/protocol.go`), not by guessing, and locked in by `TestIntegration_SearchMeetings_EmptyQueryIsToolError` and its siblings, which assert `err == nil && result.IsError == true` specifically -- a regression back to returning a Go error for these cases would fail that assertion, not just look different.

**Decision 3 -- `internal/vault` identifies heimdall's own notes by frontmatter shape, not a special marker.** A note is treated as "a heimdall meeting" if it parses as `templates/meeting-note.md.tmpl`'s frontmatter shape (date/title/participants/duration/platform/tags); anything else in the meetings folder is silently skipped rather than erroring the whole scan. This means a user's own unrelated notes co-located in the same vault folder don't break `list_meetings`/`search_meetings` -- verified by `TestListMeetings_SkipsNonHeimdallNotes`.

**Consequences**:
- Real, protocol-level verification exists for this feature at two levels: `internal/mcpserver`'s tests connect a real MCP client to the server over the SDK's in-memory transport (exercising actual JSON-RPC, not just Go function calls); development also included a manual smoke test driving the real `heimdall mcp` stdio binary with a hand-built JSON-RPC request sequence, which is what caught two real bugs no unit test could have (see CHANGELOG's Added entry for this feature) -- `heimdall mcp` originally required `DEEPGRAM_API_KEY`/`ANTHROPIC_API_KEY` to even start (it resolved the whole config instead of just the one field it needs), and a normal client disconnect was reported as a fatal error with exit code 1.
- `internal/vault`'s search is a plain case-insensitive substring match over note content, not an index -- fine at the scale of one person's meeting history (hundreds to low thousands of notes), not designed to scale further. If that ever matters, it's a contained change inside one package, not a protocol-level one.
- This PR's `.claude/DECISIONS.md` diff was written against `main` directly (not stacked on the session's other PRs, which independently claim ID-005 through ID-008 on their own branches) -- whichever of these merges second will hit a trivial renumbering conflict on this file, not a real content conflict.
