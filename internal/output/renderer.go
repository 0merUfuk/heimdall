package output

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/templates"
)

// ObsidianWriter implements the Writer interface by rendering meeting notes
// as Obsidian-native markdown and writing them to a vault directory.
//
// Pipeline Stage 6 (RENDER) per PIPELINE.md. This stage has ONE job:
// templates to Obsidian vault markdown. No audio processing, no LLM calls.
//
// Mitigations:
//   - V-016: Vault path validation at construction time
//   - V-017: File naming collision prevention (numeric suffix)
//   - Atomic writes via temp file + os.Rename
type ObsidianWriter struct {
	vaultPath      string // absolute path to the Obsidian vault root
	meetingsFolder string // relative folder within vault (e.g., "meetings")
	templatePath   string // custom template path, or "" for embedded default
	tmpl           *template.Template
}

// templateFuncs returns the template function map used by the meeting note template.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDuration": formatDuration,
		"speakerName":    speakerName,
	}
}

// formatDuration converts a time.Duration to a human-readable format.
// Examples: "1h 23m", "45m", "0m".
func formatDuration(d time.Duration) string {
	total := int(d.Minutes())
	hours := total / 60
	minutes := total % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// speakerName maps a speaker ID to a display name using the provided speaker map.
// Falls back to "Speaker N" if the ID is not found in the map.
func speakerName(speaker int, speakerMap map[int]string) string {
	if name, ok := speakerMap[speaker]; ok {
		return name
	}
	return fmt.Sprintf("Speaker %d", speaker)
}

// maxCollisionAttempts limits the number of filename collision checks to prevent
// infinite loops in pathological cases.
const maxCollisionAttempts = 1000

// sanitizePattern matches characters that are not alphanumeric or hyphens.
var sanitizePattern = regexp.MustCompile(`[^a-z0-9-]+`)

// multiHyphenPattern matches consecutive hyphens.
var multiHyphenPattern = regexp.MustCompile(`-{2,}`)

// NewObsidianWriter creates a new ObsidianWriter that writes to the given vault.
//
// Parameters:
//   - vaultPath: absolute path to the Obsidian vault root directory (~ is expanded)
//   - meetingsFolder: relative folder name within the vault (e.g., "meetings")
//   - templatePath: path to a custom template, or "" to use the embedded default
//
// Returns an error if:
//   - vaultPath is empty
//   - vaultPath does not exist (V-016)
//   - a custom templatePath is provided but cannot be read or parsed
func NewObsidianWriter(vaultPath, meetingsFolder, templatePath string) (*ObsidianWriter, error) {
	if vaultPath == "" {
		return nil, fmt.Errorf("obsidian writer: vault path is required")
	}

	// Expand ~ to home directory.
	expanded, err := expandHome(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("obsidian writer: expanding vault path: %w", err)
	}

	// V-016: Validate vault path exists at construction time, not at render time.
	info, err := os.Stat(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("obsidian writer: vault not found at %s — set it with `heimdall config set obsidian.vault_path <path>`", expanded)
		}
		return nil, fmt.Errorf("obsidian writer: checking vault path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("obsidian writer: vault path %s is not a directory", expanded)
	}

	if meetingsFolder == "" {
		meetingsFolder = "meetings"
	}

	// Parse the template (custom or embedded default).
	tmpl, err := parseTemplate(templatePath)
	if err != nil {
		return nil, fmt.Errorf("obsidian writer: parsing template: %w", err)
	}

	return &ObsidianWriter{
		vaultPath:      expanded,
		meetingsFolder: meetingsFolder,
		templatePath:   templatePath,
		tmpl:           tmpl,
	}, nil
}

// Write renders the meeting note and writes it atomically to the Obsidian vault.
//
// File path format: {vault}/{meetingsFolder}/{YYYY-MM-DD}/{sanitized-title}.md
//
// Returns the full path of the written file. Implements Writer interface.
func (o *ObsidianWriter) Write(note *heimdall.MeetingNote) (string, error) {
	if note == nil {
		return "", fmt.Errorf("obsidian writer: note is nil")
	}

	// Render the template to a buffer.
	rendered, err := o.render(note)
	if err != nil {
		return "", fmt.Errorf("obsidian writer: rendering template: %w", err)
	}

	// Determine the output directory: {vault}/{meetingsFolder}/{YYYY-MM-DD}/
	dateDir := note.Date.Format("2006-01-02")
	outputDir := filepath.Join(o.vaultPath, o.meetingsFolder, dateDir)

	// M-003: Verify the output path does not escape the vault root.
	// A malicious meetingsFolder like "../../Library" would write outside the vault.
	cleanOutput := filepath.Clean(outputDir)
	cleanVault := filepath.Clean(o.vaultPath)
	if !strings.HasPrefix(cleanOutput, cleanVault+string(filepath.Separator)) && cleanOutput != cleanVault {
		return "", fmt.Errorf("obsidian writer: output path %s escapes vault root %s", cleanOutput, cleanVault)
	}

	// Create the directory tree if it does not exist.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("obsidian writer: creating output directory %s: %w", outputDir, err)
	}

	// Determine the filename with collision prevention (V-017).
	filename := sanitizeFilename(note.Title)
	outputPath, err := resolveCollision(outputDir, filename)
	if err != nil {
		return "", fmt.Errorf("obsidian writer: resolving filename: %w", err)
	}

	// Atomic write: write to temp file in the same directory, then rename.
	if err := atomicWrite(outputPath, rendered); err != nil {
		return "", fmt.Errorf("obsidian writer: writing file: %w", err)
	}

	return outputPath, nil
}

// render executes the template with the given MeetingNote and returns the rendered bytes.
func (o *ObsidianWriter) render(note *heimdall.MeetingNote) ([]byte, error) {
	var buf bytes.Buffer
	if err := o.tmpl.Execute(&buf, note); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}
	return buf.Bytes(), nil
}

// parseTemplate parses the meeting note template. If templatePath is empty,
// the embedded default template is used. Otherwise, the file at templatePath is read.
func parseTemplate(templatePath string) (*template.Template, error) {
	funcs := templateFuncs()

	if templatePath == "" {
		tmpl, err := template.New("meeting-note").Funcs(funcs).Parse(templates.MeetingNoteTemplate)
		if err != nil {
			return nil, fmt.Errorf("parsing embedded template: %w", err)
		}
		return tmpl, nil
	}

	// Custom template from file.
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading custom template %s: %w", templatePath, err)
	}
	tmpl, err := template.New("meeting-note").Funcs(funcs).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing custom template %s: %w", templatePath, err)
	}
	return tmpl, nil
}

// sanitizeFilename converts a meeting title to a filesystem-safe filename.
// Rules: lowercase, spaces to hyphens, remove special characters,
// collapse multiple hyphens, trim leading/trailing hyphens.
// Falls back to "meeting" if the result is empty after sanitization.
func sanitizeFilename(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = sanitizePattern.ReplaceAllString(s, "")
	s = multiHyphenPattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if s == "" {
		return "meeting"
	}
	return s
}

// resolveCollision finds a non-colliding filename in the given directory.
// If {dir}/{name}.md does not exist, it is returned.
// Otherwise, {dir}/{name}-2.md, {dir}/{name}-3.md, etc. are tried.
// V-017: Never silently overwrite an existing file.
func resolveCollision(dir, name string) (string, error) {
	candidate := filepath.Join(dir, name+".md")
	if !fileExists(candidate) {
		return candidate, nil
	}

	for i := 2; i <= maxCollisionAttempts; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d.md", name, i))
		if !fileExists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("too many files with name %q in %s (checked %d variants)", name, dir, maxCollisionAttempts)
}

// atomicWrite writes data to a file atomically by writing to a temp file in the
// same directory and then renaming. This prevents partial writes on crash.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)

	// Create temp file in the same directory to ensure rename works (same filesystem).
	tmp, err := os.CreateTemp(dir, ".heimdall-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on any error path.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write content.
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing to temp file: %w", err)
	}

	// L-001: Set explicit permissions instead of relying on umask.
	// Use 0644 because Obsidian notes should be readable by the user's editor.
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}

	// Sync to disk before rename to ensure data integrity.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Rename temp file to final path (atomic on POSIX).
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file to %s: %w", path, err)
	}

	success = true
	return nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	// Handle ~/... pattern.
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}

	// ~user/... patterns are not supported.
	return "", fmt.Errorf("unsupported home path pattern: %s", path)
}

// fileExists returns true if the file at path exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
