// Package templates provides embedded template files for heimdall output rendering.
// Templates are embedded at compile time using go:embed directives.
package templates

import _ "embed"

// MeetingNoteTemplate is the default Obsidian meeting note template.
// It is rendered by the output.ObsidianWriter using Go text/template.
//
//go:embed meeting-note.md.tmpl
var MeetingNoteTemplate string
