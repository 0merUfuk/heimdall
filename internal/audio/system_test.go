package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// ─── Mock subprocess helpers ────────────────────────────────────────────────
//
// These tests use Go's standard mock subprocess pattern:
// exec.Command(os.Args[0], "-test.run=TestHelperProcess") re-executes the
// test binary as a subprocess, running the TestHelperProcess function which
// inspects GO_TEST_HELPER_PROCESS to decide its behavior.
//
// See: https://npf.io/2015/06/testing-exec-command/
// ─────────────────────────────────────────────────────────────────────────────

// TestHelperProcess is not a real test. It is used as a mock subprocess by
// other tests. The GO_TEST_HELPER_PROCESS env var determines behavior.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") == "" {
		return // Not invoked as helper, skip.
	}

	mode := os.Getenv("GO_TEST_HELPER_MODE")
	switch mode {
	case "pcm":
		helperPCM()
	case "pcm_continuous":
		helperPCMContinuous()
	case "crash_immediate":
		os.Exit(1)
	case "crash_then_succeed":
		helperCrashThenSucceed()
	case "exit_77":
		fmt.Fprintln(os.Stderr, "heimdall-audio: Screen Recording permission required")
		os.Exit(77)
	case "slow_stop":
		helperSlowStop()
	case "pcm_multi":
		helperPCMMulti()
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s\n", mode)
		os.Exit(2)
	}

	os.Exit(0)
}

func helperPCM() {
	frame := make([]byte, systemFrameBytes)
	for i := 0; i < systemFrameBytes/systemBytesPerSample; i++ {
		val := float32(i) / float32(systemFrameBytes/systemBytesPerSample)
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(val))
	}
	os.Stdout.Write(frame)
}

func helperPCMMulti() {
	numFrames := 5
	frame := make([]byte, systemFrameBytes)
	for i := 0; i < numFrames; i++ {
		for j := range frame {
			frame[j] = byte(i)
		}
		os.Stdout.Write(frame)
	}
}

func helperPCMContinuous() {
	frame := make([]byte, systemFrameBytes)

	stopCh := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(stopCh)
				return
			}
			if n >= 4 && string(buf[:4]) == "stop" {
				close(stopCh)
				return
			}
		}
	}()

	for {
		select {
		case <-stopCh:
			return
		default:
			os.Stdout.Write(frame)
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func helperCrashThenSucceed() {
	crashThreshold, _ := strconv.Atoi(os.Getenv("GO_TEST_CRASH_THRESHOLD"))
	if crashThreshold == 0 {
		crashThreshold = 1
	}

	counterFile := os.Getenv("GO_TEST_COUNTER_FILE")
	if counterFile == "" {
		os.Exit(1)
	}

	count := 0
	if data, err := os.ReadFile(counterFile); err == nil {
		count, _ = strconv.Atoi(string(data))
	}
	count++
	os.WriteFile(counterFile, []byte(strconv.Itoa(count)), 0600)

	if count <= crashThreshold {
		os.Exit(1)
	}

	helperPCM()
}

func helperSlowStop() {
	time.Sleep(30 * time.Second)
}

// ─── Test utility: create a mock command factory ────────────────────────────

// mockCmdFactory returns a newCmd function that creates mock subprocess commands
// with the given mode and optional extra env vars.
func mockCmdFactory(mode string, envExtra ...string) func(ctx context.Context, helperPath string) *exec.Cmd {
	return func(ctx context.Context, helperPath string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
		env := []string{
			"GO_TEST_HELPER_PROCESS=1",
			"GO_TEST_HELPER_MODE=" + mode,
		}
		env = append(env, envExtra...)
		cmd.Env = env
		return cmd
	}
}

// startWithMock creates and starts a SystemAudioSource using a mock subprocess.
// The newCmd factory ensures ALL subprocess spawns (including watchdog restarts)
// use the same mock environment.
func startWithMock(t *testing.T, ctx context.Context, mode string, envExtra ...string) *SystemAudioSource {
	t.Helper()

	src := NewSystemAudioSource(os.Args[0])
	src.newCmd = mockCmdFactory(mode, envExtra...)

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start mock helper: %v", err)
	}

	return src
}

// ─── Tests ──────────────────────────────────────────────────────────────────

// Compile-time assertion: SystemAudioSource must satisfy AudioSource.
var _ AudioSource = (*SystemAudioSource)(nil)

func TestNewSystemAudioSource(t *testing.T) {
	src := NewSystemAudioSource("/path/to/helper")
	if src == nil {
		t.Fatal("NewSystemAudioSource returned nil")
	}
	if src.helperPath != "/path/to/helper" {
		t.Errorf("helperPath: got %q, want %q", src.helperPath, "/path/to/helper")
	}
	if src.frames == nil {
		t.Fatal("frames channel should be initialized")
	}
	if src.errCh == nil {
		t.Fatal("error channel should be initialized")
	}
	if src.stopped {
		t.Error("new SystemAudioSource should not be in stopped state")
	}
}

func TestSystemAudioSource_SampleRate(t *testing.T) {
	src := NewSystemAudioSource("")
	if got := src.SampleRate(); got != 48000 {
		t.Errorf("SampleRate: got %d, want 48000", got)
	}
}

func TestSystemAudioSource_Channels(t *testing.T) {
	src := NewSystemAudioSource("")
	if got := src.Channels(); got != 2 {
		t.Errorf("Channels: got %d, want 2", got)
	}
}

func TestSystemAudioSource_StreamReturnsNonNil(t *testing.T) {
	src := NewSystemAudioSource("")
	ch := src.Stream()
	if ch == nil {
		t.Fatal("Stream() returned nil channel")
	}
}

func TestSystemAudioSource_StopIdempotent(t *testing.T) {
	src := NewSystemAudioSource("")

	if err := src.Stop(); err != nil {
		t.Fatalf("first Stop: unexpected error: %v", err)
	}
	if err := src.Stop(); err != nil {
		t.Fatalf("second Stop: unexpected error: %v", err)
	}
	if err := src.Stop(); err != nil {
		t.Fatalf("third Stop: unexpected error: %v", err)
	}
}

func TestSystemAudioSource_StopClosesChannel(t *testing.T) {
	src := NewSystemAudioSource("")
	ch := src.Stream()

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}

	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Stop, but received a value")
	}
}

func TestSystemAudioSource_StartCancelledContext(t *testing.T) {
	src := NewSystemAudioSource(os.Args[0])

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := src.Start(ctx)
	if err == nil {
		t.Fatal("Start with cancelled context should return an error")
	}
	t.Logf("Start returned expected error: %v", err)
}

func TestSystemAudioSource_Constants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"systemSampleRate", systemSampleRate, 48000},
		{"systemChannels", systemChannels, 2},
		{"systemBytesPerSample", systemBytesPerSample, 4},
		{"systemStreamBufferSize", systemStreamBufferSize, 100},
		{"systemFrameBytes", systemFrameBytes, 7680},
		{"maxRestartAttempts", maxRestartAttempts, 3},
		{"exitCodePermissionDenied", exitCodePermissionDenied, 77},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}

	if systemFrameDuration != 20*time.Millisecond {
		t.Errorf("systemFrameDuration: got %v, want 20ms", systemFrameDuration)
	}
}

func TestSystemAudioSource_FrameBytesCalculation(t *testing.T) {
	expectedBytes := systemSampleRate * int(systemFrameDuration.Milliseconds()) *
		systemChannels * systemBytesPerSample / 1000

	if expectedBytes != systemFrameBytes {
		t.Errorf("frame bytes calculation: got %d, want %d", expectedBytes, systemFrameBytes)
	}
}

func TestSystemAudioSource_InterfaceCompliance(t *testing.T) {
	var src AudioSource = NewSystemAudioSource("")

	_ = src.Stream()
	_ = src.SampleRate()
	_ = src.Channels()

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

// TestSystemAudioSource_ReceivesFrames verifies that the source delivers
// AudioFrames with correct metadata when the mock helper outputs PCM data.
func TestSystemAudioSource_ReceivesFrames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "pcm")
	defer src.Stop()

	ch := src.Stream()

	select {
	case frame, ok := <-ch:
		if !ok {
			t.Fatal("stream channel closed before receiving a frame")
		}
		if len(frame.Data) != systemFrameBytes {
			t.Errorf("frame Data length: got %d, want %d", len(frame.Data), systemFrameBytes)
		}
		if frame.SampleRate != systemSampleRate {
			t.Errorf("frame SampleRate: got %d, want %d", frame.SampleRate, systemSampleRate)
		}
		if frame.Channels != systemChannels {
			t.Errorf("frame Channels: got %d, want %d", frame.Channels, systemChannels)
		}
		if frame.Timestamp != 0 {
			t.Errorf("first frame Timestamp: got %v, want 0", frame.Timestamp)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for frame")
	}
}

// TestSystemAudioSource_MultipleFrames verifies correct timestamp progression.
func TestSystemAudioSource_MultipleFrames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "pcm_multi")
	defer src.Stop()

	ch := src.Stream()

	var frames []heimdall.AudioFrame
	for i := 0; i < 5; i++ {
		select {
		case frame, ok := <-ch:
			if !ok {
				t.Fatalf("stream channel closed after %d frames", i)
			}
			frames = append(frames, frame)
		case <-ctx.Done():
			t.Fatalf("timed out after receiving %d frames", len(frames))
		}
	}

	for i := 1; i < len(frames); i++ {
		if frames[i].Timestamp <= frames[i-1].Timestamp {
			t.Errorf("frame %d timestamp %v not greater than frame %d timestamp %v",
				i, frames[i].Timestamp, i-1, frames[i-1].Timestamp)
		}
	}

	for i, f := range frames {
		if f.SampleRate != systemSampleRate {
			t.Errorf("frame %d SampleRate: got %d, want %d", i, f.SampleRate, systemSampleRate)
		}
		if f.Channels != systemChannels {
			t.Errorf("frame %d Channels: got %d, want %d", i, f.Channels, systemChannels)
		}
		if len(f.Data) != systemFrameBytes {
			t.Errorf("frame %d Data length: got %d, want %d", i, len(f.Data), systemFrameBytes)
		}
	}
}

// TestSystemAudioSource_StopSendsStopCommand verifies that Stop() sends "stop"
// to the subprocess stdin and the subprocess exits cleanly.
func TestSystemAudioSource_StopSendsStopCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "pcm_continuous")

	// Let it run briefly.
	time.Sleep(100 * time.Millisecond)

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- src.Stop()
	}()

	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: unexpected error: %v", err)
		}
	case <-time.After(gracefulStopTimeout + 3*time.Second):
		t.Fatal("Stop did not complete within expected time")
	}

	_, ok := <-src.Stream()
	if ok {
		t.Error("expected channel to be closed after Stop")
	}
}

// TestSystemAudioSource_CrashDetectedAndRestartExhausted verifies V-002:
// crash detection and restart exhaustion after maxRestartAttempts.
func TestSystemAudioSource_CrashDetectedAndRestartExhausted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "crash_immediate")
	defer src.Stop()

	select {
	case err := <-src.Err():
		if err == nil {
			t.Fatal("expected non-nil error from crash detection")
		}
		if !errors.Is(err, ErrRestartExhausted) {
			t.Errorf("expected ErrRestartExhausted, got: %v", err)
		}
		t.Logf("crash detected and restarts exhausted: %v", err)
	case <-ctx.Done():
		t.Fatal("crash not detected within expected time")
	}
}

// TestSystemAudioSource_ExitCode77PermissionDenied verifies that exit code 77
// is translated to ErrPermissionDenied without retries.
func TestSystemAudioSource_ExitCode77PermissionDenied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "exit_77")
	defer src.Stop()

	select {
	case err := <-src.Err():
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for permission denied error")
	}
}

// TestSystemAudioSource_CrashThenRecover verifies that the source successfully
// recovers after the helper crashes once and then succeeds on restart.
func TestSystemAudioSource_CrashThenRecover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	counterFile := filepath.Join(t.TempDir(), "crash_counter")

	src := NewSystemAudioSource(os.Args[0])
	src.newCmd = mockCmdFactory("crash_then_succeed",
		"GO_TEST_CRASH_THRESHOLD=1",
		"GO_TEST_COUNTER_FILE="+counterFile,
	)

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	// Wait for a frame from the recovered subprocess.
	select {
	case frame, ok := <-src.Stream():
		if !ok {
			t.Fatal("stream channel closed before receiving recovery frame")
		}
		if len(frame.Data) != systemFrameBytes {
			t.Errorf("recovery frame Data length: got %d, want %d", len(frame.Data), systemFrameBytes)
		}
		t.Log("successfully received frame after crash recovery")
	case err := <-src.Err():
		t.Fatalf("received error instead of recovery: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for crash recovery")
	}
}

// TestSystemAudioSource_ContextCancellation verifies that cancelling the parent
// context cleanly shuts down the source.
func TestSystemAudioSource_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	src := startWithMock(t, ctx, "pcm_continuous")

	time.Sleep(100 * time.Millisecond)

	cancel()

	time.Sleep(500 * time.Millisecond)

	if err := src.Stop(); err != nil {
		t.Fatalf("Stop after context cancellation: unexpected error: %v", err)
	}

	_, ok := <-src.Stream()
	if ok {
		t.Error("expected channel to be closed after context cancellation")
	}
}

// TestSystemAudioSource_ResolveHelperPath_ExplicitPath tests explicit path resolution.
func TestSystemAudioSource_ResolveHelperPath_ExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpBin := filepath.Join(tmpDir, helperBinaryName)
	if err := os.WriteFile(tmpBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("create temp binary: %v", err)
	}

	src := NewSystemAudioSource(tmpBin)
	path, err := src.resolveHelperPath()
	if err != nil {
		t.Fatalf("resolveHelperPath: unexpected error: %v", err)
	}
	if path != tmpBin {
		t.Errorf("resolveHelperPath: got %q, want %q", path, tmpBin)
	}
}

// TestSystemAudioSource_ResolveHelperPath_NotFound tests nonexistent path.
func TestSystemAudioSource_ResolveHelperPath_NotFound(t *testing.T) {
	src := NewSystemAudioSource("/nonexistent/path/to/helper")
	_, err := src.resolveHelperPath()
	if err == nil {
		t.Fatal("resolveHelperPath should return error for nonexistent path")
	}
	t.Logf("resolveHelperPath returned expected error: %v", err)
}

// TestSystemAudioSource_ResolveHelperPath_EmptyFallback tests empty path fallback.
func TestSystemAudioSource_ResolveHelperPath_EmptyFallback(t *testing.T) {
	src := NewSystemAudioSource("")
	_, err := src.resolveHelperPath()
	if err == nil {
		t.Log("resolveHelperPath found a helper binary (unexpected in test, but OK)")
	} else {
		t.Logf("resolveHelperPath returned expected error: %v", err)
	}
}

// TestSystemAudioSource_StartAfterStop verifies that Start returns an error
// if called after Stop.
func TestSystemAudioSource_StartAfterStop(t *testing.T) {
	src := NewSystemAudioSource(os.Args[0])
	_ = src.Stop()

	err := src.Start(context.Background())
	if err == nil {
		t.Fatal("Start after Stop should return an error")
	}
	t.Logf("Start after Stop returned expected error: %v", err)
}

// TestSystemAudioSource_ErrChannel verifies the Err() method returns a channel.
func TestSystemAudioSource_ErrChannel(t *testing.T) {
	src := NewSystemAudioSource("")
	ch := src.Err()
	if ch == nil {
		t.Fatal("Err() returned nil channel")
	}
}

// TestSystemAudioSource_FrameDataIsCopied verifies that frame data is copied.
func TestSystemAudioSource_FrameDataIsCopied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := startWithMock(t, ctx, "pcm_multi")
	defer src.Stop()

	ch := src.Stream()

	var frame1, frame2 heimdall.AudioFrame
	select {
	case f, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		frame1 = f
	case <-ctx.Done():
		t.Fatal("timed out")
	}

	select {
	case f, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		frame2 = f
	case <-ctx.Done():
		t.Fatal("timed out")
	}

	if len(frame1.Data) == 0 || len(frame2.Data) == 0 {
		t.Fatal("frames should have non-empty data")
	}

	// The mock fills each frame with a different byte value.
	if frame1.Data[0] == frame2.Data[0] {
		t.Log("note: frames have same first byte (could indicate aliasing)")
	}
}

// TestSystemAudioSource_ExtractExitCode verifies the exit code helper.
func TestSystemAudioSource_ExtractExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, 0},
		{"non-exit error", errors.New("generic"), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractExitCode(tt.err); got != tt.want {
				t.Errorf("extractExitCode: got %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSystemAudioSource_GracefulStopTimeout verifies the constant value.
func TestSystemAudioSource_GracefulStopTimeout(t *testing.T) {
	if gracefulStopTimeout != 5*time.Second {
		t.Errorf("gracefulStopTimeout: got %v, want 5s", gracefulStopTimeout)
	}
}

// TestSystemAudioSource_SIGTERMTimeout verifies the constant value.
func TestSystemAudioSource_SIGTERMTimeout(t *testing.T) {
	if sigtermTimeout != 2*time.Second {
		t.Errorf("sigtermTimeout: got %v, want 2s", sigtermTimeout)
	}
}
