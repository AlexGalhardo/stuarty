package ingest

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"
)

// pcm renders mono 16-bit samples as the little-endian bytes ComputeBeats reads.
func pcm(x []float64) []byte {
	b := make([]byte, 2*len(x))

	for i, v := range x {
		putS16(b[2*i:], int16(math.Round(max(-1, min(1, v))*32000)))
	}

	return b
}

// lcg is a fixed noise source so a fixture is the same on every run.
type lcg uint64

func (l *lcg) next() float64 {
	*l = *l*6364136223846793005 + 1442695040888963407

	return float64(int64(*l>>11))/float64(1<<52) - 1
}

// clickTrack is a 1 kHz click with a 20 ms decay on every beat, from firstMS, and the true beats.
func clickTrack(bpm, secs, firstMS float64) ([]float64, []int64) {
	x := make([]float64, int(secs*SampleRate))

	var truth []int64

	period := 60 / bpm

	for t := firstMS / 1000; t < secs-0.05; t += period {
		at := int64(math.Round(t * SampleRate))
		truth = append(truth, at)

		for k := 0; k < SampleRate/10 && int(at)+k < len(x); k++ {
			s := float64(k) / SampleRate
			x[int(at)+k] += 0.8 * math.Sin(2*math.Pi*1000*s) * math.Exp(-s/0.02)
		}
	}

	return x, truth
}

// drumLoop is a kick on every beat, a hi-hat on the off beats and a steady pad: the beats are
// the kicks.
func drumLoop(bpm, secs float64) ([]float64, []int64) {
	x := make([]float64, int(secs*SampleRate))
	noise, period := lcg(7), 60/bpm

	for i := range x {
		x[i] = 0.12 * math.Sin(2*math.Pi*220*float64(i)/SampleRate)
	}

	var truth []int64

	for n := 0; float64(n)*period < secs-0.3; n++ {
		at := int(math.Round(float64(n) * period * SampleRate))
		truth = append(truth, int64(at))

		phase := 0.0

		for k := 0; k < SampleRate/5 && at+k < len(x); k++ {
			s := float64(k) / SampleRate
			phase += 2 * math.Pi * (50 + 90*math.Exp(-s/0.03)) / SampleRate
			x[at+k] += 0.7 * math.Sin(phase) * math.Exp(-s/0.08)
		}

		hat := at + int(period*SampleRate/2)
		for k := 0; k < SampleRate/50 && hat+k < len(x); k++ {
			x[hat+k] += 0.25 * noise.next() * math.Exp(-float64(k)/SampleRate/0.008)
		}
	}

	return x, truth
}

// hitRate is the share of true beats that have a detected beat within tolMS.
func hitRate(got, truth []int64, tolMS int64) float64 {
	tol, hits := tolMS*SampleRate/1000, 0

	for _, t := range truth {
		for _, g := range got {
			if d := g - t; d >= -tol && d <= tol {
				hits++

				break
			}
		}
	}

	return float64(hits) / float64(len(truth))
}

// TestBeatsClickTrack: on a 120 BPM click track at least 90% of the beats are within 40 ms of the
// true beats (P3-RND-005). The measured rate is recorded in SDD-011 and in the commit.
func TestBeatsClickTrack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		make func() ([]float64, []int64)
		bpm  float64
	}{
		{"click 120", func() ([]float64, []int64) { return clickTrack(120, 30, 250) }, 120},
		{"click 90", func() ([]float64, []int64) { return clickTrack(90, 30, 100) }, 90},
		{"click 150", func() ([]float64, []int64) { return clickTrack(150, 30, 0) }, 150},
		{"drums 120", func() ([]float64, []int64) { return drumLoop(120, 30) }, 120},
		{"drums 100", func() ([]float64, []int64) { return drumLoop(100, 30) }, 100},
		{"drums 128", func() ([]float64, []int64) { return drumLoop(128, 30) }, 128},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			x, truth := c.make()

			b, err := ComputeBeats(context.Background(), bytes.NewReader(pcm(x)), 1)
			if err != nil {
				t.Fatal(err)
			}

			rate := hitRate(b.Beats, truth, 40)
			t.Logf("tempo %.1f (true %.0f), confidence %.2f, %d beats for %d, hit rate %.3f (40 ms), %.3f (20 ms)",
				b.TempoBPM, c.bpm, b.Confidence, len(b.Beats), len(truth), rate, hitRate(b.Beats, truth, 20))

			if rate < 0.9 {
				t.Errorf("hit rate %.3f, want at least 0.9", rate)
			}

			if math.Abs(b.TempoBPM-c.bpm) > 2 {
				t.Errorf("tempo %.1f, want %.0f within 2 BPM", b.TempoBPM, c.bpm)
			}

			if b.SampleCount != int64(len(x)) {
				t.Errorf("sample_count %d, want %d", b.SampleCount, len(x))
			}
		})
	}
}

// Stereo input is mixed down: the same grid as the mono signal.
func TestBeatsStereoEqualsMono(t *testing.T) {
	t.Parallel()

	x, _ := drumLoop(120, 12)
	st := make([]byte, 0, 4*len(x))

	for _, v := range x {
		s := pcm([]float64{v})
		st = append(st, s...)
		st = append(st, s...)
	}

	mono, err := ComputeBeats(context.Background(), bytes.NewReader(pcm(x)), 1)
	if err != nil {
		t.Fatal(err)
	}

	both, err := ComputeBeats(context.Background(), bytes.NewReader(st), 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(mono.Beats) == 0 || len(mono.Beats) != len(both.Beats) {
		t.Fatalf("mono %d beats, stereo %d", len(mono.Beats), len(both.Beats))
	}

	for i := range mono.Beats {
		if mono.Beats[i] != both.Beats[i] {
			t.Fatalf("beat %d: %d against %d", i, mono.Beats[i], both.Beats[i])
		}
	}
}

// Audio with no steady pulse gets an empty grid: silence, a tone, noise and a single click.
func TestBeatsNoPulse(t *testing.T) {
	t.Parallel()

	noise, white := lcg(3), make([]float64, 20*SampleRate)
	for i := range white {
		white[i] = 0.3 * noise.next()
	}

	tone := make([]float64, 20*SampleRate)
	for i := range tone {
		tone[i] = 0.5 * math.Sin(2*math.Pi*440*float64(i)/SampleRate)
	}

	one, _ := clickTrack(120, 20, 5000)
	one = one[:6*SampleRate]
	one = append(one, make([]float64, 14*SampleRate)...)

	for name, x := range map[string][]float64{
		"silence": make([]float64, 20*SampleRate), "tone": tone, "noise": white, "one click": one,
	} {
		b, err := ComputeBeats(context.Background(), bytes.NewReader(pcm(x)), 1)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("%s: confidence %.2f, %d beats", name, b.Confidence, len(b.Beats))

		if len(b.Beats) != 0 || b.TempoBPM != 0 {
			t.Errorf("%s: %d beats at %.1f BPM, want none", name, len(b.Beats), b.TempoBPM)
		}
	}
}

// The same bytes give the same grid, and a cancelled context stops the read.
func TestBeatsDeterministicAndCancel(t *testing.T) {
	t.Parallel()

	x, _ := drumLoop(120, 10)
	raw := pcm(x)

	a, _ := ComputeBeats(context.Background(), bytes.NewReader(raw), 1)
	b, _ := ComputeBeats(context.Background(), bytes.NewReader(raw), 1)

	ja, jb := binary.Size(a.Beats), binary.Size(b.Beats)
	if ja != jb || len(a.Beats) == 0 {
		t.Fatalf("grids differ: %d and %d", ja, jb)
	}

	for i := range a.Beats {
		if a.Beats[i] != b.Beats[i] {
			t.Fatalf("beat %d differs", i)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ComputeBeats(ctx, bytes.NewReader(raw), 1); err == nil {
		t.Error("a cancelled context did not stop the analysis")
	}
}

func TestBeatsRefusesNoChannels(t *testing.T) {
	t.Parallel()

	if _, err := ComputeBeats(context.Background(), bytes.NewReader(nil), 0); err == nil {
		t.Error("0 channels accepted")
	}
}
