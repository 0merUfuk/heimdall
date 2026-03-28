---
description: >
  Adversarial verification of heimdall implementation correctness.
  Runs 4 subagents that attack from different angles: gap finding, assumption
  attacking, ground truth verification, and devil's advocacy. Use after completing
  work, before releases, or to attack a plan before implementation.
argument-hint: "[plan] [scope]"
allowed-tools: Read, Grep, Glob, Bash(git diff:*), Bash(git log:*), Bash(git tag:*), Bash(git status:*), Bash(grep:*), Bash(cat:*), Bash(ls:*), Bash(find:*), Bash(wc:*), Bash(make build:*), Bash(make test:*), Bash(go vet:*), Bash(go build:*), Bash(go test:*)
---

# /doublecheck — Critical Verification Pass

**Usage**: `/doublecheck $ARGUMENTS`

## Current State

- Recent changes: !`git diff HEAD --stat | tail -5`
- Recent commits: !`git log --oneline -5`
- Current branch: !`git branch --show-current`

**Examples**:
```
/doublecheck                  # verify whatever was just implemented
/doublecheck api              # verify API layer changes
/doublecheck plan             # attack a plan before implementation begins
```

---

## What This Command Does

Runs an adversarial verification pass on the implementation. This is NOT a confirmation pass — it is a structured search for problems. Four subagents attack from different angles simultaneously.

**This command is different from `/audit`.**
- `/audit` asks: "Are the context files accurate against ground truth?"
- `/doublecheck` asks: "Is the implementation itself complete, correct, and production-ready?"

> **Critical**: "Looks good" and "seems correct" are forbidden. Every agent must produce specific findings or justify exactly why each check passed. Absence of findings is suspicious — look harder.

---

## The Mindset

Assume at least one flaw exists. You are not here to confirm — you are here to find. A false "all clear" shipped to a real user is worse than a false alarm caught internally. Every agent is adversarial by design.

---

## Execution Instructions

---

### Step 0 — Establish ground truth baseline

Before launching subagents, collect from the project root:

```bash
git diff HEAD                                                  # every changed line
git diff HEAD --stat                                           # which files changed
git log --oneline -5                                           # recent commits
cat tasks/todo.md 2>/dev/null                                  # session plan (if exists)
```

Identify:
- **What scope** is being verified (inferred from `$ARGUMENTS` or from recent changes)
- **What was claimed done** — from `tasks/todo.md` or conversation context
- **What mode** — implementation (default) or plan (`/doublecheck plan`)

**If `git diff HEAD` is empty**: use `git diff HEAD~1..HEAD` and note: "Verifying committed changes."

---

### Step 1 — Launch 4 adversarial subagents IN PARALLEL

All four launch simultaneously. Each has one attack vector. They are not validating — they are hunting.

---

#### Agent 1: Gap Finder

**Mission**: Find every delta between the stated requirements and what was actually delivered.

**What to check**:
- Read `tasks/todo.md` (if it exists) — every completed item must be traceable to an actual code change in the diff
- Read `git diff HEAD` — every changed line across every file
- For each claimed requirement: COMPLETE / PARTIAL / MISSING
- Check if files changed outside the expected scope (unintended side effects)
- Check CHANGELOG.md (if present) — does the entry accurately describe what changed?

**go-net-http (Go) gap checks**:
- Are all new exported functions covered by tests?
- Are new packages properly wired into dependency injection (if flat uses DI)?
- Are new API routes registered in the router?


**Red flags**: Todo items marked done with no corresponding diff. Files modified that have no relation to the stated work. Claimed feature exists but grep for its key identifier returns nothing. CHANGELOG entry describes a feature not visible in the diff.

---

#### Agent 2: Assumption Attacker

**Mission**: Surface every assumption embedded in the implementation. Challenge each one. Find the one that breaks a real user's workflow.

**What to check**:
- Read all changed source files — for every function, what does it assume about its inputs?
- For configuration: what does the code assume about environment variables, config files, or defaults?
- For external dependencies: what happens if a dependency is unavailable (database down, API timeout, queue unreachable)?
- For user input: what happens with empty strings, special characters, extremely long values, Unicode, null values?

**go-net-http (Go) assumption checks**:
- Are all error returns from external calls checked?
- Do goroutines have proper context cancellation and cleanup?
- Are nil pointer dereferences possible on error paths?
- Do template files assume specific struct fields that could be zero-value?
- Are `{{ .Field }}` template references guarded against nil/empty?


**Red flags**: Function accepts input with no validation. Config value used without default. External call with no timeout. Error swallowed silently. Assumption that directory or file exists without checking.

---

#### Agent 3: Ground Truth Verifier

**Mission**: Verify every claim against actual running state and git facts. Trust nothing from conversation — only what commands confirm.

**What to check**:

**go-net-http (Go) ground truth**:
- `make build` or `go build ./...` succeeds — binary compiles
- `make test` or `go test ./...` passes — no failing tests
- `go vet ./...` passes — no static analysis issues
- Grep changed files for `TODO`, `FIXME`, `HACK` — report every hit
- Run the binary with `--help` (if CLI) — does output match documentation?


**Cross-checks**:
- Do version strings in source match documentation?
- If a release was done: does the git tag exist and match the source version?
- Are CHANGELOG entries consistent with actual git history?

**Red flags**: Build fails. Tests fail. Grep finds uncommitted debug code (`console.log`, `fmt.Println` used for debugging). Version mismatch between source and docs. Documented feature not reachable from CLI or API.

---

#### Agent 4: Devil's Advocate

**Mission**: Find the most likely real-world failure mode for an actual user running this for the first time, or running it in an edge case nobody considered.

**What to check**:

**go-net-http (Go) failure modes**:
- What happens when the binary is run from the wrong directory?
- What happens when required environment variables are missing?
- What happens when the database is unreachable at startup?
- What is the first error a new developer sees when setting up locally?
- What does the error message look like — is it actionable or a raw Go panic?


**General failure modes**:
- What service shape or configuration breaks the current implementation? Missing optional features? Empty collections? Zero entities?
- What happens when a configuration file has unexpected format or encoding?
- What happens when disk is full, permissions are wrong, or port is already in use?
- What is the cryptic error the user sees instead of a helpful message?
- What edge case was the implementation not designed for but will encounter in production?

**Red flags**: Hard dependency on specific file paths with no graceful degradation. Application gives a panic/stack trace instead of a user-readable error on bad input. Process hangs indefinitely on misconfiguration instead of failing fast. Works on one OS/arch but not another due to path or binary assumptions.

---

### Step 2 — Collect all findings

Wait for all 4 agents. Aggregate into:

- **Critical** — breaks real usage, blocks merging or releasing
- **Warning** — edge case risk, misleading documentation, or quality concern
- **Verified** — specifically checked and confirmed correct (list the check, not just "checked")

---

### Step 3 — Produce terminal report

```
  ── DOUBLECHECK ──────────────────────────────────────────────

    Project: heimdall
    Mode:    Implementation
    Changed: {N} files

  ── Gap Finder ───────────────────────────────────────────────

    {finding, or checkmark with exact checks run}

  ── Assumption Attacker ──────────────────────────────────────

    {finding, or checkmark with exact checks run}

  ── Ground Truth Verifier ────────────────────────────────────

    go build:    {PASS / FAIL — exact output}
    go test:     {PASS / FAIL — exact output}
    go vet:      {PASS / FAIL — exact output}

    {other findings, or checkmark with exact checks run}

  ── Devil's Advocate ─────────────────────────────────────────

    {finding, or checkmark with exact failure mode checked}

  ── Verdict ──────────────────────────────────────────────────

    Confidence:  {0–100}%
    Critical:    {N}
    Warnings:    {N}
    Verified:    {N} checks

    {SHIP / FIX FIRST / REDESIGN}
```

**Confidence score**:
- 95-100% — SHIP. Genuine absence of issues after thorough adversarial search.
- 80-94% — Fix warnings first, then ship or release.
- 60-79% — Critical issue present. FIX FIRST.
- Below 60% — Significant rework needed. REDESIGN.

**Verdict rules**:
- Any Critical — FIX FIRST minimum
- Confidence < 80% — FIX FIRST
- Confidence >= 95% + zero Criticals — SHIP

---

### Step 4 — If issues found: fix and re-verify

1. Fix every Critical — address root cause, not just the symptom
2. Re-run only the agent whose check failed — not the full doublecheck
3. Re-score confidence after each fix
4. Do not call this complete until confidence >= 80% and zero Criticals remain

---

## Plan Mode (`/doublecheck plan`)

When invoked before implementation, the 4 agents attack the plan instead of code:

| Agent | Plan-mode focus |
|-------|----------------|
| Gap Finder | Does this plan address every requirement? What is not covered? |
| Assumption Attacker | What does this plan assume about the codebase, runtime environment, or user behavior that could be wrong? |
| Ground Truth Verifier | Do the referenced files, paths, and interfaces actually exist as the plan assumes? |
| Devil's Advocate | What makes this plan fail at implementation time or break for a real user? |

**Verdict**: PROCEED / REVISE / RETHINK

---

## Rules

- "Looks good" is forbidden
- "Seems correct" is forbidden
- Every finding must cite a specific file, line number, or command output
- Confidence score is mandatory — no vague summary replaces it
- If an agent finds nothing: list every specific check it ran and why each passed
- The hardest-to-find problem is the most important — do not stop at surface issues
