package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: ClaudeAnalyzer must satisfy Analyzer.
var _ Analyzer = (*ClaudeAnalyzer)(nil)

const (
	// DefaultModel is the default Claude model for meeting analysis.
	// Exported so that CLI commands (record, recover) use a single source of truth.
	DefaultModel = "claude-haiku-4-5"

	// defaultBaseURL is the Anthropic Messages API endpoint.
	defaultBaseURL = "https://api.anthropic.com"

	// defaultMaxTokens is the maximum response tokens for meeting analysis.
	defaultMaxTokens = 4096

	// anthropicVersion is the API version header value.
	anthropicVersion = "2023-06-01"

	// maxRetries is the number of retry attempts on API failure (V-009).
	maxRetries = 3

	// fallbackSummary is the summary text used when analysis fails after all retries (V-009).
	fallbackSummary = "Analysis failed -- raw transcript included below"
)

// retryDelays defines exponential backoff durations for retry attempts.
// Delays: 1s, 2s, 4s as specified in the task.
var retryDelays = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
}

// ClaudeAnalyzer implements the Analyzer interface using the Anthropic Messages API.
// It performs post-meeting analysis including speaker identification, summarization,
// decision extraction, and action item extraction.
type ClaudeAnalyzer struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewClaudeAnalyzer creates a new ClaudeAnalyzer with the given API key.
// The API key is used for authentication with the Anthropic Messages API.
func NewClaudeAnalyzer(apiKey string) *ClaudeAnalyzer {
	return &ClaudeAnalyzer{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// WithBaseURL sets a custom base URL for the API. This is used in tests
// to point at an httptest server.
func (c *ClaudeAnalyzer) WithBaseURL(url string) *ClaudeAnalyzer {
	c.baseURL = strings.TrimRight(url, "/")
	return c
}

// WithHTTPClient sets a custom HTTP client. This is used in tests.
func (c *ClaudeAnalyzer) WithHTTPClient(client *http.Client) *ClaudeAnalyzer {
	c.client = client
	return c
}

// Summarize analyzes a complete meeting transcript and returns structured notes.
// It implements retry with exponential backoff on API failure (V-009).
// On all retries exhausted, returns a partial MeetingNote with the raw transcript.
// Never returns nil -- always returns something usable.
func (c *ClaudeAnalyzer) Summarize(ctx context.Context, segments []heimdall.Segment, opts heimdall.AnalyzeOpts) (*heimdall.MeetingNote, error) {
	// Handle empty or nil segments.
	if len(segments) == 0 {
		return &heimdall.MeetingNote{
			Summary:     "No transcript content to analyze.",
			SpeakerMap:  map[int]string{},
			Segments:    segments,
			Decisions:   []heimdall.Decision{},
			ActionItems: []heimdall.ActionItem{},
			Topics:      []heimdall.Topic{},
			Followups:   []heimdall.Followup{},
			MeetingType: "general",
		}, nil
	}

	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	userPrompt := buildUserPrompt(segments, opts)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry with exponential backoff.
			delay := retryDelays[attempt-1]
			select {
			case <-ctx.Done():
				return c.buildFallbackNote(segments, fmt.Errorf("context cancelled during retry: %w", ctx.Err())), ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := c.callAPI(ctx, model, userPrompt)
		if err != nil {
			lastErr = err
			continue
		}

		note, err := c.parseResponse(result, segments)
		if err != nil {
			lastErr = fmt.Errorf("parsing Claude response: %w", err)
			continue
		}

		return note, nil
	}

	// All retries exhausted -- return fallback note (V-009).
	return c.buildFallbackNote(segments, lastErr), nil
}

// apiRequest is the request body for the Anthropic Messages API.
type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system"`
	Messages  []apiMessage `json:"messages"`
}

// apiMessage is a message in the Anthropic Messages API conversation.
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiResponse is the response from the Anthropic Messages API.
type apiResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

// contentBlock is a content block in the API response.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// apiErrorResponse represents an error from the Anthropic API.
type apiErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// callAPI makes a single API call to the Anthropic Messages endpoint.
func (c *ClaudeAnalyzer) callAPI(ctx context.Context, model, userPrompt string) (string, error) {
	reqBody := apiRequest{
		Model:     model,
		MaxTokens: defaultMaxTokens,
		System:    systemPrompt,
		Messages: []apiMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshalling request: %w", err)
	}

	url := c.baseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Anthropic-Version", anthropicVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Anthropic API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return "", fmt.Errorf("Anthropic API error (status %d): %s: %s", resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
		}
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	// Extract text content from response blocks.
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in API response")
}

// analysisResult is the JSON structure returned by Claude's analysis.
type analysisResult struct {
	SpeakerMap  map[string]string `json:"speaker_map"`
	Summary     string            `json:"summary"`
	Decisions   []struct {
		Description string `json:"description"`
		DecidedBy   string `json:"decided_by"`
	} `json:"decisions"`
	ActionItems []struct {
		Task     string `json:"task"`
		Owner    string `json:"owner"`
		Deadline string `json:"deadline"`
		Priority string `json:"priority"`
	} `json:"action_items"`
	Topics []struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"topics"`
	Followups []struct {
		Question string `json:"question"`
		RaisedBy string `json:"raised_by"`
	} `json:"followups"`
	MeetingType string `json:"meeting_type"`
}

// parseResponse parses Claude's JSON response into a MeetingNote.
func (c *ClaudeAnalyzer) parseResponse(text string, segments []heimdall.Segment) (*heimdall.MeetingNote, error) {
	// Strip markdown code fences if present (Claude sometimes wraps JSON in ```json ... ```).
	text = stripCodeFences(text)
	text = strings.TrimSpace(text)

	var result analysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing analysis JSON: %w", err)
	}

	// Map string speaker IDs to int speaker IDs.
	speakerMap := make(map[int]string, len(result.SpeakerMap))
	for idStr, name := range result.SpeakerMap {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue // Skip non-numeric speaker IDs.
		}
		speakerMap[id] = name
	}

	// Build participants list from speaker map.
	participants := make([]heimdall.Participant, 0, len(speakerMap))
	for _, name := range speakerMap {
		participants = append(participants, heimdall.Participant{Name: name})
	}

	// Map decisions, ensuring non-nil slices.
	decisions := make([]heimdall.Decision, 0, len(result.Decisions))
	for _, d := range result.Decisions {
		decisions = append(decisions, heimdall.Decision{
			Description: d.Description,
			DecidedBy:   d.DecidedBy,
		})
	}

	// Map action items, ensuring non-nil slices.
	actionItems := make([]heimdall.ActionItem, 0, len(result.ActionItems))
	for _, a := range result.ActionItems {
		actionItems = append(actionItems, heimdall.ActionItem{
			Task:     a.Task,
			Owner:    a.Owner,
			Deadline: a.Deadline,
			Priority: a.Priority,
		})
	}

	// Map topics, ensuring non-nil slices.
	topics := make([]heimdall.Topic, 0, len(result.Topics))
	for _, t := range result.Topics {
		topics = append(topics, heimdall.Topic{
			Title:   t.Title,
			Content: t.Content,
		})
	}

	// Map followups, ensuring non-nil slices.
	followups := make([]heimdall.Followup, 0, len(result.Followups))
	for _, f := range result.Followups {
		followups = append(followups, heimdall.Followup{
			Question: f.Question,
			RaisedBy: f.RaisedBy,
		})
	}

	meetingType := result.MeetingType
	if meetingType == "" {
		meetingType = "general"
	}

	return &heimdall.MeetingNote{
		Summary:      result.Summary,
		Decisions:    decisions,
		ActionItems:  actionItems,
		Topics:       topics,
		Followups:    followups,
		SpeakerMap:   speakerMap,
		Participants: participants,
		Segments:     segments,
		MeetingType:  meetingType,
	}, nil
}

// buildFallbackNote creates a partial MeetingNote when analysis fails (V-009).
// It includes the raw transcript segments and an error summary.
// Never returns nil.
func (c *ClaudeAnalyzer) buildFallbackNote(segments []heimdall.Segment, lastErr error) *heimdall.MeetingNote {
	summary := fallbackSummary
	if lastErr != nil {
		summary = fmt.Sprintf("%s (error: %v)", fallbackSummary, lastErr)
	}

	return &heimdall.MeetingNote{
		Summary:     summary,
		Segments:    segments,
		SpeakerMap:  map[int]string{},
		Decisions:   []heimdall.Decision{},
		ActionItems: []heimdall.ActionItem{},
		Topics:      []heimdall.Topic{},
		Followups:   []heimdall.Followup{},
		MeetingType: "general",
		IsFallback:  true,
	}
}

// stripCodeFences removes markdown code fences from a string.
// Claude sometimes wraps JSON output in ```json ... ``` blocks.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (e.g., "```json\n" or "```\n").
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence.
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}
