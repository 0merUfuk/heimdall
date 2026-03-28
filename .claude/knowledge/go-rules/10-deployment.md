**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Deployment -- go

**Purpose**: Define release automation, cross-compilation, distribution, and code signing patterns for Go CLI binaries.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Go CLI distribution follows a well-established pipeline: GoReleaser handles cross-compilation, archive creation, checksum generation, GitHub Release publishing, and Homebrew formula generation in a single declarative configuration. The release is triggered by pushing a semantic version tag (`v1.0.0`), and the entire pipeline runs in CI with no manual steps.

For macOS-targeting CLIs, code signing and notarization are non-negotiable distribution requirements. Starting with macOS 13+, Gatekeeper blocks unsigned binaries -- users cannot run them without terminal workarounds that train bad security habits. An Apple Developer ID certificate, combined with `codesign` and `xcrun notarytool`, produces binaries that run without quarantine dialogs. GoReleaser Pro includes a built-in notarize pipe; the open-source version requires a custom hook.

A dual-binary architecture (Go CLI + Swift audio helper) adds complexity to the release pipeline. The Swift helper cannot be cross-compiled -- it must be built natively on macOS. This means the GitHub Actions release job must run on `macos-latest` (which has cost implications: macOS runners are approximately 10x more expensive than Linux runners). GoReleaser archives both binaries together, and the Homebrew formula installs both to `bin/`.

---

## Rules

### Always
- Use GoReleaser for all release automation -- do not script cross-compilation manually
- Commit `.goreleaser.yaml` to version control; validate with `goreleaser check` in CI
- Trigger releases via git tags only (`v*` pattern in GitHub Actions) -- never on branch push
- Test the release pipeline with `goreleaser release --snapshot --clean` before the first real release
- Cross-compile for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` at minimum
- Use `ldflags` to embed version, commit, and build date at link time
- Generate checksums file (`checksums.txt`) with SHA256 for all archives
- Provide a Homebrew tap for macOS distribution
- Code-sign and notarize macOS binaries before any public release
- Keep `go.sum` in version control; fail CI if `go mod tidy` changes it
- Set `CGO_ENABLED=0` for the Go binary when no CGO dependencies exist

### Never
- Never run `goreleaser release` without a git tag -- use `--snapshot` for local testing
- Never skip `fetch-depth: 0` in CI checkout -- GoReleaser needs full history for changelog
- Never publish pre-release tags (`-beta`, `-rc`) as stable Homebrew formulas
- Never distribute unsigned macOS binaries -- Gatekeeper quarantine blocks them
- Never hardcode version strings in source -- embed at link time with `ldflags`
- Never cross-compile Swift -- it must be built natively on macOS

---

## Patterns

### Pattern: .goreleaser.yaml -- Production Config

```yaml
# DO: Complete config with cross-compilation, archives, checksums, brew
version: 2

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: heimdall
    main: ./cmd/heimdall
    binary: heimdall
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: default
    format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - README.md
      - LICENSE

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

snapshot:
  name_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"

brews:
  - name: heimdall
    description: "CLI meeting companion -- captures, transcribes, and analyzes meetings"
    homepage: "https://github.com/yourorg/heimdall"
    folder: Formula
    repository:
      owner: yourorg
      name: homebrew-tap
      branch: "releases/{{ .Version }}"
      pull_request:
        enabled: true
        base:
          owner: yourorg
          name: homebrew-tap
          branch: main
```

```yaml
# DON'T: Minimal config that skips checksums, cross-compilation, or Homebrew
builds:
  - main: ./cmd/heimdall
# Only current platform, no ldflags, no checksums, no brew
```

### Pattern: Version Embedding with ldflags

```go
// DO: Declare version vars in main package for linker injection
// cmd/heimdall/main.go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("heimdall %s (commit: %s, built: %s)\n", version, commit, date)
    },
}
```

```go
// DON'T: Hardcode version in source
const Version = "1.0.0" // requires source change on every release
```

### Pattern: GitHub Actions Release Workflow

```yaml
# DO: Trigger GoReleaser on version tags only
name: Release
on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: macos-latest    # macOS for Swift helper build + notarization
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0     # GoReleaser needs full history for changelog

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

```yaml
# DON'T: Run goreleaser on every push to main
on:
  push:
    branches: [main] # triggers on every commit, creates spurious releases
```

### Pattern: macOS Notarization Hook

```yaml
# DO: Notarize macOS binaries in GoReleaser
notarize:
  macos:
    - enabled: '{{ isEnvSet "MACOS_SIGN_P12" }}'
      sign:
        certificate: "{{.Env.MACOS_SIGN_P12}}"
        password: "{{.Env.MACOS_SIGN_PASSWORD}}"
      notarize:
        issuer_id: "{{.Env.NOTARIZE_ISSUER_ID}}"
        key_id: "{{.Env.NOTARIZE_KEY_ID}}"
        key: "{{.Env.NOTARIZE_KEY}}"
```

```yaml
# DON'T: Skip signing and tell users to run xattr workarounds
# This is user-hostile and trains bad security habits
```

### Pattern: Homebrew Tap Repository Structure

```
homebrew-tap/
├── Formula/
│   └── heimdall.rb          # auto-generated by GoReleaser
└── README.md

# Formula file (auto-generated):
class Heimdall < Formula
  desc "CLI meeting companion"
  homepage "https://github.com/yourorg/heimdall"
  url "https://github.com/yourorg/heimdall/releases/download/v1.0.0/heimdall_1.0.0_darwin_arm64.tar.gz"
  sha256 "abc123..."
  version "1.0.0"

  def install
    bin.install "heimdall"
    bin.install "heimdall-audio"  # Swift helper bundled in archive
  end
end

# User installs with:
# brew tap yourorg/tap
# brew install heimdall
```

### Pattern: Local Release Testing

```bash
# DO: Test with --snapshot before the first real release
goreleaser release --snapshot --clean
# Builds and archives without publishing; catches config errors early

# DON'T: Run a real release to test config
goreleaser release --clean  # publishes to GitHub; cannot undo
```

### Pattern: CI Build Pipeline (Non-Release)

```yaml
# DO: Validate on every PR -- build, test, vet, lint, goreleaser check
name: CI
on:
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Build
        run: go build ./...

      - name: Test
        run: go test -race -count=1 ./...

      - name: Vet
        run: go vet ./...

      - name: GoReleaser check
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: check

      - name: Govulncheck
        uses: golang/govulncheck-action@v1
        with:
          go-version-input: stable
          go-package: ./...
```

### Pattern: Dual-Binary Archive (Go + Swift)

```yaml
# DO: Build Swift helper separately, include in GoReleaser archive
# In CI, before GoReleaser:
- name: Build Swift helper
  if: runner.os == 'macOS'
  run: |
    cd audio-helper
    swift build -c release
    cp .build/release/heimdall-audio ../bin/

# In .goreleaser.yaml:
archives:
  - id: default
    format: tar.gz
    files:
      - README.md
      - LICENSE
      - bin/heimdall-audio   # Swift binary bundled alongside Go binary
```

---

## Checklist

- [ ] `.goreleaser.yaml` committed to version control
- [ ] `goreleaser check` runs in CI on every PR
- [ ] Cross-compilation targets: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`
- [ ] `ldflags` embed `version`, `commit`, and `date` at build time
- [ ] `CGO_ENABLED=0` set for Go binary (Swift helper built separately)
- [ ] Checksums file (`checksums.txt`) generated with SHA256
- [ ] GitHub Actions workflow triggers only on `v*` tags
- [ ] `fetch-depth: 0` set in `actions/checkout` step
- [ ] Homebrew tap repository configured with GoReleaser `brews` block
- [ ] macOS binaries code-signed and notarized before public release
- [ ] `goreleaser release --snapshot --clean` tested locally before first real release
- [ ] Pre-release tags (`-beta`, `-rc`) excluded from stable Homebrew formula
- [ ] `go.sum` committed and `go mod tidy` checked in CI
- [ ] `heimdall version` command outputs embedded version metadata
- [ ] Swift helper built on `macos-latest` runner in CI (cannot cross-compile)
