**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Layered Architecture -- go

**Purpose**: Define the dependency direction, layer boundaries, and type translation rules for a layered Go CLI application.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Layered architecture in Go is enforced structurally through the package import system. The Go compiler rejects import cycles, which means dependency direction -- the most critical architectural constraint -- is verified at compile time rather than by convention. Outer layers (adapters, delivery) may import inner layers (use cases, domain). Inner layers must never import outer layers.

For CLI applications with a pipeline architecture, three layers are sufficient: domain types (the shared vocabulary), pipeline use cases (orchestration), and infrastructure adapters (external service implementations). The "delivery" layer is simply the Cobra command handlers in `cmd/`. A fourth formal layer adds boilerplate that dwarfs the logic in small-to-medium CLI tools.

The composition root -- `main.go` -- is the only place where concrete types from all layers meet. This is where dependency injection happens explicitly: adapters are constructed, then passed as interfaces to use cases. Go's structural typing means adapters satisfy use-case interfaces implicitly, with no `implements` keyword required.

---

## Rules

### Always
- Enforce inward-only dependency direction: outer layers import inner layers, never the reverse
- Define interfaces at the point of consumption (inner/use-case layer), not at the point of provision (adapter layer)
- Keep domain types free of external imports -- stdlib only in the domain package
- Translate types at layer boundaries: adapters convert SDK types to domain types before returning
- Wire all concrete types together only in the composition root (`main.go`)
- Translate errors at layer boundaries: inner layers return domain errors; outer layers convert to user-facing messages
- Split use-case structs by command/flow (`RecordService`, `AnalyzeService`) -- each with 5 or fewer dependencies

### Never
- Never import an adapter package from the use-case or domain layer
- Never pass SDK-specific types (Deepgram response structs, Anthropic message objects) through to inner layers
- Never use `init()` to register implementations -- wiring happens explicitly in the composition root
- Never create a god service with 10+ methods covering every feature
- Never add layers prematurely -- start flat, extract layers when a second adapter or test mock demands it

---

## Patterns

### Pattern: Dependency Direction -- Interface Defined in Inner Layer

```go
// DO: Use-case layer defines what it needs -- no import of adapters
// internal/usecase/record.go  (INNER LAYER)
package usecase

import "context"

type AudioSource interface {
    Stream() <-chan AudioFrame
    Stop() error
}

type Transcriber interface {
    Connect(ctx context.Context) error
    Send(frame AudioFrame) error
    Receive() <-chan Segment
    Close() error
}

type RecordService struct {
    audio AudioSource
    txcr  Transcriber
}

func NewRecordService(a AudioSource, t Transcriber) *RecordService {
    return &RecordService{audio: a, txcr: t}
}
```

```go
// DON'T: Inner layer imports outer adapter
// internal/usecase/record.go
import "github.com/user/app/internal/audio"       // WRONG
import "github.com/user/app/internal/transcriber"  // WRONG

type RecordService struct {
    audio *audio.MalgoSource              // coupled to concrete type
    txcr  *transcriber.DeepgramClient     // coupled to concrete type
}
```

### Pattern: Layer Boundaries with Type Translation

```go
// DO: Adapter translates between external and domain types
// internal/transcriber/deepgram.go  (OUTER -- ADAPTER LAYER)
package transcriber

import (
    "github.com/deepgram/deepgram-go-sdk/v2"
    "github.com/user/app/internal/domain"
)

type DeepgramTranscriber struct {
    client *deepgram.Client
}

func translateResult(r deepgram.TranscriptResult) domain.Segment {
    return domain.Segment{
        Speaker:    r.Channel.Alternatives[0].Words[0].Speaker,
        Text:       r.Channel.Alternatives[0].Transcript,
        StartTime:  r.Start,
        Confidence: r.Channel.Alternatives[0].Confidence,
    }
}
```

```go
// DON'T: Pass framework/adapter types to inner layers
func (s *RecordService) Process(r deepgram.TranscriptResult) {
    // Use case now coupled to Deepgram SDK -- breaks on provider swap
}
```

### Pattern: Four-Layer CLI Architecture

```
+-------------------------------------------------+
|  DELIVERY (CLI Commands / Cobra handlers)       |  <- cmd/ or internal/cli/
|  Translates args -> use case calls              |
|  Translates errors -> exit codes                |
+-------------------------------------------------+
|  APPLICATION / USE CASE                         |  <- internal/usecase/
|  Orchestrates pipeline stages                   |
|  Owns interfaces for its dependencies           |
+-------------------------------------------------+
|  DOMAIN / ENTITIES                              |  <- internal/domain/
|  AudioFrame, Segment, MeetingNote               |
|  Zero external imports                          |
+-------------------------------------------------+
|  ADAPTERS / INFRASTRUCTURE                      |  <- internal/audio/, transcriber/
|  MalgoSource, DeepgramClient, ClaudeClient      |
|  Implements use-case interfaces                 |
+-------------------------------------------------+
     (imports flow upward; data flows down)
```

```go
// DO: Composition root -- main.go wires everything
package main

import (
    "github.com/user/app/internal/audio"
    "github.com/user/app/internal/transcriber"
    "github.com/user/app/internal/analyzer"
    "github.com/user/app/internal/usecase"
)

func main() {
    src   := audio.NewMalgoSource(cfg.SampleRate, cfg.Channels)
    txcr  := transcriber.NewDeepgramClient(cfg.DeepgramKey)
    anlzr := analyzer.NewClaudeAnalyzer(cfg.ClaudeKey)

    svc := usecase.NewRecordService(src, txcr, anlzr)
    // DeepgramClient satisfies usecase.Transcriber implicitly
}
```

### Pattern: Domain Package -- Zero Imports

```go
// DO: internal/domain/types.go -- stdlib only
package domain

import "time"

type AudioFrame struct {
    Data       []byte
    SampleRate int
    Channels   int
    Timestamp  time.Time
}

type Segment struct {
    Speaker    string
    Text       string
    StartTime  float64
    EndTime    float64
    Confidence float64
}

type MeetingNote struct {
    Title       string
    Date        time.Time
    Attendees   []string
    Summary     string
    ActionItems []ActionItem
    Transcript  []Segment
}
```

```go
// DON'T: Domain types import SDK or framework types
import "github.com/deepgram/deepgram-go-sdk/v2"
type Segment struct {
    Raw deepgram.Word // domain now coupled to Deepgram SDK version
}
```

### Pattern: Error Handling at Layer Boundaries

```go
// DO: Inner layers return typed/sentinel errors
// internal/usecase/errors.go
package usecase

import "errors"

var (
    ErrAudioCaptureFailed   = errors.New("audio capture failed")
    ErrTranscriptionTimeout = errors.New("transcription timeout")
    ErrAnalysisFailed       = errors.New("analysis failed")
)
```

```go
// DO: Delivery layer translates to user-facing messages
// cmd/heimdall/commands/record.go
func runRecord(ctx context.Context, svc *usecase.RecordService) error {
    if err := svc.Run(ctx); err != nil {
        switch {
        case errors.Is(err, usecase.ErrAudioCaptureFailed):
            fmt.Fprintln(os.Stderr, "error: microphone not accessible. Check permissions.")
            return fmt.Errorf("record: %w", err)
        case errors.Is(err, usecase.ErrAnalysisFailed):
            fmt.Fprintln(os.Stderr, "warning: AI analysis failed, raw transcript saved.")
            return nil // graceful degradation
        default:
            return fmt.Errorf("record: %w", err)
        }
    }
    return nil
}
```

### Pattern: Testing via Interface Substitution

```go
// DO: Test use cases with in-memory fakes, not mocks
// internal/usecase/record_test.go
package usecase_test

type fakeAudioSource struct {
    frames []domain.AudioFrame
}

func (f *fakeAudioSource) Stream() <-chan domain.AudioFrame {
    ch := make(chan domain.AudioFrame, len(f.frames))
    for _, fr := range f.frames {
        ch <- fr
    }
    close(ch)
    return ch
}

func (f *fakeAudioSource) Stop() error { return nil }

func TestRecordService_ProcessesAllFrames(t *testing.T) {
    src  := &fakeAudioSource{frames: testFrames}
    txcr := &fakeTranscriber{}
    svc  := NewRecordService(src, txcr)

    note, err := svc.Run(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    if len(note.Transcript) == 0 {
        t.Error("expected transcript segments")
    }
}
```

### Pattern: Avoiding Premature Layering

```go
// DO: Start with direct implementation; extract layers when needed
// Early stage -- audio.go and audio_test.go in the same package
package audio

type MalgoSource struct { /* ... */ }

func NewMalgoSource(rate, ch int) *MalgoSource { /* ... */ }

// No interface yet -- only one implementation exists.
// When the Swift SystemAudioSource is added, THEN extract AudioSource interface.
```

```go
// DON'T: Create 4 layers for a 3-file project
// internal/domain/audio.go        -- just a type alias
// internal/usecase/capture.go     -- just calls the adapter
// internal/adapter/malgo.go       -- the actual code
// internal/delivery/record.go     -- just calls the use case
// Boilerplate dwarfs logic; premature architecture
```

---

## Checklist

- [ ] Domain package (`internal/domain`) has zero external (non-stdlib) imports
- [ ] Interfaces are defined in the use-case package, not the adapter packages
- [ ] No use-case or domain file imports from adapter packages
- [ ] Layer boundary translation: adapter converts SDK types to domain types before returning
- [ ] Composition root (`main.go`) is the only file where all layer packages are imported together
- [ ] Error types/sentinels defined in use-case layer; translated to user messages in delivery layer
- [ ] Each use-case struct has 5 or fewer interface dependencies
- [ ] Tests for use-case layer use in-memory fakes, not real external services
- [ ] No `init()` used to register implementations
- [ ] `go build ./...` has zero import cycle errors
- [ ] Layer boundaries verified: `grep -r "internal/audio" internal/usecase/` returns zero results
- [ ] Graceful degradation: if outer adapter fails, use case returns typed error for delivery layer fallback
