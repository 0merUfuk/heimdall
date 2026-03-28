---
description: >
  Audit heimdall dependencies for known vulnerabilities.
  Uses the appropriate scanner for each tech stack in the project.
argument-hint: ""
---

# /dep-audit -- Dependency Vulnerability Audit

## When to Use

- Before releases to verify no known vulnerabilities ship
- After updating dependency manifests or lockfiles
- When triaging a new CVE advisory
- After adding a new third-party dependency

## Invocation

```bash
/dep-audit
```

---

## Behavior

### Step 1: Identify Tech Stacks

Read CLAUDE.md and project files to determine which stacks need scanning.

### Go Dependencies (go-net-http)

```bash
go mod verify
govulncheck ./...
```


### Trivy Scan (all stacks)

```bash
trivy fs . --scanners vuln --severity HIGH,CRITICAL
```

### Merge and Deduplicate

Cross-reference findings from all scanners. Deduplicate by CVE ID.

---

## Output

```markdown
# Dependency Audit Report

## Verdict: CLEAN | VULNERABILITIES_FOUND

## Summary
- Scanner results: N findings per tool
- Unique CVEs after deduplication: N

## Critical / High
| CVE | Package | Current | Fixed | Scanner | Reachable? |
|-----|---------|---------|-------|---------|------------|

## Medium / Low
| CVE | Package | Current | Fixed | Scanner | Reachable? |
|-----|---------|---------|-------|---------|------------|

## Remediation
1. Update affected packages to fixed versions
```

---

## Prerequisites

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
brew install trivy
```
