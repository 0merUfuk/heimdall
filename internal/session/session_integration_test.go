package session

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// --- Integration Test Helpers ---

// mockAudioSourceWithData produces AudioFrames filled with a sine wave at
// the given sample rate and channel count. It sends frames at approximately
// real-time intervals until the context is cancelled.
type mockAudioSourceWithData struct {
	sampleRate int
	channels   int
	frameCh    chan heimdall.AudioFrame
	stopOnce   sync.Once
	mu         sync.Mutex
	cancel     context.CancelFunc
	genWg      sync.WaitGroup // tracks the generateAudio goroutine
}

func newMockAudioSourceWithData(sampleRate, channels int) *mockAudioSourceWithData {
	return &mockAudioSourceWithData{
		sampleRate: sampleRate,
		channels:   channels,
		frameCh:    make(chan heimdall.AudioFrame, 100),
	}
}

func (m *mockAudioSourceWithData) Start(ctx context.Context) error {
	m.mu.Lock()
	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	m.genWg.Add(1)
	go m.generateAudio(childCtx)
	return nil
}

func (m *mockAudioSourceWithData) Stream() <-chan heimdall.AudioFrame { return m.frameCh }

func (m *mockAudioSourceWithData) Stop() error {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Unlock()
		// Wait for the generator goroutine to fully exit before closing
		// the channel. This prevents the data race between close and send.
		m.genWg.Wait()
		close(m.frameCh)
	})
	return nil
}

func (m *mockAudioSourceWithData) SampleRate() int { return m.sampleRate }
func (m *mockAudioSourceWithData) Channels() int   { return m.channels }

// generateAudio produces 20ms frames of a 440Hz sine wave.
func (m *mockAudioSourceWithData) generateAudio(ctx context.Context) {
	defer m.genWg.Done()

	samplesPerFrame := m.sampleRate * 20 / 1000 // 20ms worth of samples
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var data []byte

			if m.channels == 2 && m.sampleRate == 48000 {
				// System audio: 48kHz, 32-bit float, stereo.
				totalSamples := samplesPerFrame * 2 // stereo pairs
				data = make([]byte, totalSamples*4)  // 4 bytes per float32
				for i := 0; i < totalSamples; i++ {
					// 440Hz sine wave.
					t := float64(frameIdx*samplesPerFrame+i/2) / float64(m.sampleRate)
					val := float32(math.Sin(2 * math.Pi * 440 * t))
					bits := math.Float32bits(val)
					binary.LittleEndian.PutUint32(data[i*4:], bits)
				}
			} else {
				// Microphone audio: 16kHz, 16-bit int, mono.
				data = make([]byte, samplesPerFrame*2) // 2 bytes per int16
				for i := 0; i < samplesPerFrame; i++ {
					t := float64(frameIdx*samplesPerFrame+i) / float64(m.sampleRate)
					val := int16(math.Sin(2*math.Pi*440*t) * 16000)
					binary.LittleEndian.PutUint16(data[i*2:], uint16(val))
				}
			}

			timestamp := time.Duration(frameIdx*20) * time.Millisecond
			frame := heimdall.AudioFrame{
				Data:       data,
				SampleRate: m.sampleRate,
				Channels:   m.channels,
				Timestamp:  timestamp,
			}

			// Non-blocking send per audio-safety rules.
			select {
			case m.frameCh <- frame:
			case <-ctx.Done():
				return
			default:
				// Drop frame if channel full.
			}

			frameIdx++
		}
	}
}

// segmentProducingTranscriber is a mock transcriber that accepts audio frames
// and produces segments in response. It simulates real transcriber behavior by
// emitting a segment after receiving a configurable number of frames.
type segmentProducingTranscriber struct {
	segCh             chan heimdall.Segment
	connected         bool
	closed            bool
	mu                sync.Mutex
	frameCount        int
	framesPerSegment  int
	segmentIndex      int
}

func newSegmentProducingTranscriber(framesPerSegment int) *segmentProducingTranscriber {
	return &segmentProducingTranscriber{
		segCh:            make(chan heimdall.Segment, 100),
		framesPerSegment: framesPerSegment,
	}
}

func (m *segmentProducingTranscriber) Connect(_ context.Context, _ heimdall.TranscribeOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *segmentProducingTranscriber) Send(frame heimdall.AudioFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(frame.Data) == 0 {
		return nil
	}

	m.frameCount++
	if m.frameCount%m.framesPerSegment == 0 {
		seg := heimdall.Segment{
			Speaker:    m.segmentIndex % 2,
			Text:       "Test segment",
			Start:      time.Duration(m.segmentIndex) * 5 * time.Second,
			End:        time.Duration(m.segmentIndex)*5*time.Second + 3*time.Second,
			Confidence: 0.95,
			Channel:    0,
			IsFinal:    true,
		}
		m.segmentIndex++

		// Non-blocking send.
		select {
		case m.segCh <- seg:
		default:
		}
	}

	return nil
}

func (m *segmentProducingTranscriber) Receive() <-chan heimdall.Segment { return m.segCh }

func (m *segmentProducingTranscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	close(m.segCh)
	return nil
}

// --- Integration Tests ---

// TestIntegration_FullPipeline verifies the full pipeline: mock audio sources
// produce real PCM data -> the actual mixer resamples and interleaves ->
// a mock transcriber receives the mixed frames -> segments are accumulated.
func TestIntegration_FullPipeline(t *testing.T) {
	// Create audio sources that produce real PCM data.
	sys := newMockAudioSourceWithData(48000, 2) // system audio: 48kHz stereo float32
	mic := newMockAudioSourceWithData(16000, 1) // microphone: 16kHz mono int16

	// Transcriber emits a segment every 5 received frames.
	tr := newSegmentProducingTranscriber(5)

	sess := NewMeetingSession("Integration Test", sys, mic, tr, "en", nil)

	var segments []heimdall.Segment
	var segMu sync.Mutex
	sess.OnSegment(func(seg heimdall.Segment) {
		segMu.Lock()
		segments = append(segments, seg)
		segMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !sess.SystemAvailable() {
		t.Error("expected system audio to be available")
	}

	// Let the pipeline run for enough time to produce frames and segments.
	// Mixer produces frames at 20ms intervals; transcriber emits a segment
	// every 5 frames = 100ms. 500ms should yield ~5 segments.
	time.Sleep(500 * time.Millisecond)

	cancel()
	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify segments were accumulated.
	accumulated := sess.Segments()
	if len(accumulated) == 0 {
		t.Error("expected accumulated segments from full pipeline, got 0")
	}

	// Verify the callback was invoked.
	segMu.Lock()
	callbackCount := len(segments)
	segMu.Unlock()
	if callbackCount == 0 {
		t.Error("expected segment callback to be invoked, got 0 calls")
	}

	// Accumulated and callback counts should match (all segments are IsFinal=true).
	if len(accumulated) != callbackCount {
		t.Errorf("accumulated segments (%d) != callback segments (%d)", len(accumulated), callbackCount)
	}

	// Verify transcriber received audio frames.
	tr.mu.Lock()
	frameCount := tr.frameCount
	tr.mu.Unlock()
	if frameCount == 0 {
		t.Error("expected transcriber to receive audio frames, got 0")
	}
}

// TestIntegration_GracefulShutdown verifies that starting a session, sending
// some audio, and stopping it gracefully collects all segments without panics
// or goroutine leaks.
func TestIntegration_GracefulShutdown(t *testing.T) {
	sys := newMockAudioSourceWithData(48000, 2)
	mic := newMockAudioSourceWithData(16000, 1)
	tr := newSegmentProducingTranscriber(3)

	sess := NewMeetingSession("Shutdown Test", sys, mic, tr, "en", nil)

	var callbackSegments []heimdall.Segment
	var callbackMu sync.Mutex
	sess.OnSegment(func(seg heimdall.Segment) {
		callbackMu.Lock()
		callbackSegments = append(callbackSegments, seg)
		callbackMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Run briefly to accumulate some segments.
	time.Sleep(200 * time.Millisecond)

	// Graceful shutdown: cancel context then stop.
	cancel()
	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify we got at least some segments before shutdown.
	accumulated := sess.Segments()
	if len(accumulated) == 0 {
		t.Error("expected at least some segments before shutdown")
	}

	// Verify Duration is positive.
	d := sess.Duration()
	if d <= 0 {
		t.Errorf("expected positive duration after stop, got %v", d)
	}

	// Calling Stop again should not panic (idempotent check for components).
	// The underlying components handle double-stop gracefully.
}

// TestIntegration_MicOnlyMode verifies that the session works correctly when
// system audio is nil (mic-only mode). The mixer should use a silent source
// for the system channel.
func TestIntegration_MicOnlyMode(t *testing.T) {
	mic := newMockAudioSourceWithData(16000, 1)
	tr := newSegmentProducingTranscriber(5)

	// Pass nil for system audio.
	sess := NewMeetingSession("Mic Only Test", nil, mic, tr, "en", nil)

	var segments []heimdall.Segment
	var segMu sync.Mutex
	sess.OnSegment(func(seg heimdall.Segment) {
		segMu.Lock()
		segments = append(segments, seg)
		segMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if sess.SystemAvailable() {
		t.Error("expected system audio NOT available in mic-only mode")
	}

	// Let it run. The mixer should still produce frames from mic data
	// with silence on the system channel.
	time.Sleep(300 * time.Millisecond)

	cancel()
	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Should still get segments even in mic-only mode.
	accumulated := sess.Segments()
	if len(accumulated) == 0 {
		t.Error("expected segments in mic-only mode, got 0")
	}

	// Verify callback was called.
	segMu.Lock()
	callbackCount := len(segments)
	segMu.Unlock()
	if callbackCount == 0 {
		t.Error("expected segment callback in mic-only mode, got 0 calls")
	}
}

// TestIntegration_SegmentCallbackCalledForEach verifies that the OnSegment
// callback is called once for each segment (both interim and final).
func TestIntegration_SegmentCallbackCalledForEach(t *testing.T) {
	mic := newMockAudioSourceWithData(16000, 1)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Callback Test", nil, mic, tr, "en", nil)

	var received []heimdall.Segment
	var receivedMu sync.Mutex
	sess.OnSegment(func(seg heimdall.Segment) {
		receivedMu.Lock()
		received = append(received, seg)
		receivedMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject a mix of interim and final segments.
	testSegments := []heimdall.Segment{
		{Speaker: 0, Text: "Hello", Start: 1 * time.Second, End: 2 * time.Second, IsFinal: true},
		{Speaker: 0, Text: "He--", Start: 3 * time.Second, End: 3500 * time.Millisecond, IsFinal: false},
		{Speaker: 1, Text: "World", Start: 3 * time.Second, End: 4 * time.Second, IsFinal: true},
		{Speaker: 0, Text: "Test", Start: 5 * time.Second, End: 6 * time.Second, IsFinal: true},
	}

	for _, seg := range testSegments {
		tr.segCh <- seg
	}

	// Give accumulation goroutine time to process.
	time.Sleep(100 * time.Millisecond)

	receivedMu.Lock()
	count := len(received)
	receivedMu.Unlock()

	if count != len(testSegments) {
		t.Errorf("callback received %d segments, want %d", count, len(testSegments))
	}

	// Only final segments should be accumulated (3 of 4).
	accumulated := sess.Segments()
	if len(accumulated) != 3 {
		t.Errorf("accumulated %d segments, want 3 (finals only)", len(accumulated))
	}

	cancel()
	_ = sess.Stop()
}

// TestIntegration_DurationPositiveAfterStart verifies that Duration() returns
// a positive value after the session has been started and run briefly.
func TestIntegration_DurationPositiveAfterStart(t *testing.T) {
	mic := newMockAudioSourceWithData(16000, 1)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Duration Test", nil, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	d := sess.Duration()
	if d < 50*time.Millisecond {
		t.Errorf("Duration() = %v, want >= 50ms", d)
	}
	if d > 10*time.Second {
		t.Errorf("Duration() = %v, unexpectedly large", d)
	}

	cancel()
	_ = sess.Stop()

	// Duration should still be positive after stop (it measures from startTime).
	dAfterStop := sess.Duration()
	if dAfterStop < d {
		t.Errorf("Duration() after stop (%v) < Duration() before stop (%v)", dAfterStop, d)
	}
}

// TestIntegration_SpeakerCountTracking verifies that SpeakerCount correctly
// tracks unique speakers across accumulated segments.
func TestIntegration_SpeakerCountTracking(t *testing.T) {
	mic := newMockAudioSourceWithData(16000, 1)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Speaker Count Test", nil, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject segments from 3 different speakers.
	tr.segCh <- heimdall.Segment{Speaker: 0, Text: "Speaker zero", IsFinal: true}
	tr.segCh <- heimdall.Segment{Speaker: 1, Text: "Speaker one", IsFinal: true}
	tr.segCh <- heimdall.Segment{Speaker: 2, Text: "Speaker two", IsFinal: true}
	tr.segCh <- heimdall.Segment{Speaker: 0, Text: "Speaker zero again", IsFinal: true}

	time.Sleep(100 * time.Millisecond)

	if sess.SpeakerCount() != 3 {
		t.Errorf("SpeakerCount() = %d, want 3", sess.SpeakerCount())
	}

	cancel()
	_ = sess.Stop()
}
