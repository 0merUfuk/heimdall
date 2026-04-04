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

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/audio"
	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/output"
	"github.com/0merUfuk/heimdall/internal/recovery"
	"github.com/0merUfuk/heimdall/internal/session"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

var (
	recordTitle        string
	recordLanguage     string
	recordParticipants string
	recordKeywords     string
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a meeting with live transcription",
	Long: `Start recording a meeting. Captures system audio and microphone,
streams to Deepgram for real-time transcription with speaker diarization,
and displays the live transcript in the terminal.

When run with no flags, uses defaults from ~/.heimdall/config.yaml:
  - Title is auto-generated as "Meeting YYYY-MM-DD HH:MM"
  - Language, participants, and keywords come from config
  - CLI flags override config values when provided

Press Ctrl+C to stop recording.

Requires DEEPGRAM_API_KEY environment variable.`,
	Example: `  heimdall record                                    # uses config defaults
  heimdall record --title "Sprint Planning"          # custom title
  heimdall record --title "1:1" --participants "Sarah" # override participants
  heimdall record --language multi                    # auto-detect language`,
	RunE: runRecord,
}

func init() {
	recordCmd.Flags().StringVar(&recordTitle, "title", "", "meeting title (auto-generated if omitted)")
	recordCmd.Flags().StringVar(&recordLanguage, "language", "en", "transcription language code (e.g., en, tr, multi)")
	recordCmd.Flags().StringVar(&recordParticipants, "participants", "", "comma-separated list of participant names (hints for speaker identification)")
	recordCmd.Flags().StringVar(&recordKeywords, "keywords", "", "comma-separated list of context keywords for analysis")
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

	// Pre-validate config and Obsidian vault before starting the recording.
	// Better to fail now than after a 1-hour meeting.
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		log.Printf("warning: config file is malformed: %v", err)
		fmt.Printf("Fix by editing %s or resetting with: heimdall config init\n", config.ConfigPath())
	}
	if cfg != nil {
		if err := cfg.ResolveEnvVars(); err != nil {
			log.Printf("warning: resolving config env vars: %v", err)
		}
	}

	// Auto-generate title if not provided via CLI flag.
	if recordTitle == "" {
		recordTitle = time.Now().Format("Meeting 2006-01-02 15:04")
	}

	// Use config language as default when --language was not explicitly changed.
	if recordLanguage == "en" && cfg != nil && cfg.Deepgram.Language != "" {
		recordLanguage = cfg.Deepgram.Language
	}

	// Merge participants: CLI flag overrides config, but if flag is empty use config.
	var participants []string
	if recordParticipants != "" {
		for _, p := range strings.Split(recordParticipants, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				participants = append(participants, trimmed)
			}
		}
	} else if cfg != nil && len(cfg.Participants) > 0 {
		participants = cfg.Participants
	}

	// Merge keywords: CLI flag overrides config, but if flag is empty use config.
	var keywords []string
	if recordKeywords != "" {
		for _, k := range strings.Split(recordKeywords, ",") {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				keywords = append(keywords, trimmed)
			}
		}
	} else if cfg != nil && len(cfg.Keywords) > 0 {
		keywords = cfg.Keywords
	}

	// Print header.
	fmt.Printf("heimdall %s -- recording \"%s\"\n", version, recordTitle)
	fmt.Println("Warning: Recording active -- ensure all participants have consented to recording.")

	// Create a context that listens for interrupt signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Validate Obsidian vault path.
	if cfg != nil {
		if cfg.Obsidian.VaultPath != "" {
			expandedPath := config.ExpandHome(cfg.Obsidian.VaultPath)
			if _, err := os.Stat(expandedPath); err != nil {
				fmt.Printf("Warning: Obsidian vault path not found: %s\n", expandedPath)
				fmt.Println("Meeting notes will not be saved. Fix with:")
				fmt.Printf("  heimdall config set obsidian.vault_path /path/to/vault\n\n")
			}
		} else {
			fmt.Println("Note: Obsidian vault not configured. Meeting notes will not be saved.")
			fmt.Println("Set it with: heimdall config set obsidian.vault_path /path/to/vault")
			fmt.Println()
		}
	}

	// Stage 1: Create audio sources.
	var systemSource audio.AudioSource
	systemSource = audio.NewSystemAudioSource("")
	micSource := audio.NewMicrophoneSource()

	// Stage 3: Create the transcriber.
	dgTranscriber := transcriber.NewDeepgramTranscriber(apiKey)

	// Create the session orchestrator (wires stages 1-4).
	sess := session.NewMeetingSession(recordTitle, systemSource, micSource, dgTranscriber, recordLanguage)

	// V-006: Create crash recovery writer (writes segments to disk every 30s).
	recWriter, err := recovery.NewRecoveryWriter(recordTitle, time.Now(), recordLanguage)
	if err != nil {
		log.Printf("warning: crash recovery unavailable: %v", err)
	} else {
		recWriter.Start(ctx)
		defer recWriter.Stop()
	}

	// Register segment callback BEFORE Start to avoid data race.
	// OnSegment must be called before Start (see session.go contract).
	sess.OnSegment(func(seg heimdall.Segment) {
		displaySegment(seg)
		if recWriter != nil && seg.IsFinal {
			recWriter.AddSegment(seg)
		}
	})

	// Start the session (starts all pipeline stages).
	if err := sess.Start(ctx); err != nil {
		return fmt.Errorf("failed to start recording: %w", err)
	}

	// Brief pause to allow system audio permission check to fire.
	// Without this, the status line may show "system on" before
	// the Screen Recording denial is detected by the watchdog.
	time.Sleep(500 * time.Millisecond)

	// Print status line.
	sysStatus := "off"
	if sess.SystemAvailable() {
		sysStatus = "on"
	}
	fmt.Printf("Audio: system %s  mic on  | STT: deepgram (connected) | recording...\n", sysStatus)
	if sysStatus == "off" {
		fmt.Println("  (system audio unavailable -- recording mic only)")
	}
	fmt.Println()

	// Wait for interrupt signal.
	<-ctx.Done()

	fmt.Printf("\n\nStopping recording...\n")

	// Capture duration BEFORE Stop() — Stop() can take several seconds which
	// would inflate the reported recording duration.
	recordedDuration := sess.Duration()

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

	// Stage 5: Analyze via Claude (if API key available).
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" && len(sess.Segments()) > 0 {
		fmt.Println()
		fmt.Println("Note: ANTHROPIC_API_KEY not set -- Claude analysis skipped.")
		fmt.Println("The raw transcript was displayed above but no meeting note was saved.")
		fmt.Println("To enable analysis: export ANTHROPIC_API_KEY=your_key")
		fmt.Println("To analyze this session later: heimdall recover")
	}
	if anthropicKey != "" && len(sess.Segments()) > 0 {
		fmt.Println("Generating meeting summary via Claude...")

		// Config was pre-loaded and validated before recording started.
		// Reuse the same cfg variable to avoid loading config twice.

		// Determine the Claude model: config value, then fallback to default.
		model := analyzer.DefaultModel
		if cfg != nil && cfg.Claude.Model != "" && !strings.HasPrefix(cfg.Claude.Model, "${") {
			model = cfg.Claude.Model
		}

		// participants and keywords were already merged from config/CLI flags above.

		claude := analyzer.NewClaudeAnalyzer(anthropicKey)
		analyzeOpts := heimdall.AnalyzeOpts{
			Model:        model,
			Language:     recordLanguage,
			Participants: participants,
			Keywords:     keywords,
		}

		analyzeCtx, analyzeCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer analyzeCancel()

		note, err := claude.Summarize(analyzeCtx, sess.Segments(), analyzeOpts)
		if err != nil {
			log.Printf("warning: Claude analysis failed: %v", err)
		}

		// Detect fallback note from exhausted retries (V-009).
		if note != nil && note.IsFallback {
			fmt.Println("Warning: Claude analysis failed after retries. Raw transcript will be saved.")
		}

		if note != nil {
			note.Title = recordTitle
			note.Date = time.Now()
			note.Duration = recordedDuration
			note.Platform = "desktop"

			// Stage 6: Write to Obsidian vault.
			if cfg != nil && cfg.Obsidian.VaultPath != "" {
				writer, err := output.NewObsidianWriter(cfg.Obsidian.VaultPath, cfg.Obsidian.MeetingsFolder, "")
				if err == nil {
					path, err := writer.Write(note)
					if err != nil {
						log.Printf("warning: failed to write meeting note: %v", err)
						// Fallback: print summary to stdout so analysis is not lost.
						fmt.Printf("\n--- Meeting Summary ---\n%s\n", note.Summary)
						if len(note.ActionItems) > 0 {
							fmt.Println("\nAction Items:")
							for _, ai := range note.ActionItems {
								fmt.Printf("  - %s (owner: %s)\n", ai.Task, ai.Owner)
							}
						}
						fmt.Println("\nTo retry writing, use: heimdall recover")
					} else {
						fmt.Printf("Meeting note saved: %s\n", path)
						// V-006: clean up recovery file after successful write.
						if recWriter != nil {
							_ = recWriter.Cleanup()
						}
					}
				} else {
					log.Printf("warning: failed to create obsidian writer: %v", err)
					// Fallback: print summary to stdout so analysis is not lost.
					fmt.Printf("\n--- Meeting Summary ---\n%s\n", note.Summary)
					if len(note.ActionItems) > 0 {
						fmt.Println("\nAction Items:")
						for _, ai := range note.ActionItems {
							fmt.Printf("  - %s (owner: %s)\n", ai.Task, ai.Owner)
						}
					}
					fmt.Println("\nTo retry writing, use: heimdall recover")
				}
			} else {
				fmt.Println("Obsidian vault not configured -- summary displayed above only")
			}
		}
	}

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
