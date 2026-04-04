// Package config provides configuration loading, saving, and validation
// for heimdall. Configuration is stored at ~/.heimdall/config.yaml with
// API keys referenced as environment variables (never plaintext).
package config

// DefaultConfig returns a Config populated with sensible defaults.
// API keys are stored as environment variable references that must be
// resolved via ResolveEnvVars before use.
//
// Participants and Keywords are intentionally empty in defaults.
// Set via 'heimdall config init' or 'heimdall config set participants "Name1,Name2"'.
func DefaultConfig() *Config {
	return &Config{
		Deepgram: DeepgramConfig{
			APIKey:   "${DEEPGRAM_API_KEY}",
			Model:    "nova-3",
			Language: "en",
		},
		Claude: ClaudeConfig{
			APIKey: "${ANTHROPIC_API_KEY}",
			Model:  "claude-haiku-4-5",
		},
		Obsidian: ObsidianConfig{
			MeetingsFolder: "meetings",
			Template:       "default",
		},
		Audio: AudioConfig{
			SystemAudio:   true,
			Microphone:    true,
			SaveRecording: false,
			RecordingPath: "~/.heimdall/recordings/",
		},
		Output: OutputConfig{
			IncludeTranscript: true,
			IncludeTimestamps: true,
			Language:          "en",
		},
	}
}
