package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func TestGainDB(t *testing.T) {
	t.Parallel()
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name         string
		i, tp        *float64
		samples      int64
		target, want float64
	}{
		{"boost", f(-20), f(-12), 48000, -14, 6},
		{"cut", f(-8), f(-3), 48000, -14, -6},
		{"the limiter takes the excess over the ceiling", f(-20), f(-4), 48000, -14, 6},
		{"the limit of the limiter wins over the boost", f(-30), f(-4), 48000, -14, 2.5 + MaxLimitingDB},
		{"silent", nil, nil, 48000, -14, 0},
		{"too short", f(-30), f(-20), MinMeasuredSamples - 1, -14, 0},
		{"exactly 0.4 s", f(-20), f(-20), MinMeasuredSamples, -14, 6},
		{"infinite peak", f(-20), f(math.Inf(-1)), 48000, -14, 0},
	}
	for _, c := range cases {
		if got := GainDB(c.i, c.tp, c.samples, c.target); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func parseHeader(t *testing.T, b []byte) (format, bits, rate, channels uint16, data uint32) {
	t.Helper()
	if string(b[0:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[36:40]) != "data" {
		t.Fatalf("not a canonical WAV header: %q", b[:44])
	}
	return binary.LittleEndian.Uint16(b[20:]), binary.LittleEndian.Uint16(b[34:]),
		uint16(binary.LittleEndian.Uint32(b[24:]) / 1000), binary.LittleEndian.Uint16(b[22:]),
		binary.LittleEndian.Uint32(b[40:])
}

func TestFloatWAVThenGainToPCM16(t *testing.T) {
	t.Parallel()
	const n = 5000
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", n)}}
	srcs := []plan.AudioSource{{ItemID: "1", AssetID: "a", Samples: n, Gain: flat(1)}}

	var f32 bytes.Buffer
	if err := MixToFloatWAV(context.Background(), srcs, n, o, &f32); err != nil {
		t.Fatal(err)
	}
	if format, bits, rate, ch, data := parseHeader(
		t,
		f32.Bytes(),
	); format != 3 || bits != 32 || rate != 48 || ch != 2 ||
		data != n*8 ||
		f32.Len() != 44+n*8 {
		t.Errorf(
			"float header: format %d bits %d rate %d ch %d data %d len %d",
			format,
			bits,
			rate,
			ch,
			data,
			f32.Len(),
		)
	}

	var pcm bytes.Buffer
	if err := GainToPCM16WAV(bytes.NewReader(f32.Bytes()), &pcm, n, 0); err != nil {
		t.Fatal(err)
	}
	if format, bits, _, _, data := parseHeader(t, pcm.Bytes()); format != 1 || bits != 16 || data != n*4 {
		t.Errorf("pcm header: format %d bits %d data %d", format, bits, data)
	}
	// Unity gain gives back the decoded samples.
	if !bytes.Equal(pcm.Bytes()[44:], o.streams["a"]) {
		t.Error("a mix at unity gain is not the source")
	}

	// -6.0206 dB halves the samples.
	var half bytes.Buffer
	if err := GainToPCM16WAV(bytes.NewReader(f32.Bytes()), &half, n, -20*math.Log10(2)); err != nil {
		t.Fatal(err)
	}
	a := int16(binary.LittleEndian.Uint16(o.streams["a"][40:]))
	h := int16(binary.LittleEndian.Uint16(half.Bytes()[44+40:]))
	if d := int(a)/2 - int(h); d < -1 || d > 1 {
		t.Errorf("half of %d is %d", a, h)
	}

	// A boost that passes full scale clamps.
	var loud bytes.Buffer
	if err := GainToPCM16WAV(bytes.NewReader(f32.Bytes()), &loud, n, 40); err != nil {
		t.Fatal(err)
	}
	if v := int16(
		binary.LittleEndian.Uint16(loud.Bytes()[44+4*100:]),
	); v != math.MaxInt16 &&
		v != math.MinInt16 {
		t.Errorf("clamped sample = %d", v)
	}

	if err := GainToPCM16WAV(bytes.NewReader(f32.Bytes()[:100]), &pcm, n, 0); err == nil {
		t.Error("a short float mix was accepted")
	}
}

func TestSilentWAV(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	if err := SilentWAV(&b, 100000); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 44+400000 || !bytes.Equal(b.Bytes()[44:48], []byte{0, 0, 0, 0}) {
		t.Errorf("silent WAV is %d bytes", b.Len())
	}
	if _, _, _, _, data := parseHeader(t, b.Bytes()); data != 400000 {
		t.Errorf("data size %d", data)
	}
}
