package heimdall

import (
	"regexp"
	"strings"
)

// sanitizePattern strips everything except Unicode letters, Unicode digits,
// and hyphens. \p{L}/\p{N} (not the ASCII-only [a-zA-Z0-9]) so a title in
// Turkish, Arabic, CJK, or any other script keeps its actual characters
// instead of being reduced to a near-empty string of hyphens.
var sanitizePattern = regexp.MustCompile(`[^\p{L}\p{N}-]+`)

var multiHyphenPattern = regexp.MustCompile(`-{2,}`)

// SanitizeFilename converts a meeting title into a filesystem-safe filename
// stem (no extension): lowercased, spaces to hyphens, everything else
// stripped except Unicode letters/digits/hyphens, consecutive hyphens
// collapsed, leading/trailing hyphens trimmed. Returns fallback if nothing
// usable remains after sanitization (e.g. a title that is pure punctuation
// or emoji).
//
// This is the single source of truth for title-to-filename conversion --
// internal/output and internal/recovery both call it (with their own
// fallback string) rather than keeping parallel copies. They drifted once
// already: output/renderer.go picked up the Unicode-aware pattern fixing
// Turkish filenames (see .claude/KNOWN_ISSUES.md / CHANGELOG.md), but
// internal/recovery's copy was never updated and kept stripping Turkish
// characters from crash-recovery filenames.
func SanitizeFilename(title, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = sanitizePattern.ReplaceAllString(s, "")
	s = multiHyphenPattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if s == "" {
		return fallback
	}
	return s
}
