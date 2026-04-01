// Package recovery implements periodic crash recovery file writing and reading.
// During recording, a RecoveryWriter atomically persists transcript segments
// every 30 seconds to ~/.heimdall/recovery/. On clean shutdown the file is
// deleted. On crash, orphaned files can be scanned and re-processed.
//
// Atomic writes (V-006): data is written to a temp file then renamed, so a
// crash mid-write never corrupts the recovery file.
package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// DefaultWriteInterval is the default time between recovery file writes.
const DefaultWriteInterval = 30 * time.Second

// RecoveryMetadata contains information about the recording session.
type RecoveryMetadata struct {
	Title     string    `json:"title"`
	StartTime time.Time `json:"start_time"`
	Language  string    `json:"language"`
	Platform  string    `json:"platform"`
}

// RecoveryFile represents the on-disk recovery file format.
type RecoveryFile struct {
	Metadata RecoveryMetadata   `json:"metadata"`
	Segments []heimdall.Segment `json:"segments"`
	// Path is the filesystem path of the recovery file. Not serialized to JSON.
	// Populated by ListRecoveryFiles and LoadRecoveryFile.
	Path string `json:"-"`
}

// RecoveryWriter periodically persists transcript segments to disk for
// crash recovery. It is safe for concurrent use.
type RecoveryWriter struct {
	dir      string
	filename string
	mu       sync.Mutex
	segments []heimdall.Segment
	metadata RecoveryMetadata
	interval time.Duration
	stopped  bool // set by Stop; prevents race with concurrent Cleanup
	cleaned  bool // set by Cleanup; prevents Stop from re-creating the file
}

// RecoveryDir returns the path to the recovery directory (~/.heimdall/recovery/).
func RecoveryDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".heimdall", "recovery")
	}
	return filepath.Join(home, ".heimdall", "recovery")
}

// NewRecoveryWriter creates a new recovery writer that persists segments to
// the recovery directory. It creates the directory if it does not exist.
// The language parameter is stored in metadata so the recovery/analyze path
// can pass it through to Claude (Bug #23 fix).
func NewRecoveryWriter(title string, startTime time.Time, language string) (*RecoveryWriter, error) {
	return NewRecoveryWriterWithInterval(title, startTime, language, DefaultWriteInterval)
}

// NewRecoveryWriterWithInterval creates a recovery writer with a custom write interval.
// This is primarily used in tests to avoid 30-second waits.
func NewRecoveryWriterWithInterval(title string, startTime time.Time, language string, interval time.Duration) (*RecoveryWriter, error) {
	dir := RecoveryDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating recovery directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.json",
		startTime.Format("2006-01-02T15-04-05"),
		sanitizeTitle(title),
	)

	return &RecoveryWriter{
		dir:      dir,
		filename: filename,
		metadata: RecoveryMetadata{
			Title:     title,
			StartTime: startTime,
			Language:  language,
			Platform:  runtime.GOOS,
		},
		interval: interval,
	}, nil
}

// AddSegment appends a segment to the in-memory buffer. Thread-safe.
func (r *RecoveryWriter) AddSegment(seg heimdall.Segment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segments = append(r.segments, seg)
}

// Start begins the periodic write loop. It writes every interval (default 30s)
// until the context is cancelled. Start returns immediately -- the write loop
// runs in a separate goroutine.
func (r *RecoveryWriter) Start(ctx context.Context) {
	go r.writeLoop(ctx)
}

// writeLoop runs the periodic flush until context cancellation.
func (r *RecoveryWriter) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := r.Flush(); err != nil {
				// Log but do not abort -- recovery is best-effort.
				fmt.Fprintf(os.Stderr, "recovery: flush error: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// Stop is a convenience method that performs a final Flush.
// The write loop goroutine exits via context cancellation, not Stop.
// If Cleanup was already called, Stop is a no-op to avoid re-creating
// the recovery file that was just deleted.
func (r *RecoveryWriter) Stop() error {
	r.mu.Lock()
	if r.cleaned || r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	r.mu.Unlock()
	return r.Flush()
}

// Flush writes the current segments to disk atomically.
// It writes to a temporary file then renames it to the final path.
// The file permissions are set to 0600 (owner read/write only).
func (r *RecoveryWriter) Flush() error {
	r.mu.Lock()
	rf := RecoveryFile{
		Metadata: r.metadata,
		Segments: make([]heimdall.Segment, len(r.segments)),
	}
	copy(rf.Segments, r.segments)
	r.mu.Unlock()

	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling recovery file: %w", err)
	}

	finalPath := filepath.Join(r.dir, r.filename)

	// Atomic write: temp file -> rename.
	tmpFile, err := os.CreateTemp(r.dir, "recovery-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp recovery file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp recovery file: %w", err)
	}

	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting recovery file permissions: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp recovery file: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming recovery file: %w", err)
	}

	return nil
}

// Cleanup deletes the recovery file. Called on clean shutdown after a
// successful Obsidian write. Sets the cleaned flag so that a subsequent
// Stop() does not re-create the file via Flush().
func (r *RecoveryWriter) Cleanup() error {
	r.mu.Lock()
	r.cleaned = true
	r.mu.Unlock()

	finalPath := filepath.Join(r.dir, r.filename)
	err := os.Remove(finalPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing recovery file: %w", err)
	}
	return nil
}

// FilePath returns the full path to the recovery file.
func (r *RecoveryWriter) FilePath() string {
	return filepath.Join(r.dir, r.filename)
}

// ListRecoveryFiles scans the recovery directory for orphaned recovery files.
// Returns an empty slice if the directory does not exist.
func ListRecoveryFiles() ([]RecoveryFile, error) {
	dir := RecoveryDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading recovery directory: %w", err)
	}

	var files []RecoveryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		rf, err := LoadRecoveryFile(path)
		if err != nil {
			// Skip corrupt files -- log with cleanup guidance.
			fmt.Fprintf(os.Stderr, "recovery: skipping corrupt file %s: %v\n", entry.Name(), err)
			fmt.Fprintf(os.Stderr, "  To remove: rm %s\n", filepath.Join(RecoveryDir(), entry.Name()))
			continue
		}
		rf.Path = path
		files = append(files, *rf)
	}

	return files, nil
}

// LoadRecoveryFile reads and deserializes a single recovery file.
func LoadRecoveryFile(path string) (*RecoveryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading recovery file: %w", err)
	}

	var rf RecoveryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing recovery file: %w", err)
	}
	rf.Path = path

	return &rf, nil
}

// sanitizeTitle converts a meeting title to a filename-safe string.
// Lowercases, replaces non-alphanumeric characters with hyphens, and
// collapses consecutive hyphens.
func sanitizeTitle(title string) string {
	// Replace non-alphanumeric characters with hyphens.
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	s := re.ReplaceAllString(title, "-")
	s = strings.ToLower(s)
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}
