package audio

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Epic 53, SDD-004 section 7 "A composition at a speed": the sources of the nested timeline are moved by the
// time map and decoded at their own speed times the speed of the item. The decoder of the export is FFmpeg with
// atempo, which keeps the pitch (TestCompositionSpeedKeepsThePitch in engine/export); here the decoder is a
// stand-in that reads the source at the speed by whole samples, so the mix is exact and its samples can be held
// against the golden files.

// speedOpener opens a source and reads it at its constant speed: output sample i is the source sample
// SourceStart + floor(i x speed).
type speedOpener struct{ streams map[string][]byte }

func (o *speedOpener) Open(_ context.Context, s plan.AudioSource) (io.ReadCloser, error) {
	raw := o.streams[s.AssetID]
	out := make([]byte, 0, s.Samples*4)

	for i := range s.Samples {
		at := (s.SourceStart + i*int64(s.Speed.Num)/int64(max(s.Speed.Den, 1))) * 4
		if at+4 > int64(len(raw)) {
			out = append(out, 0, 0, 0, 0)

			continue
		}

		out = append(out, raw[at:at+4]...)
	}

	return io.NopCloser(bytes.NewReader(out)), nil
}

// speedScene is a nested timeline of 12 frames: a clip for 8 frames and another from frame 8 that starts at
// frame 5 of its asset at the volume 0.6.
func speedScene(t *testing.T) *timeline.Document {
	t.Helper()

	child := nestedDoc(t, "prj_c1c1c1c1c1")

	a := timeline.NewItem("itm_c1c1c1c1c1", timeline.ItemClip, "trk_a000000001", 0, 8)
	a.Asset = "ast_a000000001"

	b := timeline.NewItem("itm_c2c2c2c2c2", timeline.ItemClip, "trk_a000000002", 8, 4)
	b.Asset, b.Volume, b.SourceIn = "ast_a000000002", 0.6, 5
	child.Items[a.ID], child.Items[b.ID] = a, b

	return child
}

func channels(mixed []float32) (l, r []float64) {
	for i := 0; i+1 < len(mixed); i += 2 {
		l, r = append(l, float64(mixed[i])), append(r, float64(mixed[i+1]))
	}

	return l, r
}

func TestCompositionSpeedMix(t *testing.T) {
	t.Parallel()

	child := speedScene(t)
	opts := plan.Options{Nested: func(id string) (*timeline.Document, bool) { return child, id == child.ID }}
	o := &speedOpener{streams: map[string][]byte{
		"ast_a000000001": tone("ast_a000000001", 48000),
		"ast_a000000002": tone("ast_a000000002", 48000),
	}}

	// The nested timeline alone, at speed 1: the reference that the composition at a speed reads at its rate.
	alone := mix(t, plan.Audio(child, 0, 12), 12*1600, o)

	for _, c := range []struct {
		name     string
		num, den int
		dur      int
	}{{"half", 1, 2, 24}, {"double", 2, 1, 6}} {
		parent := nestedDoc(t, "prj_f0f0f0f0f0")
		parent.Assets = map[string]timeline.Asset{}

		comp := timeline.NewItem("itm_k1k1k1k1k1", timeline.ItemComposition, "trk_a000000001", 4, c.dur)
		comp.Timeline, comp.Volume = child.ID, 0.5
		comp.Speed = timeline.Rational{Num: c.num, Den: c.den}
		parent.Items[comp.ID] = comp

		total := int64(4+c.dur+4) * 1600
		got := mix(t, plan.AudioWith(parent, 0, 4+c.dur+4, opts), total, o)

		// Parent sample at + j is the nested sample j x n / d, scaled by the bus gain 0.5: the nested mix read
		// at the speed of the item, to the float.
		at := int64(4 * 1600)

		for j := range int64(c.dur) * 1600 {
			src := j * int64(c.num) / int64(c.den)
			for ch := range int64(2) {
				want, have := alone[src*2+ch]*0.5, got[(at+j)*2+ch]
				if want != have {
					t.Fatalf(
						"%s: parent sample %d is %v, the nested sample %d at the bus gain is %v",
						c.name,
						at+j,
						have,
						src,
						want,
					)
				}
			}
		}

		// Nothing sounds before the item or after it.
		for j := range at * 2 {
			if got[j] != 0 {
				t.Fatalf("%s: sample %d before the item is %v", c.name, j, got[j])
			}
		}

		for j := (at + int64(c.dur)*1600) * 2; j < int64(len(got)); j++ {
			if got[j] != 0 {
				t.Fatalf("%s: sample %d after the item is %v", c.name, j, got[j])
			}
		}

		l, r := channels(got[at*2 : (at+int64(c.dur)*1600)*2])
		golden(t, "composition_speed_"+c.name, l, r)
	}
}
