---
name: heimdall-handoff
description: Use when ending or transferring a Heimdall work session. Packages volatile state into a verified handoff file plus a lean loader prompt so the next Hermes, Claude, or Codex session resumes without re-solving completed work.
version: 1.0.0
author: Hermes Agent
license: MIT
metadata:
  hermes:
    tags: [heimdall, handoff, continuity, compaction, agent-ecosystem]
    related_skills: [heimdall-continue, port-agent-ecosystem]
---

# Heimdall Handoff

## Overview

Create a lossless, drift-verifiable handoff for Heimdall sessions. The handoff is not a generic summary: it is a state artifact for the next agent. It must separate durable project truth from volatile session state, list what must not be re-done, and give the next session exact verification commands before it acts.

This ports the continuity pattern extracted from the agent-ecosystem know-how: the next session treats the handoff as claims, verifies HEAD/status/symbols first, then resumes from a closed open-question register and prioritized next moves.

## When to Use

- End of a Heimdall development, strategy, release, or audit session.
- Before `/clear`, context compaction, switching tools, or handing work to Claude Code/Codex.
- After subagents produced findings that need durable preservation.
- Before a branch/PR handoff where rework would be costly.

Do not use this for a trivial answer-only chat with no repo state, no decisions, and no follow-up work.

## Output Files

Write both files when there is meaningful work:

1. `tasks/session-handoff-YYYY-MM-DD-HHMM.md` — full state handoff.
2. `tasks/session-summary.md` — short compatibility pointer that links to the newest handoff and states the top next action.

`tasks/` is intentionally gitignored live state. If a finding must survive beyond the current initiative, move it into committed docs (`docs/`, `.claude/`, `.codex/`, or `.hermes/skills/`) before handing off.

## Source Hierarchy

Read in this order before writing:

1. `git status --short`, `git branch --show-current`, `git rev-parse HEAD`, `git log --oneline -5`.
2. `docs/MASTER_PLAN.md` — original MVP workplan and release gate.
3. `docs/STRATEGY_V2.md` and `docs/PRODUCTIZATION.md` — current roadmap and monetization/productization direction.
4. `.claude/SERVICE_CONTEXT.md`, `.claude/NEXT_STEPS.md`, `.claude/KNOWN_ISSUES.md`, `.claude/DECISIONS.md` when present.
5. `tasks/todo.md`, previous `tasks/session-handoff*.md`, and `tasks/session-summary.md` when present.
6. Relevant changed files from `git diff --name-only` and staged diff.

Completion criterion: every claim in the handoff is backed by one of these sources or explicitly labeled as an assumption.

## Full Handoff Structure

Use these sections exactly.

### 0. Loader Prompt

A pasteable prompt for the next session. It must tell the next agent to read this handoff first, verify git HEAD/branch/status before trusting it, re-read named source files, restate loaded state, and wait for user go-ahead if the next step is destructive, release-affecting, or external-facing.

### 1. Snapshot

Date/time, branch, HEAD SHA, dirty/staged/untracked status, active runtime(s), and current phase or initiative.

### 2. Ground Truth Sources

List files that define current truth. Include exact paths and why each matters. Distinguish committed docs from gitignored task state.

### 3. Completed This Session — Do Not Re-Do

Use imperative framing: "Do NOT re-fix/rebuild/research these unless verification proves drift." Include commits, files, tests, and decisions.

### 4. Current In-Progress State

What was being changed, which files are modified, what remains partially done, and whether the work is safe to continue or should be reverted.

### 5. Decisions and Decision Rights

Closed register: settled decisions, agent-decidable open questions, and owner-only questions. If a question is not in this register, treat it as settled by existing docs/code.

### 6. Blockers, Traps, and Non-Obvious Context

Include environmental traps, failing commands, flaky tests, API limits, release gates, macOS permissions, code-signing constraints, and known false positives.

### 7. Artifact Map

Table of changed/created files with status: `committed`, `modified`, `staged`, `untracked`, `gitignored`, or `external`.

### 8. Drift Verification Checklist

Commands the next session must run before acting: `git rev-parse HEAD`, `git status --short`, `git log --oneline -5`, `git diff --name-only`, plus project-specific symbol grep, TOML parse, `go test`, `make build`, release tag checks, or GitHub PR/release checks.

### 9. Next Moves

Numbered, prioritized, executable next steps. Each item needs objective, files likely involved, verification command, and stop/ask condition.

### 10. Startup Sequence for Next Session

Run drift checks, read ground-truth files, compare current repo state to the handoff, state what changed, then continue from Next Move #1 or ask if drift invalidates it.

### 11. Appendix

Paste compact subagent summaries, command outputs, PR/release URLs, or source links too detailed for the main sections. Redact credentials and personal tokens.

## Lean `tasks/session-summary.md` Format

Keep the compatibility summary under ~40 lines and point to the latest full handoff.

## Verification Checklist

- [ ] Full handoff exists under `tasks/` with all 11 sections.
- [ ] `tasks/session-summary.md` points to the full handoff.
- [ ] Branch, HEAD, and git status were captured from live commands.
- [ ] Completed work has a Do Not Re-Do register.
- [ ] Open questions are partitioned into agent-decidable vs owner-only.
- [ ] Drift verification commands are concrete and runnable.
- [ ] Next moves include verification and stop/ask conditions.
- [ ] No credentials, tokens, or private API keys are present.
