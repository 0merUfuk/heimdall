**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Testing -- go

**Purpose**: Define testing patterns, conventions, and tooling for idiomatic Go test suites.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Go's testing philosophy is built into the language and toolchain. Test files end in `_test.go`, live alongside the code they test, and are automatically excluded from production builds. The `go test` command discovers and runs them with no configuration. Test functions follow the signature `func TestXxx(t *testing.T)` where `Xxx` starts with a capital letter. This convention-over-configuration approach means there is no test runner to install, no configuration file to maintain, and no test discovery mechanism to debug.

Table-driven tests are the idiomatic Go pattern for functions with multiple test cases. A slice of structs defines inputs and expected outputs, and `t.Run` creates a named subtest for each case. This pattern scales from 2 cases to 200 cases without structural changes, enables selective execution (`go test -run TestFoo/case_name`), and produces clear, filterable output. Since Go 1.22, loop variable semantics changed so that each iteration creates a new variable -- eliminating the long-standing `tt := tt` capture bug in parallel subtests.

The race detector (`go test -race ./...`) is non-negotiable for any concurrent code. It detects data races at test time using a happens-before analysis. For an audio pipeline application with multiple goroutines sharing channels and buffers, the race detector is the primary tool for catching concurrency bugs. CI must run with `-race` on every PR -- no exceptions.

---

## Rules

### Always
- Name test files `*_test.go` in the same package as the code under test
- Use table-driven tests with `t.Run` for any function with more than 2 test cases
- Give every table entry a `name` field; use it in `t.Run(tt.name, ...)`
- Use `require.NoError` / `require.NotNil` (testify) before dereferencing results -- stops execution on failure
- Mark test helpers with `t.Helper()` so error line numbers point to the caller
- Use `t.TempDir()` for filesystem operations in tests -- automatically cleaned up
- Run all tests with `-race` flag: `go test -race ./...`
- Use `t.Parallel()` on both outer test function and subtests for independent cases
- Test interfaces, not implementations -- inject mock/fake implementations through interfaces
- Default to black-box testing (`package foo_test`) for public API tests

### Never
- Never use `init()` in test files -- creates hidden state that breaks test isolation
- Never call `os.Exit` in test helpers -- bypasses `t.Cleanup` callbacks; use `t.Fatal` instead
- Never test implementation details (private fields, unexported functions) -- test exported behavior only
- Never hardcode `/tmp/` paths -- use `t.TempDir()` which is isolated per test
- Never reach into real external services in unit tests -- inject mocks via interfaces
- Never use `assert` (testify) for critical preconditions -- use `require` which stops execution

---

## Patterns

### Pattern: Table-Driven Tests with t.Run

```go
// DO: Slice of structs with named cases and t.Run subtests
func TestReverseRunes(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {"empty", "", ""},
        {"ascii", "hello", "olleh"},
        {"unicode", "Hello, world", "dlrow ,olleH"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := ReverseRunes(tt.input)
            if got != tt.want {
                t.Errorf("ReverseRunes(%q) = %q; want %q", tt.input, got, tt.want)
            }
        })
    }
}
```

```go
// DON'T: Separate test functions for each case
func TestReverseRunesEmpty(t *testing.T)   { /* ... */ }
func TestReverseRunesASCII(t *testing.T)   { /* ... */ }
func TestReverseRunesUnicode(t *testing.T) { /* ... */ }
// No scalability, no shared structure, clutters test output
```

### Pattern: Parallel Subtests (Go 1.22+)

```go
// DO: t.Parallel() on both outer and inner
// Go 1.22+ creates a new loop variable per iteration -- no capture needed
func TestProcess(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name  string
        input int
        want  int
    }{
        {"double_three", 3, 6},
        {"double_zero", 0, 0},
        {"double_negative", -5, -10},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got := double(tt.input)
            if got != tt.want {
                t.Errorf("double(%d) = %d; want %d", tt.input, got, tt.want)
            }
        })
    }
}
```

```go
// DON'T (Go < 1.22): Forget tt := tt capture -- all parallel subtests share same tt
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // tt is the LAST value of the loop -- race condition
        got := double(tt.input) // wrong input
    })
}
```

### Pattern: Interface Mocking for Dependency Injection

```go
// DO: Define interface, inject fake in tests
type Transcriber interface {
    Transcribe(ctx context.Context, audio []byte) (string, error)
}

type mockTranscriber struct {
    result string
    err    error
}

func (m *mockTranscriber) Transcribe(_ context.Context, _ []byte) (string, error) {
    return m.result, m.err
}

func TestAnalyzer_Summarize(t *testing.T) {
    mock := &mockTranscriber{result: "hello world"}
    a := NewAnalyzer(mock)
    note, err := a.Summarize(context.Background())
    require.NoError(t, err)
    assert.Contains(t, note.Summary, "hello")
}
```

```go
// DON'T: Reach into real external services in unit tests
func TestAnalyzer_Summarize(t *testing.T) {
    a := NewAnalyzer(deepgram.NewClient(os.Getenv("API_KEY")))
    // Flaky, slow, costs money, requires network
}
```

### Pattern: Testify require vs assert

```go
// DO: Use require when subsequent steps depend on the result
func TestLoadConfig(t *testing.T) {
    cfg, err := LoadConfig("testdata/valid.yaml")
    require.NoError(t, err)    // stops test if config fails to load
    require.NotNil(t, cfg)     // stops test if cfg is nil

    assert.Equal(t, "deepgram", cfg.TranscriberType) // non-fatal; report and continue
    assert.Equal(t, 16000, cfg.SampleRate)
}
```

```go
// DON'T: Use assert for critical preconditions
func TestLoadConfig(t *testing.T) {
    cfg, err := LoadConfig("testdata/valid.yaml")
    assert.NoError(t, err)
    // If err != nil, cfg is nil -- next line panics with unhelpful output
    assert.Equal(t, "deepgram", cfg.TranscriberType)
}
```

### Pattern: t.Helper for Test Utilities

```go
// DO: Mark helper functions so error lines point to the caller
func assertSegmentsEqual(t *testing.T, got, want []Segment) {
    t.Helper()
    if len(got) != len(want) {
        t.Fatalf("segment count: got %d, want %d", len(got), len(want))
    }
    for i := range want {
        if got[i].Text != want[i].Text {
            t.Errorf("segment[%d].Text: got %q, want %q", i, got[i].Text, want[i].Text)
        }
    }
}

func TestTranscription(t *testing.T) {
    got := transcribe(testAudio)
    assertSegmentsEqual(t, got, expectedSegments) // error points HERE, not inside helper
}
```

### Pattern: Filesystem Tests with t.TempDir

```go
// DO: Use t.TempDir() for ephemeral filesystem state
func TestObsidianWriter(t *testing.T) {
    dir := t.TempDir()
    w := NewObsidianWriter(dir)
    note := &MeetingNote{Title: "Weekly Sync", Summary: "Discussed roadmap"}

    err := w.Write(note)
    require.NoError(t, err)

    content, err := os.ReadFile(filepath.Join(dir, "Weekly Sync.md"))
    require.NoError(t, err)
    assert.Contains(t, string(content), "Weekly Sync")
}
```

```go
// DON'T: Write to real paths or shared temp locations
func TestObsidianWriter(t *testing.T) {
    w := NewObsidianWriter("/tmp/test-notes")
    // Shared across parallel tests; not cleaned up automatically
}
```

### Pattern: Build Tags for Integration Tests

```go
// DO: Separate integration tests with build tags
// internal/transcriber/deepgram_integration_test.go
//go:build integration

package transcriber_test

import (
    "context"
    "os"
    "testing"

    "github.com/user/app/internal/transcriber"
)

func TestDeepgramTranscriber_RealConnection(t *testing.T) {
    apiKey := os.Getenv("DEEPGRAM_API_KEY")
    if apiKey == "" {
        t.Skip("DEEPGRAM_API_KEY not set")
    }

    client := transcriber.NewDeepgramClient(apiKey)
    err := client.Connect(context.Background())
    if err != nil {
        t.Fatalf("connect failed: %v", err)
    }
    defer client.Close()

    // Send test audio, verify transcription
}

// Run with: go test -tags integration ./...
// CI: only in dedicated integration test stage, not on every PR
```

```go
// DON'T: Mix integration and unit tests without build tags
func TestDeepgramTranscriber_RealConnection(t *testing.T) {
    // Runs on every `go test ./...` -- flaky, slow, requires credentials
    client := transcriber.NewDeepgramClient(os.Getenv("DEEPGRAM_API_KEY"))
    // ...
}
```

### Pattern: Error Path Testing with Injected Failures

```go
// DO: Test error paths by injecting failures through interfaces
func TestRecordService_TranscriberFailure(t *testing.T) {
    tests := []struct {
        name    string
        txErr   error
        wantErr bool
    }{
        {"transcriber_connect_fails", errors.New("connection refused"), true},
        {"transcriber_send_fails", errors.New("write: broken pipe"), true},
        {"transcriber_works", nil, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            svc := NewRecordService(RecordServiceParams{
                Audio:       &fakeAudioSource{frames: testFrames},
                Transcriber: &fakeTranscriber{connectErr: tt.txErr},
                Output:      &fakeOutput{},
            })

            err := svc.Run(context.Background())
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

```go
// DON'T: Only test the happy path -- error paths are where bugs hide
func TestRecordService_Works(t *testing.T) {
    svc := NewRecordService(/* all happy fakes */)
    require.NoError(t, svc.Run(context.Background()))
    // What happens when the transcriber fails? Untested.
}
```

### Pattern: Golden File Testing for Template Output

```go
// DO: Compare generated output against a golden file
func TestObsidianRenderer_GoldenFile(t *testing.T) {
    note := &MeetingNote{
        Title:   "Weekly Sync",
        Summary: "Discussed Q2 roadmap",
        Transcript: []Segment{
            {Speaker: "Alice", Text: "Let's review the roadmap"},
        },
    }

    got, err := renderNote(note)
    require.NoError(t, err)

    golden := filepath.Join("testdata", "weekly_sync.golden.md")
    if os.Getenv("UPDATE_GOLDEN") != "" {
        os.WriteFile(golden, got, 0644)
    }

    want, err := os.ReadFile(golden)
    require.NoError(t, err)
    assert.Equal(t, string(want), string(got))
}

// Update golden files: UPDATE_GOLDEN=1 go test ./...
```

---

## Checklist

- [ ] All test files end in `_test.go` and are in the correct package
- [ ] Test functions named `TestXxx(t *testing.T)` with capital X
- [ ] Table-driven tests used for any function with more than 2 cases
- [ ] Each table entry has a `name` field; tests use `t.Run(tt.name, ...)`
- [ ] `require.NoError` / `require.NotNil` used before dereferencing results
- [ ] Mock implementations defined via interfaces, not concrete types
- [ ] `t.Helper()` called in all assertion helper functions
- [ ] `t.TempDir()` used for filesystem tests -- no hardcoded `/tmp/` paths
- [ ] Tests run cleanly with `go test -race ./...`
- [ ] CI runs `go test -race ./...` on every PR
- [ ] Coverage generated with `go test -coverprofile=coverage.out ./...`
- [ ] Black-box testing (`package foo_test`) used by default for public API tests
- [ ] Integration tests separated with `//go:build integration` build tag
