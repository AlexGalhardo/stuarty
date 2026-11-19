package audio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The processor chains of the mix (SDD-004 section 7.1): the chain of a source runs on its decoded frames before the
// envelope; the chain of a track runs on the sum of the sources of that track.

// addChained is add for a source with a chain: its frames of [lo, hi) go through the chain, then the envelope.
func (t *track) addChained(ctx context.Context, open Opener, out []float32, b0, lo, hi int64) error {
	if t.run == nil {
		chain, err := t.buildChain(ctx, open)
		if err != nil {
			return err
		}

		t.ctx, t.open = ctx, open
		t.run = &runner{chain: chain, pull: t.pullStream}
	}

	m := hi - lo
	l, r := make([]float64, m), make([]float64, m)

	if err := t.run.next(l, r); err != nil {
		return err
	}

	start := t.src.StartSample

	for i := range m {
		g := t.gain(lo + i - start)
		at := (lo + i - b0) * Channels
		out[at] += float32(mul(l[i], g))
		out[at+1] += float32(mul(r[i], g))
	}

	if hi == start+t.src.Samples {
		t.close()
	}

	return nil
}

// pullStream fills the next frames of the decoded source, as floats; after the last frame of the source it gives
// zeros, which is what the chain reads ahead of the end.
func (t *track) pullStream(l, r []float64) error {
	var buf [BytesPerSample]byte

	for i := range l {
		l[i], r[i] = 0, 0

		if t.pulled >= t.src.Samples {
			continue
		}

		t.pulled++

		if t.r == nil && !t.done {
			rc, err := t.open.Open(t.ctx, t.src)
			if err != nil {
				return err
			}

			t.rc, t.r = rc, bufio.NewReaderSize(rc, 1<<16)
		}

		if !t.done {
			if _, err := io.ReadFull(t.r, buf[:]); err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					return err
				}

				t.done = true
				t.close()

				continue
			}

			l[i], r[i] = float64(s16(buf[0:2]))/32768, float64(s16(buf[2:4]))/32768
		}

		if t.pulled == t.src.Samples {
			t.close()
		}
	}

	return nil
}

// buildChain builds the chain of the source. A noise reduction with a marked range gets the profile of that range of
// the asset, decoded once at speed 1 before the source is processed.
func (t *track) buildChain(ctx context.Context, open Opener) (*Chain, error) {
	var prof noiseProfile

	if a, b := MarkedProfile(t.src.Chain); b > a {
		var err error
		if prof, err = markedProfile(ctx, open, t.src, a, b); err != nil {
			return nil, err
		}
	}

	return NewChain(t.src.Chain, prof)
}

// markedProfile is the mean power per bin of the range [startMS, endMS) of the asset of src. A range the asset does
// not reach is cut to what it has; one with less than a frame gives nil, the estimated profile.
func markedProfile(
	ctx context.Context,
	open Opener,
	src plan.AudioSource,
	startMS, endMS float64,
) (noiseProfile, error) {
	const perMS = SampleRate / 1000

	probe := plan.AudioSource{
		ItemID: src.ItemID, AssetID: src.AssetID, SourceStart: int64(startMS) * perMS,
		Samples: int64(endMS-startMS) * perMS, Speed: timeline.Rational{Num: 1, Den: 1}, Denoise: src.Denoise,
	}

	rc, err := open.Open(ctx, probe)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	// Streamed one sample at a time: the range may be an hour of the asset, which is not held in memory at once.
	in := bufio.NewReaderSize(io.LimitReader(rc, probe.Samples*BytesPerSample), 1<<16)
	m := newProfileMeter()

	var buf [BytesPerSample]byte

	for n := 0; ; n++ {
		if n%SampleRate == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		if _, err := io.ReadFull(in, buf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}

			return nil, err
		}

		m.write(float64(s16(buf[0:2]))/32768, float64(s16(buf[2:4]))/32768)
	}

	return m.result(), nil
}

// bus is the sources of one track and the chain that runs on their sum.
type bus struct {
	id      string
	members []*track
	run     *runner
	pos     int64 // the next output sample of the sum to read
	chain   []plan.ProcessorSpec
}

func (b *bus) name() string { return "track " + b.id }

func (b *bus) close() {
	for _, m := range b.members {
		m.close()
	}
}

// pull is the input of the bus chain: the sum of the sources over the next len(l) samples.
func (b *bus) pull(ctx context.Context, open Opener) func(l, r []float64) error {
	return func(l, r []float64) error {
		n := int64(len(l))
		sum := make([]float32, n*Channels)

		for _, m := range b.members {
			if err := m.add(ctx, open, sum, b.pos, n); err != nil {
				return fmt.Errorf("audio source %s: %w", m.name(), err)
			}
		}

		for i := range l {
			l[i], r[i] = float64(sum[i*Channels]), float64(sum[i*Channels+1])
		}

		b.pos += n

		return nil
	}
}

func (b *bus) add(ctx context.Context, open Opener, out []float32, _, n int64) error {
	if b.run == nil {
		chain, err := NewChain(b.chain, nil)
		if err != nil {
			return err
		}

		b.run = &runner{chain: chain, pull: b.pull(ctx, open)}
	}

	l, r := make([]float64, n), make([]float64, n)
	if err := b.run.next(l, r); err != nil {
		return err
	}

	for i := range n {
		out[i*Channels] += float32(l[i])
		out[i*Channels+1] += float32(r[i])
	}

	return nil
}

// buildUnits groups the sources: those of a track that has a chain go into one bus, at the place of the first of them;
// every other source is a unit of its own.
func buildUnits(srcs []plan.AudioSource) []unit {
	var units []unit

	buses := map[string]*bus{}

	for i := range srcs {
		t := &track{src: srcs[i]}

		if srcs[i].Track == "" || len(srcs[i].BusChain) == 0 {
			units = append(units, t)

			continue
		}

		b, ok := buses[srcs[i].Track]
		if !ok {
			b = &bus{id: srcs[i].Track, chain: srcs[i].BusChain}
			buses[srcs[i].Track] = b
			units = append(units, b)
		}

		b.members = append(b.members, t)
	}

	return units
}
