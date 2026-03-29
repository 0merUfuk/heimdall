---
description: >
  Captures development session findings and proposes targeted improvements to
  the .claude/ ecosystem. Reads session artifacts, briefs the architect agent,
  and applies human-approved additions to rules, agent prompts, and checklists.
argument-hint: ""
allowed-tools: Read, Glob, Grep, Edit, Write, Bash, Agent
---

# Session Learn

## When to Use
- At the end of a development session
- After a significant debugging session
- When a pattern was discovered that should be encoded

## Execution

### Step 1: Read Session Artifacts

1. Read `tasks/session-summary.md` if it exists
2. Read `git log --oneline -10` for recent changes
3. Read `git diff HEAD~5..HEAD --stat` for scope of changes
4. Read `.claude/KNOWN_ISSUES.md` for any new issues added

### Step 2: Identify Learnings

Look for:
- **New rules**: patterns that should be enforced (e.g., "always check X before Y")
- **Agent gaps**: tasks that no agent handles well
- **Skill candidates**: workflows repeated manually that should be automated
- **Stale artifacts**: rules/agents that reference removed code

### Step 3: Brief the Architect

Spawn the architect agent with findings:
> "Session learning report: [findings]. Review and propose ecosystem changes."

### Step 4: Apply Changes

With architect recommendations:
- Update rules if constraints were violated
- Update agent prompts if scope was unclear
- Create new skills if workflows were repeated
- Remove stale references
