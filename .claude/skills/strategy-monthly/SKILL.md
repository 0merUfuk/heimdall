---
description: >
  Monthly strategy review for heimdall. Orchestrates product-lead, tech-lead,
  and growth-lead in parallel, synthesizes findings, and produces a comprehensive
  strategy assessment. Use at month boundaries or when major strategic questions arise.
argument-hint: "[--dry-run]"
allowed-tools: Read, Write, Edit, Grep, Glob, WebSearch, WebFetch, Bash, Agent
---

# Monthly Strategy Review

## When to Use
- At the start of each month
- After a major release
- When strategic direction is questioned

## Execution

### Step 1: Read Baseline Context

Read these files yourself (fast, sequential):
1. `docs/architecture/STRATEGY.md` -- competitive positioning
2. `docs/architecture/ROADMAP.md` -- phase we are in
3. `.claude/SERVICE_CONTEXT.md` -- what is implemented
4. `git log --oneline --since="30 days ago"` -- monthly activity

### Step 2: Spawn All Three Leads in Parallel

Launch in a SINGLE message:

**Product Lead:**
> "Run a monthly product review for heimdall. Assess competitive position, user feedback (GitHub issues), and roadmap adherence. Produce your standard monthly assessment."

**Tech Lead:**
> "Run a monthly technical health review for heimdall. Assess architecture drift, dependency health, test coverage, and code quality. Produce your standard technical health report."

**Growth Lead:**
> "Run a monthly growth review for heimdall. Assess GitHub stars/forks, community mentions, content pipeline, and channel opportunities. Produce your standard growth report."

### Step 3: Synthesize

Combine all three reports into a unified assessment:

```markdown
# Monthly Strategy Review -- {YYYY-MM}

## Overall Health: N/10

## Product (from product-lead)
{key findings and priority shifts}

## Technical (from tech-lead)
{health score, critical findings, recommendations}

## Growth (from growth-lead)
{channel performance, content pipeline, opportunities}

## Strategic Decisions
{any changes to strategy, roadmap, or priorities -- with rationale}

## Next Month Priorities
1. {top priority across all dimensions}
2. {second}
3. {third}
```

Write to `tasks/monthly-{YYYY-MM}.md`.

### Step 4: Update Strategy (if needed)

If findings require strategy changes:
- Edit `docs/architecture/STRATEGY.md` with `> Updated {date}:` callouts
- Edit `docs/architecture/ROADMAP.md` if timeline shifts
- Recommend running the architect agent if ecosystem needs updating
