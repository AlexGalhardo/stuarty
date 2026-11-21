package ingest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// wavBytes builds a PCM WAV with a LIST chunk before data, so the 44-byte header is not assumed.
func wavBytes(samples []int16, rf64 bool) []byte {
	const channels = 2
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	data := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(s))
	}
	if rf64 {
		b.WriteString("RF64")
		le(uint32(0xFFFFFFFF))
	} else {
		b.WriteString("RIFF")
		le(uint32(0))
	}
	b.WriteString("WAVE")
	if rf64 {
		b.WriteString("ds64")
		le(uint32(28))
		le(uint64(0))
		le(uint64(len(data)))
		le(uint64(len(samples) / channels))
		le(uint32(0))
	}
	b.WriteString("fmt ")
	le(uint32(16))
	le(uint16(1))
	le(uint16(channels))
	le(uint32(48000))
	le(uint32(48000 * 2 * channels))
	le(uint16(2 * channels))
	le(uint16(16))
	b.WriteString("LIST")
	le(uint32(5))
	b.WriteString("abcde\x00") // odd size, padded
	b.WriteString("data")
	if rf64 {
		le(uint32(0xFFFFFFFF))
	} else {
		le(uint32(len(data)))
	}
	b.Write(data)
	return b.Bytes()
}

func ramp(frames int) []int16 {
	s := make([]int16, 2*frames)
	for i := range frames {
		s[2*i] = int16(i%2000 - 1000)
		s[2*i+1] = int16(-(i % 3000))
	}
	return s
}

func TestReadWAVWalksChunks(t *testing.T) {
	for _, rf64 := range []bool{false, true} {
		b := wavBytes(ramp(1000), rf64)
		w, err := ReadWAV(bytes.NewReader(b))
		if err != nil || w.SampleRate != 48000 || w.Channels != 2 || w.Bits != 16 || w.DataSize != 4000 {
			t.Fatalf("rf64=%v: %+v %v", rf64, w, err)
		}
		if got := b[w.DataOffset : w.DataOffset+4]; got[0] != byte(0x18) {
			// sample 0 is -1000 = 0xFC18
			t.Errorf("rf64=%v: data offset points at %x", rf64, got)
		}
	}
	for _, bad := range [][]byte{nil, []byte("RIFF"), []byte("RIFF\x00\x00\x00\x00AVI LIST"), make([]byte, 64)} {
		if _, err := ReadWAV(bytes.NewReader(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestComputePeaksLevelsAndFormat(t *testing.T) {
	const frames = 2_000_003 // not a multiple of any level: the last groups are partial
	s := ramp(frames)
	p, err := ComputePeaks(bytes.NewReader(wavBytes(s, false)[wavDataStart(t, s):]), 2, 48000)
	if err != nil {
		t.Fatal(err)
	}
	if p.SampleCount != frames {
		t.Fatalf("sample count %d", p.SampleCount)
	}
	for i, l := range p.Levels {
		if want := PeakCount(frames, PeakLevels[i]); len(l) != want {
			t.Errorf("level %d: %d peaks, want %d", i, len(l), want)
		}
	}
	// Level 0 peak 0 is the min and max of the first 128 frames over both channels.
	lo, hi := s[0], s[0]
	for _, v := range s[:256] {
		lo, hi = min(lo, v), max(hi, v)
	}
	if p.Levels[0][0] != (PeakPair{lo, hi}) {
		t.Errorf("first peak %+v, want %d..%d", p.Levels[0][0], lo, hi)
	}
	// Each higher level is exactly the min and max of four consecutive peaks below it.
	for l := 1; l < 5; l++ {
		for i, g := range p.Levels[l] {
			lo, hi := int16(32767), int16(-32768)
			for _, q := range p.Levels[l-1][i*4 : min(i*4+4, len(p.Levels[l-1]))] {
				lo, hi = min(lo, q.Min), max(hi, q.Max)
			}
			if g != (PeakPair{lo, hi}) {
				t.Fatalf("level %d peak %d: %+v, want %d..%d", l, i, g, lo, hi)
			}
		}
	}
	if err := p.Check(p.Min, p.Max); err != nil {
		t.Errorf("check of a correct file: %v", err)
	}
	if err := p.Check(p.Min-1, p.Max); err == nil {
		t.Error("a wrong global minimum passed")
	}

	b := p.Marshal()
	if string(b[:4]) != "VKPK" || binary.LittleEndian.Uint16(b[6:]) != 5 ||
		binary.LittleEndian.Uint32(b[8:]) != 48000 ||
		binary.LittleEndian.Uint64(b[16:]) != frames {
		t.Errorf("header %x", b[:24])
	}
	back, err := ParsePeaks(b)
	if err != nil || back.SampleCount != frames || len(back.Levels[4]) != len(p.Levels[4]) ||
		back.Levels[2][7] != p.Levels[2][7] {
		t.Errorf("round trip: %v", err)
	}
	if err := back.Check(p.Min, p.Max); err != nil {
		t.Error(err)
	}
	if _, err := ParsePeaks(b[:100]); err == nil {
		t.Error("a truncated file parsed")
	}
	bad := append([]byte(nil), b...)
	copy(bad, "XXXX")
	if _, err := ParsePeaks(bad); err == nil {
		t.Error("bad magic parsed")
	}
}

func wavDataStart(t *testing.T, s []int16) int {
	t.Helper()
	w, err := ReadWAV(bytes.NewReader(wavBytes(s[:4], false)))
	if err != nil {
		t.Fatal(err)
	}
	return int(w.DataOffset)
}

func TestPeaksOfFileSizeFor60Seconds(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.wav")
	if err := os.WriteFile(p, wavBytes(ramp(48000*60), false), 0o600); err != nil {
		t.Fatal(err)
	}
	pk, err := PeaksOfFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// SDD-011 4.6: 22,500 + 5,625 + 1,407 + 352 + 88 peaks for 2,880,000 samples.
	want := []int{22500, 5625, 1407, 352, 88}
	for i, l := range pk.Levels {
		if len(l) != want[i] {
			t.Errorf("level %d: %d, want %d", i, len(l), want[i])
		}
	}
}

// A request that computes peaks gives the work up when its client goes away (SDD-011 6.5).
func TestPeaksOfFileContextStopsWhenTheContextEnds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(p, wavBytes(ramp(48000*2), false), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if pk, err := PeaksOfFileContext(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("PeaksOfFileContext with an ended context = %v, %v; want context.Canceled", pk, err)
	}

	if _, err := PeaksOfFileContext(t.Context(), p); err != nil {
		t.Fatalf("PeaksOfFileContext: %v", err)
	}
}
