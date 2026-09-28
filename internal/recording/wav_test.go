package recording

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

func TestNewWAVWriter_CreatesValidEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if len(data) != wavHeaderSize {
		t.Fatalf("file size: got %d, want exactly the %d-byte header for zero frames", len(data), wavHeaderSize)
	}
	assertValidHeader(t, data, 16000, 1, 0)
}

func TestWAVWriter_WritesFramesAndPatchesHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}

	frame1 := heimdall.AudioFrame{Data: []byte{1, 2, 3, 4}}
	frame2 := heimdall.AudioFrame{Data: []byte{5, 6, 7, 8, 9, 10}}

	if err := w.WriteFrame(frame1); err != nil {
		t.Fatalf("WriteFrame(1): unexpected error: %v", err)
	}
	if err := w.WriteFrame(frame2); err != nil {
		t.Fatalf("WriteFrame(2): unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	wantDataBytes := len(frame1.Data) + len(frame2.Data)
	if len(data) != wavHeaderSize+wantDataBytes {
		t.Fatalf("file size: got %d, want %d (header + %d data bytes)", len(data), wavHeaderSize+wantDataBytes, wantDataBytes)
	}
	assertValidHeader(t, data, 16000, 1, uint32(wantDataBytes))

	gotPCM := data[wavHeaderSize:]
	wantPCM := append(append([]byte{}, frame1.Data...), frame2.Data...)
	if string(gotPCM) != string(wantPCM) {
		t.Errorf("PCM payload: got %v, want %v", gotPCM, wantPCM)
	}
}

func TestWAVWriter_StereoHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 48000, 2)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	assertValidHeader(t, data, 48000, 2, 0)
}

func TestWAVWriter_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got error: %v", err)
	}
}

func TestWAVWriter_WriteAfterCloseIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	// Must not error and must not grow the file -- WriteFrame after Close is
	// silently dropped, matching the audio-safety posture of never letting a
	// sink failure disrupt the pipeline.
	if err := w.WriteFrame(heimdall.AudioFrame{Data: []byte{1, 2, 3}}); err != nil {
		t.Errorf("WriteFrame after Close should be a no-op, got error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if len(data) != wavHeaderSize {
		t.Errorf("file grew after Close: got %d bytes, want %d (header only)", len(data), wavHeaderSize)
	}
}

func TestWAVWriter_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "recordings", "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("parent directory was not created: %v", err)
	}
}

func TestWAVWriter_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	w.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions: got %o, want 0600", perm)
	}
}

func TestWAVWriter_ConcurrentWriteFrame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			_ = w.WriteFrame(heimdall.AudioFrame{Data: []byte{1, 2}})
		})
	}
	wg.Wait()

	if err := w.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if len(data) != wavHeaderSize+n*2 {
		t.Errorf("file size: got %d, want %d (no lost writes under concurrency)", len(data), wavHeaderSize+n*2)
	}
}

func TestPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	w, err := NewWAVWriter(path, 16000, 1)
	if err != nil {
		t.Fatalf("NewWAVWriter: unexpected error: %v", err)
	}
	defer w.Close()

	if got := w.Path(); got != path {
		t.Errorf("Path(): got %q, want %q", got, path)
	}
}

func TestFileName(t *testing.T) {
	start := time.Date(2026, 9, 13, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		title string
		want  string
	}{
		{"Sprint Planning", "2026-09-13T14-30-00-sprint-planning.wav"},
		{"Sağlık Toplantısı", "2026-09-13T14-30-00-sağlık-toplantısı.wav"},
		{"", "2026-09-13T14-30-00-untitled.wav"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := FileName(tt.title, start); got != tt.want {
				t.Errorf("FileName(%q, ...): got %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestDir_ReturnsHeimdallRecordingsPath(t *testing.T) {
	got := Dir()
	if filepath.Base(got) != "recordings" {
		t.Errorf("Dir(): got %q, want a path ending in .../recordings", got)
	}
	if filepath.Base(filepath.Dir(got)) != ".heimdall" {
		t.Errorf("Dir(): got %q, want a path under .../.heimdall/recordings", got)
	}
}

// assertValidHeader parses a WAV header per the canonical 16-bit PCM layout
// and checks it against the expected sample rate, channel count, and data
// size, independent of WAVWriter's own internals (a from-scratch parse, not
// a round-trip through the same code that produced it).
func assertValidHeader(t *testing.T, data []byte, wantSampleRate, wantChannels int, wantDataBytes uint32) {
	t.Helper()
	if len(data) < wavHeaderSize {
		t.Fatalf("file too short to contain a WAV header: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" {
		t.Errorf("missing RIFF magic, got %q", data[0:4])
	}
	if string(data[8:12]) != "WAVE" {
		t.Errorf("missing WAVE magic, got %q", data[8:12])
	}
	if string(data[12:16]) != "fmt " {
		t.Errorf("missing fmt subchunk, got %q", data[12:16])
	}
	if string(data[36:40]) != "data" {
		t.Errorf("missing data subchunk, got %q", data[36:40])
	}

	audioFormat := binary.LittleEndian.Uint16(data[20:22])
	if audioFormat != 1 {
		t.Errorf("audio format: got %d, want 1 (PCM)", audioFormat)
	}
	channels := binary.LittleEndian.Uint16(data[22:24])
	if int(channels) != wantChannels {
		t.Errorf("channels: got %d, want %d", channels, wantChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(data[24:28])
	if int(sampleRate) != wantSampleRate {
		t.Errorf("sample rate: got %d, want %d", sampleRate, wantSampleRate)
	}
	bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
	if bitsPerSample != 16 {
		t.Errorf("bits per sample: got %d, want 16", bitsPerSample)
	}
	gotDataBytes := binary.LittleEndian.Uint32(data[40:44])
	if gotDataBytes != wantDataBytes {
		t.Errorf("data chunk size: got %d, want %d", gotDataBytes, wantDataBytes)
	}
	riffSize := binary.LittleEndian.Uint32(data[4:8])
	if riffSize != 36+wantDataBytes {
		t.Errorf("RIFF chunk size: got %d, want %d", riffSize, 36+wantDataBytes)
	}
}

// TestWAVWriter_CheckpointMidRecording: a checkpoint must (a) make the file
// valid for everything written so far -- what survives a crash -- and (b)
// not disturb later writes, which must append after the existing payload
// rather than overwrite it from the header position.
func TestWAVWriter_CheckpointMidRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	w, err := NewWAVWriter(path, 16000, 2)
	if err != nil {
		t.Fatal(err)
	}

	first := heimdall.AudioFrame{Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	if err := w.WriteFrame(first); err != nil {
		t.Fatal(err)
	}
	if err := w.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// Simulated crash: read the file as it is on disk right now.
	snapshot, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != wavHeaderSize+len(first.Data) {
		t.Fatalf("snapshot size: got %d, want %d", len(snapshot), wavHeaderSize+len(first.Data))
	}
	assertValidHeader(t, snapshot, 16000, 2, uint32(len(first.Data)))

	second := heimdall.AudioFrame{Data: []byte{9, 10, 11, 12}}
	if err := w.WriteFrame(second); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte{}, first.Data...), second.Data...)
	assertValidHeader(t, final, 16000, 2, uint32(len(want)))
	if string(final[wavHeaderSize:]) != string(want) {
		t.Errorf("payload after checkpoint: got %v, want %v", final[wavHeaderSize:], want)
	}

	if err := w.Checkpoint(); err != nil {
		t.Errorf("Checkpoint after Close must be a no-op, got %v", err)
	}
}

// TestWAVWriter_StopsAtFormatLimit: the WAV header counts payload bytes in a
// uint32, so past 4 GiB the counter would wrap and the header would describe
// a fraction of the file. The writer must stop instead, keeping the file
// valid, and report the limit exactly once.
func TestWAVWriter_StopsAtFormatLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	w, err := NewWAVWriter(path, 16000, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Pretend almost the whole budget is already written, without doing 4 GiB
	// of I/O.
	w.mu.Lock()
	w.dataBytes = uint32(maxWAVDataBytes) - 4
	w.mu.Unlock()

	if err := w.WriteFrame(heimdall.AudioFrame{Data: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatalf("a frame that exactly fits must still be written: %v", err)
	}
	err = w.WriteFrame(heimdall.AudioFrame{Data: []byte{5, 6}})
	if err == nil || !strings.Contains(err.Error(), "4 GiB") {
		t.Fatalf("crossing the limit must report it once, got %v", err)
	}
	if err := w.WriteFrame(heimdall.AudioFrame{Data: []byte{7, 8}}); err != nil {
		t.Errorf("later frames must be dropped quietly, got %v", err)
	}
	if got := w.dataBytes; got != uint32(maxWAVDataBytes) {
		t.Errorf("dataBytes: got %d, want the limit %d (no wrap)", got, uint32(maxWAVDataBytes))
	}
}
