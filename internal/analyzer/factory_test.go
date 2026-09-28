package analyzer

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewFromName(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		wantType any
		wantErr  string
	}{
		{"", Settings{APIKey: "sk-test"}, &ClaudeAnalyzer{}, ""},
		{ProviderAPI, Settings{APIKey: "sk-test"}, &ClaudeAnalyzer{}, ""},
		{ProviderAPI, Settings{}, nil, "ANTHROPIC_API_KEY is not configured"},
		{ProviderClaudeCode, Settings{}, &ClaudeCodeAnalyzer{}, ""},
		{ProviderOllama, Settings{}, &OllamaAnalyzer{}, ""},
		{ProviderCodex, Settings{}, &CodexAnalyzer{}, ""},
		{"gpt", Settings{}, nil, `unknown analyzer "gpt"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewFromName(tt.name, tt.settings)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err: got %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if reflect.TypeOf(a) != reflect.TypeOf(tt.wantType) {
				t.Errorf("type: got %T, want %T", a, tt.wantType)
			}
		})
	}
}

func TestNewFromName_UnknownListsEveryProvider(t *testing.T) {
	_, err := NewFromName("nope", Settings{})
	for _, p := range Providers() {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not list provider %q", err, p)
		}
	}
}

func TestNewFromName_OllamaSettings(t *testing.T) {
	a, err := NewFromName(ProviderOllama, Settings{OllamaBaseURL: "http://127.0.0.1:9999/", OllamaMaxContext: 16384})
	if err != nil {
		t.Fatal(err)
	}
	o := a.(*OllamaAnalyzer)
	if o.BaseURL() != "http://127.0.0.1:9999" || o.maxContext != 16384 {
		t.Errorf("settings not applied: baseURL=%q maxContext=%d", o.BaseURL(), o.maxContext)
	}
}

func TestProviderHelpers(t *testing.T) {
	for _, p := range []string{"", ProviderAPI, ProviderClaudeCode, ProviderOllama, ProviderCodex} {
		if !IsValidProvider(p) {
			t.Errorf("IsValidProvider(%q) = false", p)
		}
	}
	if IsValidProvider("openai") {
		t.Error(`IsValidProvider("openai") = true`)
	}

	wantKey := map[string]bool{"": true, ProviderAPI: true, ProviderClaudeCode: false, ProviderOllama: false, ProviderCodex: false}
	for p, want := range wantKey {
		if got := RequiresAPIKey(p); got != want {
			t.Errorf("RequiresAPIKey(%q) = %v, want %v", p, got, want)
		}
	}

	wantModel := map[string]string{
		"":                 DefaultModel,
		ProviderAPI:        DefaultModel,
		ProviderClaudeCode: "",
		ProviderOllama:     DefaultOllamaModel,
		ProviderCodex:      DefaultCodexModel,
	}
	for p, want := range wantModel {
		if got := DefaultModelFor(p); got != want {
			t.Errorf("DefaultModelFor(%q) = %q, want %q", p, got, want)
		}
	}

	// The cloud backends keep their long-standing 120s budget (no
	// regression); local/agent backends get longer.
	if DefaultTimeoutFor(ProviderAPI) != 120*time.Second || DefaultTimeoutFor(ProviderClaudeCode) != 120*time.Second {
		t.Error("api/claude-code timeout must stay 120s")
	}
	if DefaultTimeoutFor(ProviderOllama) <= DefaultTimeoutFor(ProviderAPI) || DefaultTimeoutFor(ProviderCodex) <= DefaultTimeoutFor(ProviderAPI) {
		t.Error("ollama/codex need a longer budget than the API")
	}
}
