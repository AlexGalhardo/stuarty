package plan

import (
	"cmp"
	"maps"
	"math"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// SampleRate is the rate of the mix (SDD-004 §7).
const SampleRate = 48000

// subdivisions is how many straight segments approximate a curved easing in a
// gain envelope.
const subdivisions = 8

// GainPoint is a point of a piecewise-linear gain envelope. Sample counts
// output samples from the start of the source's range.
type GainPoint struct {
	Sample int64   `json:"sample"`
	Gain   float64 `json:"gain"`
}

// ProcessorSpec is one enabled processor of a chain as the plan carries it (SDD-004 §7.1): the kind and every
// parameter with its value, the defaults filled in.
type ProcessorSpec struct {
	Kind   string             `json:"kind"`
	Params map[string]float64 `json:"params"`
}

// processorSpecs is the plan form of a chain: the enabled processors, in order, or nil when there is none.
func processorSpecs(chain []timeline.Processor) []ProcessorSpec {
	var out []ProcessorSpec

	for _, p := range chain {
		if p.Disabled {
			continue
		}

		out = append(out, ProcessorSpec{Kind: p.Kind, Params: p.FullParams()})
	}

	return out
}

// AudioSource is one source of the mix (SDD-004 §7): a clip with audio or an
// audio item, clipped to the requested frames.
type AudioSource struct {
	ItemID      string            `json:"item"`
	AssetID     string            `json:"asset"`
	StartSample int64             `json:"start_sample"` // position on the output, at 48 kHz
	SourceStart int64             `json:"source_start"` // first sample of the asset's normalized audio
	Samples     int64             `json:"samples"`      // output samples
	Speed       timeline.Rational `json:"speed"`        // the mean speed when Curve is set
	Curve       *SpeedRamp        `json:"speed_curve,omitempty"`
	Reverse     bool              `json:"reverse,omitempty"` // read backwards from the end of source frame SourceStart
	Gain        []GainPoint       `json:"gain"`              // volume or its keyframes, times the fades
	Denoise     float64           `json:"denoise,omitempty"` // noise reduction strength 0 to 1 (SDD-004 §7); absent means off
	Voice       string            `json:"voice,omitempty"`   // voice preset (SDD-004 §7); absent means none
	// Timeline is the timeline whose assets table names AssetID, for a source that comes from a composition.
	Timeline string `json:"timeline,omitempty"`
	// Chain is the enabled processors of the item and, when its track has a chain too, Track names the track and
	// BusChain is that chain (the same on every source of the track): SDD-004 §7.1. Absent means none.
	Chain    []ProcessorSpec `json:"chain,omitempty"`
	Track    string          `json:"track,omitempty"`
	BusChain []ProcessorSpec `json:"bus_chain,omitempty"`
}

// SpeedRamp is the speed curve of an item as the plan carries it (SDD-004 §3.11): the points,
// the duration of the whole item and the item frame at which the window starts, which is what
// timeline.CurveOffset needs to give the source position of any later frame.
type SpeedRamp struct {
	Points     []timeline.SpeedPoint `json:"points"`
	ItemFrames int                   `json:"item_frames"`
	FirstFrame int                   `json:"first_frame"`
}

// curveOf is the ramp of an item whose window starts at timeline frame cs, or nil when its speed is constant.
func curveOf(it *timeline.Item, cs int) *SpeedRamp {
	if len(it.SpeedCurve) == 0 {
		return nil
	}
	return &SpeedRamp{Points: it.SpeedCurve, ItemFrames: it.Duration, FirstFrame: cs - it.Start}
}

// audible reports whether the item is a source: a clip whose asset has audio
// or an audio item, not muted itself or by its track, and not the placeholder of an
// unfilled optional slot (SDD-004 §7).
func audible(doc *timeline.Document, it *timeline.Item) bool {
	if it.AudioMuted || it.Disabled || !doc.TrackSounds(it.Track) || it.Asset == timeline.PlaceholderAsset {
		return false
	}

	if it.Type == timeline.ItemAudio {
		return true
	}

	return it.Type == timeline.ItemClip && doc.Assets[it.Asset].HasAudio
}

// frameSample is the output sample of timeline frame t: t × 48000 × den / num,
// floored.
func frameSample(fps timeline.Rational, t int) int64 {
	return int64(t) * SampleRate * int64(fps.Den) / int64(fps.Num)
}

// sources lists the audible items by track order (bottom first), start and id.
func sources(doc *timeline.Document) []*timeline.Item {
	var out []*timeline.Item

	for _, id := range slices.Sorted(maps.Keys(doc.Items)) {
		it := doc.Items[id]
		if audible(doc, &it) {
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

// gainAt is the gain of an item at frame r inside it, which may be
// fractional: volume or its keyframes times the linear fades.
func gainAt(it *timeline.Item, r float64) float64 {
	g := property(it, "volume", it.Volume, r)

	if it.FadeIn > 0 && r < float64(it.FadeIn) {
		g *= r / float64(it.FadeIn)
	}

	if end := float64(it.Duration); it.FadeOut > 0 && r > end-float64(it.FadeOut) {
		g *= (end - r) / float64(it.FadeOut)
	}

	return max(g, 0)
}

// Audio returns every audio source that sounds in frames [from, to), clipped
// to that range. It is pure.
func Audio(doc *timeline.Document, from, to int) []AudioSource {
	var out []AudioSource

	for _, it := range sources(doc) {
		cs, ce := max(from, it.Start), min(to, it.Start+it.Duration)
		if cs >= ce {
			continue
		}

		out = append(out, audioSource(doc, it, cs, ce))
	}

	return out
}

func audioSource(doc *timeline.Document, it *timeline.Item, cs, ce int) AudioSource {
	fps := doc.Settings.FPS
	start := frameSample(fps, cs)

	pos := int64(it.SourceIn)
	if it.Speed.Den > 0 {
		pos += int64(it.SourceOffset(cs - it.Start))
	}

	gain := envelope(it, fps, cs-it.Start, ce-it.Start)

	// The volume of the track is a factor on the whole envelope.
	if k := doc.TrackGain(it.Track); k != 1 {
		for i := range gain {
			gain[i].Gain = round3(gain[i].Gain * k)
		}
	}

	src := AudioSource{
		ItemID:      it.ID,
		AssetID:     it.Asset,
		StartSample: start,
		SourceStart: frameSample(fps, int(pos)),
		Samples:     frameSample(fps, ce) - start,
		Speed:       it.Speed,
		Curve:       curveOf(it, cs),
		Reverse:     it.Reverse,
		Gain:        gain,
		Denoise:     round3(it.Denoise),
		Voice:       it.Voice,
		Chain:       processorSpecs(it.Processors),
	}

	if bus := processorSpecs(doc.Tracks[it.Track].Processors); len(bus) > 0 {
		src.Track, src.BusChain = it.Track, bus
	}

	return src
}

// envelope is the gain of the item over frames [rs, re) inside it as points in
// output samples from rs: a point at every keyframe and fade edge, and a few
// straight segments for each curved easing between them.
func envelope(it *timeline.Item, fps timeline.Rational, rs, re int) []GainPoint {
	marks := map[int]bool{rs: true, re: true}

	for _, kf := range it.Keyframes["volume"] {
		marks[kf.T] = true
	}

	if it.FadeIn > 0 {
		marks[it.FadeIn] = true
	}

	if it.FadeOut > 0 {
		marks[it.Duration-it.FadeOut] = true
	}

	frames := []int{}

	for f := range marks {
		if f >= rs && f <= re {
			frames = append(frames, f)
		}
	}

	slices.Sort(frames)

	base := frameSample(fps, rs+it.Start)

	var pts []GainPoint

	add := func(sample int64, g float64) {
		p := GainPoint{Sample: sample - base, Gain: round3(g)}
		if n := len(pts); n > 0 && pts[n-1].Sample == p.Sample {
			pts[n-1] = p
			return
		}

		pts = append(pts, p)
	}

	for i, f := range frames {
		add(frameSample(fps, f+it.Start), gainAt(it, float64(f)))

		if i+1 < len(frames) {
			curve(it, fps, f, frames[i+1], add)
		}
	}

	return pts
}

// curve adds the points between frames a and b that a curved easing needs.
func curve(it *timeline.Item, fps timeline.Rational, a, b int, add func(int64, float64)) {
	easing := segmentEasing(it, a)
	switch easing {
	case "linear":
		return
	case "hold":
		// The value stays until the next keyframe: one sample before it.
		add(frameSample(fps, b+it.Start)-1, gainAt(it, float64(a)))

		return
	}

	for k := 1; k < subdivisions; k++ {
		r := float64(a) + float64(b-a)*float64(k)/subdivisions
		add(int64(math.Floor(r*SampleRate*float64(fps.Den)/float64(fps.Num)))+frameSample(fps, it.Start), gainAt(it, r))
	}
}

// segmentEasing is the easing of the volume keyframe that governs frame a.
func segmentEasing(it *timeline.Item, a int) string {
	easing := "linear"

	for _, kf := range it.Keyframes["volume"] {
		if kf.T <= a {
			easing = kf.Easing
		}
	}

	return easing
}
