// Package localstt implements local, offline speech-to-text via whisper.cpp
// (the `whisper-cli` binary), operating in BATCH mode on a saved audio file
// -- not the streaming internal/transcriber.Transcriber interface that
// Deepgram/Soniox implement.
//
// This is a deliberate architectural choice, not an oversight: a batch
// transcription pass over a full meeting can take much longer than
// internal/session's 30-second shutdown budget (sized for cloud STT's
// "send CloseStream, wait briefly for final results"), so forcing a batch
// operation through that interface risked silently truncating or emptying
// the transcript on any real meeting -- the same failure class as this
// project's own historical Bug #1 (empty transcription from a diarize/
// multichannel conflict). Local transcription is instead its own command
// (`heimdall transcribe`) operating on a WAV file saved via --save-audio,
// decoupled entirely from the live recording's real-time constraints. See
// .claude/DECISIONS.md ID-007 and ID-008 for the full rationale.
package localstt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// whisperBinDefault is the whisper.cpp CLI binary name, resolved via PATH.
// Homebrew's formula was renamed from "whisper-cpp" to "whisper.cpp" but
// still installs this same binary name -- verified empirically against
// whisper.cpp 1.9.4.
const whisperBinDefault = "whisper-cli"

// Valid model size names, matching the ggml model files published at
// https://huggingface.co/ggerganov/whisper.cpp.
const (
	ModelTiny   = "tiny"
	ModelBase   = "base"
	ModelSmall  = "small"
	ModelMedium = "medium"
	ModelLarge  = "large"
)

var validModelNames = map[string]bool{
	ModelTiny: true, ModelBase: true, ModelSmall: true, ModelMedium: true, ModelLarge: true,
}

// ModelDir returns ~/.heimdall/models/, where downloaded ggml model files
// live.
func ModelDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".heimdall", "models")
	}
	return filepath.Join(home, ".heimdall", "models")
}

// ResolveModelPath turns a bare model size name (tiny/base/small/medium/
// large) into its expected path under ModelDir(). Anything that looks like
// a path already (contains a path separator, or ends in .bin) is returned
// unchanged, so `--model /custom/path/model.bin` works too.
func ResolveModelPath(nameOrPath string) string {
	if strings.ContainsRune(nameOrPath, filepath.Separator) || strings.HasSuffix(nameOrPath, ".bin") {
		return nameOrPath
	}
	return filepath.Join(ModelDir(), "ggml-"+nameOrPath+".bin")
}

// ModelDownloadURL returns the Hugging Face URL for a given model size
// name. Returns an error for anything that isn't one of the five published
// sizes -- this is a small, fixed set, not a place to guess at a URL.
func ModelDownloadURL(name string) (string, error) {
	if !validModelNames[name] {
		return "", fmt.Errorf("unknown model %q: valid sizes are tiny, base, small, medium, large", name)
	}
	return fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", name), nil
}

// commandRunner abstracts subprocess execution for testability, mirroring
// internal/analyzer's ClaudeCodeAnalyzer pattern.
type commandRunner func(ctx context.Context, name string, args []string) error

// execCommandRunner is the production commandRunner.
//
// stdout is intentionally discarded: whisper-cli prints a preview
// transcript to stdout regardless of --no-prints (verified empirically
// against 1.9.4) -- the authoritative output is the JSON file written via
// --output-file, which Client.TranscribeFile reads directly rather than
// parsing subprocess output.
func execCommandRunner(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w (stderr: %s)", err, truncateForError(stderr.String()))
	}
	return err
}

// Client transcribes audio files via whisper-cli.
type Client struct {
	binPath string
	runner  commandRunner
}

// NewClient creates a Client that invokes whisper-cli resolved via PATH.
func NewClient() *Client {
	return &Client{binPath: whisperBinDefault, runner: execCommandRunner}
}

// WithBinPath overrides the whisper-cli binary path.
func (c *Client) WithBinPath(path string) *Client {
	c.binPath = path
	return c
}

// WithRunner overrides the subprocess runner. Used in tests.
func (c *Client) WithRunner(r commandRunner) *Client {
	c.runner = r
	return c
}

// Options configures one TranscribeFile call.
type Options struct {
	// ModelPath is the resolved path to a ggml .bin model file. Use
	// ResolveModelPath to turn a size name into a path. Required.
	ModelPath string

	// Language is a heimdall language code (e.g. "en", "tr"). Empty,
	// "multi", or "auto" all map to whisper's own "auto" (language
	// auto-detect) -- "multi" is heimdall's Deepgram-derived vocabulary for
	// the same concept.
	Language string
}

// TranscribeFile runs whisper-cli against a WAV file and returns the
// resulting segments. wavPath is expected to be the stereo WAV
// internal/recording produces (L=system, R=mic): TranscribeFile always
// passes --diarize, which whisper.cpp uses to tag each segment with a "0"/
// "1" speaker based on relative per-channel energy during that segment --
// real, if coarse, 2-party speaker separation, verified empirically against
// whisper.cpp 1.9.4's actual --output-json output (see .claude/DECISIONS.md
// ID-008): {"transcription": [{"offsets": {"from": ms, "to": ms}, "text":
// "...", "speaker": "0"}, ...]}.
//
// whisper-cli returns exit code 0 even when it fails outright (e.g. the
// model file doesn't exist) -- also verified empirically, so exit code is
// NOT used as the success signal. The only reliable signal is whether the
// output JSON file was actually written; that's what this checks.
func (c *Client) TranscribeFile(ctx context.Context, wavPath string, opts Options) ([]heimdall.Segment, error) {
	if opts.ModelPath == "" {
		return nil, fmt.Errorf("localstt: a model path is required")
	}
	if _, err := os.Stat(opts.ModelPath); err != nil {
		return nil, fmt.Errorf("whisper model not found at %s -- run 'heimdall model download <size>' first", opts.ModelPath)
	}
	if _, err := os.Stat(wavPath); err != nil {
		return nil, fmt.Errorf("audio file not found at %s: %w", wavPath, err)
	}

	outDir, err := os.MkdirTemp("", "heimdall-whisper-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp output directory: %w", err)
	}
	defer os.RemoveAll(outDir)
	outPrefix := filepath.Join(outDir, "result")

	lang := opts.Language
	if lang == "" || lang == "multi" {
		lang = "auto"
	}

	args := []string{
		"-m", opts.ModelPath,
		"-f", wavPath,
		"--diarize",
		"--output-json",
		"--output-file", outPrefix,
		"--no-prints",
		"--language", lang,
	}

	runErr := c.runner(ctx, c.binPath, args)

	data, readErr := os.ReadFile(outPrefix + ".json")
	if readErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			return nil, fmt.Errorf("whisper-cli not found on PATH -- install it with 'brew install whisper-cpp', or use a cloud --transcriber instead")
		}
		if runErr != nil {
			return nil, fmt.Errorf("whisper-cli failed: %w", runErr)
		}
		return nil, fmt.Errorf("whisper-cli did not produce an output file -- check that the model and audio file are valid")
	}

	return parseWhisperJSON(data)
}

// whisperOutput models the subset of whisper-cli's --output-json schema
// this package needs. Unknown fields (systeminfo, model, params, result,
// ...) are ignored by json.Unmarshal.
type whisperOutput struct {
	Transcription []whisperSegment `json:"transcription"`
}

type whisperSegment struct {
	Offsets struct {
		From int64 `json:"from"` // milliseconds
		To   int64 `json:"to"`   // milliseconds
	} `json:"offsets"`
	Text    string `json:"text"`
	Speaker string `json:"speaker"` // "0"/"1" when --diarize is set; may be absent otherwise
}

// parseWhisperJSON converts whisper-cli's JSON output into heimdall
// Segments. Every segment is final -- there is no interim/streaming
// concept in batch transcription.
func parseWhisperJSON(data []byte) ([]heimdall.Segment, error) {
	var out whisperOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing whisper-cli JSON output: %w", err)
	}

	segments := make([]heimdall.Segment, 0, len(out.Transcription))
	for _, s := range out.Transcription {
		speaker := 0
		if s.Speaker != "" {
			if n, err := strconv.Atoi(s.Speaker); err == nil {
				speaker = n
			}
		}
		segments = append(segments, heimdall.Segment{
			Speaker: speaker,
			// whisper's token-level text carries a leading space (BPE
			// tokenization artifact) -- verified empirically, trimmed here.
			Text:       strings.TrimSpace(s.Text),
			Start:      time.Duration(s.Offsets.From) * time.Millisecond,
			End:        time.Duration(s.Offsets.To) * time.Millisecond,
			Confidence: 1.0, // whisper-cli's plain JSON output carries no confidence score
			IsFinal:    true,
		})
	}
	return segments, nil
}

const maxErrLen = 500

func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxErrLen {
		return s[:maxErrLen] + "..."
	}
	return s
}
