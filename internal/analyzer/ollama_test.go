package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: OllamaAnalyzer must satisfy Analyzer.
var _ Analyzer = (*OllamaAnalyzer)(nil)

// fakeOllama is an httptest stand-in for an Ollama server's /api/show and
// /api/chat endpoints. Response shapes mirror what Ollama 0.20.2 actually
// returned in the probes recorded in .claude/DECISIONS.md ID-011.
type fakeOllama struct {
	contextLength int    // /api/show model_info["qwen3.context_length"]; 0 = omit
	showStatus    int    // /api/show status; 0 = 200
	chatStatus    int    // /api/chat status; 0 = 200
	content       string // /api/chat message.content
	doneReason    string // "" = "stop"
	promptEval    int    // prompt_eval_count; -1 = echo request num_ctx
	evalCount     int

	mu        sync.Mutex
	chatCalls atomic.Int32
	showCalls atomic.Int32
	lastChat  map[string]any
}

func (f *fakeOllama) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			f.showCalls.Add(1)
			if f.showStatus != 0 && f.showStatus != http.StatusOK {
				w.WriteHeader(f.showStatus)
				_, _ = w.Write([]byte(`{"error":"model 'nope:1b' not found"}`))
				return
			}
			info := map[string]any{"general.architecture": "qwen3"}
			if f.contextLength > 0 {
				info["qwen3.context_length"] = f.contextLength
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model_info": info})
		case "/api/chat":
			f.chatCalls.Add(1)
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.lastChat = req
			f.mu.Unlock()
			if f.chatStatus != 0 && f.chatStatus != http.StatusOK {
				w.WriteHeader(f.chatStatus)
				_, _ = w.Write([]byte(`{"error":"model runner crashed"}`))
				return
			}
			promptEval := f.promptEval
			if promptEval == -1 {
				promptEval = int(req["options"].(map[string]any)["num_ctx"].(float64))
			}
			done := f.doneReason
			if done == "" {
				done = "stop"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model":             req["model"],
				"message":           map[string]any{"role": "assistant", "content": f.content},
				"done":              true,
				"done_reason":       done,
				"prompt_eval_count": promptEval,
				"eval_count":        f.evalCount,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeOllama) lastChatRequest() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastChat
}

func TestOllamaAnalyzer_SuccessfulAnalysis(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON(), promptEval: 900, evalCount: 300}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if note.IsFallback {
		t.Fatalf("IsFallback: got true, want false (summary %q)", note.Summary)
	}
	if !strings.Contains(note.Summary, "Sprint review") {
		t.Errorf("Summary: got %q, want to contain 'Sprint review'", note.Summary)
	}
	if note.SpeakerMap[2] != "Sarah" {
		t.Errorf("SpeakerMap[2]: got %q, want %q", note.SpeakerMap[2], "Sarah")
	}
	if got := fake.chatCalls.Load(); got != 1 {
		t.Errorf("chat calls: got %d, want 1", got)
	}
}

// TestOllamaAnalyzer_RequestShape pins every field of the native /api/chat
// request that correctness depends on: an explicit num_ctx (the whole point
// of using the native endpoint), schema-enforced output, thinking disabled,
// deterministic temperature, and the system/user split (V-014).
func TestOllamaAnalyzer_RequestShape(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON(), promptEval: 900}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	if _, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	req := fake.lastChatRequest()

	if req["model"] != DefaultOllamaModel {
		t.Errorf("model: got %v, want default %q", req["model"], DefaultOllamaModel)
	}
	if req["stream"] != false {
		t.Errorf("stream: got %v, want false", req["stream"])
	}
	if req["think"] != false {
		t.Errorf("think: got %v, want false", req["think"])
	}
	format, ok := req["format"].(map[string]any)
	if !ok || format["type"] != "object" {
		t.Fatalf("format: got %v, want the analysis JSON schema object", req["format"])
	}
	opts := req["options"].(map[string]any)
	if opts["num_ctx"] != float64(8192) {
		t.Errorf("num_ctx: got %v, want 8192 (smallest bucket for a short transcript)", opts["num_ctx"])
	}
	if opts["num_predict"] != float64(ollamaOutputReserve) {
		t.Errorf("num_predict: got %v, want %d", opts["num_predict"], ollamaOutputReserve)
	}
	if opts["temperature"] != float64(0) {
		t.Errorf("temperature: got %v, want 0", opts["temperature"])
	}

	msgs := req["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages: got %d, want 2 (system + user)", len(msgs))
	}
	sys := msgs[0].(map[string]any)
	user := msgs[1].(map[string]any)
	if sys["role"] != "system" || sys["content"] != systemPrompt {
		t.Errorf("messages[0]: want role=system with systemPrompt, got role=%v", sys["role"])
	}
	if user["role"] != "user" || !strings.Contains(user["content"].(string), "<transcript>") {
		t.Errorf("messages[1]: want role=user containing the <transcript> block, got %v", user["role"])
	}
}

func TestOllamaAnalyzer_CustomModel(t *testing.T) {
	fake := &fakeOllama{contextLength: 32768, content: sampleAnalysisJSON(), promptEval: 900}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	if _, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{Model: "qwen2.5:7b"}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got := fake.lastChatRequest()["model"]; got != "qwen2.5:7b" {
		t.Errorf("model: got %v, want qwen2.5:7b", got)
	}
}

// longSegments builds a transcript of roughly n bytes of speech.
func longSegments(n int) []heimdall.Segment {
	line := "We went through the onboarding funnel numbers again and nobody committed to anything yet. "
	var segs []heimdall.Segment
	for total := 0; total < n; total += len(line) {
		i := len(segs)
		segs = append(segs, heimdall.Segment{Speaker: i % 3, Text: line, Start: time.Duration(i) * 9 * time.Second, IsFinal: true})
	}
	return segs
}

// TestOllamaAnalyzer_TranscriptTooLong_NeverSent is the regression test for
// the silent-truncation failure measured in ID-011: a transcript that cannot
// fit the context window must produce a fallback note WITHOUT any model call,
// never a plausible-looking note built from a truncated prompt.
func TestOllamaAnalyzer_TranscriptTooLong_NeverSent(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON()}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 8192)
	note, err := a.Summarize(context.Background(), longSegments(60_000), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true for a transcript exceeding the context limit")
	}
	if !strings.Contains(note.Summary, "context limit") || !strings.Contains(note.Summary, "nothing was sent") {
		t.Errorf("Summary should explain the context-limit refusal, got %q", note.Summary)
	}
	if got := fake.chatCalls.Load(); got != 0 {
		t.Errorf("chat calls: got %d, want 0 -- an over-long transcript must never be sent", got)
	}
}

// TestOllamaAnalyzer_ModelContextCapsLimit: the model's own trained context
// length wins when it is smaller than the configured cap.
func TestOllamaAnalyzer_ModelContextCapsLimit(t *testing.T) {
	fake := &fakeOllama{contextLength: 8192, content: sampleAnalysisJSON()}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 131072)
	note, _ := a.Summarize(context.Background(), longSegments(40_000), heimdall.AnalyzeOpts{})
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true -- model context 8192 cannot hold ~17K tokens")
	}
	if got := fake.chatCalls.Load(); got != 0 {
		t.Errorf("chat calls: got %d, want 0", got)
	}
}

func TestOllamaAnalyzer_LongTranscriptGetsLargerBucket(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON(), promptEval: 12000}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	// ~20K chars of speech -> ~25K-byte prompt (timestamps, speaker labels,
	// system prompt) -> ~8.3K estimated tokens + 4096 output reserve.
	note, err := a.Summarize(context.Background(), longSegments(20_000), heimdall.AnalyzeOpts{})
	if err != nil || note.IsFallback {
		t.Fatalf("Summarize: err=%v fallback=%v summary=%q", err, note.IsFallback, note.Summary)
	}
	opts := fake.lastChatRequest()["options"].(map[string]any)
	if opts["num_ctx"] != float64(16384) {
		t.Errorf("num_ctx: got %v, want 16384 for a ~12.4K-token requirement", opts["num_ctx"])
	}
}

func TestOllamaAnalyzer_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens here any more

	a := NewOllamaAnalyzer(url, 0)
	note, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true when Ollama is not running")
	}
	if !strings.Contains(note.Summary, "ollama serve") {
		t.Errorf("Summary should tell the user to start Ollama, got %q", note.Summary)
	}
}

func TestOllamaAnalyzer_ModelNotInstalled(t *testing.T) {
	fake := &fakeOllama{showStatus: http.StatusNotFound}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, _ := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{Model: "nope:1b"})
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true")
	}
	if !strings.Contains(note.Summary, "ollama pull nope:1b") {
		t.Errorf("Summary should give the exact pull command, got %q", note.Summary)
	}
	if got := fake.chatCalls.Load(); got != 0 {
		t.Errorf("chat calls: got %d, want 0", got)
	}
}

// TestOllamaAnalyzer_TruncationGuard: if Ollama reports a prompt that filled
// the entire window, the output is treated as untrustworthy and never
// rendered as a real analysis.
func TestOllamaAnalyzer_TruncationGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the full 1s+2s retry backoff")
	}
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON(), promptEval: -1}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: unexpected error: %v", err)
	}
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true when the prompt filled the context window")
	}
	if !strings.Contains(note.Summary, "truncated") {
		t.Errorf("Summary should mention truncation, got %q", note.Summary)
	}
	if got := fake.chatCalls.Load(); got != int32(maxRetries) {
		t.Errorf("chat calls: got %d, want %d (retried like any other failure)", got, maxRetries)
	}
}

func TestOllamaAnalyzer_OutputHitLengthLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the full 1s+2s retry backoff")
	}
	fake := &fakeOllama{contextLength: 40960, content: `{"summary": "cut o`, doneReason: "length", promptEval: 900}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, _ := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true")
	}
	if !strings.Contains(note.Summary, "token limit") {
		t.Errorf("Summary should mention the output limit, got %q", note.Summary)
	}
}

func TestOllamaAnalyzer_ServerErrorThenFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the full 1s+2s retry backoff")
	}
	fake := &fakeOllama{contextLength: 40960, chatStatus: http.StatusInternalServerError}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, _ := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{})
	if !note.IsFallback {
		t.Fatal("IsFallback: got false, want true")
	}
	if !strings.Contains(note.Summary, "model runner crashed") {
		t.Errorf("Summary should carry Ollama's error text, got %q", note.Summary)
	}
}

func TestOllamaAnalyzer_EmptyTranscript_NoCalls(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON()}
	srv := fake.server(t)

	a := NewOllamaAnalyzer(srv.URL, 0)
	note, err := a.Summarize(context.Background(), nil, heimdall.AnalyzeOpts{})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if note.IsFallback {
		t.Error("IsFallback: got true, want false for an empty transcript")
	}
	if fake.showCalls.Load()+fake.chatCalls.Load() != 0 {
		t.Error("an empty transcript must not reach the server at all")
	}
}

func TestOllamaAnalyzer_LogsTokenUsage(t *testing.T) {
	fake := &fakeOllama{contextLength: 40960, content: sampleAnalysisJSON(), promptEval: 1234, evalCount: 321}
	srv := fake.server(t)

	var buf bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(origOut); log.SetFlags(origFlags) })

	a := NewOllamaAnalyzer(srv.URL, 0)
	if _, err := a.Summarize(context.Background(), sampleSegments(), heimdall.AnalyzeOpts{}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"model=" + DefaultOllamaModel, "input_tokens=1234", "output_tokens=321", "num_ctx=8192", "latency="} {
		if !strings.Contains(got, want) {
			t.Errorf("log line %q missing %q", got, want)
		}
	}
}

func TestNewOllamaAnalyzer_Defaults(t *testing.T) {
	a := NewOllamaAnalyzer("", 0)
	if a.BaseURL() != DefaultOllamaBaseURL {
		t.Errorf("BaseURL: got %q, want %q", a.BaseURL(), DefaultOllamaBaseURL)
	}
	if a.maxContext != DefaultOllamaMaxContext {
		t.Errorf("maxContext: got %d, want %d", a.maxContext, DefaultOllamaMaxContext)
	}
	if got := NewOllamaAnalyzer("http://127.0.0.1:11434/", 0).BaseURL(); got != "http://127.0.0.1:11434" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}

func TestPickContextBucket(t *testing.T) {
	tests := []struct {
		needed, limit, want int
	}{
		{100, 32768, 8192},
		{8192, 32768, 8192},
		{8193, 32768, 16384},
		{16385, 32768, 32768},
		{20000, 24000, 24000}, // bucket 32768 exceeds limit -> clamp
		{32769, 40960, 40960},
		{40000, 40960, 40960},
	}
	for _, tt := range tests {
		if got := pickContextBucket(tt.needed, tt.limit); got != tt.want {
			t.Errorf("pickContextBucket(%d, %d) = %d, want %d", tt.needed, tt.limit, got, tt.want)
		}
	}
}

// TestEstimateTokens_Conservative: the estimate must not undercount the
// measured real prompt (45.3K chars -> 12,971 tokens on Ollama 0.20.2).
func TestEstimateTokens_Conservative(t *testing.T) {
	prompt := strings.Repeat("a", 45300)
	if got := estimateTokens(prompt); got < 12971 {
		t.Errorf("estimateTokens(45.3K chars) = %d, undercounts the measured 12971", got)
	}
}

func TestIsLoopbackURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://localhost:11434", true},
		{"http://LOCALHOST:11434", true},
		{"http://127.0.0.1:11434", true},
		{"http://[::1]:11434", true},
		{"http://192.168.1.20:11434", false},
		{"https://ollama.example.com", false},
		{"::not a url", false},
	}
	for _, tt := range tests {
		if got := IsLoopbackURL(tt.url); got != tt.want {
			t.Errorf("IsLoopbackURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}
