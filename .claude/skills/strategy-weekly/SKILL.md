---
description: >
  Weekly strategy sync for heimdall. Reviews git activity, test health,
  known issues, and produces a tactical brief. Use every Monday or on
  demand for a quick project health pulse.
argument-hint: "[--dry-run]"
allowed-tools: Read, Write, Grep, Glob, Bash, Agent
---

# Weekly Strategy Sync

## When to Use
- Every Monday (or start of work week)
- Before planning a sprint or work session
- When you need a quick project health pulse

## Execution

### Step 1: Gather Data (parallel reads)

Read in parallel:
- `git log --oneline -20` -- what shipped this week
- `go test ./... -race -count=1` -- test health
- `.claude/KNOWN_ISSUES.md` -- unresolved problems
- `.claude/SERVICE_CONTEXT.md` -- current state
- `docs/MASTER_PLAN.md` -- progress tracker

### Step 2: Spawn Tech-Lead

Launch the tech-lead agent to assess codebase health:
> "Run a quick technical health check. Focus on: test results, any new warnings from go vet, dependency status. Produce a 5-line summary."

### Step 3: Produce Weekly Brief

Write to `tasks/weekly-{YYYY-MM-DD}.md`:

```markdown
# Weekly Brief -- {date}

## What Shipped
- {from git log}

## Test Health
- {pass/fail count, any flaky tests}

## Open Issues
- {from KNOWN_ISSUES.md}

## Priority This Week
1. {top priority with rationale}
2. {second priority}
3. {third priority}

## Blockers
- {anything blocking progress}
```
