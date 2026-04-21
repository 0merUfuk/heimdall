package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/audio"
	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/consent"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/output"
	"github.com/0merUfuk/heimdall/internal/recovery"
	"github.com/0merUfuk/heimdall/internal/session"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

var (
	recordTitle               string
	recordLanguage            string
	recordParticipants        string
	recordKeywords            string
	recordProfile             string
	recordTranscriber         string
	recordConsentAcknowledged bool
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a meeting with live transcription",
	Long: `Start recording a meeting. Captures system audio and microphone,
streams to the configured transcriber (Deepgram by default, Soniox opt-in
via --transcriber soniox) for real-time transcription with speaker
diarization, and displays the live transcript in the terminal.

Press Ctrl+C to stop recording.

Requires the selected transcriber's API key:
  - DEEPGRAM_API_KEY (default)
  - SONIOX_API_KEY   (with --transcriber soniox)`,
	Example: `  heimdall record                                 # auto-title, config defaults
  heimdall record --profile daily                  # use "daily" profile
  heimdall record --title "Sprint Planning"        # custom title
  heimdall record --profile 1on1 --title "Special" # profile + override`,
	RunE: runRecord,
}

func init() {
	recordCmd.Flags().StringVar(&recordTitle, "title", "", "meeting title (auto-generated if omitted)")
	recordCmd.Flags().StringVar(&recordLanguage, "language", "en", "transcription language code (e.g., en, tr, multi)")
	recordCmd.Flags().StringVar(&recordParticipants, "participants", "", "comma-separated list of participant names (hints for speaker identification)")
	recordCmd.Flags().StringVar(&recordKeywords, "keywords", "", "comma-separated list of context keywords for analysis (Deepgram only; ignored for Soniox)")
	recordCmd.Flags().StringVar(&recordProfile, "profile", "", "meeting profile name (from config)")
	recordCmd.Flags().StringVar(&recordTranscriber, "transcriber", "deepgram", "transcription provider: deepgram (default) or soniox")
	recordCmd.Flags().BoolVar(&recordConsentAcknowledged, "consent-acknowledged", false,
		"acknowledge the recording-consent banner non-interactively (for scripts/CI; does not persist to config)")
	rootCmd.AddCommand(recordCmd)
}

func runRecord(cmd *cobra.Command, args []string) error {
	// Validate --transcriber up front so an unknown value fails fast with a
	// clear message before we open audio devices or prompt for consent.
	switch recordTranscriber {
	case "", transcriber.ProviderDeepgram, transcriber.ProviderSoniox:
	default:
		return fmt.Errorf("--transcriber %q is not valid: use %q (default) or %q",
			recordTranscriber, transcriber.ProviderDeepgram, transcriber.ProviderSoniox)
	}

	// Provider-specific API-key preflight. We do NOT require DEEPGRAM_API_KEY
	// when --transcriber=soniox is in use, or vice-versa.
	var apiKey, sonioxAPIKey string
	if recordTranscriber == transcriber.ProviderSoniox {
		sonioxAPIKey = os.Getenv("SONIOX_API_KEY")
		if sonioxAPIKey == "" {
			return fmt.Errorf("SONIOX_API_KEY environment variable is not set\n\n" +
				"Set it with:\n  export SONIOX_API_KEY=your_key_here\n\n" +
				"Get an API key at: https://soniox.com/")
		}
	} else {
		apiKey = os.Getenv("DEEPGRAM_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("DEEPGRAM_API_KEY environment variable is not set\n\n" +
				"Set it with:\n  export DEEPGRAM_API_KEY=your_key_here\n\n" +
				"Get a free API key at: https://console.deepgram.com/")
		}
	}

	// Create a context that listens for interrupt signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Pre-validate config and Obsidian vault before starting the recording.
	// Better to fail now than after a 1-hour meeting.
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		log.Printf("warning: config file is malformed: %v", err)
		fmt.Printf("Fix by editing %s or resetting with: heimdall config init\n", config.ConfigPath())
	}

	// First-run consent gate (audit §6 item 4, AD-011 Option A). Runs before
	// any audio source is opened or any API call is made so that a refusal
	// never leaves hardware resources or network connections half-initialised.
	// The gate uses the UNRESOLVED cfg so the Consent.Acknowledged bool read
	// reflects the raw config bytes, not an env-expanded clone.
	if err := consent.Gate(ctx, consent.Options{
		FlagAcknowledged: recordConsentAcknowledged,
		ConfigPath:       config.ConfigPath(),
		Cfg:              cfg,
		PromptReader:     os.Stdin,
		PromptWriter:     os.Stderr,
		IsTerminalFn:     func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
	}); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "\nConsent cancelled. No recording started.")
			return nil
		}
		return err
	}

	if cfg != nil {
		if err := cfg.ResolveEnvVars(); err != nil {
			log.Printf("warning: resolving config env vars: %v", err)
		}
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

	// Apply profile settings if specified.
	var participants []string
	var keywords []string

	if recordProfile != "" && cfg != nil && cfg.Profiles != nil {
		if profile, ok := cfg.Profiles[recordProfile]; ok {
			if recordTitle == "" && profile.Title != "" {
				recordTitle = profile.Title
			}
			if !cmd.Flags().Changed("language") && profile.Language != "" {
				recordLanguage = profile.Language
			}
			if recordParticipants == "" && len(profile.Participants) > 0 {
				participants = profile.Participants
			}
			if recordKeywords == "" && len(profile.Keywords) > 0 {
				keywords = profile.Keywords
			}
		} else {
			names := make([]string, 0)
			for k := range cfg.Profiles {
				names = append(names, k)
			}
			fmt.Printf("Warning: profile %q not found. Available: %s\n", recordProfile, strings.Join(names, ", "))
		}
	}

	// Config language as default when flag not changed. Scope the fallback
	// to the active provider so a user with `deepgram.language: de` in
	// config running `heimdall record --transcriber soniox` does not end
	// up with a German Soniox session (and vice versa).
	if !cmd.Flags().Changed("language") && cfg != nil && recordLanguage == "en" {
		switch recordTranscriber {
		case transcriber.ProviderSoniox:
			if cfg.Soniox.Language != "" {
				recordLanguage = cfg.Soniox.Language
			}
		default:
			if cfg.Deepgram.Language != "" {
				recordLanguage = cfg.Deepgram.Language
			}
		}
	}

	// Auto-generate title if still empty.
	if recordTitle == "" {
		recordTitle = time.Now().Format("Meeting 2006-01-02 15:04")
	}

	// Parse CLI participants if flag was provided (overrides profile).
	if recordParticipants != "" {
		participants = nil
		for _, p := range strings.Split(recordParticipants, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				participants = append(participants, trimmed)
			}
		}
	}

	// Parse CLI keywords if flag was provided (overrides profile).
	if recordKeywords != "" {
		keywords = nil
		for _, k := range strings.Split(recordKeywords, ",") {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				keywords = append(keywords, trimmed)
			}
		}
	}

	// Fall back to global config keywords if no keywords from profile or CLI.
	if len(keywords) == 0 && cfg != nil && len(cfg.Keywords) > 0 {
		keywords = cfg.Keywords
	}

	// Print header.
	fmt.Printf("heimdall %s -- recording \"%s\"\n", version, recordTitle)
	fmt.Println("Warning: Recording active -- ensure all participants have consented to recording.")

	// Stage 1: Create audio sources.
	var systemSource audio.AudioSource
	systemSource = audio.NewSystemAudioSource("")
	micSource := audio.NewMicrophoneSource()

	// Stage 3: Create the transcriber via the factory. Deepgram is the
	// default path; Soniox is opt-in. The factory resolves unknown names
	// to a clean error — we've already validated the flag above, so this
	// call is the single-source-of-truth provider constructor.
	//
	// Construct config shims from the env-var-preflighted API keys above.
	// `cfg` (loaded from ~/.heimdall/config.yaml) provides the non-secret
	// fields (model, language) as overrides when set; env-derived keys win
	// for the secret itself so users who export the shell variable see the
	// expected behaviour without running `heimdall config set`.
	dgCfg := config.DeepgramConfig{APIKey: apiKey}
	snxCfg := config.SonioxConfig{APIKey: sonioxAPIKey}
	if cfg != nil {
		if cfg.Deepgram.Model != "" {
			dgCfg.Model = cfg.Deepgram.Model
		}
		if cfg.Deepgram.Language != "" {
			dgCfg.Language = cfg.Deepgram.Language
		}
		if cfg.Soniox.Model != "" {
			snxCfg.Model = cfg.Soniox.Model
		}
		if cfg.Soniox.Language != "" {
			snxCfg.Language = cfg.Soniox.Language
		}
	}

	sttProvider, err := transcriber.NewFromName(recordTranscriber, dgCfg, snxCfg)
	if err != nil {
		return err
	}

	// Create the session orchestrator (wires stages 1-4).
	sess := session.NewMeetingSession(recordTitle, systemSource, micSource, sttProvider, recordLanguage, keywords)

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
	sttName := recordTranscriber
	if sttName == "" {
		sttName = transcriber.ProviderDeepgram
	}
	fmt.Printf("Audio: system %s  mic on  | STT: %s (connected) | recording...\n", sysStatus, sttName)
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
