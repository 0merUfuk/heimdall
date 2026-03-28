// Package audio defines the AudioSource interface for audio capture sources.
// Implementations include microphone capture (malgo) and system audio capture
// (Swift subprocess via Core Audio Taps).
package audio

import (
	"context"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// AudioSource defines the interface for audio capture sources.
// Implementations include microphone capture (malgo) and system audio capture (Swift subprocess).
type AudioSource interface {
	// Start begins audio capture. Blocks until the source is ready to stream.
	// Returns an error if the audio device is unavailable or permissions are denied.
	Start(ctx context.Context) error

	// Stream returns a read-only channel of AudioFrames.
	// The channel is closed when Stop() is called or the context is cancelled.
	// Implementations must never block on sends -- drop frames if the consumer is slow.
	Stream() <-chan heimdall.AudioFrame

	// Stop gracefully stops audio capture and closes the Stream channel.
	// Safe to call multiple times (idempotent).
	Stop() error

	// SampleRate returns the capture sample rate in Hz (e.g., 16000, 48000).
	SampleRate() int

	// Channels returns the number of audio channels (1=mono, 2=stereo).
	Channels() int
}
