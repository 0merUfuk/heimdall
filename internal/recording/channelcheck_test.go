package recording

import (
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// speechLike returns band-limited noise with slow amplitude modulation, a
// reasonable stand-in for speech for correlation purposes.
func speechLike(seconds int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	n := seconds * 16000
	out := make([]float64, n)
	var lp float64
	for i := range out {
		lp = 0.9*lp + 0.1*rng.NormFloat64()
		env := 0.6 + 0.4*math.Sin(2*math.Pi*float64(i)/16000/1.7)
		out[i] = lp * env * 9000
	}
	return out
}

func interleave(l, r []float64) []int16 {
	pcm := make([]int16, len(l)*2)
	for i := range l {
		pcm[i*2] = int16(l[i])
		pcm[i*2+1] = int16(r[i])
	}
	return pcm
}

func delayed(x []float64, samples int, gain float64) []float64 {
	out := make([]float64, len(x))
	for i := samples; i < len(x); i++ {
		out[i] = x[i-samples] * gain
	}
	return out
}

func TestAnalyzeStereoPCM16_DetectsDelayedCopy(t *testing.T) {
	l := speechLike(30, 1)
	// The exact shape of the live-test bug: R = L delayed 98.2 ms, ~equal gain.
	r := delayed(l, 1571, 0.99)
	rep := AnalyzeStereoPCM16(interleave(l, r))
	if !rep.Conclusive || !rep.Duplicate {
		t.Fatalf("want a conclusive duplicate, got %+v", rep)
	}
	if math.Abs(rep.LagMS-98) > 4 {
		t.Errorf("lag = %.1f ms, want ~98", rep.LagMS)
	}
}

func TestAnalyzeStereoPCM16_IndependentChannelsPass(t *testing.T) {
	rep := AnalyzeStereoPCM16(interleave(speechLike(30, 1), speechLike(30, 2)))
	if !rep.Conclusive || rep.Duplicate {
		t.Fatalf("independent channels flagged: %+v", rep)
	}
}

// A partly overlapping mix (room echo of the remote side into the mic plus
// the user's own voice) must not be flagged: only near-copies are.
func TestAnalyzeStereoPCM16_MixedChannelsPass(t *testing.T) {
	l := speechLike(30, 1)
	own := speechLike(30, 3)
	echo := delayed(l, 900, 0.3)
	r := make([]float64, len(l))
	for i := range r {
		r[i] = own[i] + echo[i]
	}
	rep := AnalyzeStereoPCM16(interleave(l, r))
	if rep.Duplicate {
		t.Fatalf("mic with real own-voice content flagged as duplicate: %+v", rep)
	}
}

func TestAnalyzeStereoPCM16_SilenceIsInconclusive(t *testing.T) {
	rep := AnalyzeStereoPCM16(make([]int16, 16000*2*30))
	if rep.Conclusive || rep.Duplicate {
		t.Fatalf("silence must be inconclusive, got %+v", rep)
	}
}

func writeTestWAV(t *testing.T, pcm []int16) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.wav")
	hdr := make([]byte, wavHeaderSize)
	copy(hdr, "RIFF")
	body := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(body[i*2:], uint16(s))
	}
	if err := os.WriteFile(path, append(hdr, body...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnalyzeWAV(t *testing.T) {
	l := speechLike(90, 7)
	dup := writeTestWAV(t, interleave(l, delayed(l, 1571, 1)))
	rep, err := AnalyzeWAV(dup)
	if err != nil || !rep.Duplicate {
		t.Fatalf("duplicate WAV: rep=%+v err=%v", rep, err)
	}
	ind := writeTestWAV(t, interleave(l, speechLike(90, 8)))
	rep, err = AnalyzeWAV(ind)
	if err != nil || rep.Duplicate || !rep.Conclusive {
		t.Fatalf("independent WAV: rep=%+v err=%v", rep, err)
	}
}

func frameOf(pcm []int16) heimdall.AudioFrame {
	b := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return heimdall.AudioFrame{Data: b, SampleRate: 16000, Channels: 2}
}

func TestChannelMonitor_WarnsOnceOnDuplicate(t *testing.T) {
	l := speechLike(40, 9)
	pcm := interleave(l, delayed(l, 1571, 1))
	got := make(chan ChannelReport, 4)
	m := NewChannelMonitor(func(r ChannelReport) { got <- r })
	step := 320 * 2 // 20 ms frames
	for i := 0; i+step <= len(pcm); i += step {
		m.Feed(frameOf(pcm[i : i+step]))
	}
	select {
	case r := <-got:
		if !r.Duplicate {
			t.Fatalf("callback with non-duplicate report: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("monitor never reported the duplicate channels")
	}
	select {
	case <-got:
		t.Fatal("monitor reported more than once")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestChannelMonitor_SilentOnIndependentChannels(t *testing.T) {
	pcm := interleave(speechLike(40, 9), speechLike(40, 10))
	called := make(chan struct{}, 1)
	m := NewChannelMonitor(func(ChannelReport) { called <- struct{}{} })
	step := 320 * 2
	for i := 0; i+step <= len(pcm); i += step {
		m.Feed(frameOf(pcm[i : i+step]))
	}
	select {
	case <-called:
		t.Fatal("independent channels triggered the duplicate warning")
	case <-time.After(1500 * time.Millisecond):
	}
}
