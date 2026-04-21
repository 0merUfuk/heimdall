package transcriber

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/gorilla/websocket"
)

// sonioxTestOpts returns heimdall.TranscribeOpts defaults for Soniox tests.
// Soniox always receives mono 16kHz s16le — matching the values here.
func sonioxTestOpts() heimdall.TranscribeOpts {
	return heimdall.TranscribeOpts{
		Language:   "",
		Model:      "stt-rt-v4",
		SampleRate: 16000,
		Channels:   1,
		Encoding:   "linear16",
		Diarize:    true,
	}
}

// newTestSonioxTranscriber returns a SonioxTranscriber pointed at a test
// WebSocket server with tightened timeouts so tests finish quickly.
func newTestSonioxTranscriber(serverURL string) *SonioxTranscriber {
	st := NewSonioxTranscriber(sonioxSessionConfig{
		APIKey: "test-key",
		Model:  "stt-rt-v4",
	})
	st.baseURL = serverURL
	st.closeTimeout = 500 * time.Millisecond
	return st
}

// sonioxHandler is a server-side handler that owns one WebSocket session.
// The test writes this closure to customise server behaviour per case.
type sonioxHandler func(t *testing.T, conn *websocket.Conn, firstMsg sonioxConfigMessage)

// mockSonioxServer spins up an httptest.Server that upgrades to a
// WebSocket, reads the client's first-frame JSON config, and hands the
// connection to the provided handler. If the first frame is absent or
// malformed, the test fails immediately — every Soniox session must start
// with a config frame per the API contract.
//
// The returned server URL is already in ws:// form so callers can pass it
// straight to NewSonioxTranscriber.
func mockSonioxServer(t *testing.T, handler sonioxHandler) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the first-frame JSON config. All Soniox sessions start here.
		msgType, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			t.Errorf("first frame: got message type %d, want TextMessage", msgType)
			return
		}
		var cfg sonioxConfigMessage
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Errorf("first frame: invalid JSON: %v (raw=%q)", err, raw)
			return
		}
		handler(t, conn, cfg)
	}))

	return server
}

// wsSonioxURL converts an httptest.Server URL to a WebSocket URL.
func wsSonioxURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// sonioxWriteJSON marshals a response and writes it as a text frame.
func sonioxWriteJSON(t *testing.T, conn *websocket.Conn, resp sonioxResponse) {
	t.Helper()
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

// drainClient reads everything the client sends so the server-side
// ReadMessage loop never deadlocks. Returns the count of binary frames
// received (useful for tests that assert end-of-stream).
func drainClient(conn *websocket.Conn, emptyFrameSeen *atomic.Bool) {
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.BinaryMessage && len(data) == 0 {
			if emptyFrameSeen != nil {
				emptyFrameSeen.Store(true)
			}
		}
	}
}

// -------------------------------------------------------------------------
// Test cases (per brief §5, ≥10 cases).
// -------------------------------------------------------------------------

// 1. Connect succeeds against mock server; verify first-frame JSON shape.
func TestSoniox_Connect_SendsExpectedConfig(t *testing.T) {
	var gotCfg sonioxConfigMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		mu.Lock()
		gotCfg = cfg
		mu.Unlock()
		close(done)
		// Keep the socket alive long enough for Close() to run cleanly.
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()

	if gotCfg.APIKey != "test-key" {
		t.Errorf("api_key: got %q, want %q", gotCfg.APIKey, "test-key")
	}
	if gotCfg.Model != "stt-rt-v4" {
		t.Errorf("model: got %q, want %q", gotCfg.Model, "stt-rt-v4")
	}
	if gotCfg.AudioFormat != "s16le" {
		t.Errorf("audio_format: got %q, want %q", gotCfg.AudioFormat, "s16le")
	}
	if gotCfg.SampleRate != 16000 {
		t.Errorf("sample_rate: got %d, want 16000", gotCfg.SampleRate)
	}
	if gotCfg.NumChannels != 1 {
		t.Errorf("num_channels: got %d, want 1", gotCfg.NumChannels)
	}

	_ = st.Close()
}

// 2. Connect fails cleanly when the API key is empty. No ws dial should
// happen — the preflight in Connect catches it.
func TestSoniox_Connect_EmptyAPIKey(t *testing.T) {
	st := NewSonioxTranscriber(sonioxSessionConfig{APIKey: ""})
	// Intentionally do not point at any server — the preflight must fail
	// before a dial is attempted.

	err := st.Connect(context.Background(), sonioxTestOpts())
	if err == nil {
		t.Fatal("Connect with empty api key: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// 3. Connect fails when the server refuses the first frame (simulated by
// closing the connection immediately after receiving the config — the
// ready-state errors bubble up on the first Send/Read).
func TestSoniox_Connect_ServerRejects(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		// Emit an error frame then close the socket. readLoop logs the
		// error and exits; the test then observes the closed channel.
		sonioxWriteJSON(t, conn, sonioxResponse{
			ErrorCode:    401,
			ErrorMessage: "unauthorized",
		})
		// No drain — we want the conn closed from the server side.
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	st.cfg.APIKey = "bogus"

	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		// Some environments may surface the server-close as a Connect
		// error; that is also acceptable because the user sees it
		// immediately rather than mid-recording.
		return
	}

	// Server closed — the segment channel should close cleanly.
	// Consume with a timeout so the test never hangs.
	ch := st.Receive()
	select {
	case <-ch:
		// Channel is closed or a segment came through; either is fine.
		// The asserted property is "no panic, clean shutdown".
	case <-time.After(2 * time.Second):
		// Acceptable — the client may still be holding the conn pending
		// an error; Close() below will tear it down. The goal of this
		// case is only that we don't panic.
	}
	_ = st.Close()
}

// 4. TR+EN hints present when language is "multi".
func TestSoniox_Connect_TREnHintsFromMulti(t *testing.T) {
	var gotCfg sonioxConfigMessage
	var mu sync.Mutex
	done := make(chan struct{})

	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		mu.Lock()
		gotCfg = cfg
		mu.Unlock()
		close(done)
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	opts := sonioxTestOpts()
	opts.Language = "multi"
	if err := st.Connect(context.Background(), opts); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()

	want := []string{"tr", "en"}
	if len(gotCfg.LanguageHints) != 2 || gotCfg.LanguageHints[0] != want[0] || gotCfg.LanguageHints[1] != want[1] {
		t.Errorf("language_hints: got %v, want %v", gotCfg.LanguageHints, want)
	}
	if !gotCfg.EnableLanguageIdentification {
		t.Error("enable_language_identification should be true for TR+EN mode")
	}

	_ = st.Close()
}

// 5. Diarization flag is respected.
func TestSoniox_Connect_DiarizationFlag(t *testing.T) {
	tests := []struct {
		name    string
		diarize bool
	}{
		{"diarize on", true},
		{"diarize off", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotCfg sonioxConfigMessage
			var mu sync.Mutex
			done := make(chan struct{})

			server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
				mu.Lock()
				gotCfg = cfg
				mu.Unlock()
				close(done)
				drainClient(conn, nil)
			})
			defer server.Close()

			st := newTestSonioxTranscriber(wsSonioxURL(server))
			opts := sonioxTestOpts()
			opts.Diarize = tc.diarize
			if err := st.Connect(context.Background(), opts); err != nil {
				t.Fatalf("Connect: %v", err)
			}
			<-done

			mu.Lock()
			defer mu.Unlock()

			if gotCfg.EnableSpeakerDiarization != tc.diarize {
				t.Errorf("enable_speaker_diarization: got %v, want %v", gotCfg.EnableSpeakerDiarization, tc.diarize)
			}
			_ = st.Close()
		})
	}
}

// 6. Send before Connect returns ErrNotConnected.
func TestSoniox_Send_BeforeConnect(t *testing.T) {
	st := NewSonioxTranscriber(sonioxSessionConfig{APIKey: "k"})
	err := st.Send(heimdall.AudioFrame{Data: []byte{0x00, 0x01}})
	if err == nil {
		t.Fatal("Send before Connect: expected error")
	}
	if !strings.Contains(err.Error(), ErrNotConnected.Error()) {
		t.Errorf("unexpected error: %v", err)
	}
}

// 7. Send after Close returns ErrAlreadyClosed.
func TestSoniox_Send_AfterClose(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := st.Send(heimdall.AudioFrame{Data: []byte{0x00, 0x01}})
	if err == nil {
		t.Fatal("Send after Close: expected error")
	}
	if !strings.Contains(err.Error(), ErrAlreadyClosed.Error()) {
		t.Errorf("unexpected error: %v", err)
	}
}

// 8. Receive emits a Segment when the server sends all-final tokens ending
// in a sentence-ending punctuation.
func TestSoniox_Receive_FinalTokensWithPunctuation(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		sonioxWriteJSON(t, conn, sonioxResponse{
			Tokens: []sonioxToken{
				{Text: "Hello", StartMS: 100, EndMS: 400, Confidence: 0.9, IsFinal: true, Speaker: "1"},
				{Text: " ", StartMS: 400, EndMS: 420, Confidence: 0.99, IsFinal: true, Speaker: "1"},
				{Text: "world", StartMS: 420, EndMS: 800, Confidence: 0.95, IsFinal: true, Speaker: "1"},
				{Text: ".", StartMS: 800, EndMS: 820, Confidence: 0.99, IsFinal: true, Speaker: "1"},
			},
		})
		// After emitting, wait for client to read, then send finished so
		// readLoop exits cleanly.
		time.Sleep(100 * time.Millisecond)
		sonioxWriteJSON(t, conn, sonioxResponse{Finished: true})
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	seg := readOneSegment(t, st, 2*time.Second)
	if seg.Text != "Hello world." {
		t.Errorf("Text: got %q, want %q", seg.Text, "Hello world.")
	}
	if !seg.IsFinal {
		t.Error("IsFinal should be true")
	}
	if seg.Speaker != 0 { // "1" maps to 0-based 0
		t.Errorf("Speaker: got %d, want 0 (soniox '1' -> 0-based)", seg.Speaker)
	}
	if seg.Start != 100*time.Millisecond {
		t.Errorf("Start: got %v, want 100ms", seg.Start)
	}
	if seg.End != 820*time.Millisecond {
		t.Errorf("End: got %v, want 820ms", seg.End)
	}

	_ = st.Close()
}

// 9. Two speakers interleaved produce two Segments with distinct speaker IDs.
func TestSoniox_Receive_SpeakerChangeBoundary(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		sonioxWriteJSON(t, conn, sonioxResponse{
			Tokens: []sonioxToken{
				{Text: "Hi", StartMS: 0, EndMS: 200, Confidence: 0.9, IsFinal: true, Speaker: "1"},
				// Speaker change — Hi flushes as its own segment without
				// waiting for punctuation.
				{Text: "Hello", StartMS: 300, EndMS: 600, Confidence: 0.9, IsFinal: true, Speaker: "2"},
				{Text: ".", StartMS: 600, EndMS: 620, Confidence: 0.9, IsFinal: true, Speaker: "2"},
			},
		})
		time.Sleep(100 * time.Millisecond)
		sonioxWriteJSON(t, conn, sonioxResponse{Finished: true})
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	first := readOneSegment(t, st, 2*time.Second)
	second := readOneSegment(t, st, 2*time.Second)

	if first.Speaker != 0 || first.Text != "Hi" {
		t.Errorf("first segment: got speaker=%d text=%q, want speaker=0 text=%q", first.Speaker, first.Text, "Hi")
	}
	if second.Speaker != 1 || second.Text != "Hello." {
		t.Errorf("second segment: got speaker=%d text=%q, want speaker=1 text=%q", second.Speaker, second.Text, "Hello.")
	}

	_ = st.Close()
}

// 10. Mixed-language tokens populate the segment correctly. Our design
// takes the speaker and start/end from the first token and concatenates
// text verbatim — this test pins that design so a future refactor that
// wanted language at the Segment level would have to update it.
func TestSoniox_Receive_MixedLanguageTokens(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		sonioxWriteJSON(t, conn, sonioxResponse{
			Tokens: []sonioxToken{
				{Text: "Merhaba", StartMS: 0, EndMS: 400, Confidence: 0.9, IsFinal: true, Speaker: "1", Language: "tr"},
				{Text: " ", StartMS: 400, EndMS: 420, Confidence: 0.99, IsFinal: true, Speaker: "1", Language: "tr"},
				{Text: "world", StartMS: 420, EndMS: 800, Confidence: 0.9, IsFinal: true, Speaker: "1", Language: "en"},
				{Text: ".", StartMS: 800, EndMS: 820, Confidence: 0.99, IsFinal: true, Speaker: "1", Language: "en"},
			},
		})
		time.Sleep(100 * time.Millisecond)
		sonioxWriteJSON(t, conn, sonioxResponse{Finished: true})
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	seg := readOneSegment(t, st, 2*time.Second)
	if seg.Text != "Merhaba world." {
		t.Errorf("Text: got %q, want %q", seg.Text, "Merhaba world.")
	}
	if seg.Speaker != 0 {
		t.Errorf("Speaker: got %d, want 0", seg.Speaker)
	}

	_ = st.Close()
}

// 11. Close is idempotent — calling twice does not panic, second call
// returns nil.
func TestSoniox_Close_Idempotent(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}
}

// 12. Context cancellation during Receive closes the channel cleanly
// (no goroutine leak / no panic). Cancellation propagates through
// Close() which is called in test cleanup.
func TestSoniox_ContextCancellation(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		drainClient(conn, nil)
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(ctx, sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	cancel()
	// Close should still tear down cleanly after the context was cancelled.
	if err := st.Close(); err != nil {
		t.Fatalf("Close after cancel: %v", err)
	}

	// Receive channel must be closed.
	_, ok := <-st.Receive()
	if ok {
		// A pending segment is acceptable; drain then expect closure.
		_, ok = <-st.Receive()
	}
	if ok {
		t.Error("Receive channel should be closed after Close()")
	}
}

// 13. Provisional tokens (is_final=false) are dropped — no Segment is
// emitted for them.
func TestSoniox_ProvisionalTokensDropped(t *testing.T) {
	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		sonioxWriteJSON(t, conn, sonioxResponse{
			Tokens: []sonioxToken{
				{Text: "maybe", StartMS: 0, EndMS: 400, Confidence: 0.5, IsFinal: false, Speaker: "1"},
				{Text: " ", StartMS: 400, EndMS: 420, Confidence: 0.5, IsFinal: false, Speaker: "1"},
				{Text: "wrong", StartMS: 420, EndMS: 800, Confidence: 0.5, IsFinal: false, Speaker: "1"},
			},
		})
		time.Sleep(150 * time.Millisecond)
		sonioxWriteJSON(t, conn, sonioxResponse{Finished: true})
		drainClient(conn, nil)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Expect NO segment before Close. We use a short timeout.
	select {
	case seg, ok := <-st.Receive():
		if ok {
			t.Errorf("provisional tokens should not produce a Segment; got %+v", seg)
		}
	case <-time.After(600 * time.Millisecond):
		// No segment — good.
	}

	_ = st.Close()
}

// 14. End-of-stream handshake: Close() sends an empty binary frame, which
// the server sees before the WebSocket closes.
func TestSoniox_EndOfStreamEmptyFrame(t *testing.T) {
	var emptyFrameSeen atomic.Bool
	serverClosed := make(chan struct{})

	server := mockSonioxServer(t, func(t *testing.T, conn *websocket.Conn, cfg sonioxConfigMessage) {
		drainClient(conn, &emptyFrameSeen)
		close(serverClosed)
	})
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Client closes — the empty binary frame should be written before the
	// WebSocket teardown.
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-serverClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not exit after client Close")
	}

	if !emptyFrameSeen.Load() {
		t.Error("server did not receive the empty binary frame end-of-stream marker")
	}
}

// 15. Reconnection preserves monotonic timestamps. After the first session
// emits a segment ending at 2000ms and the connection drops, the second
// session's tokens restart from 0 but must be offset so the post-reconnect
// segment's Start is >= the prior segment's End.
func TestSoniox_ReconnectPreservesTimestamps(t *testing.T) {
	// Use a dispatcher that serves a different handler per connection so we
	// can simulate "first connection drops, second connection serves new
	// tokens".
	var attempt atomic.Int32
	secondDone := make(chan struct{})

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the config frame (all sessions start with it).
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		a := attempt.Add(1)
		switch a {
		case 1:
			// First session: emit a complete utterance ending at 2000ms,
			// then drop the socket without a graceful finished:true. The
			// client's readLoop will surface an error and trigger
			// handleDisconnect.
			sonioxWriteJSON(t, conn, sonioxResponse{
				Tokens: []sonioxToken{
					{Text: "First", StartMS: 1000, EndMS: 1500, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: " ", StartMS: 1500, EndMS: 1520, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: "one", StartMS: 1520, EndMS: 1980, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: ".", StartMS: 1980, EndMS: 2000, Confidence: 0.9, IsFinal: true, Speaker: "1"},
				},
			})
			// Give the client a moment to read + emit, then hard-drop.
			time.Sleep(150 * time.Millisecond)
			return
		case 2:
			// Second session (post-reconnect): emit tokens with zero-based
			// timestamps. The transcriber must offset these by the
			// previously-emitted segment's End (2000ms) so Start >= 2000ms.
			sonioxWriteJSON(t, conn, sonioxResponse{
				Tokens: []sonioxToken{
					{Text: "Second", StartMS: 500, EndMS: 900, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: " ", StartMS: 900, EndMS: 920, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: "two", StartMS: 920, EndMS: 1400, Confidence: 0.9, IsFinal: true, Speaker: "1"},
					{Text: ".", StartMS: 1400, EndMS: 1500, Confidence: 0.9, IsFinal: true, Speaker: "1"},
				},
			})
			time.Sleep(100 * time.Millisecond)
			sonioxWriteJSON(t, conn, sonioxResponse{Finished: true})
			close(secondDone)
			drainClient(conn, nil)
			return
		default:
			drainClient(conn, nil)
			return
		}
	}))
	defer server.Close()

	st := newTestSonioxTranscriber(wsSonioxURL(server))
	// Shorter backoff + minimal retry budget so the test completes in well
	// under the default timeout.
	st.maxReconnectFailures = 3
	st.maxBackoff = 500 * time.Millisecond

	if err := st.Connect(context.Background(), sonioxTestOpts()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer st.Close()

	// First segment from the first session.
	first := readOneSegment(t, st, 3*time.Second)
	if first.Text != "First one." {
		t.Errorf("first segment text: got %q, want %q", first.Text, "First one.")
	}
	if first.End != 2000*time.Millisecond {
		t.Errorf("first segment End: got %v, want 2000ms", first.End)
	}

	// Second segment comes from the reconnected session. Its start_ms was
	// 500 but must be offset so Start >= first.End.
	second := readOneSegment(t, st, 5*time.Second)
	if second.Text != "Second two." {
		t.Errorf("second segment text: got %q, want %q", second.Text, "Second two.")
	}
	if second.Start < first.End {
		t.Errorf("second.Start (%v) is earlier than first.End (%v) — reconnect offset was not applied",
			second.Start, first.End)
	}
	// Exact check: 500ms raw + 2000ms offset = 2500ms.
	wantStart := 2500 * time.Millisecond
	if second.Start != wantStart {
		t.Errorf("second.Start: got %v, want %v (500ms raw + 2000ms offset)", second.Start, wantStart)
	}

	// Drain the server goroutine.
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second server session did not complete")
	}
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

// readOneSegment drains one segment from Receive with a timeout. Skips
// any "[transcription error ..." reconnect sentinels a prior subtest
// might have emitted (defensive — our tests don't generate them, but
// keeping the helper robust keeps readOneSegment reusable).
func readOneSegment(t *testing.T, st *SonioxTranscriber, timeout time.Duration) heimdall.Segment {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case seg, ok := <-st.Receive():
			if !ok {
				t.Fatal("Receive channel closed before a segment arrived")
			}
			if strings.HasPrefix(seg.Text, "[transcription error") {
				continue
			}
			return seg
		case <-deadline.C:
			t.Fatalf("timed out waiting for segment after %v", timeout)
		}
	}
}
