// Package transcriber defines the Transcriber interface for speech-to-text providers.
// The primary implementation is DeepgramTranscriber (WebSocket streaming with
// speaker diarization via Deepgram Nova-3).
package transcriber

import (
	"context"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Transcriber defines the interface for speech-to-text providers.
// The primary implementation is DeepgramTranscriber (WebSocket streaming).
type Transcriber interface {
	// Connect establishes a connection to the STT provider.
	// Must be called before Send. Returns an error if connection fails.
	Connect(ctx context.Context, opts heimdall.TranscribeOpts) error

	// Send streams an audio frame to the transcription provider.
	// Returns an error if the connection is closed or the frame cannot be sent.
	Send(frame heimdall.AudioFrame) error

	// Receive returns a read-only channel of transcribed Segments.
	// Both interim (IsFinal=false) and final (IsFinal=true) results are delivered.
	// The channel is closed when Close() is called.
	Receive() <-chan heimdall.Segment

	// Close gracefully closes the connection to the STT provider.
	// Waits for any pending final transcription results before closing.
	Close() error
}
