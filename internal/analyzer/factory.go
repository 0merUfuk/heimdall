package analyzer

import "fmt"

// Provider names accepted by NewFromName. Kept as constants so the record
// command's --analyzer flag validation and error messages can reference the
// canonical spellings without string duplication (mirrors
// internal/transcriber/factory.go's ProviderDeepgram/ProviderSoniox pattern).
const (
	ProviderAPI        = "api"         // Anthropic Messages API, requires ANTHROPIC_API_KEY
	ProviderClaudeCode = "claude-code" // local `claude` CLI subprocess, uses the user's own Claude Code login
)

// NewFromName constructs an Analyzer by provider name. "api" (the default)
// requires apiKey to be non-empty -- an empty key produces a clear
// "configure ANTHROPIC_API_KEY" error instead of an ambiguous 401 mid-call.
// "claude-code" needs no key at all: it shells out to the user's own
// already-authenticated `claude` CLI, so an empty apiKey is fine on that
// path.
func NewFromName(name, apiKey string) (Analyzer, error) {
	switch name {
	case "", ProviderAPI:
		if apiKey == "" {
			return nil, fmt.Errorf("analyzer api: ANTHROPIC_API_KEY is not configured. Set it, or use --analyzer claude-code to analyze via a local Claude Code login instead")
		}
		return NewClaudeAnalyzer(apiKey), nil
	case ProviderClaudeCode:
		return NewClaudeCodeAnalyzer(), nil
	default:
		return nil, fmt.Errorf("unknown analyzer %q: valid options are %q, %q", name, ProviderAPI, ProviderClaudeCode)
	}
}
