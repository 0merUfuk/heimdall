**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Error Handling -- go

**Purpose**: Define error creation, wrapping, comparison, and propagation rules for idiomatic Go applications.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Errors in Go are values, not exceptions. Every function that can fail returns `error` as its last return value. The caller checks the error immediately with a guard clause and decides whether to handle it, wrap it with context, or return it upward. This explicit error flow is a deliberate language design choice that trades verbosity for clarity -- you always know exactly where errors are checked and where they propagate.

Since Go 1.13, the standard library provides `errors.Is` and `errors.As` for comparing and extracting errors through wrapped chains. The `%w` verb in `fmt.Errorf` wraps an error while preserving the chain, allowing callers to use `errors.Is` to match sentinel errors deep in the stack. Go 1.20 added `errors.Join` for aggregating multiple independent errors (useful for config validation). These three mechanisms -- wrapping, sentinel comparison, and type extraction -- cover virtually all error handling needs without external libraries.

The most impactful rule is **wrap at boundaries, not every level**. Adding `fmt.Errorf("context: %w", err)` in every private helper produces verbose, redundant chains like `save: write: encode: marshal: ...`. Wrap only when crossing package or subsystem boundaries where the context adds genuine diagnostic value. Log errors exactly once, at the outermost handler (CLI command, HTTP handler) -- never log-and-return through multiple layers.

---

## Rules

### Always
- Check every returned error -- never assign to `_` in production code
- Add context when returning errors across package boundaries: `fmt.Errorf("context: %w", err)`
- Use `errors.Is(err, target)` instead of `==` for sentinel comparison -- it traverses wrapped chains
- Use `errors.As(err, &target)` instead of type assertions -- it works through wrapped chains
- Keep error messages lowercase with no trailing punctuation -- they compose into larger messages
- Return errors upward; log exactly once at the outermost handler (CLI command, HTTP handler)
- Implement `Unwrap() error` on custom error types for chain compatibility
- Use `errors.Join` (Go 1.20+) for multi-error aggregation in validation

### Never
- Never log-and-return errors -- logging at every layer produces duplicate noise
- Never wrap at every call level -- wrap only at meaningful boundaries (package API surface, subsystem entry points)
- Never compare errors with `==` -- it breaks when errors are wrapped
- Never use type assertions (`err.(*MyError)`) -- use `errors.As` which traverses the chain
- Never call `log.Fatal` or `os.Exit` inside library code -- return the error to the caller
- Never match on `err.Error()` string content -- it is fragile across library versions

---

## Patterns

### Pattern: Wrapping Errors with Context

```go
// DO: Wrap with %w to preserve chain and add context
func readConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("readConfig %q: %w", path, err)
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("readConfig unmarshal: %w", err)
    }
    return &cfg, nil
}
```

```go
// DON'T: Discard original error with %v
func readConfigBad(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("readConfig %q: %v", path, err)
        // Chain broken; errors.Is won't match os.ErrNotExist
    }
    return nil, nil
}
```

### Pattern: Sentinel Errors with errors.Is

```go
// DO: Define sentinel, wrap on emit, unwrap on check
var ErrNotFound = errors.New("not found")

func GetItem(id string) (*Item, error) {
    if !exists(id) {
        return nil, fmt.Errorf("GetItem %q: %w", id, ErrNotFound)
    }
    return item, nil
}

func handler(id string) {
    _, err := GetItem(id)
    if errors.Is(err, ErrNotFound) {
        // Correctly traverses wrapped chain
        fmt.Println("item not found")
    }
}
```

```go
// DON'T: Compare with == -- breaks when error is wrapped
if err == ErrNotFound {
    // This fails after fmt.Errorf("...: %w", ErrNotFound)
}
```

### Pattern: Custom Error Types with errors.As

```go
// DO: Implement error interface; use errors.As for extraction
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation: field %q: %s", e.Field, e.Message)
}

func process(input Input) error {
    if input.Name == "" {
        return &ValidationError{Field: "name", Message: "required"}
    }
    return nil
}

func handle(input Input) {
    err := process(input)
    var ve *ValidationError
    if errors.As(err, &ve) {
        fmt.Printf("invalid field: %s\n", ve.Field)
    }
}
```

```go
// DON'T: Type-assert directly -- breaks with wrapping
if ve, ok := err.(*ValidationError); ok {
    // Fails if err is wrapped with fmt.Errorf("...: %w", err)
}
```

### Pattern: errors.Join for Multi-Error Aggregation (Go 1.20+)

```go
// DO: Collect all errors before returning
func validateConfig(cfg Config) error {
    var errs []error
    if cfg.APIKey == "" {
        errs = append(errs, errors.New("api_key is required"))
    }
    if cfg.OutputDir == "" {
        errs = append(errs, errors.New("output_dir is required"))
    }
    if cfg.SampleRate <= 0 {
        errs = append(errs, errors.New("sample_rate must be positive"))
    }
    return errors.Join(errs...) // returns nil if slice is empty
}
```

```go
// DON'T: Return on first error when multiple validations are independent
func validateConfigBad(cfg Config) error {
    if cfg.APIKey == "" {
        return errors.New("api_key is required") // hides remaining issues
    }
    if cfg.OutputDir == "" {
        return errors.New("output_dir is required")
    }
    return nil
}
```

### Pattern: Opaque Errors -- Assert Behavior, Not Type

```go
// DO: Test behavior via interface, not type identity
type temporary interface {
    Temporary() bool
}

func isTemporary(err error) bool {
    var t temporary
    return errors.As(err, &t) && t.Temporary()
}

func withRetry(op func() error) error {
    for i := 0; i < 3; i++ {
        err := op()
        if err == nil || !isTemporary(err) {
            return err
        }
        time.Sleep(time.Duration(i+1) * time.Second)
    }
    return fmt.Errorf("operation failed after retries")
}
```

```go
// DON'T: Import a package just to type-switch its private error types
switch err.(type) {
case *net.OpError:    // tight coupling to net package internals
case *url.Error:      // breaks if the underlying package refactors
}
```

### Pattern: Logging at the Boundary Only

```go
// DO: Wrap and return in inner layers; log once at the top
// internal/transcriber/deepgram.go
func (d *DeepgramTranscriber) Connect(ctx context.Context) error {
    if err := d.dial(ctx); err != nil {
        return fmt.Errorf("transcriber connect: %w", err)
    }
    return nil
}

// cmd/heimdall/commands/record.go (delivery layer -- log here)
func runRecord(cmd *cobra.Command, args []string) error {
    if err := svc.Run(cmd.Context()); err != nil {
        slog.Error("record failed", "error", err)
        return err
    }
    return nil
}
```

```go
// DON'T: Log at every layer -- produces duplicate noise
func (d *DeepgramTranscriber) Connect(ctx context.Context) error {
    if err := d.dial(ctx); err != nil {
        log.Printf("dial failed: %v", err) // logged here
        return fmt.Errorf("connect: %w", err) // AND returned upward to be logged again
    }
    return nil
}
```

### Pattern: Graceful Degradation via Error Type Branching

```go
// DO: Use sentinel errors to drive graceful degradation in the pipeline
func (s *RecordService) Run(ctx context.Context) (*MeetingNote, error) {
    segments, err := s.transcribe(ctx)
    if err != nil {
        return nil, fmt.Errorf("record: %w", err)
    }

    note, err := s.analyzer.Summarize(ctx, segments)
    if err != nil {
        // Claude failed -- fall back to raw transcript
        slog.Warn("analysis failed, saving raw transcript",
            "error", err)
        return s.buildRawNote(segments), nil
    }

    return note, nil
}

// Caller (delivery layer) handles remaining cases:
func runRecord(ctx context.Context, svc *RecordService) error {
    note, err := svc.Run(ctx)
    if err != nil {
        switch {
        case errors.Is(err, ErrAudioCaptureFailed):
            return fmt.Errorf("microphone not accessible: %w", err)
        case errors.Is(err, ErrTranscriptionTimeout):
            return fmt.Errorf("transcription service timed out: %w", err)
        default:
            return fmt.Errorf("recording failed: %w", err)
        }
    }
    return writeNote(note)
}
```

### Pattern: Error Context Without Over-Wrapping

```go
// DO: Wrap at the public API boundary, not in every private helper
// internal/transcriber/deepgram.go

// Public API -- add package context
func (d *DeepgramTranscriber) Connect(ctx context.Context) error {
    if err := d.dial(ctx); err != nil {
        return fmt.Errorf("transcriber connect: %w", err) // boundary wrap
    }
    return nil
}

// Private helper -- return errors directly (no redundant wrap)
func (d *DeepgramTranscriber) dial(ctx context.Context) error {
    conn, _, err := websocket.Dial(ctx, d.url, nil)
    if err != nil {
        return err // caller adds context
    }
    d.conn = conn
    return nil
}
```

```go
// DON'T: Wrap in every private function
func (d *DeepgramTranscriber) dial(ctx context.Context) error {
    conn, _, err := websocket.Dial(ctx, d.url, nil)
    if err != nil {
        return fmt.Errorf("dial: %w", err) // unnecessary -- caller wraps too
    }
    d.conn = conn
    return nil
}
// Result: "transcriber connect: dial: dial tcp: connection refused"
//          ^^^redundant prefix chain
```

---

## Checklist

- [ ] All error returns are checked -- no `_` discards in production paths
- [ ] Error messages are lowercase, present tense, no trailing period
- [ ] `fmt.Errorf("context: %w", err)` used at package boundaries
- [ ] Sentinel errors compared with `errors.Is`, never `==`
- [ ] Custom error types use `errors.As` for extraction, not type assertions
- [ ] Custom error types implement `Unwrap() error` if they wrap another error
- [ ] Library functions never call `log.Fatal` or `os.Exit`
- [ ] Multi-field validation uses `errors.Join` to report all failures at once
- [ ] Error logging done at the top boundary only (CLI root command)
- [ ] No string matching on `err.Error()` -- use typed comparisons
- [ ] Sentinel errors are exported only when callers must branch on them
