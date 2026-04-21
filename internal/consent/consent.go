// Package consent implements the first-run recording-consent gate shown
// before heimdall begins capturing audio. The gate is printed only once per
// user: after the user presses Enter, the acknowledgement is persisted to the
// config file and the banner does not appear on subsequent runs.
//
// Audit reference: tasks/strategic-audit-2026-04-16.md §6 item 4.
// Ratified by AD-011 (Option A).
package consent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/0merUfuk/heimdall/internal/config"
)

// Banner is the consent text printed before recording starts on first run.
// Wording deliberately names the external providers (Deepgram, Anthropic) and
// the user's legal responsibility for obtaining consent. See PRIVACY.md for
// the full data-flow disclosure.
const Banner = `heimdall records this meeting:
  - Your microphone audio
  - System audio (other participants on your speakers)

Audio is streamed to Deepgram for real-time transcription.
Transcripts are sent to Anthropic for summarization.
Both providers are governed by the API keys you control.
See PRIVACY.md for the full data-flow and biometric-data notice.

Recording consent laws vary by jurisdiction. 12+ US states require
ALL participants to consent. EU/UK require participants to be informed.
You are responsible for obtaining consent.

Press Enter to acknowledge and continue, or Ctrl+C to cancel.
(This prompt appears only once. Use --consent-acknowledged in scripts.)
`

// NotAcknowledgedError is returned when the gate cannot prompt the user
// (non-interactive terminal) and no other acknowledgement path was provided.
// The Error() text is actionable and suitable for display to the end user.
type NotAcknowledgedError struct{}

func (NotAcknowledgedError) Error() string {
	return "Recording consent not acknowledged. Run `heimdall record` " +
		"interactively once, or pass --consent-acknowledged, or set " +
		"consent.acknowledged=true in config."
}

// Options configures a single invocation of Gate. Tests inject PromptReader,
// PromptWriter, and IsTerminalFn to drive the flow without a real TTY; the
// CLI wires os.Stdin, os.Stderr, and a real isatty check.
type Options struct {
	// FlagAcknowledged is true when --consent-acknowledged was passed on the
	// command line. Acknowledges the banner for this invocation only (does
	// NOT persist to config -- rationale: scripted/CI users should not
	// accidentally flip a persistent bit for the local user).
	FlagAcknowledged bool

	// ConfigPath is where the config file lives (or should be written).
	// Typically config.ConfigPath().
	ConfigPath string

	// Cfg is the currently loaded config. If nil, a default config is used
	// as the write target when the user acknowledges interactively.
	Cfg *config.Config

	// PromptReader is the stream to read the Enter keypress from. CLI passes
	// os.Stdin; tests pass a bytes.Reader.
	PromptReader io.Reader

	// PromptWriter is where the banner is printed. CLI passes os.Stderr so
	// the banner never pollutes stdout pipelines; tests pass a bytes.Buffer.
	PromptWriter io.Writer

	// IsTerminalFn reports whether PromptReader is attached to an interactive
	// terminal. CLI passes a real isatty(stdin) check; tests pass a stub.
	IsTerminalFn func() bool

	// Now returns the current time. Tests override to pin acknowledged_at.
	Now func() time.Time
}

// Gate enforces the first-run consent decision tree:
//
//  1. If opts.FlagAcknowledged is true -> proceed, do not write to config.
//  2. Else if opts.Cfg.Consent.Acknowledged is true -> proceed silently.
//  3. Else if PromptReader is a TTY -> print banner, wait for Enter, persist.
//  4. Else (non-interactive, no flag, no prior config) -> return
//     NotAcknowledgedError with an actionable message.
//
// Ctx is honoured during the blocking read: if the user hits Ctrl+C while the
// banner is displayed, Gate returns context.Canceled without writing config.
func Gate(ctx context.Context, opts Options) error {
	// 1. Flag overrides everything. Does not persist.
	if opts.FlagAcknowledged {
		return nil
	}

	// 2. Config already says the user acknowledged. Silent pass.
	if opts.Cfg != nil && opts.Cfg.Consent.Acknowledged {
		return nil
	}

	// 3. Interactive TTY -> print banner and wait for Enter.
	if opts.IsTerminalFn != nil && opts.IsTerminalFn() {
		if _, err := fmt.Fprint(opts.PromptWriter, Banner); err != nil {
			return fmt.Errorf("printing consent banner: %w", err)
		}

		// Read a single line. Run the blocking ReadString in a goroutine so
		// we can also watch ctx.Done() for Ctrl+C mid-prompt.
		type readResult struct {
			err error
		}
		done := make(chan readResult, 1)
		reader := bufio.NewReader(opts.PromptReader)
		go func() {
			_, err := reader.ReadString('\n')
			done <- readResult{err: err}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-done:
			// EOF without any input (stdin closed) is equivalent to cancel.
			// Treat as NotAcknowledged so we never silently acknowledge
			// without a keypress.
			if res.err != nil && !errors.Is(res.err, io.EOF) {
				return fmt.Errorf("reading consent response: %w", res.err)
			}
			if errors.Is(res.err, io.EOF) {
				// No newline observed -- treat as not acknowledged.
				return NotAcknowledgedError{}
			}
		}

		// Persist to config. If no cfg was provided, start from defaults.
		cfg := opts.Cfg
		if cfg == nil {
			cfg = config.DefaultConfig()
		}
		now := time.Now
		if opts.Now != nil {
			now = opts.Now
		}
		cfg.Consent.Acknowledged = true
		cfg.Consent.AcknowledgedAt = now().UTC().Format(time.RFC3339)

		if err := cfg.Save(opts.ConfigPath); err != nil {
			return fmt.Errorf("persisting consent acknowledgement: %w", err)
		}
		return nil
	}

	// 4. Non-interactive and no acknowledgement anywhere. Emit the banner to
	// the prompt stream so the operator sees what they would be accepting,
	// then fail with an actionable message.
	if opts.PromptWriter != nil {
		_, _ = fmt.Fprint(opts.PromptWriter, Banner)
	}
	return NotAcknowledgedError{}
}
