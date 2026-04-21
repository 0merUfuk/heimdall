package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/config"
)

// screenRecordingRunner is the hook used by the Screen Recording permission
// check to invoke `heimdall-audio --check-permissions`. Tests replace this
// with a stub so we can assert doctor's output for both granted and denied
// cases without shelling out to the real Swift binary.
//
// The runner returns (stdout, exitCode, err). An err from exec.Run() that is
// *not* an ExitError (e.g., binary not found) is returned verbatim; ExitError
// is decoded into exitCode so the caller can distinguish "denied" (exit 77)
// from "binary not found" (err != nil).
var screenRecordingRunner = defaultScreenRecordingRunner

func defaultScreenRecordingRunner() (stdout string, exitCode int, err error) {
	helperPath, err := locateAudioHelper()
	if err != nil {
		return "", 0, err
	}
	cmd := exec.Command(helperPath, "--check-permissions")
	out, runErr := cmd.Output()
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return string(out), ee.ExitCode(), nil
		}
		return string(out), 0, runErr
	}
	return string(out), 0, nil
}

// locateAudioHelper returns the path to the heimdall-audio binary, preferring
// PATH and falling back to a sibling of the running heimdall binary. Mirrors
// the lookup order used by the "heimdall-audio helper found in PATH" check.
func locateAudioHelper() (string, error) {
	if p, err := exec.LookPath("heimdall-audio"); err == nil {
		return p, nil
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("heimdall-audio not found: %w", err)
	}
	sibling := filepath.Join(filepath.Dir(exePath), "heimdall-audio")
	if _, err := os.Stat(sibling); err == nil {
		return sibling, nil
	}
	return "", fmt.Errorf("heimdall-audio not found in PATH or next to heimdall")
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check prerequisites for recording",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("heimdall doctor -- checking prerequisites...")
	fmt.Println()
	passed := 0
	total := 0

	// Check macOS version (>= 14.2 required for system audio capture).
	total++
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			ver := strings.TrimSpace(string(out))
			parts := strings.Split(ver, ".")
			major, minor := 0, 0
			if len(parts) >= 1 {
				fmt.Sscanf(parts[0], "%d", &major)
			}
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%d", &minor)
			}
			if major > 14 || (major == 14 && minor >= 2) {
				fmt.Printf("  [pass] macOS %s (>= 14.2 required)\n", ver)
				passed++
			} else {
				fmt.Printf("  [FAIL] macOS %s (>= 14.2 required for system audio capture)\n", ver)
			}
		} else {
			fmt.Printf("  [FAIL] Could not determine macOS version\n")
		}
	} else {
		fmt.Printf("  [FAIL] Not macOS (system audio capture requires macOS 14.2+)\n")
	}

	// Check Deepgram API key.
	total++
	if os.Getenv("DEEPGRAM_API_KEY") != "" {
		fmt.Printf("  [pass] Deepgram API key configured\n")
		passed++
	} else {
		fmt.Printf("  [FAIL] DEEPGRAM_API_KEY not set\n")
	}

	// Check Soniox API key (optional — Soniox is opt-in via
	// `--transcriber soniox`, so absence is not a doctor failure).
	total++
	if os.Getenv("SONIOX_API_KEY") != "" {
		fmt.Printf("  [pass] Soniox API key configured\n")
		passed++
	} else {
		fmt.Printf("  [info] Soniox API key not configured (optional, for --transcriber soniox)\n")
		passed++ // opt-in provider; absence is not a failure
	}

	// Check Anthropic API key.
	total++
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		fmt.Printf("  [pass] Anthropic API key configured\n")
		passed++
	} else {
		fmt.Printf("  [FAIL] ANTHROPIC_API_KEY not set -- Claude analysis will be skipped\n")
	}

	// Check heimdall-audio binary.
	total++
	helperFound := false
	if _, err := exec.LookPath("heimdall-audio"); err == nil {
		fmt.Printf("  [pass] heimdall-audio helper found in PATH\n")
		passed++
		helperFound = true
	} else {
		// Check next to the heimdall binary.
		exePath, _ := os.Executable()
		if exePath != "" {
			dir := filepath.Dir(exePath)
			if _, err := os.Stat(filepath.Join(dir, "heimdall-audio")); err == nil {
				fmt.Printf("  [pass] heimdall-audio helper found\n")
				passed++
				helperFound = true
			} else {
				fmt.Printf("  [FAIL] heimdall-audio not found -- system audio capture unavailable\n")
			}
		} else {
			fmt.Printf("  [FAIL] heimdall-audio not found\n")
		}
	}

	// Check Screen Recording permission (audit §6 item 9). On non-macOS hosts
	// this check is a no-op — Core Audio Taps is macOS-only, so the earlier
	// macOS-version [FAIL] already captured the platform mismatch.
	total++
	passed += runScreenRecordingCheck(helperFound)

	// Check Obsidian vault.
	total++
	cfg, err := config.Load(config.ConfigPath())
	if err == nil {
		// Resolve env vars so ${VAULT_PATH} references work.
		_ = cfg.ResolveEnvVars()
	}
	if err == nil && cfg.Obsidian.VaultPath != "" {
		expandedPath := config.ExpandHome(cfg.Obsidian.VaultPath)
		if info, err := os.Stat(expandedPath); err == nil && info.IsDir() {
			fmt.Printf("  [pass] Obsidian vault: %s\n", cfg.Obsidian.VaultPath)
			passed++
		} else {
			fmt.Printf("  [FAIL] Obsidian vault path not found: %s\n", cfg.Obsidian.VaultPath)
		}
	} else {
		fmt.Printf("  [warn] Obsidian vault not configured -- run 'heimdall config init'\n")
		passed++ // Not a failure, just unconfigured.
	}

	fmt.Printf("\n%d/%d checks passed.", passed, total)
	if passed == total {
		fmt.Println(" Ready to record.")
		return nil
	}

	fmt.Println(" Fix the issues above before recording.")
	return fmt.Errorf("%d/%d checks failed", total-passed, total)
}

// runScreenRecordingCheck prints the Screen Recording permission check line
// and returns 1 if the check passes (granted, or skipped on non-macOS), 0 if
// it fails. The caller adds the return value to the cumulative pass count.
//
// Behaviour matrix:
//
//	GOOS != darwin              -> [skip], counts as pass (macOS-only feature)
//	helperFound == false        -> [skip], counts as pass (the earlier
//	                               "heimdall-audio not found" FAIL already
//	                               signalled the root cause; re-reporting it
//	                               as a second failure is noise)
//	runner returns err != nil   -> [FAIL], counts as fail
//	stdout contains "granted"   -> [pass], counts as pass
//	otherwise (denied/unknown)  -> [FAIL] with actionable guidance
func runScreenRecordingCheck(helperFound bool) int {
	if runtime.GOOS != "darwin" {
		fmt.Println("  [skip] Screen Recording permission (macOS-only)")
		return 1
	}
	if !helperFound {
		fmt.Println("  [skip] Screen Recording permission (heimdall-audio not available)")
		return 1
	}

	stdout, exitCode, err := screenRecordingRunner()
	if err != nil {
		fmt.Printf("  [FAIL] Screen Recording permission check: %v\n", err)
		return 0
	}

	// The subprocess prints a single deterministic line; prefer string
	// matching over exit-code matching because the line is what the user
	// sees if they run the helper themselves.
	stdout = strings.TrimSpace(stdout)
	if strings.Contains(stdout, "granted") {
		fmt.Println("  [pass] Screen Recording permission granted")
		return 1
	}

	// Denied (exit 77) or unexpected. Either way, surface actionable guidance.
	fmt.Println("  [FAIL] Screen Recording permission denied")
	fmt.Println("         Grant in System Settings -> Privacy & Security -> Screen Recording")
	fmt.Println("         -> enable 'heimdall-audio', then re-run 'heimdall doctor'")
	_ = exitCode // exit code reserved for future telemetry; string match is authoritative
	return 0
}
