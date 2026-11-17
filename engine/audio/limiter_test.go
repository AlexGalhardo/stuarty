package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func floatMix(frames int, sample func(i int) float32) []byte {
	var b bytes.Buffer
	h, _ := wavHeader(int64(frames), true)
	b.Write(h)
	for i := range frames {
		v := math.Float32bits(sample(i))
		_ = binary.Write(&b, binary.LittleEndian, v)
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	return b.Bytes()
}

func TestLimitToPCM16WAVHoldsTheCeiling(t *testing.T) {
	t.Parallel()
	const n = 48000
	// A quiet tone with three sharp peaks at -3 dBFS.
	in := floatMix(n, func(i int) float32 {
		if i%12000 == 6000 {
			return 0.7
		}
		return float32(0.05 * math.Sin(float64(i)/20))
	})

	var out bytes.Buffer
	if err := LimitToPCM16WAV(bytes.NewReader(in), &out, n, 12); err != nil {
		t.Fatal(err)
	}
	if got, want := out.Len(), 44+n*4; got != want {
		t.Fatalf("%d bytes, want %d", got, want)
	}

	ceiling := math.Pow(10, LimiterCeilingDBFS/20) * 32768
	var quiet float64
	for i := range n {
		l := math.Abs(float64(int16(binary.LittleEndian.Uint16(out.Bytes()[44+i*4:]))))
		if l > ceiling+1 {
			t.Fatalf("frame %d is %.0f, over the ceiling %.0f", i, l, ceiling)
		}
		if i == 100 {
			quiet = l
		}
	}
	// The quiet part before the first peak keeps its gain (+12 dB, x3.98 of 0.05*sin(5)).
	if want := 0.05 * math.Abs(math.Sin(100.0/20)) * math.Pow(10, 12.0/20) * 32768; math.Abs(quiet-want) > 2 {
		t.Errorf("quiet frame %.0f, want about %.0f", quiet, want)
	}
}

func TestLimitToPCM16WAVLeavesAMixUnderTheCeiling(t *testing.T) {
	t.Parallel()
	const n = 10000
	in := floatMix(n, func(i int) float32 { return float32(0.1 * math.Sin(float64(i)/7)) })

	var plain, limited bytes.Buffer
	if err := GainToPCM16WAV(bytes.NewReader(in), &plain, n, 6); err != nil {
		t.Fatal(err)
	}
	if err := LimitToPCM16WAV(bytes.NewReader(in), &limited, n, 6); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain.Bytes(), limited.Bytes()) {
		t.Error("the limiter changed a mix that does not reach the ceiling")
	}
}
