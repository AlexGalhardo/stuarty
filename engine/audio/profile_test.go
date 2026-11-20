package audio

import (
	"context"
	"encoding/binary"
	"io"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// patternPCM is a stream of 16-bit stereo frames of a fixed pseudo-random pattern, made as it is read, so that a long
// stream costs the test no memory.
type patternPCM struct{ left, i int64 }

func (p *patternPCM) sample() (int16, int16) {
	v := int16((p.i*7919)%2001 - 1000)

	return v, -v / 2
}

func (p *patternPCM) Read(b []byte) (int, error) {
	if p.left == 0 {
		return 0, io.EOF
	}

	n := 0
	for n+BytesPerSample <= len(b) && p.left > 0 {
		l, r := p.sample()
		binary.LittleEndian.PutUint16(b[n:], uint16(l))
		binary.LittleEndian.PutUint16(b[n+2:], uint16(r))
		n += BytesPerSample
		p.i++
		p.left--
	}

	return n, nil
}

func (p *patternPCM) Close() error { return nil }

func patternOpener() Opener {
	return OpenerFunc(func(_ context.Context, src plan.AudioSource) (io.ReadCloser, error) {
		return &patternPCM{left: src.Samples}, nil
	})
}

// A marked noise profile is measured as the range is decoded: the result is the one of the whole range in memory,
// and an hour-long range (the largest a document may mark) is not held in memory at once.
func TestMarkedProfileIsStreamed(t *testing.T) {
	ctx := context.Background()

	got, err := markedProfile(ctx, patternOpener(), plan.AudioSource{ItemID: "a"}, 0, 500)
	if err != nil {
		t.Fatal(err)
	}

	src := &patternPCM{}
	l, r := make([]float64, 24000), make([]float64, 24000)

	for i := range l {
		a, b := src.sample()
		l[i], r[i] = float64(a)/32768, float64(b)/32768
		src.i++
	}

	want := measureNoiseProfile(l, r)
	if len(got) != nrBins || len(want) != nrBins {
		t.Fatalf("profiles of %d and %d bins", len(got), len(want))
	}

	for k := range want {
		if got[k] != want[k] {
			t.Fatalf("bin %d: streamed %v, in memory %v", k, got[k], want[k])
		}
	}

	// Two minutes of a marked range: held in memory it would take 115 MB (the bytes and two slices of floats).
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	if _, err := markedProfile(ctx, patternOpener(), plan.AudioSource{ItemID: "a"}, 0, 120_000); err != nil {
		t.Fatal(err)
	}

	runtime.ReadMemStats(&after)

	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20 {
		t.Errorf("measuring a two-minute profile allocated %d bytes", grew)
	}
}
