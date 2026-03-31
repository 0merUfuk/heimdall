package transcriber

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/gorilla/websocket"
)

// testOpts returns default TranscribeOpts for testing.
func testOpts() heimdall.TranscribeOpts {
	return heimdall.TranscribeOpts{
		Language:    "en",
		Model:       "nova-3",
		SampleRate:  16000,
		Channels:    2,
		Encoding:    "linear16",
		Diarize:     true,
		Punctuate:   true,
		SmartFormat: true,
	}
}

// testTranscriber creates a DeepgramTranscriber with short timeouts for testing.
func testTranscriber(serverURL string) *DeepgramTranscriber {
	dt := NewDeepgramTranscriber("test-key")
	dt.baseURL = serverURL
	dt.reconnectInterval = 1 * time.Hour // disable proactive reconnect by default
	dt.closeTimeout = 500 * time.Millisecond
	return dt
}

// mockDeepgramServer creates a test WebSocket server that simulates Deepgram.
// The handler function receives the WebSocket connection for custom behavior.
func mockDeepgramServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
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
		handler(conn)
	}))

	return server
}

// wsURL converts an httptest.Server URL to a WebSocket URL.
func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// makeDeepgramResponse creates a Deepgram-format JSON response for testing.
func makeDeepgramResponse(transcript string, speaker int, start, duration float64, channel int, isFinal bool) []byte {
	resp := deepgramResponse{
		Type: "Results",
		Channel: struct {
			Index        int `json:"index"`
			Alternatives []struct {
				Transcript string  `json:"transcript"`
				Confidence float64 `json:"confidence"`
				Words      []struct {
					Word       string  `json:"word"`
					Start      float64 `json:"start"`
					End        float64 `json:"end"`
					Confidence float64 `json:"confidence"`
					Speaker    int     `json:"speaker"`
					Channel    int     `json:"channel"`
				} `json:"words"`
			} `json:"alternatives"`
		}{
			Index: channel,
		},
		IsFinal:  isFinal,
		Start:    start,
		Duration: duration,
	}
	resp.Channel.Alternatives = append(resp.Channel.Alternatives, struct {
		Transcript string  `json:"transcript"`
		Confidence float64 `json:"confidence"`
		Words      []struct {
			Word       string  `json:"word"`
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Confidence float64 `json:"confidence"`
			Speaker    int     `json:"speaker"`
			Channel    int     `json:"channel"`
		} `json:"words"`
	}{
		Transcript: transcript,
		Confidence: 0.95,
		Words: []struct {
			Word       string  `json:"word"`
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Confidence float64 `json:"confidence"`
			Speaker    int     `json:"speaker"`
			Channel    int     `json:"channel"`
		}{
			{
				Word:       strings.Split(transcript, " ")[0],
				Start:      start,
				End:        start + duration,
				Confidence: 0.95,
				Speaker:    speaker,
				Channel:    channel,
			},
		},
	})

	data, _ := json.Marshal(resp)
	return data
}

// echoServer keeps the WebSocket connection alive, consuming all messages.
// It closes the connection when it receives a CloseStream message.
func echoServer(conn *websocket.Conn) {
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.TextMessage {
			var msg deepgramMessage
			if json.Unmarshal(data, &msg) == nil && msg.Type == "CloseStream" {
				return
			}
		}
	}
}

func TestNewDeepgramTranscriber(t *testing.T) {
	dt := NewDeepgramTranscriber("test-key")

	if dt.apiKey != "test-key" {
		t.Errorf("apiKey = %q; want %q", dt.apiKey, "test-key")
	}
	if dt.reconnectInterval != defaultReconnectInterval {
		t.Errorf("reconnectInterval = %v; want %v", dt.reconnectInterval, defaultReconnectInterval)
	}
	if dt.keepAliveInterval != defaultKeepAliveInterval {
		t.Errorf("keepAliveInterval = %v; want %v", dt.keepAliveInterval, defaultKeepAliveInterval)
	}
	if dt.maxReconnectFailures != defaultMaxReconnectFailures {
		t.Errorf("maxReconnectFailures = %d; want %d", dt.maxReconnectFailures, defaultMaxReconnectFailures)
	}
	if dt.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q; want %q", dt.baseURL, defaultBaseURL)
	}
}

func TestConnect(t *testing.T) {
	server := mockDeepgramServer(t, echoServer)
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	dt.mu.Lock()
	if dt.conn == nil {
		t.Error("conn is nil after Connect")
	}
	if dt.startTime.IsZero() {
		t.Error("startTime not set after Connect")
	}
	dt.mu.Unlock()

	if err := dt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestConnectAlreadyConnected(t *testing.T) {
	server := mockDeepgramServer(t, echoServer)
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	err := dt.Connect(context.Background(), testOpts())
	if err == nil {
		t.Fatal("expected error on second Connect, got nil")
	}
}

func TestSend(t *testing.T) {
	received := make(chan []byte, 1)

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				select {
				case received <- data:
				default:
				}
			}
		}
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	testData := []byte{0x01, 0x02, 0x03, 0x04}
	frame := heimdall.AudioFrame{
		Data:       testData,
		SampleRate: 16000,
		Channels:   2,
		Timestamp:  1 * time.Second,
	}

	if err := dt.Send(frame); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case data := <-received:
		if len(data) != len(testData) {
			t.Errorf("received data length = %d; want %d", len(data), len(testData))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for data at mock server")
	}
}

func TestSendNotConnected(t *testing.T) {
	dt := NewDeepgramTranscriber("test-key")

	frame := heimdall.AudioFrame{Data: []byte{0x01}}
	err := dt.Send(frame)
	if err == nil {
		t.Fatal("expected error on Send without Connect, got nil")
	}
}

func TestReceive(t *testing.T) {
	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		// Wait for audio from client.
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				break
			}
		}

		// Send a transcription result.
		resp := makeDeepgramResponse("hello world", 0, 1.0, 2.0, 0, true)
		if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
			return
		}

		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	if err := dt.Send(heimdall.AudioFrame{Data: []byte{0x01}}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case seg := <-dt.Receive():
		if seg.Text != "hello world" {
			t.Errorf("segment.Text = %q; want %q", seg.Text, "hello world")
		}
		if seg.Speaker != 0 {
			t.Errorf("segment.Speaker = %d; want %d", seg.Speaker, 0)
		}
		if !seg.IsFinal {
			t.Error("segment.IsFinal = false; want true")
		}
		if seg.Channel != 0 {
			t.Errorf("segment.Channel = %d; want %d", seg.Channel, 0)
		}
		if seg.Confidence != 0.95 {
			t.Errorf("segment.Confidence = %f; want %f", seg.Confidence, 0.95)
		}
		expectedStart := time.Duration(1.0 * float64(time.Second))
		if seg.Start != expectedStart {
			t.Errorf("segment.Start = %v; want %v", seg.Start, expectedStart)
		}
		expectedEnd := time.Duration(3.0 * float64(time.Second))
		if seg.End != expectedEnd {
			t.Errorf("segment.End = %v; want %v", seg.End, expectedEnd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for segment")
	}
}

func TestReceiveInterimVsFinal(t *testing.T) {
	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		// Wait for audio.
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				break
			}
		}

		// Send interim result.
		interim := makeDeepgramResponse("hel", 0, 0.0, 0.5, 0, false)
		if err := conn.WriteMessage(websocket.TextMessage, interim); err != nil {
			return
		}

		// Send final result.
		final := makeDeepgramResponse("hello world", 0, 0.0, 1.0, 0, true)
		if err := conn.WriteMessage(websocket.TextMessage, final); err != nil {
			return
		}

		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	if err := dt.Send(heimdall.AudioFrame{Data: []byte{0x01}}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	var segments []heimdall.Segment
	for i := 0; i < 2; i++ {
		select {
		case seg := <-dt.Receive():
			segments = append(segments, seg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for segment %d", i)
		}
	}

	if segments[0].IsFinal {
		t.Error("first segment should be interim (IsFinal=false)")
	}
	if !segments[1].IsFinal {
		t.Error("second segment should be final (IsFinal=true)")
	}
}

func TestClose(t *testing.T) {
	var closeStreamReceived atomic.Bool

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				var msg deepgramMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "CloseStream" {
					closeStreamReceived.Store(true)

					// Send a final result after CloseStream.
					resp := makeDeepgramResponse("final words", 0, 10.0, 1.0, 0, true)
					conn.WriteMessage(websocket.TextMessage, resp)
					return
				}
			}
		}
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	err := dt.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if !closeStreamReceived.Load() {
		t.Error("CloseStream message not received by server")
	}
}

func TestCloseIdempotent(t *testing.T) {
	server := mockDeepgramServer(t, echoServer)
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if err := dt.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := dt.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestKeepAliveMessages(t *testing.T) {
	var keepAliveCount atomic.Int32

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				var msg deepgramMessage
				if json.Unmarshal(data, &msg) == nil {
					if msg.Type == "KeepAlive" {
						keepAliveCount.Add(1)
					}
					if msg.Type == "CloseStream" {
						return
					}
				}
			}
		}
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.keepAliveInterval = 100 * time.Millisecond

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	// Wait for a few KeepAlive messages.
	time.Sleep(550 * time.Millisecond)

	if err := dt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	count := keepAliveCount.Load()
	if count < 3 {
		t.Errorf("keepAliveCount = %d; want >= 3 (in 550ms with 100ms interval)", count)
	}
}

func TestReconnectOnNetworkError(t *testing.T) {
	var connectionCount atomic.Int32

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		count := connectionCount.Add(1)

		if count == 1 {
			// First connection: close immediately to simulate network error.
			conn.Close()
			return
		}

		// Second connection onwards: stay alive.
		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.maxBackoff = 200 * time.Millisecond

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	// Wait for reconnection to happen.
	time.Sleep(3 * time.Second)

	finalCount := connectionCount.Load()
	if finalCount < 2 {
		t.Errorf("connectionCount = %d; want >= 2 (initial + reconnect)", finalCount)
	}

	if err := dt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestProactiveReconnection(t *testing.T) {
	var connectionCount atomic.Int32

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		connectionCount.Add(1)
		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.reconnectInterval = 1 * time.Second // Short interval for testing.

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	// Wait for proactive reconnection to trigger.
	time.Sleep(4 * time.Second)

	count := connectionCount.Load()
	if count < 2 {
		t.Errorf("connectionCount = %d; want >= 2 (proactive reconnection should have triggered)", count)
	}

	// Verify timeOffset was updated.
	dt.mu.Lock()
	offset := dt.timeOffset
	dt.mu.Unlock()

	if offset < 1*time.Second {
		t.Errorf("timeOffset = %v; want >= 1s after proactive reconnection", offset)
	}

	if err := dt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTimestampAdjustmentAfterReconnection(t *testing.T) {
	var connectionCount atomic.Int32

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		count := connectionCount.Add(1)

		if count == 1 {
			// First connection: send a response, then close to trigger reconnection.
			for {
				msgType, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if msgType == websocket.BinaryMessage {
					break
				}
			}

			resp := makeDeepgramResponse("before reconnect", 0, 5.0, 1.0, 0, true)
			if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
				return
			}

			// Give client time to read the response, then close.
			time.Sleep(200 * time.Millisecond)
			conn.Close()
			return
		}

		// Second connection: send a response with timestamps starting at 0.
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				resp := makeDeepgramResponse("after reconnect", 0, 2.0, 1.0, 0, true)
				conn.WriteMessage(websocket.TextMessage, resp)
			}
			if msgType == websocket.TextMessage {
				var msg deepgramMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "CloseStream" {
					return
				}
			}
		}
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.maxBackoff = 200 * time.Millisecond

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	// Trigger server to send first response.
	dt.Send(heimdall.AudioFrame{Data: []byte{0x01}})

	// Read first segment (before reconnection).
	select {
	case seg := <-dt.Receive():
		if seg.Text != "before reconnect" {
			t.Errorf("first segment text = %q; want %q", seg.Text, "before reconnect")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first segment")
	}

	// Wait for reconnection to complete.
	time.Sleep(3 * time.Second)

	if connectionCount.Load() < 2 {
		t.Fatal("reconnection did not happen")
	}

	// Send audio on new connection.
	dt.Send(heimdall.AudioFrame{Data: []byte{0x02}})

	// Read second segment -- timestamps should have offset applied.
	select {
	case seg := <-dt.Receive():
		if seg.Text != "after reconnect" {
			t.Errorf("second segment text = %q; want %q", seg.Text, "after reconnect")
		}
		// After reconnection, the timeOffset should be > 0,
		// so the segment start should be > 2.0 seconds (raw start from server).
		if seg.Start <= time.Duration(2.0*float64(time.Second)) {
			t.Errorf("second segment start = %v; want > 2s (should have time offset applied)", seg.Start)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second segment after reconnection")
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		opts    heimdall.TranscribeOpts
		want    map[string]string
		notWant []string // parameters that must NOT be present
	}{
		{
			name: "stereo multichannel excludes diarize",
			opts: heimdall.TranscribeOpts{
				Language:    "en",
				Model:       "nova-3",
				SampleRate:  16000,
				Channels:    2,
				Encoding:    "linear16",
				Diarize:     true, // should be ignored when Channels > 1
				Punctuate:   true,
				SmartFormat: true,
				Keywords:    []string{"Kubernetes", "gRPC"},
			},
			want: map[string]string{
				"language":     "en",
				"model":        "nova-3",
				"sample_rate":  "16000",
				"channels":     "2",
				"encoding":     "linear16",
				"punctuate":    "true",
				"smart_format": "true",
				"multichannel": "true",
			},
			notWant: []string{"diarize=true"},
		},
		{
			name: "minimal stereo",
			opts: heimdall.TranscribeOpts{
				Language:   "en",
				Model:      "nova-3",
				SampleRate: 16000,
				Channels:   2,
				Encoding:   "linear16",
			},
			want: map[string]string{
				"language":     "en",
				"model":        "nova-3",
				"sample_rate":  "16000",
				"channels":     "2",
				"encoding":     "linear16",
				"multichannel": "true",
			},
			notWant: []string{"diarize=true"},
		},
		{
			name: "mono with diarize",
			opts: heimdall.TranscribeOpts{
				Language:   "en",
				Model:      "nova-3",
				SampleRate: 16000,
				Channels:   1,
				Encoding:   "linear16",
				Diarize:    true,
			},
			want: map[string]string{
				"language":    "en",
				"model":       "nova-3",
				"sample_rate": "16000",
				"channels":    "1",
				"encoding":    "linear16",
				"diarize":     "true",
			},
			notWant: []string{"multichannel=true"},
		},
		{
			name: "mono without diarize",
			opts: heimdall.TranscribeOpts{
				Language:   "en",
				Model:      "nova-3",
				SampleRate: 16000,
				Channels:   1,
				Encoding:   "linear16",
			},
			want: map[string]string{
				"language":    "en",
				"model":       "nova-3",
				"sample_rate": "16000",
				"channels":    "1",
				"encoding":    "linear16",
			},
			notWant: []string{"multichannel=true", "diarize=true"},
		},
		{
			name: "multi language uses detect_language",
			opts: heimdall.TranscribeOpts{
				Language:   "multi",
				Model:      "nova-3",
				SampleRate: 16000,
				Channels:   2,
				Encoding:   "linear16",
			},
			want: map[string]string{
				"detect_language": "true",
				"model":           "nova-3",
				"multichannel":    "true",
			},
			notWant: []string{"language=multi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt := NewDeepgramTranscriber("test-key")
			dt.opts = tt.opts

			urlStr, err := dt.buildURL()
			if err != nil {
				t.Fatalf("buildURL() error = %v", err)
			}

			for key, expectedValue := range tt.want {
				if !strings.Contains(urlStr, key+"="+expectedValue) {
					t.Errorf("URL missing %s=%s; got %s", key, expectedValue, urlStr)
				}
			}

			for _, notWanted := range tt.notWant {
				if strings.Contains(urlStr, notWanted) {
					t.Errorf("URL should NOT contain %s; got %s", notWanted, urlStr)
				}
			}
		})
	}
}

func TestResponseToSegment(t *testing.T) {
	tests := []struct {
		name      string
		resp      deepgramResponse
		offset    time.Duration
		wantOK    bool
		wantText  string
		wantStart time.Duration
	}{
		{
			name: "valid final result",
			resp: func() deepgramResponse {
				var r deepgramResponse
				json.Unmarshal(makeDeepgramResponse("hello world", 0, 1.0, 2.0, 0, true), &r)
				return r
			}(),
			wantOK:    true,
			wantText:  "hello world",
			wantStart: time.Duration(1.0 * float64(time.Second)),
		},
		{
			name: "with time offset",
			resp: func() deepgramResponse {
				var r deepgramResponse
				json.Unmarshal(makeDeepgramResponse("offset text", 1, 3.0, 1.0, 1, true), &r)
				return r
			}(),
			offset:    55 * time.Minute,
			wantOK:    true,
			wantText:  "offset text",
			wantStart: 55*time.Minute + time.Duration(3.0*float64(time.Second)),
		},
		{
			name: "empty transcript skipped",
			resp: func() deepgramResponse {
				var r deepgramResponse
				json.Unmarshal(makeDeepgramResponse("", 0, 0, 0, 0, false), &r)
				return r
			}(),
			wantOK: false,
		},
		{
			name: "wrong type skipped",
			resp: deepgramResponse{
				Type: "Metadata",
			},
			wantOK: false,
		},
		{
			name: "no alternatives skipped",
			resp: deepgramResponse{
				Type: "Results",
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt := NewDeepgramTranscriber("test-key")
			dt.timeOffset = tt.offset

			seg, ok := dt.responseToSegment(tt.resp)
			if ok != tt.wantOK {
				t.Errorf("ok = %v; want %v", ok, tt.wantOK)
				return
			}

			if !ok {
				return
			}

			if seg.Text != tt.wantText {
				t.Errorf("text = %q; want %q", seg.Text, tt.wantText)
			}
			if seg.Start != tt.wantStart {
				t.Errorf("start = %v; want %v", seg.Start, tt.wantStart)
			}
		})
	}
}

func TestReconnectMaxFailures(t *testing.T) {
	var connectionCount atomic.Int32

	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		count := connectionCount.Add(1)

		if count == 1 {
			// First connection succeeds briefly, then dies.
			time.Sleep(200 * time.Millisecond)
			conn.Close()
			return
		}

		// All subsequent connections immediately close to simulate persistent failure.
		conn.Close()
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.maxReconnectFailures = 3
	dt.maxBackoff = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := dt.Connect(ctx, testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	// Wait for reconnection attempts to exhaust.
	time.Sleep(5 * time.Second)

	count := connectionCount.Load()
	if count < 2 {
		t.Errorf("connectionCount = %d; want >= 2 (initial + reconnect attempts)", count)
	}

	// Force cleanup since the transcriber may be in a broken state after max failures.
	cancel()
	dt.mu.Lock()
	dt.closed = true
	if dt.conn != nil {
		dt.conn.Close()
	}
	dt.mu.Unlock()
	dt.wg.Wait()
	close(dt.segments)
}

func TestMultichannelResponses(t *testing.T) {
	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		// Wait for audio.
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				break
			}
		}

		// Send channel 0 (system audio / AD-007 left).
		resp0 := makeDeepgramResponse("remote speaker", 0, 1.0, 1.0, 0, true)
		conn.WriteMessage(websocket.TextMessage, resp0)

		// Send channel 1 (microphone / AD-007 right).
		resp1 := makeDeepgramResponse("local speaker", 1, 1.0, 1.0, 1, true)
		conn.WriteMessage(websocket.TextMessage, resp1)

		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	dt.Send(heimdall.AudioFrame{Data: []byte{0x01}})

	var segments []heimdall.Segment
	for i := 0; i < 2; i++ {
		select {
		case seg := <-dt.Receive():
			segments = append(segments, seg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for segment %d", i)
		}
	}

	if segments[0].Channel != 0 {
		t.Errorf("first segment channel = %d; want 0 (system)", segments[0].Channel)
	}
	if segments[1].Channel != 1 {
		t.Errorf("second segment channel = %d; want 1 (mic)", segments[1].Channel)
	}
}

func TestConnectURLParameters(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		echoServer(conn)
	}))
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	opts := testOpts()
	opts.Keywords = []string{"Kubernetes", "gRPC"}

	if err := dt.Connect(context.Background(), opts); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	requiredParams := []string{
		"model=nova-3",
		"language=en",
		"multichannel=true",
		"channels=2",
		"sample_rate=16000",
		"encoding=linear16",
		"punctuate=true",
		"smart_format=true",
	}

	// With Channels=2, diarize must NOT be present (mutually exclusive with multichannel).
	forbiddenParams := []string{
		"diarize=true",
	}

	for _, param := range requiredParams {
		if !strings.Contains(receivedURL, param) {
			t.Errorf("URL missing parameter %q; got %s", param, receivedURL)
		}
	}

	for _, param := range forbiddenParams {
		if strings.Contains(receivedURL, param) {
			t.Errorf("URL should NOT contain %q; got %s", param, receivedURL)
		}
	}
}

func TestConnectAuthHeader(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		echoServer(conn)
	}))
	defer server.Close()

	dt := testTranscriber(wsURL(server))
	dt.apiKey = "my-secret-key"

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	if receivedAuth != "Token my-secret-key" {
		t.Errorf("Authorization header = %q; want %q", receivedAuth, "Token my-secret-key")
	}
}

func TestSendAfterClose(t *testing.T) {
	server := mockDeepgramServer(t, echoServer)
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if err := dt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err := dt.Send(heimdall.AudioFrame{Data: []byte{0x01}})
	if err == nil {
		t.Fatal("expected error on Send after Close, got nil")
	}
}

func TestErrorResponseHandling(t *testing.T) {
	server := mockDeepgramServer(t, func(conn *websocket.Conn) {
		// Wait for audio.
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				break
			}
		}

		// Send an error response (as Deepgram would for invalid parameters).
		errorResp := `{"type":"Error","err_code":"BAD_REQUEST","err_msg":"diarize and multichannel are mutually exclusive","description":"Invalid parameters"}`
		conn.WriteMessage(websocket.TextMessage, []byte(errorResp))

		// Then send a valid result to prove the readLoop continues after errors.
		resp := makeDeepgramResponse("after error", 0, 1.0, 1.0, 0, true)
		conn.WriteMessage(websocket.TextMessage, resp)

		echoServer(conn)
	})
	defer server.Close()

	dt := testTranscriber(wsURL(server))

	if err := dt.Connect(context.Background(), testOpts()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer dt.Close()

	dt.Send(heimdall.AudioFrame{Data: []byte{0x01}})

	// Should receive the valid segment (error response is logged and skipped).
	select {
	case seg := <-dt.Receive():
		if seg.Text != "after error" {
			t.Errorf("segment.Text = %q; want %q", seg.Text, "after error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for segment after error response")
	}
}
