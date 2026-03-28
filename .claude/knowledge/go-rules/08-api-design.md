**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# API Design -- go

**Purpose**: Define internal Go API design patterns for CLI tools -- interfaces, command structure, and function signatures.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

API design in Go CLI applications is about the internal interfaces between packages, not HTTP REST endpoints. The primary APIs are: the interfaces that define provider contracts (AudioSource, Transcriber, Analyzer, OutputWriter), the Cobra command structure that defines the user-facing CLI, and the constructor/function signatures that wire everything together. Each of these surfaces has distinct design rules.

The most important Go API principle is "accept interfaces, return structs." Function parameters should accept interfaces for flexibility and testability; return values should be concrete structs so callers have full access without type assertions. This principle, combined with Go's structural typing, means a concrete type satisfies an interface by having the right methods -- no explicit declaration required. The compiler verifies the contract at build time.

Interfaces should be defined at the consumer, not the producer. The package that uses an interface should define it, containing only the methods it needs. This prevents premature abstraction, avoids circular dependencies, and follows Go's structural typing naturally. A single-implementation interface is premature abstraction -- define the concrete struct first; extract the interface when a second implementation or test mock is needed.

---

## Rules

### Always
- Accept interfaces, return concrete structs -- parameters consume behavior; return values expose concrete capabilities
- Keep interfaces small: single-method interfaces use `-er` suffix (`Reader`, `Transcriber`); multi-method use noun names (`AudioSource`)
- Define interfaces at the consumer, not the producer -- the consuming package owns the interface definition
- Use `cobra.Command` for all CLI command routing -- never manual `os.Args` parsing or flat `if/switch`
- Use `PersistentPreRunE` on the root command for config loading, permission checks, and cross-cutting validation
- Make `RunE` functions delegate to named functions on injected structs -- not inline anonymous closures
- Prefer `RunE` over `Run` -- it composes with error handling infrastructure
- Provide `Short` and `Long` descriptions and `Example` field on all commands

### Never
- Never define fat interfaces (6+ methods) -- consumers must satisfy the entire interface even if they need 1 method
- Never place interfaces in the producer package -- forces all consumers to import it; defeats Go's structural typing
- Never extract an interface until you have 2+ implementations (including test mocks)
- Never return `interface{}` / `any` from functions -- defeats type safety; use typed results or generics
- Never put business logic in `RunE` handlers -- parse flags, build service, delegate
- Never call `os.Exit` inside library or command handler functions -- return errors; let `main()` handle exit codes

---

## Patterns

### Pattern: Accept Interface / Return Struct

```go
// DO: Accept io.Reader so callers can pass files, buffers, network streams
func NewTranscriber(source io.Reader, opts TranscribeOpts) *DeepgramTranscriber {
    return &DeepgramTranscriber{source: source, opts: opts}
}
```

```go
// DON'T: Accept a concrete type -- locks callers to one implementation
func NewTranscriber(source *os.File, opts TranscribeOpts) *DeepgramTranscriber {
    return &DeepgramTranscriber{source: source}
}
```

### Pattern: Small Interface at the Consumer

```go
// DO: Define the interface in the package that uses it, with only what it needs
// internal/usecase/record.go
package usecase

type Summarizer interface {
    Summarize(ctx context.Context, segments []Segment, opts AnalyzeOpts) (*MeetingNote, error)
}

// internal/analyzer/claude.go -- implements Summarizer without knowing the interface
type ClaudeAnalyzer struct {
    client *anthropic.Client
}

func (c *ClaudeAnalyzer) Summarize(ctx context.Context, segments []Segment, opts AnalyzeOpts) (*MeetingNote, error) {
    // Implementation -- satisfies usecase.Summarizer implicitly
    return &MeetingNote{}, nil
}
```

```go
// DON'T: Define a fat interface in the producer package
// internal/claude/client.go
type Client interface {
    Summarize(...) (*MeetingNote, error)
    ListModels(...) ([]string, error)   // Not needed by consumers
    CountTokens(...) (int, error)       // Not needed by consumers
}
```

### Pattern: Cobra Command Wiring

```go
// DO: Hierarchical command structure with RunE and named functions
var rootCmd = &cobra.Command{
    Use:   "heimdall",
    Short: "CLI meeting companion",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return initConfig(cmd)
    },
}

var recordCmd = &cobra.Command{
    Use:     "record",
    Short:   "Record and transcribe a meeting",
    Long:    "Start recording audio from microphone and system, transcribe in real-time, and analyze on completion.",
    Example: "  heimdall record\n  heimdall record --output-dir ~/notes",
    RunE:    runRecord, // named function, not anonymous -- easier to test
}

func init() {
    rootCmd.AddCommand(recordCmd)
    recordCmd.Flags().StringP("output", "o", "", "Output vault path")
}
```

```go
// DON'T: Parse os.Args manually
func main() {
    if os.Args[1] == "record" { /* ... */ }
    if os.Args[1] == "analyze" { /* ... */ }
    // No help, no flag parsing, no error handling
}
```

### Pattern: Interface Mockability for Tests

```go
// DO: Inject interface dependencies so tests can mock them
type Recorder struct {
    audio       AudioSource
    transcriber Transcriber
    analyzer    Analyzer
}

func NewRecorder(a AudioSource, t Transcriber, an Analyzer) *Recorder {
    return &Recorder{audio: a, transcriber: t, analyzer: an}
}

// In tests:
type mockAudio struct{}

func (m *mockAudio) Start(ctx context.Context) error     { return nil }
func (m *mockAudio) Stream() <-chan AudioFrame            { ch := make(chan AudioFrame); close(ch); return ch }
func (m *mockAudio) Stop() error                          { return nil }
func (m *mockAudio) SampleRate() int                      { return 16000 }
func (m *mockAudio) Channels() int                        { return 1 }
```

```go
// DON'T: Hardcode implementations -- untestable
type Recorder struct {
    audio *malgo.Device // concrete type, can't mock
}
```

### Pattern: Viper + Cobra Config Binding

```go
// DO: Bind flags to Viper so env vars override config, flags override env vars
func init() {
    recordCmd.Flags().String("deepgram-key", "", "Deepgram API key")
    _ = viper.BindPFlag("deepgram.api_key", recordCmd.Flags().Lookup("deepgram-key"))
    viper.SetEnvPrefix("HEIMDALL")
    viper.AutomaticEnv()
}
```

```go
// DON'T: Read os.Getenv directly in command functions
func runRecord(cmd *cobra.Command, args []string) error {
    key := os.Getenv("DEEPGRAM_KEY") // no priority ordering, no validation
}
```

### Pattern: Exit Code Handling in main

```go
// DO: Translate errors to exit codes in main only
func main() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

```go
// DON'T: Call os.Exit in library or command handler functions
func runRecord(cmd *cobra.Command, args []string) error {
    if err := doSomething(); err != nil {
        os.Exit(1) // bypasses defer cleanup, untestable
    }
    return nil
}
```

### Pattern: Functional Options for Complex Configuration

```go
// DO: Use functional options when a constructor has many optional parameters
type TranscriberOption func(*DeepgramTranscriber)

func WithModel(model string) TranscriberOption {
    return func(t *DeepgramTranscriber) { t.model = model }
}

func WithLanguage(lang string) TranscriberOption {
    return func(t *DeepgramTranscriber) { t.language = lang }
}

func WithDiarization(enabled bool) TranscriberOption {
    return func(t *DeepgramTranscriber) { t.diarize = enabled }
}

func NewDeepgramTranscriber(apiKey string, opts ...TranscriberOption) *DeepgramTranscriber {
    t := &DeepgramTranscriber{
        apiKey:   apiKey,
        model:    "nova-3",   // sensible default
        language: "en",       // sensible default
        diarize:  true,       // sensible default
    }
    for _, opt := range opts {
        opt(t)
    }
    return t
}

// Usage:
t := NewDeepgramTranscriber(key,
    WithModel("nova-3"),
    WithDiarization(true),
)
```

```go
// DON'T: Boolean/config explosion in constructor signature
func NewDeepgramTranscriber(
    apiKey, model, lang string,
    diarize bool,
    maxSpeakers int,
    punctuate bool,
) *DeepgramTranscriber {
    // Hard to read; easy to swap positional booleans
    return &DeepgramTranscriber{}
}
```

### Pattern: Interface Segregation for Different Consumers

```go
// DO: Different consumers define different slices of the same provider
// internal/mixer/mixer.go -- only needs stream and metadata
type AudioProvider interface {
    Stream() <-chan AudioFrame
    SampleRate() int
    Channels() int
}

// internal/usecase/record.go -- needs lifecycle control
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
}

// MalgoSource satisfies BOTH interfaces implicitly -- no declaration needed
```

```go
// DON'T: Force all consumers to depend on the same fat interface
type AudioSource interface {
    Start(ctx context.Context) error
    Stream() <-chan AudioFrame
    Stop() error
    SampleRate() int
    Channels() int
    Resample(rate int) error
    SetGain(float64) error
}
// Mixer only needs 3 methods but must mock all 7
```

### Pattern: Command Grouping for Discoverability

```go
// DO: Group related commands with AddGroup for complex CLIs
func init() {
    rootCmd.AddGroup(&cobra.Group{ID: "recording", Title: "Recording Commands:"})
    rootCmd.AddGroup(&cobra.Group{ID: "management", Title: "Management Commands:"})

    recordCmd.GroupID = "recording"
    rootCmd.AddCommand(recordCmd)

    listCmd.GroupID = "management"
    rootCmd.AddCommand(listCmd)

    doctorCmd.GroupID = "management"
    rootCmd.AddCommand(doctorCmd)
}
// Help output:
// Recording Commands:
//   record    Record and transcribe a meeting
//
// Management Commands:
//   list      List past meeting notes
//   doctor    Check system prerequisites
```

---

## Checklist

- [ ] All external dependencies sit behind interfaces in `internal/`
- [ ] Interfaces defined in the consumer package, not the provider package
- [ ] No interface has more than 5 methods; single-method interfaces use `-er` suffix
- [ ] `cobra.Command` used for all CLI command routing
- [ ] `PersistentPreRunE` on root command handles config load and permission checks
- [ ] All `RunE` functions delegate to a named function on an injected struct
- [ ] Viper binds flags, env vars, and config file with `HEIMDALL_` env prefix
- [ ] `main()` is the only place `os.Exit` is called
- [ ] Test files use mock implementations, not real external services
- [ ] No concrete types accepted as function parameters when an interface would work
- [ ] No `interface{}` / `any` return types; use typed results or generics
- [ ] All commands have `Short` and `Long` descriptions; `Example` field for common usage
