---
description: >
  Shows current workplan status for heimdall. Reads MASTER_PLAN.md progress
  tracker, NEXT_STEPS.md priorities, and KNOWN_ISSUES.md blockers, then
  cross-references git log for shipped evidence. Use at the start of each
  work session to know what to focus on.
argument-hint: "[phase]"
allowed-tools: Read, Grep, Glob, Bash(git log:*), Bash(git status:*), Bash(git branch:*), Bash(git tag:*), Bash(ls:*), Bash(wc:*)
---

**Version**: 1.0
**Created**: 2026-04-19
**Last Updated**: 2026-04-19
**Authors:** Omer Ufuk

---

# /sprint -- Current Workplan Status

**Usage**: `/sprint` or `/sprint $ARGUMENTS`

## Current State

- Current branch: !`git branch --show-current`
- Recent commits: !`git log --oneline -5`
- Latest tag: !`git tag --list --sort=-version:refname | head -1`
- Master plan: !`ls docs/MASTER_PLAN.md 2>/dev/null && echo "present" || echo "MISSING"`

**Examples**:
```
/sprint           # full workplan status across all phases
/sprint 1E        # drill into a specific phase only
/sprint 1B        # show Phase 1B subtasks + ship evidence
```

---

## What This Command Does

Prints a concise status of the heimdall workplan. Reads the Progress Tracker in `docs/MASTER_PLAN.md` (19 subtasks across 6 phases), cross-references git log for shipped evidence, and merges in the live priority list from `.claude/NEXT_STEPS.md` and open blockers from `.claude/KNOWN_ISSUES.md`. Produces a workplan status report with phase-level completion and the top unblocked next action.

Ported from the mythix `/sprint` pattern (same shape: workplan + git cross-reference + top-action recommendation), rewired to heimdall's inputs.

---

## Source Hierarchy (read in order)

1. **`docs/MASTER_PLAN.md`** — authoritative subtask list with Progress Tracker at the bottom. 19 subtasks across Phase 0 + 1A/1B/1C/1D/1E.
2. **`.claude/NEXT_STEPS.md`** — live priority list, usually 3-7 items.
3. **`.claude/KNOWN_ISSUES.md`** — open issues that could block the next action.
4. **`.claude/SERVICE_CONTEXT.md`** — current state snapshot (version label, status summary).
5. **Git log** — evidence of shipped work since the last tag.

If MASTER_PLAN.md is missing, stop with a clear error — it is the required source of truth.

---

## Execution Instructions

When this command is invoked, execute the following steps in order.

---

### Step 1 — Parse scope

Parse `$ARGUMENTS`:

- **Empty**: full report across all phases (Phase 0 + 1A/1B/1C/1D/1E).
- **Phase ID** (e.g. `1E`, `1B`, `0`): filter the Open Subtasks section to that phase only, but still print the overall summary.
- **Anything else**: stop with "Unrecognized scope: {arg}. Expected empty or a phase ID (0, 1A, 1B, 1C, 1D, 1E)."

---

### Step 2 — Read the Master Plan Progress Tracker

Read `docs/MASTER_PLAN.md` and locate the `## Progress Tracker` section. For each phase heading (`### Phase X: ...`), parse the list of `- [x]` / `- [ ]` items that follow. Record:

- Subtask ID (e.g. `0.4`, `1E.2`, `HC-4`)
- Title
- Checked state (done / open)
- Parent phase

Also capture the bottom footer line `**Total subtasks**: N` as the denominator for overall completion.

---

### Step 3 — Compute phase rollups

For each phase, compute `done / total` and status:

- `[x] all complete` — every subtask checked
- `[!] N open` — one or more subtasks still `[ ]`
- `[-] not started` — zero checked

Also compute the overall rollup: total done / total subtasks across all phases.

---

### Step 4 — Cross-reference git log for evidence

For every **open** subtask, search the recent commit history for evidence of work-in-progress:

```bash
git log --oneline --all --since="2026-01-01" | head -50
git log --oneline main..HEAD 2>/dev/null
```

Heuristic match: if commits mention the subtask ID (`1E.2`), the subtask title keywords, or the related package path, mark as **IN PROGRESS**. Otherwise mark as **TODO**.

If the current branch is not `main`, also note any commits on the branch that appear to touch the open subtask.

Classification per open subtask:
- **TODO** — not yet started
- **IN PROGRESS** — commits on a branch, uncommitted changes, or recent PR activity
- **BLOCKED** — referenced in KNOWN_ISSUES.md as dependent on an unresolved issue

---

### Step 5 — Read NEXT_STEPS.md priorities

Extract the top priority items. The file is usually structured as a bullet list or numbered list under a "Priorities" or "Next" section. Preserve ordering.

For each priority item, check whether it corresponds to an open MASTER_PLAN subtask or is a separate line-item (e.g. "tag v0.1.0 release"). Tag each with its source.

---

### Step 6 — Read KNOWN_ISSUES.md

Extract currently-open issue entries (not yet marked resolved). For each, note:

- Short title
- Severity if indicated (CRITICAL / WARNING / MINOR)
- Whether it blocks any open MASTER_PLAN subtask

---

### Step 7 — Pick the top unblocked action

From open subtasks + NEXT_STEPS.md priorities + unresolved KNOWN_ISSUES blockers, select the single most actionable item. Preference order:

1. An open MASTER_PLAN subtask that has no KNOWN_ISSUES blocker.
2. A NEXT_STEPS.md priority that ties to a concrete deliverable.
3. An unresolved CRITICAL-severity KNOWN_ISSUES entry.

Produce a 2-line justification: why this item and what a "done" looks like.

---

### Step 8 — Print Sprint Status Report

```
HEIMDALL SPRINT STATUS — {YYYY-MM-DD}

Branch:      {current branch}
Latest tag:  {tag or "(none)"}
Plan source: docs/MASTER_PLAN.md

-- Phase Rollup ------------------------------------------------

Phase 0:   {N}/{N}  {status marker}  Foundation + Spike
Phase 1A:  {N}/{N}  {status marker}  Config + Doctor
Phase 1B:  {N}/{N}  {status marker}  Claude Analysis + Obsidian Output
Phase 1C:  {N}/{N}  {status marker}  Full Record Command
Phase 1D:  {N}/{N}  {status marker}  Testing + Security
Phase 1E:  {N}/{N}  {status marker}  Distribution + Release

Overall: {done}/{total} subtasks complete ({percent}%)

-- Open Subtasks -----------------------------------------------

[TODO]        {id}  {title}
[IN PROGRESS] {id}  {title} — {branch or commit evidence}
[BLOCKED]     {id}  {title} — waiting on {blocker}

(none open — all subtasks complete)

-- Current Priorities (NEXT_STEPS.md) --------------------------

1. {priority item} — {maps to subtask ID or "standalone"}
2. {priority item} — ...

-- Open Issues (KNOWN_ISSUES.md) -------------------------------

[{severity}]  {issue title} — {blocks subtask X / standalone}

(none open)

-- Recent Activity (git) ---------------------------------------

Last 5 merged PRs / commits:
  {hash}  {subject}
  {hash}  {subject}
  ...

-- Top Unblocked Action ----------------------------------------

{subtask-id or priority item}: {title}

Why: {single sentence}
Done when: {single sentence — the acceptance criterion}
```

---

## Error Conditions

| Situation | Action |
|-----------|--------|
| `docs/MASTER_PLAN.md` not found | Stop: "MASTER_PLAN.md missing — cannot compute workplan status without source of truth." |
| Progress Tracker section not found in MASTER_PLAN.md | Stop: "Progress Tracker section not found. Expected `## Progress Tracker` with per-phase `- [x]`/`- [ ]` items." |
| `.claude/NEXT_STEPS.md` missing | Warn but continue: "NEXT_STEPS.md missing — skipping Current Priorities section." |
| `.claude/KNOWN_ISSUES.md` missing | Warn but continue: "KNOWN_ISSUES.md missing — skipping Open Issues section." |
| No git history available | Skip ship-evidence cross-reference; mark all open items as TODO; warn: "No git history — cannot determine shipped work." |
| `$ARGUMENTS` is an unrecognized phase ID | Stop: "Unrecognized scope: {arg}. Expected empty or a phase ID (0, 1A, 1B, 1C, 1D, 1E)." |
| All subtasks are complete (19/19) | Replace "Top Unblocked Action" section with "All MASTER_PLAN subtasks complete. Next action sources: NEXT_STEPS.md line items or a new phase workplan." |

---

## Scope Boundaries

**This skill DOES:**
- Read `docs/MASTER_PLAN.md`, `.claude/NEXT_STEPS.md`, `.claude/KNOWN_ISSUES.md`, `.claude/SERVICE_CONTEXT.md`
- Run read-only `git log`, `git branch`, `git tag`, `git status` commands
- Cross-reference git evidence against open subtasks
- Print a status report with the top unblocked next action

**This skill DOES NOT:**
- Modify any file (read-only)
- Update the Progress Tracker (that's done manually at session end per MASTER_PLAN.md § Session Handoff Protocol)
- Spawn agents or run builds/tests — use `/pipeline-health` or `/strategy-weekly` for those
- Override the user's judgment on what to work on next — the recommendation is a suggestion, not a directive

---

## Quick Reference

```bash
# Key files
docs/MASTER_PLAN.md             # 19-subtask execution plan with Progress Tracker
.claude/NEXT_STEPS.md           # live priority list
.claude/KNOWN_ISSUES.md         # open issues / blockers
.claude/SERVICE_CONTEXT.md      # current state snapshot

# Useful git queries
git log --oneline --all -20                          # recent activity across all branches
git log --oneline main..HEAD 2>/dev/null             # commits on current branch vs main
git tag --list --sort=-version:refname | head -3     # recent tags
git log --all --grep="1E.2\|1E\.2\|final review"     # find commits referencing a subtask
```

---

## Bridge to Other Skills / Agents

- `/continue` — heavier cousin; reads SERVICE_CONTEXT + NEXT_STEPS + KNOWN_ISSUES + session-summary and actively resumes work. Use `/sprint` when you just want a read-only status snapshot.
- `/strategy-weekly` — far heavier; reviews git activity + test health + priorities across a longer horizon. Use when the weekly review cadence is due.
- `/audit` — use when the Progress Tracker itself looks stale vs git reality. `/sprint` trusts the tracker; `/audit` verifies it.
- `/pipeline-health` — complementary: `/sprint` tells you *what* to work on, `/pipeline-health` tells you whether the system is operable before you start.
