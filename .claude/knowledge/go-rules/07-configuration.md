**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** oracle-cli (go)

---

# Configuration -- go

**Purpose**: Define configuration loading, validation, and injection patterns for Go CLI applications using Cobra and Viper.
**Audience**: Developers building go applications
**Stack**: Go 1.25

---

## Overview / Core Concepts

Configuration in Go CLI applications follows a strict precedence hierarchy managed by Viper: explicit `Set()` calls > CLI flags > environment variables > config file > default values. This hierarchy ensures that the most specific source always wins, and users can override any setting at any level. Viper integrates tightly with Cobra's flag system, so a single flag definition simultaneously supports CLI flags, environment variables, and config file keys.

The canonical architecture is: load config in `PersistentPreRunE` (not `init()`), unmarshal into a typed struct, validate immediately, and inject via `cmd.SetContext()`. This runs before every subcommand, receives the command context for error handling, and avoids the global state problems that `init()` creates. Each subcommand retrieves the validated config from context -- never from the global Viper instance.

API keys and other secrets must never appear in config files committed to version control. The recommended approach is environment variables with a `HEIMDALL_` prefix (e.g., `HEIMDALL_DEEPGRAM_API_KEY`). Viper's `SetEnvPrefix` and `AutomaticEnv` handle the mapping automatically. The config file (`~/.heimdall/heimdall.yaml`) stores non-sensitive preferences; the environment provides credentials.

---

## Rules

### Always
- Follow Viper precedence: explicit Set() > CLI flags > environment variables > config file > defaults
- Set defaults for every config key with `viper.SetDefault` before reading config
- Unmarshal into a typed struct with `mapstructure` tags -- never use ad-hoc `Get` calls scattered through code
- Validate config immediately after loading -- check required fields, value ranges, dependent constraints
- Use `viper.New()` instances instead of the global viper -- global state breaks parallel tests
- Set an env prefix with `SetEnvPrefix("HEIMDALL")` to prevent OS variable collisions
- Use `SetEnvKeyReplacer` to map kebab-case config keys to SCREAMING_SNAKE_CASE env vars
- Initialize config in `PersistentPreRunE`, not in `init()` or `cobra.OnInitialize`
- Inject config into subcommands via `cmd.SetContext()` -- never re-read global viper in subcommands
- Handle `viper.ConfigFileNotFoundError` gracefully -- config file is optional on fresh installs

### Never
- Never log secrets (API keys, tokens, passwords) at any log level, including debug
- Never store API keys in the config file on disk -- use environment variables
- Never use `cobra.OnInitialize` with global viper -- errors cannot be returned; forces `log.Fatal` or panic
- Never read `viper.GetString` without `viper.IsSet` when the distinction between "not found" and "empty" matters
- Never call `viper.ReadInConfig()` in `init()` -- runs before flags are parsed, preventing CLI overrides

---

## Patterns

### Pattern: Root Command with Viper Integration

```go
// DO: Bind flags and load config in PersistentPreRunE
var rootCmd = &cobra.Command{
    Use:   "heimdall",
    Short: "Meeting companion CLI",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return initConfig(cmd)
    },
}

func init() {
    rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
    rootCmd.PersistentFlags().String("api-key", "", "Deepgram API key")
    rootCmd.PersistentFlags().String("output-dir", "", "Obsidian vault path")
}

func initConfig(cmd *cobra.Command) error {
    v := viper.New()

    if err := v.BindPFlags(cmd.Flags()); err != nil {
        return fmt.Errorf("bindflags: %w", err)
    }

    if cfgFile := v.GetString("config"); cfgFile != "" {
        v.SetConfigFile(cfgFile)
    } else {
        home, err := os.UserHomeDir()
        if err != nil {
            return fmt.Errorf("home dir: %w", err)
        }
        v.SetConfigName("heimdall")
        v.SetConfigType("yaml")
        v.AddConfigPath(filepath.Join(home, ".heimdall"))
        v.AddConfigPath(".")
    }

    v.SetEnvPrefix("HEIMDALL")
    v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
    v.AutomaticEnv()

    v.SetDefault("transcriber.sample-rate", 16000)
    v.SetDefault("transcriber.channels", 2)
    v.SetDefault("analyzer.model", "claude-sonnet-4-20250514")

    if err := v.ReadInConfig(); err != nil {
        var notFound viper.ConfigFileNotFoundError
        if !errors.As(err, &notFound) {
            return fmt.Errorf("read config: %w", err)
        }
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return fmt.Errorf("unmarshal config: %w", err)
    }
    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }

    cmd.SetContext(context.WithValue(cmd.Context(), configKey{}, &cfg))
    return nil
}
```

```go
// DON'T: Use cobra.OnInitialize with global viper
func init() {
    cobra.OnInitialize(initConfigGlobal) // errors can't be returned
}
var globalCfg Config
func initConfigGlobal() {
    viper.SetConfigName("config")
    viper.ReadInConfig()          // silently ignores errors
    viper.Unmarshal(&globalCfg)   // global state -- impossible to test in parallel
}
```

### Pattern: Typed Config Struct with Validation

```go
// DO: Define a complete typed struct; validate after unmarshal
type Config struct {
    APIKey      string           `mapstructure:"api-key"`
    OutputDir   string           `mapstructure:"output-dir"`
    Transcriber TranscriberConfig `mapstructure:"transcriber"`
    Analyzer    AnalyzerConfig   `mapstructure:"analyzer"`
}

type TranscriberConfig struct {
    Provider   string `mapstructure:"provider"`
    Model      string `mapstructure:"model"`
    SampleRate int    `mapstructure:"sample-rate"`
    Channels   int    `mapstructure:"channels"`
}

type AnalyzerConfig struct {
    Provider string `mapstructure:"provider"`
    Model    string `mapstructure:"model"`
}

func (c *Config) Validate() error {
    var errs []error
    if c.APIKey == "" {
        errs = append(errs, errors.New("api-key is required (set HEIMDALL_API_KEY or --api-key)"))
    }
    if c.OutputDir == "" {
        errs = append(errs, errors.New("output-dir is required"))
    }
    if c.Transcriber.SampleRate <= 0 {
        errs = append(errs, errors.New("transcriber.sample-rate must be positive"))
    }
    return errors.Join(errs...) // returns nil if slice is empty
}
```

```go
// DON'T: Read individual keys scattered throughout the codebase
func record(cmd *cobra.Command) {
    apiKey := viper.GetString("api-key")       // no validation, no type safety
    sampleRate := viper.GetInt("sample-rate")  // scattered, hard to audit
    model := viper.GetString("analyzer.model") // string keys can typo silently
}
```

### Pattern: Environment Variable Binding with Prefix

```go
// DO: SetEnvPrefix + AutomaticEnv + SetEnvKeyReplacer
v := viper.New()
v.SetEnvPrefix("HEIMDALL")
v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
v.AutomaticEnv()

// Config key "api-key"                    -> env var "HEIMDALL_API_KEY"
// Config key "transcriber.sample-rate"    -> env var "HEIMDALL_TRANSCRIBER_SAMPLE_RATE"
```

```go
// DON'T: Manually bind every key to os.Getenv
apiKey := os.Getenv("HEIMDALL_API_KEY")
if apiKey == "" {
    apiKey = viper.GetString("api-key") // bypasses Viper precedence entirely
}
```

### Pattern: Config File YAML Schema

```yaml
# ~/.heimdall/heimdall.yaml
# API keys should be set via HEIMDALL_API_KEY env var, not here
output-dir: ~/notes

transcriber:
  provider: deepgram
  model: nova-3
  sample-rate: 16000
  channels: 2

analyzer:
  provider: claude
  model: claude-sonnet-4-20250514
```

### Pattern: Injecting Config into Subcommands via Context

```go
// DO: Store config on context in PersistentPreRunE; retrieve in subcommand
type configKey struct{}

func configFromCtx(ctx context.Context) *Config {
    cfg, ok := ctx.Value(configKey{}).(*Config)
    if !ok {
        panic("config not in context -- PersistentPreRunE not set up correctly")
    }
    return cfg
}

var recordCmd = &cobra.Command{
    Use:   "record",
    Short: "Start recording a meeting",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg := configFromCtx(cmd.Context())
        return runRecord(cmd.Context(), cfg)
    },
}
```

```go
// DON'T: Re-read global viper in every subcommand
var recordCmd = &cobra.Command{
    RunE: func(cmd *cobra.Command, args []string) error {
        apiKey := viper.GetString("api-key")    // global state, hard to test
        outputDir := viper.GetString("output-dir")
    },
}
```

### Pattern: Config Wizard for First-Run Setup

```go
// DO: Interactive setup wizard that creates the config file
func runConfigWizard() error {
    home, err := os.UserHomeDir()
    if err != nil {
        return fmt.Errorf("wizard home dir: %w", err)
    }

    configDir := filepath.Join(home, ".heimdall")
    if err := os.MkdirAll(configDir, 0755); err != nil {
        return fmt.Errorf("wizard mkdir: %w", err)
    }

    configPath := filepath.Join(configDir, "heimdall.yaml")
    if _, err := os.Stat(configPath); err == nil {
        fmt.Println("Config file already exists:", configPath)
        return nil
    }

    fmt.Println("Welcome to heimdall! Let's set up your configuration.")
    fmt.Println()
    fmt.Print("Obsidian vault path: ")
    var vaultPath string
    fmt.Scanln(&vaultPath)

    cfg := fmt.Sprintf("output-dir: %s\n\ntranscriber:\n  provider: deepgram\n  model: nova-3\n  sample-rate: 16000\n  channels: 2\n\nanalyzer:\n  provider: claude\n  model: claude-sonnet-4-20250514\n", vaultPath)

    if err := os.WriteFile(configPath, []byte(cfg), 0600); err != nil {
        return fmt.Errorf("wizard write config: %w", err)
    }

    fmt.Println()
    fmt.Println("Config written to:", configPath)
    fmt.Println("Set API keys via environment variables:")
    fmt.Println("  export HEIMDALL_DEEPGRAM_API_KEY=...")
    fmt.Println("  export HEIMDALL_CLAUDE_API_KEY=...")
    return nil
}
```

### Pattern: Config Directory Resolution

```go
// DO: Resolve paths from os.UserHomeDir, not hardcoded strings
func configDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("configDir: %w", err)
    }
    return filepath.Join(home, ".heimdall"), nil
}
```

```go
// DON'T: Hardcode home directory paths
dir := "/Users/omerufuk/.heimdall" // breaks on any other machine
dir := "~/.heimdall"               // ~ is shell syntax; Go won't expand it
```

---

## Checklist

- [ ] All config keys have `viper.SetDefault` values
- [ ] Config loaded in `PersistentPreRunE`, not `init()` or `cobra.OnInitialize`
- [ ] `viper.New()` used -- no global Viper state
- [ ] `SetEnvPrefix` set to `HEIMDALL` to avoid OS variable collisions
- [ ] `SetEnvKeyReplacer` maps kebab-case keys to SCREAMING_SNAKE_CASE env vars
- [ ] `viper.Unmarshal` targets a typed struct with `mapstructure` tags
- [ ] `cfg.Validate()` called immediately after unmarshal
- [ ] Required-but-missing fields produce actionable error messages with env var names
- [ ] No API keys, tokens, or passwords appear in logs or startup output
- [ ] Config injected into subcommands via `cmd.Context()`, not global Viper
- [ ] `viper.ConfigFileNotFoundError` handled gracefully (config file is optional)
- [ ] Config file location: `~/.heimdall/heimdall.yaml` (primary) and `.` (project-level override)
