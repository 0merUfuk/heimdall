---
description: go-net-http service conventions
globs:
  - "**/*.go"
---

# go-net-http Service Rules

## Stack: go / net-http

- Architecture: layered (internal/ packages + cmd/heimdall/ CLI)
- Testing: go-test
- Data layer: none

## Conventions

- Follow the layered architecture pattern: each internal/ package has a single concern (audio, transcriber, analyzer, mixer, output, config, recovery, session)
- Write tests using go-test
- Reference `.claude/knowledge/go-rules/` for best practices
