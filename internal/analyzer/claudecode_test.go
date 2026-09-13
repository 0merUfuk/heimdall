package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: ClaudeCodeAnalyzer must satisfy Analyzer.
var _ Analyzer = (*ClaudeCodeAnalyzer)(nil)

// wrapInClaudeCodeEnvelope wraps a text string in the `claude -p
// --output-format json` result envelope shape.
func wrapInClaudeCodeEnvelope(result string, isError bool) string {
	b, _ := json.Marshal(claudeCodeEnvelope{Type: "result", Result: result, IsError: isError})
	return string(b)
}

// stubRunner returns a commandRunner that ignores its arguments and always
// returns the given stdout/err, recording the args/stdin it was called with.
func stubRunner(stdout string, err error, capturedArgs *[]string, capturedStdin *string) commandRunner {
	return func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		if capturedArgs != nil {
			*capturedArgs = args
		}
		if capturedStdin != nil {
			*capturedStdin = stdin
		}
		return stdout, err
	}
}

func TestClaudeCodeAnalyzer_SuccessfulAnalysis(t *testing.T) {
	var callCount atomic.Int32
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		callCount.Add(1)
		return wrapInClaudeCodeEnvelope(sampleAnalysisJSON(), false), nil
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	segments := sampleSegments()

	note, err := analyzer.Summarize(context.Background(), segments, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if note == nil {
		t.Fatal("Summarize: returned nil note")
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("runner call count: got %d, want 1", got)
	}
	if !strings.Contains(note.Summary, "Sprint review") {
		t.Errorf("Summary: got %q, want to contain 'Sprint review'", note.Summary)
	}
	if len(note.Decisions) != 1 {
		t.Errorf("Decisions length: got %d, want 1", len(note.Decisions))
	}
	if note.SpeakerMap[2] != "Sarah" {
		t.Errorf("SpeakerMap[2]: got %q, want %q", note.SpeakerMap[2], "Sarah")
	}
}

func TestClaudeCodeAnalyzer_NotLoggedIn(t *testing.T) {
	// The real CLI exits non-zero AND emits a well-formed JSON envelope with
	// is_error:true and a "Not logged in" result -- verified empirically
	// against `claude -p` directly. callOnce must surface a clear,
	// actionable error rather than a raw JSON dump.
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		return wrapInClaudeCodeEnvelope("Not logged in · Please run /login", true), &exec.ExitError{}
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)

	// Summarize never returns a hard error on exhausted retries (V-009) --
	// it falls back. Assert the fallback note carries the actionable message.
	note, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize should fall back, not error: %v", err)
	}
	if note == nil || !note.IsFallback {
		t.Fatalf("expected a fallback note, got %+v", note)
	}
	if !strings.Contains(note.Summary, "not logged in") && !strings.Contains(note.Summary, "/login") {
		t.Errorf("fallback summary should mention the login remedy, got: %q", note.Summary)
	}
}

func TestClaudeCodeAnalyzer_BinaryNotFound(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	note, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize should fall back, not error: %v", err)
	}
	if note == nil || !note.IsFallback {
		t.Fatalf("expected a fallback note, got %+v", note)
	}
	if !strings.Contains(note.Summary, "not found") && !strings.Contains(note.Summary, "install Claude Code") {
		t.Errorf("fallback summary should mention installing Claude Code, got: %q", note.Summary)
	}
}

func TestClaudeCodeAnalyzer_UnparseableOutput(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		return "not json at all", nil
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	note, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize should fall back, not error: %v", err)
	}
	if note == nil || !note.IsFallback {
		t.Fatalf("expected a fallback note, got %+v", note)
	}
}

func TestClaudeCodeAnalyzer_EmptyResult(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		return wrapInClaudeCodeEnvelope("", false), nil
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	note, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize should fall back, not error: %v", err)
	}
	if note == nil || !note.IsFallback {
		t.Fatalf("expected a fallback note for an empty result, got %+v", note)
	}
}

func TestClaudeCodeAnalyzer_RetryThenSucceed(t *testing.T) {
	var callCount atomic.Int32
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		count := callCount.Add(1)
		if count <= 2 {
			return wrapInClaudeCodeEnvelope("transient failure", true), &exec.ExitError{}
		}
		return wrapInClaudeCodeEnvelope(sampleAnalysisJSON(), false), nil
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	note, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if got := callCount.Load(); got != 3 {
		t.Errorf("runner call count: got %d, want 3", got)
	}
	if !strings.Contains(note.Summary, "Sprint review") {
		t.Errorf("expected the eventual success to be used, got summary: %q", note.Summary)
	}
}

func TestClaudeCodeAnalyzer_ArgsAndStdin(t *testing.T) {
	var capturedArgs []string
	var capturedStdin string
	runner := stubRunner(wrapInClaudeCodeEnvelope(sampleAnalysisJSON(), false), nil, &capturedArgs, &capturedStdin)

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	_, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	for _, want := range []string{"-p", "--bare", "--restricted", "--permission-prompts", "none", "--output-format", "json", "--system-prompt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args should contain %q, got: %v", want, capturedArgs)
		}
	}
	// No --model flag when opts.Model is empty -- let Claude Code use its own default.
	if strings.Contains(joined, "--model") {
		t.Errorf("args should not contain --model when opts.Model is empty, got: %v", capturedArgs)
	}
	// The transcript goes over stdin, not argv (long transcripts could
	// otherwise approach OS argument-length limits).
	if !strings.Contains(capturedStdin, "<transcript>") {
		t.Errorf("stdin should contain the transcript, got: %q", capturedStdin)
	}
}

func TestClaudeCodeAnalyzer_ModelFlagWhenSet(t *testing.T) {
	var capturedArgs []string
	runner := stubRunner(wrapInClaudeCodeEnvelope(sampleAnalysisJSON(), false), nil, &capturedArgs, nil)

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	_, err := analyzer.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{Model: "opus"})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}

	found := false
	for i, a := range capturedArgs {
		if a == "--model" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "opus" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --model opus in args, got: %v", capturedArgs)
	}
}

func TestClaudeCodeAnalyzer_EmptyTranscript(t *testing.T) {
	// No runner call expected -- empty transcript short-circuits before any
	// subprocess is spawned (same contract as ClaudeAnalyzer).
	called := false
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		called = true
		return "", nil
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	note, err := analyzer.Summarize(context.Background(), nil, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if called {
		t.Error("runner should not be invoked for an empty transcript")
	}
	if !strings.Contains(note.Summary, "No transcript") {
		t.Errorf("Summary: got %q, want to contain 'No transcript'", note.Summary)
	}
}

func TestClaudeCodeAnalyzer_ContextTimeout(t *testing.T) {
	// callOnce should respect context cancellation rather than hang forever
	// on a wedged subprocess.
	runner := func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return wrapInClaudeCodeEnvelope(sampleAnalysisJSON(), false), nil
		}
	}

	analyzer := NewClaudeCodeAnalyzer().WithRunner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	note, err := analyzer.Summarize(ctx, sampleSegments(), heimdall.AnalyzeOpts{})
	if note == nil {
		t.Fatal("Summarize should return a non-nil note even on timeout")
	}
	// Either the immediate ctx.Err() propagates, or the fallback path was
	// used -- both are acceptable, matching ClaudeAnalyzer's own
	// ContextCancellation test contract.
	if err == nil && note.Summary == "" {
		t.Error("expected either an error or a populated fallback summary")
	}
}

func TestExecCommandRunner_BinaryNotFound(t *testing.T) {
	_, err := execCommandRunner(context.Background(), "heimdall-this-binary-does-not-exist", nil, "")
	if err == nil {
		t.Fatal("expected an error for a nonexistent binary")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("expected errors.Is(err, exec.ErrNotFound), got: %v", err)
	}
}

func TestExecCommandRunner_RealProcess(t *testing.T) {
	// Exercise the real os/exec path (not the stub) against a portable
	// command so the stdin-piping and stdout-capture wiring is verified
	// end-to-end, without depending on the `claude` binary being installed
	// or authenticated in the test environment.
	out, err := execCommandRunner(context.Background(), "cat", nil, "hello from stdin")
	if err != nil {
		t.Fatalf("execCommandRunner: unexpected error: %v", err)
	}
	if out != "hello from stdin" {
		t.Errorf("stdout: got %q, want %q", out, "hello from stdin")
	}
}
