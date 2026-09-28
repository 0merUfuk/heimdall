package main

import (
	"fmt"
	"strings"

	"github.com/0merUfuk/heimdall/internal/config"
	"github.com/0merUfuk/heimdall/internal/transcriber"
)

// transcriberProviders lists the accepted --transcriber / transcriber.provider
// values in preference order. Soniox is first: it is the default when its key
// is present (AD-012), Deepgram is the fallback, Whisper is the offline mode.
var transcriberProviders = []string{
	transcriber.ProviderSoniox,
	transcriber.ProviderDeepgram,
	transcriber.ProviderWhisper,
}

func validTranscriberProvider(name string) bool {
	for _, p := range transcriberProviders {
		if p == name {
			return true
		}
	}
	return false
}

func transcriberProviderList() string {
	quoted := make([]string, len(transcriberProviders))
	for i, p := range transcriberProviders {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(quoted, ", ")
}

// transcriberChoice is a resolved provider plus a one-line explanation of why
// it was picked, printed at record start so a silent fallback is never a
// surprise.
type transcriberChoice struct {
	Name string
	Why  string
}

// resolveTranscriber picks the speech-to-text provider for `record`.
//
// The flag defaults to "", so a non-empty value is always explicit.
//
// Precedence: an explicit --transcriber flag, then transcriber.provider from
// config, then auto -- Soniox when SONIOX_API_KEY is set, else Deepgram when
// DEEPGRAM_API_KEY is set. An explicit choice is never second-guessed: if its
// key is missing the caller reports that, instead of quietly switching to a
// different provider (which would send the meeting audio somewhere the user
// did not ask for). Only the auto path falls back.
func resolveTranscriber(flagValue string, cfg *config.Config, getenv func(string) string) (transcriberChoice, error) {
	if flagValue != "" {
		if !validTranscriberProvider(flagValue) {
			return transcriberChoice{}, fmt.Errorf("--transcriber %q is not valid: use one of %s", flagValue, transcriberProviderList())
		}
		return transcriberChoice{Name: flagValue, Why: "--transcriber flag"}, nil
	}
	if cfg != nil && cfg.Transcriber.Provider != "" && !isUnresolvedRef(cfg.Transcriber.Provider) {
		if !validTranscriberProvider(cfg.Transcriber.Provider) {
			return transcriberChoice{}, fmt.Errorf("transcriber.provider in %s: %q is not valid: use one of %s",
				config.ConfigPath(), cfg.Transcriber.Provider, transcriberProviderList())
		}
		return transcriberChoice{Name: cfg.Transcriber.Provider, Why: "transcriber.provider in config"}, nil
	}
	if getenv("SONIOX_API_KEY") != "" {
		return transcriberChoice{Name: transcriber.ProviderSoniox, Why: "auto: SONIOX_API_KEY is set"}, nil
	}
	if getenv("DEEPGRAM_API_KEY") != "" {
		return transcriberChoice{Name: transcriber.ProviderDeepgram, Why: "auto: SONIOX_API_KEY is not set, falling back to Deepgram"}, nil
	}
	return transcriberChoice{}, fmt.Errorf("no speech-to-text provider is configured\n\n" +
		"Pick one:\n" +
		"  export SONIOX_API_KEY=your_key      # recommended (https://soniox.com/)\n" +
		"  export DEEPGRAM_API_KEY=your_key    # alternative (https://console.deepgram.com/)\n" +
		"  heimdall record --transcriber whisper   # offline: no key, transcribed on this machine after the meeting")
}
