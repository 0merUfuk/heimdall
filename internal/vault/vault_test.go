package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// realNoteFixture mirrors the exact output shape of
// templates/meeting-note.md.tmpl (see internal/output/renderer.go) --
// frontmatter delimiters, participant wikilinks, section headers.
func realNoteFixture(date, title string, participants []string, body string) string {
	var b strings.Builder
	for _, p := range participants {
		b.WriteString("  - \"[[" + p + "]]\"\n")
	}
	participantsYAML := b.String()
	return "---\n" +
		"date: " + date + "\n" +
		"type: meeting\n" +
		"title: \"" + title + "\"\n" +
		"participants:\n" + participantsYAML +
		"duration: 30m0s\n" +
		"platform: desktop\n" +
		"tags:\n" +
		"  - meeting\n" +
		"---\n\n" +
		"# " + title + "\n\n" + body
}

// writeVault creates a temp vault with the given meetings, laid out exactly
// as internal/output/renderer.go writes them: {vault}/{meetingsFolder}/{date}/{title}.md
func writeVault(t *testing.T, meetingsFolder string, notes map[string]string) (vaultPath string) {
	t.Helper()
	vaultPath = t.TempDir()
	for relPath, content := range notes {
		full := filepath.Join(vaultPath, meetingsFolder, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return vaultPath
}

func TestListMeetings(t *testing.T) {
	vaultPath := writeVault(t, "meetings", map[string]string{
		"2026-09-01/standup.md":  realNoteFixture("2026-09-01", "Standup", []string{"Alice", "Bob"}, "## Summary\n\nDaily sync.\n"),
		"2026-09-05/planning.md": realNoteFixture("2026-09-05", "Sprint Planning", []string{"Alice"}, "## Summary\n\nPlanned sprint 12.\n"),
		"2026-08-20/retro.md":    realNoteFixture("2026-08-20", "Retro", nil, "## Summary\n\nRetro notes.\n"),
	})

	t.Run("lists all, sorted by date descending", func(t *testing.T) {
		meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 0)
		if err != nil {
			t.Fatalf("ListMeetings: unexpected error: %v", err)
		}
		if len(meetings) != 3 {
			t.Fatalf("got %d meetings, want 3", len(meetings))
		}
		if meetings[0].Title != "Sprint Planning" || meetings[1].Title != "Standup" || meetings[2].Title != "Retro" {
			t.Errorf("expected date-descending order, got: %v", []string{meetings[0].Title, meetings[1].Title, meetings[2].Title})
		}
	})

	t.Run("participants have wikilink brackets stripped", func(t *testing.T) {
		meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range meetings {
			if m.Title != "Standup" {
				continue
			}
			if len(m.Participants) != 2 || m.Participants[0] != "Alice" || m.Participants[1] != "Bob" {
				t.Errorf("Standup participants: got %v, want [Alice Bob] (no brackets)", m.Participants)
			}
		}
	})

	t.Run("since filters out earlier meetings", func(t *testing.T) {
		since, _ := time.Parse("2006-01-02", "2026-09-01")
		meetings, err := ListMeetings(vaultPath, "meetings", since, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(meetings) != 2 {
			t.Fatalf("got %d meetings, want 2 (retro on 08-20 excluded)", len(meetings))
		}
	})

	t.Run("limit caps the result", func(t *testing.T) {
		meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(meetings) != 1 {
			t.Fatalf("got %d meetings, want 1", len(meetings))
		}
		if meetings[0].Title != "Sprint Planning" {
			t.Errorf("limit=1 should return the most recent, got %q", meetings[0].Title)
		}
	})

	t.Run("path is set and vault-relative", func(t *testing.T) {
		meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range meetings {
			if m.Path == "" {
				t.Errorf("meeting %q has an empty Path", m.Title)
			}
			if filepath.IsAbs(m.Path) {
				t.Errorf("meeting %q Path should be vault-relative, got absolute: %q", m.Title, m.Path)
			}
		}
	})
}

func TestListMeetings_EmptyVault(t *testing.T) {
	vaultPath := t.TempDir() // no meetings folder created at all
	meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListMeetings on an empty vault should not error, got: %v", err)
	}
	if meetings != nil {
		t.Errorf("expected nil/empty result, got %v", meetings)
	}
}

func TestListMeetings_SkipsNonHeimdallNotes(t *testing.T) {
	vaultPath := writeVault(t, "meetings", map[string]string{
		"2026-09-01/standup.md": realNoteFixture("2026-09-01", "Standup", nil, "## Summary\n\nok\n"),
		"2026-09-01/random.md":  "# Just a random note\n\nNo frontmatter here, not a heimdall meeting.\n",
	})

	meetings, err := ListMeetings(vaultPath, "meetings", time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meetings) != 1 {
		t.Fatalf("got %d meetings, want 1 (the non-frontmatter note should be skipped)", len(meetings))
	}
}

func TestReadMeeting(t *testing.T) {
	content := realNoteFixture("2026-09-01", "Sprint Planning", []string{"Alice"}, "## Summary\n\nPlanned sprint 12.\n")
	vaultPath := writeVault(t, "meetings", map[string]string{
		"2026-09-01/sprint-planning.md": content,
	})

	t.Run("exact path match", func(t *testing.T) {
		got, meeting, ok, err := ReadMeeting(vaultPath, "meetings", filepath.Join("meetings", "2026-09-01", "sprint-planning.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected a match")
		}
		if got != content {
			t.Error("content mismatch on exact path match")
		}
		if meeting.Title != "Sprint Planning" {
			t.Errorf("Title: got %q, want %q", meeting.Title, "Sprint Planning")
		}
	})

	t.Run("title substring fallback, case-insensitive", func(t *testing.T) {
		_, meeting, ok, err := ReadMeeting(vaultPath, "meetings", "sprint")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected a title-substring match")
		}
		if meeting.Title != "Sprint Planning" {
			t.Errorf("Title: got %q, want %q", meeting.Title, "Sprint Planning")
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, _, ok, err := ReadMeeting(vaultPath, "meetings", "nonexistent meeting")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected no match")
		}
	})
}

func TestSearchMeetings(t *testing.T) {
	vaultPath := writeVault(t, "meetings", map[string]string{
		"2026-09-01/standup.md":  realNoteFixture("2026-09-01", "Standup", nil, "## Summary\n\nDiscussed the Kubernetes migration timeline.\n"),
		"2026-09-05/planning.md": realNoteFixture("2026-09-05", "Sprint Planning", nil, "## Summary\n\nPlanned the next sprint, no infra topics.\n"),
	})

	t.Run("finds a matching meeting with a snippet", func(t *testing.T) {
		results, err := SearchMeetings(vaultPath, "meetings", "Kubernetes", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		if results[0].Title != "Standup" {
			t.Errorf("Title: got %q, want %q", results[0].Title, "Standup")
		}
		if results[0].Snippet == "" {
			t.Error("expected a non-empty snippet")
		}
	})

	// Regression test: a real `heimdall mcp` stdio smoke test during
	// development found that a match near the top of a short note produced
	// a snippet bleeding backward into raw frontmatter YAML (e.g.
	// `participants:   - "[[Alice]]" duration: ... tags: ...`) because the
	// snippet window was centered on the match with no lower bound besides
	// 0. The snippet must never contain frontmatter syntax.
	t.Run("snippet never bleeds into frontmatter on a short note", func(t *testing.T) {
		vp := writeVault(t, "meetings", map[string]string{
			"2026-09-10/short.md": realNoteFixture("2026-09-10", "Short Note", nil, "## Summary\n\nKubernetes rollout.\n"),
		})
		results, err := SearchMeetings(vp, "meetings", "Kubernetes", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		for _, marker := range []string{"participants:", "duration:", "platform:", frontmatterDelim} {
			if strings.Contains(results[0].Snippet, marker) {
				t.Errorf("snippet leaked frontmatter (found %q): %q", marker, results[0].Snippet)
			}
		}
	})

	// A query matching only a frontmatter field (e.g. a participant's name,
	// which appears nowhere in the body) must not panic and should fall
	// back to a sane snippet rather than an invalid slice.
	t.Run("query matching only frontmatter does not panic", func(t *testing.T) {
		vp := writeVault(t, "meetings", map[string]string{
			"2026-09-11/withparticipant.md": realNoteFixture("2026-09-11", "1:1", []string{"Zbigniew"}, "## Summary\n\nRegular check-in.\n"),
		})
		results, err := SearchMeetings(vp, "meetings", "Zbigniew", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1 (participant name should still be findable)", len(results))
		}
	})

	t.Run("case-insensitive", func(t *testing.T) {
		results, err := SearchMeetings(vaultPath, "meetings", "kubernetes", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
	})

	t.Run("no match returns empty, not an error", func(t *testing.T) {
		results, err := SearchMeetings(vaultPath, "meetings", "nonexistent-term-xyz", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results, want 0", len(results))
		}
	})

	t.Run("empty query returns nothing", func(t *testing.T) {
		results, err := SearchMeetings(vaultPath, "meetings", "", time.Time{}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil for an empty query, got %v", results)
		}
	})
}

func TestStripWikilink(t *testing.T) {
	tests := []struct{ input, want string }{
		{"[[Sarah]]", "Sarah"},
		{"[[Unknown Speaker 0]]", "Unknown Speaker 0"},
		{"Sarah", "Sarah"}, // no brackets -- pass through unchanged
		{"", ""},
	}
	for _, tt := range tests {
		if got := stripWikilink(tt.input); got != tt.want {
			t.Errorf("stripWikilink(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}
