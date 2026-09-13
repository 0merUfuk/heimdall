package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/0merUfuk/heimdall/internal/localstt"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local Whisper models for offline transcription",
}

var modelDownloadCmd = &cobra.Command{
	Use:   "download <size>",
	Short: "Download a local Whisper model (tiny, base, small, medium, large)",
	Long: `Downloads a ggml Whisper model from Hugging Face to
~/.heimdall/models/, for use with 'heimdall transcribe' (local, offline
transcription of a saved audio file).

Model sizes trade accuracy for speed and disk space (roughly, per the
whisper.cpp project): tiny (~75MB, fastest), base (~140MB), small (~460MB),
medium (~1.5GB), large (~3GB, most accurate). "base" is a reasonable
default for most meetings.`,
	Example: `  heimdall model download base
  heimdall model download small`,
	Args: cobra.ExactArgs(1),
	RunE: runModelDownload,
}

func init() {
	modelCmd.AddCommand(modelDownloadCmd)
	rootCmd.AddCommand(modelCmd)
}

func runModelDownload(cmd *cobra.Command, args []string) error {
	name := args[0]

	url, err := localstt.ModelDownloadURL(name)
	if err != nil {
		return err
	}

	path := localstt.ResolveModelPath(name)
	if info, err := os.Stat(path); err == nil {
		fmt.Printf("Model already downloaded: %s (%.1f MB)\n", path, float64(info.Size())/1024/1024)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}

	fmt.Printf("Downloading %s model from %s ...\n", name, url)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading model: server returned status %d", resp.StatusCode)
	}

	// Atomic write: temp file -> rename, so an interrupted download never
	// leaves a corrupt .bin file that whisper-cli would fail to load with a
	// confusing error (V-006-style pattern used throughout this codebase).
	tmpPath := path + ".downloading"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating model file: %w", err)
	}

	written, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("downloading model: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing model file: %w", closeErr)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("finalizing model file: %w", err)
	}

	fmt.Printf("Downloaded %s model (%.1f MB) -> %s\n", name, float64(written)/1024/1024, path)
	fmt.Printf("Use it with: heimdall transcribe --file <audio.wav> --model %s\n", name)
	return nil
}
