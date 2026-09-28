package main

import (
	"strings"
	"testing"

	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveTranscriber(t *testing.T) {
	cfgWith := func(provider string) *config.Config {
		c := &config.Config{}
		c.Transcriber.Provider = provider
		return c
	}
	both := map[string]string{"SONIOX_API_KEY": "s", "DEEPGRAM_API_KEY": "d"}

	tests := []struct {
		name    string
		flag    string
		cfg     *config.Config
		env     map[string]string
		want    string
		wantWhy string
		wantErr string
	}{
		{name: "auto prefers soniox when both keys exist", cfg: nil, env: both, want: "soniox", wantWhy: "SONIOX_API_KEY is set"},
		{name: "auto falls back to deepgram", env: map[string]string{"DEEPGRAM_API_KEY": "d"}, want: "deepgram", wantWhy: "falling back to Deepgram"},
		{name: "auto soniox only", env: map[string]string{"SONIOX_API_KEY": "s"}, want: "soniox"},
		{name: "nothing configured lists the options", env: nil, wantErr: "--transcriber whisper"},
		{name: "flag beats config and keys", flag: "whisper", cfg: cfgWith("soniox"), env: both, want: "whisper", wantWhy: "flag"},
		{name: "config beats auto", cfg: cfgWith("deepgram"), env: both, want: "deepgram", wantWhy: "config"},
		{name: "explicit deepgram is not swapped for soniox", flag: "deepgram", env: map[string]string{"SONIOX_API_KEY": "s"}, want: "deepgram"},
		{name: "invalid flag", flag: "elevenlabs", env: both, wantErr: "not valid"},
		{name: "invalid config value", cfg: cfgWith("nope"), env: both, wantErr: "transcriber.provider"},
		{name: "unresolved env ref in config is ignored", cfg: cfgWith("${TRANSCRIBER}"), env: map[string]string{"SONIOX_API_KEY": "s"}, want: "soniox"},
		{name: "empty flag value means auto", flag: "", env: map[string]string{"SONIOX_API_KEY": "s"}, want: "soniox"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTranscriber(tc.flag, tc.cfg, envOf(tc.env))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tc.want {
				t.Errorf("provider = %q, want %q", got.Name, tc.want)
			}
			if tc.wantWhy != "" && !strings.Contains(got.Why, tc.wantWhy) {
				t.Errorf("why = %q, want it to contain %q", got.Why, tc.wantWhy)
			}
		})
	}
}

// TestTranscriberProviderNames_MatchConfig keeps the provider names that
// internal/config duplicates (it cannot import the transcriber package) in
// sync with the canonical constants.
func TestTranscriberProviderNames_MatchConfig(t *testing.T) {
	base := func(provider string) *config.Config {
		c := config.DefaultConfig()
		c.Deepgram.APIKey = "k" // Validate still requires one
		c.Obsidian.VaultPath = "/tmp/vault"
		c.Claude.Analyzer = "ollama" // no Anthropic key needed
		c.Transcriber.Provider = provider
		return c
	}
	for _, name := range []string{transcriber.ProviderSoniox, transcriber.ProviderDeepgram, transcriber.ProviderWhisper} {
		c := base(name)
		if err := c.Validate(); err != nil {
			t.Errorf("config rejects transcriber.provider %q: %v", name, err)
		}
		if !validTranscriberProvider(name) {
			t.Errorf("validTranscriberProvider(%q) = false", name)
		}
	}
	if err := base("elevenlabs").Validate(); err == nil {
		t.Error("config accepted an unknown transcriber.provider")
	}
}

func TestSetConfigValue_TranscriberProvider(t *testing.T) {
	cfg := &config.Config{}
	if err := setConfigValue(cfg, "transcriber.provider", "soniox"); err != nil || cfg.Transcriber.Provider != "soniox" {
		t.Fatalf("set soniox: err=%v provider=%q", err, cfg.Transcriber.Provider)
	}
	if err := setConfigValue(cfg, "transcriber.provider", "elevenlabs"); err == nil {
		t.Error("setting an unknown provider should fail")
	}
}
