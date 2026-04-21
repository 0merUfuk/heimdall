package main

import (
	"runtime"
	"strings"
	"testing"
)

// captureStdout (shared with config_test.go) redirects os.Stdout for fn and
// returns what was written. We rely on it here because runScreenRecordingCheck
// prints directly via the fmt package.

// withMockScreenRecordingRunner temporarily replaces the global runner hook
// and restores it on cleanup.
func withMockScreenRecordingRunner(t *testing.T, stdout string, exitCode int, err error) {
	t.Helper()
	orig := screenRecordingRunner
	screenRecordingRunner = func() (string, int, error) {
		return stdout, exitCode, err
	}
	t.Cleanup(func() { screenRecordingRunner = orig })
}

// TestScreenRecordingCheck_Granted_Darwin verifies that a granted response
// from the helper produces a [pass] line and returns 1 (counts toward pass).
// Skipped on non-darwin because the function returns the [skip] path there.
func TestScreenRecordingCheck_Granted_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only path")
	}
	withMockScreenRecordingRunner(t, "screen-recording-permission: granted\n", 0, nil)

	var got int
	out := captureStdout(t, func() {
		got = runScreenRecordingCheck(true)
	})

	if got != 1 {
		t.Errorf("return: got %d, want 1 (pass counts as 1)", got)
	}
	if !strings.Contains(out, "[pass] Screen Recording permission granted") {
		t.Errorf("output missing [pass] line, got: %q", out)
	}
}

// TestScreenRecordingCheck_Denied_Darwin verifies that a denied response
// produces [FAIL] with the actionable System-Settings guidance and returns 0.
func TestScreenRecordingCheck_Denied_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only path")
	}
	withMockScreenRecordingRunner(t, "screen-recording-permission: denied\n", 77, nil)

	var got int
	out := captureStdout(t, func() {
		got = runScreenRecordingCheck(true)
	})

	if got != 0 {
		t.Errorf("return: got %d, want 0 (fail counts as 0)", got)
	}
	if !strings.Contains(out, "[FAIL] Screen Recording permission denied") {
		t.Errorf("output missing [FAIL] line, got: %q", out)
	}
	if !strings.Contains(out, "System Settings") {
		t.Errorf("output missing actionable guidance, got: %q", out)
	}
}

// TestScreenRecordingCheck_HelperMissing verifies that when the heimdall-audio
// helper is not found, the check is [skip]'d (not re-reported as a fail; the
// helper-not-found FAIL upstream is sufficient) and counts as pass.
func TestScreenRecordingCheck_HelperMissing(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only; non-darwin takes the GOOS skip path first")
	}
	// Mock runner that would panic if called — the skip branch must not reach it.
	withMockScreenRecordingRunner(t, "", 0, errNotCalled{})

	var got int
	out := captureStdout(t, func() {
		got = runScreenRecordingCheck(false)
	})

	if got != 1 {
		t.Errorf("return: got %d, want 1 (skip counts as pass)", got)
	}
	if !strings.Contains(out, "[skip]") {
		t.Errorf("output missing [skip] line, got: %q", out)
	}
}

// TestScreenRecordingCheck_RunnerError verifies that an error from the runner
// (e.g., helper crashed, spawn failed) surfaces as a [FAIL] and counts as 0.
func TestScreenRecordingCheck_RunnerError(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only path")
	}
	withMockScreenRecordingRunner(t, "", 0, errSpawnFailed{})

	var got int
	out := captureStdout(t, func() {
		got = runScreenRecordingCheck(true)
	})

	if got != 0 {
		t.Errorf("return: got %d, want 0 (runner error counts as fail)", got)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("output missing [FAIL] line, got: %q", out)
	}
}

// TestScreenRecordingCheck_NonDarwinSkips verifies the non-macOS guard: on
// linux/windows the check prints [skip] and counts as pass. On a darwin CI
// host this test is skipped because we can't fake runtime.GOOS at runtime.
func TestScreenRecordingCheck_NonDarwinSkips(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("need non-darwin host to exercise GOOS skip branch")
	}
	// Runner must not be invoked on non-darwin hosts.
	withMockScreenRecordingRunner(t, "", 0, errNotCalled{})

	var got int
	out := captureStdout(t, func() {
		got = runScreenRecordingCheck(true)
	})

	if got != 1 {
		t.Errorf("return: got %d, want 1 (skip counts as pass)", got)
	}
	if !strings.Contains(out, "[skip]") {
		t.Errorf("output missing [skip] line, got: %q", out)
	}
}

// errNotCalled marks a runner that should never be invoked in a given test.
type errNotCalled struct{}

func (errNotCalled) Error() string { return "runner should not have been called" }

// errSpawnFailed simulates an exec failure (binary missing, permission denied).
type errSpawnFailed struct{}

func (errSpawnFailed) Error() string { return "spawn failed" }
