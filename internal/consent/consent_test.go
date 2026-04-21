package consent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/config"
)

// fixedNow returns a deterministic time for assertions on AcknowledgedAt.
func fixedNow() time.Time {
	return time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
}

// newTempConfigPath returns a path inside a test-scoped temp dir where the
// consent gate can write a config file without touching the user's home.
func newTempConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "config.yaml")
}

// TestGate_ConfigAlreadyAcknowledged_NoBannerNoWrite verifies that when the
// config already marks consent as acknowledged, Gate returns silently and
// does not touch the prompt streams or rewrite the config file.
func TestGate_ConfigAlreadyAcknowledged_NoBannerNoWrite(t *testing.T) {
	cfgPath := newTempConfigPath(t)
	cfg := config.DefaultConfig()
	cfg.Consent.Acknowledged = true
	cfg.Consent.AcknowledgedAt = "2026-04-01T00:00:00Z"

	var out bytes.Buffer
	err := Gate(context.Background(), Options{
		ConfigPath:   cfgPath,
		Cfg:          cfg,
		PromptReader: strings.NewReader(""),
		PromptWriter: &out,
		IsTerminalFn: func() bool { return true },
	})
	if err != nil {
		t.Fatalf("Gate: unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("banner printed when config already acknowledged: %q", out.String())
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("config file should not be written when already acknowledged, stat err=%v", err)
	}
}

// TestGate_FlagAcknowledged_NoWrite verifies that --consent-acknowledged
// lets the run proceed silently without persisting anything to config. This
// is the scripted/CI path: the user passed the flag, so they do not want a
// persistent bit flipped on their local machine.
func TestGate_FlagAcknowledged_NoWrite(t *testing.T) {
	cfgPath := newTempConfigPath(t)
	cfg := config.DefaultConfig()

	var out bytes.Buffer
	err := Gate(context.Background(), Options{
		FlagAcknowledged: true,
		ConfigPath:       cfgPath,
		Cfg:              cfg,
		PromptReader:     strings.NewReader(""),
		PromptWriter:     &out,
		IsTerminalFn:     func() bool { return true },
	})
	if err != nil {
		t.Fatalf("Gate: unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("banner printed when flag acknowledged: %q", out.String())
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("config should not be written when flag-only acknowledged, stat err=%v", err)
	}
	if cfg.Consent.Acknowledged {
		t.Errorf("cfg.Consent.Acknowledged mutated by flag path (should remain false)")
	}
}

// TestGate_TTY_PromptAndPersist verifies the interactive happy path:
// banner prints, Enter is read, config.yaml is written with Acknowledged=true
// and a valid RFC3339 timestamp.
func TestGate_TTY_PromptAndPersist(t *testing.T) {
	cfgPath := newTempConfigPath(t)
	cfg := config.DefaultConfig()

	var out bytes.Buffer
	err := Gate(context.Background(), Options{
		ConfigPath:   cfgPath,
		Cfg:          cfg,
		PromptReader: strings.NewReader("\n"), // user hits Enter
		PromptWriter: &out,
		IsTerminalFn: func() bool { return true },
		Now:          fixedNow,
	})
	if err != nil {
		t.Fatalf("Gate: unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "heimdall records this meeting") {
		t.Errorf("banner not printed; got: %q", out.String())
	}

	// Config file must exist and reflect acknowledgement.
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load post-gate: %v", err)
	}
	if !loaded.Consent.Acknowledged {
		t.Error("Consent.Acknowledged not persisted as true")
	}
	want := fixedNow().UTC().Format(time.RFC3339)
	if loaded.Consent.AcknowledgedAt != want {
		t.Errorf("AcknowledgedAt: got %q, want %q", loaded.Consent.AcknowledgedAt, want)
	}
}

// TestGate_NonTTY_NoFlag_ReturnsError verifies the scripted-without-flag
// path: a pipe/non-TTY stdin with no flag and no prior config must refuse to
// record, printing the banner once for the operator to inspect and returning
// the actionable error.
func TestGate_NonTTY_NoFlag_ReturnsError(t *testing.T) {
	cfgPath := newTempConfigPath(t)
	cfg := config.DefaultConfig()

	var out bytes.Buffer
	err := Gate(context.Background(), Options{
		ConfigPath:   cfgPath,
		Cfg:          cfg,
		PromptReader: strings.NewReader(""),
		PromptWriter: &out,
		IsTerminalFn: func() bool { return false },
	})
	if err == nil {
		t.Fatal("Gate: expected error for non-TTY without flag/config, got nil")
	}
	var nae NotAcknowledgedError
	if !errors.As(err, &nae) {
		t.Errorf("expected NotAcknowledgedError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "--consent-acknowledged") {
		t.Errorf("error message should mention --consent-acknowledged, got: %v", err)
	}
	if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
		t.Errorf("config should not be written on non-TTY refusal, stat err=%v", statErr)
	}
}

// TestGate_ContextCancelDuringPrompt verifies that Ctrl+C (ctx cancel) while
// the banner is displayed returns context.Canceled and does NOT persist.
func TestGate_ContextCancelDuringPrompt(t *testing.T) {
	cfgPath := newTempConfigPath(t)
	cfg := config.DefaultConfig()

	// A pipe reader that never returns -- simulates a user staring at the
	// banner with no keypress.
	pr, pw := io.Pipe()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())

	var out bytes.Buffer

	// Cancel shortly after Gate starts reading.
	done := make(chan error, 1)
	go func() {
		done <- Gate(ctx, Options{
			ConfigPath:   cfgPath,
			Cfg:          cfg,
			PromptReader: pr,
			PromptWriter: &out,
			IsTerminalFn: func() bool { return true },
			Now:          fixedNow,
		})
	}()

	// Give the goroutine a moment to print the banner and block on read.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Gate did not return after ctx cancel")
	}

	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("config should not be written on cancel, stat err=%v", err)
	}
}

// TestGate_AtomicWrite_PreservesOtherKeys verifies that the consent write
// path goes through Config.Save (atomic temp+rename) and does not clobber
// unrelated config keys. We pre-populate a config with a custom vault path
// and a keyword, then have the gate acknowledge, and assert the pre-existing
// fields survive.
func TestGate_AtomicWrite_PreservesOtherKeys(t *testing.T) {
	cfgPath := newTempConfigPath(t)

	// Seed an existing config on disk with distinguishing values.
	seed := config.DefaultConfig()
	seed.Obsidian.VaultPath = "/tmp/pre-existing-vault"
	seed.Keywords = []string{"Kubernetes", "gRPC"}
	if err := seed.Save(cfgPath); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	// Load it back (mimicking what record.go does).
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load seed: %v", err)
	}

	var out bytes.Buffer
	if err := Gate(context.Background(), Options{
		ConfigPath:   cfgPath,
		Cfg:          cfg,
		PromptReader: strings.NewReader("\n"),
		PromptWriter: &out,
		IsTerminalFn: func() bool { return true },
		Now:          fixedNow,
	}); err != nil {
		t.Fatalf("Gate: unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load post-gate: %v", err)
	}
	if loaded.Obsidian.VaultPath != "/tmp/pre-existing-vault" {
		t.Errorf("vault_path clobbered: got %q", loaded.Obsidian.VaultPath)
	}
	if len(loaded.Keywords) != 2 || loaded.Keywords[0] != "Kubernetes" {
		t.Errorf("keywords clobbered: got %v", loaded.Keywords)
	}
	if !loaded.Consent.Acknowledged {
		t.Error("Consent.Acknowledged not persisted")
	}

	// No leftover temp files from the atomic write.
	entries, err := os.ReadDir(filepath.Dir(cfgPath))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
