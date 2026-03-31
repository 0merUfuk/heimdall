package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/config"
)

var listSince string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List past meeting notes",
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVar(&listSince, "since", "", "filter meetings since date (YYYY-MM-DD)")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Obsidian.VaultPath == "" {
		return fmt.Errorf("obsidian vault_path not configured\n\nSet it with:\n  heimdall config set obsidian.vault_path /path/to/vault")
	}

	vaultPath := config.ExpandHome(cfg.Obsidian.VaultPath)

	// Validate --since format before walking.
	if listSince != "" {
		if _, err := time.Parse("2006-01-02", listSince); err != nil {
			return fmt.Errorf("invalid --since format: %q (expected YYYY-MM-DD)", listSince)
		}
	}

	meetingsDir := filepath.Join(vaultPath, cfg.Obsidian.MeetingsFolder)
	if _, err := os.Stat(meetingsDir); os.IsNotExist(err) {
		fmt.Println("No meetings found.")
		return nil
	}

	// Walk the meetings directory.
	type meeting struct {
		date string
		name string
		path string
	}

	var meetings []meeting

	err = filepath.Walk(meetingsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		rel, _ := filepath.Rel(meetingsDir, path)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		date := ""
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if len(parts) >= 2 {
			date = parts[0]
		}

		if listSince != "" && date < listSince {
			return nil
		}

		meetings = append(meetings, meeting{date: date, name: name, path: rel})
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan meetings: %w", err)
	}

	if len(meetings) == 0 {
		fmt.Println("No meetings found.")
		return nil
	}

	// Sort by date descending.
	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].date > meetings[j].date
	})

	// Print table.
	fmt.Printf("%-12s %-40s\n", "Date", "Title")
	fmt.Printf("%-12s %-40s\n", "----", "-----")
	for _, m := range meetings {
		fmt.Printf("%-12s %-40s\n", m.date, m.name)
	}
	fmt.Printf("\n%d meeting(s) found.\n", len(meetings))

	return nil
}
