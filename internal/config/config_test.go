package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultConfig verifies that DefaultConfig returns sensible defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Deepgram.APIKey != "${DEEPGRAM_API_KEY}" {
		t.Errorf("Deepgram.APIKey: got %q, want ${DEEPGRAM_API_KEY}", cfg.Deepgram.APIKey)
	}
	if cfg.Deepgram.Model != "nova-3" {
		t.Errorf("Deepgram.Model: got %q, want nova-3", cfg.Deepgram.Model)
	}
	if cfg.Deepgram.Language != "en" {
		t.Errorf("Deepgram.Language: got %q, want en", cfg.Deepgram.Language)
	}
	if cfg.Claude.APIKey != "${ANTHROPIC_API_KEY}" {
		t.Errorf("Claude.APIKey: got %q, want ${ANTHROPIC_API_KEY}", cfg.Claude.APIKey)
	}
	if cfg.Claude.Model != "claude-haiku-4-5" {
		t.Errorf("Claude.Model: got %q, want claude-haiku-4-5", cfg.Claude.Model)
	}
	if cfg.Obsidian.MeetingsFolder != "meetings" {
		t.Errorf("Obsidian.MeetingsFolder: got %q, want meetings", cfg.Obsidian.MeetingsFolder)
	}
	if cfg.Obsidian.Template != "default" {
		t.Errorf("Obsidian.Template: got %q, want default", cfg.Obsidian.Template)
	}
	if !cfg.Audio.SystemAudio {
		t.Error("Audio.SystemAudio: expected true")
	}
	if !cfg.Audio.Microphone {
		t.Error("Audio.Microphone: expected true")
	}
	if cfg.Audio.SaveRecording {
		t.Error("Audio.SaveRecording: expected false")
	}
	if cfg.Audio.RecordingPath != "~/.heimdall/recordings/" {
		t.Errorf("Audio.RecordingPath: got %q, want ~/.heimdall/recordings/", cfg.Audio.RecordingPath)
	}
	if !cfg.Output.IncludeTranscript {
		t.Error("Output.IncludeTranscript: expected true")
	}
	if !cfg.Output.IncludeTimestamps {
		t.Error("Output.IncludeTimestamps: expected true")
	}
	if cfg.Output.Language != "en" {
		t.Errorf("Output.Language: got %q, want en", cfg.Output.Language)
	}
}

// TestLoad_FromYAMLFile verifies that Load reads and parses a YAML config file.
func TestLoad_FromYAMLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `deepgram:
  api_key: ${DEEPGRAM_API_KEY}
  model: nova-2
  language: tr
claude:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-sonnet-4-6
obsidian:
  vault_path: /tmp/vault
  meetings_folder: notes
  template: custom
audio:
  system_audio: false
  microphone: true
  save_recording: true
  recording_path: /tmp/recordings/
output:
  include_transcript: false
  include_timestamps: false
  language: tr
keywords:
  - Kubernetes
  - gRPC
`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if cfg.Deepgram.Model != "nova-2" {
		t.Errorf("Deepgram.Model: got %q, want nova-2", cfg.Deepgram.Model)
	}
	if cfg.Deepgram.Language != "tr" {
		t.Errorf("Deepgram.Language: got %q, want tr", cfg.Deepgram.Language)
	}
	if cfg.Claude.Model != "claude-sonnet-4-6" {
		t.Errorf("Claude.Model: got %q, want claude-sonnet-4-6", cfg.Claude.Model)
	}
	if cfg.Obsidian.VaultPath != "/tmp/vault" {
		t.Errorf("Obsidian.VaultPath: got %q, want /tmp/vault", cfg.Obsidian.VaultPath)
	}
	if cfg.Obsidian.MeetingsFolder != "notes" {
		t.Errorf("Obsidian.MeetingsFolder: got %q, want notes", cfg.Obsidian.MeetingsFolder)
	}
	if cfg.Audio.SystemAudio {
		t.Error("Audio.SystemAudio: expected false")
	}
	if !cfg.Audio.SaveRecording {
		t.Error("Audio.SaveRecording: expected true")
	}
	if cfg.Output.IncludeTranscript {
		t.Error("Output.IncludeTranscript: expected false")
	}
	if cfg.Output.Language != "tr" {
		t.Errorf("Output.Language: got %q, want tr", cfg.Output.Language)
	}
	if len(cfg.Keywords) != 2 {
		t.Fatalf("Keywords length: got %d, want 2", len(cfg.Keywords))
	}
	if cfg.Keywords[0] != "Kubernetes" {
		t.Errorf("Keywords[0]: got %q, want Kubernetes", cfg.Keywords[0])
	}
}

// TestLoad_NonExistentFile verifies that Load returns DefaultConfig for a missing file.
func TestLoad_NonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load: expected no error for missing file, got: %v", err)
	}

	defaults := DefaultConfig()
	if cfg.Deepgram.Model != defaults.Deepgram.Model {
		t.Errorf("Deepgram.Model: got %q, want default %q", cfg.Deepgram.Model, defaults.Deepgram.Model)
	}
	if cfg.Claude.Model != defaults.Claude.Model {
		t.Errorf("Claude.Model: got %q, want default %q", cfg.Claude.Model, defaults.Claude.Model)
	}
}

// TestLoad_InvalidYAML verifies that Load returns an error for malformed YAML.
func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `deepgram:
  api_key: [invalid yaml
  model: nova-3
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing config file") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

// TestSave_AndLoad_Roundtrip verifies that saving and loading produces the same config.
func TestSave_AndLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := DefaultConfig()
	original.Deepgram.Model = "nova-2"
	original.Obsidian.VaultPath = "/home/user/vault"
	original.Keywords = []string{"Go", "WebSocket"}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if loaded.Deepgram.Model != original.Deepgram.Model {
		t.Errorf("Deepgram.Model roundtrip: got %q, want %q", loaded.Deepgram.Model, original.Deepgram.Model)
	}
	if loaded.Obsidian.VaultPath != original.Obsidian.VaultPath {
		t.Errorf("Obsidian.VaultPath roundtrip: got %q, want %q", loaded.Obsidian.VaultPath, original.Obsidian.VaultPath)
	}
	if len(loaded.Keywords) != len(original.Keywords) {
		t.Fatalf("Keywords roundtrip: got %d items, want %d", len(loaded.Keywords), len(original.Keywords))
	}
	for i, kw := range loaded.Keywords {
		if kw != original.Keywords[i] {
			t.Errorf("Keywords[%d] roundtrip: got %q, want %q", i, kw, original.Keywords[i])
		}
	}
}

// TestProfile_Roundtrip verifies that profiles survive save/load.
func TestProfile_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := DefaultConfig()
	original.Obsidian.VaultPath = "/tmp/vault"
	original.Profiles = map[string]Profile{
		"daily": {
			Title:        "Daily Standup",
			Participants: []string{"Alice", "Bob"},
			Keywords:     []string{"sprint", "blockers"},
			Language:     "en",
		},
		"1on1": {
			Title:    "1:1 with Manager",
			Language: "tr",
		},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if len(loaded.Profiles) != 2 {
		t.Fatalf("Profiles count: got %d, want 2", len(loaded.Profiles))
	}

	daily := loaded.Profiles["daily"]
	if daily.Title != "Daily Standup" {
		t.Errorf("daily.Title: got %q, want %q", daily.Title, "Daily Standup")
	}
	if len(daily.Participants) != 2 {
		t.Fatalf("daily.Participants count: got %d, want 2", len(daily.Participants))
	}
	if daily.Participants[0] != "Alice" {
		t.Errorf("daily.Participants[0]: got %q, want Alice", daily.Participants[0])
	}
	if len(daily.Keywords) != 2 {
		t.Fatalf("daily.Keywords count: got %d, want 2", len(daily.Keywords))
	}
	if daily.Language != "en" {
		t.Errorf("daily.Language: got %q, want en", daily.Language)
	}

	oneOnOne := loaded.Profiles["1on1"]
	if oneOnOne.Title != "1:1 with Manager" {
		t.Errorf("1on1.Title: got %q, want %q", oneOnOne.Title, "1:1 with Manager")
	}
	if oneOnOne.Language != "tr" {
		t.Errorf("1on1.Language: got %q, want tr", oneOnOne.Language)
	}
	if len(oneOnOne.Participants) != 0 {
		t.Errorf("1on1.Participants: got %d, want 0", len(oneOnOne.Participants))
	}
}

// TestLoad_WithProfiles verifies that profiles are loaded from YAML.
func TestLoad_WithProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `deepgram:
  api_key: ${DEEPGRAM_API_KEY}
  model: nova-3
  language: en
claude:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-haiku-4-5
obsidian:
  vault_path: /tmp/vault
  meetings_folder: meetings
  template: default
audio:
  system_audio: true
  microphone: true
  save_recording: false
  recording_path: ~/.heimdall/recordings/
output:
  include_transcript: true
  include_timestamps: true
  language: en
profiles:
  standup:
    title: Daily Standup
    participants:
      - Alice
      - Bob
    keywords:
      - sprint
    language: en
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if len(cfg.Profiles) != 1 {
		t.Fatalf("Profiles count: got %d, want 1", len(cfg.Profiles))
	}

	standup, ok := cfg.Profiles["standup"]
	if !ok {
		t.Fatal("Profiles: missing 'standup' profile")
	}
	if standup.Title != "Daily Standup" {
		t.Errorf("standup.Title: got %q, want %q", standup.Title, "Daily Standup")
	}
	if len(standup.Participants) != 2 {
		t.Fatalf("standup.Participants count: got %d, want 2", len(standup.Participants))
	}
}

// TestDefaultConfig_ProfilesNil verifies that Profiles is nil by default.
func TestDefaultConfig_ProfilesNil(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Profiles != nil {
		t.Errorf("Profiles: expected nil, got %v", cfg.Profiles)
	}
}

// TestSave_CreatesParentDirs verifies that Save creates missing directories.
func TestSave_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Save did not create the config file")
	}
}

// TestResolveEnvVars_ExpandsVariables verifies that env var references are expanded.
func TestResolveEnvVars_ExpandsVariables(t *testing.T) {
	t.Setenv("DEEPGRAM_API_KEY", "dg-test-key-123")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key-456")

	cfg := DefaultConfig()
	if err := cfg.ResolveEnvVars(); err != nil {
		t.Fatalf("ResolveEnvVars: unexpected error: %v", err)
	}

	if cfg.Deepgram.APIKey != "dg-test-key-123" {
		t.Errorf("Deepgram.APIKey: got %q, want dg-test-key-123", cfg.Deepgram.APIKey)
	}
	if cfg.Claude.APIKey != "sk-ant-test-key-456" {
		t.Errorf("Claude.APIKey: got %q, want sk-ant-test-key-456", cfg.Claude.APIKey)
	}
}

// TestResolveEnvVars_MissingVariable verifies that a missing env var produces an error.
func TestResolveEnvVars_MissingVariable(t *testing.T) {
	// Unset the variable to ensure it is not set.
	t.Setenv("DEEPGRAM_API_KEY", "some-value")
	os.Unsetenv("ANTHROPIC_API_KEY")

	cfg := DefaultConfig()
	err := cfg.ResolveEnvVars()
	if err == nil {
		t.Fatal("ResolveEnvVars: expected error for missing env var, got nil")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("error should mention ANTHROPIC_API_KEY, got: %v", err)
	}
}

// TestResolveEnvVars_NoReferencesIsNoop verifies that values without ${} are unchanged.
func TestResolveEnvVars_NoReferencesIsNoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Deepgram.APIKey = "plain-key"
	cfg.Claude.APIKey = "another-plain-key"
	cfg.Deepgram.Model = "nova-3"

	if err := cfg.ResolveEnvVars(); err != nil {
		t.Fatalf("ResolveEnvVars: unexpected error: %v", err)
	}

	if cfg.Deepgram.APIKey != "plain-key" {
		t.Errorf("Deepgram.APIKey: got %q, want plain-key", cfg.Deepgram.APIKey)
	}
	if cfg.Deepgram.Model != "nova-3" {
		t.Errorf("Deepgram.Model: got %q, want nova-3", cfg.Deepgram.Model)
	}
}

// TestValidate_ValidConfig verifies that a complete config passes validation.
func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: expected no error for valid config, got: %v", err)
	}
}

// TestValidate_MissingVaultPath catches a missing vault path.
func TestValidate_MissingVaultPath(t *testing.T) {
	cfg := DefaultConfig()
	// VaultPath is empty by default.

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for missing vault_path, got nil")
	}
	if !strings.Contains(err.Error(), "vault_path") {
		t.Errorf("error should mention vault_path, got: %v", err)
	}
}

// TestValidate_InvalidDeepgramModel catches an invalid Deepgram model name.
func TestValidate_InvalidDeepgramModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Deepgram.Model = "invalid-model"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for invalid model, got nil")
	}
	if !strings.Contains(err.Error(), "deepgram.model") {
		t.Errorf("error should mention deepgram.model, got: %v", err)
	}
}

// TestValidate_InvalidClaudeModel catches an invalid Claude model name.
func TestValidate_InvalidClaudeModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Claude.Model = "gpt-4"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for invalid model, got nil")
	}
	if !strings.Contains(err.Error(), "claude.model") {
		t.Errorf("error should mention claude.model, got: %v", err)
	}
}

// TestValidate_NoAudioSources catches both audio sources disabled.
func TestValidate_NoAudioSources(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Audio.SystemAudio = false
	cfg.Audio.Microphone = false

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for no audio sources, got nil")
	}
	if !strings.Contains(err.Error(), "audio source") {
		t.Errorf("error should mention audio source, got: %v", err)
	}
}

// TestValidate_EmptyAPIKey catches an empty Deepgram API key.
func TestValidate_EmptyAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Deepgram.APIKey = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for empty api_key, got nil")
	}
	if !strings.Contains(err.Error(), "deepgram.api_key") {
		t.Errorf("error should mention deepgram.api_key, got: %v", err)
	}
}

// TestValidate_EmptyClaudeAPIKey catches an empty Claude API key.
func TestValidate_EmptyClaudeAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Claude.APIKey = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for empty api_key, got nil")
	}
	if !strings.Contains(err.Error(), "claude.api_key") {
		t.Errorf("error should mention claude.api_key, got: %v", err)
	}
}

// TestValidate_EmptyOutputLanguage catches an empty output language.
func TestValidate_EmptyOutputLanguage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Output.Language = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error for empty language, got nil")
	}
	if !strings.Contains(err.Error(), "output.language") {
		t.Errorf("error should mention output.language, got: %v", err)
	}
}

// TestValidate_EnvVarRefModelAccepted verifies that env var references in model
// fields pass validation (they will be resolved later).
func TestValidate_EnvVarRefModelAccepted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/tmp/vault"
	cfg.Deepgram.Model = "${DEEPGRAM_MODEL}"
	cfg.Claude.Model = "${CLAUDE_MODEL}"

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: env var ref in model field should pass, got: %v", err)
	}
}

// TestConfigDir_ReturnsHomeBased verifies that ConfigDir returns a path under home.
func TestConfigDir_ReturnsHomeBased(t *testing.T) {
	dir := ConfigDir()
	if !strings.HasSuffix(dir, ".heimdall") {
		t.Errorf("ConfigDir: got %q, want suffix .heimdall", dir)
	}
}

// TestConfigPath_ReturnsYAMLPath verifies ConfigPath ends with config.yaml.
func TestConfigPath_ReturnsYAMLPath(t *testing.T) {
	path := ConfigPath()
	if !strings.HasSuffix(path, filepath.Join(".heimdall", "config.yaml")) {
		t.Errorf("ConfigPath: got %q, want suffix .heimdall/config.yaml", path)
	}
}

// TestExpandHome verifies tilde expansion in paths.
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde prefix", "~/Documents", filepath.Join(home, "Documents")},
		{"tilde with nested", "~/.heimdall/recordings", filepath.Join(home, ".heimdall", "recordings")},
		{"absolute path unchanged", "/usr/local/bin", "/usr/local/bin"},
		{"relative path unchanged", "relative/path", "relative/path"},
		{"bare tilde", "~", home},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandHome(tt.path)
			if got != tt.want {
				t.Errorf("ExpandHome(%q): got %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestLoad_PartialYAML verifies that a partial YAML file merges with defaults.
func TestLoad_PartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Only override one field -- the rest should come from defaults.
	content := `deepgram:
  model: nova-2
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	// Overridden field.
	if cfg.Deepgram.Model != "nova-2" {
		t.Errorf("Deepgram.Model: got %q, want nova-2", cfg.Deepgram.Model)
	}

	// Defaults preserved.
	if cfg.Deepgram.APIKey != "${DEEPGRAM_API_KEY}" {
		t.Errorf("Deepgram.APIKey: got %q, want ${DEEPGRAM_API_KEY} (default)", cfg.Deepgram.APIKey)
	}
	if cfg.Claude.Model != "claude-haiku-4-5" {
		t.Errorf("Claude.Model: got %q, want claude-haiku-4-5 (default)", cfg.Claude.Model)
	}
}

// TestResolveEnvVars_PartialRef verifies mixed text with env var reference.
func TestResolveEnvVars_PartialRef(t *testing.T) {
	t.Setenv("DEEPGRAM_API_KEY", "dg-key")
	t.Setenv("ANTHROPIC_API_KEY", "sk-key")

	cfg := DefaultConfig()
	cfg.Obsidian.VaultPath = "/vault/${ANTHROPIC_API_KEY}/data"

	// Resolve -- the partial ref in vault_path should expand too.
	// But first set the required env vars.
	if err := cfg.ResolveEnvVars(); err != nil {
		t.Fatalf("ResolveEnvVars: unexpected error: %v", err)
	}

	if cfg.Obsidian.VaultPath != "/vault/sk-key/data" {
		t.Errorf("VaultPath: got %q, want /vault/sk-key/data", cfg.Obsidian.VaultPath)
	}
}
