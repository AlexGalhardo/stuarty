package audio

import "math"

// The audio processors of SDD-004 section 7.1: pure functions over stereo frames with their state held in the
// value, so the preview and the export run the same code and the result does not depend on how a stream is cut into
// calls. Every product goes through mul, so no fused multiply-add changes a bit between systems.

// stage is one processor of a chain.
type stage interface {
	// process runs the stage in place over the frames l[i], r[i].
	process(l, r []float64)
	// latency is how many frames the stage delays its output; the chain compensates it.
	latency() int
}

const (
	maxFilterHz  = 21600 // the highest frequency a biquad is designed for
	silenceFloor = 1e-9  // the level that dB() reports as -180 dBFS
)

func dB(x float64) float64 { return 20 * math.Log10(math.Max(x, silenceFloor)) }

func fromDB(db float64) float64 { return math.Pow(10, db/20) }

// coef is the one-pole coefficient of a time in milliseconds at 48 kHz; 0 ms is 0 (instant).
func coef(ms float64) float64 {
	if ms <= 0 {
		return 0
	}

	return math.Exp(-1 / (ms * SampleRate / 1000))
}

// follow moves v to target with the coefficient k: v = k v + (1 - k) target.
func follow(v, target, k float64) float64 { return mul(k, v) + mul(1-k, target) }

// biquad is a second-order filter in transposed direct form II, with the state of both channels.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             [2]float64
}

func (q *biquad) run(c int, x float64) float64 {
	y := mul(q.b0, x) + q.z1[c]
	q.z1[c] = mul(q.b1, x) - mul(q.a1, y) + q.z2[c]
	q.z2[c] = mul(q.b2, x) - mul(q.a2, y)

	return y
}

// newBiquad divides the cookbook coefficients by a0.
func newBiquad(b0, b1, b2, a0, a1, a2 float64) *biquad {
	return &biquad{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0}
}

func cookbook(f, q float64) (c, alpha float64) {
	w := 2 * math.Pi * math.Min(f, maxFilterHz) / SampleRate
	s, c := math.Sincos(w)

	return c, s / (2 * q)
}

func peakingBiquad(f, gainDB, q float64) *biquad {
	c, alpha := cookbook(f, q)
	a := math.Pow(10, gainDB/40)

	return newBiquad(1+alpha*a, -2*c, 1-alpha*a, 1+alpha/a, -2*c, 1-alpha/a)
}

func lowpassBiquad(f, q float64) *biquad {
	c, alpha := cookbook(f, q)

	return newBiquad((1-c)/2, 1-c, (1-c)/2, 1+alpha, -2*c, 1-alpha)
}

func highpassBiquad(f, q float64) *biquad {
	c, alpha := cookbook(f, q)

	return newBiquad((1+c)/2, -(1 + c), (1+c)/2, 1+alpha, -2*c, 1-alpha)
}

func bandpassBiquad(f, q float64) *biquad {
	c, alpha := cookbook(f, q)

	return newBiquad(alpha, 0, -alpha, 1+alpha, -2*c, 1-alpha)
}

const butterworthQ = math.Sqrt2 / 2

// equaliser is the high-pass, the low-pass and the bands, in that order.
type equaliser struct{ filters []*biquad }

func newEqualiser(p map[string]float64) *equaliser {
	e := &equaliser{}
	if f := p["hp_hz"]; f > 0 {
		e.filters = append(e.filters, highpassBiquad(f, butterworthQ))
	}

	if f := p["lp_hz"]; f > 0 {
		e.filters = append(e.filters, lowpassBiquad(f, butterworthQ))
	}

	for _, n := range []string{"b1", "b2", "b3", "b4", "b5", "b6"} {
		if g := p[n+"_gain_db"]; g != 0 {
			e.filters = append(e.filters, peakingBiquad(p[n+"_hz"], g, p[n+"_q"]))
		}
	}

	return e
}

func (e *equaliser) latency() int { return 0 }

func (e *equaliser) process(l, r []float64) {
	for i := range l {
		for _, q := range e.filters {
			l[i] = q.run(0, l[i])
			r[i] = q.run(1, r[i])
		}
	}
}

// compressor is a feed-forward, linked, soft-knee compressor.
type compressor struct {
	threshold, ratio, knee, makeup float64
	attack, release                float64
	g                              float64 // the smoothed reduction in dB, 0 or below
}

func newCompressor(p map[string]float64) *compressor {
	return &compressor{
		threshold: p["threshold_db"], ratio: p["ratio"], knee: p["knee_db"], makeup: p["makeup_db"],
		attack: coef(p["attack_ms"]), release: coef(p["release_ms"]),
	}
}

func (c *compressor) latency() int { return 0 }

// reduction is the wanted gain change in dB for a level in dB: 0 or below.
func (c *compressor) reduction(level float64) float64 {
	o := level - c.threshold
	w := c.knee

	var y float64

	switch {
	case 2*o < -w:
		return 0
	case w > 0 && 2*math.Abs(o) <= w:
		y = level + mul(1/c.ratio-1, mul(o+w/2, o+w/2))/(2*w)
	default:
		y = c.threshold + o/c.ratio
	}

	return y - level
}

func (c *compressor) process(l, r []float64) {
	for i := range l {
		want := c.reduction(dB(math.Max(math.Abs(l[i]), math.Abs(r[i]))))

		k := c.release
		if want < c.g {
			k = c.attack
		}

		c.g = follow(c.g, want, k)
		gain := fromDB(c.g + c.makeup)
		l[i] = mul(l[i], gain)
		r[i] = mul(r[i], gain)
	}
}

// limiter is an instant-attack peak limiter: no sample passes the ceiling, and the gain returns to 1 with the release.
type limiter struct {
	ceiling, release, g float64
}

func newLimiter(p map[string]float64) *limiter {
	return &limiter{ceiling: fromDB(p["ceiling_db"]), release: coef(p["release_ms"]), g: 1}
}

func (m *limiter) latency() int { return 0 }

func (m *limiter) process(l, r []float64) {
	for i := range l {
		need := 1.0
		if p := math.Max(math.Abs(l[i]), math.Abs(r[i])); p > m.ceiling {
			need = m.ceiling / p
		}

		m.g = math.Min(need, follow(m.g, 1, m.release))
		l[i] = mul(l[i], m.g)
		r[i] = mul(r[i], m.g)
	}
}

// gate lowers the signal by range_db while the level is under the threshold.
type gate struct {
	threshold, floor, attack, release, decay float64
	holdSamples                              int
	env, g                                   float64
	open                                     bool
	hold                                     int
}

const gateHysteresisDB = 3

func newGate(p map[string]float64) *gate {
	return &gate{
		threshold: p["threshold_db"], floor: fromDB(p["range_db"]),
		attack: coef(p["attack_ms"]), release: coef(p["release_ms"]), decay: coef(10),
		holdSamples: int(math.Round(p["hold_ms"] * SampleRate / 1000)),
		g:           fromDB(p["range_db"]),
	}
}

func (t *gate) latency() int { return 0 }

func (t *gate) process(l, r []float64) {
	for i := range l {
		t.env = math.Max(math.Max(math.Abs(l[i]), math.Abs(r[i])), mul(t.decay, t.env))
		level := dB(t.env)

		if level >= t.threshold {
			t.open = true
		}

		if t.open {
			switch {
			case level >= t.threshold-gateHysteresisDB:
				t.hold = t.holdSamples
			case t.hold > 0:
				t.hold--
			default:
				t.open = false
			}
		}

		target, k := t.floor, t.release
		if t.open {
			target, k = 1, t.attack
		}

		t.g = follow(t.g, target, k)
		l[i] = mul(l[i], t.g)
		r[i] = mul(r[i], t.g)
	}
}

// deesser lowers only the band around the frequency when its level passes the threshold.
type deesser struct {
	band                   *biquad
	threshold, slope       float64
	rise, fall, gUp, gDown float64
	env, h                 float64
}

const deesserQ = 1.2

func newDeesser(p map[string]float64) *deesser {
	return &deesser{
		band: bandpassBiquad(p["frequency_hz"], deesserQ), threshold: p["threshold_db"], slope: 1 - 1/p["ratio"],
		rise: coef(0.5), fall: coef(15), gUp: coef(30), gDown: coef(1), h: 1,
	}
}

func (d *deesser) latency() int { return 0 }

func (d *deesser) process(l, r []float64) {
	for i := range l {
		bl, br := d.band.run(0, l[i]), d.band.run(1, r[i])
		p := math.Max(math.Abs(bl), math.Abs(br))

		if p > d.env {
			d.env = follow(d.env, p, d.rise)
		} else {
			d.env = mul(d.fall, d.env)
		}

		want := 1.0
		if level := dB(d.env); level > d.threshold {
			want = fromDB(-mul(level-d.threshold, d.slope))
		}

		k := d.gUp
		if want < d.h {
			k = d.gDown
		}

		d.h = follow(d.h, want, k)
		l[i] -= mul(1-d.h, bl)
		r[i] -= mul(1-d.h, br)
	}
}
