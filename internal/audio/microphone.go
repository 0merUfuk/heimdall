// Package audio provides audio capture source implementations.
// This file implements MicrophoneSource using the malgo library (miniaudio Go bindings)
// for capturing microphone audio at 16kHz, 16-bit signed int, mono PCM.
package audio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gen2brain/malgo"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

const (
	// micSampleRate is the capture sample rate in Hz.
	micSampleRate = 16000

	// micChannels is the number of capture channels (mono).
	micChannels = 1

	// micFrameDuration is the target duration per audio frame (~20ms).
	// At 16kHz, 16-bit mono: 16000 * 0.02 * 2 bytes = 640 bytes per frame.
	micFrameDuration = 20 * time.Millisecond

	// micFrameBytes is the expected byte count per frame chunk.
	// 16000 samples/sec * 0.020 sec * 1 channel * 2 bytes/sample = 640 bytes.
	micFrameBytes = 640

	// micStreamBufferSize is the channel buffer capacity in frames.
	// At 20ms per frame, 100 frames = 2 seconds of buffering.
	micStreamBufferSize = 100
)

// MicrophoneSource captures audio from the system microphone using malgo (miniaudio).
// It implements the AudioSource interface.
type MicrophoneSource struct {
	mu        sync.Mutex
	ctx       *malgo.AllocatedContext
	device    *malgo.Device
	frameCh   chan heimdall.AudioFrame
	cancelFn  context.CancelFunc
	started   bool
	stopped   bool
	closeOnce sync.Once

	// startTime tracks when recording began for timestamp calculation.
	startTime time.Time

	// bytesRead tracks total bytes received for timestamp calculation.
	bytesRead int64
}

// Compile-time assertion: MicrophoneSource must satisfy AudioSource.
var _ AudioSource = (*MicrophoneSource)(nil)

// NewMicrophoneSource creates a new MicrophoneSource.
// Call Start() to begin capturing audio.
func NewMicrophoneSource() *MicrophoneSource {
	return &MicrophoneSource{
		frameCh: make(chan heimdall.AudioFrame, micStreamBufferSize),
	}
}

// Start begins microphone audio capture. It initializes the malgo context and device,
// then starts streaming audio frames to the Stream() channel.
//
// Returns an error if:
//   - No microphone device is available
//   - Microphone permission is denied (macOS: System Settings > Privacy & Security > Microphone)
//   - The context is already cancelled
func (m *MicrophoneSource) Start(ctx context.Context) error {
	// Check if context is already cancelled before doing any work.
	select {
	case <-ctx.Done():
		return fmt.Errorf("microphone start: %w", ctx.Err())
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return errors.New("microphone start: already started")
	}

	// Initialize malgo context.
	malgoCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("microphone init context: %w", classifyMalgoError(err))
	}

	// Configure capture device: 16kHz, 16-bit signed int, mono.
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = micChannels
	deviceConfig.SampleRate = micSampleRate
	deviceConfig.PeriodSizeInMilliseconds = uint32(micFrameDuration.Milliseconds())

	// Create a derived context for the capture goroutine.
	captureCtx, cancelFn := context.WithCancel(ctx)

	// Data callback: called by malgo from its audio thread with captured PCM data.
	// The second parameter (pSample/pInputSamples) contains the captured audio.
	onData := func(pOutputSample, pInputSamples []byte, framecount uint32) {
		if len(pInputSamples) == 0 {
			return
		}

		// Copy the data since the underlying buffer is owned by malgo and may be reused.
		data := make([]byte, len(pInputSamples))
		copy(data, pInputSamples)

		// Calculate timestamp from total bytes read.
		// bytesRead / (sampleRate * channels * bytesPerSample) = seconds elapsed
		m.mu.Lock()
		bytesRead := m.bytesRead
		m.bytesRead += int64(len(data))
		m.mu.Unlock()

		bytesPerSecond := int64(micSampleRate * micChannels * 2) // 2 bytes per sample (16-bit)
		timestamp := time.Duration(bytesRead * int64(time.Second) / bytesPerSecond)

		frame := heimdall.AudioFrame{
			Data:       data,
			SampleRate: micSampleRate,
			Channels:   micChannels,
			Timestamp:  timestamp,
		}

		// Non-blocking send per audio-safety rules.
		// Drop frame if consumer is slow -- never block the audio callback.
		select {
		case m.frameCh <- frame:
		case <-captureCtx.Done():
			return
		default:
			// Frame dropped -- consumer is too slow. Log at debug level.
			log.Printf("microphone: frame dropped (consumer too slow), timestamp=%v", timestamp)
		}
	}

	callbacks := malgo.DeviceCallbacks{
		Data: onData,
	}

	// Initialize the device.
	device, err := malgo.InitDevice(malgoCtx.Context, deviceConfig, callbacks)
	if err != nil {
		cancelFn()
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
		return fmt.Errorf("microphone init device: %w", classifyMalgoError(err))
	}

	// Start the device.
	if err := device.Start(); err != nil {
		device.Uninit()
		cancelFn()
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
		return fmt.Errorf("microphone start device: %w", classifyMalgoError(err))
	}

	m.ctx = malgoCtx
	m.device = device
	m.cancelFn = cancelFn
	m.started = true
	m.startTime = time.Now()
	m.bytesRead = 0

	// Monitor context cancellation to auto-stop.
	go m.watchContext(captureCtx)

	return nil
}

// watchContext monitors the parent context for cancellation and triggers Stop().
func (m *MicrophoneSource) watchContext(ctx context.Context) {
	<-ctx.Done()
	// Context cancelled -- stop the device. Stop() is idempotent so this is safe
	// even if Stop() was already called directly.
	_ = m.Stop()
}

// Stream returns a read-only channel of AudioFrames.
// The channel is closed when Stop() is called or the context is cancelled.
// Frames are ~20ms chunks of 16kHz, 16-bit signed int, mono PCM (640 bytes each).
func (m *MicrophoneSource) Stream() <-chan heimdall.AudioFrame {
	return m.frameCh
}

// Stop gracefully stops audio capture and closes the Stream channel.
// Safe to call multiple times (idempotent).
//
// IMPORTANT: device.Uninit() joins the audio thread before returning. The onData
// callback running on that thread acquires m.mu for bytesRead tracking. Holding
// m.mu across Uninit() would deadlock. Therefore we:
//  1. Acquire lock, set stopped=true, save device/ctx references, nil them out, cancel context.
//  2. Release lock.
//  3. Call device.Uninit() and malgoCtx.Free() OUTSIDE the lock.
func (m *MicrophoneSource) Stop() error {
	m.mu.Lock()

	if m.stopped {
		m.mu.Unlock()
		return nil
	}

	// Cancel the context to signal the watchContext goroutine and data callback.
	if m.cancelFn != nil {
		m.cancelFn()
		m.cancelFn = nil
	}

	// Save references and nil them under the lock so no other call to Stop() will
	// attempt to uninit the same device/context concurrently.
	device := m.device
	malgoCtx := m.ctx
	m.device = nil
	m.ctx = nil
	m.started = false
	m.stopped = true
	m.mu.Unlock()

	// Uninitialize device OUTSIDE the lock. Uninit() joins the audio thread, which
	// may be blocked on m.mu inside onData -- since we already released the lock,
	// the audio thread can complete and Uninit() returns without deadlock.
	if device != nil {
		device.Uninit()
	}

	// Free the malgo context OUTSIDE the lock.
	if malgoCtx != nil {
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
	}

	// Close the channel exactly once, draining it first to prevent panics.
	// sync.Once ensures this is safe even if Stop() is called concurrently.
	m.closeOnce.Do(func() {
		// After device.Uninit(), no more callbacks will fire, so the channel is safe to drain.
		for {
			select {
			case <-m.frameCh:
				// Discard remaining frames.
			default:
				close(m.frameCh)
				return
			}
		}
	})

	return nil
}

// SampleRate returns the capture sample rate in Hz (16000).
func (m *MicrophoneSource) SampleRate() int {
	return micSampleRate
}

// Channels returns the number of audio channels (1 = mono).
func (m *MicrophoneSource) Channels() int {
	return micChannels
}

// classifyMalgoError inspects a malgo error and returns a user-friendly wrapped error
// with actionable guidance for common failure modes.
func classifyMalgoError(err error) error {
	if err == nil {
		return nil
	}

	var malgoErr malgo.Result

	if errors.As(err, &malgoErr) {
		switch malgoErr {
		case malgo.ErrAccessDenied:
			return fmt.Errorf(
				"microphone permission denied: grant microphone access in "+
					"System Settings > Privacy & Security > Microphone: %w", err)
		case malgo.ErrNoDevice:
			return fmt.Errorf(
				"no microphone found: connect a microphone and try again: %w", err)
		case malgo.ErrNoBackend:
			return fmt.Errorf(
				"no audio backend available: ensure audio system is functioning: %w", err)
		case malgo.ErrFormatNotSupported:
			return fmt.Errorf(
				"audio format not supported by device (16kHz/16-bit/mono): %w", err)
		}
	}

	return err
}
