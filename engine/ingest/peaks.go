package ingest

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// WAV is what ingest needs from a WAV or RF64 header.
type WAV struct {
	SampleRate int
	Channels   int
	Bits       int
	DataOffset int64
	DataSize   int64
}

var errWAV = errors.New("not a PCM WAV file")

// ReadWAV walks the RIFF or RF64 chunks to the fmt and data chunks, so the 80 bytes of header
// that a muxer wrote are not assumed (SDD-011 4.6). A data size of 0xFFFFFFFF is taken from
// the ds64 chunk, or from the end of the file.
func ReadWAV(f io.ReadSeeker) (WAV, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return WAV{}, fmt.Errorf("%w: %w", errWAV, err)
	}
	magic := string(hdr[0:4])
	if (magic != "RIFF" && magic != "RF64") || string(hdr[8:12]) != "WAVE" {
		return WAV{}, errWAV
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return WAV{}, err
	}
	pos := int64(12)
	var w WAV
	var ds64Data int64 = -1
	gotFmt := false
	for pos+8 <= end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return WAV{}, err
		}
		var ch [8]byte
		if _, err := io.ReadFull(f, ch[:]); err != nil {
			return WAV{}, errWAV
		}
		id, size := string(ch[0:4]), int64(binary.LittleEndian.Uint32(ch[4:8]))
		body := pos + 8
		switch id {
		case "ds64":
			var b [24]byte
			if _, err := io.ReadFull(f, b[:]); err != nil {
				return WAV{}, errWAV
			}
			ds64Data = rdI64(b[8:16])
		case "fmt ":
			var b [16]byte
			if _, err := io.ReadFull(f, b[:]); err != nil {
				return WAV{}, errWAV
			}
			if tag := binary.LittleEndian.Uint16(b[0:2]); tag != 1 && tag != 0xFFFE {
				return WAV{}, fmt.Errorf("%w: format tag %d", errWAV, tag)
			}
			w.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
			w.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
			w.Bits = int(binary.LittleEndian.Uint16(b[14:16]))
			gotFmt = true
		case "data":
			if !gotFmt {
				return WAV{}, errWAV
			}
			w.DataOffset = body
			switch {
			case size == 0xFFFFFFFF && ds64Data >= 0:
				w.DataSize = ds64Data
			case size == 0xFFFFFFFF:
				w.DataSize = end - body
			default:
				w.DataSize = min(size, end-body)
			}
			return w, nil
		}
		pos = body + size + size%2
	}
	return WAV{}, errWAV
}

// rdI64 reads a little-endian 64-bit value that a valid file keeps below 2^63.
func rdI64(b []byte) int64 { return int64(binary.LittleEndian.Uint64(b) & math.MaxInt64) }

// rdS16 reads a signed 16-bit sample.
func rdS16(b []byte) int16 {
	u := binary.LittleEndian.Uint16(b)
	return int16(u) //nolint:gosec // two's-complement reinterpretation
}

// putS16 writes a signed 16-bit sample.
func putS16(b []byte, v int16) {
	u := uint16(v) //nolint:gosec // two's-complement reinterpretation
	binary.LittleEndian.PutUint16(b, u)
}

// PeakPair is the minimum and maximum 16-bit sample of a group of sample frames.
type PeakPair struct{ Min, Max int16 }

// Peaks is the content of peaks-1.bin.
type Peaks struct {
	SampleRate  int
	SampleCount int64
	Levels      [5][]PeakPair
	// Min and Max are the extremes of every sample seen while counting.
	Min, Max int16
}

// ComputePeaks reads 16-bit interleaved samples from r and builds the five levels of
// SDD-011 4.6. Each peak covers both channels together; level 0 groups PeakLevels[0] sample
// frames and each higher level is the min and max of four consecutive peaks below it.
func ComputePeaks(r io.Reader, channels, sampleRate int) (*Peaks, error) {
	if channels < 1 {
		return nil, errors.New("peaks: no channels")
	}
	p := &Peaks{SampleRate: sampleRate, Min: 0, Max: 0}
	br := bufio.NewReaderSize(r, 1<<20)
	buf := make([]byte, 2*channels*4096)
	group := PeakLevels[0]
	cur := PeakPair{Min: 32767, Max: -32768}
	inGroup := 0
	first := true
	for {
		n, err := io.ReadFull(br, buf)
		frames := n / (2 * channels)
		for i := range frames {
			for c := range channels {
				off := (i*channels + c) * 2
				v := rdS16(buf[off:])
				cur.Min, cur.Max = min(cur.Min, v), max(cur.Max, v)
				if first {
					p.Min, p.Max, first = v, v, false
				} else {
					p.Min, p.Max = min(p.Min, v), max(p.Max, v)
				}
			}
			p.SampleCount++
			inGroup++
			if inGroup == group {
				p.Levels[0] = append(p.Levels[0], cur)
				cur, inGroup = PeakPair{Min: 32767, Max: -32768}, 0
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, err
		}
	}
	if inGroup > 0 {
		p.Levels[0] = append(p.Levels[0], cur)
	}
	for l := 1; l < len(p.Levels); l++ {
		below := p.Levels[l-1]
		for i := 0; i < len(below); i += 4 {
			g := PeakPair{Min: 32767, Max: -32768}
			for _, q := range below[i:min(i+4, len(below))] {
				g.Min, g.Max = min(g.Min, q.Min), max(g.Max, q.Max)
			}
			p.Levels[l] = append(p.Levels[l], g)
		}
	}
	return p, nil
}

// PeakCount is ceil(sampleCount / samplesPerPeak).
func PeakCount(sampleCount int64, samplesPerPeak int) int {
	return int((sampleCount + int64(samplesPerPeak) - 1) / int64(samplesPerPeak))
}

const peaksHeader = 24

// Marshal is the little-endian file format of SDD-011 4.6.
func (p *Peaks) Marshal() []byte {
	n := len(p.Levels)
	size := peaksHeader + 16*n
	for _, l := range p.Levels {
		size += 4 * len(l)
	}
	b := make([]byte, size)
	copy(b, "VKPK")
	binary.LittleEndian.PutUint16(b[4:], 1)
	binary.LittleEndian.PutUint16(b[6:], uint16(n&0xFFFF))
	binary.LittleEndian.PutUint32(b[8:], uint32(p.SampleRate&math.MaxUint32))
	binary.LittleEndian.PutUint64(b[16:], uint64(p.SampleCount&math.MaxInt64))
	off := peaksHeader + 16*n
	for i, l := range p.Levels {
		t := peaksHeader + 16*i
		binary.LittleEndian.PutUint32(b[t:], uint32(PeakLevels[i]&math.MaxUint32))
		binary.LittleEndian.PutUint32(b[t+4:], uint32(len(l)&math.MaxUint32))
		binary.LittleEndian.PutUint64(b[t+8:], uint64(off&math.MaxInt64))
		for _, q := range l {
			putS16(b[off:], q.Min)
			putS16(b[off+2:], q.Max)
			off += 4
		}
	}
	return b
}

// ParsePeaks reads the format back, checking the structure.
func ParsePeaks(b []byte) (*Peaks, error) {
	if len(b) < peaksHeader || string(b[:4]) != "VKPK" {
		return nil, errors.New("peaks: bad magic")
	}
	if v := binary.LittleEndian.Uint16(b[4:]); v != 1 {
		return nil, fmt.Errorf("peaks: version %d", v)
	}
	n := int(binary.LittleEndian.Uint16(b[6:]))
	if n != len(PeakLevels) || len(b) < peaksHeader+16*n {
		return nil, errors.New("peaks: bad level count")
	}
	p := &Peaks{
		SampleRate:  int(binary.LittleEndian.Uint32(b[8:])),
		SampleCount: rdI64(b[16:]),
	}
	for i := range n {
		t := peaksHeader + 16*i
		if int(binary.LittleEndian.Uint32(b[t:])) != PeakLevels[i] {
			return nil, errors.New("peaks: bad samples per peak")
		}
		count := int(binary.LittleEndian.Uint32(b[t+4:]))
		off := int(rdI64(b[t+8:]))
		if off < 0 || off+4*count > len(b) {
			return nil, errors.New("peaks: level outside the file")
		}
		for j := range count {
			o := off + 4*j
			p.Levels[i] = append(
				p.Levels[i],
				PeakPair{rdS16(b[o:]), rdS16(b[o+2:])},
			)
		}
	}
	return p, nil
}

// Check applies the checks of SDD-011 5.1: every level has ceil(sample_count / samples_per_peak)
// peaks, and the extremes of the top level equal the extremes seen while counting (min, max).
func (p *Peaks) Check(min16, max16 int16) error {
	for i, l := range p.Levels {
		if want := PeakCount(p.SampleCount, PeakLevels[i]); len(l) != want {
			return fmt.Errorf("peaks: level %d has %d peaks, want %d", i, len(l), want)
		}
	}
	top := p.Levels[len(p.Levels)-1]
	if len(top) == 0 {
		return errors.New("peaks: no samples")
	}
	lo, hi := top[0].Min, top[0].Max
	for _, q := range top {
		lo, hi = min(lo, q.Min), max(hi, q.Max)
	}
	if lo != min16 || hi != max16 {
		return fmt.Errorf("peaks: top level spans %d..%d, samples span %d..%d", lo, hi, min16, max16)
	}
	return nil
}

// PeaksOfFile reads a WAV file and builds its peaks.
func PeaksOfFile(path string) (*Peaks, error) {
	return PeaksOfFileContext(context.Background(), path)
}

// ctxReader stops a long read as soon as its context ends.
type ctxReader struct {
	ctx context.Context //nolint:containedctx // the reader lives for one call
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}

	return c.r.Read(p)
}

// PeaksOfFileContext is PeaksOfFile that stops, with the error of ctx, when ctx ends: a request that computes
// the peaks of a long file gives the work up when its client goes away (SDD-011 section 6.5).
func PeaksOfFileContext(ctx context.Context, path string) (*Peaks, error) {
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
		return nil, fmt.Errorf("peaks: %d-bit samples", w.Bits)
	}
	if _, err := f.Seek(w.DataOffset, io.SeekStart); err != nil {
		return nil, err
	}
	return ComputePeaks(ctxReader{ctx: ctx, r: io.LimitReader(f, w.DataSize)}, w.Channels, w.SampleRate)
}
