package transcriber

import (
	"context"
	"testing"

	heimdall "github.com/0merUfuk/heimdall/internal/heimdall"
)

// mockTranscriber is a minimal implementation of Transcriber used to verify the
// interface is implementable and that its contract can be exercised in tests.
type mockTranscriber struct {
	connected  bool
	segmentCh  chan heimdall.Segment
	sentFrames []heimdall.AudioFrame
}

func newMockTranscriber() *mockTranscriber {
	return &mockTranscriber{
		segmentCh: make(chan heimdall.Segment, 16),
	}
}

func (m *mockTranscriber) Connect(ctx context.Context, opts heimdall.TranscribeOpts) error {
	m.connected = true
	return nil
}

func (m *mockTranscriber) Send(frame heimdall.AudioFrame) error {
	m.sentFrames = append(m.sentFrames, frame)
	return nil
}

func (m *mockTranscriber) Receive() <-chan heimdall.Segment {
	return m.segmentCh
}

func (m *mockTranscriber) Close() error {
	if m.connected {
		m.connected = false
		close(m.segmentCh)
	}
	return nil
}

// Compile-time assertion: mockTranscriber must satisfy Transcriber.
var _ Transcriber = (*mockTranscriber)(nil)

// TestTranscriber_InterfaceCompliance verifies that mockTranscriber satisfies
// the Transcriber interface and all methods are callable.
func TestTranscriber_InterfaceCompliance(t *testing.T) {
	var tr Transcriber = newMockTranscriber()

	ctx := context.Background()
	opts := heimdall.TranscribeOpts{
		Language:   "en",
		Model:      "nova-3",
		SampleRate: 16000,
		Channels:   2,
		Encoding:   "linear16",
		Diarize:    true,
	}

	if err := tr.Connect(ctx, opts); err != nil {
		t.Fatalf("Connect: unexpected error: %v", err)
	}

	frame := heimdall.AudioFrame{
		Data:       make([]byte, 3200),
		SampleRate: 16000,
		Channels:   2,
	}
	if err := tr.Send(frame); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	ch := tr.Receive()
	if ch == nil {
		t.Fatal("Receive: returned nil channel")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
}

// TestTranscriber_ReceiveIsReadOnly verifies Receive() returns a receive-only channel.
func TestTranscriber_ReceiveIsReadOnly(t *testing.T) {
	tr := newMockTranscriber()

	ch := tr.Receive()
	// The interface signature is <-chan heimdall.Segment — this assignment is a compile-time check.
	var roCh <-chan heimdall.Segment = ch
	if roCh == nil {
		t.Fatal("receive channel should not be nil")
	}
}

// TestTranscriber_SendAccumulatesFrames verifies Send captures each frame.
func TestTranscriber_SendAccumulatesFrames(t *testing.T) {
	tr := newMockTranscriber()
	ctx := context.Background()

	if err := tr.Connect(ctx, heimdall.TranscribeOpts{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	frames := []heimdall.AudioFrame{
		{Data: []byte{0x00, 0x01}, SampleRate: 16000, Channels: 2},
		{Data: []byte{0x02, 0x03}, SampleRate: 16000, Channels: 2},
		{Data: []byte{0x04, 0x05}, SampleRate: 16000, Channels: 2},
	}

	for i, f := range frames {
		if err := tr.Send(f); err != nil {
			t.Fatalf("Send[%d]: unexpected error: %v", i, err)
		}
	}

	if len(tr.sentFrames) != 3 {
		t.Errorf("sentFrames: got %d, want 3", len(tr.sentFrames))
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestTranscriber_ReceiveDeliversSegments verifies segments written to the mock's
// internal channel are readable from Receive().
func TestTranscriber_ReceiveDeliversSegments(t *testing.T) {
	tr := newMockTranscriber()
	ctx := context.Background()

	if err := tr.Connect(ctx, heimdall.TranscribeOpts{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	want := heimdall.Segment{
		Speaker:    0,
		Text:       "Hello world",
		Confidence: 0.95,
		IsFinal:    true,
	}
	tr.segmentCh <- want

	ch := tr.Receive()
	got, ok := <-ch
	if !ok {
		t.Fatal("expected to receive a segment, channel was closed")
	}
	if got.Text != want.Text {
		t.Errorf("segment Text: got %q, want %q", got.Text, want.Text)
	}
	if got.Speaker != want.Speaker {
		t.Errorf("segment Speaker: got %d, want %d", got.Speaker, want.Speaker)
	}
	if got.Confidence != want.Confidence {
		t.Errorf("segment Confidence: got %f, want %f", got.Confidence, want.Confidence)
	}
	if got.IsFinal != want.IsFinal {
		t.Errorf("segment IsFinal: got %v, want %v", got.IsFinal, want.IsFinal)
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestTranscriber_ChannelClosedAfterClose verifies the Receive channel is closed
// when Close() is called.
func TestTranscriber_ChannelClosedAfterClose(t *testing.T) {
	tr := newMockTranscriber()
	ctx := context.Background()

	if err := tr.Connect(ctx, heimdall.TranscribeOpts{}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	ch := tr.Receive()
	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, ok := <-ch
	if ok {
		t.Error("expected Receive channel to be closed after Close, but received a value")
	}
}

// TestTranscriber_ConnectWithVariousOpts validates Connect accepts different option combos.
func TestTranscriber_ConnectWithVariousOpts(t *testing.T) {
	tests := []struct {
		name string
		opts heimdall.TranscribeOpts
	}{
		{
			name: "nova-3 with diarization",
			opts: heimdall.TranscribeOpts{
				Language: "en", Model: "nova-3", SampleRate: 16000,
				Channels: 2, Encoding: "linear16", Diarize: true,
			},
		},
		{
			name: "minimal opts",
			opts: heimdall.TranscribeOpts{},
		},
		{
			name: "with keywords",
			opts: heimdall.TranscribeOpts{
				Language: "en", Keywords: []string{"heimdall", "sprints"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newMockTranscriber()
			ctx := context.Background()
			if err := tr.Connect(ctx, tt.opts); err != nil {
				t.Errorf("Connect: unexpected error: %v", err)
			}
			if !tr.connected {
				t.Error("expected connected=true after Connect")
			}
			if err := tr.Close(); err != nil {
				t.Errorf("Close: unexpected error: %v", err)
			}
		})
	}
}
