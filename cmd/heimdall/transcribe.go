package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/localstt"
	"github.com/0merUfuk/heimdall/internal/recovery"
)

var (
	transcribeFile     string
	transcribeModel    string
	transcribeLanguage string
	transcribeTitle    string
)

var transcribeCmd = &cobra.Command{
	Use:   "transcribe",
	Short: "Transcribe a saved audio file locally via Whisper -- no cloud, no API key",
	Long: `Runs a saved WAV file (e.g. from 'heimdall record --save-audio')
through a local whisper.cpp model. Fully offline: no API key, no network
call, no per-meeting cost.

The input is expected to be the stereo WAV internal/recording writes
(L=system audio, R=microphone). --diarize is always passed to whisper-cli,
which tags each segment with a "0"/"1" speaker based on which channel was
active -- a real, if coarse, two-party separation (you vs. everyone else on
the call), not per-individual diarization like Deepgram/Soniox provide for
multi-participant calls.

Writes a transcript in the same format 'heimdall record' uses for crash
recovery, so the normal analysis path picks it up directly.

Requires the whisper-cli binary (brew install whisper-cpp) and a
downloaded model (heimdall model download <size>).`,
	Example: `  heimdall transcribe --file ~/.heimdall/recordings/2026-09-13T10-00-00-standup.wav
  heimdall transcribe --file meeting.wav --model small --language tr
  heimdall transcribe --file meeting.wav && heimdall analyze --file <printed path>`,
	RunE: runTranscribe,
}

func init() {
	transcribeCmd.Flags().StringVar(&transcribeFile, "file", "", "path to a WAV audio file, e.g. from --save-audio (required)")
	transcribeCmd.Flags().StringVar(&transcribeModel, "model", localstt.ModelSmall, "whisper model: tiny, base, small (default), medium, large, or a path to a .bin file. Use medium for Turkish or jargon-heavy audio")
	transcribeCmd.Flags().StringVar(&transcribeLanguage, "language", "en", "spoken language code (e.g. en, tr), or auto/multi for language auto-detect")
	transcribeCmd.Flags().StringVar(&transcribeTitle, "title", "", "title for the resulting transcript (defaults to the audio filename)")
	_ = transcribeCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(transcribeCmd)
}

func runTranscribe(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		return fmt.Errorf("whisper-cli not found on PATH\n\n" +
			"Install it with:\n  brew install whisper-cpp\n\n" +
			"Then download a model:\n  heimdall model download base")
	}

	modelPath := localstt.ResolveModelPath(transcribeModel)

	title := transcribeTitle
	if title == "" {
		base := filepath.Base(transcribeFile)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	fmt.Printf("Transcribing %s locally via Whisper (%s model)...\n", transcribeFile, transcribeModel)
	fmt.Println("This runs entirely on this machine -- no audio leaves it.")

	client := localstt.NewClient()

	// A batch pass over a long meeting can genuinely take a while even with
	// GPU acceleration on a large model -- this is deliberately generous,
	// unlike session.go's 30s live-shutdown budget (see DECISIONS.md ID-007
	// for why that distinction matters).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	segments, err := client.TranscribeFile(ctx, transcribeFile, localstt.Options{
		ModelPath: modelPath,
		Language:  transcribeLanguage,
	})
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		fmt.Println("Warning: no speech detected in the audio file.")
	}

	rw, err := recovery.NewRecoveryWriter(title, time.Now(), transcribeLanguage)
	if err != nil {
		return fmt.Errorf("creating transcript file: %w", err)
	}
	for _, seg := range segments {
		rw.AddSegment(seg)
	}
	if err := rw.Flush(); err != nil {
		return fmt.Errorf("writing transcript file: %w", err)
	}

	fmt.Printf("Transcribed %d segments -> %s\n", len(segments), rw.FilePath())
	fmt.Printf("To analyze: heimdall analyze --file %s\n", rw.FilePath())
	return nil
}
