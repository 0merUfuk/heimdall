---
description: >
  Creates or updates a .claude/ artifact (agent, skill, or rule) following
  heimdall conventions. Generates files with correct YAML frontmatter,
  documentation structure, and convention compliance.
argument-hint: "<type> <name> [--from <strategy-file>]"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash
---

# Provision

## When to Use
- When the ecosystem needs a new agent role
- When a repeatable workflow should become a skill
- When a constraint should become a rule
- When the architect agent recommends new artifacts

## Arguments

- `type`: one of `agent`, `skill`, `rule`
- `name`: lowercase-hyphenated name (e.g., `performance-engineer`, `benchmark`, `api-rate-limit`)
- `--from`: optional strategy file to derive context from

## Execution

### For Agents

Create `.claude/agents/{name}.md` with:

```yaml
---
name: {name}
description: >
  {1-3 line description}
tools: {appropriate tool list}
model: opus
memory: project
maxTurns: {appropriate limit}
permissionMode: bypassPermissions
---
```

Follow the established pattern: Why You Exist, What You Know, Workflow, Scope Boundaries, Escalation Protocol.

### For Skills

Create `.claude/skills/{name}/SKILL.md` with:

```yaml
---
description: >
  {description}
argument-hint: "{hint}"
allowed-tools: {tools}
---
```

Follow the established pattern: When to Use, Execution (numbered steps).

### For Rules

Create `.claude/rules/{name}.md` with clear constraint statements, examples of correct vs incorrect patterns, and the rationale.

### Validation

After creating:
- YAML frontmatter parses correctly
- All referenced files/paths exist
- Tool lists use valid tool names
- No duplicate names with existing artifacts
