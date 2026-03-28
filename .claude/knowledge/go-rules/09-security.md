**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Security -- go

**Purpose**: Define security patterns specific to Go CLI applications -- error exposure, secrets handling, file permissions, command injection prevention, and static analysis.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Security in a CLI application differs significantly from web application security. There is no SQL injection risk (no database in the typical sense), but OS command injection risk is elevated when the CLI spawns subprocesses (e.g., a Swift audio helper via `exec.Command`). File permission discipline matters more because recovery files and transcripts may contain meeting PII. And error exposure is a distinct concern: raw internal errors returned to the terminal can leak file paths, API response details, and internal URL structures.

Two layers of security tooling complement secure coding patterns. `govulncheck ./...` scans your actual call graph (not just imports) to find known vulnerabilities in dependencies. `gosec ./...` enforces 50+ OWASP-aligned rules specific to Go code: weak crypto, hardcoded credentials, file path injection (G304), and permissive file permissions (G306). Both should run in CI on every PR.

The split-brain error pattern is the most valuable defensive coding technique for CLIs: separate user-facing messages from internal errors at the type level. A `SafeError` struct carries both a generic user message and the full internal error. The delivery layer (Cobra command handler) logs the internal error and returns only the safe message to the user. This prevents accidental information disclosure while preserving full diagnostics in logs.

---

## Rules

### Always
- Separate user-facing messages from internal errors -- never expose raw errors to terminal output
- Run `govulncheck ./...` in CI on every PR to detect dependency vulnerabilities
- Run `gosec ./...` as a lint step with findings triaged against CWE identifiers
- Run `go test -race ./...` in CI -- race conditions are security vulnerabilities (TOCTOU, data corruption)
- Load secrets from environment variables via `os.Getenv` or Viper's `AutomaticEnv`
- Write files atomically: write to temp file, then `os.Rename()` to final path
- Set restrictive file permissions: `0600` for sensitive files, `0700` for sensitive directories
- Validate all external input at boundaries (Deepgram segments, CLI flags, config values)
- Use `exec.Command(binary, arg1, arg2)` with explicit argument lists for subprocess execution
- Use `crypto/rand` for any nonce, token, or unique ID generation
- Use `html/template` instead of `text/template` if output may render in a browser or web view

### Never
- Never log secrets (API keys, tokens, passwords) at any log level, including debug
- Never store API keys in config files committed to version control
- Never use `exec.Command("sh", "-c", userInput)` -- OS command injection vulnerability
- Never use `math/rand` for security-sensitive random values (session IDs, nonces, file names)
- Never suppress `gosec` findings with blanket `//nolint:gosec` -- suppress only with specific rule ID and justification
- Never use `0644` or `0666` permissions on files containing meeting transcripts or API responses
- Never call `log.Fatal` in library code -- it terminates the entire process without cleanup

---

## Patterns

### Pattern: Safe Error Exposure (Split-Brain Pattern)

```go
// DO: Separate user-facing message from internal error
type SafeError struct {
    Code     string // machine-readable, for callers
    UserMsg  string // safe, generic, shown to user
    Internal error  // never exposed externally
}

func (e *SafeError) Error() string { return e.UserMsg }
func (e *SafeError) Unwrap() error { return e.Internal }

func runRecord(cmd *cobra.Command, args []string) error {
    if err := doRecord(cmd.Context()); err != nil {
        var safeErr *SafeError
        if errors.As(err, &safeErr) {
            slog.Error("record failed",
                "code", safeErr.Code,
                "internal", safeErr.Internal)
            return fmt.Errorf("%s", safeErr.UserMsg)
        }
        slog.Error("unexpected error", "err", err)
        return errors.New("an unexpected error occurred")
    }
    return nil
}
```

```go
// DON'T: Propagate raw errors to the user
func runRecord(cmd *cobra.Command, args []string) error {
    if err := doRecord(cmd.Context()); err != nil {
        return err // may contain DB query, file paths, API response details
    }
    return nil
}
```

### Pattern: Safe OS Command Execution

```go
// DO: Pass arguments as separate strings to exec.Command
func runSwiftHelper(ctx context.Context, binaryPath string, args []string) (*exec.Cmd, error) {
    cmd := exec.CommandContext(ctx, binaryPath, args...)
    cmd.Stdout = audioOut
    return cmd, nil
}
```

```go
// DON'T: Construct shell commands with user/external input
func runSwiftHelper(flags string) (*exec.Cmd, error) {
    cmd := exec.Command("sh", "-c", "/usr/local/bin/heimdall-audio "+flags)
    // G204: Command constructed from user-controlled variable
    return cmd, nil
}
```

### Pattern: Atomic File Write

```go
// DO: Write recovery files atomically with restrictive permissions
func writeRecoveryFile(path string, data []byte) error {
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0700); err != nil {
        return fmt.Errorf("write recovery mkdir: %w", err)
    }

    tmp, err := os.CreateTemp(dir, ".tmp-*")
    if err != nil {
        return fmt.Errorf("write recovery temp: %w", err)
    }
    tmpPath := tmp.Name()

    if _, err := tmp.Write(data); err != nil {
        tmp.Close()
        os.Remove(tmpPath)
        return fmt.Errorf("write recovery write: %w", err)
    }
    if err := tmp.Close(); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("write recovery close: %w", err)
    }
    if err := os.Chmod(tmpPath, 0600); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("write recovery chmod: %w", err)
    }
    if err := os.Rename(tmpPath, path); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("write recovery rename: %w", err)
    }
    return nil
}
```

```go
// DON'T: Write directly with permissive permissions
func writeRecoveryFileBad(path string, data []byte) error {
    return os.WriteFile(path, data, 0644)
    // Wrong permission + non-atomic -- partial write on crash corrupts the file
}
```

### Pattern: Secret Loading via Environment

```go
// DO: Load secrets from environment, validate presence at startup
func loadSecrets() (*Secrets, error) {
    dgKey := os.Getenv("HEIMDALL_DEEPGRAM_API_KEY")
    if dgKey == "" {
        return nil, errors.New("HEIMDALL_DEEPGRAM_API_KEY is required")
    }
    claudeKey := os.Getenv("HEIMDALL_CLAUDE_API_KEY")
    if claudeKey == "" {
        return nil, errors.New("HEIMDALL_CLAUDE_API_KEY is required")
    }
    return &Secrets{DeepgramKey: dgKey, ClaudeKey: claudeKey}, nil
}
```

```go
// DON'T: Hardcode secrets or log them
const deepgramKey = "dg_live_abc123" // G101: hardcoded credential
slog.Debug("connecting to Deepgram", "key", cfg.DeepgramKey) // credential leak
```

### Pattern: Input Validation at Boundaries

```go
// DO: Validate external data at the point it enters your system
const (
    maxSegmentLength = 10000
    maxSpeakers      = 20
)

func parseSegment(raw json.RawMessage) (*Segment, error) {
    var seg Segment
    if err := json.Unmarshal(raw, &seg); err != nil {
        return nil, fmt.Errorf("parse segment: %w", err)
    }
    if len(seg.Text) > maxSegmentLength {
        return nil, fmt.Errorf("segment text exceeds max length %d", maxSegmentLength)
    }
    if seg.Speaker < 0 || seg.Speaker > maxSpeakers {
        return nil, fmt.Errorf("invalid speaker ID %d", seg.Speaker)
    }
    return &seg, nil
}
```

```go
// DON'T: Trust external provider data without validation
func parseSegmentBad(raw json.RawMessage) (*Segment, error) {
    var seg Segment
    json.Unmarshal(raw, &seg) // ignoring error, no validation
    return &seg, nil
}
```

### Pattern: Govulncheck in CI

```yaml
# DO: Use govulncheck in GitHub Actions
- name: Run govulncheck
  uses: golang/govulncheck-action@v1
  with:
    go-version-input: stable
    go-package: ./...
```

### Pattern: Sensitive Type Redaction

```go
// DO: Implement slog.LogValuer to prevent credential leakage in logs
type APIKey string

func (k APIKey) LogValue() slog.Value {
    if len(k) < 8 {
        return slog.StringValue("REDACTED")
    }
    return slog.StringValue(string(k[:4]) + "****")
}

// Usage:
slog.Info("connected", slog.Any("api_key", APIKey(cfg.DeepgramKey)))
// Output: connected api_key=dg12****
```

### Pattern: Subprocess Health Monitoring

```go
// DO: Monitor subprocess with a watchdog for crash detection
func monitorSubprocess(ctx context.Context, cmd *exec.Cmd, onCrash func(error)) {
    go func() {
        err := cmd.Wait()
        if err != nil && ctx.Err() == nil {
            // Process exited unexpectedly while context is still active
            onCrash(fmt.Errorf("subprocess crashed: %w", err))
        }
    }()
}

// Graceful shutdown: send stop signal, wait, then SIGKILL as last resort
func stopSubprocess(cmd *exec.Cmd) error {
    // Try graceful first via stdin "stop" command
    if cmd.Process != nil {
        cmd.Process.Signal(os.Interrupt)

        done := make(chan error, 1)
        go func() { done <- cmd.Wait() }()

        select {
        case err := <-done:
            return err
        case <-time.After(5 * time.Second):
            return cmd.Process.Kill() // last resort after 5s
        }
    }
    return nil
}
```

```go
// DON'T: Fire and forget subprocesses
cmd := exec.Command("/usr/local/bin/heimdall-audio")
cmd.Start()
// No crash detection, no cleanup, no graceful shutdown
```

### Pattern: File Path Validation

```go
// DO: Validate file paths against an allowlist or expected patterns
func validateOutputPath(path string) error {
    abs, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }

    // Ensure path is within expected directories
    home, _ := os.UserHomeDir()
    allowed := []string{
        filepath.Join(home, "notes"),
        filepath.Join(home, "Documents"),
        filepath.Join(home, ".heimdall"),
    }

    for _, prefix := range allowed {
        if strings.HasPrefix(abs, prefix) {
            return nil
        }
    }
    return fmt.Errorf("output path %q is outside allowed directories", abs)
}
```

---

## Checklist

- [ ] `govulncheck ./...` runs in CI on every PR
- [ ] `gosec ./...` runs as a lint step with findings triaged
- [ ] `go test -race ./...` runs in CI -- no race conditions tolerated
- [ ] All secrets loaded from env vars (never hardcoded or in config files committed to VCS)
- [ ] No secrets logged at any log level
- [ ] Recovery files written atomically (temp file then rename), permissions `0600`
- [ ] Recovery directory created with `0700` permissions
- [ ] All `exec.Command` calls use explicit arg lists, never `sh -c` with interpolated input
- [ ] External data (Deepgram segments, Claude responses) validated at boundary before use
- [ ] Raw errors never propagated to user-facing output; SafeError pattern or equivalent used
- [ ] Structured logging used (`log/slog`); no `fmt.Printf` in library code
- [ ] `crypto/rand` used for all nonce/ID generation; `math/rand` only for non-security random
- [ ] `go vet ./...` passes with zero warnings
- [ ] Go dependencies pinned in `go.sum`; `go mod tidy` checked in CI to detect drift
