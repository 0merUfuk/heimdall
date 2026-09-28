package recording

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sync"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

// Channel-independence check.
//
// The mixer's contract is L = system audio, R = microphone. If both carry the
// same signal (possibly time-shifted), the "system audio" channel is not
// really system audio: on the first live test the helper read the default
// input device instead of the process tap, so L and R were both the
// microphone, R = L delayed by a constant 98 ms at correlation 0.997. The
// whole meeting recorded without a single remote voice and nothing said so
// (.claude/DECISIONS.md ID-017). This check makes that failure loud.
//
// It looks for a strong normalized cross-correlation between the channels
// across lags of up to +-maxLag. The lag search matters: a plain zero-lag
// correlation of that recording is about 0, which is exactly how the bug was
// first missed.

const (
	// checkRate is the rate the check runs at after decimating 16 kHz by 8:
	// plenty for speech correlation, and 8x cheaper.
	checkRate    = 2000
	decimateBy   = 8
	maxLagMS     = 500
	dupThreshold = 0.95
	// activeRMS is the per-100ms int16 RMS above which a channel counts as
	// carrying signal (well above a quiet mic's noise floor).
	activeRMS = 150.0
	// minActiveSeconds of both-channels-active audio are needed before the
	// verdict means anything: two silent channels are trivially "identical".
	minActiveSeconds = 8.0
)

// ChannelReport is the outcome of a channel-independence check.
type ChannelReport struct {
	// Conclusive is false when there was not enough signal to judge.
	Conclusive bool
	// Duplicate is true when the channels are (near-)copies of each other.
	Duplicate bool
	// Correlation is the peak absolute normalized cross-correlation.
	Correlation float64
	// LagMS is the delay at which the peak occurs (positive: R lags L).
	LagMS         float64
	ActiveSeconds float64
}

// String is a one-line human-readable verdict.
func (r ChannelReport) String() string {
	switch {
	case !r.Conclusive:
		return fmt.Sprintf("not enough signal to judge (%.1fs active)", r.ActiveSeconds)
	case r.Duplicate:
		return fmt.Sprintf("channels are near-identical (correlation %.3f at %.0f ms lag)", r.Correlation, r.LagMS)
	default:
		return fmt.Sprintf("channels are independent (peak correlation %.3f)", r.Correlation)
	}
}

// DuplicateWarning is the user-facing message for a Duplicate report.
const DuplicateWarning = "the left (system audio) and right (microphone) channels are near-identical, so system audio is NOT being captured " +
	"-- remote participants will be missing from the transcript and speakers cannot be told apart. " +
	"Check System Settings -> Privacy & Security -> Screen & System Audio Recording for the app you run heimdall from"

// AnalyzeStereoPCM16 checks interleaved 16-bit stereo PCM at 16 kHz.
func AnalyzeStereoPCM16(pcm []int16) ChannelReport {
	frames := len(pcm) / 2
	fl := make([]float64, frames)
	fr := make([]float64, frames)
	for i := 0; i < frames; i++ {
		fl[i] = float64(pcm[i*2])
		fr[i] = float64(pcm[i*2+1])
	}
	dl, dr := decimate(fl), decimate(fr)
	active := activeSeconds(dl, dr)
	rep := ChannelReport{ActiveSeconds: active}
	if active < minActiveSeconds || len(dl) == 0 {
		return rep
	}
	rep.Conclusive = true

	// Stage 1: coarse lag search at checkRate across +-maxLag.
	coarse := newCorrelator(dl, dr)
	maxLag := maxLagMS * checkRate / 1000
	bestLag, bestC := 0, 0.0
	for lag := -maxLag; lag <= maxLag; lag++ {
		if c := coarse.at(lag); c > bestC {
			bestC, bestLag = c, lag
		}
	}

	// Stage 2: refine at the full rate around the coarse peak. Decimating by
	// 8 quantises the lag to 0.5 ms, which for anything but a lag that is a
	// multiple of 8 samples costs real correlation at speech frequencies.
	fine := newCorrelator(fl, fr)
	centre := bestLag * decimateBy
	best, lagAtBest := 0.0, centre
	for lag := centre - 2*decimateBy; lag <= centre+2*decimateBy; lag++ {
		if c := fine.at(lag); c > best {
			best, lagAtBest = c, lag
		}
	}
	rep.Correlation = best
	rep.LagMS = float64(lagAtBest) * 1000 / (checkRate * decimateBy)
	rep.Duplicate = best >= dupThreshold
	return rep
}

// decimate averages groups of decimateBy samples.
func decimate(x []float64) []float64 {
	out := make([]float64, len(x)/decimateBy)
	for i := range out {
		var sum float64
		for k := 0; k < decimateBy; k++ {
			sum += x[i*decimateBy+k]
		}
		out[i] = sum / decimateBy
	}
	return out
}

// activeSeconds counts the seconds (in 100 ms steps) during which both
// channels carry signal.
func activeSeconds(l, r []float64) float64 {
	win := checkRate / 10
	var active float64
	for i := 0; i+win <= len(l); i += win {
		var el, er float64
		for k := i; k < i+win; k++ {
			el += l[k] * l[k]
			er += r[k] * r[k]
		}
		// Averaging attenuates speech energy somewhat; /4 keeps the
		// threshold comparable to the raw one.
		if math.Sqrt(el/float64(win)) > activeRMS/4 && math.Sqrt(er/float64(win)) > activeRMS/4 {
			active += 0.1
		}
	}
	return active
}

// correlator computes the normalized cross-correlation of two equal-length
// signals at a given lag, normalising over the overlapping span only so the
// unmatched edge of a shifted copy does not depress the score.
type correlator struct {
	l, r   []float64
	sl, sr []float64 // prefix sums of squares
}

func newCorrelator(l, r []float64) *correlator {
	c := &correlator{l: l, r: r, sl: make([]float64, len(l)+1), sr: make([]float64, len(r)+1)}
	for i := range l {
		c.sl[i+1] = c.sl[i] + l[i]*l[i]
		c.sr[i+1] = c.sr[i] + r[i]*r[i]
	}
	return c
}

// at returns |corr(l[i], r[i+lag])| over the overlap.
func (c *correlator) at(lag int) float64 {
	lo, hi := 0, len(c.l)
	if lag > 0 {
		hi = len(c.l) - lag
	} else {
		lo = -lag
	}
	if hi-lo < 1 {
		return 0
	}
	var sum float64
	for i := lo; i < hi; i++ {
		sum += c.l[i] * c.r[i+lag]
	}
	den := math.Sqrt((c.sl[hi]-c.sl[lo])*(c.sr[hi+lag]-c.sr[lo+lag])) + 1e-9
	return math.Abs(sum) / den
}

// AnalyzeWAV checks a 16 kHz stereo 16-bit WAV as written by WAVWriter. It
// samples three windows (near the start, middle, and end) so a quiet stretch
// does not hide the answer; the verdict is Duplicate only if every window
// with enough signal is a duplicate.
func AnalyzeWAV(path string) (ChannelReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return ChannelReport{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ChannelReport{}, err
	}
	const bytesPerFrame = 4 // 2 channels x int16
	totalFrames := (info.Size() - wavHeaderSize) / bytesPerFrame
	if totalFrames <= 0 {
		return ChannelReport{}, nil
	}
	const windowSeconds = 40
	winFrames := int64(windowSeconds * 16000)
	if winFrames > totalFrames {
		winFrames = totalFrames
	}
	var conclusive []ChannelReport
	var totalActive float64
	for _, frac := range []float64{0.1, 0.5, 0.9} {
		start := int64(float64(totalFrames-winFrames) * frac)
		buf := make([]byte, winFrames*bytesPerFrame)
		if _, err := f.ReadAt(buf, wavHeaderSize+start*bytesPerFrame); err != nil && err != io.EOF {
			return ChannelReport{}, err
		}
		pcm := make([]int16, len(buf)/2)
		for i := range pcm {
			pcm[i] = int16(binary.LittleEndian.Uint16(buf[i*2:]))
		}
		rep := AnalyzeStereoPCM16(pcm)
		totalActive += rep.ActiveSeconds
		if rep.Conclusive {
			conclusive = append(conclusive, rep)
		}
	}
	if len(conclusive) == 0 {
		return ChannelReport{ActiveSeconds: totalActive}, nil
	}
	out := conclusive[0]
	out.Duplicate = true
	for _, c := range conclusive {
		if !c.Duplicate {
			out.Duplicate = false
		}
		if c.Correlation > out.Correlation {
			out.Correlation, out.LagMS = c.Correlation, c.LagMS
		}
	}
	out.ActiveSeconds = totalActive
	return out, nil
}

// ChannelMonitor watches a live stereo stream and calls onDuplicate once, as
// soon as it can tell, if the channels are copies of each other. It is fed
// from the audio path, so Feed only decimates and appends; the correlation
// runs on a separate goroutine.
type ChannelMonitor struct {
	onDuplicate func(ChannelReport)

	mu       sync.Mutex
	pcm      []int16 // most recent window, interleaved 16 kHz stereo
	sinceRun int     // frames appended since the last evaluation
	done     bool
	running  bool
}

const (
	monitorWindowFrames = 60 * 16000 // analysed window
	monitorEveryFrames  = 15 * 16000 // re-evaluate cadence
)

// NewChannelMonitor returns a monitor that reports to onDuplicate.
func NewChannelMonitor(onDuplicate func(ChannelReport)) *ChannelMonitor {
	return &ChannelMonitor{onDuplicate: onDuplicate}
}

// Feed offers one 16 kHz stereo 16-bit frame. Non-conforming frames are ignored.
func (m *ChannelMonitor) Feed(frame heimdall.AudioFrame) {
	if frame.Channels != 2 || len(frame.Data) < 4 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done {
		return
	}
	n := len(frame.Data) / 2
	for i := 0; i < n; i++ {
		m.pcm = append(m.pcm, int16(binary.LittleEndian.Uint16(frame.Data[i*2:])))
	}
	if extra := len(m.pcm) - monitorWindowFrames*2; extra > 0 {
		m.pcm = append(m.pcm[:0], m.pcm[extra:]...)
	}
	m.sinceRun += n / 2
	if m.sinceRun < monitorEveryFrames || m.running {
		return
	}
	m.sinceRun = 0
	m.running = true
	snapshot := append([]int16(nil), m.pcm...)
	go m.evaluate(snapshot)
}

func (m *ChannelMonitor) evaluate(snapshot []int16) {
	rep := AnalyzeStereoPCM16(snapshot)
	m.mu.Lock()
	m.running = false
	if rep.Conclusive {
		m.done = true // one verdict is enough; don't keep burning CPU
	}
	m.mu.Unlock()
	if rep.Conclusive && rep.Duplicate && m.onDuplicate != nil {
		m.onDuplicate(rep)
	}
}
