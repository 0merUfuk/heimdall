**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Project Structure -- go

**Purpose**: Define the canonical directory layout and file organization for a Go CLI application with layered architecture.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Go projects follow a convention-driven layout where the directory structure communicates architectural intent. The `cmd/` directory holds entry points (one per binary), `internal/` enforces encapsulation at the compiler level (external modules cannot import it), and package names map directly to directory names. This layout is not arbitrary -- it is the de-facto standard adopted by major Go projects including Kubernetes, Docker, and Cobra itself.

For CLI applications, the structure revolves around three principles: thin entry points in `cmd/`, all business logic in `internal/`, and embedded assets via `//go:embed`. The Go compiler enforces that `internal/` packages cannot be imported by external modules, giving you a free encapsulation boundary without any runtime mechanism. Test files live alongside the code they test -- there is no separate `test/` directory for unit tests.

The `Makefile` at the project root provides discoverable commands (`build`, `test`, `lint`, `clean`, `doctor`) so developers never need to remember multi-step shell invocations. Non-Go assets (templates, configs, static files) are embedded at compile time with `//go:embed`, ensuring the binary is self-contained and independent of the working directory.

---

## Rules

### Always
- Place each binary's entry point in `cmd/{appname}/main.go` -- keep it under 30 lines; no business logic
- Put all application logic in `internal/` -- the compiler enforces import restrictions for you
- Name packages as lowercase, short, single-word identifiers matching the directory name
- Place test files alongside the code they test (`auth.go` + `auth_test.go` in the same directory)
- Use `//go:embed` to bundle non-Go assets (templates, configs) at compile time
- Provide a `Makefile` with `build`, `test`, `lint`, `clean` targets at the project root
- Use `testdata/` subdirectories within packages for test fixture files (Go ignores them during builds)
- Use a fully qualified module path in `go.mod` (e.g., `github.com/org/repo`)
- Organize `internal/` packages by domain/feature, not by technical layer alone

### Never
- Never create a `/src` directory -- this is a Java/Maven convention that conflicts with Go tooling
- Never spread a single package across multiple directories (exception: `*_test` package names)
- Never put business logic in `main.go` -- it must only wire dependencies and delegate
- Never use `pkg/` unless you explicitly intend to provide importable code for external consumers
- Never use underscores or camelCase in package names
- Never rely on relative paths at runtime for templates or configs -- use `//go:embed`

---

## Patterns

### Pattern: CLI Project Standard Layout

```
heimdall/
├── cmd/
│   └── heimdall/
│       └── main.go              // Thin entry point: parse flags, build deps, call app
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── audio/
│   │   ├── source.go            // AudioSource interface
│   │   ├── malgo.go             // MalgoSource implementation
│   │   ├── malgo_test.go
│   │   └── testdata/
│   │       └── sample.pcm       // Test fixture
│   ├── transcriber/
│   │   ├── transcriber.go       // Transcriber interface
│   │   ├── deepgram.go          // DeepgramTranscriber implementation
│   │   └── deepgram_test.go
│   ├── analyzer/
│   │   ├── analyzer.go          // Analyzer interface
│   │   └── claude.go            // ClaudeAnalyzer implementation
│   ├── mixer/
│   │   └── mixer.go             // Resample, convert, interleave
│   ├── output/
│   │   ├── writer.go            // OutputWriter interface + ObsidianWriter
│   │   └── writer_test.go
│   └── recovery/
│       ├── recovery.go          // Crash recovery
│       └── recovery_test.go
├── templates/
│   └── meeting-note.tmpl        // Embedded via //go:embed
├── docs/
│   └── architecture/            // Design docs (human-written)
├── go.mod
├── go.sum
└── Makefile
```

### Pattern: Thin main.go

```go
// DO: main.go only wires and delegates
package main

import (
    "fmt"
    "os"

    "github.com/user/heimdall/cmd/heimdall/commands"
)

func main() {
    if err := commands.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

```go
// DON'T: Business logic in main
package main

func main() {
    // 200 lines of config parsing, WebSocket calls, file writes...
    cfg, err := parseConfig(os.Args)
    if err != nil { /* ... */ }
    conn, err := openWebSocket(cfg.DeepgramKey)
    // Completely untestable; violates separation of concerns
}
```

### Pattern: Interface + Implementation Split Within Package

```go
// internal/audio/source.go -- interface definition
package audio

import "context"

// AudioSource abstracts platform-specific audio capture.
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
    SampleRate() int
    Channels() int
}
```

```go
// internal/audio/malgo.go -- implementation
package audio

// MalgoSource implements AudioSource using the malgo library.
type MalgoSource struct {
    sampleRate int
    channels   int
    stream     chan AudioFrame
}

var _ AudioSource = (*MalgoSource)(nil) // compile-time assertion

func NewMalgoSource(sampleRate, channels int) *MalgoSource {
    return &MalgoSource{
        sampleRate: sampleRate,
        channels:   channels,
        stream:     make(chan AudioFrame, 256),
    }
}
```

### Pattern: Embed Templates and Static Assets

```go
// DO: Bundle at compile time with //go:embed
package output

import (
    "embed"
    "text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

func loadTemplates() (*template.Template, error) {
    return template.ParseFS(templateFS, "templates/*.tmpl")
}
```

```go
// DON'T: Rely on relative path at runtime
func loadTemplatesBad() (*template.Template, error) {
    return template.ParseFiles("./templates/meeting-note.tmpl")
    // Breaks when binary runs from a different working directory
}
```

### Pattern: Cobra Command Structure with Internal Delegation

```go
// cmd/heimdall/commands/root.go
package commands

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{Use: "heimdall"}

func Execute() error {
    return rootCmd.Execute()
}

func init() {
    rootCmd.AddCommand(newRecordCmd())
    rootCmd.AddCommand(newVersionCmd())
}
```

```go
// cmd/heimdall/commands/record.go
package commands

func newRecordCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "record",
        Short: "Record a meeting",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Thin: parse flags, build service, delegate
            cfg := configFromCtx(cmd.Context())
            svc := app.NewRecordService(cfg)
            return svc.Run(cmd.Context())
        },
    }
}
```

### Pattern: go.mod Module Path Convention

```go
// DO: Use full import path even for private projects
module github.com/yourorg/heimdall

go 1.25

require (
    github.com/spf13/cobra v1.8.0
)
```

```go
// DON'T: Vague or non-rooted module names
module heimdall   // Can't be imported by others; go get won't work
module myapp      // Ambiguous; conflicts with local $GOPATH conventions
```

### Pattern: Makefile with Standard Targets

```makefile
# DO: Provide discoverable targets for common operations
VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test lint doctor

build:
	go build $(LDFLAGS) -o bin/heimdall ./cmd/heimdall

clean:
	rm -rf bin/

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...
	go vet ./...

doctor:
	@echo "Checking prerequisites..."
	@which go > /dev/null 2>&1 && echo "  ok  Go installed" || echo "  FAIL  Go not found"
	@sw_vers -productVersion 2>/dev/null || echo "  FAIL  Not macOS"
```

### Pattern: .gitignore for Go CLI Projects

```gitignore
# DO: Cover all expected patterns
bin/
dist/
*.exe
*.test
*.out
coverage.out

# OS files
.DS_Store
Thumbs.db

# IDE files
.idea/
.vscode/
*.swp

# Secrets (never committed)
.env
*.pem
```

---

## Checklist

- [ ] `cmd/{appname}/main.go` contains 30 lines or fewer; no business logic
- [ ] All application logic is in `internal/`
- [ ] `pkg/` only exists if external consumers are explicitly intended
- [ ] No `/src` directory at project root
- [ ] Package names match directory names (lowercase, no underscores)
- [ ] Each directory contains exactly one package
- [ ] `_test.go` files are in the same directory as the code they test
- [ ] Non-Go assets (templates, configs) embedded with `//go:embed`
- [ ] `go.mod` uses a fully qualified module path
- [ ] `Makefile` provides `build`, `test`, `lint`, `clean` targets
- [ ] `internal/` packages organized by domain/feature, not by layer-only names
- [ ] Compile-time interface assertions present for key interface implementations
- [ ] `testdata/` used for test fixtures within packages
- [ ] `go.sum` committed to version control
