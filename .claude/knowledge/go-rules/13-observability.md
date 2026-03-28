**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Observability -- go

**Purpose**: Define structured logging, tracing, and diagnostic patterns for Go CLI applications.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Observability in a CLI application is primarily about structured logging. Unlike long-running servers where metrics and distributed tracing are essential, a CLI process runs for a bounded duration (a single meeting recording session) and exits. The primary observability tool is `log/slog` (Go 1.21+), which provides structured, leveled logging as part of the standard library. It replaces `fmt.Println`, `log.Printf`, and most third-party loggers.

`slog` outputs key-value pairs in either JSON (for machine consumption in CI or log aggregation) or text (for human-readable terminal output). The handler is selected once at startup based on environment configuration. All code uses the same `slog.Info(...)`, `slog.Error(...)` API regardless of the output format. For hot paths in the audio pipeline, `LogAttrs()` avoids key-value pair parsing and reduces allocations.

OpenTelemetry tracing is valuable for pipeline stage visibility during development and debugging -- instrumenting the capture, transcribe, analyze, and render stages with spans shows exactly where time is spent. However, the OTel SDK batches spans in memory and does not flush automatically on exit. For a CLI that exits after a single run, `defer otelShutdown(ctx)` in `main()` is critical -- without it, the final batch of spans is lost, which is exactly when you need them most. Prometheus metrics are not a primary concern for short-lived CLI processes.

---

## Rules

### Always
- Use `log/slog` for all structured logging -- replace `fmt.Println`, `log.Printf`, and third-party loggers
- Use `*Context()` methods (`slog.InfoContext(ctx, ...)`) to enable log-trace correlation
- Use `JSONHandler` in production/CI, `TextHandler` in development -- switch via env var
- Use `LogAttrs()` for hot paths to avoid allocation from key-value pair parsing
- Set the default logger once at program startup with `slog.SetDefault(logger)`
- Propagate loggers via `logger.With()` -- attach session/request IDs early and pass the enriched logger
- Implement `LogValuer` on any type containing credentials, tokens, or PII
- Initialize OTel with a shutdown function and `defer otelShutdown(ctx)` in `main()`
- Instrument each pipeline stage with an OTel span (`tracer.Start` / `defer span.End()`)
- Record errors on spans with `span.RecordError(err)` and `span.SetStatus(codes.Error, ...)`
- Use `slog.LevelVar` for dynamic log level changes without restart

### Never
- Never mix `fmt.Println`/`log.Printf` with `slog` -- produces inconsistent log formats
- Never create a new logger per function call -- use `logger.With()` to derive child loggers
- Never skip `defer otelShutdown(ctx)` -- spans are buffered and will be lost on CLI exit
- Never use spans as a substitute for logs -- spans describe operation lifecycle; logs provide diagnostic detail
- Never hardcode the log level -- use `LevelVar` so it can be changed at runtime
- Never log at DEBUG in production by default -- ship at INFO; promote to DEBUG via config

---

## Patterns

### Pattern: Logger Initialization and Default Setup

```go
// DO: Initialize logger once in main, set as default
func initLogger(env, level string) *slog.Logger {
    var h slog.Handler
    lvl := new(slog.LevelVar)
    lvl.Set(parseLevel(level))

    opts := &slog.HandlerOptions{
        Level:     lvl,
        AddSource: env == "development",
    }
    if env == "production" {
        h = slog.NewJSONHandler(os.Stderr, opts)
    } else {
        h = slog.NewTextHandler(os.Stderr, opts)
    }
    logger := slog.New(h)
    slog.SetDefault(logger)
    return logger
}

func parseLevel(s string) slog.Level {
    switch strings.ToLower(s) {
    case "debug":
        return slog.LevelDebug
    case "warn":
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

```go
// DON'T: Use fmt.Println or log.Printf for structured output
fmt.Println("starting server port=8080") // unstructured, unleveled
log.Printf("error: %v", err)            // no structured fields
```

### Pattern: Context-Aware Logging with Correlation IDs

```go
// DO: Attach correlation IDs to logger, propagate via function arguments
func handleRecording(ctx context.Context, logger *slog.Logger, sessionID string) error {
    log := logger.With(
        slog.String("session_id", sessionID),
        slog.String("component", "recorder"),
    )
    log.InfoContext(ctx, "recording started")

    if err := capture(ctx); err != nil {
        log.ErrorContext(ctx, "capture failed",
            slog.String("error", err.Error()))
        return err
    }
    log.InfoContext(ctx, "recording stopped",
        slog.Duration("duration", elapsed))
    return nil
}
```

```go
// DON'T: Re-attach fields at every call site
func handleRecordingBad(ctx context.Context, logger *slog.Logger, sessionID string) error {
    logger.Info("recording started", "session_id", sessionID)
    if err := capture(ctx); err != nil {
        logger.Error("capture failed", "session_id", sessionID, "error", err)
        return err
    }
    logger.Info("recording stopped", "session_id", sessionID)
    return nil
}
```

### Pattern: Sensitive Type Redaction via LogValuer

```go
// DO: Implement LogValuer to redact secrets in logs
type APIKey string

func (k APIKey) LogValue() slog.Value {
    if len(k) < 8 {
        return slog.StringValue("REDACTED")
    }
    return slog.StringValue(string(k[:4]) + "****")
}

// Usage:
slog.Info("deepgram connected", slog.Any("api_key", APIKey(cfg.DeepgramKey)))
// Output: deepgram connected api_key=dg12****
```

```go
// DON'T: Log raw secrets or credentials
slog.Info("deepgram connected", "api_key", cfg.DeepgramKey)
// Output: deepgram connected api_key=dg_live_abc123def456  -- full key in logs
```

### Pattern: OpenTelemetry SDK Initialization

```go
// DO: Initialize OTel with proper shutdown in main()
func setupOTel(ctx context.Context, serviceName string) (func(context.Context) error, error) {
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    traceExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
    if err != nil {
        return nil, fmt.Errorf("otel trace exporter: %w", err)
    }

    res := resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceName(serviceName),
    )
    tp := trace.NewTracerProvider(
        trace.WithBatcher(traceExp),
        trace.WithResource(res),
    )
    otel.SetTracerProvider(tp)

    return tp.Shutdown, nil
}

// In main():
otelShutdown, err := setupOTel(ctx, "heimdall")
if err != nil {
    slog.Error("otel init failed", "error", err)
} else {
    defer otelShutdown(context.Background())
}
```

```go
// DON'T: Skip shutdown -- spans are buffered and will be lost
tp := trace.NewTracerProvider(/* ... */)
otel.SetTracerProvider(tp)
// no defer tp.Shutdown() -- spans lost on CLI exit
```

### Pattern: Instrumenting a Pipeline Stage with Spans

```go
// DO: Wrap each pipeline stage in a span with semantic attributes
var tracer = otel.Tracer("heimdall/transcriber")

func (d *DeepgramTranscriber) Connect(ctx context.Context, opts TranscribeOpts) error {
    ctx, span := tracer.Start(ctx, "transcriber.connect",
        oteltrace.WithSpanKind(oteltrace.SpanKindClient),
        oteltrace.WithAttributes(
            attribute.String("provider", "deepgram"),
            attribute.String("model", opts.Model),
        ),
    )
    defer span.End()

    if err := d.dial(ctx, opts); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return fmt.Errorf("transcriber connect: %w", err)
    }
    return nil
}
```

```go
// DON'T: Swallow errors without recording them on the span
ctx, span := tracer.Start(ctx, "transcriber.connect")
defer span.End()
if err := d.dial(ctx, opts); err != nil {
    return err // span shows OK even though it failed
}
```

### Pattern: Dynamic Log Level with LevelVar

```go
// DO: Use LevelVar so log level can be changed at runtime
var logLevel = new(slog.LevelVar) // defaults to Info

func initDynamicLogger() {
    h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
    slog.SetDefault(slog.New(h))
}

// Enable debug logs at runtime (e.g., on SIGUSR1 or --verbose flag)
func enableDebug() { logLevel.Set(slog.LevelDebug) }
```

```go
// DON'T: Hardcode the level -- requires restart to change
h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
// Cannot change without restarting the process
```

### Pattern: Pipeline Stage Timing

```go
// DO: Log timing for each pipeline stage to identify bottlenecks
func (s *RecordService) Run(ctx context.Context) error {
    stages := []struct {
        name string
        fn   func(context.Context) error
    }{
        {"capture", s.capture},
        {"transcribe", s.transcribe},
        {"analyze", s.analyze},
        {"render", s.render},
    }

    for _, stage := range stages {
        start := time.Now()
        if err := stage.fn(ctx); err != nil {
            slog.ErrorContext(ctx, "stage failed",
                slog.String("stage", stage.name),
                slog.Duration("elapsed", time.Since(start)),
                slog.String("error", err.Error()))
            return fmt.Errorf("stage %s: %w", stage.name, err)
        }
        slog.InfoContext(ctx, "stage complete",
            slog.String("stage", stage.name),
            slog.Duration("elapsed", time.Since(start)))
    }
    return nil
}
```

### Pattern: Health Check Logging for Long-Running Operations

```go
// DO: Periodic health log during recording to confirm liveness
func logHealth(ctx context.Context, logger *slog.Logger, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    var frameCount int64
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            logger.InfoContext(ctx, "recording health",
                slog.Int64("frames_processed", atomic.LoadInt64(&frameCount)),
                slog.Duration("uptime", time.Since(startTime)))
        }
    }
}
```

---

## Checklist

- [ ] `log/slog` used exclusively -- no `fmt.Println` or `log.Printf` for operational output
- [ ] Logger initialized once in `main()`, set as default with `slog.SetDefault()`
- [ ] `JSONHandler` in production, `TextHandler` in development (env-driven)
- [ ] All pipeline stage entry points use `*Context()` logging methods
- [ ] Session/request IDs attached via `logger.With()` at the entry point, not repeated per call
- [ ] Sensitive types implement `LogValuer` to prevent credential leakage
- [ ] OTel SDK initialized with `defer otelShutdown(ctx)` in `main()`
- [ ] Each pipeline stage wrapped in a span (`tracer.Start` / `defer span.End()`)
- [ ] Errors recorded on spans with `span.RecordError(err)` and `span.SetStatus(codes.Error, ...)`
- [ ] Log level is dynamic via `slog.LevelVar` -- no hardcoded level in handler options
- [ ] DEBUG logging disabled by default; enabled via config or --verbose flag
- [ ] OTel logs signal: use `slog` + `otelslog` bridge (OTel logs API is still Beta in Go)
