package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server exposing your meeting history to AI agents",
	Long: `Starts a local MCP (Model Context Protocol) server over stdio,
exposing your Obsidian vault's meeting notes as three read-only tools:

  list_meetings    List past meetings, most recent first
  search_meetings  Full-text search across all meeting notes
  get_meeting      Get one meeting's full content (summary, decisions,
                   action items, transcript)

This lets any MCP client -- Claude Desktop, Claude Code, Cursor -- answer
questions like "what did we decide about the API migration?" directly from
your vault. Runs entirely locally: no network access, no data leaves your
machine beyond what your MCP client itself does with the results.

Add to Claude Desktop's config (claude_desktop_config.json):

  {
    "mcpServers": {
      "heimdall": {
        "command": "heimdall",
        "args": ["mcp"]
      }
    }
  }

Runs until the client disconnects (stdin closes) -- not a long-lived
background daemon you start separately.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Deliberately NOT cfg.ResolveEnvVars(): that resolves every field in
	// the config (deepgram.api_key, claude.api_key, ...) and errors hard if
	// any referenced env var is unset. The MCP server only ever reads
	// obsidian.vault_path -- it has no business requiring DEEPGRAM_API_KEY
	// or ANTHROPIC_API_KEY to be set just to start (found by actually
	// running `heimdall mcp` over stdio with neither set, which is a
	// completely normal way to use this command). os.ExpandEnv handles the
	// rare case where vault_path itself is templated with a ${VAR}; unlike
	// ResolveEnvVars it substitutes an empty string for an unset var rather
	// than erroring, which is fine here since the venue for user feedback
	// on a nonexistent path is doctor/config, not this command.
	vaultPath := ""
	if cfg.Obsidian.VaultPath != "" {
		vaultPath = config.ExpandHome(os.ExpandEnv(cfg.Obsidian.VaultPath))
	}

	meetingsFolder := cfg.Obsidian.MeetingsFolder
	if meetingsFolder == "" {
		meetingsFolder = "meetings"
	}

	server := mcpserver.NewServer(vaultPath, meetingsFolder, version)
	err = server.Run(context.Background(), &mcp.StdioTransport{})
	if isCleanStdioShutdown(err) {
		return nil
	}
	return err
}

// isCleanStdioShutdown reports whether err represents the client ending the
// connection normally (stdin closed) rather than an actual failure.
//
// A real MCP client (Claude Desktop, Claude Code, ...) owns this
// subprocess's lifecycle and closes its stdin when it's done -- that is the
// expected, everyday end of every session, not an error condition. Verified
// empirically by driving `heimdall mcp` with a real JSON-RPC request
// sequence over stdio and observing the exact returned error on a normal
// disconnect ("server is closing: EOF"); without this check, cobra would
// print "Error: server is closing: EOF" and exit 1 on every single clean
// session end, which would show up as a crash in an MCP client's process
// supervision even though nothing went wrong. errors.Is is tried first
// (correct if a future SDK version adds proper error wrapping) with a
// string fallback matching the exact behavior observed against SDK v1.7.0,
// whose jsonrpc2.Connection.wait does not implement Unwrap.
func isCleanStdioShutdown(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	return strings.Contains(err.Error(), "EOF")
}
