---
description: >
  Full release workflow for heimdall. Determines version bump, runs tests,
  builds binaries, tags release, and triggers GoReleaser. Produces both
  heimdall (Go) and heimdall-audio (Swift) binaries for macOS.
argument-hint: "[version]"
allowed-tools: Read, Edit, Write, Grep, Glob, Bash
---

# Release

## When to Use
- When ready to cut a new release
- After all tests pass and code is reviewed

## Arguments
- `version`: semver version to release (e.g., `1.0.0`). If omitted, determines from CHANGELOG.md.

## Execution

### Step 1: Pre-flight Checks

Run in parallel:
- `go build ./...` -- compilation
- `go test ./... -race -count=1` -- tests with race detection
- `go vet ./...` -- static analysis
- `cd audio-helper && swift build -c release` -- Swift binary
- `git status` -- clean working tree required

All must pass. If any fails, abort and report.

### Step 2: Determine Version

If version not provided:
- Read `CHANGELOG.md` for the latest `## vX.Y.Z` entry
- If it says "(Unreleased)", use that version number

### Step 3: Update Version References

- `Makefile`: update `VERSION ?= {version}`
- `CHANGELOG.md`: replace "(Unreleased)" with date
- Commit: `chore: bump version to v{version}`

### Step 4: Tag and Push

```bash
git tag -a v{version} -m "Release v{version}"
git push origin main --tags
```

### Step 5: GoReleaser (if available)

```bash
goreleaser release --clean
```

If GoReleaser is not installed, produce manual release:
```bash
make build
# Package bin/heimdall + bin/heimdall-audio + LICENSE + README.md
```

### Step 6: Verify

- Check GitHub releases page
- Verify binary downloads work
- Update `CHANGELOG.md` with next "(Unreleased)" section
