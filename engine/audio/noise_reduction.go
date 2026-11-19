package audio

import "math"

// Spectral noise reduction (SDD-004 section 7.1): short-time spectral subtraction with a noise profile that is
// either estimated as the audio plays (the minimum of the smoothed power of each bin) or measured once from a range
// the user marked. It needs no model and no dependency.

const (
	// nrFrame is the analysis frame and nrHop the step between frames; the stage delays its output by nrFrame
	// samples, which the chain compensates.
	nrFrame = 1024
	nrHop   = nrFrame / 2
	nrBins  = nrFrame/2 + 1

	nrSmooth       = 0.8 // S = 0.8 S + 0.2 P, the smoothed power the minimum is taken of
	nrWindowFrames = 20  // frames of one window of the running minimum (about 213 ms)
	nrWindows      = 8   // windows kept (about 1.7 s)
	nrBias         = 2.5 // the noise power is this times the minimum of the smoothed power
	nrGainSmooth   = 0.6 // G = 0.6 Gprev + 0.4 G over time
)

// fft is a radix-2 complex transform with twiddles from math.Sincos.
type fft struct {
	n        int
	cos, sin []float64
	rev      []int
}

func newFFT(n int) *fft {
	f := &fft{n: n, cos: make([]float64, n/2), sin: make([]float64, n/2), rev: make([]int, n)}
	for k := range n / 2 {
		s, c := math.Sincos(2 * math.Pi * float64(k) / float64(n))
		f.cos[k], f.sin[k] = c, s
	}

	bits := 0
	for 1<<bits < n {
		bits++
	}

	for i := range n {
		r := 0
		for b := range bits {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}

		f.rev[i] = r
	}

	return f
}

// transform runs the transform in place; the inverse is divided by n.
func (f *fft) transform(re, im []float64, inverse bool) {
	for i, j := range f.rev {
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}

	sign := -1.0
	if inverse {
		sign = 1
	}

	for size := 2; size <= f.n; size <<= 1 {
		half, step := size/2, f.n/size

		for i := 0; i < f.n; i += size {
			for j := range half {
				wr, wi := f.cos[j*step], sign*f.sin[j*step]
				a, b := i+j, i+j+half
				vr := mul(re[b], wr) - mul(im[b], wi)
				vi := mul(re[b], wi) + mul(im[b], wr)
				re[b], im[b] = re[a]-vr, im[a]-vi
				re[a], im[a] = re[a]+vr, im[a]+vi
			}
		}
	}

	if inverse {
		for i := range f.n {
			re[i] /= float64(f.n)
			im[i] /= float64(f.n)
		}
	}
}

// sqrtHann is the periodic square-root Hann window: its overlapped squares sum to 1 at a hop of half a frame.
func sqrtHann() []float64 {
	w := make([]float64, nrFrame)
	for i := range w {
		w[i] = math.Sqrt(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/nrFrame))
	}

	return w
}

// noiseProfile is the mean power of each bin over a range of audio.
type noiseProfile []float64

// measureNoiseProfile is the mean power per bin of the frames of the stereo samples l and r, or nil when they are
// shorter than a frame.
func measureNoiseProfile(l, r []float64) noiseProfile {
	if len(l) < nrFrame {
		return nil
	}

	var (
		win      = sqrtHann()
		tr       = newFFT(nrFrame)
		prof     = make(noiseProfile, nrBins)
		re, im   = make([]float64, nrFrame), make([]float64, nrFrame)
		re2, im2 = make([]float64, nrFrame), make([]float64, nrFrame)
		frames   int
	)

	for at := 0; at+nrFrame <= len(l); at += nrHop {
		for k := range nrFrame {
			re[k], im[k] = mul(l[at+k], win[k]), 0
			re2[k], im2[k] = mul(r[at+k], win[k]), 0
		}

		tr.transform(re, im, false)
		tr.transform(re2, im2, false)

		for k := range nrBins {
			prof[k] += (mul(re[k], re[k]) + mul(im[k], im[k]) + mul(re2[k], re2[k]) + mul(im2[k], im2[k])) / 2
		}

		frames++
	}

	for k := range prof {
		prof[k] /= float64(frames)
	}

	return prof
}

// noiseReduction is the stage.
type noiseReduction struct {
	strength, over, floor float64
	fixed                 noiseProfile // nil: estimated

	win []float64
	tr  *fft

	in    [2][]float64 // the frame being filled
	fill  int
	acc   [2][]float64 // overlap-add accumulators, one frame long
	queue [2][]float64 // finished samples, the first nrFrame of them the zeros that prime the delay
	head  int

	re, im   [2][]float64
	gain     []float64 // the time-smoothed gain of each bin
	smoothed []float64

	// The running estimate of the noise: the smoothed power, the minimum of the current window and the minima of the
	// last windows.
	s, cur  []float64
	windows [nrWindows][]float64
	nWin    int
	inWin   int
	frames  int
}

func newNoiseReduction(p map[string]float64, fixed noiseProfile) *noiseReduction {
	strength := p["strength"]
	n := &noiseReduction{
		strength: strength,
		over:     1 + 2*strength,
		floor:    fromDB(-(6 + 24*strength)),
		fixed:    fixed,
		win:      sqrtHann(),
		tr:       newFFT(nrFrame),
		gain:     make([]float64, nrBins),
		s:        make([]float64, nrBins),
		cur:      make([]float64, nrBins),

		smoothed: make([]float64, nrBins),
	}

	for c := range 2 {
		n.in[c] = make([]float64, nrFrame)
		n.acc[c] = make([]float64, nrFrame)
		n.queue[c] = make([]float64, nrFrame, 4*nrFrame)
		n.re[c], n.im[c] = make([]float64, nrFrame), make([]float64, nrFrame)
	}

	for k := range n.gain {
		n.gain[k] = 1
	}

	return n
}

func (n *noiseReduction) latency() int { return nrFrame }

func (n *noiseReduction) process(l, r []float64) {
	for i := range l {
		n.in[0][n.fill], n.in[1][n.fill] = l[i], r[i]
		n.fill++

		if n.fill == nrFrame {
			n.frame()
			copy(n.in[0], n.in[0][nrHop:])
			copy(n.in[1], n.in[1][nrHop:])
			n.fill = nrHop
		}

		l[i], r[i] = n.queue[0][n.head], n.queue[1][n.head]
		n.head++
	}

	if n.head >= 2*nrFrame {
		for c := range 2 {
			n.queue[c] = append(n.queue[c][:0], n.queue[c][n.head:]...)
		}

		n.head = 0
	}
}

// noise is the noise power of each bin for the frame whose power is p.
func (n *noiseReduction) noise(p []float64) []float64 {
	if n.fixed != nil {
		return n.fixed
	}

	for k := range nrBins {
		if n.frames == 0 {
			n.s[k], n.cur[k] = p[k], p[k]
		} else {
			n.s[k] = mul(nrSmooth, n.s[k]) + mul(1-nrSmooth, p[k])
			n.cur[k] = math.Min(n.cur[k], n.s[k])
		}
	}

	n.frames++
	n.inWin++

	if n.inWin == nrWindowFrames {
		n.windows[n.nWin%nrWindows] = append(n.windows[n.nWin%nrWindows][:0], n.cur...)
		n.nWin++
		n.inWin = 0

		copy(n.cur, n.s)
	}

	out := n.smoothed
	for k := range nrBins {
		m := n.cur[k]
		for w := range min(n.nWin, nrWindows) {
			m = math.Min(m, n.windows[w][k])
		}

		out[k] = mul(nrBias, m)
	}

	return out
}

// frame analyses the frame in n.in, filters it and adds it to the accumulators; the first nrHop samples of the
// accumulators are then finished.
func (n *noiseReduction) frame() {
	for c := range 2 {
		for k := range nrFrame {
			n.re[c][k], n.im[c][k] = mul(n.in[c][k], n.win[k]), 0
		}

		n.tr.transform(n.re[c], n.im[c], false)
	}

	power := make([]float64, nrBins)
	for k := range nrBins {
		power[k] = (mul(n.re[0][k], n.re[0][k]) + mul(n.im[0][k], n.im[0][k]) +
			mul(n.re[1][k], n.re[1][k]) + mul(n.im[1][k], n.im[1][k])) / 2
	}

	z := n.noise(power)

	for k := range nrBins {
		g := n.floor
		if power[k] > 0 {
			g = math.Max(math.Sqrt(math.Max(power[k]-mul(n.over, z[k]), 0)/power[k]), n.floor)
		}

		n.gain[k] = follow(n.gain[k], g, nrGainSmooth)
	}

	for k := range nrFrame {
		bin := k
		if bin >= nrBins {
			bin = nrFrame - k
		}

		lo, hi := max(bin-1, 0), min(bin+1, nrBins-1)
		g := mul(0.25, n.gain[lo]) + mul(0.5, n.gain[bin]) + mul(0.25, n.gain[hi])
		g = math.Max(g, n.floor)

		for c := range 2 {
			n.re[c][k] = mul(n.re[c][k], g)
			n.im[c][k] = mul(n.im[c][k], g)
		}
	}

	for c := range 2 {
		n.tr.transform(n.re[c], n.im[c], true)

		for k := range nrFrame {
			n.acc[c][k] += mul(n.re[c][k], n.win[k])
		}

		n.queue[c] = append(n.queue[c], n.acc[c][:nrHop]...)
		copy(n.acc[c], n.acc[c][nrHop:])
		clear(n.acc[c][nrHop:])
	}
}
