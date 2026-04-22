package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
  init         Interactive first-run setup
  show         Pretty-print full configuration
  edit         Open config in $EDITOR
  path         Print config file location
  get          Get a configuration value
  set          Set a configuration value
  add-profile  Create or update a meeting profile
  profiles     List all configured profiles`,
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

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Pretty-print full configuration",
	Long:  `Display the full heimdall configuration with the config file path.`,
	RunE:  runConfigShow,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config file in $EDITOR",
	Long:  `Open the heimdall config file in your preferred editor ($EDITOR, default: nano).`,
	RunE:  runConfigEdit,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file location",
	Long:  `Print the absolute path to the heimdall config file.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.ConfigPath())
	},
}

var (
	profileTitle        string
	profileParticipants string
	profileKeywords     string
	profileLanguage     string
)

var configAddProfileCmd = &cobra.Command{
	Use:   "add-profile <name>",
	Short: "Create or update a meeting profile",
	Long: `Create or update a named meeting profile with default settings.

Profiles let you store per-meeting-type defaults (title, participants,
keywords, language) and activate them with: heimdall record --profile <name>

Examples:
  heimdall config add-profile daily --title "Daily Standup" --participants "Alice,Bob"
  heimdall config add-profile 1on1 --language tr --keywords "performance,goals"`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAddProfile,
}

var configProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List all configured profiles",
	Long:  `Display all meeting profiles and their settings.`,
	RunE:  runConfigProfiles,
}

func init() {
	configAddProfileCmd.Flags().StringVar(&profileTitle, "title", "", "default meeting title for this profile")
	configAddProfileCmd.Flags().StringVar(&profileParticipants, "participants", "", "comma-separated participant names")
	configAddProfileCmd.Flags().StringVar(&profileKeywords, "keywords", "", "comma-separated context keywords")
	configAddProfileCmd.Flags().StringVar(&profileLanguage, "language", "", "transcription language code")

	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configAddProfileCmd)
	configCmd.AddCommand(configProfilesCmd)
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

	// Offer to create a profile.
	fmt.Println()
	fmt.Print("Create a meeting profile? (e.g., 'daily', '1on1') [name or empty to skip]: ")
	profileName, _ := reader.ReadString('\n')
	profileName = strings.TrimSpace(profileName)
	if profileName != "" {
		if cfg.Profiles == nil {
			cfg.Profiles = make(map[string]config.Profile)
		}
		profile := config.Profile{}

		fmt.Printf("  Default title for %q [empty to skip]: ", profileName)
		title, _ := reader.ReadString('\n')
		title = strings.TrimSpace(title)
		if title != "" {
			profile.Title = title
		}

		fmt.Printf("  Participants (comma-separated) [empty to skip]: ")
		parts, _ := reader.ReadString('\n')
		parts = strings.TrimSpace(parts)
		if parts != "" {
			profile.Participants = splitAndTrim(parts)
		}

		fmt.Printf("  Keywords (comma-separated) [empty to skip]: ")
		kws, _ := reader.ReadString('\n')
		kws = strings.TrimSpace(kws)
		if kws != "" {
			profile.Keywords = splitAndTrim(kws)
		}

		cfg.Profiles[profileName] = profile
		fmt.Printf("  Profile %q added.\n", profileName)
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

	// SEC-01: never print an API key in cleartext. rawVal may be a ${VAR}
	// placeholder (maskSecret preserves those) and resolvedVal is the
	// expanded secret from the environment (must be masked).
	if isSecretKey(key) {
		rawVal = maskSecret(rawVal)
		resolvedVal = maskSecret(resolvedVal)
	}

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
	case "deepgram.api_key", "soniox.api_key", "claude.api_key":
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

	// SEC-01: the success echo must not leak an API key into shell history
	// or CI logs. maskSecret preserves ${VAR} placeholders verbatim.
	displayValue := value
	if isSecretKey(key) {
		displayValue = maskSecret(value)
	}
	fmt.Printf("%s = %s\n", key, displayValue)
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
	case "soniox.api_key":
		cfg.Soniox.APIKey = value
	case "soniox.model":
		cfg.Soniox.Model = value
	case "soniox.language":
		cfg.Soniox.Language = value
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
	case "keywords":
		cfg.Keywords = splitAndTrim(value)
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}
	return nil
}

// splitAndTrim splits a comma-separated string and trims whitespace from each item.
// Empty items are discarded.
func splitAndTrim(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// SEC-02: mask API keys before dumping YAML. ${VAR} placeholders pass
	// through unchanged (they are not secrets). We mutate a shallow copy
	// so the loaded cfg is untouched for any subsequent use.
	display := *cfg
	display.Deepgram.APIKey = maskSecret(display.Deepgram.APIKey)
	display.Soniox.APIKey = maskSecret(display.Soniox.APIKey)
	display.Claude.APIKey = maskSecret(display.Claude.APIKey)

	data, err := yaml.Marshal(&display)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	fmt.Printf("# heimdall configuration (%s)\n", cfgPath)
	fmt.Println("---")
	fmt.Print(string(data))
	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	cfgPath := config.ConfigPath()

	// Ensure the config file exists before opening the editor.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := cfg.Save(cfgPath); err != nil {
			return fmt.Errorf("creating default config: %w", err)
		}
		fmt.Printf("Created default config at %s\n", cfgPath)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	editorCmd := exec.Command(editor, cfgPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("running editor %q: %w", editor, err)
	}
	return nil
}

func runConfigAddProfile(cmd *cobra.Command, args []string) error {
	name := args[0]
	cfgPath := config.ConfigPath()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.Profile)
	}

	profile := cfg.Profiles[name]

	if profileTitle != "" {
		profile.Title = profileTitle
	}
	if profileParticipants != "" {
		profile.Participants = splitAndTrim(profileParticipants)
	}
	if profileKeywords != "" {
		profile.Keywords = splitAndTrim(profileKeywords)
	}
	if profileLanguage != "" {
		if warn := validateLanguage(profileLanguage); warn != "" {
			fmt.Printf("  %s\n", warn)
		}
		profile.Language = profileLanguage
	}

	cfg.Profiles[name] = profile

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Profile %q saved.\n", name)
	printProfile(name, profile)
	return nil
}

func runConfigProfiles(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles configured.")
		fmt.Println("Create one with: heimdall config add-profile <name> --title \"...\"")
		return nil
	}

	// Sort profile names for stable output.
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("%d profile(s):\n\n", len(names))
	for _, name := range names {
		printProfile(name, cfg.Profiles[name])
		fmt.Println()
	}
	return nil
}

// printProfile prints a single profile's settings.
func printProfile(name string, p config.Profile) {
	fmt.Printf("  [%s]\n", name)
	if p.Title != "" {
		fmt.Printf("    title:        %s\n", p.Title)
	}
	if len(p.Participants) > 0 {
		fmt.Printf("    participants: %s\n", strings.Join(p.Participants, ", "))
	}
	if len(p.Keywords) > 0 {
		fmt.Printf("    keywords:     %s\n", strings.Join(p.Keywords, ", "))
	}
	if p.Language != "" {
		fmt.Printf("    language:     %s\n", p.Language)
	}
}
