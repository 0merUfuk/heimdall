package transcriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/gorilla/websocket"
)

// Compile-time assertion: DeepgramTranscriber must satisfy Transcriber.
var _ Transcriber = (*DeepgramTranscriber)(nil)

// Sentinel errors for the Deepgram transcriber.
var (
	// ErrNotConnected is returned when Send is called before Connect.
	ErrNotConnected = errors.New("transcriber not connected")

	// ErrAlreadyConnected is returned when Connect is called on an active connection.
	ErrAlreadyConnected = errors.New("transcriber already connected")

	// ErrAlreadyClosed is returned when operations are attempted on a closed transcriber.
	ErrAlreadyClosed = errors.New("transcriber already closed")

	// ErrReconnectFailed is returned when reconnection fails after max retries.
	ErrReconnectFailed = errors.New("reconnection failed after max retries")
)

const (
	// defaultReconnectInterval is the proactive reconnection interval (V-001).
	// Deepgram has a 60-minute timeout; we reconnect at 55 minutes.
	defaultReconnectInterval = 55 * time.Minute

	// defaultKeepAliveInterval is how often KeepAlive messages are sent.
	defaultKeepAliveInterval = 10 * time.Second

	// defaultMaxReconnectFailures is the maximum consecutive reconnection
	// failures before giving up (V-005).
	defaultMaxReconnectFailures = 5

	// defaultMaxBackoff is the maximum backoff duration between reconnection
	// attempts (V-005).
	defaultMaxBackoff = 30 * time.Second

	// defaultCloseTimeout is how long to wait for final results after
	// sending CloseStream.
	defaultCloseTimeout = 5 * time.Second

	// defaultBaseURL is the Deepgram streaming API endpoint.
	defaultBaseURL = "wss://api.deepgram.com/v1/listen"

	// segmentBufferSize is the channel buffer for outgoing segments.
	segmentBufferSize = 256
)

// deepgramResponse represents a transcription result from Deepgram.
type deepgramResponse struct {
	Type string `json:"type"`

	// Error fields — Deepgram sends these for invalid params, auth failures, etc.
	ErrCode     string `json:"err_code"`
	ErrMsg      string `json:"err_msg"`
	Description string `json:"description"`

	Channel struct {
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
	} `json:"channel"`
	IsFinal     bool    `json:"is_final"`
	SpeechFinal bool    `json:"speech_final"`
	Start       float64 `json:"start"`
	Duration    float64 `json:"duration"`
}

// deepgramMessage is used for sending control messages to Deepgram.
type deepgramMessage struct {
	Type string `json:"type"`
}

// DeepgramTranscriber implements the Transcriber interface using Deepgram's
// WebSocket streaming API with speaker diarization.
//
// It handles:
//   - V-001: Proactive reconnection at 55 minutes (before the 60-min timeout)
//   - V-005: Network disruption reconnection with exponential backoff
//   - KeepAlive messages every 10 seconds
//   - Timestamp alignment across reconnections
type DeepgramTranscriber struct {
	apiKey   string
	conn     *websocket.Conn
	connID   uint64 // monotonically increasing connection identifier
	segments chan heimdall.Segment

	mu      sync.Mutex // protects conn, connID, closed, startTime, timeOffset, reconnecting
	writeMu sync.Mutex // serializes WebSocket writes (gorilla/websocket forbids concurrent writes)
	closed  bool

	// Connection timing for V-001 proactive reconnection.
	startTime  time.Time
	timeOffset time.Duration

	opts   heimdall.TranscribeOpts
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// reconnecting guards against concurrent handleDisconnect calls.
	reconnecting bool

	// Configurable fields (defaults set in NewDeepgramTranscriber, overridable for tests).
	reconnectInterval    time.Duration
	keepAliveInterval    time.Duration
	maxReconnectFailures int
	maxBackoff           time.Duration
	closeTimeout         time.Duration
	baseURL              string

	// closeStreamDone is signaled when the readLoop has finished after a CloseStream.
	closeStreamDone chan struct{}
}

// NewDeepgramTranscriber creates a new Deepgram transcriber with the given API key.
// The transcriber is not connected until Connect is called.
func NewDeepgramTranscriber(apiKey string) *DeepgramTranscriber {
	return &DeepgramTranscriber{
		apiKey:               apiKey,
		segments:             make(chan heimdall.Segment, segmentBufferSize),
		reconnectInterval:    defaultReconnectInterval,
		keepAliveInterval:    defaultKeepAliveInterval,
		maxReconnectFailures: defaultMaxReconnectFailures,
		maxBackoff:           defaultMaxBackoff,
		closeTimeout:         defaultCloseTimeout,
		baseURL:              defaultBaseURL,
		closeStreamDone:      make(chan struct{}, 1),
	}
}

// Connect establishes a WebSocket connection to Deepgram and starts
// background goroutines for reading responses, sending keepalives, and
// monitoring reconnection timing.
func (d *DeepgramTranscriber) Connect(ctx context.Context, opts heimdall.TranscribeOpts) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("transcriber connect: %w", ErrAlreadyClosed)
	}
	if d.conn != nil {
		d.mu.Unlock()
		return fmt.Errorf("transcriber connect: %w", ErrAlreadyConnected)
	}
	d.opts = opts
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.mu.Unlock()

	conn, err := d.dial()
	if err != nil {
		return fmt.Errorf("transcriber connect: %w", err)
	}

	d.mu.Lock()
	d.conn = conn
	d.connID++
	d.startTime = time.Now()
	currentConnID := d.connID
	d.mu.Unlock()

	d.wg.Add(3)
	go d.readLoop(conn, currentConnID)
	go d.keepAliveLoop(currentConnID)
	go d.reconnectTimer()

	return nil
}

// Send streams an audio frame to the Deepgram WebSocket connection.
// Returns an error if the connection is not established or writing fails.
func (d *DeepgramTranscriber) Send(frame heimdall.AudioFrame) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("transcriber send: %w", ErrAlreadyClosed)
	}
	conn := d.conn
	d.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("transcriber send: %w", ErrNotConnected)
	}

	if err := d.writeMessage(conn, websocket.BinaryMessage, frame.Data); err != nil {
		// Trigger reconnection on write error (V-005).
		go d.handleDisconnect(conn, err)
		return fmt.Errorf("transcriber send: %w", err)
	}

	return nil
}

// Receive returns a read-only channel of transcribed Segments.
// Both interim (IsFinal=false) and final (IsFinal=true) results are delivered.
// The channel is closed when Close() completes.
func (d *DeepgramTranscriber) Receive() <-chan heimdall.Segment {
	return d.segments
}

// Close gracefully shuts down the transcriber by sending a CloseStream
// message and waiting for final results before closing the WebSocket.
func (d *DeepgramTranscriber) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	conn := d.conn
	d.mu.Unlock()

	// Send CloseStream message to get final results.
	if conn != nil {
		msg := deepgramMessage{Type: "CloseStream"}
		data, _ := json.Marshal(msg)
		_ = d.writeMessage(conn, websocket.TextMessage, data)

		// Wait for readLoop to finish processing final results.
		select {
		case <-d.closeStreamDone:
		case <-time.After(d.closeTimeout):
		}
	}

	// Cancel the context to stop all goroutines.
	if d.cancel != nil {
		d.cancel()
	}

	// Close the WebSocket connection to unblock any blocked ReadMessage calls.
	d.mu.Lock()
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	d.mu.Unlock()

	// Wait for all goroutines to exit.
	d.wg.Wait()

	// Close the segments channel after all producers have stopped.
	close(d.segments)

	return nil
}

// writeMessage serializes all WebSocket write operations through writeMu.
// gorilla/websocket does not support concurrent writes, so all code paths
// that write to a connection must use this method.
func (d *DeepgramTranscriber) writeMessage(conn *websocket.Conn, messageType int, data []byte) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return conn.WriteMessage(messageType, data)
}

// dial creates a new WebSocket connection to Deepgram with the configured options.
func (d *DeepgramTranscriber) dial() (*websocket.Conn, error) {
	u, err := d.buildURL()
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	header.Set("Authorization", "Token "+d.apiKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, httpResp, err := dialer.DialContext(d.ctx, u, header)
	if err != nil {
		if httpResp != nil {
			body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1024))
			httpResp.Body.Close()
			return nil, fmt.Errorf("websocket dial: %w (HTTP %d: %s)", err, httpResp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	return conn, nil
}

// buildURL constructs the Deepgram WebSocket URL from TranscribeOpts.
func (d *DeepgramTranscriber) buildURL() (string, error) {
	u, err := url.Parse(d.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}

	q := u.Query()

	if d.opts.Model != "" {
		q.Set("model", d.opts.Model)
	}
	if d.opts.Language == "multi" {
		q.Set("detect_language", "true")
	} else if d.opts.Language != "" {
		q.Set("language", d.opts.Language)
	}
	if d.opts.Punctuate {
		q.Set("punctuate", "true")
	}
	if d.opts.SmartFormat {
		q.Set("smart_format", "true")
	}
	if d.opts.SampleRate > 0 {
		q.Set("sample_rate", fmt.Sprintf("%d", d.opts.SampleRate))
	}
	if d.opts.Channels > 0 {
		q.Set("channels", fmt.Sprintf("%d", d.opts.Channels))
	}
	if d.opts.Encoding != "" {
		q.Set("encoding", d.opts.Encoding)
	}

	// Multichannel and diarize are set independently based on TranscribeOpts.
	// In v1.0 we always send mono (channels=1) with diarize=true, so only
	// diarize is set here. The independent logic supports future use cases
	// like per-channel diarization (multichannel + diarize together).
	if d.opts.Channels > 1 {
		q.Set("multichannel", "true")
	}
	if d.opts.Diarize {
		q.Set("diarize", "true")
	}

	// Opt out of Deepgram's Model Improvement Program to keep meeting data private.
	q.Set("mip_opt_out", "true")

	if len(d.opts.Keywords) > 0 {
		q.Set("keywords", strings.Join(d.opts.Keywords, ","))
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// readLoop continuously reads messages from the given WebSocket connection,
// parses Deepgram JSON responses, and sends Segments to the output channel.
// Each readLoop is scoped to a specific connection identified by connID.
// When the connection is replaced (by reconnection), this goroutine exits
// without triggering further reconnection attempts.
func (d *DeepgramTranscriber) readLoop(conn *websocket.Conn, connID uint64) {
	defer d.wg.Done()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			d.mu.Lock()
			closed := d.closed
			currentConnID := d.connID
			d.mu.Unlock()

			if closed {
				// Signal that readLoop is done after CloseStream.
				select {
				case d.closeStreamDone <- struct{}{}:
				default:
				}
				return
			}

			// If this connection has been replaced by reconnection,
			// exit silently -- the new readLoop handles the new connection.
			if connID != currentConnID {
				return
			}

			// Trigger reconnection on read error (V-005).
			go d.handleDisconnect(conn, err)
			return
		}

		var resp deepgramResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}

		// Check for Deepgram error responses (invalid params, auth failures, etc.).
		if resp.Type == "Error" || resp.Type == "error" {
			errDetail := resp.ErrMsg
			if errDetail == "" {
				errDetail = resp.Description
			}
			log.Printf("deepgram error: [%s] %s", resp.ErrCode, errDetail)
			continue
		}

		segment, ok := d.responseToSegment(resp)
		if !ok {
			continue
		}

		// Non-blocking send per audio-safety rules.
		select {
		case d.segments <- segment:
		case <-d.ctx.Done():
			return
		default:
			// Channel full -- drop segment rather than block.
		}
	}
}

// responseToSegment converts a Deepgram response to a heimdall.Segment.
// Returns false if the response should be skipped (empty transcript, wrong type).
func (d *DeepgramTranscriber) responseToSegment(resp deepgramResponse) (heimdall.Segment, bool) {
	if resp.Type != "Results" {
		return heimdall.Segment{}, false
	}

	if len(resp.Channel.Alternatives) == 0 {
		return heimdall.Segment{}, false
	}

	alt := resp.Channel.Alternatives[0]
	if alt.Transcript == "" {
		return heimdall.Segment{}, false
	}

	d.mu.Lock()
	offset := d.timeOffset
	d.mu.Unlock()

	speaker := 0
	if len(alt.Words) > 0 {
		speaker = alt.Words[0].Speaker
	}
	// In multichannel mode, the channel index identifies the speaker source
	// (0=system audio, 1=mic). In mono+diarize mode (v1.0 default), this
	// branch is not taken and speaker comes from Deepgram's diarization.
	if d.opts.Channels > 1 && resp.Channel.Index >= 0 {
		speaker = resp.Channel.Index
	}

	startDur := time.Duration(resp.Start*float64(time.Second)) + offset
	endDur := time.Duration((resp.Start+resp.Duration)*float64(time.Second)) + offset

	return heimdall.Segment{
		Speaker:    speaker,
		Text:       alt.Transcript,
		Start:      startDur,
		End:        endDur,
		Confidence: alt.Confidence,
		Channel:    resp.Channel.Index,
		IsFinal:    resp.IsFinal,
	}, true
}

// keepAliveLoop sends KeepAlive messages at the configured interval
// to prevent the Deepgram idle timeout (NET-0001 error).
// Each loop is scoped to a specific connection identified by myConnID.
// When the connection is replaced (by reconnection), this goroutine exits
// so that stale keepAliveLoop goroutines do not accumulate.
func (d *DeepgramTranscriber) keepAliveLoop(myConnID uint64) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			if d.connID != myConnID {
				d.mu.Unlock()
				return // Connection was replaced, exit this keepAliveLoop.
			}
			conn := d.conn
			closed := d.closed
			d.mu.Unlock()

			if closed || conn == nil {
				return
			}

			msg := deepgramMessage{Type: "KeepAlive"}
			data, _ := json.Marshal(msg)

			if err := d.writeMessage(conn, websocket.TextMessage, data); err != nil {
				// Write error will be handled by the read/send path.
				return
			}
		}
	}
}

// reconnectTimer handles V-001: proactive reconnection before the 60-minute
// Deepgram WebSocket timeout. It checks the connection age and triggers
// a seamless reconnection at the configured interval.
func (d *DeepgramTranscriber) reconnectTimer() {
	defer d.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			closed := d.closed
			startTime := d.startTime
			d.mu.Unlock()

			if closed {
				return
			}

			if time.Since(startTime) >= d.reconnectInterval {
				d.proactiveReconnect()
			}
		}
	}
}

// proactiveReconnect performs a seamless reconnection for V-001.
// It opens a new connection before closing the old one to minimize audio loss.
func (d *DeepgramTranscriber) proactiveReconnect() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	oldConn := d.conn
	elapsed := time.Since(d.startTime)
	d.mu.Unlock()

	// Open new connection first (seamless handoff per pipeline-rules.md).
	newConn, err := d.dial()
	if err != nil {
		// Failed to establish new connection -- keep using the old one.
		// It will eventually be force-closed by Deepgram at 60 min.
		log.Printf("warning: proactive reconnect at 55min failed: %v (will retry on disconnect)", err)
		return
	}

	// Swap connections and update state.
	d.mu.Lock()
	// Re-check closed AFTER the dial: Close() can run during the dial window
	// (dial does not hold d.mu). If Close() already set d.closed and reached
	// wg.Wait(), installing newConn and spawning a readLoop below would leave
	// that readLoop blocked on ReadMessage forever (its socket is never closed)
	// -- deadlocking Close(). Bail out and close the orphaned connection instead.
	if d.closed {
		d.mu.Unlock()
		newConn.Close()
		return
	}
	d.conn = newConn
	d.connID++
	newConnID := d.connID
	d.timeOffset += elapsed
	d.startTime = time.Now()

	// Register the new goroutines in the WaitGroup while still holding d.mu.
	// Close() sets d.closed=true under this same mutex before it ever reaches
	// wg.Wait(); since we just observed d.closed==false, these Add calls
	// provably happen-before any wg.Wait() in Close() -- no Add-after-Wait race.
	d.wg.Add(2)
	// The old-connection closer is bounded by a grace-period sleep, so it cannot
	// deadlock Close(). Register it under the same lock to keep all Add calls
	// ordered before Close()'s wg.Wait().
	closeOld := oldConn != nil
	if closeOld {
		d.wg.Add(1)
	}
	d.mu.Unlock()

	// Start readLoop and keepAliveLoop for the new connection.
	go d.readLoop(newConn, newConnID)
	go d.keepAliveLoop(newConnID)

	// Close old connection -- this will cause the old readLoop to exit.
	// The old readLoop checks connID and will not trigger handleDisconnect
	// since its connID no longer matches the current one.
	if closeOld {
		// Send CloseStream to old connection using writeMessage for proper
		// write mutex serialization.
		closeMsg := []byte(`{"type":"CloseStream"}`)
		_ = d.writeMessage(oldConn, websocket.TextMessage, closeMsg)

		// Close old connection after a brief grace period for CloseStream to flush.
		go func() {
			defer d.wg.Done()
			time.Sleep(500 * time.Millisecond)
			oldConn.Close()
		}()
	}
}

// handleDisconnect handles unexpected disconnections (V-005).
// It attempts reconnection with exponential backoff.
// The failedConn parameter identifies which connection failed, preventing
// stale reconnection attempts after a proactive reconnect has already replaced it.
func (d *DeepgramTranscriber) handleDisconnect(failedConn *websocket.Conn, originalErr error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}

	// If the connection has already been replaced (by proactiveReconnect),
	// do not attempt another reconnection.
	if d.conn != failedConn {
		d.mu.Unlock()
		return
	}

	// Guard against concurrent handleDisconnect calls for the same failure.
	if d.reconnecting {
		d.mu.Unlock()
		return
	}
	d.reconnecting = true

	// Close the broken connection.
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	elapsed := time.Since(d.startTime)
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.reconnecting = false
		d.mu.Unlock()
	}()

	backoff := 1 * time.Second

	for attempt := 0; attempt < d.maxReconnectFailures; attempt++ {
		// Check if we have been shut down.
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		// Wait with backoff.
		select {
		case <-d.ctx.Done():
			return
		case <-time.After(backoff):
		}

		newConn, err := d.dial()
		if err != nil {
			// Exponential backoff: 1s, 2s, 4s, 8s, 16s, max 30s.
			backoff = time.Duration(math.Min(
				float64(backoff)*2,
				float64(d.maxBackoff),
			))
			continue
		}

		// Reconnection successful.
		d.mu.Lock()
		// Re-check closed AFTER the dial (same race as proactiveReconnect):
		// Close() may have run during the backoff/dial window. If it already
		// reached wg.Wait(), installing newConn and spawning a readLoop here
		// would deadlock Close() on a readLoop that never sees a closed socket.
		if d.closed {
			d.mu.Unlock()
			newConn.Close()
			return
		}
		d.conn = newConn
		d.connID++
		newConnID := d.connID
		d.timeOffset += elapsed
		d.startTime = time.Now()

		// Register goroutines under d.mu so the Add provably happens-before
		// Close()'s wg.Wait() (Close sets d.closed under this same mutex).
		d.wg.Add(2)
		d.mu.Unlock()

		// Restart the readLoop and keepAliveLoop for the new connection.
		go d.readLoop(newConn, newConnID)
		go d.keepAliveLoop(newConnID)
		return
	}

	// All retries exhausted -- send an error segment to notify the consumer.
	d.mu.Lock()
	currentOffset := d.timeOffset
	d.mu.Unlock()

	select {
	case d.segments <- heimdall.Segment{
		Text:    fmt.Sprintf("[transcription error: reconnection failed after %d attempts: %v]", d.maxReconnectFailures, originalErr),
		IsFinal: true,
		Start:   currentOffset + elapsed,
		End:     currentOffset + elapsed,
	}:
	case <-d.ctx.Done():
	default:
	}
}
