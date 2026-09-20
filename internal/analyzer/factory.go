package analyzer

import (
	"fmt"
	"strings"
	"time"
)

// Provider names accepted by NewFromName. Kept as constants so the record
// command's --analyzer flag validation and error messages can reference the
// canonical spellings without string duplication (mirrors
// internal/transcriber/factory.go's ProviderDeepgram/ProviderSoniox pattern).
const (
	ProviderAPI        = "api"         // Anthropic Messages API, requires ANTHROPIC_API_KEY
	ProviderClaudeCode = "claude-code" // local `claude` CLI subprocess, uses the user's own Claude Code login
	ProviderOllama     = "ollama"      // local Ollama server, on-device model -- no network, no account
	ProviderCodex      = "codex"       // local `codex` CLI subprocess, uses the user's own Codex (ChatGPT) login
)

// providers lists every valid provider name in display order.
var providers = []string{ProviderAPI, ProviderClaudeCode, ProviderOllama, ProviderCodex}

// Providers returns every valid provider name, in display order.
func Providers() []string {
	return append([]string(nil), providers...)
}

// IsValidProvider reports whether name selects a known backend. The empty
// string is valid and means the default (ProviderAPI).
func IsValidProvider(name string) bool {
	if name == "" {
		return true
	}
	for _, p := range providers {
		if p == name {
			return true
		}
	}
	return false
}

// ProviderList renders Providers() as `"api", "claude-code", ...` for error
// messages.
func ProviderList() string {
	quoted := make([]string, len(providers))
	for i, p := range providers {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(quoted, ", ")
}

// RequiresAPIKey reports whether the provider needs ANTHROPIC_API_KEY. Only
// the direct-API backend does; the other three reuse an existing local login
// or run fully on-device.
func RequiresAPIKey(name string) bool {
	return name == "" || name == ProviderAPI
}

// DefaultModelFor returns the model a provider uses when none is configured.
// Every default is the cheapest model that reliably produces the analysis
// JSON: Haiku for the Anthropic API, the smallest capable local model that
// fits a 24 GB Mac for Ollama, and the "fast and affordable" tier at low
// reasoning effort for Codex. claude-code returns "" so the user's own
// `claude` default applies unless they configure one explicitly.
func DefaultModelFor(name string) string {
	switch name {
	case "", ProviderAPI:
		return DefaultModel
	case ProviderOllama:
		return DefaultOllamaModel
	case ProviderCodex:
		return DefaultCodexModel
	default:
		return ""
	}
}

// DefaultTimeoutFor bounds one complete Summarize call (all retries
// included) for the provider. The cloud backends keep the long-standing
// 120s budget; the local model needs far longer (a 1-hour meeting through a
// 14B model on Apple Silicon is minutes, not seconds, plus a cold model
// load), and Codex runs an agent loop around the model call.
func DefaultTimeoutFor(name string) time.Duration {
	switch name {
	case ProviderOllama:
		return 20 * time.Minute
	case ProviderCodex:
		return 10 * time.Minute
	default:
		return 120 * time.Second
	}
}

// Settings carries the backend-specific configuration NewFromName needs.
// Zero values select each backend's defaults.
type Settings struct {
	// APIKey is the Anthropic API key (ProviderAPI only).
	APIKey string

	// OllamaBaseURL is the Ollama server root, e.g. http://localhost:11434
	// (ProviderOllama only). Empty selects DefaultOllamaBaseURL.
	OllamaBaseURL string

	// OllamaMaxContext caps the context window heimdall will request from
	// Ollama, in tokens (ProviderOllama only). Zero selects
	// DefaultOllamaMaxContext.
	OllamaMaxContext int
}

// NewFromName constructs an Analyzer by provider name. "api" (the default)
// requires s.APIKey to be non-empty -- an empty key produces a clear
// "configure ANTHROPIC_API_KEY" error instead of an ambiguous 401 mid-call.
// The other backends need no key at all.
func NewFromName(name string, s Settings) (Analyzer, error) {
	switch name {
	case "", ProviderAPI:
		if s.APIKey == "" {
			return nil, fmt.Errorf("analyzer api: ANTHROPIC_API_KEY is not configured. Set it, or use --analyzer claude-code / codex (existing local login) or --analyzer ollama (fully local) instead")
		}
		return NewClaudeAnalyzer(s.APIKey), nil
	case ProviderClaudeCode:
		return NewClaudeCodeAnalyzer(), nil
	case ProviderOllama:
		return NewOllamaAnalyzer(s.OllamaBaseURL, s.OllamaMaxContext), nil
	case ProviderCodex:
		return NewCodexAnalyzer(), nil
	default:
		return nil, fmt.Errorf("unknown analyzer %q: valid options are %s", name, ProviderList())
	}
}
