package main

import "strings"

// secretKeys is the set of config keys whose values are API secrets and
// must be masked before any display (config get, config set echo, config show).
//
// Keep this list in sync with internal/config/config.go — every field that
// holds an API token MUST appear here. Missing an entry is a SEC-01 leak.
var secretKeys = map[string]struct{}{
	"deepgram.api_key": {},
	"soniox.api_key":   {},
	"claude.api_key":   {},
}

// isSecretKey reports whether a dotted config key path refers to an API
// credential that must be masked before display.
func isSecretKey(key string) bool {
	_, ok := secretKeys[key]
	return ok
}

// maskSecret returns a display-safe rendering of an API-key value.
//
// Rules:
//   - Empty string        → "" (nothing to leak).
//   - ${VAR} placeholder  → passthrough verbatim (not a secret; it's a ref).
//   - Shorter than 12     → fully masked as "********" (too short to safely
//                            reveal any suffix without leaking most of the key).
//   - Anthropic prefix    → first 7 chars + "..." + last 4 (e.g., "sk-ant-****...abc3").
//     The 7-char anchor preserves the "sk-ant-" prefix plus one extra character
//     so operators can confirm the key family at a glance.
//   - Other                → "****...abcd" (8 stars + "..." + last 4).
//
// Never log, echo, or display an API key through any path other than this
// helper.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}

	// Environment variable placeholders are config references, not secrets.
	// Preserve them verbatim so users see the reference they wrote.
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		return s
	}

	if len(s) < 12 {
		return "********"
	}

	last4 := s[len(s)-4:]

	if strings.HasPrefix(s, "sk-ant-") {
		// 7 chars of prefix + "****...{last4}" — preserves the "sk-ant-"
		// marker so operators recognize an Anthropic key at a glance.
		return s[:7] + "****..." + last4
	}

	return "****..." + last4
}
