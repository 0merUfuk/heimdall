package analyzer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: CodexAnalyzer must satisfy Analyzer.
var _ Analyzer = (*CodexAnalyzer)(nil)

// codexUsageLimitJSONL is the exact event stream codex-cli 0.154.0 emitted
// when the account's usage limit was reached (captured 2026-09-19).
const codexUsageLimitJSONL = `{"type":"thread.started","thread_id":"01a0b680-442e-70d2-a497-23a46fdc9cf5"}
{"type":"turn.started"}
{"type":"error","message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at 2:33 PM."}
{"type":"turn.failed","error":{"message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at 2:33 PM."}}
`

// codexSuccessJSONL is a successful turn: a recovered transient error event
// followed by turn.completed with usage.
const codexSuccessJSONL = `{"type":"thread.started","thread_id":"t1"}
{"type":"turn.started"}
{"type":"error","message":"Reconnecting... 1/5"}
{"type":"turn.completed","usage":{"input_tokens":1500,"cached_input_tokens":0,"output_tokens":400}}
`

// argValue returns the value following flag in args ("" if absent).
func argValue(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// configOverrides returns every `-c key=value` override in args, by key.
func configOverrides(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" {
			if k, v, ok := strings.Cut(args[i+1], "="); ok {
				out[k] = v
			}
		}
	}
	return out
}

// codexStub fakes `codex exec`: it writes answer to the -o file (as the real
// CLI does with --output-last-message) and prints jsonl to stdout.
func codexStub(answer, jsonl string, runErr error, calls *atomic.Int32, gotArgs *[]string, gotStdin *string) commandRunner {
	return func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		if calls != nil {
			calls.Add(1)
		}
		if gotArgs != nil {
			*gotArgs = append([]string(nil), args...)
		}
		if gotStdin != nil {
			*gotStdin = stdin
		}
		if answer != "" {
			if err := os.WriteFile(argValue(args, "-o"), []byte(answer), 0o600); err != nil {
				return "", err
			}
		}
		return jsonl, runErr
	}
}

func TestCodexAnalyzer_SuccessfulAnalysis(t *testing.T) {
	var calls atomic.Int32
	a := NewCodexAnalyzer().WithRunner(codexStub(sampleAnalysisJSON(), codexSuccessJSONL, nil, &calls, nil, nil))

	note, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if note.IsFallback {
		t.Fatalf("IsFallback: got true (summary %q)", note.Summary)
	}
	if !strings.Contains(note.Summary, "Sprint review") {
		t.Errorf("Summary: got %q", note.Summary)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("runner calls: got %d, want 1 (a recovered transient error must not fail the turn)", got)
	}
}

// TestCodexAnalyzer_ArgsAndStdin pins the isolation and cost flags: cheapest
// model at low effort by default, user config ignored, strict config, no
// project docs, read-only sandbox in an empty private dir, instructions kept
// separate from the transcript (which arrives on stdin).
func TestCodexAnalyzer_ArgsAndStdin(t *testing.T) {
	var args []string
	var stdin string
	var workDir string
	runner := func(ctx context.Context, name string, a []string, in string) (string, error) {
		args, stdin = a, in
		workDir = argValue(a, "-C")
		entries, err := os.ReadDir(workDir)
		if err != nil || len(entries) != 0 {
			t.Errorf("work dir %q must exist and be empty during the call (entries=%d err=%v)", workDir, len(entries), err)
		}
		if info, err := os.Stat(filepath.Dir(workDir)); err == nil && info.Mode().Perm() != 0o700 {
			t.Errorf("temp dir permissions: got %v, want 0700", info.Mode().Perm())
		}
		return codexStub(sampleAnalysisJSON(), codexSuccessJSONL, nil, nil, nil, nil)(ctx, name, a, in)
	}
	a := NewCodexAnalyzer().WithRunner(runner)

	if _, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if args[0] != "exec" || args[len(args)-1] != "-" {
		t.Errorf("args must be `exec ... -` (prompt from stdin), got %v", args)
	}
	for _, flag := range []string{"--ignore-user-config", "--strict-config", "--ephemeral", "--skip-git-repo-check", "--ignore-rules", "--json"} {
		found := false
		for _, a := range args {
			if a == flag {
				found = true
			}
		}
		if !found {
			t.Errorf("missing flag %s in %v", flag, args)
		}
	}
	if got := argValue(args, "--sandbox"); got != "read-only" {
		t.Errorf("--sandbox: got %q, want read-only", got)
	}
	if got := argValue(args, "-m"); got != DefaultCodexModel {
		t.Errorf("-m: got %q, want default %q", got, DefaultCodexModel)
	}

	overrides := configOverrides(args)
	if overrides["model_reasoning_effort"] != `"low"` {
		t.Errorf("model_reasoning_effort: got %q, want \"low\"", overrides["model_reasoning_effort"])
	}
	if overrides["project_doc_max_bytes"] != "0" {
		t.Errorf("project_doc_max_bytes: got %q, want 0", overrides["project_doc_max_bytes"])
	}
	// The instructions value is a JSON string literal (a valid TOML basic
	// string); decoding it must give back systemPrompt exactly.
	var instr string
	if err := json.Unmarshal([]byte(overrides["developer_instructions"]), &instr); err != nil {
		t.Fatalf("developer_instructions is not a quoted string literal: %v", err)
	}
	if instr != systemPrompt {
		t.Error("developer_instructions must carry systemPrompt verbatim")
	}

	if strings.Contains(stdin, "CRITICAL RULES") {
		t.Error("stdin must carry only the user prompt; the system prompt belongs in developer_instructions")
	}
	if !strings.Contains(stdin, "<transcript>") {
		t.Error("stdin must carry the <transcript> block")
	}

	if _, err := os.Stat(filepath.Dir(workDir)); !os.IsNotExist(err) {
		t.Errorf("temp dir %q must be removed after the call (stat err=%v)", filepath.Dir(workDir), err)
	}
}

func TestCodexAnalyzer_CustomModel(t *testing.T) {
	var args []string
	a := NewCodexAnalyzer().WithRunner(codexStub(sampleAnalysisJSON(), codexSuccessJSONL, nil, nil, &args, nil))
	if _, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{Model: "gpt-5.6-terra"}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got := argValue(args, "-m"); got != "gpt-5.6-terra" {
		t.Errorf("-m: got %q, want gpt-5.6-terra", got)
	}
}

func TestCodexAnalyzer_UsageLimit_Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the full 1s+2s retry backoff")
	}
	var calls atomic.Int32
	a := NewCodexAnalyzer().WithRunner(codexStub("", codexUsageLimitJSONL, &exec.ExitError{}, &calls, nil, nil))

	note, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true")
	}
	if !strings.Contains(note.Summary, "usage limit") {
		t.Errorf("Summary should surface Codex's own message, got %q", note.Summary)
	}
	if got := calls.Load(); got != int32(maxRetries) {
		t.Errorf("runner calls: got %d, want %d", got, maxRetries)
	}
}

func TestCodexAnalyzer_BinaryNotFound(t *testing.T) {
	a := NewCodexAnalyzer().WithRunner(codexStub("", "", exec.ErrNotFound, nil, nil, nil))
	_, err := a.callOnce(context.Background(), DefaultCodexModel, "prompt")
	if err == nil || !strings.Contains(err.Error(), "codex CLI not found on PATH") {
		t.Errorf("err: got %v, want a clear not-found message", err)
	}
}

func TestCodexAnalyzer_EmptyResult(t *testing.T) {
	a := NewCodexAnalyzer().WithRunner(codexStub("   \n", codexSuccessJSONL, nil, nil, nil, nil))
	_, err := a.callOnce(context.Background(), DefaultCodexModel, "prompt")
	if err == nil || !strings.Contains(err.Error(), "empty result") {
		t.Errorf("err: got %v, want empty-result error", err)
	}
}

func TestCodexAnalyzer_NoOutputFile(t *testing.T) {
	a := NewCodexAnalyzer().WithRunner(codexStub("", codexSuccessJSONL, nil, nil, nil, nil))
	_, err := a.callOnce(context.Background(), DefaultCodexModel, "prompt")
	if err == nil || !strings.Contains(err.Error(), "no final message") {
		t.Errorf("err: got %v, want no-final-message error", err)
	}
}

func TestCodexAnalyzer_ProcessFailedWithoutEvents(t *testing.T) {
	a := NewCodexAnalyzer().WithRunner(codexStub("", "", &exec.ExitError{}, nil, nil, nil))
	_, err := a.callOnce(context.Background(), DefaultCodexModel, "prompt")
	if err == nil || !strings.Contains(err.Error(), "codex CLI failed") {
		t.Errorf("err: got %v, want codex CLI failed", err)
	}
}

func TestParseCodexEvents(t *testing.T) {
	tests := []struct {
		name        string
		jsonl       string
		wantFailure string
		wantUsage   bool
	}{
		{"usage limit", codexUsageLimitJSONL, "usage limit", false},
		{"recovered transient", codexSuccessJSONL, "", true},
		{"error without completion", `{"type":"error","message":"boom"}`, "boom", false},
		{"turn.failed without message", `{"type":"turn.failed"}`, "turn failed", false},
		{"garbage lines ignored", "not json\n{\"type\":\"turn.completed\"}\n", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure, usage := parseCodexEvents(tt.jsonl)
			if tt.wantFailure == "" && failure != "" {
				t.Errorf("failure: got %q, want none", failure)
			}
			if tt.wantFailure != "" && !strings.Contains(failure, tt.wantFailure) {
				t.Errorf("failure: got %q, want to contain %q", failure, tt.wantFailure)
			}
			if (usage != nil) != tt.wantUsage {
				t.Errorf("usage present: got %v, want %v", usage != nil, tt.wantUsage)
			}
			if usage != nil && (usage.InputTokens != 1500 || usage.OutputTokens != 400) {
				t.Errorf("usage: got %+v", *usage)
			}
		})
	}
}
