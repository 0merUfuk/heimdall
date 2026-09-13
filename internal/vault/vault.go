// Package vault reads meeting notes back out of an Obsidian vault --
// the inverse of internal/output, which only writes them (Pipeline Stage 6,
// RENDER, per PIPELINE.md: "has ONE job"). Reading is a different concern
// with its own callers: internal/mcpserver exposes it to MCP clients, and
// it is written to be reusable by a future `heimdall search` CLI command
// (STRATEGY_V2 Phase 3) without either depending on the other.
//
// Parses exactly the frontmatter shape templates/meeting-note.md.tmpl
// produces (see internal/output/renderer.go) -- date, title, participants
// (as Obsidian [[wikilinks]]), duration, platform, tags. A file that
// doesn't parse as that shape (no frontmatter, or a vault note heimdall
// didn't write) is skipped rather than erroring the whole scan, so a
// vault with other, unrelated notes in the same folder doesn't break
// listing/search.
package vault

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// frontmatterDelim marks the start/end of a note's YAML frontmatter block.
const frontmatterDelim = "---"

// Meeting is one parsed meeting note's frontmatter plus its vault-relative
// path (the stable identifier ReadMeeting accepts).
type Meeting struct {
	Path         string   `json:"path"`
	Date         string   `json:"date"`
	Title        string   `json:"title"`
	Participants []string `json:"participants,omitempty"`
	Duration     string   `json:"duration,omitempty"`
	Platform     string   `json:"platform,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// SearchResult is one Meeting plus a short excerpt showing where the query matched.
type SearchResult struct {
	Meeting
	Snippet string `json:"snippet"`
}

type noteFrontmatter struct {
	Date         string   `yaml:"date"`
	Title        string   `yaml:"title"`
	Participants []string `yaml:"participants"`
	Duration     string   `yaml:"duration"`
	Platform     string   `yaml:"platform"`
	Tags         []string `yaml:"tags"`
}

// wikilinkPattern strips Obsidian's [[...]] wrapper, which the template
// bakes directly into each participant's YAML string value (e.g. the
// literal scalar `[[Sarah]]`, not a YAML list of a list).
var wikilinkPattern = regexp.MustCompile(`^\[\[(.*)\]\]$`)

func stripWikilink(s string) string {
	if m := wikilinkPattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// meetingsDir returns vaultPath/meetingsFolder.
func meetingsDir(vaultPath, meetingsFolder string) string {
	return filepath.Join(vaultPath, meetingsFolder)
}

// walkNotes calls fn for every .md file's raw content under the meetings
// directory. Read errors on an individual file are skipped, not fatal --
// one unreadable note should not break listing the rest.
func walkNotes(vaultPath, meetingsFolder string, fn func(path, relPath string, content []byte)) error {
	dir := meetingsDir(vaultPath, meetingsFolder)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // no meetings yet -- not an error
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip walk errors on individual entries
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(vaultPath, path)
		if relErr != nil {
			rel = path
		}
		fn(path, rel, content)
		return nil
	})
}

// parseFrontmatter extracts and parses the YAML frontmatter block from note
// content. Returns ok=false (not an error) if content has no frontmatter --
// callers treat that as "not a heimdall meeting note, skip it".
func parseFrontmatter(content []byte, relPath string) (Meeting, bool) {
	text := string(content)
	if !strings.HasPrefix(text, frontmatterDelim) {
		return Meeting{}, false
	}
	rest := text[len(frontmatterDelim):]
	yamlBlock, _, found := strings.Cut(rest, "\n"+frontmatterDelim)
	if !found {
		return Meeting{}, false
	}

	var fm noteFrontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return Meeting{}, false
	}

	participants := make([]string, 0, len(fm.Participants))
	for _, p := range fm.Participants {
		if s := stripWikilink(p); s != "" {
			participants = append(participants, s)
		}
	}

	title := fm.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(relPath), ".md")
	}

	return Meeting{
		Path:         relPath,
		Date:         fm.Date,
		Title:        title,
		Participants: participants,
		Duration:     fm.Duration,
		Platform:     fm.Platform,
		Tags:         fm.Tags,
	}, true
}

// bodyOffset returns the byte offset where a note's body starts (right
// after the closing frontmatter delimiter), or 0 if content has no
// frontmatter. Used to keep search snippets from bleeding backward into
// raw YAML -- a query match near the top of the body would otherwise pull
// frontmatter syntax into the snippet on a short note.
func bodyOffset(content []byte) int {
	text := string(content)
	if !strings.HasPrefix(text, frontmatterDelim) {
		return 0
	}
	rest := text[len(frontmatterDelim):]
	_, body, found := strings.Cut(rest, "\n"+frontmatterDelim)
	if !found {
		return 0
	}
	// body's offset within the original text: past both delimiters and the
	// YAML block strings.Cut already consumed.
	return len(text) - len(body)
}

// ListMeetings scans the vault's meetings folder and returns parsed
// frontmatter for every note, sorted by date descending (most recent
// first). since filters to meetings on or after that date (zero value: no
// filter); limit caps the result count (0 or negative: no cap).
func ListMeetings(vaultPath, meetingsFolder string, since time.Time, limit int) ([]Meeting, error) {
	var meetings []Meeting

	err := walkNotes(vaultPath, meetingsFolder, func(_, relPath string, content []byte) {
		m, ok := parseFrontmatter(content, relPath)
		if !ok {
			return
		}
		if !since.IsZero() && m.Date != "" && m.Date < since.Format("2006-01-02") {
			return
		}
		meetings = append(meetings, m)
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].Date > meetings[j].Date
	})

	if limit > 0 && len(meetings) > limit {
		meetings = meetings[:limit]
	}
	return meetings, nil
}

// ReadMeeting returns the full raw markdown content of one meeting note,
// plus its parsed frontmatter. identifier is matched, in order: (1) exactly
// against a vault-relative path (as ListMeetings/SearchMeetings return in
// Meeting.Path -- the reliable, unambiguous path), then (2) as a
// case-insensitive substring against the note's title (a convenience for a
// caller that only knows roughly what the meeting was called). Returns
// ok=false if nothing matches.
func ReadMeeting(vaultPath, meetingsFolder, identifier string) (content string, meeting Meeting, ok bool, err error) {
	exactPath := filepath.Join(vaultPath, identifier)
	if data, readErr := os.ReadFile(exactPath); readErr == nil {
		m, parsed := parseFrontmatter(data, identifier)
		if !parsed {
			m = Meeting{Path: identifier, Title: strings.TrimSuffix(filepath.Base(identifier), ".md")}
		}
		return string(data), m, true, nil
	}

	lowerQuery := strings.ToLower(identifier)
	var matchContent string
	var matchMeeting Meeting
	found := false

	walkErr := walkNotes(vaultPath, meetingsFolder, func(_, relPath string, content []byte) {
		if found {
			return
		}
		m, parsed := parseFrontmatter(content, relPath)
		if !parsed {
			return
		}
		if strings.Contains(strings.ToLower(m.Title), lowerQuery) {
			matchContent = string(content)
			matchMeeting = m
			found = true
		}
	})
	if walkErr != nil {
		return "", Meeting{}, false, walkErr
	}
	return matchContent, matchMeeting, found, nil
}

// snippetRadius is how many characters of context to include on each side
// of a search match in SearchResult.Snippet.
const snippetRadius = 120

// SearchMeetings does a case-insensitive substring search over meeting note
// content (frontmatter + body) and returns matches with a short snippet
// around the first match, sorted by date descending. since and limit behave
// as in ListMeetings.
func SearchMeetings(vaultPath, meetingsFolder, query string, since time.Time, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	lowerQuery := strings.ToLower(query)

	var results []SearchResult
	err := walkNotes(vaultPath, meetingsFolder, func(_, relPath string, content []byte) {
		m, ok := parseFrontmatter(content, relPath)
		if !ok {
			return
		}
		if !since.IsZero() && m.Date != "" && m.Date < since.Format("2006-01-02") {
			return
		}

		text := string(content)
		idx := strings.Index(strings.ToLower(text), lowerQuery)
		if idx == -1 {
			return
		}

		// Clamp the snippet to the body -- a match near the top of a short
		// note would otherwise pull raw frontmatter YAML into the snippet.
		// If the match itself falls inside the frontmatter (e.g. a
		// participant name), fall back to showing the start of the body
		// rather than a snippet centered on a location before it.
		bodyStart := bodyOffset(content)
		matchStart, matchEnd := idx, idx+len(query)
		if matchStart < bodyStart {
			matchStart, matchEnd = bodyStart, bodyStart
		}
		start := max(bodyStart, matchStart-snippetRadius)
		end := max(start, min(len(text), matchEnd+snippetRadius))
		snippet := strings.TrimSpace(text[start:end])

		results = append(results, SearchResult{Meeting: m, Snippet: snippet})
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Date > results[j].Date
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
