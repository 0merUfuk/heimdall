package mixer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/audio"
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

const (
	// frameDuration is the duration of each output frame.
	frameDuration = 20 * time.Millisecond

	// outputSampleRate is the target sample rate for the interleaved stereo output.
	outputSampleRate = 16000

	// outputChannels is the number of channels in the output (stereo).
	outputChannels = 2

	// samplesPerFrame is the number of mono samples per output frame at 16kHz/20ms.
	samplesPerFrame = 320 // 16000 * 0.020

	// systemSamplesPerFrame is the number of mono samples needed from the system
	// audio source at 48kHz for one output frame. 48000 * 0.020 = 960.
	// After stereo-to-mono: 960 stereo pairs = 1920 float32 values.
	systemSamplesPerFrame = 960

	// outputChanSize is the buffered channel capacity for output frames.
	outputChanSize = 100

	// ringBufferSeconds is the ring buffer duration between mixer and consumer.
	ringBufferSeconds = 30

	// sourceReadTimeout is how long to wait for a frame from a source before
	// treating it as silence for that cycle.
	sourceReadTimeout = 50 * time.Millisecond
)

// Mixer consumes two AudioSource streams (system audio + microphone), resamples,
// converts, and interleaves them into a stereo stream at 16kHz/16-bit for Deepgram.
//
// Pipeline Stage 2 (MIX) per PIPELINE.md. Dual-channel convention (AD-007):
//   - Left channel  = system audio (remote meeting participants)
//   - Right channel = microphone (local user)
//
// The mixer maintains a 30-second ring buffer between its output and the consumer
// to absorb network hiccups (V-005).
type Mixer struct {
	systemSource audio.AudioSource // 48kHz, 32-bit float, stereo
	micSource    audio.AudioSource // 16kHz, 16-bit int, mono
	output       chan heimdall.AudioFrame
	ringBuffer   *RingBuffer
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	started      bool
	wg           sync.WaitGroup

	// Internal sample buffers for absorbing timing differences between sources.
	sysBuf  []int16 // resampled system audio samples (mono, 16kHz, int16)
	micBuf  []int16 // mic samples (mono, 16kHz, int16)
	sysMu   sync.Mutex
	micMu   sync.Mutex

	// elapsed tracks the current output timestamp.
	elapsed time.Duration
}

// NewMixer creates a new Mixer that consumes from the given audio sources.
// systemSource should provide 48kHz/32-bit float/stereo audio.
// micSource should provide 16kHz/16-bit int/mono audio.
func NewMixer(systemSource, micSource audio.AudioSource) *Mixer {
	return &Mixer{
		systemSource: systemSource,
		micSource:    micSource,
	}
}

// Start begins the mixing process. It starts goroutines that read from both
// audio sources, resample/convert as needed, and produce interleaved stereo
// frames on the output channel.
//
// Start is not safe to call concurrently. Returns an error if already started.
func (m *Mixer) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("mixer: already started")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.output = make(chan heimdall.AudioFrame, outputChanSize)
	m.ringBuffer = NewRingBuffer(ringBufferSeconds, outputSampleRate, outputChannels)
	m.sysBuf = nil
	m.micBuf = nil
	m.elapsed = 0
	m.started = true

	// Start reader goroutines that drain audio sources into internal buffers.
	m.wg.Add(3)
	go m.readSystemAudio()
	go m.readMicAudio()
	go m.mixLoop()

	return nil
}

// Stream returns a read-only channel of interleaved stereo AudioFrames at
// 16kHz/16-bit. Each frame is 20ms (320 stereo sample pairs = 1280 bytes).
//
// The channel is closed when Stop() is called.
func (m *Mixer) Stream() <-chan heimdall.AudioFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.output
}

// Stop gracefully stops the mixer. It cancels the context, waits for goroutines
// to finish, and closes the output channel. Safe to call multiple times.
func (m *Mixer) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	m.mu.Unlock()

	// Cancel context to signal all goroutines to stop.
	m.cancel()

	// Wait for all goroutines to finish.
	m.wg.Wait()

	// Close the ring buffer so any blocked reader unblocks.
	m.ringBuffer.Close()

	// Close output channel. At this point no goroutine is writing to it.
	close(m.output)

	return nil
}

// readSystemAudio drains the system audio source and appends resampled samples
// to the internal system buffer.
func (m *Mixer) readSystemAudio() {
	defer m.wg.Done()

	stream := m.systemSource.Stream()
	monoLogged := false
	for {
		select {
		case <-m.ctx.Done():
			return
		case frame, ok := <-stream:
			if !ok {
				return
			}
			// System audio arrives as 48kHz, 32-bit float, typically stereo.
			// Convert: stereo float32 -> mono float32 -> resample 48->16 -> int16.
			// If mono (e.g., hardware returns 1ch), skip the stereo-to-mono step.
			floatSamples := BytesToFloat32(frame.Data)
			if len(floatSamples) == 0 {
				continue
			}

			var resampled []int16
			if frame.Channels >= 2 {
				resampled = Resample48to16Mono(floatSamples)
			} else {
				// Mono system audio -- skip stereo-to-mono, resample directly.
				if !monoLogged {
					log.Printf("mixer: system audio is mono (%dch), skipping stereo-to-mono conversion", frame.Channels)
					monoLogged = true
				}
				resampled = Resample48to16(floatSamples)
			}

			m.sysMu.Lock()
			m.sysBuf = append(m.sysBuf, resampled...)
			m.sysMu.Unlock()
		}
	}
}

// readMicAudio drains the microphone audio source and appends samples to the
// internal microphone buffer.
func (m *Mixer) readMicAudio() {
	defer m.wg.Done()

	stream := m.micSource.Stream()
	for {
		select {
		case <-m.ctx.Done():
			return
		case frame, ok := <-stream:
			if !ok {
				return
			}
			// Mic audio arrives as 16kHz, 16-bit int, mono — use as-is.
			samples := BytesToInt16(frame.Data)
			if len(samples) == 0 {
				continue
			}

			m.micMu.Lock()
			m.micBuf = append(m.micBuf, samples...)
			m.micMu.Unlock()
		}
	}
}

// mixLoop runs at a 20ms cycle, consuming samples from the internal buffers
// and producing interleaved stereo output frames.
func (m *Mixer) mixLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.produceMixedFrame()
		}
	}
}

// produceMixedFrame takes samplesPerFrame from each internal buffer, interleaves
// them into a stereo frame, and sends it to the output channel.
func (m *Mixer) produceMixedFrame() {
	// Extract system audio samples.
	sysSamples := m.drainSysSamples(samplesPerFrame)

	// Extract mic samples.
	micSamples := m.drainMicSamples(samplesPerFrame)

	// If both are empty, skip this frame entirely (no audio to mix).
	if len(sysSamples) == 0 && len(micSamples) == 0 {
		return
	}

	// Pad shorter slice with silence if needed.
	if len(sysSamples) == 0 {
		sysSamples = make([]int16, samplesPerFrame)
	}
	if len(micSamples) == 0 {
		micSamples = make([]int16, samplesPerFrame)
	}

	// Interleave: L=system, R=mic (AD-007).
	stereo := InterleaveInt16(sysSamples, micSamples)
	data := Int16ToBytes(stereo)

	frame := heimdall.AudioFrame{
		Data:       data,
		SampleRate: outputSampleRate,
		Channels:   outputChannels,
		Timestamp:  m.elapsed,
	}
	m.elapsed += frameDuration

	// Non-blocking send to output channel — drop frame if channel is full.
	select {
	case m.output <- frame:
	default:
		log.Printf("mixer: output channel full, frame dropped at %v", frame.Timestamp)
	}

	// Also write to ring buffer for consumer buffering (V-005).
	if !m.ringBuffer.Write(frame) {
		// Oldest frame was dropped from ring buffer — this is expected under
		// backpressure and logged at debug level only.
	}
}

// drainSysSamples extracts up to n samples from the system buffer.
// Returns fewer samples if the buffer has less than n.
func (m *Mixer) drainSysSamples(n int) []int16 {
	m.sysMu.Lock()
	defer m.sysMu.Unlock()

	if len(m.sysBuf) == 0 {
		return nil
	}

	take := n
	if take > len(m.sysBuf) {
		take = len(m.sysBuf)
	}

	samples := make([]int16, take)
	copy(samples, m.sysBuf[:take])
	m.sysBuf = m.sysBuf[take:]

	return samples
}

// drainMicSamples extracts up to n samples from the mic buffer.
// Returns fewer samples if the buffer has less than n.
func (m *Mixer) drainMicSamples(n int) []int16 {
	m.micMu.Lock()
	defer m.micMu.Unlock()

	if len(m.micBuf) == 0 {
		return nil
	}

	take := n
	if take > len(m.micBuf) {
		take = len(m.micBuf)
	}

	samples := make([]int16, take)
	copy(samples, m.micBuf[:take])
	m.micBuf = m.micBuf[take:]

	return samples
}

// RingBuffer returns the mixer's ring buffer for direct consumer access.
// The consumer (e.g., Deepgram transcriber) reads from this buffer to get
// frames with backpressure absorption.
func (m *Mixer) RingBuffer() *RingBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ringBuffer
}
