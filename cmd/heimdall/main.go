// Package main provides the CLI entry point for heimdall.
// heimdall is a meeting companion that captures audio, transcribes with speaker
// diarization, analyzes via Claude, and writes structured notes to an Obsidian vault.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "heimdall",
	Short: "Meeting companion -- capture, transcribe, analyze, write to Obsidian",
	Long: `heimdall captures meeting audio, transcribes with speaker diarization,
analyzes via Claude, and writes structured notes to your Obsidian vault.

Quick start:
  heimdall config init                         First-run setup wizard
  heimdall record                              Record with auto-title
  heimdall record --profile daily              Use a saved profile
  heimdall record --title "Sprint Planning"    Record with custom title
  heimdall doctor                              Check prerequisites
  heimdall config show                         View current settings`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
