// Package audio provides audio capture source implementations.
// This file implements SystemAudioSource which wraps the heimdall-audio Swift
// subprocess to capture system audio via Core Audio Taps (macOS 14.2+).
//
// The Swift binary outputs raw PCM (48kHz, 32-bit float, stereo interleaved) to stdout.
// This wrapper reads from stdout, packages data into AudioFrames, and implements
// the AudioSource interface.
//
// V-002 mitigation: crash detection within 2 seconds, auto-restart with exponential
// backoff (1s, 2s, 4s), max 3 retries.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

const (
	// systemSampleRate is the capture sample rate in Hz (48kHz from Swift helper).
	systemSampleRate = 48000

	// systemChannels is the number of capture channels (stereo).
	systemChannels = 2

	// systemBytesPerSample is the byte count per sample (32-bit float = 4 bytes).
	systemBytesPerSample = 4

	// systemFrameDuration is the target duration per audio frame (~20ms).
	// At 48kHz, 32-bit float, stereo: 48000 * 0.020 * 2 * 4 = 7680 bytes per frame.
	systemFrameDuration = 20 * time.Millisecond

	// systemFrameBytes is the expected byte count per frame chunk.
	// 48000 samples/sec * 0.020 sec * 2 channels * 4 bytes/sample = 7680 bytes.
	systemFrameBytes = 7680

	// systemStreamBufferSize is the channel buffer capacity in frames.
	// At 20ms per frame, 100 frames = 2 seconds of buffering.
	systemStreamBufferSize = 100

	// maxRestartAttempts is the maximum number of automatic restart attempts
	// before giving up (V-002).
	maxRestartAttempts = 3

	// gracefulStopTimeout is how long to wait after sending "stop" to stdin
	// before escalating to SIGTERM.
	gracefulStopTimeout = 5 * time.Second

	// sigtermTimeout is how long to wait after SIGTERM before escalating to SIGKILL.
	sigtermTimeout = 2 * time.Second

	// exitCodePermissionDenied is the Swift helper exit code for Screen Recording
	// permission denied (V-003).
	exitCodePermissionDenied = 77

	// helperBinaryName is the name of the Swift audio helper binary.
	helperBinaryName = "heimdall-audio"
)

// ErrPermissionDenied is returned when the Swift helper exits with code 77,
// indicating Screen Recording permission has not been granted.
var ErrPermissionDenied = errors.New("screen recording permission denied: " +
	"grant permission in System Settings > Privacy & Security > Screen Recording")

// ErrRestartExhausted is returned when the Swift helper has crashed and all
// automatic restart attempts have been exhausted.
var ErrRestartExhausted = errors.New("system audio helper crashed and all restart attempts exhausted")

// processHandle tracks a single subprocess lifecycle. cmd.Wait() is called
// exactly once in a dedicated goroutine, and the result is broadcast via waitDone.
type processHandle struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	stdin    io.WriteCloser
	waitDone chan struct{} // closed when cmd.Wait() completes
	waitErr  error         // result of cmd.Wait(), valid after waitDone is closed
}

// SystemAudioSource captures system audio by wrapping the heimdall-audio Swift subprocess.
// It implements the AudioSource interface.
type SystemAudioSource struct {
	helperPath string // path to heimdall-audio binary
	proc       *processHandle
	frames     chan heimdall.AudioFrame
	errCh      chan error // communicates fatal errors from goroutines
	mu         sync.Mutex
	stopped    bool
	closeOnce  sync.Once
	ctx        context.Context
	cancel     context.CancelFunc
	startTime  time.Time

	// newCmd creates the subprocess command. Override in tests to inject
	// mock environment variables. If nil, defaults to exec.CommandContext.
	newCmd func(ctx context.Context, helperPath string) *exec.Cmd
}

// Compile-time assertion: SystemAudioSource must satisfy AudioSource.
var _ AudioSource = (*SystemAudioSource)(nil)

// NewSystemAudioSource creates a new SystemAudioSource.
// helperPath specifies the path to the heimdall-audio binary.
// If empty, the binary is located by searching:
//  1. The same directory as the running heimdall binary
//  2. $PATH
func NewSystemAudioSource(helperPath string) *SystemAudioSource {
	return &SystemAudioSource{
		helperPath: helperPath,
		frames:     make(chan heimdall.AudioFrame, systemStreamBufferSize),
		errCh:      make(chan error, 1),
	}
}

// Start begins system audio capture by spawning the Swift helper subprocess.
// It starts reader and watchdog goroutines to handle audio data and crash detection.
//
// Returns an error if:
//   - The heimdall-audio binary cannot be found
//   - The subprocess fails to start
//   - Screen Recording permission is denied (exit code 77)
//   - The context is already cancelled
func (s *SystemAudioSource) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("system audio start: %w", ctx.Err())
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return errors.New("system audio start: source has been stopped")
	}

	if s.proc != nil {
		return errors.New("system audio start: already started")
	}

	// Locate the helper binary.
	helperPath, err := s.resolveHelperPath()
	if err != nil {
		return fmt.Errorf("system audio start: %w", err)
	}

	// Create a derived context for the capture lifecycle.
	captureCtx, cancelFn := context.WithCancel(ctx)
	s.ctx = captureCtx
	s.cancel = cancelFn

	// Start the subprocess.
	proc, err := s.spawnHelper(captureCtx, helperPath)
	if err != nil {
		cancelFn()
		return fmt.Errorf("system audio start: %w", err)
	}
	s.proc = proc

	s.startTime = time.Now()

	// Start the reader goroutine that reads PCM data from stdout.
	go s.readLoop(captureCtx, proc)

	// Start the watchdog goroutine for crash detection and auto-restart (V-002).
	go s.watchdog(captureCtx, helperPath)

	return nil
}

// Stream returns a read-only channel of AudioFrames.
// The channel is closed when Stop() is called or the context is cancelled.
// Frames are ~20ms chunks of 48kHz, 32-bit float, stereo PCM (7680 bytes each).
func (s *SystemAudioSource) Stream() <-chan heimdall.AudioFrame {
	return s.frames
}

// Stop gracefully stops the Swift subprocess and closes the Stream channel.
// The shutdown sequence is:
//  1. Send "stop\n" to stdin (graceful shutdown request)
//  2. Wait up to 5 seconds for process to exit
//  3. If not exited: send SIGTERM
//  4. If still not exited after 2 more seconds: send SIGKILL
//
// Safe to call multiple times (idempotent).
func (s *SystemAudioSource) Stop() error {
	s.mu.Lock()

	// Cancel the context to signal all goroutines.
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	// Attempt graceful shutdown of the subprocess.
	var stopErr error
	if s.proc != nil {
		stopErr = s.stopProcess(s.proc)
		s.proc = nil
	}

	s.stopped = true
	s.mu.Unlock()

	// Close the channel exactly once, draining first to prevent panics.
	s.closeOnce.Do(func() {
		for {
			select {
			case <-s.frames:
				// Discard remaining frames.
			default:
				close(s.frames)
				return
			}
		}
	})

	return stopErr
}

// SampleRate returns the capture sample rate in Hz (48000).
func (s *SystemAudioSource) SampleRate() int {
	return systemSampleRate
}

// Channels returns the number of audio channels (2 = stereo).
func (s *SystemAudioSource) Channels() int {
	return systemChannels
}

// resolveHelperPath locates the heimdall-audio binary.
// Search order:
//  1. s.helperPath (if explicitly set)
//  2. Same directory as the running binary
//  3. $PATH
func (s *SystemAudioSource) resolveHelperPath() (string, error) {
	// If explicitly set, verify it exists.
	if s.helperPath != "" {
		if _, err := os.Stat(s.helperPath); err != nil {
			return "", fmt.Errorf("helper binary not found at %s: %w", s.helperPath, err)
		}
		return s.helperPath, nil
	}

	// Try same directory as the running binary.
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), helperBinaryName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Try $PATH.
	if path, err := exec.LookPath(helperBinaryName); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("cannot find %s: check installation or set helper path explicitly", helperBinaryName)
}

// spawnHelper creates and starts a subprocess, returning a processHandle.
// The handle includes a dedicated goroutine that calls cmd.Wait() exactly once.
func (s *SystemAudioSource) spawnHelper(ctx context.Context, helperPath string) (*processHandle, error) {
	var cmd *exec.Cmd
	if s.newCmd != nil {
		cmd = s.newCmd(ctx, helperPath)
	} else {
		cmd = exec.CommandContext(ctx, helperPath)
	}
	cmd.Stderr = os.Stderr // Forward Swift helper's stderr for diagnostics.

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start helper subprocess: %w", err)
	}

	ph := &processHandle{
		cmd:      cmd,
		stdout:   stdout,
		stdin:    stdin,
		waitDone: make(chan struct{}),
	}

	// Single goroutine calls Wait() exactly once, then broadcasts via waitDone.
	go func() {
		ph.waitErr = cmd.Wait()
		close(ph.waitDone)
	}()

	return ph, nil
}

// stopProcess performs the 3-phase shutdown: stdin "stop" -> SIGTERM -> SIGKILL.
func (s *SystemAudioSource) stopProcess(proc *processHandle) error {
	if proc == nil || proc.cmd.Process == nil {
		return nil
	}

	// Phase 1: Send "stop\n" to stdin for graceful shutdown.
	if proc.stdin != nil {
		_, _ = io.WriteString(proc.stdin, "stop\n")
		_ = proc.stdin.Close()
	}

	// Wait for process to exit with graceful timeout.
	select {
	case <-proc.waitDone:
		return nil
	case <-time.After(gracefulStopTimeout):
	}

	// Phase 2: SIGTERM.
	log.Printf("system audio: helper did not exit after stop command, sending SIGTERM")
	_ = proc.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-proc.waitDone:
		return nil
	case <-time.After(sigtermTimeout):
	}

	// Phase 3: SIGKILL (last resort).
	log.Printf("system audio: helper did not exit after SIGTERM, sending SIGKILL")
	_ = proc.cmd.Process.Kill()

	// Wait briefly for SIGKILL to take effect.
	select {
	case <-proc.waitDone:
	case <-time.After(2 * time.Second):
	}

	return nil
}

// readLoop reads raw PCM data from the subprocess stdout and creates AudioFrames.
// Each frame is 20ms of audio: 48000 * 2 * 4 * 0.020 = 7680 bytes.
// Runs until the context is cancelled or stdout is closed (subprocess exit).
func (s *SystemAudioSource) readLoop(ctx context.Context, proc *processHandle) {
	buf := make([]byte, systemFrameBytes)
	var bytesRead int64

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if proc.stdout == nil {
			return
		}

		// Read exactly one frame worth of data.
		n, err := io.ReadFull(proc.stdout, buf)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, clean exit.
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
				// Subprocess exited or pipe closed. The watchdog will handle restart.
				return
			}
			log.Printf("system audio: read error: %v", err)
			return
		}

		if n == 0 {
			continue
		}

		// Copy the buffer since we reuse it.
		data := make([]byte, n)
		copy(data, buf[:n])

		// Calculate timestamp from total bytes read.
		bytesPerSecond := int64(systemSampleRate * systemChannels * systemBytesPerSample)
		timestamp := time.Duration(bytesRead * int64(time.Second) / bytesPerSecond)
		bytesRead += int64(n)

		frame := heimdall.AudioFrame{
			Data:       data,
			SampleRate: systemSampleRate,
			Channels:   systemChannels,
			Timestamp:  timestamp,
		}

		// Non-blocking send per audio-safety rules.
		select {
		case s.frames <- frame:
		case <-ctx.Done():
			return
		default:
			// Frame dropped -- consumer is too slow. Log at debug level.
			log.Printf("system audio: frame dropped (consumer too slow), timestamp=%v", timestamp)
		}
	}
}

// watchdog monitors the subprocess for unexpected exits and implements
// automatic restart with exponential backoff (V-002).
// Detects crashes within 2 seconds. Max 3 restart attempts with backoff: 1s, 2s, 4s.
func (s *SystemAudioSource) watchdog(ctx context.Context, helperPath string) {
	var restartCount int

	for {
		// Get current process handle.
		s.mu.Lock()
		proc := s.proc
		s.mu.Unlock()

		if proc == nil {
			return
		}

		// Wait for the process to exit via the waitDone channel.
		// This does NOT call cmd.Wait() -- it listens for the result.
		select {
		case <-ctx.Done():
			return
		case <-proc.waitDone:
			// Process exited.
		}

		// Check if we were asked to stop.
		if ctx.Err() != nil {
			return
		}

		// Analyze the exit.
		exitCode := extractExitCode(proc.waitErr)

		// Exit code 77: Screen Recording permission denied -- do not retry.
		if exitCode == exitCodePermissionDenied {
			log.Printf("system audio: Screen Recording permission denied (exit code 77)")
			s.sendError(ErrPermissionDenied)
			return
		}

		// Log the crash.
		gapStart := time.Now()
		log.Printf("system audio: helper crashed (exit code %d, attempt %d/%d)",
			exitCode, restartCount+1, maxRestartAttempts)

		// Check if we have restart attempts remaining.
		restartCount++
		if restartCount > maxRestartAttempts {
			log.Printf("system audio: all %d restart attempts exhausted", maxRestartAttempts)
			s.sendError(ErrRestartExhausted)
			return
		}

		// Exponential backoff: 1s, 2s, 4s.
		backoff := time.Duration(1<<(restartCount-1)) * time.Second

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Attempt restart.
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}

		newProc, err := s.spawnHelper(ctx, helperPath)
		if err != nil {
			s.mu.Unlock()
			log.Printf("system audio: restart failed: %v", err)
			continue // Will retry on next loop iteration.
		}
		s.proc = newProc
		s.mu.Unlock()

		// Track the audio gap.
		gapDuration := time.Since(gapStart)
		log.Printf("system audio: helper restarted (gap: %v)", gapDuration)

		// Start a new reader goroutine for the new process.
		go s.readLoop(ctx, newProc)
	}
}

// extractExitCode returns the exit code from a Wait() error.
// Returns 0 for nil error, -1 for non-exit errors.
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// sendError sends a fatal error through the error channel (non-blocking).
func (s *SystemAudioSource) sendError(err error) {
	select {
	case s.errCh <- err:
	default:
	}
}

// Err returns the error channel for fatal errors (e.g., permission denied,
// restart exhaustion). Consumers should select on this alongside Stream()
// to detect unrecoverable failures.
func (s *SystemAudioSource) Err() <-chan error {
	return s.errCh
}
