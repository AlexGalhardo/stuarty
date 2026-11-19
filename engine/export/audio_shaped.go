package export

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

// This file is the audio of an item with a speed curve or with reverse (SDD-004 sections 3.11, 3.12 and 7).
// FFmpeg decodes a plain range of the source; the shape is made here, in integer arithmetic, so it needs no
// filter beyond the ones of the argument grammar and follows the frames of the video exactly.

// bytesPerFrame is the bytes of one stereo sample of the 16-bit stream.
const bytesPerFrame = audio.BytesPerSample

// openShaped is Open for a source with Reverse or a Curve.
func (o *audioOpener) openShaped(ctx context.Context, src plan.AudioSource) (io.ReadCloser, error) {
	fps := o.s.job.Doc.Settings.FPS
	if src.Samples <= 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	var (
		out []byte
		err error
	)

	if src.Curve != nil {
		out, err = o.curved(ctx, src, fps)
	} else {
		out, err = o.reversed(ctx, src, fps)
	}

	if err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(out)), nil
}

// readAll decodes a plain range of the source at speed 1 with the noise reduction of the item.
func (o *audioOpener) readAll(
	ctx context.Context,
	src plan.AudioSource,
	start, n int64,
	speed timeline.Rational,
) ([]byte, error) {
	plain := plan.AudioSource{
		ItemID: src.ItemID, AssetID: src.AssetID, SourceStart: start, Samples: n, Speed: speed, Denoise: src.Denoise,
	}

	rc, err := o.Open(ctx, plain)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	return io.ReadAll(io.LimitReader(rc, n*bytesPerFrame))
}

// reversed is the audio of a reversed item of constant speed: the range the item reads, decoded forwards
// with the tempo change, with its samples in the opposite order. Output sample j reads the source at the
// end of source frame SourceStart minus j times the speed (SDD-004 section 3.12).
func (o *audioOpener) reversed(ctx context.Context, src plan.AudioSource, fps timeline.Rational) ([]byte, error) {
	num, den := int64(src.Speed.Num), int64(src.Speed.Den)
	if num <= 0 || den <= 0 {
		num, den = 1, 1
	}

	need := (src.Samples*num + den - 1) / den
	start := src.SourceStart + samplesAt(fps, 1) - need
	lead := int64(0)

	if start < 0 {
		lead = (-start*den + num - 1) / num
		start = 0
	}

	fwd := make([]byte, src.Samples*bytesPerFrame)

	if lead < src.Samples {
		got, err := o.readAll(ctx, src, start, src.Samples-lead, src.Speed)
		if err != nil {
			return nil, err
		}

		copy(fwd[lead*bytesPerFrame:], got)
	}

	reverseFrames(fwd)

	return fwd, nil
}

// reverseFrames puts the stereo samples of pcm in the opposite order.
func reverseFrames(pcm []byte) {
	for i, j := 0, len(pcm)/bytesPerFrame-1; i < j; i, j = i+1, j-1 {
		var tmp [bytesPerFrame]byte

		a, b := pcm[i*bytesPerFrame:(i+1)*bytesPerFrame], pcm[j*bytesPerFrame:(j+1)*bytesPerFrame]
		copy(tmp[:], a)
		copy(a, b)
		copy(b, tmp[:])
	}
}

// curved is the audio of an item with a speed curve, with or without reverse: the source is read at the
// positions the curve gives to the boundaries of the frames of the item, and straight between them, by
// linear interpolation of the samples (the pitch follows the speed, like a tape). Positions are 48 kHz
// samples in 16.16 fixed point, so the bytes are the same on every machine.
func (o *audioOpener) curved(ctx context.Context, src plan.AudioSource, fps timeline.Rational) ([]byte, error) {
	ramp := src.Curve
	first, last := ramp.FirstFrame, ramp.ItemFrames
	base := samplesAt(fps, timeline.CurveOffset(ramp.Points, ramp.ItemFrames, first))
	frame := samplesAt(fps, 1)

	pos := make([]int64, src.Samples)

	var (
		k          = -1
		bk, bn     int64
		offA, offB int64
	)

	// A curve inside a composition at a speed n/d: the frames of the item pass n/d times faster than the
	// output, so the boundary of frame r falls at output sample floor(boundary x d / n) (SDD-004 section 7).
	scale := func(x int64) int64 { return x }

	if ts := ramp.TimeScale; ts != nil && ts.Num > 0 && ts.Den > 0 {
		scale = func(x int64) int64 { return x * int64(ts.Den) / int64(ts.Num) }
	}

	boundary := func(r int) int64 { return scale(samplesAt(fps, r) - samplesAt(fps, first)) }

	for j := range src.Samples {
		// The frame k of the window whose output samples hold j; the offsets of its two boundaries are
		// computed once per frame (they use arbitrary precision).
		for k < 0 || boundary(first+k+1) <= j && first+k+1 < last {
			k++
			bk, bn = boundary(first+k), boundary(first+k+1)
			offA = samplesAt(fps, timeline.CurveOffset(ramp.Points, ramp.ItemFrames, first+k)) - base
			offB = samplesAt(fps, timeline.CurveOffset(ramp.Points, ramp.ItemFrames, first+k+1)) - base
		}

		rel := offA<<16 + ((offB-offA)<<16)*(j-bk)/max(bn-bk, 1)
		if src.Reverse {
			pos[j] = (src.SourceStart+frame-1)<<16 - rel
		} else {
			pos[j] = src.SourceStart<<16 + rel
		}
	}

	lo, hi := min(pos[0], pos[len(pos)-1])>>16-1, max(pos[0], pos[len(pos)-1])>>16+2
	lo = max(lo, 0)

	var pcm []byte

	if hi > lo {
		var err error
		if pcm, err = o.readAll(ctx, src, lo, hi-lo, timeline.Rational{Num: 1, Den: 1}); err != nil {
			return nil, err
		}
	}

	have := int64(len(pcm) / bytesPerFrame)
	sample := func(i int64, ch int) int64 {
		if i < 0 || i >= have {
			return 0
		}

		at := int(i)*bytesPerFrame + 2*ch

		return le16(pcm[at:])
	}

	out := make([]byte, src.Samples*bytesPerFrame)

	for j, p := range pos {
		i, w := p>>16-lo, p&0xFFFF

		for ch := range audio.Channels {
			v := (sample(i, ch)*(65536-w) + sample(i+1, ch)*w + 32768) >> 16
			putLE16(out[j*bytesPerFrame+2*ch:], v)
		}
	}

	return out, nil
}

// le16 is the signed 16-bit little-endian sample at the start of b.
//
//nolint:gosec // G115: the bytes are one int16
func le16(b []byte) int64 {
	return int64(int16(binary.LittleEndian.Uint16(b)))
}

// putLE16 writes v, which is between two 16-bit samples, at the start of b.
//
//nolint:gosec // G115: v fits 16 bits
func putLE16(b []byte, v int64) {
	binary.LittleEndian.PutUint16(b, uint16(v))
}
