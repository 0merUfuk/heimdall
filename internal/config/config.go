package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all heimdall configuration. Loaded from ~/.heimdall/config.yaml.
type Config struct {
	Deepgram DeepgramConfig     `yaml:"deepgram"`
	Soniox   SonioxConfig       `yaml:"soniox,omitempty"`
	Claude   ClaudeConfig       `yaml:"claude"`
	Obsidian ObsidianConfig     `yaml:"obsidian"`
	Audio    AudioConfig        `yaml:"audio"`
	Output   OutputConfig       `yaml:"output"`
	Consent  ConsentConfig      `yaml:"consent,omitempty"`
	Keywords []string           `yaml:"keywords,omitempty"`
	Profiles map[string]Profile `yaml:"profiles,omitempty"`
}

// DeepgramConfig holds Deepgram API configuration.
type DeepgramConfig struct {
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	Language string `yaml:"language"`
}

// SonioxConfig holds Soniox real-time STT API configuration.
//
// Soniox is an opt-in alternative to Deepgram selected at runtime via
// `heimdall record --transcriber soniox`. The zero value is valid — users
// who never set an API key continue to use Deepgram with no config changes.
// Validation of Soniox fields is conditional on APIKey being non-empty so
// the default Deepgram path is never blocked by an unset Soniox API key.
//
// See docs/architecture/DECISIONS.md AD-011 for the strategic rationale
// (code-switched TR+EN transcription for the v1.0 Turkish market).
type SonioxConfig struct {
	// APIKey is the Soniox API key or a ${VAR} reference to one.
	APIKey string `yaml:"api_key,omitempty"`

	// Model is the Soniox real-time model name. Defaults to "stt-rt-v4"
	// when empty. The older "stt-rt-preview" alias is superseded per the
	// Soniox docs and should not be used.
	Model string `yaml:"model,omitempty"`

	// Language is the transcription language hint. Special values "multi"
	// and "auto" enable TR+EN code-switched mode. An explicit code (e.g.
	// "tr", "en") biases the decoder toward a single language.
	Language string `yaml:"language,omitempty"`
}

// ClaudeConfig holds Anthropic Claude API configuration.
type ClaudeConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

// ObsidianConfig holds Obsidian vault configuration.
type ObsidianConfig struct {
	VaultPath      string `yaml:"vault_path"`
	MeetingsFolder string `yaml:"meetings_folder"`
	Template       string `yaml:"template"`
}

// AudioConfig holds audio capture configuration.
type AudioConfig struct {
	SystemAudio   bool   `yaml:"system_audio"`
	Microphone    bool   `yaml:"microphone"`
	SaveRecording bool   `yaml:"save_recording"`
	RecordingPath string `yaml:"recording_path"`
}

// OutputConfig holds output rendering configuration.
type OutputConfig struct {
	IncludeTranscript bool   `yaml:"include_transcript"`
	IncludeTimestamps bool   `yaml:"include_timestamps"`
	Language          string `yaml:"language"`
}

// ConsentConfig holds the user's one-time recording-consent acknowledgement.
// Populated when the user presses Enter at the first-run consent banner.
// Both fields are zero-valued until acknowledgement; the banner is gated on
// Acknowledged == false.
type ConsentConfig struct {
	Acknowledged   bool   `yaml:"acknowledged,omitempty"`
	AcknowledgedAt string `yaml:"acknowledged_at,omitempty"`
}

// Profile holds per-meeting-type defaults that can be activated with --profile.
type Profile struct {
	Title        string   `yaml:"title,omitempty"`
	Participants []string `yaml:"participants,omitempty"`
	Keywords     []string `yaml:"keywords,omitempty"`
	Language     string   `yaml:"language,omitempty"`
}

// envVarPattern matches ${VAR_NAME} references in config values.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ConfigDir returns the path to the heimdall configuration directory (~/.heimdall/).
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".heimdall")
	}
	return filepath.Join(home, ".heimdall")
}

// ConfigPath returns the path to the heimdall config file (~/.heimdall/config.yaml).
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// Load reads and parses a config file from the given path. If the file does
// not exist, it returns DefaultConfig with no error. All other read or parse
// errors are returned.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Save writes the config to the given path as YAML atomically (temp file +
// rename per V-006). It creates parent directories if they do not exist. The
// directory is created with 0700 and the file is written with 0600
// permissions (owner-only) because config may contain API key references.
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Atomic write: temp file in the same directory, then rename. This
	// prevents a partial write from clobbering the existing config if the
	// process crashes or the disk fills mid-write.
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp config file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting config file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp config file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp config file: %w", err)
	}

	return nil
}

// Validate checks that the config values are valid. It returns an error
// describing the first invalid field found. Validate checks structural
// validity only -- it does not verify that API keys are functional.
func (c *Config) Validate() error {
	// Deepgram validation.
	if c.Deepgram.APIKey == "" {
		return fmt.Errorf("deepgram.api_key is required")
	}
	if c.Deepgram.Model == "" {
		return fmt.Errorf("deepgram.model is required")
	}
	validDeepgramModels := map[string]bool{
		"nova-3": true, "nova-2": true, "nova": true,
		"enhanced": true, "base": true,
	}
	if !isEnvVarRef(c.Deepgram.Model) && !validDeepgramModels[c.Deepgram.Model] {
		return fmt.Errorf("deepgram.model: unknown model %q", c.Deepgram.Model)
	}
	if c.Deepgram.Language == "" {
		return fmt.Errorf("deepgram.language is required")
	}

	// Soniox validation (conditional: only validate when the user has
	// actually configured Soniox; the zero value must not block users who
	// are happy on the Deepgram default path).
	if c.Soniox.APIKey != "" {
		validSonioxModels := map[string]bool{
			"stt-rt-v4": true,
			"stt-rt-v3": true, // auto-routes to v4 per Soniox docs
		}
		if c.Soniox.Model != "" && !isEnvVarRef(c.Soniox.Model) && !validSonioxModels[c.Soniox.Model] {
			return fmt.Errorf("soniox.model: unknown model %q", c.Soniox.Model)
		}
	}

	// Claude validation.
	if c.Claude.APIKey == "" {
		return fmt.Errorf("claude.api_key is required")
	}
	if c.Claude.Model == "" {
		return fmt.Errorf("claude.model is required")
	}
	validClaudeModels := map[string]bool{
		"claude-haiku-4-5": true, "claude-sonnet-4-5": true,
		"claude-sonnet-4-6": true, "claude-opus-4-5": true,
		"claude-opus-4-6": true,
	}
	if !isEnvVarRef(c.Claude.Model) && !validClaudeModels[c.Claude.Model] {
		return fmt.Errorf("claude.model: unknown model %q", c.Claude.Model)
	}

	// Obsidian validation.
	if c.Obsidian.VaultPath == "" {
		return fmt.Errorf("obsidian.vault_path is required")
	}
	if c.Obsidian.MeetingsFolder == "" {
		return fmt.Errorf("obsidian.meetings_folder is required")
	}

	// Audio validation.
	if !c.Audio.SystemAudio && !c.Audio.Microphone {
		return fmt.Errorf("at least one audio source must be enabled (system_audio or microphone)")
	}

	// Output validation.
	if c.Output.Language == "" {
		return fmt.Errorf("output.language is required")
	}

	return nil
}

// ResolveEnvVars expands all ${VAR_NAME} references in string fields with
// their corresponding environment variable values. Returns an error if any
// referenced environment variable is not set.
func (c *Config) ResolveEnvVars() error {
	resolvers := []struct {
		name  string
		field *string
	}{
		{"deepgram.api_key", &c.Deepgram.APIKey},
		{"deepgram.model", &c.Deepgram.Model},
		{"deepgram.language", &c.Deepgram.Language},
		{"soniox.api_key", &c.Soniox.APIKey},
		{"soniox.model", &c.Soniox.Model},
		{"soniox.language", &c.Soniox.Language},
		{"claude.api_key", &c.Claude.APIKey},
		{"claude.model", &c.Claude.Model},
		{"obsidian.vault_path", &c.Obsidian.VaultPath},
		{"obsidian.meetings_folder", &c.Obsidian.MeetingsFolder},
		{"obsidian.template", &c.Obsidian.Template},
		{"audio.recording_path", &c.Audio.RecordingPath},
		{"output.language", &c.Output.Language},
	}

	for _, r := range resolvers {
		resolved, err := resolveEnvVar(*r.field)
		if err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
		*r.field = resolved
	}

	return nil
}

// resolveEnvVar replaces all ${VAR} patterns in s with their environment
// variable values. Returns an error if any referenced variable is unset.
func resolveEnvVar(s string) (string, error) {
	var resolveErr error

	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}
		varName := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			resolveErr = fmt.Errorf("environment variable %s is not set", varName)
			return match
		}
		return val
	})

	if resolveErr != nil {
		return s, resolveErr
	}
	return result, nil
}

// isEnvVarRef returns true if s looks like an environment variable reference
// (e.g., "${SOME_VAR}"). Used to skip validation of values that will be
// resolved at runtime.
func isEnvVarRef(s string) bool {
	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")
}

// ExpandHome replaces a leading ~ in path with the user's home directory.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
