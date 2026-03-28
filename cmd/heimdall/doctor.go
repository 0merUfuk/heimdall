package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/config"
)

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

	// Check macOS version.
	total++
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			ver := strings.TrimSpace(string(out))
			fmt.Printf("  [pass] macOS %s\n", ver)
			passed++
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
	if _, err := exec.LookPath("heimdall-audio"); err == nil {
		fmt.Printf("  [pass] heimdall-audio helper found in PATH\n")
		passed++
	} else {
		// Check next to the heimdall binary.
		exePath, _ := os.Executable()
		if exePath != "" {
			dir := exePath[:strings.LastIndex(exePath, "/")]
			if _, err := os.Stat(dir + "/heimdall-audio"); err == nil {
				fmt.Printf("  [pass] heimdall-audio helper found\n")
				passed++
			} else {
				fmt.Printf("  [FAIL] heimdall-audio not found -- system audio capture unavailable\n")
			}
		} else {
			fmt.Printf("  [FAIL] heimdall-audio not found\n")
		}
	}

	// Check Obsidian vault.
	total++
	cfg, err := config.Load(config.ConfigPath())
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
	} else {
		fmt.Println(" Fix the issues above before recording.")
	}

	return nil
}
