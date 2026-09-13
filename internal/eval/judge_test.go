package eval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

func TestParseJudgeResult(t *testing.T) {
	t.Run("plain JSON", func(t *testing.T) {
		r, err := parseJudgeResult(`{"faithfulness": 9, "coverage": 7, "hallucinations": [], "missed": ["x"], "reasoning": "good"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Faithfulness != 9 || r.Coverage != 7 {
			t.Errorf("got faithfulness=%d coverage=%d, want 9/7", r.Faithfulness, r.Coverage)
		}
		if len(r.Missed) != 1 || r.Missed[0] != "x" {
			t.Errorf("Missed: got %v", r.Missed)
		}
	})

	t.Run("wrapped in markdown fences", func(t *testing.T) {
		r, err := parseJudgeResult("```json\n{\"faithfulness\": 5, \"coverage\": 5}\n```")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Faithfulness != 5 {
			t.Errorf("got faithfulness=%d, want 5", r.Faithfulness)
		}
	})

	t.Run("nil arrays normalized to empty", func(t *testing.T) {
		r, err := parseJudgeResult(`{"faithfulness": 10, "coverage": 10}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Hallucinations == nil || r.Missed == nil {
			t.Error("expected Hallucinations/Missed to be normalized to empty slices, not nil")
		}
	})

	t.Run("invalid JSON errors", func(t *testing.T) {
		if _, err := parseJudgeResult("not json"); err == nil {
			t.Error("expected an error for invalid JSON")
		}
	})
}

func TestBuildJudgePrompt(t *testing.T) {
	f := Fixture{
		Segments: []heimdall.Segment{{Speaker: 0, Text: "Let's ship it Friday."}},
	}
	note := &heimdall.MeetingNote{
		Summary:     "Team agreed to ship Friday.",
		Decisions:   []heimdall.Decision{{Description: "Ship Friday", DecidedBy: "Alex"}},
		ActionItems: []heimdall.ActionItem{{Task: "Deploy", Owner: "Alex", Deadline: "Friday"}},
	}

	prompt := buildJudgePrompt(f, note)

	for _, want := range []string{"<transcript>", "</transcript>", "<summary_output>", "</summary_output>", "Let's ship it Friday.", "Ship Friday", "Deploy"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q, got: %s", want, prompt)
		}
	}
}

func TestAPIJudge_Judge_Success(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)

		resp := judgeAPIResponse{Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: `{"faithfulness": 8, "coverage": 9, "hallucinations": [], "missed": ["a point"], "reasoning": "solid"}`}}}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer server.Close()

	judge := NewAPIJudge("test-key", "").WithBaseURL(server.URL)
	f := Fixtures()[0]
	note := &heimdall.MeetingNote{Summary: "Discussed the standup items."}

	result, err := judge.Judge(context.Background(), f, note)
	if err != nil {
		t.Fatalf("Judge: unexpected error: %v", err)
	}
	if result.Faithfulness != 8 || result.Coverage != 9 {
		t.Errorf("got faithfulness=%d coverage=%d, want 8/9", result.Faithfulness, result.Coverage)
	}
	if len(result.Missed) != 1 {
		t.Errorf("Missed: got %v", result.Missed)
	}
	if result.Latency <= 0 {
		t.Error("expected a positive Latency")
	}

	if gotHeaders.Get("X-Api-Key") != "test-key" {
		t.Errorf("X-Api-Key: got %q, want %q", gotHeaders.Get("X-Api-Key"), "test-key")
	}
	if gotHeaders.Get("Anthropic-Version") != judgeAnthropicVersion {
		t.Errorf("Anthropic-Version: got %q, want %q", gotHeaders.Get("Anthropic-Version"), judgeAnthropicVersion)
	}
	var sentReq judgeAPIRequest
	if err := json.Unmarshal(gotBody, &sentReq); err != nil {
		t.Fatalf("could not parse sent request body: %v", err)
	}
	if sentReq.Model != judgeDefaultModel {
		t.Errorf("Model: got %q, want default %q", sentReq.Model, judgeDefaultModel)
	}
	if sentReq.System != judgeSystemPrompt {
		t.Error("System prompt not sent as expected")
	}
}

func TestAPIJudge_Judge_CustomModel(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		resp := judgeAPIResponse{Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: `{"faithfulness": 10, "coverage": 10}`}}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer server.Close()

	judge := NewAPIJudge("test-key", "claude-opus-5").WithBaseURL(server.URL)
	_, err := judge.Judge(context.Background(), Fixtures()[0], &heimdall.MeetingNote{Summary: "ok"})
	if err != nil {
		t.Fatalf("Judge: unexpected error: %v", err)
	}

	var sentReq judgeAPIRequest
	json.Unmarshal(gotBody, &sentReq)
	if sentReq.Model != "claude-opus-5" {
		t.Errorf("Model: got %q, want %q", sentReq.Model, "claude-opus-5")
	}
}

func TestAPIJudge_Judge_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"server_error","message":"boom"}}`))
	}))
	defer server.Close()

	judge := NewAPIJudge("test-key", "").WithBaseURL(server.URL)
	_, err := judge.Judge(context.Background(), Fixtures()[0], &heimdall.MeetingNote{Summary: "ok"})
	if err == nil {
		t.Error("expected an error for a 500 response")
	}
}

func TestAPIJudge_Judge_MethodValueSatisfiesJudgeFunc(t *testing.T) {
	// Compile-time-ish assertion that the APIJudge.Judge method value is
	// directly assignable to JudgeFunc, as RunOptions.Judge expects.
	judge := NewAPIJudge("k", "")
	var fn JudgeFunc = judge.Judge
	_ = fn
}
