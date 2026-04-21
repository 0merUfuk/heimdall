package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/heimdall/internal/config"
)

// --- maskSecret / isSecretKey --------------------------------------------

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"env var placeholder deepgram", "${DEEPGRAM_API_KEY}", "${DEEPGRAM_API_KEY}"},
		{"env var placeholder anthropic", "${ANTHROPIC_API_KEY}", "${ANTHROPIC_API_KEY}"},
		{"env var placeholder short", "${X}", "${X}"},
		{"short string 1 char", "a", "********"},
		{"short string 11 chars", "abcdefghijk", "********"}, // exactly below threshold
		{"12-char generic uses suffix", "abcdefgh9999", "****...9999"},
		{"deepgram-style 32-char token", "0123456789abcdef0123456789abcdef", "****...cdef"},
		{"anthropic prefix 17 chars", "sk-ant-abcdefghij", "sk-ant-****...ghij"},
		{"anthropic-prefix but too short", "sk-ant-abc", "********"}, // 10 chars → below threshold
		{"exactly 12 chars non-prefix", "abcdefgh1234", "****...1234"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskSecret(tc.input)
			if got != tc.want {
				t.Errorf("maskSecret(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestMaskSecret_AnthropicKey verifies the sk-ant- prefix is preserved
// so operators can recognize the key family at a glance.
func TestMaskSecret_AnthropicKey(t *testing.T) {
	// Realistic Anthropic key shape: "sk-ant-" + 40+ opaque chars.
	key := "sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB"
	got := maskSecret(key)

	if !strings.HasPrefix(got, "sk-ant-") {
		t.Errorf("masked anthropic key should retain sk-ant- prefix; got %q", got)
	}
	if !strings.HasSuffix(got, "BBBB") {
		t.Errorf("masked anthropic key should reveal last 4 chars; got %q", got)
	}
	if strings.Contains(got, "AAAA") {
		t.Errorf("masked anthropic key leaked middle chars; got %q", got)
	}
	if len(got) >= len(key) {
		t.Errorf("masked value is not shorter than original: got %d chars, original %d", len(got), len(key))
	}
}

// TestMaskSecret_NoLeakage is a belt-and-braces check that no masked
// output contains substantial stretches of the original secret body.
func TestMaskSecret_NoLeakage(t *testing.T) {
	secret := "dg_live_0123456789abcdefghijklmnopqrstuvwxyz"
	masked := maskSecret(secret)

	// Body should not appear in the mask (except for the last 4 chars).
	body := secret[:len(secret)-4]
	if strings.Contains(masked, body) {
		t.Errorf("mask leaked secret body: masked=%q body=%q", masked, body)
	}
	// The mask format is deterministic: "****..." + last 4.
	if masked != "****...wxyz" {
		t.Errorf("maskSecret(%q) = %q; want %q", secret, masked, "****...wxyz")
	}
}

func TestIsSecretKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"deepgram.api_key", true},
		{"claude.api_key", true},
		{"deepgram.model", false},
		{"claude.model", false},
		{"obsidian.vault_path", false},
		{"", false},
		{"api_key", false}, // bare, no prefix — not a real config key
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			if got := isSecretKey(tc.key); got != tc.want {
				t.Errorf("isSecretKey(%q) = %v; want %v", tc.key, got, tc.want)
			}
		})
	}
}

// --- runConfigGet / Set / Show command-path tests ------------------------

// withTempHome redirects HOME to a temp directory for the duration of the
// test, seeds a config file at $HOME/.heimdall/config.yaml with the given
// values, and returns the config path. Caller passes literal values
// (raw secrets, NOT ${VAR} refs) to exercise the masking path.
func withTempHome(t *testing.T, deepgramKey, claudeKey string) string {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Deepgram.APIKey = deepgramKey
	cfg.Claude.APIKey = claudeKey
	cfg.Obsidian.VaultPath = filepath.Join(tmp, "vault")

	path := config.ConfigPath()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	return path
}

// captureStdout redirects stdout for fn's execution and returns what was
// written. Tests assert on this to confirm secrets never reach the terminal.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	return <-done
}

// TestRunConfigGet_MasksDeepgramAPIKey verifies that `config get deepgram.api_key`
// never prints the raw secret, even when the secret is stored as a literal value
// (i.e., when a user violated the ${VAR} convention).
func TestRunConfigGet_MasksDeepgramAPIKey(t *testing.T) {
	const rawSecret = "dg_live_supersecret_1234567890abcdef" // literal, not a ref
	withTempHome(t, rawSecret, "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigGet(nil, []string{"deepgram.api_key"}); err != nil {
			t.Fatalf("runConfigGet: %v", err)
		}
	})

	if strings.Contains(out, rawSecret) {
		t.Errorf("runConfigGet leaked raw secret in output: %q", out)
	}
	if !strings.Contains(out, "****") {
		t.Errorf("output should contain mask marker; got %q", out)
	}
	if !strings.Contains(out, "cdef") {
		t.Errorf("output should reveal last 4 chars as an identification anchor; got %q", out)
	}
}

// TestRunConfigGet_PreservesEnvVarRef verifies that a ${VAR} placeholder
// resolves via the environment, and the resolved value is masked while the
// raw placeholder is shown verbatim.
func TestRunConfigGet_PreservesEnvVarRef(t *testing.T) {
	const rawSecret = "sk-ant-api03-MMMMMMMMMMMMMMMMMMMMMMMMMMMMDEAD"
	// ResolveEnvVars iterates fields in order and bails on first missing var.
	// Set both so the claude.api_key field actually gets resolved.
	t.Setenv("DEEPGRAM_API_KEY", "dg_live_irrelevant_to_this_test_xyz")
	t.Setenv("ANTHROPIC_API_KEY", rawSecret)
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigGet(nil, []string{"claude.api_key"}); err != nil {
			t.Fatalf("runConfigGet: %v", err)
		}
	})

	if strings.Contains(out, rawSecret) {
		t.Errorf("runConfigGet leaked resolved env-var secret in output: %q", out)
	}
	if !strings.Contains(out, "${ANTHROPIC_API_KEY}") {
		t.Errorf("raw placeholder should be preserved verbatim; got %q", out)
	}
	// The resolved value is anthropic-style so the mask should include sk-ant-.
	if !strings.Contains(out, "sk-ant-****...DEAD") {
		t.Errorf("resolved value should be masked in anthropic style; got %q", out)
	}
}

// TestRunConfigGet_NonSecretKeyUnchanged verifies masking only fires for
// known secret keys — ordinary values pass through unchanged.
func TestRunConfigGet_NonSecretKeyUnchanged(t *testing.T) {
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigGet(nil, []string{"deepgram.model"}); err != nil {
			t.Fatalf("runConfigGet: %v", err)
		}
	})

	if !strings.Contains(out, "nova-3") {
		t.Errorf("deepgram.model should print as-is; got %q", out)
	}
}

// TestRunConfigSet_MasksAPIKeyEcho verifies that the success echo after
// `config set deepgram.api_key <secret>` does NOT print the raw secret,
// which would leak into shell history and CI logs.
func TestRunConfigSet_MasksAPIKeyEcho(t *testing.T) {
	const rawSecret = "dg_live_brandnewkey_9876543210ZZZZ"
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigSet(nil, []string{"deepgram.api_key", rawSecret}); err != nil {
			t.Fatalf("runConfigSet: %v", err)
		}
	})

	if strings.Contains(out, rawSecret) {
		t.Errorf("runConfigSet echoed raw secret: %q", out)
	}
	if !strings.Contains(out, "deepgram.api_key") {
		t.Errorf("echo should name the key; got %q", out)
	}
	if !strings.Contains(out, "ZZZZ") {
		t.Errorf("echo should reveal last 4 chars as identification anchor; got %q", out)
	}
}

// TestRunConfigSet_EnvVarRefPassthrough verifies that setting a ${VAR}
// placeholder echoes the placeholder verbatim (not a secret, no masking).
func TestRunConfigSet_EnvVarRefPassthrough(t *testing.T) {
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigSet(nil, []string{"claude.api_key", "${MY_CLAUDE_KEY}"}); err != nil {
			t.Fatalf("runConfigSet: %v", err)
		}
	})

	if !strings.Contains(out, "${MY_CLAUDE_KEY}") {
		t.Errorf("env var placeholder should pass through echo; got %q", out)
	}
}

// TestRunConfigSet_NonSecretEchoUnchanged verifies that non-secret values
// echo as-is — nova-3 is not PII.
func TestRunConfigSet_NonSecretEchoUnchanged(t *testing.T) {
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigSet(nil, []string{"deepgram.model", "nova-3"}); err != nil {
			t.Fatalf("runConfigSet: %v", err)
		}
	})

	if !strings.Contains(out, "deepgram.model = nova-3") {
		t.Errorf("non-secret echo should be verbatim; got %q", out)
	}
}

// TestRunConfigShow_MasksBothSecrets verifies that `config show` masks
// literal API keys in the YAML dump while preserving ${VAR} placeholders.
func TestRunConfigShow_MasksBothSecrets(t *testing.T) {
	const deepgramRaw = "dg_live_topsecret_aaaabbbbccccdddd"
	const claudeRaw = "sk-ant-api03-YYYYYYYYYYYYYYYYYYYYYYYY1234"
	withTempHome(t, deepgramRaw, claudeRaw)

	out := captureStdout(t, func() {
		if err := runConfigShow(nil, nil); err != nil {
			t.Fatalf("runConfigShow: %v", err)
		}
	})

	if strings.Contains(out, deepgramRaw) {
		t.Errorf("config show leaked deepgram secret: %q", out)
	}
	if strings.Contains(out, claudeRaw) {
		t.Errorf("config show leaked claude secret: %q", out)
	}
	if !strings.Contains(out, "****...dddd") {
		t.Errorf("deepgram key should be masked with last-4 suffix; got %q", out)
	}
	if !strings.Contains(out, "sk-ant-****...1234") {
		t.Errorf("claude key should be masked in anthropic style; got %q", out)
	}
}

// TestRunConfigShow_PreservesPlaceholders verifies that ${VAR} refs are
// shown verbatim in the YAML dump — masking them would confuse users who
// rely on seeing which env var a config points at.
func TestRunConfigShow_PreservesPlaceholders(t *testing.T) {
	withTempHome(t, "${DEEPGRAM_API_KEY}", "${ANTHROPIC_API_KEY}")

	out := captureStdout(t, func() {
		if err := runConfigShow(nil, nil); err != nil {
			t.Fatalf("runConfigShow: %v", err)
		}
	})

	if !strings.Contains(out, "${DEEPGRAM_API_KEY}") {
		t.Errorf("deepgram env-var ref should pass through; got %q", out)
	}
	if !strings.Contains(out, "${ANTHROPIC_API_KEY}") {
		t.Errorf("claude env-var ref should pass through; got %q", out)
	}
}
