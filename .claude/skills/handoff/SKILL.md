---
description: >
  Package the current Heimdall session into a full, drift-verifiable handoff
  plus a lean resume summary. Use before context compaction, /clear, switching
  agents, or ending a multi-step development/release/research session.
argument-hint: "[optional-handoff-name]"
allowed-tools: Read, Glob, Grep, Write, Edit, Bash(git status:*), Bash(git branch:*), Bash(git rev-parse:*), Bash(git log:*), Bash(git diff:*), Bash(go test:*), Bash(make:*), Bash(gh pr:*), Bash(gh release:*)
---

# /handoff -- Verified Session Handoff

## Purpose

Create a handoff the next Claude Code, Hermes, or Codex session can trust *after verification*. This is not a casual summary. It is a state package with a pasteable loader prompt, a Do Not Re-Do register, closed open-question register, artifact map, and drift checks.

## Source Hierarchy

Read in order:

1. `git status --short`, `git branch --show-current`, `git rev-parse HEAD`, `git log --oneline -5`.
2. `docs/MASTER_PLAN.md`.
3. `docs/STRATEGY_V2.md` and `docs/PRODUCTIZATION.md`.
4. `.claude/SERVICE_CONTEXT.md`, `.claude/NEXT_STEPS.md`, `.claude/KNOWN_ISSUES.md`, `.claude/DECISIONS.md` when present.
5. `tasks/todo.md`, latest `tasks/session-handoff*.md`, `tasks/session-summary.md` when present.
6. `git diff --name-only` and any staged diff.

## Output

Write:

1. `tasks/session-handoff-YYYY-MM-DD-HHMM.md` — full handoff.
2. `tasks/session-summary.md` — short pointer to the newest full handoff and top next action.

## Full Handoff Template

Use these exact sections:

0. **Loader Prompt** — pasteable prompt telling the next session to read this file, verify HEAD/status/symbols, re-read source files, restate state, and wait for user go-ahead if the next move is destructive, release-affecting, or external-facing.
1. **Snapshot** — date/time, branch, HEAD SHA, dirty/staged/untracked status, active runtime, current initiative.
2. **Ground Truth Sources** — paths and why each matters; distinguish committed docs from gitignored task state.
3. **Completed This Session — Do Not Re-Do** — imperative list of solved items that must not be reworked unless verification proves drift.
4. **Current In-Progress State** — partial work, modified files, safe continue/revert notes.
5. **Decisions and Decision Rights** — settled decisions, agent-decidable questions, owner-only questions. If not listed, treat as settled by existing docs/code.
6. **Blockers, Traps, and Non-Obvious Context** — release gates, macOS permissions, flaky tests, API limits, false positives.
7. **Artifact Map** — table: Path / State / Purpose / Next action.
8. **Drift Verification Checklist** — commands the next session must run (`git rev-parse HEAD`, `git status --short`, `git log --oneline -5`, `git diff --name-only`, plus relevant `go test`, `make build`, TOML/grep checks).
9. **Next Moves** — prioritized executable steps with objective, files, verification command, and stop/ask condition.
10. **Startup Sequence for Next Session** — run drift checks, read sources, compare state, state drift, continue or ask.
11. **Appendix** — compact subagent summaries, command outputs, PR/release URLs, source links; redact secrets.

## Compatibility Summary

`tasks/session-summary.md` must be short and point to the newest full handoff.

## Verification

- Full handoff has all 11 sections.
- Summary points at the full handoff.
- Branch, HEAD, and status came from live commands.
- Completed work has explicit Do Not Re-Do entries.
- Open questions are partitioned into agent-decidable vs owner-only.
- Drift commands are concrete and runnable.
- No credentials, tokens, API keys, or private secrets are present.
