package main

// helpers_test.go — tests for pure helper functions and subcommand paths that
// do not require real audio devices, network, or external APIs.
//
// Covered in this file:
//   - formatDuration
//   - displaySegment
//   - splitAndTrim
//   - validateLanguage
//   - validateVaultPath
//   - validateClaudeModel / knownClaudeModelList
//   - getConfigValue (error paths)
//   - setConfigValue (all branches)
//   - printProfile
//   - runConfigProfiles (no-profiles and with-profiles paths)
//   - runConfigAddProfile (create + update paths)
//   - runList (no-vault-configured, empty-dir, populated-dir, --since filter, invalid-since)
//   - runRecover (no-recovery-files path)
//   - runRecord: validates the --transcriber flag fast-fail paths and missing
//     API-key paths without opening audio devices or network connections.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  string
	}{
		{"zero", 0, "00:00:00"},
		{"one second", time.Second, "00:00:01"},
		{"59 seconds", 59 * time.Second, "00:00:59"},
		{"one minute", 60 * time.Second, "00:01:00"},
		{"90 seconds", 90 * time.Second, "00:01:30"},
		{"one hour", 3600 * time.Second, "01:00:00"},
		{"1h2m3s", (3600 + 120 + 3) * time.Second, "01:02:03"},
		{"two hours", 2 * 3600 * time.Second, "02:00:00"},
		{"large duration", 10*3600*time.Second + 59*time.Minute + 59*time.Second, "10:59:59"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.input)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// displaySegment
// ---------------------------------------------------------------------------

func TestDisplaySegment_Final(t *testing.T) {
	seg := heimdall.Segment{
		Start:   5 * time.Second,
		End:     7 * time.Second,
		Speaker: 2,
		Text:    "hello world",
		IsFinal: true,
	}

	out := captureStdout(t, func() {
		displaySegment(seg)
	})

	if !strings.Contains(out, "[00:00:05]") {
		t.Errorf("expected timestamp in output, got: %q", out)
	}
	if !strings.Contains(out, "Speaker 2") {
		t.Errorf("expected speaker label in output, got: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected text in output, got: %q", out)
	}
	// Final segment must end with a newline.
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("final segment output should end with newline, got: %q", out)
	}
}

func TestDisplaySegment_Interim_Short(t *testing.T) {
	seg := heimdall.Segment{
		Start:   0,
		End:     1 * time.Second,
		Speaker: 1,
		Text:    "interim text",
		IsFinal: false,
	}

	out := captureStdout(t, func() {
		displaySegment(seg)
	})

	// Interim: written with \r, no trailing \n.
	if !strings.HasPrefix(out, "\r") {
		t.Errorf("interim segment should start with carriage return, got: %q", out)
	}
	if strings.HasSuffix(out, "\n") {
		t.Errorf("interim segment should NOT end with newline, got: %q", out)
	}
	if !strings.Contains(out, "interim text") {
		t.Errorf("expected text in output, got: %q", out)
	}
}

func TestDisplaySegment_Interim_LongTextTruncated(t *testing.T) {
	// Text longer than 80 chars should be truncated to 77 chars + "..."
	longText := strings.Repeat("a", 100)
	seg := heimdall.Segment{
		Start:   2 * time.Second,
		Speaker: 0,
		Text:    longText,
		IsFinal: false,
	}

	out := captureStdout(t, func() {
		displaySegment(seg)
	})

	// Must not contain the full 100-char string; must end with "..."
	if strings.Contains(out, longText) {
		t.Error("interim long text was not truncated")
	}
	if !strings.Contains(out, "...") {
		t.Errorf("truncated interim should contain '...', got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// splitAndTrim
// ---------------------------------------------------------------------------

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"single item", "alice", []string{"alice"}},
		{"two items", "alice,bob", []string{"alice", "bob"}},
		{"items with spaces", " alice , bob ", []string{"alice", "bob"}},
		{"empty items discarded", "alice,,bob", []string{"alice", "bob"}},
		{"only commas", ",,", nil},
		{"whitespace only item", "alice, , bob", []string{"alice", "bob"}},
		{"trailing comma", "alice,bob,", []string{"alice", "bob"}},
		{"leading comma", ",alice,bob", []string{"alice", "bob"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrim(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitAndTrim(%q) = %v; want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitAndTrim(%q)[%d] = %q; want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validateLanguage
// ---------------------------------------------------------------------------

func TestValidateLanguage(t *testing.T) {
	tests := []struct {
		name    string
		lang    string
		wantMsg bool // true = expect a non-empty warning
	}{
		{"english", "en", false},
		{"turkish", "tr", false},
		{"multi auto-detect", "multi", false},
		{"unknown code", "xx", true},
		{"empty string", "", true},
		{"mixed case", "EN", true}, // codes are case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateLanguage(tt.lang)
			if tt.wantMsg && got == "" {
				t.Errorf("validateLanguage(%q) returned empty warning; want a warning", tt.lang)
			}
			if !tt.wantMsg && got != "" {
				t.Errorf("validateLanguage(%q) returned unexpected warning: %q", tt.lang, got)
			}
		})
	}
}

func TestValidateLanguage_WarnContainsLang(t *testing.T) {
	warn := validateLanguage("zz")
	if !strings.Contains(warn, "zz") {
		t.Errorf("warning should mention the bad language code, got: %q", warn)
	}
}

// ---------------------------------------------------------------------------
// validateVaultPath
// ---------------------------------------------------------------------------

func TestValidateVaultPath(t *testing.T) {
	tmp := t.TempDir()

	// Valid directory path.
	if warn := validateVaultPath(tmp); warn != "" {
		t.Errorf("valid dir should produce no warning, got: %q", warn)
	}

	// Non-existent path.
	nonExistent := filepath.Join(tmp, "does_not_exist")
	if warn := validateVaultPath(nonExistent); warn == "" {
		t.Error("non-existent path should produce a warning")
	}

	// Path to a file, not a directory.
	filePath := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if warn := validateVaultPath(filePath); warn == "" {
		t.Error("file path (not dir) should produce a warning")
	}
}

func TestValidateVaultPath_TildeExpansion(t *testing.T) {
	// ~/ prefix should be expanded — we just check it doesn't panic and
	// returns a warning for a non-existent path (since ~/nonexistent_abc123
	// is extremely unlikely to exist).
	warn := validateVaultPath("~/heimdall_test_nonexistent_xyz9876")
	if warn == "" {
		t.Error("non-existent tilde path should produce a warning")
	}
}

// ---------------------------------------------------------------------------
// validateClaudeModel / knownClaudeModelList
// ---------------------------------------------------------------------------

func TestValidateClaudeModel(t *testing.T) {
	tests := []struct {
		model   string
		wantMsg bool
	}{
		{"claude-haiku-4-5", false},
		{"claude-sonnet-4-5", false},
		{"claude-sonnet-4-6", false},
		{"claude-opus-4-5", false},
		{"gpt-4o", true},  // not a Claude model
		{"", true},        // empty is unknown
		{"claude-3", true}, // old naming, not in known set
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := validateClaudeModel(tt.model)
			if tt.wantMsg && got == "" {
				t.Errorf("validateClaudeModel(%q) should warn, got empty", tt.model)
			}
			if !tt.wantMsg && got != "" {
				t.Errorf("validateClaudeModel(%q) should not warn, got: %q", tt.model, got)
			}
		})
	}
}

func TestKnownClaudeModelList_NonEmpty(t *testing.T) {
	models := knownClaudeModelList()
	if len(models) == 0 {
		t.Error("knownClaudeModelList should return at least one model")
	}
	for _, m := range models {
		if !strings.HasPrefix(m, "claude-") {
			t.Errorf("unexpected model in list: %q", m)
		}
	}
}

// ---------------------------------------------------------------------------
// getConfigValue — error paths not yet covered
// ---------------------------------------------------------------------------

func TestGetConfigValue_UnknownKey(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := getConfigValue(cfg, "nonexistent.key")
	if err == nil {
		t.Error("getConfigValue with unknown key should return error")
	}
}

func TestGetConfigValue_TopLevelKey(t *testing.T) {
	cfg := config.DefaultConfig()
	// "keywords" is a top-level slice — just verify it doesn't blow up.
	_, err := getConfigValue(cfg, "keywords")
	if err != nil {
		// It may return an error or not, depending on YAML encoding; we only
		// require it doesn't panic.
		_ = err
	}
}

func TestGetConfigValue_NotAMap(t *testing.T) {
	cfg := config.DefaultConfig()
	// Accessing a leaf value as if it were a nested map should return an error.
	_, err := getConfigValue(cfg, "deepgram.model.sub")
	if err == nil {
		t.Error("getConfigValue on leaf-as-map should return error")
	}
}

// ---------------------------------------------------------------------------
// setConfigValue — branches not exercised by existing tests
// ---------------------------------------------------------------------------

func TestSetConfigValue_AllKeys(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"deepgram.api_key", "dg_test"},
		{"deepgram.model", "nova-3"},
		{"deepgram.language", "tr"},
		{"soniox.api_key", "snx_test"},
		{"soniox.model", "stt-rt-v4"},
		{"soniox.language", "tr"},
		{"claude.api_key", "sk-ant-test"},
		{"claude.model", "claude-haiku-4-5"},
		{"obsidian.vault_path", "/tmp/vault"},
		{"obsidian.meetings_folder", "meetings"},
		{"obsidian.template", "custom"},
		{"audio.system_audio", "true"},
		{"audio.microphone", "false"},
		{"audio.save_recording", "true"},
		{"audio.recording_path", "/tmp/recordings"},
		{"output.include_transcript", "false"},
		{"output.include_timestamps", "false"},
		{"output.language", "tr"},
		{"keywords", "sprint,planning"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			cfg := config.DefaultConfig()
			err := setConfigValue(cfg, tt.key, tt.value)
			if err != nil {
				t.Errorf("setConfigValue(%q, %q) unexpected error: %v", tt.key, tt.value, err)
			}
		})
	}
}

func TestSetConfigValue_AudioBoolFalse(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Audio.SystemAudio = true
	if err := setConfigValue(cfg, "audio.system_audio", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Audio.SystemAudio {
		t.Error("audio.system_audio should have been set to false")
	}
}

func TestSetConfigValue_AudioBoolTrue(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Audio.Microphone = false
	if err := setConfigValue(cfg, "audio.microphone", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Audio.Microphone {
		t.Error("audio.microphone should have been set to true")
	}
}

func TestSetConfigValue_UnknownKey(t *testing.T) {
	cfg := config.DefaultConfig()
	err := setConfigValue(cfg, "totally.bogus.key", "value")
	if err == nil {
		t.Error("setConfigValue with unknown key should return error")
	}
}

func TestSetConfigValue_KeywordsSlice(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigValue(cfg, "keywords", "alpha, beta, gamma"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Keywords) != 3 {
		t.Errorf("keywords should have 3 items, got %d: %v", len(cfg.Keywords), cfg.Keywords)
	}
}

// ---------------------------------------------------------------------------
// printProfile
// ---------------------------------------------------------------------------

func TestPrintProfile(t *testing.T) {
	p := config.Profile{
		Title:        "Daily Standup",
		Participants: []string{"Alice", "Bob"},
		Keywords:     []string{"sprint", "blocker"},
		Language:     "en",
	}

	out := captureStdout(t, func() {
		printProfile("daily", p)
	})

	if !strings.Contains(out, "daily") {
		t.Errorf("output should contain profile name, got: %q", out)
	}
	if !strings.Contains(out, "Daily Standup") {
		t.Errorf("output should contain title, got: %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("output should contain participant, got: %q", out)
	}
	if !strings.Contains(out, "sprint") {
		t.Errorf("output should contain keyword, got: %q", out)
	}
	if !strings.Contains(out, "en") {
		t.Errorf("output should contain language, got: %q", out)
	}
}

func TestPrintProfile_EmptyProfile(t *testing.T) {
	// An empty profile should not panic; just print the name brackets.
	out := captureStdout(t, func() {
		printProfile("empty", config.Profile{})
	})
	if !strings.Contains(out, "empty") {
		t.Errorf("empty profile output should still contain name, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// runConfigProfiles
// ---------------------------------------------------------------------------

func TestRunConfigProfiles_NoProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	// Ensure no profiles.
	cfg.Profiles = nil
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runConfigProfiles(nil, nil); err != nil {
			t.Fatalf("runConfigProfiles: %v", err)
		}
	})

	if !strings.Contains(out, "No profiles") {
		t.Errorf("expected 'No profiles' message, got: %q", out)
	}
}

func TestRunConfigProfiles_WithProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.Profile{
		"daily": {Title: "Daily Standup", Language: "en"},
		"1on1":  {Title: "1-on-1", Language: "tr"},
	}
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runConfigProfiles(nil, nil); err != nil {
			t.Fatalf("runConfigProfiles: %v", err)
		}
	})

	if !strings.Contains(out, "daily") {
		t.Errorf("expected 'daily' profile in output, got: %q", out)
	}
	if !strings.Contains(out, "1on1") {
		t.Errorf("expected '1on1' profile in output, got: %q", out)
	}
	// Output should mention the count.
	if !strings.Contains(out, "2 profile") {
		t.Errorf("expected count '2 profile(s)' in output, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// runConfigAddProfile
// ---------------------------------------------------------------------------

func TestRunConfigAddProfile_CreateNew(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Set package-level flags used by the command handler.
	profileTitle = "Sprint Planning"
	profileParticipants = "Alice,Bob"
	profileKeywords = "sprint,velocity"
	profileLanguage = "en"
	defer func() {
		profileTitle = ""
		profileParticipants = ""
		profileKeywords = ""
		profileLanguage = ""
	}()

	out := captureStdout(t, func() {
		if err := runConfigAddProfile(nil, []string{"sprint"}); err != nil {
			t.Fatalf("runConfigAddProfile: %v", err)
		}
	})

	if !strings.Contains(out, "sprint") {
		t.Errorf("output should confirm profile name, got: %q", out)
	}

	// Reload and verify.
	loaded, err := config.Load(config.ConfigPath())
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	p, ok := loaded.Profiles["sprint"]
	if !ok {
		t.Fatal("profile 'sprint' was not persisted")
	}
	if p.Title != "Sprint Planning" {
		t.Errorf("profile title = %q; want %q", p.Title, "Sprint Planning")
	}
	if len(p.Participants) != 2 {
		t.Errorf("profile participants = %v; want [Alice Bob]", p.Participants)
	}
}

func TestRunConfigAddProfile_UpdateExisting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.Profile{
		"daily": {Title: "Old Title", Language: "en"},
	}
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	profileTitle = "New Title"
	profileParticipants = ""
	profileKeywords = ""
	profileLanguage = ""
	defer func() {
		profileTitle = ""
	}()

	captureStdout(t, func() {
		if err := runConfigAddProfile(nil, []string{"daily"}); err != nil {
			t.Fatalf("runConfigAddProfile: %v", err)
		}
	})

	loaded, err := config.Load(config.ConfigPath())
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	p := loaded.Profiles["daily"]
	if p.Title != "New Title" {
		t.Errorf("profile title = %q; want %q", p.Title, "New Title")
	}
}

// ---------------------------------------------------------------------------
// runConfigSet — validation warning branches
// ---------------------------------------------------------------------------

func TestRunConfigSet_APIKeyWarningDisplayed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Setting a literal API key (not a ${VAR} ref) should print a warning.
	out := captureStdout(t, func() {
		_ = runConfigSet(nil, []string{"deepgram.api_key", "literal_key_no_env_ref"})
	})

	if !strings.Contains(out, "Warning") {
		t.Errorf("expected warning about literal API key, got: %q", out)
	}
}

func TestRunConfigSet_LanguageValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runConfigSet(nil, []string{"deepgram.language", "xx_unknown"})
	})

	if !strings.Contains(out, "Warning") {
		t.Errorf("expected language warning, got: %q", out)
	}
}

func TestRunConfigSet_ModelValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runConfigSet(nil, []string{"claude.model", "gpt-4o-not-a-claude-model"})
	})

	if !strings.Contains(out, "Warning") {
		t.Errorf("expected model warning, got: %q", out)
	}
}

func TestRunConfigSet_VaultPathValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runConfigSet(nil, []string{"obsidian.vault_path", "/absolutely/nonexistent/path/xyz"})
	})

	if !strings.Contains(out, "Warning") {
		t.Errorf("expected vault path warning, got: %q", out)
	}
}

func TestRunConfigSet_UnknownKey_Error(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := runConfigSet(nil, []string{"totally.bogus.key", "value"})
	if err == nil {
		t.Error("runConfigSet with unknown key should return error")
	}
}

// ---------------------------------------------------------------------------
// runList
// ---------------------------------------------------------------------------

// setupVaultWithMeetings creates a temporary vault directory with the given
// meeting note files at the specified date-subdirectory paths and returns the
// vault path and a fully seeded config.
func setupVaultWithMeetings(t *testing.T, notes map[string]string) (vaultPath string) {
	t.Helper()
	tmp := t.TempDir()
	vaultPath = filepath.Join(tmp, "vault")
	meetingsDir := filepath.Join(vaultPath, "meetings")

	for rel, content := range notes {
		full := filepath.Join(meetingsDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	return vaultPath
}

func TestRunList_NoVaultConfigured(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = "" // explicitly no vault
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := runList(listCmd, nil)
	if err == nil {
		t.Fatal("expected error when vault not configured")
	}
	if !strings.Contains(err.Error(), "vault_path") {
		t.Errorf("error should mention vault_path, got: %q", err)
	}
}

func TestRunList_EmptyMeetingsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	vaultPath := setupVaultWithMeetings(t, map[string]string{})

	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = vaultPath
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// meetings dir does not exist → "No meetings found."
	out := captureStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})

	if !strings.Contains(out, "No meetings found") {
		t.Errorf("expected 'No meetings found', got: %q", out)
	}
}

func TestRunList_WithMeetings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	vaultPath := setupVaultWithMeetings(t, map[string]string{
		"2026-01-15/Sprint Planning.md": "# Sprint Planning\n",
		"2026-02-10/1-on-1.md":          "# 1-on-1\n",
	})

	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = vaultPath
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		listSince = ""
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})

	if !strings.Contains(out, "Sprint Planning") {
		t.Errorf("expected 'Sprint Planning' in output, got: %q", out)
	}
	if !strings.Contains(out, "1-on-1") {
		t.Errorf("expected '1-on-1' in output, got: %q", out)
	}
	if !strings.Contains(out, "2 meeting") {
		t.Errorf("expected meeting count in output, got: %q", out)
	}
}

func TestRunList_SinceFilter(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	vaultPath := setupVaultWithMeetings(t, map[string]string{
		"2026-01-15/OldMeeting.md": "# Old\n",
		"2026-03-20/NewMeeting.md": "# New\n",
	})

	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = vaultPath
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out := captureStdout(t, func() {
		listSince = "2026-02-01"
		defer func() { listSince = "" }()
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})

	if strings.Contains(out, "OldMeeting") {
		t.Errorf("OldMeeting should be filtered out by --since, got: %q", out)
	}
	if !strings.Contains(out, "NewMeeting") {
		t.Errorf("NewMeeting should be in output, got: %q", out)
	}
}

func TestRunList_InvalidSinceFormat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	vaultPath := setupVaultWithMeetings(t, map[string]string{})
	// Create the meetings dir so we get past the vault check.
	if err := os.MkdirAll(filepath.Join(vaultPath, "meetings"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = vaultPath
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	listSince = "not-a-date"
	defer func() { listSince = "" }()

	err := runList(listCmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --since format")
	}
	if !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error should mention expected format, got: %q", err)
	}
}

// ---------------------------------------------------------------------------
// runRecover — no recovery files path (safe to call; reads ~/.heimdall/recovery)
// ---------------------------------------------------------------------------

func TestRunRecover_NoFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Ensure the recovery dir doesn't exist (empty home → no ~/.heimdall dir).
	out := captureStdout(t, func() {
		if err := runRecover(nil, nil); err != nil {
			t.Fatalf("runRecover: %v", err)
		}
	})

	if !strings.Contains(out, "No recovery files") {
		t.Errorf("expected 'No recovery files' message, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// runRecord — fast-fail validation paths (no audio devices or network)
// ---------------------------------------------------------------------------

func TestRunRecord_InvalidTranscriberFlag(t *testing.T) {
	// Reset flag to avoid pollution across tests.
	origTranscriber := recordTranscriber
	defer func() { recordTranscriber = origTranscriber }()

	recordTranscriber = "unknown_provider"
	err := runRecord(recordCmd, nil)
	if err == nil {
		t.Fatal("expected error for unknown --transcriber value")
	}
	if !strings.Contains(err.Error(), "unknown_provider") {
		t.Errorf("error should mention the bad provider name, got: %q", err)
	}
}

func TestRunRecord_MissingDeepgramKey(t *testing.T) {
	origTranscriber := recordTranscriber
	defer func() { recordTranscriber = origTranscriber }()

	recordTranscriber = "deepgram"
	// Ensure the env var is absent.
	t.Setenv("DEEPGRAM_API_KEY", "")

	err := runRecord(recordCmd, nil)
	if err == nil {
		t.Fatal("expected error when DEEPGRAM_API_KEY is not set")
	}
	if !strings.Contains(err.Error(), "DEEPGRAM_API_KEY") {
		t.Errorf("error should mention DEEPGRAM_API_KEY, got: %q", err)
	}
}

func TestRunRecord_MissingSonioxKey(t *testing.T) {
	origTranscriber := recordTranscriber
	defer func() { recordTranscriber = origTranscriber }()

	recordTranscriber = "soniox"
	t.Setenv("SONIOX_API_KEY", "")

	err := runRecord(recordCmd, nil)
	if err == nil {
		t.Fatal("expected error when SONIOX_API_KEY is not set")
	}
	if !strings.Contains(err.Error(), "SONIOX_API_KEY") {
		t.Errorf("error should mention SONIOX_API_KEY, got: %q", err)
	}
}
