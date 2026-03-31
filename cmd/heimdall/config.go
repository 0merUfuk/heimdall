package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/0merUfuk/heimdall/internal/config"
)

// supportedLanguages lists language codes supported by Deepgram Nova-3.
// "multi" is a special value mapped to detect_language=true.
var supportedLanguages = map[string]string{
	"en":    "English",
	"tr":    "Turkish",
	"es":    "Spanish",
	"fr":    "French",
	"de":    "German",
	"it":    "Italian",
	"pt":    "Portuguese",
	"nl":    "Dutch",
	"ja":    "Japanese",
	"ko":    "Korean",
	"zh":    "Chinese",
	"ru":    "Russian",
	"hi":    "Hindi",
	"pl":    "Polish",
	"sv":    "Swedish",
	"da":    "Danish",
	"no":    "Norwegian",
	"fi":    "Finnish",
	"uk":    "Ukrainian",
	"id":    "Indonesian",
	"multi": "Auto-detect",
}

// knownClaudeModels lists Claude model identifiers known at build time.
var knownClaudeModels = map[string]bool{
	"claude-haiku-4-5":  true,
	"claude-sonnet-4-5": true,
	"claude-sonnet-4-6": true,
	"claude-opus-4-5":   true,
	"claude-opus-4-6":   true,
}

// validateLanguage checks if a language code is supported. Returns a warning
// message if unknown, empty string if valid.
func validateLanguage(lang string) string {
	if _, ok := supportedLanguages[lang]; ok {
		return ""
	}
	supported := make([]string, 0, len(supportedLanguages))
	for code, name := range supportedLanguages {
		supported = append(supported, fmt.Sprintf("%s (%s)", code, name))
	}
	return fmt.Sprintf("Warning: %q is not a known Deepgram language. Supported: %s",
		lang, strings.Join(supported, ", "))
}

// validateVaultPath checks if a vault path exists. Returns a warning if not.
func validateVaultPath(path string) string {
	expanded := config.ExpandHome(path)
	info, err := os.Stat(expanded)
	if err != nil {
		return fmt.Sprintf("Warning: vault path %q does not exist. Create it before recording.", expanded)
	}
	if !info.IsDir() {
		return fmt.Sprintf("Warning: vault path %q is not a directory.", expanded)
	}
	return ""
}

// validateClaudeModel checks if a model is known. Returns a warning if not.
func validateClaudeModel(model string) string {
	if knownClaudeModels[model] {
		return ""
	}
	return fmt.Sprintf("Warning: %q is not a recognized Claude model. Known models: %s",
		model, strings.Join(knownClaudeModelList(), ", "))
}

func knownClaudeModelList() []string {
	models := make([]string, 0, len(knownClaudeModels))
	for m := range knownClaudeModels {
		models = append(models, m)
	}
	return models
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage heimdall configuration",
	Long: `View and manage the heimdall configuration file (~/.heimdall/config.yaml).

Subcommands:
  init  Interactive first-run setup
  get   Get a configuration value
  set   Set a configuration value`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive first-run configuration setup",
	Long: `Walks through essential configuration values and writes them to
~/.heimdall/config.yaml. API keys are stored as environment variable
references (e.g., ${DEEPGRAM_API_KEY}), never as plaintext.`,
	RunE: runConfigInit,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Print the value of a configuration key. Use dotted paths for nested values.

Examples:
  heimdall config get obsidian.vault_path
  heimdall config get claude.model
  heimdall config get deepgram.language`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Update a single configuration value. Use dotted paths for nested values.

Examples:
  heimdall config set obsidian.vault_path ~/Documents/Obsidian/MyVault
  heimdall config set claude.model claude-sonnet-4-6
  heimdall config set deepgram.language tr`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	cfgPath := config.ConfigPath()

	// Check if config already exists.
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config file already exists at %s\n", cfgPath)
		fmt.Print("Overwrite? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	cfg := config.DefaultConfig()

	fmt.Println("heimdall config init -- setting up configuration")
	fmt.Println()

	// Vault path — validate existence.
	fmt.Print("Obsidian vault path (e.g., ~/Documents/Obsidian/MyVault): ")
	vaultPath, _ := reader.ReadString('\n')
	vaultPath = strings.TrimSpace(vaultPath)
	if vaultPath != "" {
		cfg.Obsidian.VaultPath = vaultPath
		if warn := validateVaultPath(vaultPath); warn != "" {
			fmt.Printf("  %s\n", warn)
		} else {
			absPath, _ := filepath.Abs(config.ExpandHome(vaultPath))
			fmt.Printf("  Vault found: %s\n", absPath)
		}
	}

	// Meetings folder within vault.
	fmt.Printf("Meetings folder within vault [%s]: ", cfg.Obsidian.MeetingsFolder)
	meetingsFolder, _ := reader.ReadString('\n')
	meetingsFolder = strings.TrimSpace(meetingsFolder)
	if meetingsFolder != "" {
		cfg.Obsidian.MeetingsFolder = meetingsFolder
	}

	// Claude model — validate against known models.
	fmt.Printf("Claude model [%s]: ", cfg.Claude.Model)
	claudeModel, _ := reader.ReadString('\n')
	claudeModel = strings.TrimSpace(claudeModel)
	if claudeModel != "" {
		cfg.Claude.Model = claudeModel
		if warn := validateClaudeModel(claudeModel); warn != "" {
			fmt.Printf("  %s\n", warn)
		}
	}

	// Deepgram language — validate against supported languages.
	fmt.Printf("Transcription language [%s]: ", cfg.Deepgram.Language)
	language, _ := reader.ReadString('\n')
	language = strings.TrimSpace(language)
	if language != "" {
		if warn := validateLanguage(language); warn != "" {
			fmt.Printf("  %s\n", warn)
			fmt.Print("  Use this language anyway? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Printf("  Keeping default: %s\n", cfg.Deepgram.Language)
			} else {
				cfg.Deepgram.Language = language
			}
		} else {
			cfg.Deepgram.Language = language
		}
	}

	// Save the config.
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConfiguration saved to %s\n", cfgPath)
	fmt.Println()
	fmt.Println("API keys are read from environment variables:")
	fmt.Println("  export DEEPGRAM_API_KEY=your_deepgram_key")
	fmt.Println("  export ANTHROPIC_API_KEY=your_anthropic_key")

	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Show the raw (unresolved) value first.
	rawVal, err := getConfigValue(cfg, key)
	if err != nil {
		return err
	}

	// Resolve env vars so the user sees the actual value.
	_ = cfg.ResolveEnvVars()
	resolvedVal, _ := getConfigValue(cfg, key)

	if rawVal != resolvedVal {
		fmt.Printf("%s (raw: %s)\n", resolvedVal, rawVal)
	} else {
		fmt.Println(resolvedVal)
	}
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Validate specific fields before saving.
	switch key {
	case "deepgram.api_key", "claude.api_key":
		if !strings.HasPrefix(value, "${") {
			fmt.Println("Warning: API keys should be stored as environment variable references.")
			fmt.Printf("  Recommended: heimdall config set %s '${ENV_VAR_NAME}'\n", key)
			fmt.Println("  The value you provided will be stored as-is in the config file.")
		}
	case "deepgram.language":
		if warn := validateLanguage(value); warn != "" {
			fmt.Printf("  %s\n", warn)
		}
	case "claude.model":
		if warn := validateClaudeModel(value); warn != "" {
			fmt.Printf("  %s\n", warn)
		}
	case "obsidian.vault_path":
		if warn := validateVaultPath(value); warn != "" {
			fmt.Printf("  %s\n", warn)
		}
	}

	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s = %s\n", key, value)
	return nil
}

// getConfigValue returns the string value for a dotted config key path.
func getConfigValue(cfg *config.Config, key string) (string, error) {
	// Marshal config to a generic map for dotted key access.
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("parsing config: %w", err)
	}

	parts := strings.SplitN(key, ".", 2)
	current := any(m)

	for i, part := range parts {
		asMap, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("key %q: %s is not a map", key, strings.Join(parts[:i], "."))
		}
		val, exists := asMap[part]
		if !exists {
			return "", fmt.Errorf("key %q not found", key)
		}
		current = val
	}

	return fmt.Sprintf("%v", current), nil
}

// setConfigValue sets a string value for a dotted config key path.
func setConfigValue(cfg *config.Config, key, value string) error {
	switch key {
	case "deepgram.api_key":
		cfg.Deepgram.APIKey = value
	case "deepgram.model":
		cfg.Deepgram.Model = value
	case "deepgram.language":
		cfg.Deepgram.Language = value
	case "claude.api_key":
		cfg.Claude.APIKey = value
	case "claude.model":
		cfg.Claude.Model = value
	case "obsidian.vault_path":
		cfg.Obsidian.VaultPath = value
	case "obsidian.meetings_folder":
		cfg.Obsidian.MeetingsFolder = value
	case "obsidian.template":
		cfg.Obsidian.Template = value
	case "audio.system_audio":
		cfg.Audio.SystemAudio = value == "true"
	case "audio.microphone":
		cfg.Audio.Microphone = value == "true"
	case "audio.save_recording":
		cfg.Audio.SaveRecording = value == "true"
	case "audio.recording_path":
		cfg.Audio.RecordingPath = value
	case "output.include_transcript":
		cfg.Output.IncludeTranscript = value == "true"
	case "output.include_timestamps":
		cfg.Output.IncludeTimestamps = value == "true"
	case "output.language":
		cfg.Output.Language = value
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}
	return nil
}
