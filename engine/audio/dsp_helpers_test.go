package audio

import (
	"bytes"
	"flag"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

var update = flag.Bool("update", false, "rewrite the golden WAV files of the processors")

func sine(freq, amp float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = amp * math.Sin(2*math.Pi*freq*float64(i)/SampleRate)
	}

	return out
}

// noise is uniform white noise of the given RMS from a seeded generator, the same on every system.
func noise(seed uint64, rms float64, n int) []float64 {
	g := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	out := make([]float64, n)

	for i := range out {
		out[i] = (g.Float64()*2 - 1) * rms * math.Sqrt(3)
	}

	return out
}

func add(a, b []float64) []float64 {
	out := make([]float64, len(a))
	for i := range out {
		out[i] = a[i] + b[i]
	}

	return out
}

func rms(x []float64) float64 {
	var sum float64
	for _, v := range x {
		sum += v * v
	}

	return math.Sqrt(sum / float64(len(x)))
}

func db(x float64) float64 { return 20 * math.Log10(x) }

func spec(kind string, kv ...any) plan.ProcessorSpec {
	p := timeline.Processor{Kind: kind}
	for i := 0; i < len(kv); i += 2 {
		if p.Params == nil {
			p.Params = map[string]float64{}
		}

		switch v := kv[i+1].(type) {
		case int:
			p.Params[kv[i].(string)] = float64(v)
		case float64:
			p.Params[kv[i].(string)] = v
		}
	}

	return plan.ProcessorSpec{Kind: kind, Params: p.FullParams()}
}

// through runs a chain over the same signal on both channels and returns the left channel, aligned with the input.
func through(t *testing.T, in []float64, specs ...plan.ProcessorSpec) []float64 {
	t.Helper()

	l, _ := throughStereo(t, in, in, 0, specs...)

	return l
}

// throughStereo runs the chain with the runner, in calls of the given size (0 is one call).
func throughStereo(t *testing.T, inL, inR []float64, call int, specs ...plan.ProcessorSpec) (l, r []float64) {
	t.Helper()

	chain, err := NewChain(specs, nil)
	if err != nil {
		t.Fatal(err)
	}

	return runChain(t, chain, inL, inR, call)
}

func runChain(t *testing.T, chain *Chain, inL, inR []float64, call int) (l, r []float64) {
	t.Helper()

	n := len(inL)
	pos := 0
	run := &runner{chain: chain, pull: func(pl, pr []float64) error {
		for i := range pl {
			pl[i], pr[i] = 0, 0
			if pos < n {
				pl[i], pr[i] = inL[pos], inR[pos]
			}

			pos++
		}

		return nil
	}}

	l, r = make([]float64, n), make([]float64, n)
	if call <= 0 {
		call = n
	}

	for at := 0; at < n; at += call {
		end := min(at+call, n)
		if err := run.next(l[at:end], r[at:end]); err != nil {
			t.Fatal(err)
		}
	}

	return l, r
}

// goldenTolerance is how far a sample of a golden file may be from the one computed: none on the system that wrote the
// files, two steps of the 16-bit scale elsewhere (SDD-004 section 7.1).
func goldenTolerance() int {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return 0
	}

	return 2
}

// golden compares the 16-bit WAV of the stereo signal with testdata/<name>.wav, or writes it with -update.
func golden(t *testing.T, name string, l, r []float64) {
	t.Helper()

	frames := int64(len(l))

	head, err := wavHeader(frames, false)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	buf.Write(head)

	for i := range l {
		for _, v := range []float64{l[i], r[i]} {
			p := pcm16(math.Floor(v*32768 + 0.5))
			buf.WriteByte(byte(p))
			buf.WriteByte(byte(p >> 8))
		}
	}

	path := filepath.Join("testdata", name+".wav")

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.Bytes()
	if len(got) != len(want) {
		t.Fatalf("%s: %d bytes, want %d", path, len(got), len(want))
	}

	tol := goldenTolerance()

	for i := 44; i+1 < len(got); i += 2 {
		a := int(int16(uint16(got[i]) | uint16(got[i+1])<<8))
		b := int(int16(uint16(want[i]) | uint16(want[i+1])<<8))

		if d := a - b; d > tol || d < -tol {
			t.Fatalf("%s: sample %d is %d, want %d (tolerance %d)", path, (i-44)/2, a, b, tol)
		}
	}
}

// fixture is half a second of a voice-like signal: a low tone, a mid tone and a high tone over a little noise.
func fixture(n int) (l, r []float64) {
	l = add(add(sine(110, 0.2, n), sine(1000, 0.2, n)), add(sine(6500, 0.1, n), noise(7, 0.01, n)))
	r = make([]float64, n)

	for i := range r {
		r[i] = l[i] * 0.8
	}

	return l, r
}
