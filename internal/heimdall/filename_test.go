package heimdall

import "testing"

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{"simple title", "Sprint Planning", "meeting", "sprint-planning"},
		{"special characters", "1:1 with Sarah!", "meeting", "11-with-sarah"},
		{"multiple spaces", "Team   Sync   Meeting", "meeting", "team-sync-meeting"},
		{"leading/trailing spaces", "  Sprint Review  ", "meeting", "sprint-review"},
		{"all special chars falls back", "!!!@@@###", "meeting", "meeting"},
		{"empty falls back", "", "meeting", "meeting"},
		{"only spaces falls back", "   ", "meeting", "meeting"},
		{"hyphens preserved", "sprint-planning", "meeting", "sprint-planning"},
		{"consecutive hyphens collapsed", "sprint---planning", "meeting", "sprint-planning"},
		{"different fallback", "###", "untitled", "untitled"},
		// The whole point of \p{L}/\p{N} over [a-zA-Z0-9]: real non-ASCII
		// scripts must survive sanitization, not be reduced to hyphens/empty.
		{"Turkish characters preserved", "Sağlık Toplantısı", "meeting", "sağlık-toplantısı"},
		{"Turkish characters preserved, other fallback", "Çağrı Değerlendirmesi", "untitled", "çağrı-değerlendirmesi"},
		{"CJK characters preserved", "会議のメモ", "meeting", "会議のメモ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q, %q): got %q, want %q", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}
