package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/audio"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/session"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

var (
	recordTitle string
	recordApp   string
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a meeting with live transcription",
	Long: `Start recording a meeting. Captures system audio and microphone,
streams to Deepgram for real-time transcription with speaker diarization,
and displays the live transcript in the terminal.

Press Ctrl+C to stop recording.

Requires DEEPGRAM_API_KEY environment variable.`,
	Example: `  heimdall record --title "Sprint Planning"
  heimdall record --title "1:1 with Sarah" --app "Zoom"`,
	RunE: runRecord,
}

func init() {
	recordCmd.Flags().StringVar(&recordTitle, "title", "", "meeting title (required)")
	recordCmd.Flags().StringVar(&recordApp, "app", "", "target application for process-specific capture (optional)")
	_ = recordCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(recordCmd)
}

func runRecord(cmd *cobra.Command, args []string) error {
	// Check for Deepgram API key.
	apiKey := os.Getenv("DEEPGRAM_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEEPGRAM_API_KEY environment variable is not set\n\n" +
			"Set it with:\n  export DEEPGRAM_API_KEY=your_key_here\n\n" +
			"Get a free API key at: https://console.deepgram.com/")
	}

	// Print header.
	fmt.Printf("heimdall %s -- recording \"%s\"\n", version, recordTitle)

	// Create a context that listens for interrupt signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Stage 1: Create audio sources.
	var systemSource audio.AudioSource
	systemSource = audio.NewSystemAudioSource("")
	micSource := audio.NewMicrophoneSource()

	// Stage 3: Create the transcriber.
	dgTranscriber := transcriber.NewDeepgramTranscriber(apiKey)

	// Create the session orchestrator (wires stages 1-4).
	sess := session.NewMeetingSession(recordTitle, systemSource, micSource, dgTranscriber)

	// Register the live display callback.
	sess.OnSegment(func(seg heimdall.Segment) {
		displaySegment(seg)
	})

	// Start the session (starts all pipeline stages).
	if err := sess.Start(ctx); err != nil {
		return fmt.Errorf("failed to start recording: %w", err)
	}

	// Print status line.
	sysStatus := "off"
	if sess.SystemAvailable() {
		sysStatus = "on"
	}
	fmt.Printf("Audio: system %s  mic on  | STT: deepgram (connected) | recording...\n\n", sysStatus)

	// Start the elapsed time display ticker.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Wait for interrupt signal.
	<-ctx.Done()

	fmt.Printf("\n\nStopping recording...\n")

	// Stop the session with a timeout to prevent hanging.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- sess.Stop()
	}()

	select {
	case err := <-stopDone:
		if err != nil {
			log.Printf("warning: shutdown error: %v", err)
		}
	case <-stopCtx.Done():
		log.Printf("warning: shutdown timed out after 30 seconds")
	}

	// Print summary.
	printSummary(sess)

	return nil
}

// displaySegment prints a transcript segment to the terminal.
// Format: [HH:MM:SS] Speaker N: text...
func displaySegment(seg heimdall.Segment) {
	// For interim results, show them on the same line (overwrite).
	// For final results, print on a new line.
	timestamp := formatDuration(seg.Start)

	if seg.IsFinal {
		fmt.Printf("[%s] Speaker %d: %s\n", timestamp, seg.Speaker, seg.Text)
	} else {
		// Interim results: print without newline, with carriage return for overwrite.
		text := seg.Text
		if len(text) > 80 {
			text = text[:77] + "..."
		}
		fmt.Printf("\r[%s] Speaker %d: %s", timestamp, seg.Speaker, text)
	}
}

// formatDuration formats a duration as HH:MM:SS.
func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// printSummary prints the recording summary after shutdown.
func printSummary(sess *session.MeetingSession) {
	segments := sess.Segments()
	duration := sess.Duration()
	speakers := sess.SpeakerCount()

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Meeting recorded: %s | %d segments | %d speakers\n",
		formatDuration(duration),
		len(segments),
		speakers,
	)
	fmt.Println(strings.Repeat("-", 60))
}
