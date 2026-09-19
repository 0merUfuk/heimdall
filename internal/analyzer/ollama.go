package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: OllamaAnalyzer must satisfy Analyzer.
var _ Analyzer = (*OllamaAnalyzer)(nil)

const (
	// DefaultOllamaModel is the default local model: the smallest model
	// tier that reliably produces the analysis JSON while fitting (with a
	// 32K context) in the unified memory of a 24 GB Apple Silicon Mac.
	DefaultOllamaModel = "qwen3:14b"

	// DefaultOllamaBaseURL is Ollama's standard local listen address.
	DefaultOllamaBaseURL = "http://localhost:11434"

	// DefaultOllamaMaxContext caps the context window heimdall requests.
	// The KV cache grows linearly with it, so this bounds memory use;
	// ~32K tokens covers a 1-2 hour English meeting in a single pass.
	DefaultOllamaMaxContext = 32768

	// ollamaOutputReserve is the output budget (num_predict) and the
	// headroom kept free in the context window for the response. The
	// Anthropic backend uses the same 4096-token cap (defaultMaxTokens).
	ollamaOutputReserve = 4096

	// ollamaCharsPerToken is a deliberately conservative bytes-per-token
	// ratio for sizing num_ctx. Measured against qwen2.5 on a real prompt:
	// 45.3K chars -> 12,971 tokens (~3.5). Counting bytes rather than runes
	// inflates non-ASCII text (Turkish diacritics are 2 bytes), which is the
	// right direction: those languages also tokenize denser.
	ollamaCharsPerToken = 3

	// ollamaMinContext is the smallest window ever requested.
	ollamaMinContext = 8192

	// ollamaHTTPTimeout bounds one HTTP call. Generous by design: a long
	// transcript through a 14B model plus a cold model load is minutes.
	ollamaHTTPTimeout = 15 * time.Minute
)

// ollamaContextBuckets are the only num_ctx values requested. Ollama reloads
// the model whenever num_ctx changes, so sizing to fixed buckets (rather than
// to the exact prompt) keeps repeat runs on a warm model.
var ollamaContextBuckets = []int{8192, 16384, 32768, 40960, 65536, 131072}

// OllamaAnalyzer implements Analyzer against a local Ollama server's native
// /api/chat endpoint, so meeting content never leaves the machine.
//
// It deliberately does NOT use Ollama's OpenAI-compatible
// /v1/chat/completions endpoint (or its Anthropic-compatible /v1/messages
// one): neither lets the caller set the context window per request, and
// Ollama silently truncates an over-long prompt to its default window --
// measured on Ollama 0.20.2: a 60-minute transcript (12,971 tokens) was cut
// to 4,096 from the front, dropping the system prompt and the meeting's
// early action items while still returning well-formed, plausible JSON. See
// .claude/DECISIONS.md ID-011. The native endpoint sets num_ctx per request
// and also enforces the output JSON schema at decode time (`format`).
type OllamaAnalyzer struct {
	baseURL    string
	maxContext int
	client     *http.Client
}

// NewOllamaAnalyzer creates an OllamaAnalyzer. An empty baseURL selects
// DefaultOllamaBaseURL; maxContext <= 0 selects DefaultOllamaMaxContext.
func NewOllamaAnalyzer(baseURL string, maxContext int) *OllamaAnalyzer {
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	if maxContext <= 0 {
		maxContext = DefaultOllamaMaxContext
	}
	return &OllamaAnalyzer{
		baseURL:    strings.TrimRight(baseURL, "/"),
		maxContext: maxContext,
		client: &http.Client{
			Timeout: ollamaHTTPTimeout,
			// Never follow redirects. Ollama does not issue them, and Go
			// re-sends the POST body on 307/308 -- so a redirecting service
			// on the "local" address could forward the transcript to another
			// host while heimdall reports the run as on-device.
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return fmt.Errorf("refusing redirect to %s: the transcript must go only to the configured Ollama server", req.URL.Redacted())
			},
		},
	}
}

// WithHTTPClient sets a custom HTTP client. Used in tests.
func (o *OllamaAnalyzer) WithHTTPClient(client *http.Client) *OllamaAnalyzer {
	o.client = client
	return o
}

// BaseURL returns the Ollama server URL this analyzer talks to.
func (o *OllamaAnalyzer) BaseURL() string {
	return o.baseURL
}

// IsLoopbackURL reports whether rawURL points at this machine (localhost or
// a loopback IP). Used to tell the user honestly whether "local" analysis
// really stays on-device when ollama.base_url has been pointed elsewhere.
func IsLoopbackURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Summarize analyzes a complete meeting transcript with a local model.
//
// Before any model call it sizes the context window to the prompt. A
// transcript that cannot fit the model's window is never sent (a truncated
// prompt yields confidently incomplete notes); instead the V-009 fallback
// note is returned with an actionable message, exactly like an exhausted
// retry. Retry/backoff/parse/fallback is shared with every other backend via
// summarizeWithRetry.
func (o *OllamaAnalyzer) Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	model := opts.Model
	if model == "" {
		model = DefaultOllamaModel
	}

	if len(segments) == 0 {
		// summarizeWithRetry short-circuits empty transcripts without a call.
		return summarizeWithRetry(ctx, segments, opts, nil)
	}

	userPrompt := buildUserPrompt(segments, opts)
	needed := estimateTokens(systemPrompt+userPrompt) + ollamaOutputReserve

	modelMax, err := o.modelContextLength(ctx, model)
	if err != nil {
		return buildFallbackNote(segments, err), nil
	}
	limit := o.maxContext
	if modelMax > 0 && modelMax < limit {
		limit = modelMax
	}
	if needed > limit {
		return buildFallbackNote(segments, fmt.Errorf(
			"transcript needs ~%d tokens but the local context limit is %d (model %s max %d, ollama.max_context %d); "+
				"nothing was sent to the model. Re-run with a larger-context model, raise ollama.max_context if memory allows, "+
				"or explicitly choose a cloud backend (--analyzer api)",
			needed, limit, model, modelMax, o.maxContext)), nil
	}
	numCtx := pickContextBucket(needed, limit)

	return summarizeWithRetry(ctx, segments, opts, func(ctx context.Context, userPrompt string) (string, error) {
		return o.callOnce(ctx, model, numCtx, userPrompt)
	})
}

// estimateTokens is a conservative (over-)estimate of a prompt's token
// count; see ollamaCharsPerToken.
func estimateTokens(s string) int {
	return len(s)/ollamaCharsPerToken + 1
}

// pickContextBucket returns the smallest bucket >= needed, clamped to limit.
// Callers guarantee needed <= limit.
func pickContextBucket(needed, limit int) int {
	if needed < ollamaMinContext {
		needed = ollamaMinContext
	}
	for _, b := range ollamaContextBuckets {
		if b >= needed {
			if b > limit {
				return limit
			}
			return b
		}
	}
	return limit
}

// ollamaErrorResponse is Ollama's error body shape: {"error": "..."}.
type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// modelContextLength asks /api/show for the model's trained context length
// ("<family>.context_length" in model_info). It doubles as the reachability
// and model-installed preflight, so both failures surface as one clear,
// actionable error before any transcript is sent. Returns 0 (no error) when
// the server does not report a length -- the configured cap then applies.
func (o *OllamaAnalyzer) modelContextLength(ctx context.Context, model string) (int, error) {
	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ollama is not reachable at %s -- start it with `ollama serve` (or open the Ollama app), or choose another --analyzer: %w", o.baseURL, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading ollama response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("ollama model %q is not installed -- run `ollama pull %s`", model, model)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ollama /api/show error (status %d): %s", resp.StatusCode, ollamaErrorText(respBody))
	}

	var show struct {
		ModelInfo map[string]any `json:"model_info"`
	}
	if err := json.Unmarshal(respBody, &show); err != nil {
		return 0, fmt.Errorf("parsing ollama /api/show response: %w", err)
	}
	for k, v := range show.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			if n, ok := v.(float64); ok && n > 0 {
				return int(n), nil
			}
		}
	}
	return 0, nil
}

// ollamaChatRequest is the native /api/chat request body.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []apiMessage    `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format"`
	Think    bool            `json:"think"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx"`
	NumPredict  int     `json:"num_predict"`
}

// ollamaChatResponse is the subset of the native /api/chat response we use.
type ollamaChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// errOllamaTruncated marks a response produced from a prompt that filled the
// whole context window, i.e. Ollama may have silently dropped part of it.
var errOllamaTruncated = errors.New("ollama filled the entire context window; the prompt may have been truncated")

// callOnce performs one native /api/chat call and returns the model's raw
// text (the analysis JSON). It does not retry -- summarizeWithRetry owns that.
func (o *OllamaAnalyzer) callOnce(ctx context.Context, model string, numCtx int, userPrompt string) (string, error) {
	start := time.Now()

	reqBody := ollamaChatRequest{
		Model: model,
		Messages: []apiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
		Format: analysisJSONSchema,
		// Reasoning models (qwen3, ...) would otherwise spend minutes
		// "thinking" before a bounded extraction task. Verified accepted
		// (and ignored) by non-thinking models on Ollama 0.20.2.
		Think: false,
		Options: ollamaOptions{
			Temperature: 0,
			NumCtx:      numCtx,
			NumPredict:  ollamaOutputReserve,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshalling ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling ollama at %s: %w", o.baseURL, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, ollamaErrorText(respBody))
	}

	var chat ollamaChatResponse
	if err := json.Unmarshal(respBody, &chat); err != nil {
		return "", fmt.Errorf("unmarshalling ollama response: %w", err)
	}

	// Belt and braces: num_ctx is sized to fit, so this should never fire.
	// If it does, the output was generated from a partial transcript and
	// must not be rendered as if complete.
	if chat.PromptEvalCount >= numCtx {
		return "", fmt.Errorf("%w (prompt_eval_count=%d, num_ctx=%d)", errOllamaTruncated, chat.PromptEvalCount, numCtx)
	}
	if chat.DoneReason == "length" {
		return "", fmt.Errorf("ollama output hit the %d-token limit before the JSON was complete", ollamaOutputReserve)
	}
	if strings.TrimSpace(chat.Message.Content) == "" {
		return "", fmt.Errorf("ollama returned an empty response")
	}

	// Same observability line as the Anthropic backend (ID-010): token
	// counts and latency, no dollar figure (local inference has none).
	log.Printf("analyzer: model=%s input_tokens=%d output_tokens=%d num_ctx=%d latency=%s",
		model, chat.PromptEvalCount, chat.EvalCount, numCtx, time.Since(start).Round(time.Millisecond))

	return chat.Message.Content, nil
}

// ollamaErrorText extracts Ollama's {"error": "..."} message, falling back
// to the (truncated) raw body.
func ollamaErrorText(body []byte) string {
	var e ollamaErrorResponse
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return truncateForError(string(body))
}
