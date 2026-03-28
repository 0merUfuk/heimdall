package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: ObsidianWriter must satisfy Writer.
var _ Writer = (*ObsidianWriter)(nil)

// testNote returns a fully populated MeetingNote for testing.
func testNote() *heimdall.MeetingNote {
	return &heimdall.MeetingNote{
		Title:    "Sprint Planning",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 47*time.Minute + 23*time.Second,
		Summary:  "Team discussed sprint goals and assigned tasks for the upcoming iteration.",
		Decisions: []heimdall.Decision{
			{Description: "Adopt gRPC for internal services", DecidedBy: "Omer"},
			{Description: "Defer database migration to next sprint", DecidedBy: "Sarah"},
		},
		ActionItems: []heimdall.ActionItem{
			{Task: "Draft API specification", Owner: "Omer", Deadline: "2026-04-01", Priority: "high"},
			{Task: "Set up CI pipeline", Owner: "Mike", Deadline: "2026-04-03", Priority: "medium"},
		},
		Topics: []heimdall.Topic{
			{Title: "API Design", Content: "Team evaluated REST vs gRPC. Decided on gRPC for internal services."},
			{Title: "Sprint Goals", Content: "Three main goals identified for the sprint."},
		},
		Followups: []heimdall.Followup{
			{Question: "Performance benchmarks for gRPC?", RaisedBy: "Sarah"},
			{Question: "CI provider options?", RaisedBy: "Mike"},
		},
		SpeakerMap: map[int]string{
			0: "Omer",
			1: "Sarah",
			2: "Mike",
		},
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Let's start with the sprint review.", Start: 72 * time.Second, IsFinal: true},
			{Speaker: 1, Text: "I think we should adopt gRPC.", Start: 78 * time.Second, IsFinal: true},
			{Speaker: 2, Text: "Agreed, I'll set up the CI pipeline.", Start: 85 * time.Second, IsFinal: true},
		},
		Participants: []heimdall.Participant{
			{Name: "Omer"},
			{Name: "Sarah"},
			{Name: "Mike"},
		},
		MeetingType: "planning",
		Platform:    "Zoom",
	}
}

// newTestWriter creates an ObsidianWriter backed by a temporary vault directory.
// It returns the writer and the vault path. The caller is responsible for cleaning
// up the temp directory (t.TempDir() handles this automatically).
func newTestWriter(t *testing.T) (*ObsidianWriter, string) {
	t.Helper()

	vaultPath := t.TempDir()
	w, err := NewObsidianWriter(vaultPath, "meetings", "")
	if err != nil {
		t.Fatalf("NewObsidianWriter: unexpected error: %v", err)
	}
	return w, vaultPath
}

// --- Constructor tests ---

func TestNewObsidianWriter_Valid(t *testing.T) {
	vaultPath := t.TempDir()

	w, err := NewObsidianWriter(vaultPath, "meetings", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil writer")
	}
}

func TestNewObsidianWriter_DefaultMeetingsFolder(t *testing.T) {
	vaultPath := t.TempDir()

	w, err := NewObsidianWriter(vaultPath, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.meetingsFolder != "meetings" {
		t.Errorf("meetingsFolder: got %q, want %q", w.meetingsFolder, "meetings")
	}
}

func TestNewObsidianWriter_EmptyVaultPath(t *testing.T) {
	_, err := NewObsidianWriter("", "meetings", "")
	if err == nil {
		t.Fatal("expected error for empty vault path")
	}
	if !strings.Contains(err.Error(), "vault path is required") {
		t.Errorf("error message: got %q, want mention of 'vault path is required'", err.Error())
	}
}

func TestNewObsidianWriter_NonExistentVaultPath(t *testing.T) {
	_, err := NewObsidianWriter("/nonexistent/vault/path", "meetings", "")
	if err == nil {
		t.Fatal("expected error for non-existent vault path")
	}
	if !strings.Contains(err.Error(), "vault not found") {
		t.Errorf("error message: got %q, want mention of 'vault not found'", err.Error())
	}
}

func TestNewObsidianWriter_VaultPathIsFile(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewObsidianWriter(filePath, "meetings", "")
	if err == nil {
		t.Fatal("expected error when vault path is a file, not directory")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error message: got %q, want mention of 'not a directory'", err.Error())
	}
}

func TestNewObsidianWriter_CustomTemplatePath(t *testing.T) {
	vaultPath := t.TempDir()
	tmplPath := filepath.Join(t.TempDir(), "custom.md.tmpl")

	customTmpl := `# {{.Title}}`
	if err := os.WriteFile(tmplPath, []byte(customTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewObsidianWriter(vaultPath, "meetings", tmplPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil writer")
	}
}

func TestNewObsidianWriter_InvalidCustomTemplate(t *testing.T) {
	vaultPath := t.TempDir()
	tmplPath := filepath.Join(t.TempDir(), "bad.md.tmpl")

	badTmpl := `{{.Title`
	if err := os.WriteFile(tmplPath, []byte(badTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewObsidianWriter(vaultPath, "meetings", tmplPath)
	if err == nil {
		t.Fatal("expected error for invalid custom template")
	}
}

func TestNewObsidianWriter_MissingCustomTemplate(t *testing.T) {
	vaultPath := t.TempDir()
	_, err := NewObsidianWriter(vaultPath, "meetings", "/nonexistent/template.md.tmpl")
	if err == nil {
		t.Fatal("expected error for missing custom template")
	}
}

// --- Rendering tests ---

func TestWrite_FullNote(t *testing.T) {
	w, vaultPath := newTestWriter(t)
	note := testNote()

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	// Verify the file was created in the correct location.
	expectedDir := filepath.Join(vaultPath, "meetings", "2026-03-28")
	if !strings.HasPrefix(path, expectedDir) {
		t.Errorf("path prefix: got %q, want prefix %q", path, expectedDir)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Errorf("path suffix: got %q, want .md suffix", path)
	}

	// Read the rendered file.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}
	rendered := string(content)

	// Verify YAML frontmatter.
	if !strings.Contains(rendered, "date: 2026-03-28") {
		t.Error("missing or incorrect date in frontmatter")
	}
	if !strings.Contains(rendered, "type: meeting") {
		t.Error("missing type in frontmatter")
	}
	if !strings.Contains(rendered, `title: "Sprint Planning"`) {
		t.Error("missing title in frontmatter")
	}
	if !strings.Contains(rendered, "duration: 47m") {
		t.Error("missing or incorrect duration in frontmatter")
	}
	if !strings.Contains(rendered, "platform: Zoom") {
		t.Error("missing platform in frontmatter")
	}
	if !strings.Contains(rendered, "- planning") {
		t.Error("missing meeting type tag in frontmatter")
	}

	// Verify wikilinked participants in frontmatter.
	if !strings.Contains(rendered, `"[[Omer]]"`) {
		t.Error("missing wikilinked participant: Omer")
	}
	if !strings.Contains(rendered, `"[[Sarah]]"`) {
		t.Error("missing wikilinked participant: Sarah")
	}
	if !strings.Contains(rendered, `"[[Mike]]"`) {
		t.Error("missing wikilinked participant: Mike")
	}

	// Verify summary.
	if !strings.Contains(rendered, "Team discussed sprint goals") {
		t.Error("missing summary content")
	}

	// Verify decisions with wikilinks and tags.
	if !strings.Contains(rendered, "Adopt gRPC for internal services -- decided by [[Omer]] #decision") {
		t.Error("missing or malformed decision")
	}

	// Verify action items table.
	if !strings.Contains(rendered, "| Draft API specification | [[Omer]] | 2026-04-01 | high |") {
		t.Error("missing or malformed action item row")
	}

	// Verify discussion topics.
	if !strings.Contains(rendered, "### API Design") {
		t.Error("missing discussion topic heading")
	}

	// Verify follow-ups.
	if !strings.Contains(rendered, "- [ ] Performance benchmarks for gRPC? (raised by [[Sarah]])") {
		t.Error("missing or malformed follow-up")
	}

	// Verify collapsible transcript section.
	if !strings.Contains(rendered, "<details>") {
		t.Error("missing collapsible details tag")
	}
	if !strings.Contains(rendered, "Click to expand full transcript") {
		t.Error("missing transcript summary text")
	}

	// Verify transcript with speaker names from SpeakerMap.
	if !strings.Contains(rendered, "**Omer** (01:12): Let's start with the sprint review.") {
		t.Error("missing or malformed transcript entry for Omer")
	}
	if !strings.Contains(rendered, "**Sarah** (01:18): I think we should adopt gRPC.") {
		t.Error("missing or malformed transcript entry for Sarah")
	}
}

func TestWrite_EmptyFields(t *testing.T) {
	w, _ := newTestWriter(t)
	note := &heimdall.MeetingNote{
		Title:    "Empty Meeting",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 5 * time.Minute,
		Platform: "Google Meet",
	}

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}
	rendered := string(content)

	// Verify sections render cleanly with no data.
	if !strings.Contains(rendered, "## Key Decisions") {
		t.Error("missing Key Decisions section")
	}
	if !strings.Contains(rendered, "## Action Items") {
		t.Error("missing Action Items section")
	}
	if !strings.Contains(rendered, "## Follow-ups") {
		t.Error("missing Follow-ups section")
	}

	// Meeting type tag should not appear when MeetingType is empty.
	lines := strings.Split(rendered, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "- meeting" {
			// The "meeting" tag is expected.
			continue
		}
		// There should be no empty tag line after "- meeting".
		if trimmed == "-" {
			t.Error("found empty tag line (likely from empty MeetingType)")
		}
	}
}

func TestWrite_NilNote(t *testing.T) {
	w, _ := newTestWriter(t)

	_, err := w.Write(nil)
	if err == nil {
		t.Fatal("expected error for nil note")
	}
	if !strings.Contains(err.Error(), "note is nil") {
		t.Errorf("error message: got %q, want mention of 'note is nil'", err.Error())
	}
}

func TestWrite_DirectoryCreation(t *testing.T) {
	w, vaultPath := newTestWriter(t)
	note := testNote()

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	// Verify the date directory was created.
	expectedDir := filepath.Join(vaultPath, "meetings", "2026-03-28")
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("date directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected date directory, got file")
	}

	// Verify file is inside the date directory.
	if filepath.Dir(path) != expectedDir {
		t.Errorf("file directory: got %q, want %q", filepath.Dir(path), expectedDir)
	}
}

// --- Filename sanitization tests ---

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple title", "Sprint Planning", "sprint-planning"},
		{"mixed case", "Design Review", "design-review"},
		{"special characters", "1:1 with Sarah!", "11-with-sarah"},
		{"multiple spaces", "Team   Sync   Meeting", "team-sync-meeting"},
		{"leading/trailing spaces", "  Sprint Review  ", "sprint-review"},
		{"unicode characters", "Release v2.0 (Beta)", "release-v20-beta"},
		{"all special chars", "!!!@@@###", "meeting"},
		{"empty string", "", "meeting"},
		{"only spaces", "   ", "meeting"},
		{"hyphens preserved", "sprint-planning", "sprint-planning"},
		{"consecutive hyphens collapsed", "sprint---planning", "sprint-planning"},
		{"numbers", "Q1 2026 Review", "q1-2026-review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Collision prevention tests (V-017) ---

func TestWrite_CollisionPrevention(t *testing.T) {
	w, vaultPath := newTestWriter(t)
	note := testNote()

	// Write the first note.
	path1, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write(1): unexpected error: %v", err)
	}
	if !strings.HasSuffix(path1, "sprint-planning.md") {
		t.Errorf("first file: got %q, want suffix 'sprint-planning.md'", path1)
	}

	// Write a second note with the same title and date.
	path2, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write(2): unexpected error: %v", err)
	}
	if !strings.HasSuffix(path2, "sprint-planning-2.md") {
		t.Errorf("second file: got %q, want suffix 'sprint-planning-2.md'", path2)
	}

	// Write a third.
	path3, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write(3): unexpected error: %v", err)
	}
	if !strings.HasSuffix(path3, "sprint-planning-3.md") {
		t.Errorf("third file: got %q, want suffix 'sprint-planning-3.md'", path3)
	}

	// Verify all three files exist and are distinct.
	dir := filepath.Join(vaultPath, "meetings", "2026-03-28")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("file count: got %d, want 3", len(entries))
	}
}

func TestResolveCollision_NoExisting(t *testing.T) {
	dir := t.TempDir()
	path, err := resolveCollision(dir, "sprint-planning")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "sprint-planning.md") {
		t.Errorf("path: got %q, want suffix 'sprint-planning.md'", path)
	}
}

func TestResolveCollision_ExistingFile(t *testing.T) {
	dir := t.TempDir()

	// Create the first file.
	if err := os.WriteFile(filepath.Join(dir, "meeting.md"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := resolveCollision(dir, "meeting")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "meeting-2.md") {
		t.Errorf("path: got %q, want suffix 'meeting-2.md'", path)
	}
}

// --- Atomic write tests ---

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-note.md")
	data := []byte("# Test Meeting\n\nThis is a test.")

	if err := atomicWrite(path, data); err != nil {
		t.Fatalf("atomicWrite: unexpected error: %v", err)
	}

	// Verify file exists with correct content.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", string(content), string(data))
	}

	// Verify no temp files remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".heimdall-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	// Write initial content.
	if err := atomicWrite(path, []byte("original")); err != nil {
		t.Fatal(err)
	}

	// Overwrite with new content.
	if err := atomicWrite(path, []byte("updated")); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "updated" {
		t.Errorf("content: got %q, want %q", string(content), "updated")
	}
}

// --- Template function tests ---

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0m"},
		{"minutes only", 23 * time.Minute, "23m"},
		{"hours and minutes", time.Hour + 23*time.Minute, "1h 23m"},
		{"hours only", 2 * time.Hour, "2h 0m"},
		{"sub-minute", 30 * time.Second, "0m"},
		{"typical meeting", 47*time.Minute + 23*time.Second, "47m"},
		{"long meeting", 2*time.Hour + 15*time.Minute + 45*time.Second, "2h 15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v): got %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestSpeakerName(t *testing.T) {
	speakerMap := map[int]string{
		0: "Alice",
		1: "Bob",
		2: "Charlie",
	}

	tests := []struct {
		name    string
		speaker int
		want    string
	}{
		{"mapped speaker 0", 0, "Alice"},
		{"mapped speaker 1", 1, "Bob"},
		{"mapped speaker 2", 2, "Charlie"},
		{"unmapped speaker", 3, "Speaker 3"},
		{"unmapped speaker high ID", 99, "Speaker 99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := speakerName(tt.speaker, speakerMap)
			if got != tt.want {
				t.Errorf("speakerName(%d, map): got %q, want %q", tt.speaker, got, tt.want)
			}
		})
	}
}

func TestSpeakerName_NilMap(t *testing.T) {
	got := speakerName(0, nil)
	if got != "Speaker 0" {
		t.Errorf("speakerName with nil map: got %q, want %q", got, "Speaker 0")
	}
}

func TestSpeakerName_EmptyMap(t *testing.T) {
	got := speakerName(0, map[int]string{})
	if got != "Speaker 0" {
		t.Errorf("speakerName with empty map: got %q, want %q", got, "Speaker 0")
	}
}

// --- Vault path validation tests (V-016) ---

func TestWrite_VaultPathValidation(t *testing.T) {
	_, err := NewObsidianWriter("/definitely/not/a/real/vault", "meetings", "")
	if err == nil {
		t.Fatal("expected error for non-existent vault path")
	}
	if !strings.Contains(err.Error(), "vault not found") {
		t.Errorf("error should mention 'vault not found', got: %v", err)
	}
}

// --- expandHome tests ---

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("getting home dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"absolute path", "/Users/alice/vault", "/Users/alice/vault", false},
		{"relative path", "vault", "vault", false},
		{"home only", "~", home, false},
		{"home with subpath", "~/Documents/vault", filepath.Join(home, "Documents/vault"), false},
		{"tilde user (unsupported)", "~alice/vault", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandHome(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expandHome(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Integration-style tests ---

func TestWrite_CustomMeetingsFolder(t *testing.T) {
	vaultPath := t.TempDir()
	w, err := NewObsidianWriter(vaultPath, "notes/meetings", "")
	if err != nil {
		t.Fatalf("NewObsidianWriter: %v", err)
	}

	note := testNote()
	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	expectedDir := filepath.Join(vaultPath, "notes/meetings", "2026-03-28")
	if !strings.HasPrefix(path, expectedDir) {
		t.Errorf("path: got %q, want prefix %q", path, expectedDir)
	}
}

func TestWrite_SpeakerMapFallback(t *testing.T) {
	w, _ := newTestWriter(t)
	note := &heimdall.MeetingNote{
		Title:    "Unmapped Speakers",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 10 * time.Minute,
		Platform: "Zoom",
		SpeakerMap: map[int]string{
			0: "Alice",
			// Speaker 1 is not mapped.
		},
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Hello from Alice.", Start: 10 * time.Second, IsFinal: true},
			{Speaker: 1, Text: "Hello from unknown.", Start: 15 * time.Second, IsFinal: true},
		},
	}

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(content)

	if !strings.Contains(rendered, "**Alice**") {
		t.Error("mapped speaker should appear as 'Alice'")
	}
	if !strings.Contains(rendered, "**Speaker 1**") {
		t.Error("unmapped speaker should fall back to 'Speaker 1'")
	}
}

func TestWrite_NilSpeakerMap(t *testing.T) {
	w, _ := newTestWriter(t)
	note := &heimdall.MeetingNote{
		Title:      "No Speaker Map",
		Date:       time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration:   5 * time.Minute,
		Platform:   "Teams",
		SpeakerMap: nil,
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Hello.", Start: 10 * time.Second, IsFinal: true},
		},
	}

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "**Speaker 0**") {
		t.Error("with nil SpeakerMap, speaker should fall back to 'Speaker 0'")
	}
}

func TestWrite_LongDuration(t *testing.T) {
	w, _ := newTestWriter(t)
	note := &heimdall.MeetingNote{
		Title:    "Long Meeting",
		Date:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Duration: 3*time.Hour + 45*time.Minute,
		Platform: "Zoom",
	}

	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "3h 45m") {
		t.Error("long duration not formatted correctly")
	}
}

func TestWrite_CustomTemplate(t *testing.T) {
	vaultPath := t.TempDir()
	tmplDir := t.TempDir()
	tmplPath := filepath.Join(tmplDir, "custom.md.tmpl")

	customTmpl := `# {{.Title}}

Date: {{.Date.Format "2006-01-02"}}
Duration: {{formatDuration .Duration}}
`
	if err := os.WriteFile(tmplPath, []byte(customTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewObsidianWriter(vaultPath, "meetings", tmplPath)
	if err != nil {
		t.Fatalf("NewObsidianWriter: %v", err)
	}

	note := testNote()
	path, err := w.Write(note)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(content)

	if !strings.Contains(rendered, "# Sprint Planning") {
		t.Error("custom template not rendered correctly")
	}
	if strings.Contains(rendered, "## Summary") {
		t.Error("custom template should not contain default sections")
	}
}

// --- fileExists tests ---

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")

	// File does not exist.
	if fileExists(filePath) {
		t.Error("fileExists returned true for non-existent file")
	}

	// Create the file.
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// File exists.
	if !fileExists(filePath) {
		t.Error("fileExists returned false for existing file")
	}

	// Directory should return false.
	if fileExists(dir) {
		t.Error("fileExists returned true for directory")
	}
}
