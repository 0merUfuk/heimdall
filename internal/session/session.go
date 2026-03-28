// Package session implements the MeetingSession orchestrator that wires pipeline
// stages 1-4 (Capture -> Mix -> Transcribe -> Accumulate) into a single
// coordinated session.
//
// The session manages the lifecycle of all pipeline components and provides
// callbacks for live transcript display.
package session

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/audio"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/mixer"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

// MeetingSession orchestrates pipeline stages 1-4 for a recording session.
// It manages the full lifecycle: start audio sources, mix, stream to transcriber,
// accumulate segments, and graceful shutdown.
type MeetingSession struct {
	title       string
	system      audio.AudioSource
	mic         audio.AudioSource
	mixer       *mixer.Mixer
	transcriber transcriber.Transcriber

	segments  []heimdall.Segment
	mu        sync.Mutex
	startTime time.Time

	// onSegment is called for each new segment received (for live display).
	onSegment func(heimdall.Segment)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// systemAvailable tracks whether system audio was successfully started.
	systemAvailable bool
}

// NewMeetingSession creates a new session orchestrator.
// system may be nil if system audio is not available (mic-only mode).
// t must be a valid Transcriber instance.
func NewMeetingSession(title string, system audio.AudioSource, mic audio.AudioSource, t transcriber.Transcriber) *MeetingSession {
	return &MeetingSession{
		title:       title,
		system:      system,
		mic:         mic,
		transcriber: t,
	}
}

// OnSegment registers a callback that is invoked for each new segment received
// from the transcriber. This is used for live terminal display.
// Must be called before Start.
func (s *MeetingSession) OnSegment(fn func(heimdall.Segment)) {
	s.onSegment = fn
}

// Start initializes and starts the full pipeline:
//  1. Start audio sources (system + mic)
//  2. Create and start the mixer
//  3. Connect the transcriber
//  4. Start the audio-to-transcriber data flow goroutine
//  5. Start the segment accumulation goroutine
//
// If system audio fails to start, the session continues in mic-only mode.
// Returns an error only if critical components (mic, transcriber) fail.
func (s *MeetingSession) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.startTime = time.Now()
	s.systemAvailable = false

	// Stage 1: Start audio sources.
	// System audio is optional -- if it fails, continue with mic only.
	if s.system != nil {
		if err := s.system.Start(s.ctx); err != nil {
			log.Printf("warning: system audio unavailable: %v", err)
			log.Printf("continuing with microphone only")
			s.system = nil
		} else {
			s.systemAvailable = true
		}
	}

	// Microphone is required.
	if err := s.mic.Start(s.ctx); err != nil {
		// If system was started, stop it before returning.
		if s.system != nil {
			_ = s.system.Stop()
		}
		return fmt.Errorf("microphone start failed: %w", err)
	}

	// Stage 2: Create and start the mixer.
	// If system audio is not available, use a silent source placeholder.
	var systemSource audio.AudioSource
	if s.system != nil {
		systemSource = s.system
	} else {
		systemSource = newSilentSource(48000, 2)
	}

	s.mixer = mixer.NewMixer(systemSource, s.mic)
	if err := s.mixer.Start(s.ctx); err != nil {
		_ = s.mic.Stop()
		if s.system != nil {
			_ = s.system.Stop()
		}
		return fmt.Errorf("mixer start failed: %w", err)
	}

	// Stage 3: Connect the transcriber.
	opts := heimdall.TranscribeOpts{
		Model:       "nova-3",
		Language:    "en",
		SampleRate:  16000,
		Channels:    2,
		Encoding:    "linear16",
		Diarize:     true,
		Punctuate:   true,
		SmartFormat: true,
	}

	if err := s.transcriber.Connect(s.ctx, opts); err != nil {
		_ = s.mixer.Stop()
		_ = s.mic.Stop()
		if s.system != nil {
			_ = s.system.Stop()
		}
		return fmt.Errorf("transcriber connect failed: %w", err)
	}

	// Start the audio-to-transcriber data flow goroutine.
	s.wg.Add(1)
	go s.audioToTranscriber()

	// Start the segment accumulation goroutine.
	s.wg.Add(1)
	go s.accumulateSegments()

	return nil
}

// Stop gracefully shuts down the pipeline in reverse order:
//  1. Cancel the context (signals all goroutines)
//  2. Close the transcriber (sends CloseStream, waits for final results)
//  3. Stop the mixer
//  4. Stop audio sources
//
// Returns the first error encountered during shutdown.
func (s *MeetingSession) Stop() error {
	// Cancel context to signal goroutines to stop sending new data.
	if s.cancel != nil {
		s.cancel()
	}

	// Close transcriber first -- sends CloseStream and waits for final results.
	// This must happen before stopping the mixer so any in-flight data is processed.
	var firstErr error
	if s.transcriber != nil {
		if err := s.transcriber.Close(); err != nil {
			firstErr = fmt.Errorf("transcriber close: %w", err)
		}
	}

	// Stop the mixer.
	if s.mixer != nil {
		if err := s.mixer.Stop(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("mixer stop: %w", err)
		}
	}

	// Stop audio sources.
	if s.mic != nil {
		if err := s.mic.Stop(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("mic stop: %w", err)
		}
	}
	if s.system != nil {
		if err := s.system.Stop(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("system audio stop: %w", err)
		}
	}

	// Wait for all goroutines to finish.
	s.wg.Wait()

	return firstErr
}

// Segments returns a copy of all accumulated segments.
func (s *MeetingSession) Segments() []heimdall.Segment {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]heimdall.Segment, len(s.segments))
	copy(result, s.segments)
	return result
}

// Duration returns the elapsed recording time.
func (s *MeetingSession) Duration() time.Duration {
	return time.Since(s.startTime)
}

// Title returns the session title.
func (s *MeetingSession) Title() string {
	return s.title
}

// SystemAvailable returns whether system audio capture is active.
func (s *MeetingSession) SystemAvailable() bool {
	return s.systemAvailable
}

// SpeakerCount returns the number of unique speakers seen so far.
func (s *MeetingSession) SpeakerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	speakers := make(map[int]struct{})
	for _, seg := range s.segments {
		if seg.IsFinal {
			speakers[seg.Speaker] = struct{}{}
		}
	}
	return len(speakers)
}

// audioToTranscriber reads frames from the mixer output and sends them to
// the transcriber. Exits when the context is cancelled.
func (s *MeetingSession) audioToTranscriber() {
	defer s.wg.Done()

	stream := s.mixer.Stream()
	for {
		select {
		case <-s.ctx.Done():
			return
		case frame, ok := <-stream:
			if !ok {
				return
			}
			if err := s.transcriber.Send(frame); err != nil {
				if s.ctx.Err() != nil {
					return // Context cancelled, clean exit.
				}
				// Log but don't abort -- the transcriber handles reconnection.
				log.Printf("session: transcriber send error: %v", err)
			}
		}
	}
}

// accumulateSegments reads segments from the transcriber and stores them.
// Calls the OnSegment callback for each new segment.
func (s *MeetingSession) accumulateSegments() {
	defer s.wg.Done()

	segCh := s.transcriber.Receive()
	for {
		select {
		case seg, ok := <-segCh:
			if !ok {
				return
			}

			// Accumulate final segments only.
			if seg.IsFinal {
				s.mu.Lock()
				s.segments = append(s.segments, seg)
				s.mu.Unlock()
			}

			// Notify the callback for all segments (including interim for live display).
			if s.onSegment != nil {
				s.onSegment(seg)
			}

		case <-s.ctx.Done():
			// Drain remaining segments after context cancellation.
			// The transcriber may still deliver final results via Close().
			for seg := range segCh {
				if seg.IsFinal {
					s.mu.Lock()
					s.segments = append(s.segments, seg)
					s.mu.Unlock()
				}
				if s.onSegment != nil {
					s.onSegment(seg)
				}
			}
			return
		}
	}
}

// silentSource is a no-op AudioSource that produces no frames.
// Used as a placeholder when system audio is unavailable.
type silentSource struct {
	sampleRate int
	channels   int
	frameCh    chan heimdall.AudioFrame
}

func newSilentSource(sampleRate, channels int) *silentSource {
	ch := make(chan heimdall.AudioFrame)
	close(ch) // immediately closed -- produces no frames
	return &silentSource{
		sampleRate: sampleRate,
		channels:   channels,
		frameCh:    ch,
	}
}

func (s *silentSource) Start(_ context.Context) error          { return nil }
func (s *silentSource) Stream() <-chan heimdall.AudioFrame      { return s.frameCh }
func (s *silentSource) Stop() error                             { return nil }
func (s *silentSource) SampleRate() int                         { return s.sampleRate }
func (s *silentSource) Channels() int                           { return s.channels }
