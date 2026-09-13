package localstt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// realWhisperCLIOutputFixture is byte-for-byte the JSON whisper-cli 1.9.4
// actually wrote for `whisper-cli -m ggml-tiny.bin -f stereo.wav --diarize
// --output-json`, captured while building this package -- not a guess at
// the schema. See .claude/DECISIONS.md ID-008.
const realWhisperCLIOutputFixture = `{
	"systeminfo": "WHISPER : COREML = 0 | OPENVINO = 0 | ",
	"model": {"type": "tiny", "multilingual": true},
	"params": {"model": "ggml-tiny.bin", "language": "en", "translate": false},
	"result": {"language": "en"},
	"transcription": [
		{
			"timestamps": {"from": "00:00:00,000", "to": "00:00:01,000"},
			"offsets": {"from": 0, "to": 1000},
			"text": " I agree.",
			"speaker": "0"
		},
		{
			"timestamps": {"from": "00:00:01,000", "to": "00:00:02,000"},
			"offsets": {"from": 1000, "to": 2000},
			"text": " That's why I got all it.",
			"speaker": "1"
		}
	]
}`

func TestParseWhisperJSON_RealFixture(t *testing.T) {
	segments, err := parseWhisperJSON([]byte(realWhisperCLIOutputFixture))
	if err != nil {
		t.Fatalf("parseWhisperJSON: unexpected error: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("segments: got %d, want 2", len(segments))
	}

	if segments[0].Speaker != 0 {
		t.Errorf("segments[0].Speaker: got %d, want 0", segments[0].Speaker)
	}
	if segments[0].Text != "I agree." {
		t.Errorf("segments[0].Text: got %q, want %q (leading token space should be trimmed)", segments[0].Text, "I agree.")
	}
	if segments[0].Start != 0 {
		t.Errorf("segments[0].Start: got %v, want 0", segments[0].Start)
	}
	if segments[0].End != 1*time.Second {
		t.Errorf("segments[0].End: got %v, want 1s", segments[0].End)
	}
	if !segments[0].IsFinal {
		t.Error("segments[0].IsFinal: batch transcription has no interim concept, must be true")
	}

	if segments[1].Speaker != 1 {
		t.Errorf("segments[1].Speaker: got %d, want 1", segments[1].Speaker)
	}
	if segments[1].Start != 1*time.Second || segments[1].End != 2*time.Second {
		t.Errorf("segments[1] timing: got [%v, %v], want [1s, 2s]", segments[1].Start, segments[1].End)
	}
}

func TestParseWhisperJSON_EmptyTranscription(t *testing.T) {
	segments, err := parseWhisperJSON([]byte(`{"transcription": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 0 {
		t.Errorf("expected 0 segments for silent audio, got %d", len(segments))
	}
}

func TestParseWhisperJSON_MissingSpeakerDefaultsToZero(t *testing.T) {
	// Without --diarize (or on a model that doesn't emit it), "speaker" is
	// absent entirely. Must not crash, must default sensibly.
	segments, err := parseWhisperJSON([]byte(`{"transcription": [{"offsets": {"from": 0, "to": 500}, "text": "hi"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 1 || segments[0].Speaker != 0 {
		t.Errorf("expected one segment with Speaker=0, got %+v", segments)
	}
}

func TestParseWhisperJSON_InvalidJSON(t *testing.T) {
	_, err := parseWhisperJSON([]byte("not json"))
	if err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestResolveModelPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  func(string) bool // predicate against the result, since ModelDir() is host-dependent
	}{
		{"bare size name", "base", func(got string) bool { return filepath.Base(got) == "ggml-base.bin" }},
		{"bare size name tiny", "tiny", func(got string) bool { return filepath.Base(got) == "ggml-tiny.bin" }},
		{"explicit .bin path unchanged", "/custom/model.bin", func(got string) bool { return got == "/custom/model.bin" }},
		{"path with separator unchanged", "models/custom", func(got string) bool { return got == "models/custom" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveModelPath(tt.input)
			if !tt.want(got) {
				t.Errorf("ResolveModelPath(%q) = %q, failed predicate", tt.input, got)
			}
		})
	}
}

func TestModelDownloadURL(t *testing.T) {
	for _, name := range []string{"tiny", "base", "small", "medium", "large"} {
		url, err := ModelDownloadURL(name)
		if err != nil {
			t.Errorf("ModelDownloadURL(%q): unexpected error: %v", name, err)
		}
		if url == "" {
			t.Errorf("ModelDownloadURL(%q): got empty URL", name)
		}
	}

	if _, err := ModelDownloadURL("gigantic"); err == nil {
		t.Error("expected an error for an unknown model size")
	}
}

func TestModelDir_ReturnsHeimdallModelsPath(t *testing.T) {
	got := ModelDir()
	if filepath.Base(got) != "models" {
		t.Errorf("ModelDir(): got %q, want a path ending in .../models", got)
	}
	if filepath.Base(filepath.Dir(got)) != ".heimdall" {
		t.Errorf("ModelDir(): got %q, want a path under .../.heimdall/models", got)
	}
}

func TestClient_TranscribeFile_Success(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.bin")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0600); err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(dir, "test.wav")
	if err := os.WriteFile(wavPath, []byte("fake wav"), 0600); err != nil {
		t.Fatal(err)
	}

	var capturedArgs []string
	runner := func(ctx context.Context, name string, args []string) error {
		capturedArgs = args
		// Simulate whisper-cli writing its output file, mirroring the real
		// --output-file <prefix> behavior (prefix + ".json").
		outPrefix := ""
		for i, a := range args {
			if a == "--output-file" && i+1 < len(args) {
				outPrefix = args[i+1]
			}
		}
		if outPrefix == "" {
			t.Fatal("runner: --output-file not found in args")
		}
		return os.WriteFile(outPrefix+".json", []byte(realWhisperCLIOutputFixture), 0600)
	}

	client := NewClient().WithRunner(runner)
	segments, err := client.TranscribeFile(context.Background(), wavPath, Options{ModelPath: modelPath, Language: "en"})
	if err != nil {
		t.Fatalf("TranscribeFile: unexpected error: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("segments: got %d, want 2", len(segments))
	}

	for _, want := range []string{"--diarize", "--output-json", "-m", modelPath, "-f", wavPath, "--language", "en"} {
		if !slices.Contains(capturedArgs, want) {
			t.Errorf("args should contain %q, got: %v", want, capturedArgs)
		}
	}
}

func TestClient_TranscribeFile_LanguageMultiMapsToAuto(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.bin")
	os.WriteFile(modelPath, []byte("fake"), 0600)
	wavPath := filepath.Join(dir, "test.wav")
	os.WriteFile(wavPath, []byte("fake"), 0600)

	var capturedArgs []string
	runner := func(ctx context.Context, name string, args []string) error {
		capturedArgs = args
		for i, a := range args {
			if a == "--output-file" && i+1 < len(args) {
				return os.WriteFile(args[i+1]+".json", []byte(`{"transcription":[]}`), 0600)
			}
		}
		return nil
	}

	client := NewClient().WithRunner(runner)
	_, err := client.TranscribeFile(context.Background(), wavPath, Options{ModelPath: modelPath, Language: "multi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for i, a := range capturedArgs {
		if a == "--language" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "auto" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --language auto (mapped from \"multi\"), got args: %v", capturedArgs)
	}
}

func TestClient_TranscribeFile_ModelNotFound(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "test.wav")
	os.WriteFile(wavPath, []byte("fake"), 0600)

	client := NewClient().WithRunner(func(ctx context.Context, name string, args []string) error {
		t.Fatal("runner should not be invoked when the model file doesn't exist")
		return nil
	})

	_, err := client.TranscribeFile(context.Background(), wavPath, Options{ModelPath: filepath.Join(dir, "does-not-exist.bin")})
	if err == nil {
		t.Fatal("expected an error for a missing model file")
	}
}

func TestClient_TranscribeFile_AudioFileNotFound(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.bin")
	os.WriteFile(modelPath, []byte("fake"), 0600)

	client := NewClient().WithRunner(func(ctx context.Context, name string, args []string) error {
		t.Fatal("runner should not be invoked when the audio file doesn't exist")
		return nil
	})

	_, err := client.TranscribeFile(context.Background(), filepath.Join(dir, "missing.wav"), Options{ModelPath: modelPath})
	if err == nil {
		t.Fatal("expected an error for a missing audio file")
	}
}

// TestClient_TranscribeFile_SilentFailureDetectedViaMissingOutputFile
// reproduces the exact failure mode verified empirically against the real
// binary: whisper-cli can exit 0 while still failing (e.g. a corrupt model)
// and simply not write the output JSON file. TranscribeFile must treat a
// missing output file as failure regardless of the runner's returned error.
func TestClient_TranscribeFile_SilentFailureDetectedViaMissingOutputFile(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.bin")
	os.WriteFile(modelPath, []byte("fake"), 0600)
	wavPath := filepath.Join(dir, "test.wav")
	os.WriteFile(wavPath, []byte("fake"), 0600)

	// Runner "succeeds" (nil error, matching the real binary's exit-0-on-
	// failure behavior) but writes no output file.
	client := NewClient().WithRunner(func(ctx context.Context, name string, args []string) error {
		return nil
	})

	_, err := client.TranscribeFile(context.Background(), wavPath, Options{ModelPath: modelPath})
	if err == nil {
		t.Fatal("expected an error when whisper-cli produces no output file, even with a nil runner error")
	}
}

func TestClient_TranscribeFile_BinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.bin")
	os.WriteFile(modelPath, []byte("fake"), 0600)
	wavPath := filepath.Join(dir, "test.wav")
	os.WriteFile(wavPath, []byte("fake"), 0600)

	client := NewClient().WithRunner(func(ctx context.Context, name string, args []string) error {
		return &exec.Error{Name: name, Err: exec.ErrNotFound}
	})

	_, err := client.TranscribeFile(context.Background(), wavPath, Options{ModelPath: modelPath})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "brew install") {
		t.Errorf("expected an install hint in the error, got: %v", err)
	}
}

func TestExecCommandRunner_BinaryNotFound(t *testing.T) {
	err := execCommandRunner(context.Background(), "heimdall-this-binary-does-not-exist", nil)
	if err == nil {
		t.Fatal("expected an error for a nonexistent binary")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("expected errors.Is(err, exec.ErrNotFound), got: %v", err)
	}
}
