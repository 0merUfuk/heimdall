// Package output defines the Writer interface for meeting note output.
// The primary implementation is ObsidianWriter which renders meeting notes
// as Obsidian-native markdown with YAML frontmatter and writes them to
// the configured vault directory.
package output

import (
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Writer defines the interface for meeting note output writers.
// The primary implementation is ObsidianWriter (markdown template rendering + file write).
type Writer interface {
	// Write renders the meeting note and writes it to the output destination.
	// Returns the path of the written file, or an error if writing fails.
	// Implementations must write atomically (temp file -> rename) to prevent partial writes.
	Write(note *heimdall.MeetingNote) (string, error)
}
