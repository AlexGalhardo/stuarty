// Package audio mixes the audio sources of a frame range into one 48 kHz stereo track and measures
// the single loudness gain applied to it (SDD-004 section 7).
//
// The mix is streamed in blocks, so its memory does not grow with the length of the project.
package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// Layout of the mix and of the decoded streams.
const (
	SampleRate = plan.SampleRate
	Channels   = 2
	// BytesPerSample is the size of one stereo frame of the decoded streams: two signed 16-bit samples.
	BytesPerSample = 4
	// BlockSamples is how many output samples one block of the mix holds (one second).
	BlockSamples = SampleRate
)

// Opener opens the decoded stream of one source: 48 kHz stereo signed 16-bit little-endian samples
// (what FFmpeg writes with -f s16le), starting at the source's first output sample, with its speed
// already applied. The stream may be longer or shorter than src.Samples: the mixer truncates it or
// pads it with silence.
type Opener interface {
	Open(ctx context.Context, src plan.AudioSource) (io.ReadCloser, error)
}

// OpenerFunc is an Opener made of a function.
type OpenerFunc func(ctx context.Context, src plan.AudioSource) (io.ReadCloser, error)

// Open calls f.
func (f OpenerFunc) Open(ctx context.Context, src plan.AudioSource) (io.ReadCloser, error) {
	return f(ctx, src)
}

// mul is a product that is rounded before anything is added to it, so the mix is the same on every
// architecture (the explicit conversion forbids a fused multiply-add).
func mul(a, b float64) float64 { return float64(a * b) }

// track is one source being mixed.
type track struct {
	src  plan.AudioSource
	rc   io.ReadCloser
	r    *bufio.Reader
	done bool // the stream ended before src.Samples
	seg  int  // index of the envelope point at or before the next sample
}

func (t *track) close() {
	if t.rc != nil {
		_ = t.rc.Close()
		t.rc, t.r = nil, nil
	}
}

// gain is the envelope at sample p of the source, advancing seg: linear between points, flat before
// the first and after the last, and a step where two points share a sample.
func (t *track) gain(p int64) float64 {
	pts := t.src.Gain
	if len(pts) == 0 {
		return 1
	}
	for t.seg+1 < len(pts) && pts[t.seg+1].Sample <= p {
		t.seg++
	}
	a := pts[t.seg]
	if p <= a.Sample || t.seg+1 == len(pts) {
		return a.Gain
	}
	b := pts[t.seg+1]
	return a.Gain + mul(b.Gain-a.Gain, float64(p-a.Sample)/float64(b.Sample-a.Sample))
}

// Mix sums the sources into total output samples and hands the result to sink in blocks of at most
// BlockSamples stereo frames, interleaved as float32 (so a block has 2 values per frame). The slice
// is reused after sink returns. A source is opened when its first sample is needed and closed after its
// last; sources are summed in the order given, so the same input gives the same bits.
func Mix(
	ctx context.Context,
	srcs []plan.AudioSource,
	total int64,
	open Opener,
	sink func(block []float32) error,
) error {
	tracks := make([]*track, len(srcs))
	for i := range srcs {
		tracks[i] = &track{src: srcs[i]}
	}
	defer func() {
		for _, t := range tracks {
			t.close()
		}
	}()

	block := make([]float32, BlockSamples*Channels)
	for b0 := int64(0); b0 < total; b0 += BlockSamples {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := min(int64(BlockSamples), total-b0)
		out := block[:n*Channels]
		clear(out)

		for _, t := range tracks {
			if err := t.mixInto(ctx, open, out, b0, n); err != nil {
				return fmt.Errorf("audio source %s: %w", t.src.ItemID, err)
			}
		}
		if err := sink(out); err != nil {
			return err
		}
	}
	return nil
}

// mixInto adds the part of the source that falls in [b0, b0+n) to out.
func (t *track) mixInto(ctx context.Context, open Opener, out []float32, b0, n int64) error {
	start, end := t.src.StartSample, t.src.StartSample+t.src.Samples
	lo, hi := max(start, b0), min(end, b0+n)
	if lo >= hi {
		if b0+n >= end {
			t.close()
		}
		return nil
	}
	if t.r == nil && !t.done {
		rc, err := open.Open(ctx, t.src)
		if err != nil {
			return err
		}
		t.rc, t.r = rc, bufio.NewReaderSize(rc, 1<<16)
	}

	var buf [BytesPerSample]byte
	for s := lo; s < hi; s++ {
		var l, r int16
		if !t.done {
			if _, err := io.ReadFull(t.r, buf[:]); err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					return err
				}
				t.done = true
				t.close()
			} else {
				l = s16(buf[0:2])
				r = s16(buf[2:4])
			}
		}
		if l == 0 && r == 0 {
			continue
		}
		g := t.gain(s - start)
		i := (s - b0) * Channels
		out[i] += float32(mul(float64(l)/32768, g))
		out[i+1] += float32(mul(float64(r)/32768, g))
	}
	if hi == end {
		t.close()
	}
	return nil
}

// s16 reads a little-endian signed 16-bit sample.
func s16(b []byte) int16 {
	return int16(binary.LittleEndian.Uint16(b)) //nolint:gosec // two's-complement reinterpretation
}
