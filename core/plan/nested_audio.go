package plan

import (
	"cmp"
	"maps"
	"slices"
	"sort"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The sound of a nested timeline (SDD-004 §7, "Compositions"): a composition item sounds as the mix of its
// timeline, so the sources of that timeline are the item's, moved to the item's place and scaled by the gain of
// the item. The mixer sums sources linearly and applies one loudness gain to the project, so summing the nested
// sources, each scaled by the bus gain, is the same samples as mixing the nested timeline to a bus and scaling
// the bus.

// AudioWith is Audio for a document that holds composition items: the sources of the timelines they use come
// after those of the document, with ids of the form "<composition item>/<id in the nested timeline>". With no
// opts.Nested it is Audio.
func AudioWith(doc *timeline.Document, from, to int, opts Options) []AudioSource {
	return append(Audio(doc, from, to), compositionAudio(doc, from, to, opts, []string{doc.ID})...)
}

// nestedWindowAudio lists, for the preview, the sources of the compositions of a window in the form of
// WindowAudio: the same sources as AudioWith, with their gain on each frame of the window.
func nestedWindowAudio(doc *timeline.Document, from, to int, opts Options) []WindowAudio {
	fps := doc.Settings.FPS
	out := []WindowAudio{}

	if fps.Num == 0 {
		return out
	}

	toFrame := func(sample int64) int {
		return int((sample*int64(fps.Num) + SampleRate*int64(fps.Den) - 1) / (SampleRate * int64(fps.Den)))
	}

	srcs := compositionAudio(doc, from, to, opts, []string{doc.ID})
	for i := range srcs {
		s := &srcs[i]
		first := max(toFrame(s.StartSample), from)
		last := min(toFrame(s.StartSample+s.Samples)-1, to-1)

		if last < first {
			continue
		}

		speed := float64(s.Speed.Num) / float64(max(s.Speed.Den, 1))
		wa := WindowAudio{
			Item: s.ItemID, Asset: s.AssetID, First: first, Last: last, Speed: round3(speed), Curve: s.Curve,
			Reverse: s.Reverse, Denoise: s.Denoise, Voice: s.Voice, Timeline: s.Timeline,
		}
		skipped := float64(frameSample(fps, first) - s.StartSample)
		wa.SourceSeconds = round3((float64(s.SourceStart) + skipped*speed) / SampleRate)

		for t := first; t <= last; t++ {
			wa.Gain = append(wa.Gain, round3(gainAtSample(s.Gain, frameSample(fps, t)-s.StartSample)))
		}

		out = append(out, wa)
	}

	return out
}

// compositionSources lists the composition items that sound, by track order (bottom first), start and id.
func compositionSources(doc *timeline.Document) []*timeline.Item {
	var out []*timeline.Item

	for _, id := range slices.Sorted(maps.Keys(doc.Items)) {
		it := doc.Items[id]
		if it.Type == timeline.ItemComposition && !it.Disabled && !it.AudioMuted && doc.TrackSounds(it.Track) {
			out = append(out, &it)
		}
	}

	slices.SortStableFunc(out, func(a, b *timeline.Item) int {
		return cmp.Or(
			-cmp.Compare(doc.Tracks[a.Track].Order, doc.Tracks[b.Track].Order),
			cmp.Compare(a.Start, b.Start),
			cmp.Compare(a.ID, b.ID),
		)
	})

	return out
}

// ceilFrames converts a position of n frames at rate from to frames at rate to, rounded up.
func ceilFrames(n int64, from, to timeline.Rational) int64 {
	num, den := n*int64(to.Num)*int64(from.Den), int64(to.Den)*int64(from.Num)
	if den <= 0 {
		return 0
	}

	return (num + den - 1) / den
}

func compositionAudio(
	doc *timeline.Document, from, to int, opts Options, trail []string,
) []AudioSource {
	if opts.Nested == nil || len(trail) > timeline.MaxCompositionDepth {
		return nil
	}

	var out []AudioSource

	pfps := doc.Settings.FPS

	for _, it := range compositionSources(doc) {
		cs, ce := max(from, it.Start), min(to, it.Start+it.Duration)
		if cs >= ce || slices.Contains(trail, it.Timeline) {
			continue
		}

		nested, ok := opts.Nested(it.Timeline)
		if !ok || nested == nil || pfps.Num == 0 {
			continue
		}

		nfps := nested.Settings.FPS
		p0, p1 := int64(it.SourceIn+cs-it.Start), int64(it.SourceIn+ce-it.Start)
		f0, f1 := ceilFrames(p0, pfps, nfps), min(ceilFrames(p1, pfps, nfps), int64(endFrame(nested)))

		if f1 <= f0 {
			continue
		}

		next := append(slices.Clone(trail), it.Timeline)
		inner := append(Audio(nested, int(f0), int(f1)), compositionAudio(nested, int(f0), int(f1), opts, next)...)
		bus := compositionGain(doc, it, cs, ce)

		// Nested sample ns is the parent's sample start + ns - the nested sample of source_in.
		shift := frameSample(pfps, it.Start) - frameSample(pfps, it.SourceIn)
		lo, hi := frameSample(pfps, cs), frameSample(pfps, ce)

		for i := range inner {
			s := &inner[i]
			s.ItemID = it.ID + "/" + s.ItemID

			if s.Timeline == "" {
				s.Timeline = nested.ID
			}
			s.StartSample += shift

			if !s.trim(lo, hi) {
				continue
			}

			s.Gain = mulEnvelope(s.Gain, s.StartSample, bus.points, bus.base)
			out = append(out, *s)
		}
	}

	return out
}

// busGain is the gain envelope of a composition item over the frames of a window, in output samples from base.
type busGain struct {
	points []GainPoint
	base   int64
}

// compositionGain is the gain of a composition item over frames [cs, ce): its volume and keyframes, its fades
// and the volume of its track, as for any item (SDD-004 §7).
func compositionGain(doc *timeline.Document, it *timeline.Item, cs, ce int) busGain {
	fps := doc.Settings.FPS
	pts := envelope(it, fps, cs-it.Start, ce-it.Start)

	if k := doc.TrackGain(it.Track); k != 1 {
		for i := range pts {
			pts[i].Gain = round3(pts[i].Gain * k)
		}
	}

	return busGain{points: pts, base: frameSample(fps, cs)}
}

// trim cuts the source to the output samples [lo, hi), false when nothing is left. The start can only be cut for
// a source that plays forwards at a constant speed; the planner starts nested sources at or after the start of
// the item, so a cut of the start is the rounding of a sample or two.
func (s *AudioSource) trim(lo, hi int64) bool {
	if d := lo - s.StartSample; d > 0 {
		if s.Curve != nil || s.Reverse || d >= s.Samples {
			return false
		}

		s.StartSample += d
		s.SourceStart += d * int64(s.Speed.Num) / int64(max(s.Speed.Den, 1))
		s.Samples -= d
		s.Gain = shiftEnvelope(s.Gain, d)
	}

	if end := s.StartSample + s.Samples; end > hi {
		s.Samples = hi - s.StartSample
	}

	return s.Samples > 0
}

// shiftEnvelope drops the first d samples of an envelope: the points move earlier by d and the value at the new
// start is kept as its first point.
func shiftEnvelope(pts []GainPoint, d int64) []GainPoint {
	start := gainAtSample(pts, d)
	out := []GainPoint{{Sample: 0, Gain: start}}

	for _, p := range pts {
		if p.Sample > d {
			out = append(out, GainPoint{Sample: p.Sample - d, Gain: p.Gain})
		}
	}

	return out
}

// gainAtSample is the value of an envelope at a sample: linear between its points, flat before the first and
// after the last, and the later value at a sample where two points meet (a step).
func gainAtSample(pts []GainPoint, x int64) float64 {
	if len(pts) == 0 {
		return 1
	}

	i := sort.Search(len(pts), func(i int) bool { return pts[i].Sample > x })

	switch {
	case i == 0:
		return pts[0].Gain
	case i == len(pts):
		return pts[len(pts)-1].Gain
	}

	a, b := pts[i-1], pts[i]
	if b.Sample == a.Sample {
		return b.Gain
	}

	return a.Gain + (b.Gain-a.Gain)*float64(x-a.Sample)/float64(b.Sample-a.Sample)
}

// mulEnvelope is the product of the envelope of a source (in samples from its start, which is the output sample
// start) and the envelope of the bus (in samples from base): a point at every point of either, and the span
// between two of them cut in four where both change, so the curve of the product stays close to the product of
// the curves.
func mulEnvelope(src []GainPoint, start int64, bus []GainPoint, base int64) []GainPoint {
	if len(bus) == 0 {
		return src
	}

	marks := map[int64]bool{}
	last := int64(0)

	for _, p := range src {
		marks[p.Sample] = true
		last = max(last, p.Sample)
	}

	for _, p := range bus {
		if x := p.Sample + base - start; x >= 0 {
			marks[x] = true
		}
	}

	if _, ok := marks[0]; !ok {
		marks[0] = true
	}

	xs := slices.Sorted(maps.Keys(marks))

	var out []GainPoint

	add := func(x int64) {
		g := round3(gainAtSample(src, x) * gainAtSample(bus, x+start-base))
		if n := len(out); n > 0 && out[n-1].Sample == x {
			out[n-1].Gain = g

			return
		}

		out = append(out, GainPoint{Sample: x, Gain: g})
	}

	for i, x := range xs {
		add(x)

		if i+1 == len(xs) {
			break
		}

		a, b := x, xs[i+1]
		if b-a > 4 && changes(src, a, b) && changes(bus, a+start-base, b+start-base) {
			for k := int64(1); k < 4; k++ {
				add(a + (b-a)*k/4)
			}
		}
	}

	return out
}

// changes says an envelope is not constant between two samples.
func changes(pts []GainPoint, a, b int64) bool {
	return gainAtSample(pts, a) != gainAtSample(pts, b-1)
}
