package mixer

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// --- Resample Tests ---

func TestResample48to16(t *testing.T) {
	tests := []struct {
		name      string
		input     []float32
		wantLen   int
		wantFirst int16
	}{
		{
			name:      "empty input",
			input:     nil,
			wantLen:   0,
			wantFirst: 0,
		},
		{
			name:      "3 samples yields 1",
			input:     []float32{0.5, 0.0, 0.0},
			wantLen:   1,
			wantFirst: Float32ToInt16(0.5),
		},
		{
			name:    "6 samples yields 2",
			input:   []float32{0.25, 0.1, 0.2, -0.5, 0.3, 0.4},
			wantLen: 2,
		},
		{
			name:    "960 samples yields 320 (one 20ms frame)",
			input:   make([]float32, 960),
			wantLen: 320,
		},
		{
			name:    "partial remainder truncated",
			input:   []float32{0.1, 0.2, 0.3, 0.4, 0.5},
			wantLen: 1, // 5/3 = 1 (integer division)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resample48to16(tc.input)
			if len(got) != tc.wantLen {
				t.Errorf("Resample48to16: got len %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantLen > 0 && tc.wantFirst != 0 && got[0] != tc.wantFirst {
				t.Errorf("Resample48to16: got first sample %d, want %d", got[0], tc.wantFirst)
			}
		})
	}
}

func TestFloat32ToInt16(t *testing.T) {
	tests := []struct {
		name string
		in   float32
		want int16
	}{
		{"zero", 0.0, 0},
		{"positive max", 1.0, 32767},
		{"negative max", -1.0, -32767},
		{"half positive", 0.5, 16384},
		{"half negative", -0.5, -16384},
		{"clamp above 1.0", 1.5, 32767},
		{"clamp below -1.0", -1.5, -32767},
		{"small positive", 0.001, 33},
		{"small negative", -0.001, -33},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Float32ToInt16(tc.in)
			// Allow +/- 1 for rounding differences.
			diff := int(got) - int(tc.want)
			if diff < -1 || diff > 1 {
				t.Errorf("Float32ToInt16(%f) = %d, want %d (diff=%d)", tc.in, got, tc.want, diff)
			}
		})
	}
}

func TestFloat32ToInt16_Symmetry(t *testing.T) {
	// Verify that conversion is reasonably symmetric around zero.
	pos := Float32ToInt16(0.5)
	neg := Float32ToInt16(-0.5)
	if pos != -neg {
		t.Errorf("asymmetric: Float32ToInt16(0.5)=%d, Float32ToInt16(-0.5)=%d", pos, neg)
	}
}

func TestStereoToMono(t *testing.T) {
	tests := []struct {
		name   string
		stereo []float32
		want   []float32
	}{
		{"empty", nil, nil},
		{"single sample (odd)", []float32{0.5}, nil},
		{"one pair", []float32{0.4, 0.6}, []float32{0.5}},
		{"two pairs", []float32{0.2, 0.8, -0.5, 0.5}, []float32{0.5, 0.0}},
		{"silence", []float32{0.0, 0.0, 0.0, 0.0}, []float32{0.0, 0.0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StereoToMono(tc.stereo)
			if len(got) != len(tc.want) {
				t.Fatalf("StereoToMono: got len %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if math.Abs(float64(got[i]-tc.want[i])) > 1e-6 {
					t.Errorf("StereoToMono[%d] = %f, want %f", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestInterleaveInt16(t *testing.T) {
	tests := []struct {
		name  string
		left  []int16
		right []int16
		want  []int16
	}{
		{"empty", nil, nil, nil},
		{"equal length", []int16{100, 200}, []int16{300, 400}, []int16{100, 300, 200, 400}},
		{"left longer", []int16{100, 200, 300}, []int16{400}, []int16{100, 400, 200, 0, 300, 0}},
		{"right longer", []int16{100}, []int16{200, 300, 400}, []int16{100, 200, 0, 300, 0, 400}},
		{"left only", []int16{1, 2, 3}, nil, []int16{1, 0, 2, 0, 3, 0}},
		{"right only", nil, []int16{1, 2, 3}, []int16{0, 1, 0, 2, 0, 3}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InterleaveInt16(tc.left, tc.right)
			if len(got) != len(tc.want) {
				t.Fatalf("InterleaveInt16: got len %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("InterleaveInt16[%d] = %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestInt16ByteRoundTrip(t *testing.T) {
	original := []int16{0, 1, -1, 32767, -32768, 12345, -12345}
	bytes := Int16ToBytes(original)
	if len(bytes) != len(original)*2 {
		t.Fatalf("Int16ToBytes: got %d bytes, want %d", len(bytes), len(original)*2)
	}
	restored := BytesToInt16(bytes)
	if len(restored) != len(original) {
		t.Fatalf("BytesToInt16: got len %d, want %d", len(restored), len(original))
	}
	for i := range original {
		if restored[i] != original[i] {
			t.Errorf("round trip[%d] = %d, want %d", i, restored[i], original[i])
		}
	}
}

func TestBytesToFloat32(t *testing.T) {
	// Encode a known float32 value.
	val := float32(0.5)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(val))

	got := BytesToFloat32(buf)
	if len(got) != 1 {
		t.Fatalf("BytesToFloat32: got len %d, want 1", len(got))
	}
	if got[0] != val {
		t.Errorf("BytesToFloat32 = %f, want %f", got[0], val)
	}
}

func TestResample48to16Mono(t *testing.T) {
	// 6 stereo pairs at 48kHz = 12 float32 values.
	// After stereo-to-mono: 6 mono samples.
	// After 3:1 decimation: 2 samples.
	stereo := []float32{
		0.2, 0.8, // pair 0: mono = 0.5
		0.1, 0.3, // pair 1: mono = 0.2 (skipped by decimation)
		0.4, 0.6, // pair 2: mono = 0.5 (skipped by decimation)
		-0.5, 0.5, // pair 3: mono = 0.0
		0.0, 0.0, // pair 4: (skipped)
		0.0, 0.0, // pair 5: (skipped)
	}

	got := Resample48to16Mono(stereo)
	if len(got) != 2 {
		t.Fatalf("Resample48to16Mono: got len %d, want 2", len(got))
	}
	// First sample: mono[0] = 0.5 -> Float32ToInt16(0.5) = 16384
	want0 := Float32ToInt16(0.5)
	if abs16(got[0]-want0) > 1 {
		t.Errorf("Resample48to16Mono[0] = %d, want ~%d", got[0], want0)
	}
	// Second sample: mono[3] = 0.0 -> Float32ToInt16(0.0) = 0
	if got[1] != 0 {
		t.Errorf("Resample48to16Mono[1] = %d, want 0", got[1])
	}
}

// --- Ring Buffer Tests ---

func TestRingBuffer_WriteRead(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2) // 1 second = 50 frames

	frame := heimdall.AudioFrame{
		Data:       []byte{1, 2, 3, 4},
		SampleRate: 16000,
		Channels:   2,
		Timestamp:  0,
	}

	ok := rb.Write(frame)
	if !ok {
		t.Error("Write to non-full buffer should return true")
	}
	if rb.Len() != 1 {
		t.Errorf("Len after write = %d, want 1", rb.Len())
	}

	got, gotOk := rb.Read()
	if !gotOk {
		t.Error("Read from non-empty buffer should return true")
	}
	if len(got.Data) != 4 || got.Data[0] != 1 {
		t.Errorf("Read returned wrong frame data")
	}
	if rb.Len() != 0 {
		t.Errorf("Len after read = %d, want 0", rb.Len())
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2) // capacity = 50 frames
	capacity := rb.Cap()

	// Fill the buffer completely.
	for i := 0; i < capacity; i++ {
		frame := heimdall.AudioFrame{
			Data:      []byte{byte(i)},
			Timestamp: time.Duration(i) * frameDuration,
		}
		ok := rb.Write(frame)
		if !ok {
			t.Errorf("Write %d should not drop (buffer not full yet)", i)
		}
	}

	if rb.Len() != capacity {
		t.Errorf("Len = %d, want %d", rb.Len(), capacity)
	}

	// Write one more — should drop the oldest.
	overflowFrame := heimdall.AudioFrame{
		Data:      []byte{255},
		Timestamp: time.Duration(capacity) * frameDuration,
	}
	ok := rb.Write(overflowFrame)
	if ok {
		t.Error("Write to full buffer should return false (dropped oldest)")
	}
	if rb.Len() != capacity {
		t.Errorf("Len after overflow = %d, want %d", rb.Len(), capacity)
	}
	if rb.Dropped() != 1 {
		t.Errorf("Dropped = %d, want 1", rb.Dropped())
	}

	// Read the first frame — it should be frame 1 (frame 0 was dropped).
	got, _ := rb.Read()
	if len(got.Data) != 1 || got.Data[0] != 1 {
		t.Errorf("After overflow, first read should be frame 1, got data=%v", got.Data)
	}
}

func TestRingBuffer_ReadBlocksUntilWrite(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2)

	done := make(chan heimdall.AudioFrame, 1)
	go func() {
		frame, ok := rb.Read()
		if ok {
			done <- frame
		}
	}()

	// Reader should be blocked. Give it a moment.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Read should block on empty buffer")
	default:
		// Expected.
	}

	// Write a frame to unblock the reader.
	expected := heimdall.AudioFrame{Data: []byte{42}}
	rb.Write(expected)

	select {
	case got := <-done:
		if len(got.Data) != 1 || got.Data[0] != 42 {
			t.Errorf("Read got wrong data: %v", got.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("Read should unblock after Write")
	}
}

func TestRingBuffer_CloseUnblocksRead(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2)

	done := make(chan bool, 1)
	go func() {
		_, ok := rb.Read()
		done <- ok
	}()

	time.Sleep(20 * time.Millisecond)
	rb.Close()

	select {
	case ok := <-done:
		if ok {
			t.Error("Read after Close on empty buffer should return false")
		}
	case <-time.After(time.Second):
		t.Fatal("Close should unblock Read")
	}
}

func TestRingBuffer_ReadRemainingAfterClose(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2)

	rb.Write(heimdall.AudioFrame{Data: []byte{1}})
	rb.Write(heimdall.AudioFrame{Data: []byte{2}})
	rb.Close()

	// Should still be able to read the two frames.
	f1, ok1 := rb.Read()
	if !ok1 || f1.Data[0] != 1 {
		t.Errorf("First read after close: ok=%v, data=%v", ok1, f1.Data)
	}
	f2, ok2 := rb.Read()
	if !ok2 || f2.Data[0] != 2 {
		t.Errorf("Second read after close: ok=%v, data=%v", ok2, f2.Data)
	}

	// Now buffer is empty and closed — should return false.
	_, ok3 := rb.Read()
	if ok3 {
		t.Error("Read on empty closed buffer should return false")
	}
}

func TestRingBuffer_ConcurrentWriteRead(t *testing.T) {
	rb := NewRingBuffer(1, 16000, 2)
	const numFrames = 200

	var wg sync.WaitGroup

	// Writer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numFrames; i++ {
			rb.Write(heimdall.AudioFrame{
				Data:      []byte{byte(i % 256)},
				Timestamp: time.Duration(i) * frameDuration,
			})
		}
		rb.Close()
	}()

	// Reader goroutine.
	readCount := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, ok := rb.Read()
			if !ok {
				return
			}
			readCount++
		}
	}()

	wg.Wait()

	// We wrote 200 frames into a 50-frame buffer. Some will be dropped.
	// readCount should be > 0 and <= numFrames.
	if readCount == 0 {
		t.Error("Reader should have read at least some frames")
	}
	if readCount > numFrames {
		t.Errorf("Reader read more frames than were written: %d > %d", readCount, numFrames)
	}
}

// --- Mock AudioSource ---

// mockAudioSource is a test helper that implements audio.AudioSource.
type mockAudioSource struct {
	sampleRate int
	channels   int
	stream     chan heimdall.AudioFrame
	started    bool
	stopped    bool
	mu         sync.Mutex
}

func newMockSource(sampleRate, channels int) *mockAudioSource {
	return &mockAudioSource{
		sampleRate: sampleRate,
		channels:   channels,
		stream:     make(chan heimdall.AudioFrame, 100),
	}
}

func (m *mockAudioSource) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	return nil
}

func (m *mockAudioSource) Stream() <-chan heimdall.AudioFrame {
	return m.stream
}

func (m *mockAudioSource) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stream)
	}
	return nil
}

func (m *mockAudioSource) SampleRate() int { return m.sampleRate }
func (m *mockAudioSource) Channels() int   { return m.channels }

// sendFrame is a helper to send a frame to the mock source's stream.
func (m *mockAudioSource) sendFrame(frame heimdall.AudioFrame) {
	m.stream <- frame
}

// --- Mixer Tests ---

func TestMixer_StartStop(t *testing.T) {
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start should fail.
	if err := mixer.Start(ctx); err == nil {
		t.Error("Double Start should return error")
	}

	// Stop should be clean.
	sys.Stop()
	mic.Stop()
	if err := mixer.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double stop should be safe.
	if err := mixer.Stop(); err != nil {
		t.Fatalf("Double Stop: %v", err)
	}
}

func TestMixer_ProducesInterlevedStereo(t *testing.T) {
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Prepare system audio: 960 stereo float32 samples at 48kHz = one 20ms frame.
	// Each stereo pair has L=0.5, R=0.5 so mono = 0.5.
	sysFloats := make([]float32, 960*2) // 960 stereo pairs
	for i := range sysFloats {
		sysFloats[i] = 0.5
	}
	sysBytes := float32ToBytes(sysFloats)
	sys.sendFrame(heimdall.AudioFrame{
		Data:       sysBytes,
		SampleRate: 48000,
		Channels:   2,
		Timestamp:  0,
	})

	// Prepare mic audio: 320 mono int16 samples at 16kHz = one 20ms frame.
	// All samples = 1000.
	micSamples := make([]int16, 320)
	for i := range micSamples {
		micSamples[i] = 1000
	}
	micBytes := Int16ToBytes(micSamples)
	mic.sendFrame(heimdall.AudioFrame{
		Data:       micBytes,
		SampleRate: 16000,
		Channels:   1,
		Timestamp:  0,
	})

	// Wait for the mixer to produce output.
	stream := mixer.Stream()
	var frame heimdall.AudioFrame
	select {
	case frame = <-stream:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for mixed frame")
	}

	// Verify output frame properties.
	if frame.SampleRate != 16000 {
		t.Errorf("SampleRate = %d, want 16000", frame.SampleRate)
	}
	if frame.Channels != 2 {
		t.Errorf("Channels = %d, want 2", frame.Channels)
	}

	// Output should be 320 stereo pairs * 2 bytes each = 1280 bytes.
	expectedBytes := samplesPerFrame * 2 * 2
	if len(frame.Data) != expectedBytes {
		t.Errorf("Data length = %d, want %d", len(frame.Data), expectedBytes)
	}

	// Decode and verify channel assignment (AD-007).
	stereoSamples := BytesToInt16(frame.Data)
	if len(stereoSamples) != samplesPerFrame*2 {
		t.Fatalf("stereo samples = %d, want %d", len(stereoSamples), samplesPerFrame*2)
	}

	// Left channel (even indices) = system audio (resampled from 0.5 float -> ~16384 int16).
	// Right channel (odd indices) = mic audio (1000).
	expectedSys := Float32ToInt16(0.5) // ~16384
	for i := 0; i < 10; i++ {
		left := stereoSamples[2*i]
		right := stereoSamples[2*i+1]

		if abs16(left-expectedSys) > 2 {
			t.Errorf("Left[%d] = %d, want ~%d (system audio)", i, left, expectedSys)
		}
		if right != 1000 {
			t.Errorf("Right[%d] = %d, want 1000 (mic audio)", i, right)
		}
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

func TestMixer_ChannelAssignment_AD007(t *testing.T) {
	// Verify that Left=system, Right=mic per AD-007.
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// System audio: all 0.25 float -> ~8192 int16.
	sysFloats := make([]float32, 960*2)
	for i := range sysFloats {
		sysFloats[i] = 0.25
	}
	sys.sendFrame(heimdall.AudioFrame{
		Data:       float32ToBytes(sysFloats),
		SampleRate: 48000,
		Channels:   2,
		Timestamp:  0,
	})

	// Mic audio: all -5000 int16.
	micSamples := make([]int16, 320)
	for i := range micSamples {
		micSamples[i] = -5000
	}
	mic.sendFrame(heimdall.AudioFrame{
		Data:       Int16ToBytes(micSamples),
		SampleRate: 16000,
		Channels:   1,
		Timestamp:  0,
	})

	stream := mixer.Stream()
	var frame heimdall.AudioFrame
	select {
	case frame = <-stream:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for mixed frame")
	}

	stereo := BytesToInt16(frame.Data)
	expectedSys := Float32ToInt16(0.25) // ~8192

	// Check first few sample pairs.
	for i := 0; i < 5; i++ {
		left := stereo[2*i]
		right := stereo[2*i+1]

		// Left should be positive (system audio ~8192).
		if left < 0 {
			t.Errorf("Left[%d] = %d, expected positive (system audio)", i, left)
		}
		if abs16(left-expectedSys) > 2 {
			t.Errorf("Left[%d] = %d, want ~%d", i, left, expectedSys)
		}

		// Right should be -5000 (mic audio).
		if right != -5000 {
			t.Errorf("Right[%d] = %d, want -5000 (mic audio)", i, right)
		}
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

func TestMixer_SilentSystemSource(t *testing.T) {
	// When system audio has no data, left channel should be zeros.
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Only send mic data, no system audio.
	micSamples := make([]int16, 320)
	for i := range micSamples {
		micSamples[i] = 2000
	}
	mic.sendFrame(heimdall.AudioFrame{
		Data:       Int16ToBytes(micSamples),
		SampleRate: 16000,
		Channels:   1,
		Timestamp:  0,
	})

	stream := mixer.Stream()
	var frame heimdall.AudioFrame
	select {
	case frame = <-stream:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for mixed frame")
	}

	stereo := BytesToInt16(frame.Data)

	// Left channel (system) should be silence (zeros).
	// Right channel (mic) should be 2000.
	for i := 0; i < 5; i++ {
		left := stereo[2*i]
		right := stereo[2*i+1]

		if left != 0 {
			t.Errorf("Left[%d] = %d, want 0 (silent system)", i, left)
		}
		if right != 2000 {
			t.Errorf("Right[%d] = %d, want 2000 (mic)", i, right)
		}
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

func TestMixer_SilentMicSource(t *testing.T) {
	// When mic has no data, right channel should be zeros.
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Only send system audio, no mic data.
	sysFloats := make([]float32, 960*2)
	for i := range sysFloats {
		sysFloats[i] = 0.3
	}
	sys.sendFrame(heimdall.AudioFrame{
		Data:       float32ToBytes(sysFloats),
		SampleRate: 48000,
		Channels:   2,
		Timestamp:  0,
	})

	stream := mixer.Stream()
	var frame heimdall.AudioFrame
	select {
	case frame = <-stream:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for mixed frame")
	}

	stereo := BytesToInt16(frame.Data)
	expectedSys := Float32ToInt16(0.3)

	for i := 0; i < 5; i++ {
		left := stereo[2*i]
		right := stereo[2*i+1]

		if abs16(left-expectedSys) > 2 {
			t.Errorf("Left[%d] = %d, want ~%d (system)", i, left, expectedSys)
		}
		if right != 0 {
			t.Errorf("Right[%d] = %d, want 0 (silent mic)", i, right)
		}
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

func TestMixer_NoGoroutineLeakOnStop(t *testing.T) {
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the mixer run briefly.
	time.Sleep(50 * time.Millisecond)

	sys.Stop()
	mic.Stop()

	// Stop should complete without deadlock within a reasonable time.
	done := make(chan struct{})
	go func() {
		mixer.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success — Stop completed.
	case <-time.After(5 * time.Second):
		t.Fatal("mixer.Stop() timed out — possible goroutine leak")
	}
}

func TestMixer_StreamReturnsStereoFrameFormat(t *testing.T) {
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Send data to both sources.
	sysFloats := make([]float32, 960*2)
	sys.sendFrame(heimdall.AudioFrame{
		Data:       float32ToBytes(sysFloats),
		SampleRate: 48000,
		Channels:   2,
	})
	micSamples := make([]int16, 320)
	mic.sendFrame(heimdall.AudioFrame{
		Data:       Int16ToBytes(micSamples),
		SampleRate: 16000,
		Channels:   1,
	})

	stream := mixer.Stream()
	select {
	case frame := <-stream:
		if frame.SampleRate != outputSampleRate {
			t.Errorf("output SampleRate = %d, want %d", frame.SampleRate, outputSampleRate)
		}
		if frame.Channels != outputChannels {
			t.Errorf("output Channels = %d, want %d", frame.Channels, outputChannels)
		}
		// 320 stereo pairs * 2 bytes = 1280 bytes per frame.
		if len(frame.Data) != samplesPerFrame*outputChannels*2 {
			t.Errorf("output Data len = %d, want %d", len(frame.Data), samplesPerFrame*outputChannels*2)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for output frame")
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

func TestMixer_RingBufferReceivesFrames(t *testing.T) {
	sys := newMockSource(48000, 2)
	mic := newMockSource(16000, 1)

	mixer := NewMixer(sys, mic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mixer.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Send a frame pair.
	sysFloats := make([]float32, 960*2)
	sys.sendFrame(heimdall.AudioFrame{
		Data:       float32ToBytes(sysFloats),
		SampleRate: 48000,
		Channels:   2,
	})
	mic.sendFrame(heimdall.AudioFrame{
		Data:       Int16ToBytes(make([]int16, 320)),
		SampleRate: 16000,
		Channels:   1,
	})

	// Drain the output channel so the mixer produces the frame.
	stream := mixer.Stream()
	select {
	case <-stream:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for output")
	}

	// The ring buffer should also have the frame.
	rb := mixer.RingBuffer()
	if rb.Len() < 1 {
		t.Error("Ring buffer should have received at least one frame")
	}

	sys.Stop()
	mic.Stop()
	mixer.Stop()
}

// --- Test Helpers ---

// float32ToBytes converts float32 samples to little-endian byte representation.
func float32ToBytes(samples []float32) []byte {
	buf := make([]byte, len(samples)*4)
	for i, s := range samples {
		binary.LittleEndian.PutUint32(buf[4*i:], math.Float32bits(s))
	}
	return buf
}

// abs16 returns the absolute value of an int16 difference, handling overflow.
func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}
