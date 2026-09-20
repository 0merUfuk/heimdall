package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: ClaudeCodeAnalyzer must satisfy Analyzer.
var _ Analyzer = (*ClaudeCodeAnalyzer)(nil)

const (
	// claudeCodeBinDefault is the `claude` binary name, resolved via PATH.
	claudeCodeBinDefault = "claude"

	// claudeCodeCallTimeout bounds a single `claude -p` invocation. Headless
	// single-turn analysis should complete in well under a minute; this is a
	// generous ceiling so a slow model or cold start doesn't hang forever.
	claudeCodeCallTimeout = 180 * time.Second

	// claudeCodeMaxErrLen truncates subprocess output embedded in error
	// messages so a runaway or unexpected response doesn't flood the terminal.
	claudeCodeMaxErrLen = 300
)

// commandRunner abstracts subprocess execution so tests can substitute a
// fake process instead of invoking the real `claude` binary. It returns
// whatever the process wrote to stdout and the error from running it (nil on
// exit code 0). Mirrors the WithHTTPClient/WithBaseURL testability pattern
// used by ClaudeAnalyzer, adapted for subprocesses instead of HTTP.
type commandRunner func(ctx context.Context, name string, args []string, stdin string, opts runOpts) (stdout string, err error)

// runOpts are optional process settings for a commandRunner call.
type runOpts struct {
	// Dir is the working directory; empty inherits heimdall's.
	Dir string
	// Env entries ("KEY=value") are added on top of heimdall's environment.
	Env []string
}

// execCommandRunner is the production commandRunner: it actually spawns the
// process via os/exec.
func execCommandRunner(ctx context.Context, name string, args []string, stdin string, opts runOpts) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w (stderr: %s)", err, truncateForError(stderr.String()))
	}
	return stdout.String(), err
}

// ClaudeCodeAnalyzer implements Analyzer by shelling out to the locally
// installed `claude` CLI (Claude Code) in non-interactive print mode
// (`claude -p`). This lets a user who already pays for a Claude subscription
// (Pro/Max/Team) analyze meetings without configuring a separate,
// pay-per-token ANTHROPIC_API_KEY -- heimdall never sees or handles the
// credential at all, it just asks the user's own already-authenticated
// Claude Code installation to do the analysis.
//
// The call is isolated from the user's Claude Code environment -- no hooks,
// plugins, CLAUDE.md, MCP servers, or saved session (see claudeCodeArgs) --
// and runs with --permission-prompts none, so anything that would need a
// permission prompt is auto-denied rather than hanging with no TTY to answer
// it. This is a pure text-in, JSON-out completion -- the model has no
// legitimate reason to invoke a tool.
type ClaudeCodeAnalyzer struct {
	binPath string
	runner  commandRunner
}

// NewClaudeCodeAnalyzer creates a ClaudeCodeAnalyzer that invokes `claude`
// resolved via PATH.
func NewClaudeCodeAnalyzer() *ClaudeCodeAnalyzer {
	return &ClaudeCodeAnalyzer{
		binPath: claudeCodeBinDefault,
		runner:  execCommandRunner,
	}
}

// WithBinPath overrides the `claude` binary path (e.g. a non-PATH install).
func (c *ClaudeCodeAnalyzer) WithBinPath(path string) *ClaudeCodeAnalyzer {
	c.binPath = path
	return c
}

// WithRunner overrides the subprocess runner. Used in tests.
func (c *ClaudeCodeAnalyzer) WithRunner(r commandRunner) *ClaudeCodeAnalyzer {
	c.runner = r
	return c
}

// Summarize analyzes a complete meeting transcript via `claude -p` and
// returns structured notes. Shares retry/backoff/fallback policy with
// ClaudeAnalyzer via summarizeWithRetry (V-009) -- a subscription-backed
// analysis should degrade exactly as gracefully as the API-backed one.
func (c *ClaudeCodeAnalyzer) Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	return summarizeWithRetry(ctx, segments, opts, func(ctx context.Context, userPrompt string) (string, error) {
		return c.callOnce(ctx, opts.Model, userPrompt)
	})
}

// claudeCodeEnvelope is the subset of `claude -p --output-format json`'s
// result envelope that we need. Unknown fields (cost, usage, session_id,
// ...) are ignored by json.Unmarshal, so this stays forward-compatible with
// newer Claude Code versions that add fields.
type claudeCodeEnvelope struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// claudeCodeIsolationEnv turns off the context sources `--restricted` does
// not cover. Each name was verified to be read by the installed claude
// binary (2.1.271), not taken from documentation summaries.
var claudeCodeIsolationEnv = []string{
	"CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",           // no user/project CLAUDE.md in the analysis context
	"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",          // no auto-memory read or written
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", // no telemetry/update checks
}

// claudeCodeArgs builds the `claude -p` argument list. There is deliberately
// no --bare: under --bare Claude Code reads auth "strictly ANTHROPIC_API_KEY
// or apiKeyHelper (OAuth and keychain are never read)" (its own --help), which
// made this backend unusable for exactly the subscription users it exists
// for. The isolation --bare used to give is rebuilt from narrower pieces:
//   - --restricted: ignores user/project/local settings files (so their
//     hooks and enabled plugins never run -- a memory plugin would otherwise
//     capture the meeting transcript), removes command-running tools, and
//     confines file tools to the (empty) working directory;
//   - --strict-mcp-config: no MCP servers;
//   - --no-session-persistence: the transcript is not saved as a session;
//   - --disable-slash-commands: no skills;
//   - claudeCodeIsolationEnv plus an empty temp working directory.
//
// Verified empirically against a live model: a SessionStart hook and a
// CLAUDE.md canary planted in the user config dir both fire without these
// flags and stay silent with them (.claude/DECISIONS.md ID-015).
func claudeCodeArgs(model string) []string {
	args := []string{
		"-p",
		"--restricted",
		"--strict-mcp-config",
		"--no-session-persistence",
		"--disable-slash-commands",
		"--permission-prompts", "none",
		"--output-format", "json",
		"--system-prompt", systemPrompt,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

// callOnce runs a single `claude -p` invocation and returns the model's raw
// text response (expected to be the analysis JSON described in
// systemPrompt). It does not retry -- summarizeWithRetry owns that.
func (c *ClaudeCodeAnalyzer) callOnce(ctx context.Context, model, userPrompt string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, claudeCodeCallTimeout)
	defer cancel()

	// Private, empty working directory: no project CLAUDE.md, settings, or
	// files for the call to see. Removed afterwards.
	workDir, err := os.MkdirTemp("", "heimdall-claude-*")
	if err != nil {
		return "", fmt.Errorf("creating claude temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	stdout, runErr := c.runner(callCtx, c.binPath, claudeCodeArgs(model), userPrompt, runOpts{
		Dir: workDir,
		Env: claudeCodeIsolationEnv,
	})

	var envelope claudeCodeEnvelope
	if jsonErr := json.Unmarshal([]byte(stdout), &envelope); jsonErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			return "", fmt.Errorf("claude CLI not found on PATH -- install Claude Code (https://claude.com/claude-code) or use --analyzer api with ANTHROPIC_API_KEY instead")
		}
		if runErr != nil {
			return "", fmt.Errorf("claude CLI failed: %w", runErr)
		}
		return "", fmt.Errorf("claude CLI produced no parseable output: %s", truncateForError(stdout))
	}

	if envelope.IsError {
		if strings.Contains(envelope.Result, "/login") || strings.Contains(envelope.Result, "Not logged in") {
			return "", fmt.Errorf("claude CLI is not logged in -- run 'claude /login' first, or use --analyzer api with ANTHROPIC_API_KEY instead")
		}
		return "", fmt.Errorf("claude CLI returned an error: %s", truncateForError(envelope.Result))
	}

	if strings.TrimSpace(envelope.Result) == "" {
		return "", fmt.Errorf("claude CLI returned an empty result")
	}

	return envelope.Result, nil
}

// truncateForError shortens subprocess output for inclusion in an error
// message so a runaway or binary response doesn't flood the terminal.
func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > claudeCodeMaxErrLen {
		return s[:claudeCodeMaxErrLen] + "..."
	}
	return s
}
