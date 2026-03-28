package audio

import (
	"context"
	"testing"

	heimdall "github.com/0merUfuk/heimdall/internal/heimdall"
)

// mockAudioSource is a minimal implementation of AudioSource used to verify the
// interface is implementable and that its contract can be exercised in tests.
type mockAudioSource struct {
	sampleRate int
	channels   int
	frameCh    chan heimdall.AudioFrame
	stopped    bool
}

func newMockAudioSource(sampleRate, channels int) *mockAudioSource {
	return &mockAudioSource{
		sampleRate: sampleRate,
		channels:   channels,
		frameCh:    make(chan heimdall.AudioFrame, 16),
	}
}

func (m *mockAudioSource) Start(ctx context.Context) error {
	m.stopped = false
	return nil
}

func (m *mockAudioSource) Stream() <-chan heimdall.AudioFrame {
	return m.frameCh
}

func (m *mockAudioSource) Stop() error {
	if !m.stopped {
		m.stopped = true
		close(m.frameCh)
	}
	return nil
}

func (m *mockAudioSource) SampleRate() int {
	return m.sampleRate
}

func (m *mockAudioSource) Channels() int {
	return m.channels
}

// Compile-time assertion: mockAudioSource must satisfy AudioSource.
var _ AudioSource = (*mockAudioSource)(nil)

// TestAudioSource_InterfaceCompliance verifies that mockAudioSource satisfies
// the AudioSource interface and that all methods are callable.
func TestAudioSource_InterfaceCompliance(t *testing.T) {
	var src AudioSource = newMockAudioSource(16000, 1)

	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	ch := src.Stream()
	if ch == nil {
		t.Fatal("Stream: returned nil channel")
	}

	if got := src.SampleRate(); got != 16000 {
		t.Errorf("SampleRate: got %d, want 16000", got)
	}
	if got := src.Channels(); got != 1 {
		t.Errorf("Channels: got %d, want 1", got)
	}

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

// TestAudioSource_StreamIsReadOnly verifies that Stream() returns a receive-only channel.
func TestAudioSource_StreamIsReadOnly(t *testing.T) {
	src := newMockAudioSource(16000, 1)
	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	ch := src.Stream()

	// The returned channel must be usable as a read-only channel.
	// This is enforced at compile time by the interface signature (<-chan AudioFrame).
	var roCh <-chan heimdall.AudioFrame = ch
	if roCh == nil {
		t.Fatal("stream channel should not be nil")
	}

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

// TestAudioSource_StopIdempotent verifies that Stop() is safe to call multiple times.
func TestAudioSource_StopIdempotent(t *testing.T) {
	src := newMockAudioSource(16000, 1)
	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	if err := src.Stop(); err != nil {
		t.Fatalf("first Stop: unexpected error: %v", err)
	}
	// Second call must not panic or return an error.
	if err := src.Stop(); err != nil {
		t.Fatalf("second Stop (idempotent): unexpected error: %v", err)
	}
}

// TestAudioSource_StreamDeliversFrames verifies that frames sent to the mock's
// internal channel are readable from the Stream() channel.
func TestAudioSource_StreamDeliversFrames(t *testing.T) {
	src := newMockAudioSource(16000, 1)
	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	want := heimdall.AudioFrame{
		Data:       []byte{0x01, 0x02, 0x03, 0x04},
		SampleRate: 16000,
		Channels:   1,
	}
	src.frameCh <- want

	ch := src.Stream()
	got, ok := <-ch
	if !ok {
		t.Fatal("expected to receive a frame, channel was closed")
	}
	if len(got.Data) != len(want.Data) {
		t.Errorf("frame Data length: got %d, want %d", len(got.Data), len(want.Data))
	}
	if got.SampleRate != want.SampleRate {
		t.Errorf("frame SampleRate: got %d, want %d", got.SampleRate, want.SampleRate)
	}

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

// TestAudioSource_ChannelClosedAfterStop verifies that the Stream channel is
// closed when Stop() is called, signalling consumers to terminate.
func TestAudioSource_ChannelClosedAfterStop(t *testing.T) {
	src := newMockAudioSource(16000, 1)
	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	ch := src.Stream()
	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}

	// After Stop, the channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Stop, but received a value")
	}
}

// TestAudioSource_SampleRateAndChannels validates multiple common configurations.
func TestAudioSource_SampleRateAndChannels(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		channels   int
	}{
		{"microphone 16kHz mono", 16000, 1},
		{"system audio 48kHz stereo", 48000, 2},
		{"mixed output 16kHz stereo", 16000, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src AudioSource = newMockAudioSource(tt.sampleRate, tt.channels)
			if got := src.SampleRate(); got != tt.sampleRate {
				t.Errorf("SampleRate: got %d, want %d", got, tt.sampleRate)
			}
			if got := src.Channels(); got != tt.channels {
				t.Errorf("Channels: got %d, want %d", got, tt.channels)
			}
		})
	}
}
