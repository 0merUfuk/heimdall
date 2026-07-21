---
description: >
  Create a verified Heimdall session handoff for Codex/Hermes/Claude continuity.
  Produces a full state file plus a lean resume summary with drift checks.
argument-hint: "[optional-handoff-name]"
---

# /handoff -- Verified Session Handoff

Use this when ending a non-trivial Heimdall session or transferring work between agents.

## Read First

1. `git status --short`, `git branch --show-current`, `git rev-parse HEAD`, `git log --oneline -5`.
2. `docs/MASTER_PLAN.md`.
3. `docs/STRATEGY_V2.md` and `docs/PRODUCTIZATION.md`.
4. `.claude/SERVICE_CONTEXT.md`, `.claude/NEXT_STEPS.md`, `.claude/KNOWN_ISSUES.md`, `.claude/DECISIONS.md` if present.
5. `tasks/todo.md`, previous `tasks/session-handoff*.md`, and `tasks/session-summary.md` if present.
6. `git diff --name-only` and staged diff.

## Write

- `tasks/session-handoff-YYYY-MM-DD-HHMM.md` — full handoff.
- `tasks/session-summary.md` — short pointer to the newest full handoff and top next action.

## Full Handoff Sections

0. Loader Prompt — next session must verify HEAD/status before trusting this file.
1. Snapshot — date/time, branch, HEAD SHA, dirty/staged/untracked status, runtime, initiative.
2. Ground Truth Sources — exact files and why they matter.
3. Completed This Session — Do Not Re-Do — solved work that must not be reworked unless drift proves otherwise.
4. Current In-Progress State — partial work and safety notes.
5. Decisions and Decision Rights — settled decisions, agent-decidable questions, owner-only questions.
6. Blockers, Traps, and Non-Obvious Context — release gates, macOS permissions, flaky checks, API limits.
7. Artifact Map — Path / State / Purpose / Next action.
8. Drift Verification Checklist — `git rev-parse HEAD`, `git status --short`, `git log --oneline -5`, `git diff --name-only`, plus relevant `go test`, `make build`, TOML/grep checks.
9. Next Moves — ordered steps with objective, files, verification, stop/ask condition.
10. Startup Sequence — verify drift, read sources, compare, state drift, continue or ask.
11. Appendix — compact evidence; redact secrets.

## Rule

Treat every handoff as claims, not truth. The next session must verify before acting.
