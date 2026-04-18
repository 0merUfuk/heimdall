package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// mockAudioSource is a test double for audio.AudioSource.
type mockAudioSource struct {
	sampleRate int
	channels   int
	frameCh    chan heimdall.AudioFrame
	startErr   error
	stopErr    error
	started    bool
	stopped    bool
	mu         sync.Mutex
}

func newMockAudioSource(sampleRate, channels int) *mockAudioSource {
	return &mockAudioSource{
		sampleRate: sampleRate,
		channels:   channels,
		frameCh:    make(chan heimdall.AudioFrame, 100),
	}
}

func (m *mockAudioSource) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockAudioSource) Stream() <-chan heimdall.AudioFrame { return m.frameCh }

func (m *mockAudioSource) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	// Close the channel to signal end of stream.
	select {
	case <-m.frameCh:
	default:
	}
	// Only close if not already closed.
	defer func() { recover() }()
	close(m.frameCh)
	return m.stopErr
}

func (m *mockAudioSource) SampleRate() int { return m.sampleRate }
func (m *mockAudioSource) Channels() int   { return m.channels }

// mockTranscriber is a test double for transcriber.Transcriber.
type mockTranscriber struct {
	segCh      chan heimdall.Segment
	connectErr error
	closeErr   error
	sendErr    error
	connected  bool
	closed     bool
	frames     []heimdall.AudioFrame
	mu         sync.Mutex
}

func newMockTranscriber() *mockTranscriber {
	return &mockTranscriber{
		segCh: make(chan heimdall.Segment, 100),
	}
}

func (m *mockTranscriber) Connect(_ context.Context, _ heimdall.TranscribeOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	return nil
}

func (m *mockTranscriber) Send(frame heimdall.AudioFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.frames = append(m.frames, frame)
	return nil
}

func (m *mockTranscriber) Receive() <-chan heimdall.Segment { return m.segCh }

func (m *mockTranscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	close(m.segCh)
	return m.closeErr
}

func TestNewMeetingSession(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	sys := newMockAudioSource(48000, 2)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test Meeting", sys, mic, tr, "en", nil)

	if sess.Title() != "Test Meeting" {
		t.Errorf("expected title 'Test Meeting', got %q", sess.Title())
	}
	if sess.language != "en" {
		t.Errorf("expected language 'en', got %q", sess.language)
	}
}

func TestNewMeetingSession_Language(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{"turkish", "tr", "tr"},
		{"english", "en", "en"},
		{"multi", "multi", "multi"},
		{"empty defaults to en", "", "en"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mic := newMockAudioSource(16000, 1)
			tr := newMockTranscriber()
			sess := NewMeetingSession("Test", nil, mic, tr, tc.language, nil)
			if sess.language != tc.want {
				t.Errorf("language = %q, want %q", sess.language, tc.want)
			}
		})
	}
}

func TestStartAndStop(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	sys := newMockAudioSource(48000, 2)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test", sys, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify components were started.
	if !sess.SystemAvailable() {
		t.Error("expected system audio to be available")
	}

	// Let the session run briefly.
	time.Sleep(50 * time.Millisecond)
	cancel()

	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestStartWithoutSystemAudio(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	tr := newMockTranscriber()

	// Pass nil for system audio.
	sess := NewMeetingSession("Test", nil, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if sess.SystemAvailable() {
		t.Error("expected system audio to NOT be available")
	}

	cancel()
	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestSystemAudioFailsFallsBackToMicOnly(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	sys := newMockAudioSource(48000, 2)
	sys.startErr = context.DeadlineExceeded // simulate failure
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test", sys, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not fail -- falls back to mic only.
	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start should succeed with mic-only fallback: %v", err)
	}

	if sess.SystemAvailable() {
		t.Error("expected system audio NOT available after start failure")
	}

	cancel()
	_ = sess.Stop()
}

func TestSegmentAccumulation(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	sys := newMockAudioSource(48000, 2)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test", sys, mic, tr, "en", nil)

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

	// Inject segments via the mock transcriber.
	segments := []heimdall.Segment{
		{Speaker: 0, Text: "Hello", Start: 1 * time.Second, End: 2 * time.Second, IsFinal: true},
		{Speaker: 1, Text: "World", Start: 2 * time.Second, End: 3 * time.Second, IsFinal: true},
		{Speaker: 0, Text: "interim...", Start: 3 * time.Second, End: 4 * time.Second, IsFinal: false},
	}

	for _, seg := range segments {
		tr.segCh <- seg
	}

	// Give time for accumulation.
	time.Sleep(100 * time.Millisecond)

	// Check accumulated segments (only finals).
	accumulated := sess.Segments()
	if len(accumulated) != 2 {
		t.Errorf("expected 2 accumulated segments, got %d", len(accumulated))
	}

	// Check callback received all 3 (including interim).
	receivedMu.Lock()
	receivedCount := len(received)
	receivedMu.Unlock()
	if receivedCount != 3 {
		t.Errorf("expected callback to receive 3 segments, got %d", receivedCount)
	}

	// Check speaker count.
	if sess.SpeakerCount() != 2 {
		t.Errorf("expected 2 speakers, got %d", sess.SpeakerCount())
	}

	cancel()
	_ = sess.Stop()
}

func TestTranscriberConnectError(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	sys := newMockAudioSource(48000, 2)
	tr := newMockTranscriber()
	tr.connectErr = context.DeadlineExceeded

	sess := NewMeetingSession("Test", sys, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sess.Start(ctx)
	if err == nil {
		t.Fatal("expected error when transcriber connect fails")
	}
}

func TestMicStartError(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	mic.startErr = context.DeadlineExceeded
	sys := newMockAudioSource(48000, 2)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test", sys, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sess.Start(ctx)
	if err == nil {
		t.Fatal("expected error when mic start fails")
	}
}

func TestDuration(t *testing.T) {
	mic := newMockAudioSource(16000, 1)
	tr := newMockTranscriber()

	sess := NewMeetingSession("Test", nil, mic, tr, "en", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	d := sess.Duration()
	if d < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", d)
	}

	cancel()
	_ = sess.Stop()
}

func TestDownmixStereoToMono(t *testing.T) {
	tests := []struct {
		name     string
		frame    heimdall.AudioFrame
		wantCh   int
		wantData []int16 // expected mono samples (nil means check frame.Data unchanged)
	}{
		{
			name: "stereo frame is downmixed",
			frame: heimdall.AudioFrame{
				// Stereo: L=100, R=200, L=300, R=400
				Data:       []byte{100, 0, 200, 0, 44, 1, 144, 1}, // int16 LE: 100, 200, 300, 400
				SampleRate: 16000,
				Channels:   2,
				Timestamp:  42 * time.Millisecond,
			},
			wantCh:   1,
			wantData: []int16{150, 350}, // (100+200)/2, (300+400)/2
		},
		{
			name: "mono frame passes through unchanged",
			frame: heimdall.AudioFrame{
				Data:       []byte{100, 0, 200, 0},
				SampleRate: 16000,
				Channels:   1,
				Timestamp:  10 * time.Millisecond,
			},
			wantCh:   1,
			wantData: nil, // unchanged
		},
		{
			name: "zero channels passes through unchanged",
			frame: heimdall.AudioFrame{
				Data:       []byte{50, 0},
				SampleRate: 16000,
				Channels:   0,
				Timestamp:  0,
			},
			wantCh:   0,
			wantData: nil, // unchanged
		},
		{
			name: "overflow prevention with int32 arithmetic",
			frame: heimdall.AudioFrame{
				// L=32000, R=32000 — naive int16 addition would overflow
				Data:       []byte{0, 125, 0, 125, 0, 131, 0, 131}, // int16 LE: 32000, 32000, -32000, -32000
				SampleRate: 16000,
				Channels:   2,
				Timestamp:  0,
			},
			wantCh:   1,
			wantData: []int16{32000, -32000}, // (32000+32000)/2=32000, (-32000+-32000)/2=-32000
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := downmixStereoToMono(tc.frame)

			if result.Channels != tc.wantCh {
				t.Errorf("Channels = %d, want %d", result.Channels, tc.wantCh)
			}
			if result.SampleRate != tc.frame.SampleRate {
				t.Errorf("SampleRate = %d, want %d", result.SampleRate, tc.frame.SampleRate)
			}
			if result.Timestamp != tc.frame.Timestamp {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, tc.frame.Timestamp)
			}

			if tc.wantData == nil {
				// Should be unchanged.
				if len(result.Data) != len(tc.frame.Data) {
					t.Errorf("Data length = %d, want %d (unchanged)", len(result.Data), len(tc.frame.Data))
				}
			} else {
				// Parse result data back to int16 and compare.
				// Import mixer indirectly via the function under test.
				gotSamples := bytesToInt16ForTest(result.Data)
				if len(gotSamples) != len(tc.wantData) {
					t.Fatalf("got %d samples, want %d", len(gotSamples), len(tc.wantData))
				}
				for i, want := range tc.wantData {
					if gotSamples[i] != want {
						t.Errorf("sample[%d] = %d, want %d", i, gotSamples[i], want)
					}
				}
			}
		})
	}
}

// bytesToInt16ForTest is a test helper that converts little-endian bytes to int16 samples.
func bytesToInt16ForTest(data []byte) []int16 {
	n := len(data) / 2
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		samples[i] = int16(data[2*i]) | int16(data[2*i+1])<<8
	}
	return samples
}

func TestSilentSource(t *testing.T) {
	src := newSilentSource(48000, 2)

	if err := src.Start(context.Background()); err != nil {
		t.Fatalf("silent source Start failed: %v", err)
	}

	if src.SampleRate() != 48000 {
		t.Errorf("expected sample rate 48000, got %d", src.SampleRate())
	}

	if src.Channels() != 2 {
		t.Errorf("expected 2 channels, got %d", src.Channels())
	}

	// Stream channel should be closed immediately (no frames).
	select {
	case _, ok := <-src.Stream():
		if ok {
			t.Error("expected closed channel from silent source")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("silent source stream should return immediately")
	}

	if err := src.Stop(); err != nil {
		t.Fatalf("silent source Stop failed: %v", err)
	}
}
