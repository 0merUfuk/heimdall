package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

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
re-analyze them via Claude and write meeting notes to the Obsidian vault.`,
	RunE: runRecover,
}

var analyzeFile string

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Re-analyze a transcript file",
	Long: `Re-analyze a recovery transcript file via Claude and write the
meeting note to the Obsidian vault.

Example:
  heimdall analyze --file ~/.heimdall/recovery/2026-03-28T14-30-00-sprint-planning.json`,
	RunE: runAnalyze,
}

func init() {
	analyzeCmd.Flags().StringVar(&analyzeFile, "file", "", "path to recovery transcript JSON file (required)")
	_ = analyzeCmd.MarkFlagRequired("file")
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

	// Check if Claude analysis is available.
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		fmt.Println("ANTHROPIC_API_KEY not set -- cannot re-analyze.")
		fmt.Println("Set the key and run 'heimdall recover' again, or use:")
		fmt.Println("  heimdall analyze --file <path>")
		return nil
	}

	// Process each recovery file.
	cfg, _ := config.Load(config.ConfigPath())
	if cfg != nil {
		if err := cfg.ResolveEnvVars(); err != nil {
			log.Printf("warning: resolving config env vars: %v", err)
		}
	}

	for _, rf := range files {
		fmt.Printf("\nProcessing: %s (%s)...\n", rf.Metadata.Title, rf.Metadata.StartTime.Format("2006-01-02 15:04"))

		if err := analyzeRecoveryFile(rf, anthropicKey, cfg); err != nil {
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

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set\n\n" +
			"Set it with:\n  export ANTHROPIC_API_KEY=your_key_here")
	}

	cfg, _ := config.Load(config.ConfigPath())
	if cfg != nil {
		if err := cfg.ResolveEnvVars(); err != nil {
			log.Printf("warning: resolving config env vars: %v", err)
		}
	}

	return analyzeRecoveryFile(*rf, anthropicKey, cfg)
}

// analyzeRecoveryFile runs Claude analysis on a recovery file and writes the
// meeting note to the Obsidian vault.
func analyzeRecoveryFile(rf recovery.RecoveryFile, anthropicKey string, cfg *config.Config) error {
	if len(rf.Segments) == 0 {
		fmt.Println("  No segments to analyze -- skipping.")
		return nil
	}

	// Determine the Claude model to use.
	model := analyzer.DefaultModel
	if cfg != nil && cfg.Claude.Model != "" && !strings.HasPrefix(cfg.Claude.Model, "${") {
		model = cfg.Claude.Model
	}

	claude := analyzer.NewClaudeAnalyzer(anthropicKey)
	opts := heimdall.AnalyzeOpts{
		Model:    model,
		Language: rf.Metadata.Language,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("  Analyzing via Claude...")
	note, err := claude.Summarize(ctx, rf.Segments, opts)
	if err != nil {
		return fmt.Errorf("Claude analysis failed: %w", err)
	}

	if note == nil {
		return fmt.Errorf("Claude returned nil result")
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

		// Delete the recovery file after successful write.
		if rf.Path != "" {
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
