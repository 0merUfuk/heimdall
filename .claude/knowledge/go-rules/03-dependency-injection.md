**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Dependency Injection -- go

**Purpose**: Define how dependencies are wired in a Go CLI application -- when to use manual injection, constructors, and frameworks.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Dependency injection in Go is not a framework concern -- it is a language-level pattern enabled by interfaces and constructors. Every component that has dependencies should expose a `NewXxx(dep1 Interface1, dep2 Interface2) *Xxx` constructor. The composition root (`main.go`) calls these constructors in the correct order, passing concrete implementations that satisfy the required interfaces. This is manual DI, and for CLI applications with fewer than 15 components, it is the right choice.

Go's structural typing makes DI particularly clean: a `DeepgramClient` satisfies a `Transcriber` interface without any `implements` declaration. The compiler verifies this at build time when you pass the concrete type to a constructor that accepts the interface. Combined with compile-time interface assertions (`var _ Transcriber = (*DeepgramClient)(nil)`), you get full static verification of your dependency graph.

Frameworks like `go.uber.org/fx` and `google/wire` exist for larger applications (15+ components with lifecycle orchestration). Fx provides runtime reflection-based wiring with lifecycle hooks (`OnStart`/`OnStop`). Wire generates explicit wiring code at compile time with zero runtime overhead. For a CLI like heimdall (approximately 8-10 components), neither is warranted -- the manual wiring in `main.go` is readable, traceable, and debuggable.

---

## Rules

### Always
- Use manual constructor injection in `main.go` for CLI apps with fewer than 15 components
- Define `NewXxx(dep1 Interface1, dep2 Interface2) *Xxx` constructors for every component with dependencies
- Accept interfaces in constructors, not concrete types -- enables test fakes without frameworks
- Pass specific config values to constructors (`apiKey string`, `sampleRate int`), not the full config struct
- Pass `context.Context` as a function argument at call time, never as a constructor dependency
- Place all concrete type construction in the composition root (`main.go`)
- Use compile-time interface assertions for all injected types

### Never
- Never use global variables as an injection mechanism (`var db *sql.DB` at package scope)
- Never wire dependencies in `init()` -- dependencies wired there are invisible, untestable, and always-on
- Never do I/O in constructors (opening connections, dialing WebSocket endpoints, reading files) -- use `Start()` methods or lifecycle hooks
- Never use a service locator pattern (`container.Get("transcriber")` returning `interface{}`)
- Never inject the entire `Config` struct -- it hides what each component actually needs
- Never add `go.uber.org/fx` to a CLI with fewer than 10 components -- the framework overhead exceeds the benefit

---

## Patterns

### Pattern: Manual Constructor Injection (CLI -- Recommended)

```go
// DO: Explicit wiring in main() -- crystal clear, zero magic
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/user/app/internal/audio"
    "github.com/user/app/internal/transcriber"
    "github.com/user/app/internal/analyzer"
    "github.com/user/app/internal/output"
    "github.com/user/app/internal/usecase"
    "github.com/user/app/internal/config"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        fmt.Fprintln(os.Stderr, "config error:", err)
        os.Exit(1)
    }

    audioSrc := audio.NewMalgoSource(cfg.SampleRate, cfg.Channels)
    txcr     := transcriber.NewDeepgramClient(cfg.DeepgramAPIKey)
    anlzr    := analyzer.NewClaudeAnalyzer(cfg.ClaudeAPIKey, cfg.ClaudeModel)
    writer   := output.NewObsidianWriter(cfg.VaultPath)

    svc := usecase.NewRecordService(audioSrc, txcr, anlzr, writer)

    if err := svc.Run(context.Background()); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

```go
// DON'T: Global singletons pretending to be DI
var globalAudio = audio.NewMalgoSource(44100, 2) // untestable, always-on
```

### Pattern: Constructor Function Convention

```go
// DO: Constructors accept config values, not config structs
package transcriber

type DeepgramClient struct {
    apiKey  string
    conn    *websocket.Conn
    results chan domain.Segment
}

func NewDeepgramClient(apiKey string) *DeepgramClient {
    return &DeepgramClient{
        apiKey:  apiKey,
        results: make(chan domain.Segment, 64),
    }
}
```

```go
// DON'T: Constructor reaches for globals or does heavy I/O
func NewDeepgramClientBad() *DeepgramClient {
    key := os.Getenv("DEEPGRAM_KEY")              // hidden dependency on env
    conn, _ := websocket.Dial(defaultURL, nil, nil) // I/O in constructor
    return &DeepgramClient{apiKey: key, conn: conn}
}
```

### Pattern: Parameter Structs for Many Dependencies

```go
// DO: Use a params struct when a component has more than 3 dependencies
type RecordServiceParams struct {
    Audio       usecase.AudioSource
    Transcriber usecase.Transcriber
    Analyzer    usecase.Analyzer
    Output      usecase.OutputWriter
    Logger      *slog.Logger
}

func NewRecordService(p RecordServiceParams) *RecordService {
    return &RecordService{
        audio:       p.Audio,
        transcriber: p.Transcriber,
        analyzer:    p.Analyzer,
        output:      p.Output,
        log:         p.Logger,
    }
}
```

```go
// DON'T: Long constructor argument lists (hard to read, fragile ordering)
func NewRecordServiceBad(
    a usecase.AudioSource,
    t usecase.Transcriber,
    an usecase.Analyzer,
    o usecase.OutputWriter,
    l *slog.Logger,
) *RecordService { /* ... */ }
```

### Pattern: Testing with Fakes (No Framework Needed)

```go
// DO: Inject fake implementations via constructors in tests
package usecase_test

type fakeTranscriber struct {
    segments []domain.Segment
    err      error
}

func (f *fakeTranscriber) Connect(_ context.Context) error { return nil }
func (f *fakeTranscriber) Send(_ domain.AudioFrame) error  { return f.err }
func (f *fakeTranscriber) Receive() <-chan domain.Segment {
    ch := make(chan domain.Segment, len(f.segments))
    for _, s := range f.segments {
        ch <- s
    }
    close(ch)
    return ch
}
func (f *fakeTranscriber) Close() error { return nil }

func TestRecordService_AnalyzerFallback(t *testing.T) {
    svc := NewRecordService(RecordServiceParams{
        Audio:       &fakeAudioSource{},
        Transcriber: &fakeTranscriber{segments: testSegments},
        Analyzer:    &failingAnalyzer{err: usecase.ErrAnalysisFailed},
        Output:      &fakeOutput{},
    })

    _, err := svc.Run(context.Background())
    if err != nil {
        t.Errorf("expected graceful degradation, got: %v", err)
    }
}
```

### Pattern: Uber Fx for Server/Long-Running Apps (When Warranted)

```go
// DO: Use fx.New with lifecycle hooks for long-running services (15+ components)
package main

import (
    "go.uber.org/fx"
    "github.com/user/app/internal/audio"
    "github.com/user/app/internal/server"
)

func main() {
    fx.New(
        audio.Module,
        server.Module,
        fx.Invoke(startApp),
    ).Run()
}

// internal/audio/module.go
var Module = fx.Module("audio",
    fx.Provide(NewMalgoSource),
)

func NewMalgoSource(lc fx.Lifecycle, cfg *Config) *MalgoSource {
    src := &MalgoSource{sampleRate: cfg.SampleRate}
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error { return src.Start(ctx) },
        OnStop:  func(ctx context.Context) error { return src.Stop() },
    })
    return src
}
```

### Pattern: Google Wire (Compile-Time, Zero Overhead)

```go
// wire.go -- only built with wire tool, not normal go build
//go:build wireinject

package main

import (
    "github.com/google/wire"
    "github.com/user/app/internal/audio"
    "github.com/user/app/internal/transcriber"
    "github.com/user/app/internal/usecase"
)

func InitializeRecordService(cfg *Config) (*usecase.RecordService, error) {
    wire.Build(
        audio.NewMalgoSource,
        transcriber.NewDeepgramClient,
        usecase.NewRecordService,
    )
    return nil, nil // Wire replaces this body with generated code
}
```

### Pattern: Lazy Initialization with Start() Methods

```go
// DO: Separate construction from initialization
type DeepgramClient struct {
    apiKey string
    conn   *websocket.Conn // nil until Start() is called
}

func NewDeepgramClient(apiKey string) *DeepgramClient {
    return &DeepgramClient{apiKey: apiKey} // no I/O
}

func (d *DeepgramClient) Start(ctx context.Context) error {
    conn, _, err := websocket.Dial(ctx, deepgramURL, nil)
    if err != nil {
        return fmt.Errorf("deepgram start: %w", err)
    }
    d.conn = conn
    return nil
}

func (d *DeepgramClient) Stop() error {
    if d.conn != nil {
        return d.conn.Close(websocket.StatusNormalClosure, "")
    }
    return nil
}
```

```go
// DON'T: Open connections in the constructor
func NewDeepgramClient(apiKey string) (*DeepgramClient, error) {
    conn, _, err := websocket.Dial(context.Background(), deepgramURL, nil)
    if err != nil {
        return nil, err // constructor fails in tests without network
    }
    return &DeepgramClient{apiKey: apiKey, conn: conn}, nil
}
```

### Pattern: Dependency Graph Visualization

```go
// DO: Keep the composition root readable as a dependency graph
// cmd/heimdall/main.go -- read top-to-bottom as a wiring diagram
func buildServices(cfg *config.Config) (*usecase.RecordService, error) {
    // Layer 1: Infrastructure adapters
    mic     := audio.NewMalgoSource(cfg.SampleRate, cfg.Channels)
    sys     := audio.NewSystemAudioSource(cfg.SwiftHelperPath)
    txcr    := transcriber.NewDeepgramClient(cfg.DeepgramKey)
    anlzr   := analyzer.NewClaudeAnalyzer(cfg.ClaudeKey, cfg.ClaudeModel)
    writer  := output.NewObsidianWriter(cfg.VaultPath)

    // Layer 2: Domain services
    mixer   := mixer.New(mic, sys, cfg.SampleRate)

    // Layer 3: Use case orchestration
    svc := usecase.NewRecordService(usecase.RecordServiceParams{
        Audio:       mixer,
        Transcriber: txcr,
        Analyzer:    anlzr,
        Output:      writer,
    })

    return svc, nil
}
```

---

## Checklist

- [ ] All dependencies passed via constructor arguments (no global vars for dependencies)
- [ ] Constructors accept interfaces, not concrete implementations
- [ ] No I/O in constructors -- only in `Start()` methods or lifecycle hooks
- [ ] `context.Context` passed as function argument, never injected as constructor dependency
- [ ] No `init()` functions used for dependency wiring
- [ ] Fakes (implementing interfaces) used in unit tests -- no DI framework dependency in tests
- [ ] Composition root (`main.go`) is the only place where all concrete types are imported together
- [ ] Constructor signatures document actual dependencies (no mega-struct `Config` parameter)
- [ ] Compile-time interface assertions (`var _ Interface = (*Impl)(nil)`) present for all injected types
- [ ] `go.uber.org/fx` not added to CLI tools with fewer than 10 components
- [ ] Manual DI graph is readable top-to-bottom in `main.go`
