// Package recording implements optional raw-audio persistence during a
// meeting (audio.save_recording in config). Distinct from internal/recovery
// (which persists transcribed text segments every 30s for crash recovery):
// this package persists the actual PCM audio bytes flowing through the
// mixer, before they reach the transcriber.
//
// Two reasons this exists:
//   - Graceful degradation: if the transcriber and the analyzer both fail,
//     the meeting is not lost -- the raw audio is still on disk.
//   - It is the on-disk artifact a future local/offline transcriber (e.g. a
//     whisper.cpp-backed batch pass) would run against, decoupled from the
//     live recording's real-time constraints.
package recording

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// wavHeaderSize is the fixed size of a canonical 16-bit PCM WAV header
// (RIFF/WAVE chunk + fmt subchunk + data subchunk, no extension fields).
const wavHeaderSize = 44

// Dir returns the path to the recordings directory
// (~/.heimdall/recordings/), matching config's default
// audio.recording_path. Callers that have a config-supplied path should
// prefer that (config.ExpandHome(cfg.Audio.RecordingPath)) -- this is the
// fallback used when config didn't specify one.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".heimdall", "recordings")
	}
	return filepath.Join(home, ".heimdall", "recordings")
}

// FileName builds the recording filename for a meeting, mirroring
// internal/recovery's "{timestamp}-{title}.ext" convention so the two kinds
// of on-disk artifact are easy to correlate by eye.
func FileName(title string, startTime time.Time) string {
	return fmt.Sprintf("%s-%s.wav",
		startTime.Format("2006-01-02T15-04-05"),
		heimdall.SanitizeFilename(title, "untitled"),
	)
}

// WAVWriter incrementally writes PCM16 AudioFrames to a WAV file on disk,
// patching the header with the real data size on Close. Safe for concurrent
// WriteFrame calls.
type WAVWriter struct {
	f          *os.File
	path       string
	sampleRate int
	channels   int

	mu        sync.Mutex
	dataBytes uint32
	closed    bool
}

// NewWAVWriter creates the parent directory (0700, matching
// internal/recovery and internal/config's convention for heimdall-owned
// directories that may contain sensitive content) and path, and writes a
// placeholder header immediately so a crash before the first WriteFrame
// still leaves a valid, empty WAV file rather than a corrupt one.
func NewWAVWriter(path string, sampleRate, channels int) (*WAVWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating recording directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("creating recording file: %w", err)
	}

	w := &WAVWriter{f: f, path: path, sampleRate: sampleRate, channels: channels}
	if err := w.writeHeader(0); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return w, nil
}

// Path returns the file path this writer is writing to.
func (w *WAVWriter) Path() string {
	return w.path
}

// WriteFrame appends one AudioFrame's PCM bytes to the file. A write error
// is returned to the caller to log, but callers should treat it as
// non-fatal to the recording itself (audio-safety.md: a sink failure must
// never take down the live pipeline). WriteFrame is a no-op after Close.
func (w *WAVWriter) WriteFrame(frame heimdall.AudioFrame) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}

	n, err := w.f.Write(frame.Data)
	if err != nil {
		return fmt.Errorf("writing audio frame to %s: %w", w.path, err)
	}
	w.dataBytes += uint32(n)
	return nil
}

// Checkpoint patches the header with the data size written so far, so a
// crash mid-recording leaves a playable WAV of everything captured up to
// the last checkpoint instead of one whose header claims zero bytes of
// audio. The record command calls it periodically (V-006's 30s cadence).
// A no-op after Close.
func (w *WAVWriter) Checkpoint() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	if err := w.writeHeader(w.dataBytes); err != nil {
		return err
	}
	// writeHeader leaves the offset just past the header; resume appending.
	if _, err := w.f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seeking to end of %s: %w", w.path, err)
	}
	return nil
}

// Close patches the WAV header with the final data size and closes the
// file. Idempotent -- safe to call multiple times (mirrors
// RecoveryWriter.Cleanup's idempotency guard).
func (w *WAVWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.writeHeader(w.dataBytes); err != nil {
		w.f.Close()
		return err
	}
	return w.f.Close()
}

// writeHeader seeks to the start of the file and writes the 44-byte
// canonical 16-bit PCM WAV header sized for dataBytes of payload, then
// returns with the file position at offset 44 (Write advances the file
// position by the bytes written) -- so the first call, at construction with
// dataBytes=0, leaves the file correctly positioned for the first
// WriteFrame with no separate seek-to-end step needed.
func (w *WAVWriter) writeHeader(dataBytes uint32) error {
	const (
		bitsPerSample = 16
		pcmFormat     = 1
	)
	byteRate := uint32(w.sampleRate*w.channels*bitsPerSample) / 8
	blockAlign := uint16(w.channels*bitsPerSample) / 8

	header := make([]byte, wavHeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataBytes)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16) // fmt subchunk size for PCM
	binary.LittleEndian.PutUint16(header[20:22], pcmFormat)
	binary.LittleEndian.PutUint16(header[22:24], uint16(w.channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(w.sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)

	if _, err := w.f.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking to WAV header in %s: %w", w.path, err)
	}
	if _, err := w.f.Write(header); err != nil {
		return fmt.Errorf("writing WAV header to %s: %w", w.path, err)
	}
	return nil
}
