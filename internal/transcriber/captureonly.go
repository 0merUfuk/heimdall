package transcriber

import (
	"context"
	"sync"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Compile-time assertion: CaptureOnlyTranscriber must satisfy Transcriber.
var _ Transcriber = (*CaptureOnlyTranscriber)(nil)

// CaptureOnlyTranscriber is the Stage 3 stand-in for `heimdall record
// --transcriber whisper`: it transcribes nothing live and needs no network
// or API key. The session still captures and mixes audio, which the record
// command saves as a WAV and transcribes locally with whisper.cpp after the
// meeting ends -- the batch design ID-007 chose for Whisper, now reachable
// from `record` so a meeting can be captured fully offline (ID-014).
type CaptureOnlyTranscriber struct {
	segCh chan heimdall.Segment
	once  sync.Once
}

// NewCaptureOnly creates a CaptureOnlyTranscriber.
func NewCaptureOnly() *CaptureOnlyTranscriber {
	return &CaptureOnlyTranscriber{segCh: make(chan heimdall.Segment)}
}

// Connect is a no-op: there is no provider to connect to.
func (c *CaptureOnlyTranscriber) Connect(_ context.Context, _ heimdall.TranscribeOpts) error {
	return nil
}

// Send discards the frame. The audio reaches disk through the session's
// OnAudioFrame tap, not through the transcriber.
func (c *CaptureOnlyTranscriber) Send(_ heimdall.AudioFrame) error {
	return nil
}

// Receive returns a channel that never delivers a segment and is closed by
// Close, so the session's accumulator exits cleanly on shutdown.
func (c *CaptureOnlyTranscriber) Receive() <-chan heimdall.Segment {
	return c.segCh
}

// Close closes the segment channel. Idempotent.
func (c *CaptureOnlyTranscriber) Close() error {
	c.once.Do(func() { close(c.segCh) })
	return nil
}
