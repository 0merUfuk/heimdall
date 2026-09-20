**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-09-19
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

### ID-006: `internal/eval` -- deterministic golden-fixture checks as the primary quality signal, LLM-as-judge as opt-in supplement

**Date**: 2026-09-13

**Context**: heimdall had no measurement of Analyze-stage *output quality* at all -- `internal/analyzer`'s tests are httptest-mocked (per `.claude/rules/go-net-http-services.md`'s "tests use mocks, not real external services" rule) and only prove JSON-parsing plumbing works, never whether a real model call produces a faithful, complete, non-hallucinated, correctly-localized note.

**Decision**: Built `internal/eval` around seven hand-written golden transcripts (`fixtures.go`) with *minimum-bar* ground truth (minimum decision/action-item counts, forbidden strings, language markers) rather than exact-match golden outputs -- LLM prose varies run to run even at fixed settings, so exact-string goldens would be permanently flaky. Ten deterministic checks (`checks.go`) run for free against every fixture's output: coverage floors, anti-hallucination on both "should be empty" and "names must be traceable to the transcript" axes, prompt-injection-leak detection (V-014), and a crude multilingual marker check. These gate the `heimdall eval` command's exit code. A separate, explicitly opt-in LLM-as-judge layer (`judge.go`, `--judge` flag) asks `claude-sonnet-5` to score faithfulness/coverage 0-10 for nuance the deterministic layer can't capture -- this costs real tokens and never affects the exit code, so it can't turn a CI job flaky or expensive by accident.

Chose plain Go over a dedicated Python eval framework (promptfoo/deepeval/ragas/...): heimdall is a deliberately lean, few-dependency Go binary (`docs/GRILL_REPORT.md`'s "Meta-Engineering ROI" finding specifically flagged process-proportionality as a past failure mode of this project), and a second language/dependency tree/CI runner to grade output the `heimdall` binary already parses would repeat that mistake. `internal/eval` reuses `analyzer.Analyzer` directly and ships in the same binary.

**Consequences**:
- `heimdall eval` / `make eval` exist as a real regression harness: a maintainer can run it before tagging a release and get an immediate pass/fail plus per-check detail.
- The judge layer is deliberately NOT wired into the public CI workflow (`.github/workflows/ci.yml` has no secrets configured) -- provisioning `ANTHROPIC_API_KEY` as a repo secret for CI-gated judging is a decision left to the maintainer, not defaulted silently into a public workflow.
- First real runs landed 2026-09-19/20 against four backends (local `qwen3:14b`, Anthropic API, Claude Code, Codex) -- numbers in `docs/EVALUATION.md`'s "Measured results". The suite found real defects in both directions: it caught the local model's Turkish failures, and its name-traceability check flags joint owners ("Dana and Lena") as untraceable, a refinement noted there. The first real `heimdall eval` run is the actual quality baseline, not this entry's design intent.

### ID-007: Raw-audio recording as a session-level tap, not a Transcriber

**Date**: 2026-09-13

**Context**: `audio.save_recording` / `audio.recording_path` have existed in the config schema (and README's documented example) since the original MVP spec, but nothing ever read them -- `heimdall record` silently ignored the setting. Separately, while investigating this for a planned local-transcription (Whisper) feature, the `Transcriber` interface (`Connect/Send/Receive/Close`) turned out to be the wrong integration point for anything that transcribes in *batch* rather than streaming: `session.go`'s shutdown sequence (`Stop()`) budgets 30 seconds for `Close()` before `record.go` gives up and logs a timeout warning -- sized for "send CloseStream, wait briefly for final results," not "run a whole local transcription pass over a 1-hour recording." Forcing a batch transcriber through that interface risked silently producing an empty or truncated transcript on any real meeting, which is the same failure class as the project's own historical Bug #1 (empty transcription from a diarize/multichannel conflict, see GRILL_REPORT.md).

**Decision**: Implemented raw-audio saving first, as its own concern, decoupled from transcription entirely. `internal/recording` (`wav.go`) is a minimal, dependency-free 16-bit PCM WAV writer. `MeetingSession` gained a second callback, `OnAudioFrame`, mirroring the existing `OnSegment` pattern exactly -- called from `audioToTranscriber()` with the pre-downmix stereo frame (the mixer's documented 16kHz-stereo output, L=system/R=mic per AD-007) before it's converted for the transcriber. `record.go` wires a `WAVWriter` into that hook when `--save-audio` or `audio.save_recording: true` is set. `session.go` itself stays ignorant of what a caller does with the tapped frame -- persistence policy lives entirely in `cmd/heimdall`, matching the existing separation where `session` only knows "here is a frame/segment."

While building this, a filename bug surfaced: `internal/recovery.sanitizeTitle` still used the pre-V-017 `[^a-zA-Z0-9]+` pattern that strips Turkish characters, even though `internal/output.sanitizeFilename` was fixed to `[^\p{L}\p{N}-]+` back in the 31-bug sweep -- the two copies had drifted, so a Turkish meeting title produced a correct `.md` note filename but a mangled `.json` recovery filename. Consolidated both into one shared `heimdall.SanitizeFilename(title, fallback)`; `internal/recording` uses the same function for WAV filenames.

**Consequences**:
- `audio.save_recording` / `audio.recording_path` / `--save-audio` are now real, tested behavior instead of a documented no-op.
- Local Whisper transcription now has a clear, correct integration point to build against: a *separate* `heimdall transcribe --file <wav>` batch command operating on these saved WAV files, decoupled from the live `record` session's real-time shutdown budget -- not a new `Transcriber` implementation wired into `record --transcriber whisper`. This also naturally reuses `internal/recovery`'s JSON format as the hand-off into the existing `heimdall analyze` command, so a local-Whisper transcript flows through the same downstream path a Deepgram/Soniox transcript does. Implemented immediately after this decision -- see ID-008.
- `internal/recovery` and `internal/output` no longer maintain parallel copies of filename sanitization; a future fix to one applies to both automatically.

### ID-008: `heimdall transcribe` -- whisper.cpp's built-in `--diarize`, verified empirically before writing the parser

**Date**: 2026-09-13

**Context**: ID-007 set up raw-audio recording specifically so local transcription would have a clean, decoupled integration point. Two open questions before implementing it: (1) whisper.cpp has no native N-speaker diarization -- would heimdall need to hand-roll channel-splitting (transcribe the L=system and R=mic channels of the saved WAV separately, then merge by timestamp) to get any speaker separation at all? (2) what is whisper.cpp's actual `--output-json` schema, and is its exit code a reliable success signal? Neither was safe to guess: an invented JSON schema risks silently producing an empty or garbled transcript, the same failure class as this project's own historical Bug #1.

**Decision**: Installed `whisper.cpp` via Homebrew (`brew install whisper-cpp` -- note the formula was renamed from `whisper-cpp` to `whisper.cpp`, though the binary is still `whisper-cli`) and empirically verified both questions against the real binary (v1.9.4) before writing any parsing code:

1. **Diarization**: `whisper-cli --help` revealed a built-in `-di, --diarize` ("stereo audio diarization") flag -- whisper.cpp already does exactly the channel-based speaker separation that was about to be hand-rolled. No channel-splitting code needed; `heimdall transcribe` always passes `--diarize` against the stereo WAV directly.
2. **JSON schema and exit-code reliability**: generated synthetic stereo speech with macOS `say` (two mono clips, one per speaker, merged via `ffmpeg`'s `join` filter) and ran `whisper-cli -m ggml-tiny.bin -f stereo.wav --diarize --output-json` for real. Confirmed the actual schema: `{"transcription": [{"offsets": {"from": <ms>, "to": <ms>}, "text": " ...", "speaker": "0"}, ...]}` (`text` carries a leading space -- a BPE tokenization artifact, trimmed on parse). Then separately ran it against a nonexistent model path: **whisper-cli exited 0 and wrote no output JSON file at all** -- exit code is not a usable success signal. `internal/localstt.Client.TranscribeFile` checks for the output file's existence, not the subprocess's exit status.

**Consequences**:
- `internal/localstt` (`whisper.go`) shells out to `whisper-cli` in batch mode, mirroring `analyzer.ClaudeCodeAnalyzer`'s injectable-`commandRunner` testability pattern. Unit tests use the real, captured JSON fixture verbatim (not a guessed schema) via `realWhisperCLIOutputFixture`.
- `heimdall transcribe --file <wav>` was additionally smoke-tested end-to-end against the real `whisper-cli` binary and a real (tiny) model during development -- not just mocked unit tests -- confirming the full chain (subprocess invocation -> JSON parse -> `heimdall.Segment`s -> `recovery.RecoveryFile` write) produces a valid, `heimdall analyze`-consumable transcript.
- Speaker separation from `heimdall transcribe` is two-party only (system audio vs. microphone, i.e. "everyone else on the call" vs. "you") -- not per-individual diarization the way Deepgram/Soniox's voice-fingerprint diarization is for multi-participant calls. Documented as a known limitation in README and the command's own `--help` text, not glossed over.
- New `heimdall model download <size>` command and `internal/localstt.ModelDownloadURL`/`ResolveModelPath` establish `~/.heimdall/models/ggml-<size>.bin` as the model-storage convention, mirroring `~/.heimdall/{recovery,recordings}/`.

### ID-009: MCP server -- official SDK over hand-rolling, IsError over Go errors for tool-level failures

**Date**: 2026-09-13

**Context**: STRATEGY_V2 Phase 2C and `docs/PRODUCTIZATION.md` v3 both name an MCP server exposing the Obsidian vault as heimdall's core differentiator (the "knowledge graph two-way bridge" -- heimdall writes meeting notes in, this reads them back out for an agent to query). Two decisions needed: how to implement the MCP protocol itself, and how a `heimdall`-side package would read notes it did not itself just write (the vault may contain other, unrelated notes; heimdall's own notes must be identified reliably).

**Decision 1 -- use `github.com/modelcontextprotocol/go-sdk` rather than hand-rolling the protocol.** Every other external integration in this codebase (Deepgram, Soniox, the Anthropic API) hand-rolls its wire protocol deliberately, for testability via `httptest` and to avoid unnecessary dependency surface (see ID-002, ID-003). MCP does not fit that precedent: its JSON-RPC/stdio protocol, capability negotiation, and schema generation are intricate enough that a hand-rolled implementation would trade a one-time integration cost for an ongoing protocol-correctness liability, with no testability upside the way hand-rolling had for plain HTTP/WebSocket (the official SDK also ships its own in-memory transport for exactly this kind of testing -- see `internal/mcpserver/server_test.go`). This is a real, acknowledged increase in dependency surface (one new direct dependency, six new transitive ones) for a project whose "4 direct dependencies" was previously a specifically celebrated strength (`docs/GRILL_REPORT.md`) -- taken deliberately, not accidentally.

**Decision 2 -- tool-level errors are `CallToolResult{IsError: true}`, not Go `error` returns.** The SDK's own doc comment for `CallToolResult.IsError` states plainly that errors originating from the tool belong in `Content` with `IsError` set, not as an MCP protocol-level error response, "Otherwise, the LLM would not be able to see that an error occurred and self-correct." A Go `error` return becomes a protocol-level error reserved for things like "the tool doesn't exist." The first draft of this package got this backwards -- every validation failure (empty query, vault not configured, malformed date) returned a Go error, which would have made every one of those failures invisible to the calling model as anything other than an opaque RPC failure. Caught by reading the SDK's own source (`internal/.../go-mod/.../mcp/protocol.go`), not by guessing, and locked in by `TestIntegration_SearchMeetings_EmptyQueryIsToolError` and its siblings, which assert `err == nil && result.IsError == true` specifically -- a regression back to returning a Go error for these cases would fail that assertion, not just look different.

**Decision 3 -- `internal/vault` identifies heimdall's own notes by frontmatter shape, not a special marker.** A note is treated as "a heimdall meeting" if it parses as `templates/meeting-note.md.tmpl`'s frontmatter shape (date/title/participants/duration/platform/tags); anything else in the meetings folder is silently skipped rather than erroring the whole scan. This means a user's own unrelated notes co-located in the same vault folder don't break `list_meetings`/`search_meetings` -- verified by `TestListMeetings_SkipsNonHeimdallNotes`.

**Consequences**:
- Real, protocol-level verification exists for this feature at two levels: `internal/mcpserver`'s tests connect a real MCP client to the server over the SDK's in-memory transport (exercising actual JSON-RPC, not just Go function calls); development also included a manual smoke test driving the real `heimdall mcp` stdio binary with a hand-built JSON-RPC request sequence, which is what caught two real bugs no unit test could have (see CHANGELOG's Added entry for this feature) -- `heimdall mcp` originally required `DEEPGRAM_API_KEY`/`ANTHROPIC_API_KEY` to even start (it resolved the whole config instead of just the one field it needs), and a normal client disconnect was reported as a fatal error with exit code 1.
- `internal/vault`'s search is a plain case-insensitive substring match over note content, not an index -- fine at the scale of one person's meeting history (hundreds to low thousands of notes), not designed to scale further. If that ever matters, it's a contained change inside one package, not a protocol-level one.
- This entry was originally drafted as ID-005 (written against `main` directly, not stacked on the session's other PRs that independently claimed ID-005 through ID-008 on their own branches); renumbered to ID-009 during merge to resolve the collision -- a trivial renumbering, not a content conflict, exactly as anticipated when this was written.

### ID-010: Analyzer cost/latency observability -- log token counts, not a computed dollar estimate

**Date**: 2026-09-13

**Context**: `internal/analyzer/claude.go`'s `apiResponse` struct already parsed `usage.input_tokens`/`usage.output_tokens` from every Anthropic API response, but nothing ever read those fields after parsing -- no logging, no surfacing, nothing. There was zero cost/latency observability anywhere in the one LLM stage of the pipeline.

**Decision**: `callAPI` now wraps its implementation (renamed `doCallAPI`) with timing, and logs `model`, `input_tokens`, `output_tokens`, and `latency` on every successful call. Deliberately logs raw token counts, not a computed dollar figure. This project's own history is the reason: `docs/GRILL_REPORT.md` found `AD-002`'s hardcoded Deepgram price was wrong (claimed mono rate, Deepgram actually bills stereo+diarization at 2x that) and had to be corrected after the fact. Baking a *second* hardcoded price constant into this logging path would just be a second place for that exact kind of drift to go unnoticed -- token counts never go stale, a `$/token` constant does. README's "Cost Per Meeting" table remains the one place a dollar estimate lives, updated by hand when pricing changes.

**Consequences**:
- Real per-call observability exists now (`analyzer: model=claude-haiku-4-5 input_tokens=1842 output_tokens=412 latency=1.203s`), usable for debugging slow/expensive analyses or a future `heimdall stats`-style command, without owning a pricing table that can silently go wrong.
- `ClaudeCodeAnalyzer` (the `--analyzer claude-code` subprocess backend) is not covered by this change -- its own `claude -p --output-format json` envelope already reports `total_cost_usd` directly from the CLI's own live accounting, which is authoritative in a way a hardcoded constant here could never be. Wiring that through is a natural next step now that both are merged to the same base, not duplicated logic to add now.
- `doCallAPI` is a pure internal refactor (return signature grew a value, not observably different to any caller) -- covered by two new tests asserting the log line's exact content on success and its absence on total failure, not just that `Summarize` still returns the right note.

### ID-011: `OllamaAnalyzer` -- on-device analysis over Ollama's native API, never over its OpenAI/Anthropic-compatible endpoints

**Date**: 2026-09-19

**Context**: Stage 5 was the last stage with no offline path: with `heimdall transcribe` (ID-008) audio and transcript could stay on-device, but analysis always went to Anthropic (API or `claude` CLI). The goal is privacy and offline capability, not cost -- `claude-haiku-4-5` is already cheap. Before choosing an integration point, the candidate endpoints were probed on Ollama 0.20.2 (M4 Pro, 24 GB) with the real `systemPrompt` and real transcripts:

| Probe | Result |
|---|---|
| `/v1/messages` (Anthropic-compatible) with `ClaudeAnalyzer`'s exact request shape | Works -- a base-URL override would have been zero new code |
| Short fixture via `/v1/chat/completions` (plain and `json_schema`) and native `/api/chat` + `format` | Valid JSON every time, correct extraction |
| **60-minute transcript (12,971 tokens) via `/v1/chat/completions`** | Server log `truncating input prompt limit=4096 prompt=12971 keep=4`. Response was **valid, plausible JSON with the meeting's early action item silently missing** -- and the front-truncation also dropped the system prompt (anti-injection rules, schema) |
| Same transcript via native `/api/chat` with `options.num_ctx=16384` | All 12,971 tokens read, every planted item extracted, 55s |

**Decision**:
1. **Native `/api/chat`, not the portable endpoints.** Only the native API sets the context window per request; the OpenAI- and Anthropic-compatible endpoints inherit the server default and truncate silently. The failure mode is not "bad JSON" (detectable) but "good-looking, incomplete notes" (not detectable downstream). Native also gives decode-time schema enforcement (`format`), `think:false` for reasoning models, and `prompt_eval_count` for a truncation guard. Cost: LM Studio is not supported. It was not installed, so any LM Studio support would have been untested, and its OpenAI-compatible endpoint fixes context at model-load time anyway, so "portability" would not have solved the real problem there either (unverified inference).
2. **Size, never truncate.** `num_ctx` is sized to the prompt (conservative bytes/3 estimate + 4096 output reserve) in fixed buckets (8K/16K/32K/40K/...) so repeat runs reuse a warm model, capped by the model's trained context (`/api/show`) and `ollama.max_context` (default 32768). A transcript that does not fit is **never sent**: the V-009 fallback note is returned with an actionable message. After the call, `prompt_eval_count >= num_ctx` or `done_reason == "length"` is treated as a failure, never rendered.
3. **Single pass; map-reduce deferred.** A measured 60-minute English meeting needs ~13K real tokens and fits one pass. Map-reduce is the highest quality risk (merge passes drop detail) and is not needed until real meetings overflow; the clean refusal above makes that visible when it happens.
4. **No automatic cloud fallback.** A user who chose on-device analysis for privacy must never have the meeting uploaded because something failed locally. Local failure produces the existing raw-transcript note; the recovery transcript is **kept** and the CLI prints an explicit, user-initiated `heimdall analyze --file <path> --analyzer api` retry. A non-loopback `ollama.base_url` is labeled "REMOTE" in the CLI output so the privacy claim is always true.
5. **Default model `qwen3:14b`** (9.3 GB Q4_K_M, 40K context) -- the smallest tier that passes the eval suite; see results below. `qwen2.5:7b` was measured and rejected.

**Consequences**:
- `heimdall transcribe` + `heimdall analyze --analyzer ollama` is a fully offline path end to end (audio, transcript, and analysis never leave the machine). Live `heimdall record` still needs a cloud transcriber (Deepgram/Soniox) -- out of scope here.
- Two pre-existing bugs fixed along the way because the fallback story depends on them: (a) `record` and `analyze`/`recover` deleted the recovery transcript after writing a *fallback* note, destroying the only re-analyzable copy of a failed meeting; (b) `claude.analyzer` from config was honored only by `record`, although README documented it for `recover`/`analyze` too, and `config set claude.analyzer` was not a settable key.
- The shared prompt now names the output language ("Turkish (tr)") instead of passing the bare code: measured on `qwen3:14b`, "in tr" produced English summaries and "in Turkish (tr)" produced Turkish. `--language multi` gets an explicit "main language spoken in the meeting" instruction: no effect on `qwen3:14b`, but it took `claude-haiku-4-5` from 0/3 to 3/3 Turkish summaries on a code-switched meeting. The language code is also sanitized like `--participants`/`--keywords` now (it was the one prompt input that was not).
- Analysis timeouts are per backend (`analyzer.DefaultTimeoutFor`): API/claude-code keep 120s; Ollama gets 20 min (cold model load + minutes of inference on long meetings).

**Measured results** (M4 Pro 24 GB, temperature 0, 3 runs each, byte-identical -- full detail in `docs/EVALUATION.md`):

| | `qwen3:14b` (default) | `qwen2.5:7b` |
|---|---|---|
| Golden fixtures | 5/7 (both misses Turkish) | 0/7 |
| Valid JSON, first attempt | 21/21 | 21/21 |
| 60-min meeting, planted-item recall | EN 3/4 (mid-meeting item missed), TR 4/4 | -- |
| 60-min meeting latency | EN 2 m 14 s, TR 3 m 40 s | 55 s (probe, EN at lower density) |
| 120-min meeting | refused, nothing sent (~45K tokens > 40,960) | -- |

Against the local-analyzer brief's bars: JSON validity 100% (bar >= 98%) and the offline end-to-end proof (sandboxed, no network) pass; long-meeting action-item recall is 83% against both planted ground truth and the Haiku baseline measured later the same day (bar >= 85%; see `docs/EVALUATION.md`); the blind summary A/B is the owner's call.

### ID-012: `CodexAnalyzer` and a cost-tiered Codex model policy

**Date**: 2026-09-19

**Context**: The owner uses Codex as a second execution runtime and asked for "full Codex compatibility" at the lowest reliable cost, mirroring the Claude/Haiku default. Two separate things: (1) Codex as an analysis backend for heimdall users with a ChatGPT/Codex plan, and (2) Codex as a developer on this repository. The repo's `.codex/` mirror had been produced by a blind "Claude"->"Codex" rename (`.Codex/SERVICE_CONTEXT.md` paths that do not exist, "Anthropic Codex (REST)", "analyzes via Codex") and set no model anywhere, so every Codex agent inherited whatever the developer's global Codex default is -- often a high or maximum reasoning effort, the most expensive way to run routine work.

**Decision**:
1. **`--analyzer codex`** shells out to `codex exec`, the Codex counterpart of `claude-code` (ID-005), sharing `summarizeWithRetry`. Default model `gpt-5.6-luna` ("fast and affordable" tier) at `model_reasoning_effort="low"` -- a bounded extraction task does not need a frontier model or deliberate reasoning. Every flag was verified against codex-cli 0.154.0: `--ignore-user-config` (the global model/effort/hooks/MCP servers do not apply; auth still does), `--strict-config` (a future Codex that drops `developer_instructions` fails loudly instead of silently analyzing without the anti-injection rules -- the key was verified to exist because `--strict-config` rejects unknown keys), `-c developer_instructions=<systemPrompt>` (keeps the system/transcript split, V-014), `-c project_doc_max_bytes=0`, `-C <empty 0700 temp dir>`, `--sandbox read-only`, `--ephemeral`, `--ignore-rules`. The answer is read from `-o` (`--output-last-message`); the `--json` event stream is used only for failures (`turn.failed`, or `error` with no `turn.completed`) and token usage.
2. **No `--output-schema` for Codex.** OpenAI structured outputs require `additionalProperties:false` on every object, which cannot express `speaker_map`'s dynamic keys; enforcing it would need a backend-specific response rewrite. Codex follows the prompt-described schema exactly like the claude-code backend does.
3. **Cost-tiered Codex models for development** (all verified to load: Codex deserializes agent files strictly -- a planted unknown key produced "Ignoring malformed agent role definition", the real files produce zero warnings):
   - Project `.codex/config.toml` (verified loaded for this repo only): main session `gpt-5.6-terra` at `medium`.
   - `gpt-5.6-terra`/`medium` for roles whose mistakes ship bugs or bad decisions: developer, reviewer, security-reviewer, tech-lead, strategist, architect.
   - `gpt-5.6-luna`/`medium` for bounded, procedural roles: manager, tester, product-lead, growth-lead.
   - `sandbox_mode = "read-only"` for the roles whose Claude counterparts are read-only (reviewer, security-reviewer, tech-lead, strategist).
   - The frontier tier (`gpt-6-astra`) and high/xhigh/ultra effort are never defaults; escalate per task on the command line.
4. `AGENTS.md` rewritten against current reality: shared project state lives in `.claude/` (Codex agents read it there), Codex agent definitions in `.codex/agents/`, and the auto-loaded Claude rules (`.claude/rules/*.md`) are listed as required reading because Codex does not auto-load them.

**Consequences**:
- Verified live after the usage limit reset: 6/7, 6/7, 5/7 on the eval, 21/21 valid JSON, 6-16 s per fixture; the success event shapes (`item.completed` agent_message, `turn.completed` usage) match the parser. Each call carries ~16.5K input tokens of Codex's own prompt -- the "cheapest model" default keeps the per-token price low, but Codex is not the lowest-overhead backend.
- The global `~/.codex/AGENTS.md` IS injected despite `project_doc_max_bytes=0` (canary-verified), and no config key or feature flag disables it; impact measured and documented in `.claude/KNOWN_ISSUES.md`.
- No pricing data was available locally; the tiering relies on the model catalog's own descriptions ("fast and affordable" vs "balanced"). Slugs come from the local `models_cache.json` as of 2026-09-19 and will need updating when Codex retires them.

### ID-013: Cloud environments (Codex cloud, Claude Code on the web) via one setup script

**Date**: 2026-09-19

**Context**: Both cloud agents run Linux containers. `go build` of this repo fails on Linux without cgo (`internal/audio` uses malgo), the module requires Go 1.27.1 (newer than typical images), the official `golang` images set `GOTOOLCHAIN=local`, and Codex cloud's agent phase has no internet by default.

**Decision**: `scripts/cloud-setup.sh` (idempotent, Linux-only) installs the exact go.mod Go version when the image's is older -- preferably as Go's own toolchain module through `proxy.golang.org`, because Claude Code's cloud allowlist (verified in its docs) includes `proxy.golang.org` but not `dl.google.com`, where `go.dev/dl` tarball downloads redirect; the tarball is only the fallback for images with no Go >= 1.21 -- ensures a C compiler for cgo, downloads modules, and builds + vets every package during setup. No build tags or code changes were needed: with cgo and a compiler, every package (malgo included) builds, and the **entire test suite passes on Linux under `-race`** (verified in `ubuntu:24.04` and `golang:1.24` on arm64; setup plus the changed packages' tests also verified on `linux/amd64` under emulation, the likely Codex cloud architecture). Claude Code on the web runs it from a committed `.claude/settings.json` SessionStart hook (`--if-remote`: a no-op unless `CLAUDE_CODE_REMOTE=true`); Codex cloud runs it as the environment's setup script.

**Consequences**:
- Claude Code on the web's documented contract matches this design: the VM sets `CLAUDE_CODE_REMOTE=true`, repo `.claude/settings.json` SessionStart hooks run in single-repo cloud sessions, setup scripts run as root on Ubuntu 24.04 (the image tested here), and Go + GCC are preinstalled. Verified with `go.dev`/`dl.google.com` made unreachable in the container: the toolchain-module path still installs Go 1.27.1 and the build passes.
- Two real portability bugs were found only by running the script in containers, not by reading it: Ubuntu's `~/.bashrc` returns early for non-interactive shells (an appended PATH line never runs in agent shells), and an older Go earlier on PATH (`/usr/local/go/bin` in `golang:1.24`) shadowed the new one. The script now symlinks into `/usr/local/bin` and repoints stale Go binaries -- acceptable only because it exits on anything but Linux containers.
- A cloud container can build, vet, lint, run all unit tests, and run `heimdall eval` against a remote backend. It cannot capture audio (Core Audio Taps / Swift helper are macOS-only), run whisper.cpp, or reach a local Ollama.
- Security review (2026-09-20) raised the fallback toolchain download: the preferred module-proxy path is verified against Go's checksum database, but the `go.dev` tarball fallback was not. Its SHA-256 sums are now pinned in the script (from `https://go.dev/dl/?mode=json`) and verified before extraction; a version whose sums are not pinned refuses the tarball path rather than installing an unverified toolchain. Verified in containers, including a deliberately tampered pin (install refused, no Go on PATH afterwards).
- The same review flagged the committed SessionStart hook as a supply-chain surface: it runs `scripts/cloud-setup.sh` automatically in cloud sessions, with root in the container. The script is deliberately small, logs every privileged action (package install, `/usr/local/bin` symlinks, rc-file edits), and is Linux-container-only. **Any change to `scripts/cloud-setup.sh` or `.claude/settings.json` deserves the same review bar as a CI workflow file.**

### ID-014: `record --transcriber whisper` -- fully offline meeting capture

**Date**: 2026-09-19

**Context**: ID-011 made analysis offline and ID-008 made transcription offline, but only for an existing WAV -- and the only way to capture a meeting, `heimdall record`, refused to start without a working Deepgram or Soniox key. So "a meeting that never leaves the machine" was not actually possible. This surfaced concretely when the only Deepgram key available for a live test was rejected (401).

**Decision**: `--transcriber whisper` selects `transcriber.CaptureOnlyTranscriber`, a Stage 3 stand-in that transcribes nothing live (no network, no key). The session captures and mixes as usual; the audio is always saved (a WAV-creation failure is fatal before the meeting starts, because the WAV is the only record), and after the stop the record command runs the existing batch `internal/localstt` over it, writes the recovery transcript, and analyzes it with the selected backend. `whisper-cli` and the model are checked *before* the meeting. This is ID-007's batch design reached from `record`, not a streaming Whisper transcriber. With `--analyzer ollama` the meeting never leaves the machine.

**Consequences**:
- No live transcript in this mode; the terminal says so.
- The saved WAV is now checkpointed every 30 seconds (`WAVWriter.Checkpoint`, V-006's cadence) in every `--save-audio` recording: previously the header's size was only written at `Close()`, so a crash mid-meeting left a file claiming zero bytes of audio -- in whisper mode, the only copy of the meeting. A failed start no longer leaves an empty WAV behind.
- Live capture needs macOS Microphone and "Screen & System Audio Recording" permission for the app that runs heimdall (unchanged requirement, but now the only one for an offline meeting).

### ID-015: `--analyzer claude-code` without `--bare`

**Date**: 2026-09-19

**Context**: `ClaudeCodeAnalyzer` passed `--bare`, and Claude Code 2.1.271's own `--help` says that under `--bare` auth is "strictly ANTHROPIC_API_KEY or apiKeyHelper (OAuth and keychain are never read)" -- so the backend could never use the subscription login it exists for (ID-005). But `--bare` was also its isolation: no hooks, plugins, CLAUDE.md, or auto-memory in a call that carries a meeting transcript.

**Decision**: Drop `--bare`; rebuild the isolation from narrower, verified pieces: `--restricted` (ignores user/project/local settings, so their hooks and enabled plugins do not load), `--strict-mcp-config`, `--no-session-persistence`, `--disable-slash-commands`, an empty private working directory, and `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`, `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` -- each variable name confirmed present in the installed binary (a documentation summary had offered two that do not exist).

**Evidence**: against a live model, with a throwaway Claude config dir containing a user-level `CLAUDE.md` secret word and SessionStart/UserPromptSubmit hooks: plain `claude -p` answered the secret word and fired both hooks; heimdall's invocation answered "NONE" and fired neither. `heimdall eval --analyzer claude-code --model claude-haiku-4-5` then passed 7/7 through the real binary, with no session written to disk.

**Consequences**: the subscription (OAuth) path itself could not be exercised -- the machine's `claude` CLI was not logged in, and runs used API-key auth -- so it rests on Claude Code's documented behavior without `--bare`. The first `claude /login` user confirms it.
