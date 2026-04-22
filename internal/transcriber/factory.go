package transcriber

import (
	"fmt"

	"github.com/0merUfuk/heimdall/internal/config"
)

// Provider names accepted by NewFromName. Kept as constants so the record
// command's --transcriber flag validation and error messages can reference
// the canonical spellings without string duplication.
const (
	ProviderDeepgram = "deepgram"
	ProviderSoniox   = "soniox"
)

// NewFromName constructs a Transcriber by provider name. Deepgram remains
// the default path; Soniox is opt-in per AD-011 (Option A strategic pivot
// for TR+EN code-switched transcription).
//
// Both configs are passed positionally so the factory never imports
// unrelated state — the caller is responsible for ensuring env-var
// references have been resolved before passing the config in. For the
// Soniox path, an empty APIKey is a hard error so the operator gets a
// clear "configure SONIOX_API_KEY first" message instead of an ambiguous
// WebSocket handshake failure mid-recording.
func NewFromName(name string, dgCfg config.DeepgramConfig, snxCfg config.SonioxConfig) (Transcriber, error) {
	switch name {
	case "", ProviderDeepgram:
		return NewDeepgramTranscriber(dgCfg.APIKey), nil
	case ProviderSoniox:
		if snxCfg.APIKey == "" {
			return nil, fmt.Errorf("soniox: api key is not configured. Set SONIOX_API_KEY or run 'heimdall config set soniox.api_key ${SONIOX_API_KEY}'")
		}
		return NewSonioxTranscriber(sonioxSessionConfig{
			APIKey:   snxCfg.APIKey,
			Model:    snxCfg.Model,
			Language: snxCfg.Language,
		}), nil
	default:
		return nil, fmt.Errorf("unknown transcriber %q: valid options are %q, %q", name, ProviderDeepgram, ProviderSoniox)
	}
}
