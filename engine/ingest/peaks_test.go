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

// A long file reports how far it is, in growing steps that end within the size of its data (SDD-011 6.3.1).
func TestPeaksOfFileProgressReportsGrowingSteps(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(p, wavBytes(ramp(48000*60), false), 0o600); err != nil {
		t.Fatal(err)
	}

	var seen []int64

	var total int64

	pk, err := PeaksOfFileProgress(t.Context(), p, func(done, size int64) {
		seen, total = append(seen, done), size
	})
	if err != nil || pk.SampleCount != 48000*60 {
		t.Fatalf("PeaksOfFileProgress = %v, %v", pk, err)
	}

	if len(seen) == 0 || total != 48000*60*4 {
		t.Fatalf("progress reports %v for a data chunk of %d bytes", seen, total)
	}

	for i, v := range seen {
		if v > total || (i > 0 && v <= seen[i-1]) {
			t.Errorf("report %d = %d: not growing or past %d", i, v, total)
		}
	}
}

// The peaks of a WAV over 1 GiB (the size a request refuses, SDD-011 6.5) are computed by streaming the file,
// without holding it in memory. The file is a header and a hole (sparse where the system allows it) and is
// deleted when the test ends.
func TestPeaksOfFileOverOneGiB(t *testing.T) {
	const dataSize = 1<<30 + 4<<20

	head := wavBytes(nil, false)
	at := bytes.Index(head, []byte("data")) + 4
	binary.LittleEndian.PutUint32(head[at:], dataSize)

	p := filepath.Join(t.TempDir(), "long.wav")

	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Remove(p) })

	_, werr := f.Write(head)
	if werr == nil {
		werr = f.Truncate(int64(len(head)) + dataSize)
	}

	if cerr := f.Close(); werr != nil || cerr != nil {
		t.Fatalf("writing the file: %v, %v", werr, cerr)
	}

	var last int64

	pk, err := PeaksOfFileProgress(t.Context(), p, func(done, _ int64) { last = done })
	if err != nil || pk.SampleCount != dataSize/4 || last < dataSize-progressStepBytes {
		t.Fatalf("peaks of a 1 GiB file = %v, %v; last report %d", pk, err, last)
	}
}

// wavHeader is a RIFF WAV with the given format and a data chunk that declares declared bytes and holds body.
func wavHeader(channels uint16, rate, declared uint32, body []byte) []byte {
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	le(uint32(0))
	b.WriteString("WAVEfmt ")
	le(uint32(16))
	le(uint16(1))
	le(channels)
	le(rate)
	le(rate * uint32(channels) * 2)
	le(channels * 2)
	le(uint16(16))
	b.WriteString("data")
	le(declared)
	b.Write(body)

	return b.Bytes()
}

func peaksOfBytes(t *testing.T, data []byte) (*Peaks, error) {
	t.Helper()

	p := filepath.Join(t.TempDir(), "bad.wav")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return PeaksOfFileProgress(t.Context(), p, func(int64, int64) {})
}

// A header nobody vouches for never crashes or hangs the read: a size past the end is cut to the file, and a format
// that cannot be peaks is an error before anything is allocated from it.
func TestPeaksOfAMalformedWAVHeader(t *testing.T) {
	body := make([]byte, 4096)

	t.Run("declared past the end", func(t *testing.T) {
		pk, err := peaksOfBytes(t, wavHeader(2, 48000, 0xFFFFFFF0, body))
		if err != nil || pk.SampleCount != int64(len(body)/4) {
			t.Fatalf("peaks = %+v, %v", pk, err)
		}
	})

	t.Run("declared zero", func(t *testing.T) {
		pk, err := peaksOfBytes(t, wavHeader(2, 48000, 0, body))
		if err != nil || pk.SampleCount != 0 {
			t.Fatalf("peaks = %+v, %v", pk, err)
		}
	})

	for name, f := range map[string]struct {
		channels uint16
		rate     uint32
	}{"no channels": {0, 48000}, "65535 channels": {65535, 48000}, "no sample rate": {2, 0}} {
		t.Run(name, func(t *testing.T) {
			if _, err := peaksOfBytes(t, wavHeader(f.channels, f.rate, uint32(len(body)), body)); err == nil {
				t.Fatal("a format that cannot be peaks was accepted")
			}
		})
	}
}

// The data size of a ds64 chunk is a number of the file too: past the end it is cut to the file.
func TestReadWAVCutsAnRF64SizePastTheEnd(t *testing.T) {
	data := wavBytes(ramp(1000), true)
	at := bytes.Index(data, []byte("ds64")) + 8 + 8
	binary.LittleEndian.PutUint64(data[at:], 1<<62)

	w, err := ReadWAV(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if want := int64(1000 * 4); w.DataSize != want {
		t.Errorf("DataSize = %d, want %d", w.DataSize, want)
	}
}

// A file of empty chunks ends the walk of its header at once instead of a read for every eight bytes.
func TestReadWAVStopsOnAFileOfEmptyChunks(t *testing.T) {
	var b bytes.Buffer

	b.WriteString("RIFF\x00\x00\x00\x00WAVE")

	for range maxWAVChunks + 10 {
		b.WriteString("JUNK\x00\x00\x00\x00")
	}

	if _, err := ReadWAV(bytes.NewReader(b.Bytes())); !errors.Is(err, errWAV) {
		t.Fatalf("err = %v, want errWAV", err)
	}
}
