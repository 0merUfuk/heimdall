package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/heimdall"
	"github.com/0merUfuk/heimdall/internal/output"
	"github.com/0merUfuk/heimdall/internal/recovery"
)

var recoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Recover and process unfinished meeting transcripts",
	Long: `Scans the recovery directory (~/.heimdall/recovery/) for orphaned
transcript files from crashed or interrupted sessions. Offers to
re-analyze them and write meeting notes to the Obsidian vault.

Use --analyzer claude-code or --analyzer codex to analyze via a local,
already-logged-in CLI instead of ANTHROPIC_API_KEY, or --analyzer ollama to
analyze fully on-device. Defaults to claude.analyzer from config.`,
	RunE: runRecover,
}

var analyzeFile string
var recoverAnalyzer string

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Re-analyze a transcript file",
	Long: `Re-analyze a recovery transcript file (from a crashed session, or
from 'heimdall transcribe') and write the meeting note to the Obsidian vault.

With 'heimdall transcribe' and --analyzer ollama the whole path is offline:
no audio, transcript, or analysis ever leaves this machine.

Example:
  heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json
  heimdall analyze --file <path> --analyzer ollama`,
	RunE: runAnalyze,
}

func init() {
	analyzeCmd.Flags().StringVar(&analyzeFile, "file", "", "path to recovery transcript JSON file (required)")
	_ = analyzeCmd.MarkFlagRequired("file")
	recoverCmd.Flags().StringVar(&recoverAnalyzer, "analyzer", "api", analyzerFlagUsage)
	analyzeCmd.Flags().StringVar(&recoverAnalyzer, "analyzer", "api", analyzerFlagUsage)
	rootCmd.AddCommand(recoverCmd)
	rootCmd.AddCommand(analyzeCmd)
}

func runRecover(cmd *cobra.Command, args []string) error {
	files, err := recovery.ListRecoveryFiles()
	if err != nil {
		return fmt.Errorf("scanning recovery directory: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No recovery files found.")
		return nil
	}

	fmt.Printf("Found %d recovery file(s):\n\n", len(files))

	for i, rf := range files {
		fmt.Printf("  %d. %s (%s, %d segments)\n",
			i+1,
			rf.Metadata.Title,
			rf.Metadata.StartTime.Format("2006-01-02 15:04"),
			len(rf.Segments),
		)
	}

	fmt.Println()

	cfg := loadResolvedConfig()
	name := resolveAnalyzerName(cmd, recoverAnalyzer, cfg)

	// Check if analysis is available for the selected backend.
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if analyzer.RequiresAPIKey(name) && anthropicKey == "" {
		fmt.Println("ANTHROPIC_API_KEY not set -- cannot re-analyze.")
		fmt.Println("Set the key and run 'heimdall recover' again, or use:")
		fmt.Println("  heimdall recover --analyzer claude-code   (reuse a local Claude Code login)")
		fmt.Println("  heimdall recover --analyzer ollama        (analyze fully on-device)")
		fmt.Println("  heimdall analyze --file <path>")
		return nil
	}

	setup, err := newAnalyzerSetup(name, cfg, anthropicKey)
	if err != nil {
		return err
	}

	// Process each recovery file.
	for _, rf := range files {
		fmt.Printf("\nProcessing: %s (%s)...\n", rf.Metadata.Title, rf.Metadata.StartTime.Format("2006-01-02 15:04"))

		if err := analyzeRecoveryFile(rf, setup, cfg); err != nil {
			log.Printf("warning: failed to process %s: %v", rf.Metadata.Title, err)
			continue
		}
	}

	return nil
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	rf, err := recovery.LoadRecoveryFile(analyzeFile)
	if err != nil {
		return fmt.Errorf("loading recovery file: %w", err)
	}

	fmt.Printf("Loaded: %s (%d segments)\n", rf.Metadata.Title, len(rf.Segments))

	cfg := loadResolvedConfig()
	name := resolveAnalyzerName(cmd, recoverAnalyzer, cfg)

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if analyzer.RequiresAPIKey(name) && anthropicKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set\n\n" +
			"Set it with:\n  export ANTHROPIC_API_KEY=your_key_here\n" +
			"or reuse a local Claude Code login:\n  heimdall analyze --file <path> --analyzer claude-code\n" +
			"or analyze fully on-device:\n  heimdall analyze --file <path> --analyzer ollama")
	}

	setup, err := newAnalyzerSetup(name, cfg, anthropicKey)
	if err != nil {
		return err
	}

	return analyzeRecoveryFile(*rf, setup, cfg)
}

// loadResolvedConfig loads the config file and expands ${VAR} references.
// A missing or malformed config is not fatal here: analysis falls back to
// backend defaults and the note is displayed instead of written.
func loadResolvedConfig() *config.Config {
	cfg, _ := config.Load(config.ConfigPath())
	if cfg != nil {
		if err := cfg.ResolveEnvVars(); err != nil {
			log.Printf("warning: resolving config env vars: %v", err)
		}
	}
	return cfg
}

// analyzeRecoveryFile runs analysis on a recovery file with the resolved
// backend and writes the meeting note to the Obsidian vault.
func analyzeRecoveryFile(rf recovery.RecoveryFile, setup *analyzerSetup, cfg *config.Config) error {
	if len(rf.Segments) == 0 {
		fmt.Println("  No segments to analyze -- skipping.")
		return nil
	}

	opts := heimdall.AnalyzeOpts{
		Model:    setup.Model,
		Language: rf.Metadata.Language,
	}

	ctx, cancel := context.WithTimeout(context.Background(), setup.Timeout)
	defer cancel()

	fmt.Printf("  Analyzing via %s...\n", setup.Label)
	note, err := setup.Analyzer.Summarize(ctx, rf.Segments, opts)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if note == nil {
		return fmt.Errorf("analyzer returned nil result")
	}

	// A fallback note (V-009) is the raw transcript, not an analysis: keep
	// the recovery file so the meeting can be re-analyzed later.
	if note.IsFallback {
		printFallbackGuidance(setup, note, rf.Path)
	}

	// Fill in metadata from recovery file.
	note.Title = rf.Metadata.Title
	note.Date = rf.Metadata.StartTime
	note.Platform = rf.Metadata.Platform

	// Calculate approximate duration from segments.
	if len(rf.Segments) > 0 {
		lastSeg := rf.Segments[len(rf.Segments)-1]
		note.Duration = lastSeg.End
	}

	// Write to Obsidian vault if configured.
	if cfg != nil && cfg.Obsidian.VaultPath != "" {
		writer, err := output.NewObsidianWriter(cfg.Obsidian.VaultPath, cfg.Obsidian.MeetingsFolder, "")
		if err != nil {
			return fmt.Errorf("creating obsidian writer: %w", err)
		}

		path, err := writer.Write(note)
		if err != nil {
			return fmt.Errorf("writing meeting note: %w", err)
		}

		fmt.Printf("  Meeting note saved: %s\n", path)

		// Delete the recovery file after successful write -- unless the
		// note is only a fallback and the transcript still needs analysis.
		if rf.Path != "" && !note.IsFallback {
			if err := os.Remove(rf.Path); err != nil && !os.IsNotExist(err) {
				log.Printf("warning: failed to delete recovery file %s: %v", rf.Path, err)
			}
		}
	} else {
		fmt.Println("  Obsidian vault not configured -- displaying summary only:")
		fmt.Printf("\n  Title: %s\n  Summary: %s\n", note.Title, note.Summary)
	}

	return nil
}
