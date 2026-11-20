package audio

import (
	"math"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// nestedDoc decodes a small timeline of the given id with one visual track.
func nestedDoc(t *testing.T, id string) *timeline.Document {
	t.Helper()

	d, err := timeline.Decode([]byte(`{"schema_version":1,"id":"` + id + `","name":"T","settings":{},` +
		`"assets":{"ast_a000000001":{"id":"ast_a000000001","hash":"sha256:` + zeros(63) + `1","kind":"video","name":"a.mp4",` +
		`"frame_rate":{"num":30,"den":1},"frame_count":900,"width":1080,"height":1920,"has_audio":true,"sample_count":1440000},` +
		`"ast_a000000002":{"id":"ast_a000000002","hash":"sha256:` + zeros(63) + `2","kind":"video","name":"b.mp4",` +
		`"frame_rate":{"num":30,"den":1},"frame_count":900,"width":1080,"height":1920,"has_audio":true,"sample_count":1440000}},` +
		`"tracks":{"trk_a000000001":{"id":"trk_a000000001","kind":"visual","order":"a0"},` +
		`"trk_a000000002":{"id":"trk_a000000002","kind":"visual","order":"a1"}},"items":{}}`))
	if err != nil {
		t.Fatal(err)
	}

	return d
}

func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}

	return string(b)
}

// TestNestedMix: the sound of a composition is the mix of its timeline, scaled by the gain of the item and put
// where the item is. Mixing the parent gives, sample for sample, the mix of the nested timeline alone times the
// bus gain and shifted to the item's start (SDD-004 section 7, "Compositions"): there is no other bus.
func TestNestedMix(t *testing.T) {
	t.Parallel()

	child := nestedDoc(t, "prj_c1c1c1c1c1")
	for _, it := range []timeline.Item{
		func() timeline.Item {
			i := timeline.NewItem("itm_c1c1c1c1c1", timeline.ItemClip, "trk_a000000001", 0, 60)
			i.Asset = "ast_a000000001"
			return i
		}(),
		func() timeline.Item {
			i := timeline.NewItem("itm_c2c2c2c2c2", timeline.ItemClip, "trk_a000000002", 20, 70)
			i.Asset, i.Volume, i.SourceIn = "ast_a000000002", 0.6, 30
			return i
		}(),
	} {
		child.Items[it.ID] = it
	}

	parent := nestedDoc(t, "prj_f0f0f0f0f0")
	parent.Assets = map[string]timeline.Asset{}

	comp := timeline.NewItem("itm_k1k1k1k1k1", timeline.ItemComposition, "trk_a000000001", 45, 90)
	comp.Timeline, comp.Volume, comp.FadeIn = "prj_c1c1c1c1c1", 0.5, 15
	parent.Items[comp.ID] = comp

	opts := plan.Options{Nested: func(id string) (*timeline.Document, bool) { return child, id == child.ID }}
	o := &fakeOpener{streams: map[string][]byte{
		"ast_a000000001": tone("ast_a000000001", 48000*4),
		"ast_a000000002": tone("ast_a000000002", 48000*4),
	}}

	const parentSamples = 1600 * 150

	got := mix(t, plan.AudioWith(parent, 0, 150, opts), parentSamples, o)
	alone := mix(t, plan.Audio(child, 0, 90), 1600*90, o)

	// The parent's sample 45*1600 + j is the child's sample j; the bus is 0.5 after a fade-in of 15 frames.
	bus := func(j int) float64 {
		if j < 15*1600 {
			return 0.5 * float64(j) / float64(15*1600)
		}

		return 0.5
	}

	worst := 0.0

	for j := range 1600 * 90 {
		for ch := range 2 {
			want := float64(alone[j*2+ch]) * bus(j)
			have := float64(got[(45*1600+j)*2+ch])
			worst = math.Max(worst, math.Abs(want-have))
		}
	}

	// The envelope is the product of two segments cut in quarters, so a ramp is followed to within a small
	// fraction of the sample range; the constant parts are exact.
	if worst > 0.002 {
		t.Errorf("the nested mix differs from the bus-scaled mix of the nested timeline by %v", worst)
	}

	// Nothing sounds before the item or after it.
	for j := range 45 * 1600 * 2 {
		if got[j] != 0 {
			t.Fatalf("sample %d before the item is %v", j, got[j])
		}
	}

	for j := (45 + 90) * 1600 * 2; j < len(got); j++ {
		if got[j] != 0 {
			t.Fatalf("sample %d after the item is %v", j, got[j])
		}
	}

	// The same project with the sources of the child laid on the parent (no composition) and the gain of the bus
	// by hand: the constant stretch of the bus, after the fade, is the very float.
	for j := 15 * 1600; j < 1600*90; j += 997 {
		for ch := range 2 {
			if want, have := alone[j*2+ch]*0.5, got[(45*1600+j)*2+ch]; want != have {
				t.Fatalf("sample %d: nested %v, bus-scaled %v", j, have, want)
			}
		}
	}
}
