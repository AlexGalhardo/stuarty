package ingest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
)

// BeatsVersion is the version of beats-1.json.
const BeatsVersion = 1

// The constants of the beat grid (RF-044). The onset signal has one value for every 10 ms, so
// tempo is searched between 60 and 200 BPM as a lag of 30 to 100 values.
const (
	beatHop         = SampleRate / 100
	beatMinLag      = 30
	beatMaxLag      = 100
	beatPriorLag    = 50.0 // 120 BPM: the middle of the prior that settles the octave
	beatPriorWidth  = 1.0  // octaves, one standard deviation of the prior
	beatTightness   = 100.0
	beatLowPass     = 0.0258 // 1 - exp(-2 pi 200 / 48000)
	beatHighWeight  = 0.6
	beatMinPeaks    = 8
	beatMinRise     = 0.5 // in log energy, a little over a 40% rise
	beatMinConf     = 0.12
	beatStrongOnset = 1.0 // standard deviations of the onset signal
	// MaxBeatSamples is the longest audio the beat analysis reads: two hours at 48 kHz. A longer
	// file gets an empty grid, so the job stays inside the ingest limit (SDD-007 9.1).
	MaxBeatSamples = int64(2 * 3600 * SampleRate)
)

// Beats is beats-1.json: the beat grid of an audio asset, in samples at 48 kHz (RF-044). Beats
// is empty, and TempoBPM 0, when the audio has no steady pulse (speech, ambience, silence).
// Confidence is the strength of the tempo peak in [0, 1].
type Beats struct {
	Version     int     `json:"version"`
	SampleRate  int     `json:"sample_rate"`
	SampleCount int64   `json:"sample_count"`
	TempoBPM    float64 `json:"tempo_bpm"`
	Confidence  float64 `json:"confidence"`
	Beats       []int64 `json:"beats"`
}

// ComputeBeats reads 16-bit interleaved samples and finds the beat grid in Go: an onset signal
// from the energy of the first difference of the mono mix (a high-pass that favours attacks),
// a tempo from its autocorrelation under a log-normal prior around 120 BPM, and the beats by
// the dynamic programme of Ellis (2007) that keeps one period between beats while landing on
// strong onsets. No model and no third-party code. The same bytes give the same grid.
func ComputeBeats(ctx context.Context, r io.Reader, channels int) (*Beats, error) {
	if channels < 1 {
		return nil, errors.New("beats: no channels")
	}

	onset, peak, total, err := onsetSignal(ctx, r, channels)
	if err != nil {
		return nil, err
	}

	b := &Beats{Version: BeatsVersion, SampleRate: SampleRate, SampleCount: total, Beats: []int64{}}
	if total > MaxBeatSamples || len(onset) < 4*beatMaxLag {
		return b, nil
	}

	if countPeaks(onset) < beatMinPeaks || normalize(onset) == 0 {
		return b, nil
	}

	period, conf := estimatePeriod(onset)
	b.Confidence = math.Round(conf*100) / 100

	if conf < beatMinConf {
		return b, nil
	}

	idx := trackBeats(onset, period)
	idx = trimWeak(onset, idx)

	if len(idx) < 3 {
		return b, nil
	}

	for _, i := range idx {
		b.Beats = append(b.Beats, min(peak[i], total-1))
	}

	gaps := make([]int64, 0, len(b.Beats)-1)
	for i := 1; i < len(b.Beats); i++ {
		gaps = append(gaps, b.Beats[i]-b.Beats[i-1])
	}

	slices.Sort(gaps)
	b.TempoBPM = math.Round(60*SampleRate/float64(gaps[len(gaps)/2])*10) / 10

	return b, nil
}

// onsetSignal returns, for each block of beatHop sample frames, the rise of the log energy over
// the two blocks before it, summed over two bands, the sample where the block's sharpest attack
// lies, and the number of sample frames read. The high band is the first difference of the mono
// mix, which catches clicks and hats; the low band is the mix after a one-pole low-pass at about
// 200 Hz, which catches kicks and bass. The low band counts fully and the high band at
// beatHighWeight, because the pulse of a piece is carried by the low end more often than by the
// hats between the beats.
func onsetSignal(
	ctx context.Context,
	r io.Reader,
	channels int,
) (onset []float64, peak []int64, total int64, err error) {
	br := bufio.NewReaderSize(r, 1<<20)
	buf := make([]byte, 2*channels*4096)

	var (
		sumHi, sumLo, bestAbs, low float64
		lastMono, bestPos          int64
		inBlock                    int
		hi, lo                     []float64
	)

	for {
		n, rerr := io.ReadFull(br, buf)
		frames := n / (2 * channels)

		for i := range frames {
			var acc int64

			for c := range channels {
				acc += int64(rdS16(buf[(i*channels+c)*2:]))
			}

			mono := acc / int64(channels)
			d := float64(mono - lastMono)
			lastMono = mono
			low += beatLowPass * (float64(mono) - low)

			sumHi += d * d
			sumLo += low * low

			if a := math.Abs(d); a > bestAbs {
				bestAbs, bestPos = a, total
			}

			total++

			inBlock++

			if inBlock == beatHop {
				hi = append(hi, math.Log1p(sumHi/beatHop))
				lo = append(lo, math.Log1p(sumLo/beatHop))
				peak = append(peak, bestPos)
				sumHi, sumLo, bestAbs, inBlock = 0, 0, 0, 0
			}
		}

		if err := ctx.Err(); err != nil {
			return nil, nil, 0, err
		}

		if rerr != nil {
			if errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF) {
				break
			}

			return nil, nil, 0, rerr
		}
	}

	// Before the first block the energy is zero, so an attack in the first blocks counts.
	rise := func(e []float64, i int) float64 {
		before := 0.0
		for k := max(i-2, 0); k < i; k++ {
			before = math.Max(before, e[k])
		}

		return math.Max(0, e[i]-before)
	}

	onset = make([]float64, len(hi))
	for i := range hi {
		onset[i] = rise(lo, i) + beatHighWeight*rise(hi, i)
	}

	return onset, peak, total, nil
}

// normalize scales the signal to a standard deviation of 1 and returns the deviation it had.
func normalize(x []float64) float64 {
	var sum, sq float64

	for _, v := range x {
		sum += v
		sq += v * v
	}

	n := float64(len(x))
	mean := sum / n
	std := math.Sqrt(sq/n - mean*mean)

	if std < 1e-9 {
		return 0
	}

	for i := range x {
		x[i] /= std
	}

	return std
}

// countPeaks counts the onsets that are local maxima and raise the log energy by at least
// beatMinRise (the energy doubles): a pulse needs several, and one attack (a tone starting, a
// single click) is not a rhythm. It reads the signal before it is normalized, so the ripple of
// a steady tone or of noise, which is far below the limit, never counts.
func countPeaks(x []float64) int {
	n := 0

	for i := range x {
		left, right := 0.0, 0.0
		if i > 0 {
			left = x[i-1]
		}

		if i+1 < len(x) {
			right = x[i+1]
		}

		if x[i] >= beatMinRise && x[i] >= left && x[i] > right {
			n++
		}
	}

	return n
}

// estimatePeriod returns the beat period in onset values (fractional) and the strength of its
// autocorrelation peak against the energy of the signal.
func estimatePeriod(x []float64) (float64, float64) {
	var mean float64

	for _, v := range x {
		mean += v
	}

	mean /= float64(len(x))

	var zero float64

	for _, v := range x {
		zero += (v - mean) * (v - mean)
	}

	acf := make([]float64, beatMaxLag+2)

	for lag := beatMinLag - 1; lag <= beatMaxLag+1; lag++ {
		var s float64

		for i := 0; i+lag < len(x); i++ {
			s += (x[i] - mean) * (x[i+lag] - mean)
		}

		acf[lag] = s / float64(len(x)-lag) / (zero / float64(len(x)))
	}

	best, bestScore := beatMinLag, math.Inf(-1)

	for lag := beatMinLag; lag <= beatMaxLag; lag++ {
		z := math.Log2(float64(lag)/beatPriorLag) / beatPriorWidth
		if score := acf[lag] * math.Exp(-0.5*z*z); score > bestScore {
			best, bestScore = lag, score
		}
	}

	period := float64(best)

	if a, b, c := acf[best-1], acf[best], acf[best+1]; b >= a && b >= c && a-2*b+c != 0 {
		period += 0.5 * (a - c) / (a - 2*b + c)
	}

	return period, acf[best]
}

// trackBeats is the dynamic programme: every onset value is a candidate beat, the cost of a
// gap is the squared log of its ratio to the period, and the best chain wins.
func trackBeats(x []float64, period float64) []int {
	n := len(x)
	score := make([]float64, n)
	prev := make([]int, n)
	lo, hi := int(math.Round(period/2)), int(math.Round(2*period))

	for i := range x {
		best, from := 0.0, -1

		for gap := max(lo, 1); gap <= hi && gap <= i; gap++ {
			z := math.Log(float64(gap) / period)
			if s := score[i-gap] - beatTightness*z*z; from < 0 || s > best {
				best, from = s, i-gap
			}
		}

		if from < 0 || best < 0 {
			score[i], prev[i] = x[i], -1

			continue
		}

		score[i], prev[i] = x[i]+best, from
	}

	start := max(n-int(math.Round(period)), 0)
	end := start

	for i := start; i < n; i++ {
		if score[i] > score[end] {
			end = i
		}
	}

	var idx []int

	for i := end; i >= 0; i = prev[i] {
		idx = append(idx, i)
		if prev[i] < 0 {
			break
		}
	}

	for a, b := 0, len(idx)-1; a < b; a, b = a+1, b-1 {
		idx[a], idx[b] = idx[b], idx[a]
	}

	return idx
}

// trimWeak drops beats at the start and at the end where nothing is heard: a chain must stay
// on onsets, and the programme will happily keep marching over a silent intro.
func trimWeak(x []float64, idx []int) []int {
	strong := func(i int) bool {
		for j := max(i-3, 0); j <= min(i+3, len(x)-1); j++ {
			if x[j] >= beatStrongOnset {
				return true
			}
		}

		return false
	}

	for len(idx) > 0 && !strong(idx[0]) {
		idx = idx[1:]
	}

	for len(idx) > 0 && !strong(idx[len(idx)-1]) {
		idx = idx[:len(idx)-1]
	}

	return idx
}

// BeatsOfFile reads a 16-bit WAV file and builds its beat grid.
func BeatsOfFile(ctx context.Context, path string) (*Beats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	w, err := ReadWAV(f)
	if err != nil {
		return nil, err
	}

	if w.Bits != 16 {
		return nil, fmt.Errorf("beats: %d-bit samples", w.Bits)
	}

	if _, err := f.Seek(w.DataOffset, io.SeekStart); err != nil {
		return nil, err
	}

	return ComputeBeats(ctx, io.LimitReader(f, w.DataSize), w.Channels)
}
