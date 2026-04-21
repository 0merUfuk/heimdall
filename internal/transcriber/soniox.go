package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/gorilla/websocket"
)

// Compile-time assertion: SonioxTranscriber must satisfy Transcriber.
var _ Transcriber = (*SonioxTranscriber)(nil)

const (
	// defaultSonioxBaseURL is the Soniox real-time streaming WebSocket endpoint.
	defaultSonioxBaseURL = "wss://stt-rt.soniox.com/transcribe-websocket"

	// defaultSonioxModel is the current production real-time model
	// (`stt-rt-preview` is a superseded alias per docs, do not use).
	defaultSonioxModel = "stt-rt-v4"

	// sonioxSegmentBufferSize is the outgoing segment channel capacity.
	// Matches Deepgram's segmentBufferSize for symmetry.
	sonioxSegmentBufferSize = 256

	// sonioxCloseTimeout is how long Close() waits for the server's
	// {"finished": true} handshake frame after sending the empty
	// end-of-stream frame. Soniox typically responds within a few hundred
	// milliseconds; 2s is a forgiving cap.
	sonioxCloseTimeout = 2 * time.Second

	// sonioxMaxReconnectFailures matches Deepgram's V-005 policy.
	sonioxMaxReconnectFailures = 5

	// sonioxMaxBackoff matches Deepgram's V-005 policy.
	sonioxMaxBackoff = 30 * time.Second
)

// sonioxConfigMessage is the first-frame JSON payload that authenticates
// the client and configures the session. Soniox requires config over the
// WebSocket itself — there is no Authorization header or query string.
//
// Fields are omitempty so zero-value options do not leak into the wire
// payload; the mock server in soniox_test.go asserts against the shape of
// this message.
type sonioxConfigMessage struct {
	APIKey                       string   `json:"api_key"`
	Model                        string   `json:"model"`
	AudioFormat                  string   `json:"audio_format"`
	SampleRate                   int      `json:"sample_rate"`
	NumChannels                  int      `json:"num_channels"`
	LanguageHints                []string `json:"language_hints,omitempty"`
	EnableLanguageIdentification bool     `json:"enable_language_identification,omitempty"`
	EnableSpeakerDiarization     bool     `json:"enable_speaker_diarization,omitempty"`
}

// sonioxToken is a single unit in a Soniox response frame. Tokens are
// subword — spaces and punctuation come as their own tokens with `text: " "`
// or `text: "."`. Accumulator logic must concatenate `text` verbatim.
type sonioxToken struct {
	Text       string  `json:"text"`
	StartMS    int64   `json:"start_ms,omitempty"`
	EndMS      int64   `json:"end_ms,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	IsFinal    bool    `json:"is_final,omitempty"`
	Speaker    string  `json:"speaker,omitempty"`  // only when diarization enabled
	Language   string  `json:"language,omitempty"` // only when language ID enabled
}

// sonioxResponse is one JSON text frame from the server. It contains a
// batch of tokens plus metadata. When `Finished` is true this is the final
// frame before the server closes the connection.
type sonioxResponse struct {
	Tokens            []sonioxToken `json:"tokens"`
	FinalAudioProcMS  int64         `json:"final_audio_proc_ms,omitempty"`
	TotalAudioProcMS  int64         `json:"total_audio_proc_ms,omitempty"`
	Finished          bool          `json:"finished,omitempty"`
	ErrorCode         int           `json:"error_code,omitempty"`
	ErrorMessage      string        `json:"error_message,omitempty"`
}

// sonioxSessionConfig mirrors the subset of config.SonioxConfig that this
// package needs. Redeclared locally so the transcriber package does not
// import the config package (circular dependency guard: factory.go is the
// only place that touches config types).
type sonioxSessionConfig struct {
	APIKey   string
	Model    string
	Language string
}

// SonioxTranscriber implements the Transcriber interface using Soniox's
// real-time WebSocket streaming STT API.
//
// Design notes:
//   - Auth is a JSON text frame sent as the FIRST WebSocket message (not an
//     Authorization header, not a query string).
//   - Audio frames are sent as binary WebSocket messages after the config
//     frame. Raw PCM s16le 16kHz mono matches heimdall's post-downmix
//     pipeline output (ID-001) with no transcoding.
//   - V-001 proactive-reconnect logic is INTENTIONALLY omitted. Soniox's
//     session cap is 300 minutes (5 hours); the Deepgram-specific 55-minute
//     preemptive reconnect does not apply. Network-drop reconnection with
//     exponential backoff (V-005) is preserved.
//   - Graceful close: send an empty binary frame, then wait (≤2s) for the
//     server's {"finished": true} frame before tearing down. Closing the
//     WebSocket directly drops the tail of final tokens.
//   - Provisional tokens (is_final=false) are DROPPED at the accumulator
//     stage for this scaffold. Rationale: keeps the initial implementation
//     simple and the accumulator stage in session.go does not yet surface
//     interim segments for live display beyond Deepgram's existing interim
//     path. See TODO(soniox-interim) below.
type SonioxTranscriber struct {
	cfg      sonioxSessionConfig
	opts     heimdall.TranscribeOpts
	baseURL  string
	segments chan heimdall.Segment

	mu      sync.Mutex // protects conn, connID, closed, segmentsClosed, timeOffset, reconnecting
	writeMu sync.Mutex // serializes WebSocket writes (gorilla/websocket forbids concurrent writes)

	conn           *websocket.Conn
	connID         uint64
	closed         bool
	segmentsClosed bool // guards close(s.segments) against double-close

	// Network-drop reconnection state (V-005).
	reconnecting bool
	timeOffset   time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// closeStreamDone is signalled when the read loop exits after the
	// server's `finished:true` handshake (or an error while closing).
	closeStreamDone chan struct{}

	// Accumulator state. Protected by mu.
	accTokens   []sonioxToken // committed-final tokens since last boundary
	accSpeaker  string        // speaker at accumulator start
	accLanguage string        // language at accumulator start

	// Configurable fields (overrideable in tests).
	closeTimeout         time.Duration
	maxReconnectFailures int
	maxBackoff           time.Duration
}

// NewSonioxTranscriber creates a new Soniox transcriber. The transcriber
// is not connected until Connect is called. The caller is responsible for
// calling Close to release resources.
//
// The session config captures provider-specific settings (API key, model,
// language). Shared TranscribeOpts are passed at Connect time per the
// Transcriber interface — provider-specific extensions live on the config.
func NewSonioxTranscriber(cfg sonioxSessionConfig) *SonioxTranscriber {
	if cfg.Model == "" {
		cfg.Model = defaultSonioxModel
	}
	return &SonioxTranscriber{
		cfg:                  cfg,
		baseURL:              defaultSonioxBaseURL,
		segments:             make(chan heimdall.Segment, sonioxSegmentBufferSize),
		closeStreamDone:      make(chan struct{}, 1),
		closeTimeout:         sonioxCloseTimeout,
		maxReconnectFailures: sonioxMaxReconnectFailures,
		maxBackoff:           sonioxMaxBackoff,
	}
}

// Connect establishes the WebSocket connection, sends the first-message
// JSON config frame, and starts the read goroutine.
func (s *SonioxTranscriber) Connect(ctx context.Context, opts heimdall.TranscribeOpts) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("soniox connect: %w", ErrAlreadyClosed)
	}
	if s.conn != nil {
		s.mu.Unlock()
		return fmt.Errorf("soniox connect: %w", ErrAlreadyConnected)
	}
	if s.cfg.APIKey == "" {
		s.mu.Unlock()
		return fmt.Errorf("soniox connect: api key is required")
	}
	s.opts = opts
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	conn, err := s.dial()
	if err != nil {
		return fmt.Errorf("soniox connect: %w", err)
	}

	if err := s.sendConfig(conn); err != nil {
		conn.Close()
		return fmt.Errorf("soniox connect: send config: %w", err)
	}

	s.mu.Lock()
	s.conn = conn
	s.connID++
	currentConnID := s.connID
	s.mu.Unlock()

	s.wg.Add(1)
	go s.readLoop(conn, currentConnID)

	return nil
}

// Send streams an audio frame as a binary WebSocket message. The frame
// must be raw s16le 16kHz mono PCM — heimdall's post-downmix pipeline
// output (ID-001) matches this natively.
func (s *SonioxTranscriber) Send(frame heimdall.AudioFrame) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("soniox send: %w", ErrAlreadyClosed)
	}
	conn := s.conn
	s.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("soniox send: %w", ErrNotConnected)
	}

	if err := s.writeMessage(conn, websocket.BinaryMessage, frame.Data); err != nil {
		// Trigger reconnection on write error (V-005).
		go s.handleDisconnect(conn, err)
		return fmt.Errorf("soniox send: %w", err)
	}

	return nil
}

// Receive returns the read-only segment channel. The channel is closed
// when Close() completes.
func (s *SonioxTranscriber) Receive() <-chan heimdall.Segment {
	return s.segments
}

// Close gracefully tears down the connection. It sends the Soniox
// end-of-stream handshake (an empty binary frame), waits briefly for the
// server's {"finished": true} reply, and then closes the WebSocket and the
// segment channel. Calling Close more than once is safe. Also safe to call
// after a server-initiated shutdown has already closed the segments channel
// — the segmentsClosed flag prevents a double-close panic.
func (s *SonioxTranscriber) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conn := s.conn
	s.mu.Unlock()

	if conn != nil {
		// Empty binary frame = Soniox end-of-stream marker. Docs explicitly
		// warn against closing the WebSocket directly (you lose the tail
		// of final tokens).
		_ = s.writeMessage(conn, websocket.BinaryMessage, []byte{})

		select {
		case <-s.closeStreamDone:
		case <-time.After(s.closeTimeout):
		}
	}

	if s.cancel != nil {
		s.cancel()
	}

	// Close the WebSocket so any blocked ReadMessage calls unwind.
	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.mu.Unlock()

	s.wg.Wait()

	// Flush any remaining accumulated finals into a last segment before
	// closing the channel so tail tokens aren't silently dropped on tidy
	// shutdowns where the server flushed but never emitted an explicit
	// utterance boundary.
	s.mu.Lock()
	tail := s.flushAccumulatorLocked()
	offset := s.timeOffset
	alreadyClosed := s.segmentsClosed
	s.mu.Unlock()
	if tail != nil && !alreadyClosed {
		seg := s.tokensToSegment(tail, offset)
		if seg.Text != "" {
			select {
			case s.segments <- seg:
			default:
			}
		}
	}

	s.closeSegmentsOnce()
	return nil
}

// closeSegmentsOnce closes the segments channel exactly once. Both Close()
// and shutdownFromServer() call this; the segmentsClosed flag guards
// against a double-close panic.
func (s *SonioxTranscriber) closeSegmentsOnce() {
	s.mu.Lock()
	if s.segmentsClosed {
		s.mu.Unlock()
		return
	}
	s.segmentsClosed = true
	s.mu.Unlock()
	close(s.segments)
}

// shutdownFromServer tears down the session in response to a
// server-initiated finished:true frame (e.g. rate limiting, inactivity
// timeout). Mirrors the Close() tail: cancels the context, closes the
// WebSocket, closes the segments channel. Unlike Close(), it does NOT send
// an empty frame (the server is already done) and does NOT wait for
// closeStreamDone (the readLoop that called it is already returning).
func (s *SonioxTranscriber) shutdownFromServer() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.mu.Unlock()

	s.closeSegmentsOnce()
}

// writeMessage serializes WebSocket writes. gorilla/websocket does not
// support concurrent writes, so every write path goes through writeMu.
func (s *SonioxTranscriber) writeMessage(conn *websocket.Conn, messageType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return conn.WriteMessage(messageType, data)
}

// dial opens a WebSocket connection to Soniox. No headers or query
// parameters are required — auth is deferred to the first-message JSON.
func (s *SonioxTranscriber) dial() (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, httpResp, err := dialer.DialContext(s.ctx, s.baseURL, http.Header{})
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

// sendConfig writes the first-message JSON config frame. This is the
// authentication and session-configuration step — it must be the very
// first frame after the WebSocket handshake, before any audio bytes.
func (s *SonioxTranscriber) sendConfig(conn *websocket.Conn) error {
	msg := s.buildConfigMessage()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := s.writeMessage(conn, websocket.TextMessage, data); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// buildConfigMessage translates TranscribeOpts + SonioxConfig into the
// Soniox first-frame JSON shape. Language handling:
//   - "multi" or "auto" or empty → language_hints=["tr","en"] + language ID
//     on. This is the TR+EN code-switched mode that motivated AD-011.
//   - any specific code (e.g. "tr", "en") → single-language hint. We still
//     set language_hints so the decoder can fall back to English proper
//     nouns/acronyms that appear in otherwise-Turkish speech.
func (s *SonioxTranscriber) buildConfigMessage() sonioxConfigMessage {
	msg := sonioxConfigMessage{
		APIKey:                   s.cfg.APIKey,
		Model:                    s.cfg.Model,
		AudioFormat:              "s16le",
		SampleRate:               16000,
		NumChannels:              1,
		EnableSpeakerDiarization: s.opts.Diarize,
	}
	if msg.Model == "" {
		msg.Model = defaultSonioxModel
	}
	if s.opts.SampleRate > 0 {
		msg.SampleRate = s.opts.SampleRate
	}
	if s.opts.Channels > 0 {
		msg.NumChannels = s.opts.Channels
	}

	// Prefer opts.Language (from --language) over cfg.Language (config file)
	// so the flag wins when both are present.
	lang := s.opts.Language
	if lang == "" {
		lang = s.cfg.Language
	}

	switch lang {
	case "", "multi", "auto":
		msg.LanguageHints = []string{"tr", "en"}
		msg.EnableLanguageIdentification = true
	default:
		msg.LanguageHints = []string{lang}
		msg.EnableLanguageIdentification = true
	}

	return msg
}

// readLoop reads JSON response frames until the connection closes, the
// server emits `finished:true`, or the context is cancelled. Emits
// heimdall.Segment values onto the segments channel as utterance
// boundaries are crossed.
func (s *SonioxTranscriber) readLoop(conn *websocket.Conn, connID uint64) {
	defer s.wg.Done()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			currentConnID := s.connID
			s.mu.Unlock()

			if closed {
				select {
				case s.closeStreamDone <- struct{}{}:
				default:
				}
				return
			}

			// If this connection has been superseded by a reconnect,
			// exit quietly — the new readLoop owns the successor conn.
			if connID != currentConnID {
				return
			}

			go s.handleDisconnect(conn, err)
			return
		}

		var resp sonioxResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}

		if resp.ErrorCode != 0 || resp.ErrorMessage != "" {
			log.Printf("soniox error: [%d] %s", resp.ErrorCode, resp.ErrorMessage)
			continue
		}

		s.processTokens(resp.Tokens)

		if resp.Finished {
			// Server signalled clean end-of-stream. Flush any partial
			// accumulator and signal Close().
			s.mu.Lock()
			tail := s.flushAccumulatorLocked()
			offset := s.timeOffset
			wasClientInitiated := s.closed
			s.mu.Unlock()
			if tail != nil {
				seg := s.tokensToSegment(tail, offset)
				if seg.Text != "" {
					s.emit(seg)
				}
			}
			select {
			case s.closeStreamDone <- struct{}{}:
			default:
			}
			// Server-initiated finished:true (e.g. rate limiting, inactivity)
			// arrives without Close() having been called. Shut the session
			// down from this side so downstream Receive() consumers observe
			// the channel closing rather than blocking indefinitely.
			if !wasClientInitiated {
				go s.shutdownFromServer()
			}
			return
		}
	}
}

// processTokens walks a batch of tokens, buffers finals, and emits a
// Segment at each utterance boundary. Provisionals are dropped — see the
// TODO(soniox-interim) note on the struct doc comment.
func (s *SonioxTranscriber) processTokens(tokens []sonioxToken) {
	for _, tok := range tokens {
		// TODO(soniox-interim): provisional tokens are silently dropped
		// for the scaffold. If the accumulator stage grows a real interim-
		// display UI, emit is_final=false Segments here (one per batch,
		// or on a debounce) so the terminal render can flicker them in.
		if !tok.IsFinal {
			continue
		}

		s.mu.Lock()

		// Speaker-change boundary: flush current accumulator before
		// appending a token from a different speaker.
		if len(s.accTokens) > 0 && tok.Speaker != "" && tok.Speaker != s.accSpeaker {
			tail := s.flushAccumulatorLocked()
			offset := s.timeOffset
			s.mu.Unlock()
			if tail != nil {
				seg := s.tokensToSegment(tail, offset)
				if seg.Text != "" {
					s.emit(seg)
				}
			}
			s.mu.Lock()
		}

		if len(s.accTokens) == 0 {
			s.accSpeaker = tok.Speaker
			s.accLanguage = tok.Language
		}
		s.accTokens = append(s.accTokens, tok)

		// Sentence-ending punctuation boundary: flush after appending.
		if isSentenceBoundary(tok.Text) {
			tail := s.flushAccumulatorLocked()
			offset := s.timeOffset
			s.mu.Unlock()
			if tail != nil {
				seg := s.tokensToSegment(tail, offset)
				if seg.Text != "" {
					s.emit(seg)
				}
			}
			continue
		}
		s.mu.Unlock()
	}
}

// flushAccumulatorLocked returns the buffered finals and clears state.
// Caller must hold s.mu.
func (s *SonioxTranscriber) flushAccumulatorLocked() []sonioxToken {
	if len(s.accTokens) == 0 {
		return nil
	}
	out := s.accTokens
	s.accTokens = nil
	s.accSpeaker = ""
	s.accLanguage = ""
	return out
}

// tokensToSegment concatenates token texts (preserving verbatim spaces),
// derives speaker/start/end/confidence from the token stream, and returns
// a heimdall.Segment.
func (s *SonioxTranscriber) tokensToSegment(tokens []sonioxToken, offset time.Duration) heimdall.Segment {
	if len(tokens) == 0 {
		return heimdall.Segment{}
	}

	var b strings.Builder
	var sumConf float64
	var confCount int
	for _, t := range tokens {
		b.WriteString(t.Text)
		if t.Confidence > 0 {
			sumConf += t.Confidence
			confCount++
		}
	}

	text := strings.TrimSpace(b.String())
	avgConf := 0.0
	if confCount > 0 {
		avgConf = sumConf / float64(confCount)
	}

	// Speaker: take from the first token that had a non-empty label.
	// Soniox uses string labels ("1", "2", ...) — parse as int where
	// possible, otherwise fall back to 0 to keep the Segment schema
	// compatible with the existing accumulator/renderer.
	speakerInt := 0
	for _, t := range tokens {
		if t.Speaker != "" {
			speakerInt = parseSpeakerLabel(t.Speaker)
			break
		}
	}

	start := time.Duration(tokens[0].StartMS)*time.Millisecond + offset
	end := time.Duration(tokens[len(tokens)-1].EndMS)*time.Millisecond + offset

	return heimdall.Segment{
		Speaker:    speakerInt,
		Text:       text,
		Start:      start,
		End:        end,
		Confidence: avgConf,
		Channel:    0,
		IsFinal:    true,
	}
}

// emit sends a segment onto the output channel and advances timeOffset so
// the post-reconnect accumulator starts after this segment's end. Never
// blocks — consumer falling behind results in dropped segments, consistent
// with audio-safety rules.
func (s *SonioxTranscriber) emit(seg heimdall.Segment) {
	// Advance timeOffset to this segment's end so any subsequent reconnect
	// (whose token timestamps restart from zero) continues monotonically.
	// Guarded by mu because handleDisconnect reads timeOffset under the
	// same lock.
	s.mu.Lock()
	if seg.End > s.timeOffset {
		s.timeOffset = seg.End
	}
	s.mu.Unlock()

	select {
	case s.segments <- seg:
	case <-s.ctx.Done():
	default:
		// Channel full. Dropping is preferable to blocking the read goroutine
		// per audio-safety.md.
	}
}

// handleDisconnect reconnects after an unexpected network failure. Mirrors
// the Deepgram policy: exponential backoff (1s, 2s, 4s, ..., max 30s), up
// to maxReconnectFailures attempts, then surface an error segment. The
// failedConn guard prevents a stale readLoop from double-triggering
// reconnection after a successor conn has already replaced it.
func (s *SonioxTranscriber) handleDisconnect(failedConn *websocket.Conn, originalErr error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.conn != failedConn {
		s.mu.Unlock()
		return
	}
	if s.reconnecting {
		s.mu.Unlock()
		return
	}
	s.reconnecting = true

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	// Soniox token timestamps are from stream start, so a new conn resets
	// them to 0. timeOffset holds the end-time of the last-emitted segment
	// (updated on every emit()) so reconnected segments remain monotonic.
	// Single-session-to-reconnect transitions are covered by tests; the
	// multi-reconnect-in-one-meeting path inherits the same invariant from
	// the per-emission write but is not exercised by tests yet.
	existingOffset := s.timeOffset
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.reconnecting = false
		s.mu.Unlock()
	}()

	backoff := 1 * time.Second

	for attempt := 0; attempt < s.maxReconnectFailures; attempt++ {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backoff):
		}

		newConn, err := s.dial()
		if err != nil {
			backoff = time.Duration(math.Min(float64(backoff)*2, float64(s.maxBackoff)))
			continue
		}

		if err := s.sendConfig(newConn); err != nil {
			newConn.Close()
			backoff = time.Duration(math.Min(float64(backoff)*2, float64(s.maxBackoff)))
			continue
		}

		s.mu.Lock()
		s.conn = newConn
		s.connID++
		newConnID := s.connID
		// Preserve the cumulative timeOffset across reconnects.
		s.timeOffset = existingOffset
		s.mu.Unlock()

		s.wg.Add(1)
		go s.readLoop(newConn, newConnID)
		return
	}

	// All retries exhausted — notify the consumer.
	s.mu.Lock()
	offset := s.timeOffset
	s.mu.Unlock()
	select {
	case s.segments <- heimdall.Segment{
		Text:    fmt.Sprintf("[transcription error: soniox reconnection failed after %d attempts: %v]", s.maxReconnectFailures, originalErr),
		IsFinal: true,
		Start:   offset,
		End:     offset,
	}:
	case <-s.ctx.Done():
	default:
	}
}

// isSentenceBoundary returns true when `text` is a sentence-ending
// punctuation token. Soniox emits punctuation as standalone tokens.
func isSentenceBoundary(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	return t == "." || t == "?" || t == "!"
}

// parseSpeakerLabel converts Soniox's string speaker label to an int
// suitable for heimdall.Segment.Speaker. Soniox uses "1", "2", etc.; the
// Deepgram-era schema uses 0-based ints. Subtract one so Soniox "1" maps
// to Segment.Speaker=0, matching Deepgram's first-speaker convention.
// Unknown / unparseable labels fall back to 0.
func parseSpeakerLabel(label string) int {
	var n int
	if _, err := fmt.Sscanf(label, "%d", &n); err != nil {
		return 0
	}
	if n > 0 {
		return n - 1
	}
	return n
}
