package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/0merUfuk/heimdall/internal/config"
)

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

	// Vault path.
	fmt.Print("Obsidian vault path (e.g., ~/Documents/Obsidian/MyVault): ")
	vaultPath, _ := reader.ReadString('\n')
	vaultPath = strings.TrimSpace(vaultPath)
	if vaultPath != "" {
		cfg.Obsidian.VaultPath = vaultPath
	}

	// Meetings folder within vault.
	fmt.Printf("Meetings folder within vault [%s]: ", cfg.Obsidian.MeetingsFolder)
	meetingsFolder, _ := reader.ReadString('\n')
	meetingsFolder = strings.TrimSpace(meetingsFolder)
	if meetingsFolder != "" {
		cfg.Obsidian.MeetingsFolder = meetingsFolder
	}

	// Claude model.
	fmt.Printf("Claude model [%s]: ", cfg.Claude.Model)
	claudeModel, _ := reader.ReadString('\n')
	claudeModel = strings.TrimSpace(claudeModel)
	if claudeModel != "" {
		cfg.Claude.Model = claudeModel
	}

	// Deepgram language.
	fmt.Printf("Transcription language [%s]: ", cfg.Deepgram.Language)
	language, _ := reader.ReadString('\n')
	language = strings.TrimSpace(language)
	if language != "" {
		cfg.Deepgram.Language = language
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

	val, err := getConfigValue(cfg, key)
	if err != nil {
		return err
	}

	fmt.Println(val)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

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

	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("parsing config: %w", err)
	}

	parts := strings.SplitN(key, ".", 2)
	current := interface{}(m)

	for i, part := range parts {
		asMap, ok := current.(map[string]interface{})
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
