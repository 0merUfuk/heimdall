package mixer

import (
	"sync"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// RingBuffer is a thread-safe circular buffer for AudioFrames that sits between
// the mixer output and the consumer (Deepgram WebSocket). It absorbs timing
// differences between the producer (mixer) and consumer (network).
//
// When the buffer is full, Write drops the oldest frame (advances head) rather
// than blocking the producer goroutine. This is critical for audio pipeline
// safety — the mixer must never be blocked by a slow consumer (V-005).
//
// Read blocks until a frame is available or the buffer is closed.
type RingBuffer struct {
	frames   []heimdall.AudioFrame
	head     int // write position (next slot to write)
	tail     int // read position (next slot to read)
	size     int // buffer capacity in frames
	count    int // current number of frames in buffer
	mu       sync.Mutex
	notEmpty *sync.Cond
	closed   bool
	dropped  int64 // total number of dropped frames (for diagnostics)
}

// NewRingBuffer creates a ring buffer sized to hold the specified duration of audio.
//
// The capacity is calculated based on 20ms frames at the given sample rate and channel count.
// For example, 30 seconds of 16kHz stereo 16-bit audio at 20ms per frame = 1500 frames.
//
// Parameters:
//   - durationSeconds: how many seconds of audio the buffer should hold
//   - sampleRate: audio sample rate in Hz (e.g., 16000)
//   - channels: number of audio channels (1 or 2)
func NewRingBuffer(durationSeconds int, sampleRate int, channels int) *RingBuffer {
	// Each frame is 20ms of audio.
	framesPerSecond := 50 // 1000ms / 20ms = 50 frames per second
	capacity := durationSeconds * framesPerSecond
	if capacity < 1 {
		capacity = 1
	}
	rb := &RingBuffer{
		frames: make([]heimdall.AudioFrame, capacity),
		size:   capacity,
	}
	rb.notEmpty = sync.NewCond(&rb.mu)
	return rb
}

// Write adds a frame to the buffer. If the buffer is full, the oldest frame is
// dropped to make room. Returns true if the frame was written without dropping,
// false if an older frame was dropped to make room.
//
// Write never blocks the caller. This is essential for audio pipeline safety —
// the producer (mixer) must always be able to write without waiting.
func (rb *RingBuffer) Write(frame heimdall.AudioFrame) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return false
	}

	dropped := false
	if rb.count == rb.size {
		// Buffer full — drop oldest frame (advance tail).
		rb.tail = (rb.tail + 1) % rb.size
		rb.count--
		rb.dropped++
		dropped = true
	}

	rb.frames[rb.head] = frame
	rb.head = (rb.head + 1) % rb.size
	rb.count++

	// Signal any blocked reader.
	rb.notEmpty.Signal()

	return !dropped
}

// Read returns the next frame from the buffer. Blocks if the buffer is empty
// until a frame is available or Close is called.
//
// Returns (frame, true) on success, or (zero, false) if the buffer is closed
// and empty.
func (rb *RingBuffer) Read() (heimdall.AudioFrame, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for rb.count == 0 && !rb.closed {
		rb.notEmpty.Wait()
	}

	if rb.count == 0 && rb.closed {
		return heimdall.AudioFrame{}, false
	}

	frame := rb.frames[rb.tail]
	rb.frames[rb.tail] = heimdall.AudioFrame{} // help GC
	rb.tail = (rb.tail + 1) % rb.size
	rb.count--

	return frame, true
}

// Len returns the current number of frames in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Dropped returns the total number of frames that were dropped due to buffer overflow.
func (rb *RingBuffer) Dropped() int64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.dropped
}

// Cap returns the buffer capacity in frames.
func (rb *RingBuffer) Cap() int {
	return rb.size
}

// Close marks the buffer as closed. Any blocked Read calls will return.
// After Close, Write becomes a no-op. Remaining frames can still be read.
func (rb *RingBuffer) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.notEmpty.Broadcast()
}
