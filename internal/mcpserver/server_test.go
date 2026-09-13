package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// realNoteFixture mirrors the exact output shape of
// templates/meeting-note.md.tmpl (see internal/output/renderer.go and
// internal/vault's own fixture of the same shape).
func realNoteFixture(date, title, body string) string {
	return "---\n" +
		"date: " + date + "\n" +
		"type: meeting\n" +
		"title: \"" + title + "\"\n" +
		"participants:\n" +
		"  - \"[[Alice]]\"\n" +
		"duration: 30m0s\n" +
		"platform: desktop\n" +
		"tags:\n" +
		"  - meeting\n" +
		"---\n\n# " + title + "\n\n" + body
}

func writeTestVault(t *testing.T) (vaultPath string) {
	t.Helper()
	vaultPath = t.TempDir()
	notes := map[string]string{
		"2026-09-01/standup.md":  realNoteFixture("2026-09-01", "Standup", "## Summary\n\nDiscussed the Kubernetes migration.\n"),
		"2026-09-05/planning.md": realNoteFixture("2026-09-05", "Sprint Planning", "## Summary\n\nPlanned sprint 12.\n"),
	}
	for relPath, content := range notes {
		full := filepath.Join(vaultPath, "meetings", relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return vaultPath
}

// textOf extracts the concatenated text of a CallToolResult, failing the
// test if the result has no text content -- every handler in this package
// always returns exactly one TextContent block.
func textOf(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("result.Content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// --- Integration tests: real client <-> server over the actual MCP protocol ---

func connectTestSession(t *testing.T, vaultPath string) *mcp.ClientSession {
	t.Helper()
	server := NewServer(vaultPath, "meetings", "test")
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)

	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestIntegration_ListToolsRegistersAllThree(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_meetings", "search_meetings", "get_meeting"} {
		if !names[want] {
			t.Errorf("tool %q not registered; got: %v", want, names)
		}
	}
}

func TestIntegration_ListMeetings(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_meetings"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError with content: %s", textOf(t, result))
	}
	text := textOf(t, result)
	if !strings.Contains(text, "Sprint Planning") || !strings.Contains(text, "Standup") {
		t.Errorf("expected both meetings listed, got: %s", text)
	}
	// Most recent first.
	if strings.Index(text, "Sprint Planning") > strings.Index(text, "Standup") {
		t.Errorf("expected Sprint Planning (2026-09-05) before Standup (2026-09-01), got: %s", text)
	}
}

func TestIntegration_ListMeetings_LimitArgument(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_meetings",
		Arguments: map[string]any{"limit": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := textOf(t, result)
	if strings.Contains(text, "Standup") {
		t.Errorf("limit=1 should exclude the older meeting, got: %s", text)
	}
	if !strings.Contains(text, "Sprint Planning") {
		t.Errorf("limit=1 should include the most recent meeting, got: %s", text)
	}
}

func TestIntegration_SearchMeetings(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_meetings",
		Arguments: map[string]any{"query": "Kubernetes"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError with content: %s", textOf(t, result))
	}
	text := textOf(t, result)
	if !strings.Contains(text, "Standup") {
		t.Errorf("expected the Kubernetes-mentioning meeting (Standup), got: %s", text)
	}
	if strings.Contains(text, "Sprint Planning") {
		t.Errorf("Sprint Planning doesn't mention Kubernetes, should not match, got: %s", text)
	}
}

func TestIntegration_SearchMeetings_EmptyQueryIsToolError(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_meetings",
		Arguments: map[string]any{"query": ""},
	})
	// Must NOT be a protocol-level error -- the whole point of IsError is
	// that the model sees this as tool output it can react to, not an
	// opaque RPC failure.
	if err != nil {
		t.Fatalf("expected a successful RPC carrying IsError:true, got protocol error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for an empty query")
	}
	if textOf(t, result) == "" {
		t.Error("expected a non-empty explanation in Content even when IsError")
	}
}

func TestIntegration_GetMeeting_ByPath(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))
	ctx := context.Background()

	// Discover the path via list_meetings first, matching the documented flow.
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_meetings"})
	if err != nil {
		t.Fatalf("CallTool(list_meetings): %v", err)
	}
	listText := textOf(t, listResult)
	idx := strings.Index(listText, "path: ")
	if idx == -1 {
		t.Fatalf("expected a path in list_meetings output, got: %s", listText)
	}
	pathLine := listText[idx+len("path: "):]
	path := strings.TrimSpace(strings.SplitN(pathLine, "\n", 2)[0])

	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_meeting",
		Arguments: map[string]any{"identifier": path},
	})
	if err != nil {
		t.Fatalf("CallTool(get_meeting): %v", err)
	}
	if getResult.IsError {
		t.Fatalf("expected success, got IsError with content: %s", textOf(t, getResult))
	}
	got := textOf(t, getResult)
	if !strings.Contains(got, "## Summary") {
		t.Errorf("expected the full note content back, got: %s", got)
	}
}

func TestIntegration_GetMeeting_ByTitleFragment(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_meeting",
		Arguments: map[string]any{"identifier": "sprint"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError with content: %s", textOf(t, result))
	}
	if got := textOf(t, result); !strings.Contains(got, "Planned sprint 12") {
		t.Errorf("expected Sprint Planning's content, got: %s", got)
	}
}

func TestIntegration_GetMeeting_NotFoundIsToolError(t *testing.T) {
	session := connectTestSession(t, writeTestVault(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_meeting",
		Arguments: map[string]any{"identifier": "no-such-meeting-xyz"},
	})
	if err != nil {
		t.Fatalf("expected a successful RPC carrying IsError:true, got protocol error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when no meeting matches")
	}
}

func TestIntegration_VaultNotConfiguredIsToolError(t *testing.T) {
	session := connectTestSession(t, "") // empty vaultPath = "not configured"

	for _, tool := range []string{"list_meetings", "search_meetings", "get_meeting"} {
		t.Run(tool, func(t *testing.T) {
			args := map[string]any{}
			if tool == "search_meetings" {
				args["query"] = "anything"
			}
			if tool == "get_meeting" {
				args["identifier"] = "anything"
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
			if err != nil {
				t.Fatalf("expected a successful RPC carrying IsError:true, got protocol error: %v", err)
			}
			if !result.IsError {
				t.Errorf("%s: expected IsError=true when vault is not configured", tool)
			}
			if !strings.Contains(textOf(t, result), "not configured") {
				t.Errorf("%s: expected a 'not configured' explanation, got: %s", tool, textOf(t, result))
			}
		})
	}
}

// --- Unit tests: parseSince, exercised directly (no protocol overhead needed) ---

func TestParseSince(t *testing.T) {
	t.Run("empty is valid, means no filter", func(t *testing.T) {
		got, ok := parseSince("")
		if !ok {
			t.Fatal("expected ok=true for empty input")
		}
		if !got.IsZero() {
			t.Errorf("expected zero time for empty input, got %v", got)
		}
	})

	t.Run("valid date parses", func(t *testing.T) {
		got, ok := parseSince("2026-09-01")
		if !ok {
			t.Fatal("expected ok=true")
		}
		if got.Format("2006-01-02") != "2026-09-01" {
			t.Errorf("got %v, want 2026-09-01", got)
		}
	})

	t.Run("malformed date is rejected", func(t *testing.T) {
		_, ok := parseSince("not-a-date")
		if ok {
			t.Error("expected ok=false for a malformed date")
		}
	})
}
