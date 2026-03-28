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

Usage:
  heimdall record --title "Sprint Planning"    Start recording a meeting
  heimdall doctor                              Check prerequisites
  heimdall version                             Print version info`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
