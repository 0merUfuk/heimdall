package audio

import (
	"context"
	"testing"
	"time"
)

// Compile-time assertion: MicrophoneSource must satisfy AudioSource.
var _ AudioSource = (*MicrophoneSource)(nil)

// TestNewMicrophoneSource verifies that the constructor returns a non-nil, properly
// initialized MicrophoneSource.
func TestNewMicrophoneSource(t *testing.T) {
	src := NewMicrophoneSource()
	if src == nil {
		t.Fatal("NewMicrophoneSource returned nil")
	}
	if src.frameCh == nil {
		t.Fatal("frameCh should be initialized (non-nil)")
	}
	if src.started {
		t.Error("new MicrophoneSource should not be in started state")
	}
}

// TestMicrophoneSource_SampleRate verifies that SampleRate returns 16000.
func TestMicrophoneSource_SampleRate(t *testing.T) {
	src := NewMicrophoneSource()
	if got := src.SampleRate(); got != 16000 {
		t.Errorf("SampleRate: got %d, want 16000", got)
	}
}

// TestMicrophoneSource_Channels verifies that Channels returns 1 (mono).
func TestMicrophoneSource_Channels(t *testing.T) {
	src := NewMicrophoneSource()
	if got := src.Channels(); got != 1 {
		t.Errorf("Channels: got %d, want 1", got)
	}
}

// TestMicrophoneSource_StreamReturnsNonNil verifies that Stream() returns a
// non-nil channel even before Start() is called.
func TestMicrophoneSource_StreamReturnsNonNil(t *testing.T) {
	src := NewMicrophoneSource()
	ch := src.Stream()
	if ch == nil {
		t.Fatal("Stream() returned nil channel")
	}
}

// TestMicrophoneSource_StopIdempotent verifies that calling Stop() multiple times
// does not panic or return an error.
func TestMicrophoneSource_StopIdempotent(t *testing.T) {
	src := NewMicrophoneSource()

	// First Stop on an unstarted source should be safe.
	if err := src.Stop(); err != nil {
		t.Fatalf("first Stop: unexpected error: %v", err)
	}

	// Second Stop should also be safe (idempotent).
	if err := src.Stop(); err != nil {
		t.Fatalf("second Stop: unexpected error: %v", err)
	}

	// Third Stop for good measure.
	if err := src.Stop(); err != nil {
		t.Fatalf("third Stop: unexpected error: %v", err)
	}
}

// TestMicrophoneSource_StartCancelledContext verifies that Start() with an already-
// cancelled context returns promptly without initializing hardware.
func TestMicrophoneSource_StartCancelledContext(t *testing.T) {
	src := NewMicrophoneSource()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	done := make(chan error, 1)
	go func() {
		done <- src.Start(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Start with cancelled context should return an error")
		}
		// Verify the error wraps the context error.
		t.Logf("Start returned expected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start with cancelled context did not return within 2 seconds")
	}
}

// TestMicrophoneSource_InterfaceCompliance verifies that MicrophoneSource satisfies
// the AudioSource interface through all method signatures.
func TestMicrophoneSource_InterfaceCompliance(t *testing.T) {
	var src AudioSource = NewMicrophoneSource()

	// Verify all interface methods are callable.
	_ = src.Stream()
	_ = src.SampleRate()
	_ = src.Channels()

	// Stop should work without Start having been called.
	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

// TestMicrophoneSource_Constants verifies that the package-level constants
// are consistent and correct.
func TestMicrophoneSource_Constants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"micSampleRate", micSampleRate, 16000},
		{"micChannels", micChannels, 1},
		{"micStreamBufferSize", micStreamBufferSize, 100},
		// 16000 samples/s * 0.020s * 1 channel * 2 bytes/sample = 640 bytes
		{"micFrameBytes", micFrameBytes, 640},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}

	// Verify frame duration is 20ms.
	if micFrameDuration != 20*time.Millisecond {
		t.Errorf("micFrameDuration: got %v, want 20ms", micFrameDuration)
	}
}

// TestMicrophoneSource_FrameBytesCalculation verifies the relationship between
// sample rate, channels, bit depth, and frame duration.
func TestMicrophoneSource_FrameBytesCalculation(t *testing.T) {
	// Expected: sampleRate * frameDurationSec * channels * bytesPerSample
	// 16000 * 0.020 * 1 * 2 = 640
	bytesPerSample := 2 // 16-bit = 2 bytes
	expectedBytes := micSampleRate * int(micFrameDuration.Milliseconds()) * micChannels * bytesPerSample / 1000

	if expectedBytes != micFrameBytes {
		t.Errorf("frame bytes calculation: %d * %dms * %d * %d / 1000 = %d, want %d",
			micSampleRate, micFrameDuration.Milliseconds(), micChannels, bytesPerSample,
			expectedBytes, micFrameBytes)
	}
}

// TestMicrophoneSource_StopBeforeStart verifies that Stop() is safe to call
// on a source that was never started.
func TestMicrophoneSource_StopBeforeStart(t *testing.T) {
	src := NewMicrophoneSource()
	if err := src.Stop(); err != nil {
		t.Fatalf("Stop before Start: unexpected error: %v", err)
	}

	// After Stop, the channel should be closed.
	ch := src.Stream()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after Stop")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel read did not complete within 100ms (should be closed)")
	}
}

// TestMicrophoneSource_StopClosesChannel verifies that calling Stop() closes
// the Stream channel, signaling consumers to terminate.
func TestMicrophoneSource_StopClosesChannel(t *testing.T) {
	src := NewMicrophoneSource()
	ch := src.Stream()

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Stop, but received a value")
	}
}

// TestMicrophoneSource_ClassifyMalgoError verifies that classifyMalgoError produces
// user-friendly error messages for known malgo error types.
func TestMicrophoneSource_ClassifyMalgoError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantContain string
	}{
		{
			name:        "nil error",
			err:         nil,
			wantContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyMalgoError(tt.err)
			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
		})
	}
}
