package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: CodexAnalyzer must satisfy Analyzer.
var _ Analyzer = (*CodexAnalyzer)(nil)

const (
	// DefaultCodexModel is the cheapest Codex model tier ("fast and
	// affordable") -- the Codex counterpart of defaulting the Anthropic
	// backend to Haiku. Meeting analysis is a bounded, single-shot
	// extraction task; it does not need a frontier coding model.
	DefaultCodexModel = "gpt-5.6-luna"

	// codexReasoningEffort keeps per-meeting cost minimal. Extraction from a
	// transcript needs little deliberate reasoning, and the user's global
	// Codex default (often high/ultra for coding) is ignored on purpose.
	codexReasoningEffort = "low"

	// codexBinDefault is the `codex` binary name, resolved via PATH.
	codexBinDefault = "codex"

	// codexCallTimeout bounds a single `codex exec` invocation.
	codexCallTimeout = 5 * time.Minute
)

// CodexAnalyzer implements Analyzer by shelling out to the locally installed
// `codex` CLI in non-interactive mode (`codex exec`), so a user who already
// has a ChatGPT/Codex plan can analyze meetings without an Anthropic API key.
// heimdall never sees or handles the credential. It is the Codex counterpart
// of ClaudeCodeAnalyzer and shares its retry/fallback policy.
//
// Isolation flags, each verified against codex-cli 0.154.0:
//   - --ignore-user-config: the user's ~/.codex/config.toml (model, reasoning
//     effort, notify hooks, MCP servers) does not apply; auth still does.
//   - --strict-config: fail loudly if a future Codex drops a key we rely on
//     (developer_instructions) rather than silently analyzing without the
//     anti-injection rules.
//   - -c developer_instructions=...: systemPrompt goes in as developer
//     instructions, separate from the transcript (V-014), mirroring the
//     system/user split the other backends use.
//   - -c project_doc_max_bytes=0, -C <empty temp dir>, --sandbox read-only,
//     --ephemeral, --ignore-rules: no project AGENTS.md, no repository or
//     working-directory context, no writes, no persisted session.
//
// The answer is read from the file named by -o (--output-last-message), a
// stable documented flag, rather than from the --json event stream, whose
// schema is only relied on for failure messages and token usage.
type CodexAnalyzer struct {
	binPath string
	runner  commandRunner
}

// NewCodexAnalyzer creates a CodexAnalyzer that invokes `codex` via PATH.
func NewCodexAnalyzer() *CodexAnalyzer {
	return &CodexAnalyzer{
		binPath: codexBinDefault,
		runner:  execCommandRunner,
	}
}

// WithBinPath overrides the `codex` binary path.
func (c *CodexAnalyzer) WithBinPath(path string) *CodexAnalyzer {
	c.binPath = path
	return c
}

// WithRunner overrides the subprocess runner. Used in tests.
func (c *CodexAnalyzer) WithRunner(r commandRunner) *CodexAnalyzer {
	c.runner = r
	return c
}

// Summarize analyzes a complete meeting transcript via `codex exec`.
func (c *CodexAnalyzer) Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	model := opts.Model
	if model == "" {
		model = DefaultCodexModel
	}
	return summarizeWithRetry(ctx, segments, opts, func(ctx context.Context, userPrompt string) (string, error) {
		return c.callOnce(ctx, model, userPrompt)
	})
}

// codexUsage is turn.completed's token usage.
type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// codexEvent is the subset of a `codex exec --json` JSONL event we read.
// Verified shapes (codex-cli 0.154.0): {"type":"error","message":...} and
// {"type":"turn.failed","error":{"message":...}}. turn.completed's usage
// object is read best-effort for the token log line only.
type codexEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *codexUsage `json:"usage"`
}

// codexArgs builds the `codex exec` argument list.
func codexArgs(model, workDir, outFile string) ([]string, error) {
	// -c values are parsed as TOML. A JSON string literal (without HTML
	// escaping) is a valid TOML basic string for any Go string.
	var instr bytes.Buffer
	enc := json.NewEncoder(&instr)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(systemPrompt); err != nil {
		return nil, fmt.Errorf("encoding codex instructions: %w", err)
	}

	return []string{
		"exec",
		"--ignore-user-config",
		"--strict-config",
		"--ephemeral",
		"--skip-git-repo-check",
		"--ignore-rules",
		"--sandbox", "read-only",
		"-C", workDir,
		"-m", model,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", codexReasoningEffort),
		"-c", "project_doc_max_bytes=0",
		"-c", "developer_instructions=" + strings.TrimSpace(instr.String()),
		"--json",
		"-o", outFile,
		"-", // read the prompt (the transcript) from stdin
	}, nil
}

// callOnce runs a single `codex exec` invocation and returns the model's raw
// text response. It does not retry -- summarizeWithRetry owns that.
func (c *CodexAnalyzer) callOnce(ctx context.Context, model, userPrompt string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, codexCallTimeout)
	defer cancel()

	// Private (0700) scratch space: an empty working root for the agent and
	// the answer file. Removed afterwards so no analysis output lingers.
	tmpDir, err := os.MkdirTemp("", "heimdall-codex-*")
	if err != nil {
		return "", fmt.Errorf("creating codex temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	workDir := filepath.Join(tmpDir, "work")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		return "", fmt.Errorf("creating codex work dir: %w", err)
	}
	outFile := filepath.Join(tmpDir, "last-message.txt")

	args, err := codexArgs(model, workDir, outFile)
	if err != nil {
		return "", err
	}

	start := time.Now()
	stdout, runErr := c.runner(callCtx, c.binPath, args, userPrompt)

	if errors.Is(runErr, exec.ErrNotFound) {
		return "", fmt.Errorf("codex CLI not found on PATH -- install Codex (https://developers.openai.com/codex) or choose another --analyzer")
	}

	failure, usage := parseCodexEvents(stdout)
	if failure != "" {
		return "", fmt.Errorf("codex returned an error: %s (if you are not signed in, run 'codex login')", truncateForError(failure))
	}
	if runErr != nil {
		return "", fmt.Errorf("codex CLI failed: %w", runErr)
	}

	out, err := os.ReadFile(outFile)
	if err != nil {
		return "", fmt.Errorf("codex produced no final message: %w", err)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("codex returned an empty result")
	}

	if usage != nil {
		log.Printf("analyzer: model=%s input_tokens=%d output_tokens=%d latency=%s",
			model, usage.InputTokens, usage.OutputTokens, time.Since(start).Round(time.Millisecond))
	}
	return text, nil
}

// parseCodexEvents scans `codex exec --json` output. It returns a failure
// message when the turn failed -- a turn.failed event, or an error event with
// no turn.completed after it (an error event followed by a completed turn is
// a recovered transient, e.g. a stream reconnect) -- and the turn's token
// usage when reported. Unparseable lines are ignored so a future event type
// cannot break analysis.
func parseCodexEvents(stdout string) (failure string, usage *codexUsage) {
	var turnFailed, lastError string
	completed := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev codexEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "turn.failed":
			turnFailed = "turn failed"
			if ev.Error != nil && ev.Error.Message != "" {
				turnFailed = ev.Error.Message
			}
		case "error":
			lastError = ev.Message
		case "turn.completed":
			completed = true
			if ev.Usage != nil {
				usage = ev.Usage
			}
		}
	}
	switch {
	case turnFailed != "":
		return turnFailed, usage
	case !completed && lastError != "":
		return lastError, usage
	default:
		return "", usage
	}
}
