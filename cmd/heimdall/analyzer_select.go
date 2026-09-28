package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/analyzer"
	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// analyzerFlagUsage is the shared --analyzer help text for record, recover,
// analyze, and eval.
const analyzerFlagUsage = "meeting-analysis backend: api (default, needs ANTHROPIC_API_KEY), claude-code (local claude login), " +
	"ollama (fully on-device via a local Ollama server), or codex (local codex login). Defaults to claude.analyzer from config"

// resolveAnalyzerName returns the explicitly passed --analyzer flag value,
// or, when the flag was not set on the command line, claude.analyzer from
// config (the persistent default `heimdall config set claude.analyzer ...`
// writes), or the flag default. Every analysis command uses this so the
// config default applies uniformly -- previously only `record` honored it.
func resolveAnalyzerName(cmd *cobra.Command, flagValue string, cfg *config.Config) string {
	if cmd != nil && cmd.Flags().Changed("analyzer") {
		return flagValue
	}
	if cfg != nil && cfg.Claude.Analyzer != "" && !isUnresolvedRef(cfg.Claude.Analyzer) {
		return cfg.Claude.Analyzer
	}
	return flagValue
}

// validateAnalyzerName returns a user-facing error for an unknown backend.
func validateAnalyzerName(name string) error {
	if analyzer.IsValidProvider(name) {
		return nil
	}
	return fmt.Errorf("--analyzer %q is not valid: use one of %s", name, analyzer.ProviderList())
}

// analyzerSetup is a fully resolved analysis backend: the Analyzer plus the
// model, time budget, and human-readable label every command needs.
type analyzerSetup struct {
	Name     string
	Analyzer analyzer.Analyzer
	Model    string
	Timeout  time.Duration
	Label    string // e.g. "Ollama at http://localhost:11434 (on-device)"
	OnDevice bool   // true only when meeting content provably stays on this machine
}

// newAnalyzerSetup builds the selected backend from config. cfg may be nil
// (no config file); every field then falls back to the backend default.
func newAnalyzerSetup(name string, cfg *config.Config, anthropicKey string) (*analyzerSetup, error) {
	if err := validateAnalyzerName(name); err != nil {
		return nil, err
	}
	if name == "" {
		name = analyzer.ProviderAPI
	}

	settings := analyzer.Settings{APIKey: anthropicKey}
	if cfg != nil {
		settings.OllamaBaseURL = configValue(cfg.Ollama.BaseURL)
		settings.OllamaMaxContext = cfg.Ollama.MaxContext
	}

	a, err := analyzer.NewFromName(name, settings)
	if err != nil {
		return nil, err
	}

	setup := &analyzerSetup{
		Name:     name,
		Analyzer: a,
		Model:    resolveAnalyzerModel(name, cfg),
		Timeout:  analyzer.DefaultTimeoutFor(name),
	}

	switch name {
	case analyzer.ProviderAPI:
		setup.Label = "Claude (Anthropic API)"
	case analyzer.ProviderClaudeCode:
		setup.Label = "Claude Code (local login)"
	case analyzer.ProviderCodex:
		setup.Label = "Codex (local login)"
	case analyzer.ProviderOllama:
		baseURL := a.(*analyzer.OllamaAnalyzer).BaseURL()
		if analyzer.IsLoopbackURL(baseURL) {
			setup.Label = fmt.Sprintf("Ollama at %s (on-device -- the transcript does not leave this machine)", baseURL)
			setup.OnDevice = true
		} else {
			setup.Label = fmt.Sprintf("Ollama at %s (REMOTE host -- the transcript is sent to that server)", baseURL)
		}
	}
	return setup, nil
}

// resolveAnalyzerModel picks the model for a backend: its own config key,
// else its package default. The Anthropic backends read claude.model;
// claude-code keeps its long-standing behavior of passing no model at all
// (the user's own `claude` default) unless one is configured.
func resolveAnalyzerModel(name string, cfg *config.Config) string {
	var configured string
	if cfg != nil {
		switch name {
		case "", analyzer.ProviderAPI, analyzer.ProviderClaudeCode:
			configured = configValue(cfg.Claude.Model)
		case analyzer.ProviderOllama:
			configured = configValue(cfg.Ollama.Model)
		case analyzer.ProviderCodex:
			configured = configValue(cfg.Codex.Model)
		}
	}
	if configured != "" {
		return configured
	}
	return analyzer.DefaultModelFor(name)
}

// configValue returns v, or "" when v is a ${VAR} reference that could not
// be resolved (ResolveEnvVars leaves those in place and only logs).
func configValue(v string) string {
	if isUnresolvedRef(v) {
		return ""
	}
	return v
}

// fallbackReason extracts the error from a fallback note's summary, which
// buildFallbackNote formats as "<fallbackSummary> (error: <reason>)".
func fallbackReason(note *heimdall.MeetingNote) string {
	if note == nil {
		return ""
	}
	_, reason, ok := strings.Cut(note.Summary, "(error: ")
	if !ok {
		return ""
	}
	return strings.TrimSuffix(reason, ")")
}

func isUnresolvedRef(v string) bool {
	return strings.HasPrefix(v, "${")
}

// printFallbackGuidance explains a V-009 fallback note (analysis did not
// complete; raw transcript saved instead) with its actual reason, and how to
// retry from the kept transcript file. For an on-device backend the cloud
// retry is offered only as an explicit, user-initiated choice -- heimdall
// never uploads a meeting on its own after the user chose local analysis
// (.claude/DECISIONS.md ID-011).
func printFallbackGuidance(setup *analyzerSetup, note *heimdall.MeetingNote, transcriptPath string) {
	fmt.Printf("Warning: analysis via %s did not complete. The raw transcript was saved instead.\n", setup.Label)
	if reason := fallbackReason(note); reason != "" {
		fmt.Printf("Reason: %s\n", reason)
	}
	if transcriptPath == "" {
		return
	}
	fmt.Printf("The transcript is kept at: %s\n", transcriptPath)
	fmt.Printf("  Retry with the same backend:  heimdall analyze --file %s --analyzer %s\n", transcriptPath, setup.Name)
	if setup.Name != analyzer.ProviderAPI {
		if setup.OnDevice {
			fmt.Printf("  Or, only if you accept sending it to the cloud:  heimdall analyze --file %s --analyzer api\n", transcriptPath)
		} else {
			fmt.Printf("  Or with the Anthropic API:  heimdall analyze --file %s --analyzer api\n", transcriptPath)
		}
	}
}
