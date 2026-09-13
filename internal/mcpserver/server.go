// Package mcpserver exposes heimdall's Obsidian vault to MCP clients
// (Claude Desktop, Claude Code, Cursor, any MCP-speaking agent) as three
// read-only tools backed by internal/vault: list_meetings, search_meetings,
// get_meeting. This is the "Obsidian knowledge graph two-way bridge" --
// heimdall writes meeting notes into the vault (internal/output) and this
// package reads them back out for an agent to query (docs/PRODUCTIZATION.md
// v3 names this the project's core differentiator).
//
// Deliberately basic: three tools, no auth, no write access, stdio
// transport only. docs/PRODUCTIZATION.md v3 scopes richer tools
// (get_action_items, get_decisions, vault-as-memory) as a paid-tier
// addition layered on top of this free, local foundation -- not something
// to build speculatively ahead of an actual Pro tier existing.
package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/0merUfuk/heimdall/internal/vault"
)

const serverName = "heimdall"

// NewServer builds an MCP server exposing vaultPath/meetingsFolder (the
// same two fields as config.ObsidianConfig) as read-only tools. version is
// the heimdall build version (main.version), reported to MCP clients via
// the server's Implementation info.
func NewServer(vaultPath, meetingsFolder, version string) *mcp.Server {
	if version == "" {
		version = "dev"
	}
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_meetings",
		Description: "List past meetings heimdall has recorded, most recent first. Returns date, title, participants, and a path usable with get_meeting.",
	}, listMeetingsHandler(vaultPath, meetingsFolder))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_meetings",
		Description: "Full-text search across all past meeting notes (summary, decisions, action items, transcript). Returns matching meetings with a short snippet showing where the query matched.",
	}, searchMeetingsHandler(vaultPath, meetingsFolder))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_meeting",
		Description: "Get the full content of one meeting note (summary, decisions, action items, transcript), identified by the path returned from list_meetings/search_meetings, or a title fragment.",
	}, getMeetingHandler(vaultPath, meetingsFolder))

	return s
}

// textResult wraps a single string as a successful CallToolResult -- every
// handler in this package returns plain, LLM-readable text rather than a
// typed structured result, since the primary MCP clients (Claude Desktop,
// Claude Code, Cursor) surface tool output to a model that reads prose fine
// and gains nothing from JSON it would just re-read as text anyway.
func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// errorResult reports a tool-level failure (bad input, vault not
// configured, nothing found) via CallToolResult.IsError rather than a Go
// error return.
//
// This distinction is load-bearing, not stylistic: the SDK's own docs for
// CallToolResult.IsError say plainly that tool-originated errors belong in
// Content with IsError set, "not as an MCP protocol-level error response
// ... Otherwise, the LLM would not be able to see that an error occurred
// and self-correct." A Go error return becomes an MCP protocol-level
// error (reserved for "errors in finding the tool" or transport-level
// failures) -- the calling model never sees the message, so "vault not
// configured" or "invalid date" would silently fail instead of being
// something the model could read, explain to the user, or retry
// differently. Every handler in this file returns errorResult for
// business-logic failures and only a real Go error for something
// operationally exceptional.
func errorResult(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}, nil, nil
}

// parseSince parses a YYYY-MM-DD date string. Empty input is not an error
// (means "no filter"); returns ok=false for a malformed one, so a typo
// doesn't silently return unfiltered results.
func parseSince(s string) (t time.Time, ok bool) {
	if s == "" {
		return time.Time{}, true
	}
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

type listMeetingsArgs struct {
	Since string `json:"since,omitempty" jsonschema:"only include meetings on or after this date (YYYY-MM-DD)"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of meetings to return (default: no limit)"`
}

func listMeetingsHandler(vaultPath, meetingsFolder string) mcp.ToolHandlerFor[listMeetingsArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args listMeetingsArgs) (*mcp.CallToolResult, any, error) {
		if vaultPath == "" {
			return errorResult("Obsidian vault is not configured. Ask the user to run: heimdall config set obsidian.vault_path /path/to/vault")
		}
		since, ok := parseSince(args.Since)
		if !ok {
			return errorResult("invalid since date %q: expected YYYY-MM-DD", args.Since)
		}

		meetings, err := vault.ListMeetings(vaultPath, meetingsFolder, since, args.Limit)
		if err != nil {
			return nil, nil, fmt.Errorf("listing meetings: %w", err)
		}
		if len(meetings) == 0 {
			return textResult("No meetings found.")
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Found %d meeting(s):\n\n", len(meetings))
		for _, m := range meetings {
			fmt.Fprintf(&b, "- %s | %s", m.Date, m.Title)
			if len(m.Participants) > 0 {
				fmt.Fprintf(&b, " | participants: %s", strings.Join(m.Participants, ", "))
			}
			fmt.Fprintf(&b, " | path: %s\n", m.Path)
		}
		return textResult(b.String())
	}
}

type searchMeetingsArgs struct {
	Query string `json:"query" jsonschema:"the text to search for across all meeting notes"`
	Since string `json:"since,omitempty" jsonschema:"only include meetings on or after this date (YYYY-MM-DD)"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of results to return (default: no limit)"`
}

func searchMeetingsHandler(vaultPath, meetingsFolder string) mcp.ToolHandlerFor[searchMeetingsArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args searchMeetingsArgs) (*mcp.CallToolResult, any, error) {
		if vaultPath == "" {
			return errorResult("Obsidian vault is not configured. Ask the user to run: heimdall config set obsidian.vault_path /path/to/vault")
		}
		if strings.TrimSpace(args.Query) == "" {
			return errorResult("query must not be empty")
		}
		since, ok := parseSince(args.Since)
		if !ok {
			return errorResult("invalid since date %q: expected YYYY-MM-DD", args.Since)
		}

		results, err := vault.SearchMeetings(vaultPath, meetingsFolder, args.Query, since, args.Limit)
		if err != nil {
			return nil, nil, fmt.Errorf("searching meetings: %w", err)
		}
		if len(results) == 0 {
			return textResult(fmt.Sprintf("No meetings found matching %q.", args.Query))
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Found %d match(es) for %q:\n\n", len(results), args.Query)
		for _, r := range results {
			fmt.Fprintf(&b, "- %s | %s | path: %s\n", r.Date, r.Title, r.Path)
			fmt.Fprintf(&b, "  ...%s...\n\n", strings.ReplaceAll(r.Snippet, "\n", " "))
		}
		return textResult(b.String())
	}
}

type getMeetingArgs struct {
	Identifier string `json:"identifier" jsonschema:"a meeting's path (from list_meetings/search_meetings) or a title fragment"`
}

func getMeetingHandler(vaultPath, meetingsFolder string) mcp.ToolHandlerFor[getMeetingArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args getMeetingArgs) (*mcp.CallToolResult, any, error) {
		if vaultPath == "" {
			return errorResult("Obsidian vault is not configured. Ask the user to run: heimdall config set obsidian.vault_path /path/to/vault")
		}
		if strings.TrimSpace(args.Identifier) == "" {
			return errorResult("identifier must not be empty")
		}

		content, _, ok, err := vault.ReadMeeting(vaultPath, meetingsFolder, args.Identifier)
		if err != nil {
			return nil, nil, fmt.Errorf("reading meeting: %w", err)
		}
		if !ok {
			return errorResult("No meeting found matching %q. Use list_meetings or search_meetings to find a valid path.", args.Identifier)
		}
		return textResult(content)
	}
}
