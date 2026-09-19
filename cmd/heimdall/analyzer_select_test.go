package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/recovery"
)

// newAnalyzerFlagCmd returns a throwaway command with an --analyzer flag,
// optionally set on the "command line".
func newAnalyzerFlagCmd(t *testing.T, set string) (*cobra.Command, *string) {
	t.Helper()
	var v string
	c := &cobra.Command{Use: "x"}
	c.Flags().StringVar(&v, "analyzer", "api", "")
	if set != "" {
		if err := c.Flags().Set("analyzer", set); err != nil {
			t.Fatal(err)
		}
	}
	return c, &v
}

func TestResolveAnalyzerName(t *testing.T) {
	cfgOllama := config.DefaultConfig()
	cfgOllama.Claude.Analyzer = "ollama"
	cfgUnresolved := config.DefaultConfig()
	cfgUnresolved.Claude.Analyzer = "${HEIMDALL_ANALYZER}"

	t.Run("config default applies when flag not set", func(t *testing.T) {
		c, v := newAnalyzerFlagCmd(t, "")
		if got := resolveAnalyzerName(c, *v, cfgOllama); got != "ollama" {
			t.Errorf("got %q, want ollama", got)
		}
	})
	t.Run("explicit flag wins over config", func(t *testing.T) {
		c, v := newAnalyzerFlagCmd(t, "api")
		if got := resolveAnalyzerName(c, *v, cfgOllama); got != "api" {
			t.Errorf("got %q, want api", got)
		}
	})
	t.Run("nil cmd (tests, programmatic calls) still honors config", func(t *testing.T) {
		if got := resolveAnalyzerName(nil, "api", cfgOllama); got != "ollama" {
			t.Errorf("got %q, want ollama", got)
		}
	})
	t.Run("no config keeps flag default", func(t *testing.T) {
		if got := resolveAnalyzerName(nil, "api", nil); got != "api" {
			t.Errorf("got %q, want api", got)
		}
	})
	t.Run("unresolved env ref ignored", func(t *testing.T) {
		if got := resolveAnalyzerName(nil, "api", cfgUnresolved); got != "api" {
			t.Errorf("got %q, want api", got)
		}
	})
}

func TestValidateAnalyzerName(t *testing.T) {
	for _, ok := range []string{"", "api", "claude-code", "ollama", "codex"} {
		if err := validateAnalyzerName(ok); err != nil {
			t.Errorf("validateAnalyzerName(%q): unexpected %v", ok, err)
		}
	}
	err := validateAnalyzerName("gpt")
	if err == nil {
		t.Fatal("expected error for unknown analyzer")
	}
	for _, p := range analyzer.Providers() {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q should list %q", err, p)
		}
	}
}

func TestNewAnalyzerSetup_ModelResolution(t *testing.T) {
	cfg := config.DefaultConfig() // claude.model: claude-haiku-4-5
	cfg.Ollama.Model = "qwen2.5:7b"

	tests := []struct {
		name      string
		cfg       *config.Config
		wantModel string
	}{
		{"api", cfg, "claude-haiku-4-5"},
		{"api", nil, analyzer.DefaultModel},
		{"claude-code", cfg, "claude-haiku-4-5"},
		{"claude-code", nil, ""}, // user's own `claude` default, as before
		{"ollama", cfg, "qwen2.5:7b"},
		{"ollama", nil, analyzer.DefaultOllamaModel},
		{"codex", cfg, analyzer.DefaultCodexModel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup, err := newAnalyzerSetup(tt.name, tt.cfg, "sk-test")
			if err != nil {
				t.Fatalf("newAnalyzerSetup: %v", err)
			}
			if setup.Model != tt.wantModel {
				t.Errorf("model: got %q, want %q", setup.Model, tt.wantModel)
			}
			if setup.Timeout != analyzer.DefaultTimeoutFor(tt.name) {
				t.Errorf("timeout: got %v, want %v", setup.Timeout, analyzer.DefaultTimeoutFor(tt.name))
			}
		})
	}
}

func TestNewAnalyzerSetup_UnresolvedModelFallsBack(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Codex.Model = "${UNSET_CODEX_MODEL}"
	setup, err := newAnalyzerSetup("codex", cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if setup.Model != analyzer.DefaultCodexModel {
		t.Errorf("model: got %q, want default %q", setup.Model, analyzer.DefaultCodexModel)
	}
}

// TestNewAnalyzerSetup_OllamaOnDeviceLabel: the privacy claim printed to the
// user must be true -- a non-loopback ollama.base_url is called out as remote.
func TestNewAnalyzerSetup_OllamaOnDeviceLabel(t *testing.T) {
	local, err := newAnalyzerSetup("ollama", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !local.OnDevice || !strings.Contains(local.Label, "on-device") {
		t.Errorf("default ollama setup should be on-device, got %+v", local)
	}

	cfg := config.DefaultConfig()
	cfg.Ollama.BaseURL = "http://192.168.1.50:11434"
	remote, err := newAnalyzerSetup("ollama", cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if remote.OnDevice || !strings.Contains(remote.Label, "REMOTE") {
		t.Errorf("non-loopback ollama must be labeled remote, got %+v", remote)
	}
}

func TestNewAnalyzerSetup_Errors(t *testing.T) {
	if _, err := newAnalyzerSetup("api", nil, ""); err == nil {
		t.Error("api without a key should fail")
	}
	if _, err := newAnalyzerSetup("bogus", nil, "k"); err == nil {
		t.Error("unknown analyzer should fail")
	}
}

func TestSetConfigValue_AnalyzerBackendKeys(t *testing.T) {
	cfg := config.DefaultConfig()
	for key, value := range map[string]string{
		"claude.analyzer":    "ollama",
		"ollama.base_url":    "http://localhost:11434",
		"ollama.model":       "qwen3:14b",
		"ollama.max_context": "16384",
		"codex.model":        "gpt-5.6-luna",
	} {
		if err := setConfigValue(cfg, key, value); err != nil {
			t.Errorf("setConfigValue(%q, %q): %v", key, value, err)
		}
	}
	if cfg.Claude.Analyzer != "ollama" || cfg.Ollama.MaxContext != 16384 || cfg.Codex.Model != "gpt-5.6-luna" {
		t.Errorf("values not applied: claude=%+v ollama=%+v codex=%+v", cfg.Claude, cfg.Ollama, cfg.Codex)
	}

	if err := setConfigValue(cfg, "claude.analyzer", "chatgpt"); err == nil {
		t.Error("unknown claude.analyzer must be rejected at set time")
	}
	if err := setConfigValue(cfg, "ollama.max_context", "lots"); err == nil {
		t.Error("non-integer ollama.max_context must be rejected")
	}
}

func TestEvalFixtureTimeout(t *testing.T) {
	if evalFixtureTimeout("api") != 60*time.Second || evalFixtureTimeout("claude-code") != 60*time.Second {
		t.Error("api/claude-code must keep the original 60s per fixture")
	}
	if evalFixtureTimeout("ollama") < 2*time.Minute || evalFixtureTimeout("codex") < 2*time.Minute {
		t.Error("local/agent backends need a longer per-fixture budget")
	}
}

// fallbackAnalyzer always returns a V-009 fallback note, as an analyzer does
// after exhausting its retries.
type fallbackAnalyzer struct{}

func (fallbackAnalyzer) Summarize(_ context.Context, segs []heimdall.Segment, _ heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	return &heimdall.MeetingNote{Summary: "Analysis failed -- raw transcript included below (error: transcript needs ~45175 tokens but the local context limit is 32768)", Segments: segs, IsFallback: true, SpeakerMap: map[int]string{}}, nil
}

// okAnalyzer returns a normal analysis.
type okAnalyzer struct{}

func (okAnalyzer) Summarize(_ context.Context, segs []heimdall.Segment, _ heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	return &heimdall.MeetingNote{Summary: "A real summary.", Segments: segs, SpeakerMap: map[int]string{0: "Ana"}, MeetingType: "general"}, nil
}

// writeRecoveryFile creates a real recovery transcript under HOME.
func writeRecoveryFile(t *testing.T, title string) recovery.RecoveryFile {
	t.Helper()
	rw, err := recovery.NewRecoveryWriter(title, time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC), "en")
	if err != nil {
		t.Fatal(err)
	}
	rw.AddSegment(heimdall.Segment{Speaker: 0, Text: "Ana will send the report by Friday.", Start: time.Second, End: 3 * time.Second, IsFinal: true})
	if err := rw.Flush(); err != nil {
		t.Fatal(err)
	}
	rf, err := recovery.LoadRecoveryFile(rw.FilePath())
	if err != nil {
		t.Fatal(err)
	}
	return *rf
}

func vaultConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = t.TempDir()
	return cfg
}

// TestAnalyzeRecoveryFile_FallbackKeepsTranscript is the regression test for
// the pre-existing bug fixed in ID-011: a fallback note used to delete the
// recovery JSON, destroying the only re-analyzable copy of the meeting.
func TestAnalyzeRecoveryFile_FallbackKeepsTranscript(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rf := writeRecoveryFile(t, "fallback meeting")
	setup := &analyzerSetup{Name: "ollama", Analyzer: fallbackAnalyzer{}, Timeout: time.Second, Label: "Ollama (test)", OnDevice: true}

	out := captureStdout(t, func() {
		if err := analyzeRecoveryFile(rf, setup, vaultConfig(t)); err != nil {
			t.Errorf("analyzeRecoveryFile: %v", err)
		}
	})

	if _, err := os.Stat(rf.Path); err != nil {
		t.Fatalf("recovery file must be kept after a fallback note, stat: %v", err)
	}
	for _, want := range []string{"did not complete", "Reason: transcript needs ~45175 tokens but the local context limit is 32768\n", rf.Path, "--analyzer ollama", "only if you accept sending it to the cloud"} {
		if !strings.Contains(out, want) {
			t.Errorf("guidance missing %q in:\n%s", want, out)
		}
	}
}

func TestAnalyzeRecoveryFile_SuccessDeletesTranscript(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rf := writeRecoveryFile(t, "good meeting")
	setup := &analyzerSetup{Name: "api", Analyzer: okAnalyzer{}, Timeout: time.Second, Label: "test"}

	captureStdout(t, func() {
		if err := analyzeRecoveryFile(rf, setup, vaultConfig(t)); err != nil {
			t.Errorf("analyzeRecoveryFile: %v", err)
		}
	})
	if _, err := os.Stat(rf.Path); !os.IsNotExist(err) {
		t.Errorf("recovery file should be deleted after a successful analysis (stat err=%v)", err)
	}
}

// TestRunAnalyze_ConfigAnalyzerOllama drives `heimdall analyze --file` end to
// end with claude.analyzer: ollama set only in config (no flag) against a fake
// Ollama server, proving the config default now applies to analyze (it used
// to be honored by record only) and that no ANTHROPIC_API_KEY is needed.
func TestRunAnalyze_ConfigAnalyzerOllama(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_API_KEY", "")

	var chatCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{"model_info": map[string]any{"qwen3.context_length": 40960}})
		case "/api/chat":
			chatCalls++
			content := `{"speaker_map":{"0":"Ana"},"summary":"Ana owns the report.","decisions":[],"action_items":[{"task":"Send the report","owner":"Ana","deadline":"Friday","priority":"medium"}],"topics":[],"followups":[],"meeting_type":"general"}`
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"content": content}, "done_reason": "stop", "prompt_eval_count": 500, "eval_count": 80})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	vault := filepath.Join(home, "vault")
	if err := os.MkdirAll(vault, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Obsidian.VaultPath = vault
	cfg.Claude.Analyzer = "ollama"
	cfg.Ollama.BaseURL = srv.URL
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatal(err)
	}

	rf := writeRecoveryFile(t, "offline meeting")
	origFile, origAnalyzer := analyzeFile, recoverAnalyzer
	analyzeFile, recoverAnalyzer = rf.Path, "api" // flag default; config must win
	t.Cleanup(func() { analyzeFile, recoverAnalyzer = origFile, origAnalyzer })

	out := captureStdout(t, func() {
		if err := runAnalyze(nil, nil); err != nil {
			t.Errorf("runAnalyze: %v", err)
		}
	})

	if chatCalls != 1 {
		t.Fatalf("ollama chat calls: got %d, want 1\noutput:\n%s", chatCalls, out)
	}
	if !strings.Contains(out, "Meeting note saved") {
		t.Errorf("expected a saved note, got:\n%s", out)
	}
	notes, _ := filepath.Glob(filepath.Join(vault, "meetings", "*", "*.md"))
	if len(notes) != 1 {
		t.Fatalf("vault notes: got %d, want 1", len(notes))
	}
	body, _ := os.ReadFile(notes[0])
	if !strings.Contains(string(body), "Send the report") {
		t.Errorf("note does not contain the analyzed action item:\n%s", body)
	}
}

// TestConfigValidate_AcceptsEveryProvider guards the provider list that
// internal/config duplicates (to stay independent of internal/analyzer):
// every backend NewFromName accepts must also pass config validation.
func TestConfigValidate_AcceptsEveryProvider(t *testing.T) {
	for _, p := range analyzer.Providers() {
		cfg := config.DefaultConfig()
		cfg.Obsidian.VaultPath = "/tmp/vault"
		cfg.Claude.Analyzer = p
		if err := cfg.Validate(); err != nil {
			t.Errorf("claude.analyzer %q: config.Validate rejected a valid provider: %v", p, err)
		}
	}
}

// TestRunRecord_WhisperPreflight: offline capture must fail BEFORE the
// meeting starts (no devices opened, no consent prompt) when the local
// transcription it depends on is unusable, and must not require any
// transcription API key.
func TestRunRecord_WhisperPreflight(t *testing.T) {
	origT, origM := recordTranscriber, recordWhisperModel
	t.Cleanup(func() { recordTranscriber, recordWhisperModel = origT, origM })
	recordTranscriber = "whisper"
	recordWhisperModel = "base"
	t.Setenv("DEEPGRAM_API_KEY", "")
	t.Setenv("SONIOX_API_KEY", "")
	t.Setenv("HOME", t.TempDir())

	t.Run("whisper-cli missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		err := runRecord(recordCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "whisper-cli") {
			t.Fatalf("want a whisper-cli error, got %v", err)
		}
		if strings.Contains(err.Error(), "DEEPGRAM_API_KEY") {
			t.Errorf("offline capture must not ask for a Deepgram key: %v", err)
		}
	})

	t.Run("model missing", func(t *testing.T) {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "whisper-cli"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		err := runRecord(recordCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "heimdall model download base") {
			t.Fatalf("want a model-download error, got %v", err)
		}
	})
}
