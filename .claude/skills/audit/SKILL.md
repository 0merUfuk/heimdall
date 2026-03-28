---
description: >
  Audits heimdall context files against ground truth (git, code, filesystem).
  Verifies versions, code claims, docs accuracy, and project structure.
  Use when context files may have drifted from reality, after implementations,
  or before releases.
argument-hint: "[scope]"
allowed-tools: Read, Grep, Glob, Bash(git tag:*), Bash(git log:*), Bash(git diff:*), Bash(git status:*), Bash(grep:*), Bash(find:*), Bash(wc:*), Bash(cat:*), Bash(ls:*), Bash(make build:*), Bash(make test:*), Bash(go vet:*)
---

# /audit — Audit heimdall Context Files

**Usage**: `/audit` or `/audit $ARGUMENTS`

## Current State

- Git status: !`git status --short | head -5`
- Recent commits: !`git log --oneline -5`
- Latest tags: !`git tag --list --sort=-version:refname | head -3`

**Examples**:
```
/audit               # full audit of entire project
/audit docs          # audit documentation only
/audit structure     # audit project structure only
```

---

## What This Command Does

Verifies every documented claim against ground truth (git, code, filesystem). Never trusts context files — always checks. Produces a structured terminal report and writes `tasks/audit-report.md` for reference and follow-up fixes.

---

## Core Principle

**Never trust context files. Always verify against git/code/filesystem.**

Context files (SERVICE_CONTEXT.md, KNOWN_ISSUES.md, NEXT_STEPS.md) drift from reality. Items get marked done without code verification. Versions fall behind git tags. KNOWN_ISSUES.md entries stay open after resolution. This command detects all of it.

---

## Execution Instructions

When this command is invoked, execute the following steps in order.

---

### Step 1 — Determine scope

Parse `$ARGUMENTS`:
- If a scope was provided (e.g., `/audit docs`), restrict agents in Step 2 to that scope.
- If empty, run full audit of the entire project.

---

### Step 2 — Launch 4 Explore subagents IN PARALLEL

Launch all four agents simultaneously. Do not wait for one before starting the next.

---

#### Agent 1: Version Agent

**Scope**: Version strings across all sources — source code, config files, git tags, context files.

**Ground truth sources**:

- `var version` or `const version` in Go source files
- Version constants in `Makefile` (if present)

- `git tag --list --sort=-version:refname` — release tags
- `.claude/SERVICE_CONTEXT.md` — documented version
- `CHANGELOG.md` — latest versioned section headings (if present)
- `CLAUDE.md` — version references in project documentation

**Checks**:
1. Extract version from every source above
2. Compare all sources — do they agree?
3. Report any mismatch as CRITICAL
4. A `-dev` or `-rc` suffix in source is expected for unreleased work — compare the base version

---

#### Agent 2: Code Verification Agent

**Scope**: Verify documented claims about implementations against actual source code.

**Ground truth**: actual source files via Read and Grep.


**go-net-http (Go) checks**:

| Check | How to verify |
|-------|--------------|
| Documented packages exist | Verify directories and files listed in CLAUDE.md or SERVICE_CONTEXT.md |
| Dependencies match docs | Compare `go.mod` against documented dependencies |
| Build succeeds | Run `make build` or `go build ./...` |
| Tests pass | Run `make test` or `go test ./...` |
| Vet passes | Run `go vet ./...` |


**Cross-check with CLAUDE.md**: Read `CLAUDE.md` — does the documented architecture, commands, and features match what exists in code?

**Cross-check with SERVICE_CONTEXT.md**: Read `.claude/SERVICE_CONTEXT.md` — do documented states and feature lists match ground truth?

Report each as PASS (found) or FAIL (not found / wrong value).

---

#### Agent 3: Docs Accuracy Agent

**Scope**: `.claude/` context file accuracy, staleness, cross-references.

**Checks**:

**CLAUDE.md** (project root):
- Does the documented project structure match the actual filesystem?
- Do listed features, commands, or endpoints match what exists in code?
- Are there stale references to removed or renamed components?

**SERVICE_CONTEXT.md**:
- Are component statuses accurate vs code?
- Are there stale references to old technologies or removed features?

**KNOWN_ISSUES.md**:
- Are open issues still actually open? Cross-reference with code.
- Are there resolved issues still listed as open?

**NEXT_STEPS.md**:
- Are items marked done actually done in code?
- Do priorities reflect the current project state?

**DECISIONS.md** (if present):
- Do referenced files and paths still exist?
- Are factual claims still accurate?

---

#### Agent 4: Structure Agent

**Scope**: Project structure, build system, configuration.

**Checks**:

**Project structure**:
- All directories documented in CLAUDE.md exist
- No orphaned directories or files that should be documented
- `.gitignore` covers expected patterns (tasks/, node_modules/, build artifacts, etc.)

**Build system**:

- `go.mod` exists with correct module path
- `Makefile` targets exist and work (build, test)
- `go build ./...` succeeds from project root


**Agent ecosystem** (`.claude/` directory):
- All referenced agent files in `.claude/agents/` exist
- All referenced skill files in `.claude/skills/` exist
- All referenced rule files in `.claude/rules/` exist
- Knowledge directory structure is consistent (if `.claude/knowledge/` exists)

---

### Step 3 — Collect findings from all 4 agents

Wait for all agents to complete. Merge their findings into two lists:

1. **Verified** — claim checked, matches ground truth
2. **Issues** (CRITICAL or WARNING) — claim does not match ground truth

---

### Step 4 — Produce terminal report

Print the following structured report:

```
  heimdall audit — {YYYY-MM-DD}

  ── go-net-http ─────────────────────────────────────────────────

  {per-finding lines, one per check}

  ── documentation ─────────────────────────────────────────────

  {context file accuracy checks}

  ── structure ─────────────────────────────────────────────────

  {build, structure, agent ecosystem checks}

  ── summary ───────────────────────────────────────────────────

  {N} critical  |  {N} warnings  |  {N} verified
  {N} items may be safe to fix mechanically.
  {N} items require manual investigation (see tasks/audit-report.md).
```

**Finding line format**:
- `✓  {check-name}    {one-line description of what was verified}`
- `✗  {check-name}    {one-line description of what is wrong}`
- `⚠  {check-name}    {one-line description of the concern}`

---

### Step 5 — Write tasks/audit-report.md

Write a full report to `tasks/audit-report.md`. Structure:

```markdown
# heimdall Audit Report — {YYYY-MM-DD}

## Summary

{N} critical | {N} warnings | {N} verified

---

## go-net-http

### Verified
- {list of verified items with file paths}

### Issues
- {list of issues with file paths and line numbers where relevant}

---

## Documentation

### Verified
- {list of verified context file claims}

### Issues
- {list of inaccurate or stale documentation}

---

## Structure

### Verified
- {list of verified structural checks}

### Issues
- {list of structural problems}

---

## Safe to Auto-Fix

These items can be fixed mechanically without touching application source code:

1. {file}: {what to change}
2. ...

---

## Requires Manual Action

These items require code changes or manual investigation:

1. {file}: {what needs to change and why}
2. ...
```

> **Important**: The "Safe to Auto-Fix" section should be precise: include exact file paths, current content, and intended replacement text so fixes can be applied reliably.

---

## Error Conditions

| Situation | Action |
|-----------|--------|
| git not initialized | CRITICAL — cannot verify tags or history |
| `tasks/` directory missing | Create it, then write audit-report.md |
| Agent returns no findings | WARNING — note "no checks run for {scope}" and continue |
| `go.mod` missing | CRITICAL — "Not a Go module — go.mod not found" |


---

## Quick Reference

```bash
# What the agents check under the hood
git tag --list --sort=-version:refname | head -10
git log --oneline -10
git diff HEAD --stat

# Go checks
go build ./...
go test ./...
go vet ./...
grep -rn 'var version' . --include='*.go'

```
