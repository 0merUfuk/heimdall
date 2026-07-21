---
description: Continue a previous session — read context, pick up where you left off
---

## Current State

- Git status: !`git status --short | head -5`
- Current branch: !`git branch --show-current`
- Recent commits: !`git log --oneline -3`

## Resume Protocol

Read the following files in order, then continue working on the next priority item:

1. `.claude/SERVICE_CONTEXT.md` — current project state
2. `.claude/NEXT_STEPS.md` — what to work on next
3. `.claude/KNOWN_ISSUES.md` — issues to be aware of
4. Latest `tasks/session-handoff*.md` — full verified handoff, if present
5. `tasks/session-summary.md` — compatibility summary pointer, if present

Before acting on handoff claims, verify current `git rev-parse HEAD`, `git status --short`, and any drift-check commands listed in the handoff. If HEAD/status drift invalidates the top next action, report the drift and ask before continuing.

After reading, identify the highest-priority item from NEXT_STEPS.md or the latest verified handoff and begin working on it. Track progress in `tasks/todo.md`.
