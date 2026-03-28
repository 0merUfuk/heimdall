**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Core Principles -- go

**Purpose**: Establish the foundational rules of idiomatic Go that every file in the codebase must follow.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Go's design philosophy prioritizes clarity, simplicity, and mechanical sympathy. Unlike languages that offer multiple ways to solve a problem, Go deliberately constrains choice: one formatting tool (`gofmt`), one error handling pattern (explicit returns), one concurrency primitive (goroutines + channels), and one composition model (embedding, not inheritance). These constraints are features, not limitations -- they eliminate style debates and produce codebases that are readable by anyone who knows Go.

The language's type system is structurally typed: a type satisfies an interface if it implements the methods, with no explicit declaration required. This enables powerful decoupling between packages. Combined with Go's strict import cycle enforcement, it naturally produces layered architectures where dependencies flow inward without circular references.

For CLI applications like heimdall, the core principles translate directly: thin `main.go` that wires dependencies, `internal/` packages that own their interfaces, error values that flow upward with context, and goroutine lifecycles tied to `context.Context`. Every principle below serves this architecture.

---

## Rules

### Always
- Format all code with `gofmt` -- no exceptions, no style debates; the tool decides
- Return `error` as the last return value; check immediately with a guard clause
- Use `defer` to place cleanup code next to acquisition code (file closes, mutex unlocks, connection cleanup)
- Prefer composition over inheritance -- embed types and interfaces to build behaviors
- Accept interfaces, return concrete types -- functions consume behavior but expose concrete values
- Design zero values to be useful (`var b bytes.Buffer`, `var mu sync.Mutex` work without init)
- Use `var _ Interface = (*Impl)(nil)` for compile-time interface compliance checks near the type definition
- Keep interfaces small -- one-method interfaces are idiomatic; split anything over 3 methods
- Name exported identifiers without stuttering: `bufio.Reader` not `bufio.BufReader`; `Owner()` not `GetOwner()`
- Use `iota` with typed constants for enumerations -- never raw integers or untyped strings
- Mark errors as first-class values -- handle, wrap, or return them; never ignore silently
- Run `go test -race ./...` to detect data races in all concurrent code

### Never
- Never use `panic` in library code -- `panic` is for truly unrecoverable states in `main()` or `init()` only
- Never swallow errors with `_ = err` -- every error must be handled, wrapped, or explicitly documented as best-effort
- Never define fat interfaces (5+ methods) upfront -- start minimal; the consumer defines what it needs
- Never hide complex initialization in `init()` (network connections, file I/O, global state mutation) -- use explicit constructors
- Never use `Get` prefix on getters -- `Name()` not `GetName()`; setters use `SetName()`
- Never communicate by sharing memory when channels would work -- prefer channels for goroutine coordination in pipelines

---

## Patterns

### Pattern: Guard Clause / Happy-Path Flow

```go
// DO: Return early on error, keep happy path unindented
func processFile(name string) error {
    f, err := os.Open(name)
    if err != nil {
        return fmt.Errorf("processFile: %w", err)
    }
    defer f.Close()

    d, err := f.Stat()
    if err != nil {
        return fmt.Errorf("processFile stat: %w", err)
    }

    return doWork(f, d)
}
```

```go
// DON'T: Pyramid of doom with nested else
func processFileBad(name string) error {
    f, err := os.Open(name)
    if err == nil {
        defer f.Close()
        if d, err := f.Stat(); err == nil {
            return doWork(f, d)
        } else {
            return err
        }
    } else {
        return err
    }
}
```

### Pattern: Accept Interfaces, Return Concrete Types

```go
// DO: Accept io.Reader (behavior), return *Config (concrete)
func LoadConfig(r io.Reader) (*Config, error) {
    var cfg Config
    if err := json.NewDecoder(r).Decode(&cfg); err != nil {
        return nil, fmt.Errorf("loading config: %w", err)
    }
    return &cfg, nil
}
```

```go
// DON'T: Return interface -- forces callers to type-assert
func LoadConfigBad(r io.Reader) (Configurer, error) {
    // Caller now needs .(Config) assertion; lose type safety
    return &Config{}, nil
}
```

### Pattern: Small Interface Composition

```go
// DO: Define minimal interfaces at point of use
type AudioSource interface {
    Stream() <-chan AudioFrame
    Stop() error
}

// Compose only when the composed behavior is a real concept
type ReadWriteCloser interface {
    io.Reader
    io.Writer
    io.Closer
}
```

```go
// DON'T: Large monolithic interface
type AudioService interface {
    Stream() <-chan AudioFrame
    Stop() error
    Start(ctx context.Context) error
    SampleRate() int
    Channels() int
    Resample(rate int) error
    // ... 10 more methods -- impossible to mock, hard to satisfy
}
```

### Pattern: Defer for Cleanup

```go
// DO: Defer immediately after acquisition
func writeFile(path string, data []byte) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.Write(data)
    return err
}
```

```go
// DON'T: Manual cleanup in each return path
func writeFileBad(path string, data []byte) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    if _, err = f.Write(data); err != nil {
        f.Close() // easy to forget in complex functions
        return err
    }
    f.Close()
    return nil
}
```

### Pattern: Zero Value Usefulness

```go
// DO: Design structs so the zero value is ready
type SafeCounter struct {
    mu sync.Mutex // zero value is unlocked
    n  int        // zero value is 0
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.n++
}
```

```go
// DON'T: Require special initialization for basic use
type BadCounter struct {
    initialized bool
    n           int
}

func (c *BadCounter) Inc() {
    if !c.initialized {
        panic("call Init() first") // fragile and un-Go-like
    }
    c.n++
}
```

### Pattern: Compile-Time Interface Assertion

```go
// DO: Assert interface compliance at compile time
var _ io.Writer = (*MyWriter)(nil)
var _ AudioSource = (*MalgoSource)(nil)

// If MalgoSource doesn't implement AudioSource, compilation fails.
// Place this near the type definition, not in tests.
```

```go
// DON'T: Rely on runtime discovery
// Interfaces are discovered at runtime only -- bugs surface late
// No compile-time assertion means interface drift goes unnoticed
```

### Pattern: Iota for Typed Constants

```go
// DO: Use iota for enumerations with a String() method
type LogLevel int

const (
    LogDebug LogLevel = iota
    LogInfo
    LogWarn
    LogError
)

func (l LogLevel) String() string {
    switch l {
    case LogDebug:
        return "DEBUG"
    case LogInfo:
        return "INFO"
    case LogWarn:
        return "WARN"
    case LogError:
        return "ERROR"
    default:
        return "UNKNOWN"
    }
}
```

```go
// DON'T: Use raw integers or untyped strings for enumerations
const (
    DEBUG = 0
    INFO  = 1
    // No type safety; can pass any int accidentally
)
```

### Pattern: Context Propagation for Cancellation

```go
// DO: Thread context through all long-running operations
func (s *RecordService) Run(ctx context.Context) error {
    g, gCtx := errgroup.WithContext(ctx)

    g.Go(func() error {
        return s.capture(gCtx) // respects cancellation
    })
    g.Go(func() error {
        return s.transcribe(gCtx) // respects cancellation
    })

    return g.Wait() // blocks until all goroutines complete or one fails
}
```

```go
// DON'T: Ignore context and rely on manual shutdown signals
func (s *RecordService) Run() error {
    go s.capture()     // no context -- cannot cancel on shutdown
    go s.transcribe()  // no context -- goroutine leak if caller exits
    select {}          // blocks forever
}
```

### Pattern: Package-Level Doc Comments

```go
// DO: Every package has a doc comment in doc.go or the primary file
// Package audio provides audio capture abstractions for the heimdall pipeline.
//
// It defines the AudioSource interface for platform-swappable audio capture
// and provides implementations for microphone input (via malgo) and system
// audio (via the Swift subprocess helper).
package audio
```

```go
// DON'T: Skip the package comment
package audio // no documentation -- godoc shows nothing
```

---

## Checklist

- [ ] All code passes `gofmt -l .` (zero output = no changes needed)
- [ ] All code passes `go vet ./...` with zero warnings
- [ ] Every exported function has a doc comment starting with the function name
- [ ] Error return is always the last return value
- [ ] All error values are checked (no discarded `err` in production paths)
- [ ] No `panic` in library code (only in `main()` or `init()` for truly unrecoverable states)
- [ ] Interfaces are defined at point of consumption, not in the providing package
- [ ] No `GetXxx()` naming -- use `Xxx()` for getters
- [ ] `defer` used for all resource cleanup (files, connections, locks)
- [ ] Zero values are functional without a special `Init()` call
- [ ] Compile-time interface assertions present for all critical interface implementations
- [ ] `go test -race ./...` passes (no data races)
- [ ] No `init()` functions performing I/O or network calls
- [ ] Interfaces have 3 or fewer methods; larger behaviors composed from smaller interfaces
