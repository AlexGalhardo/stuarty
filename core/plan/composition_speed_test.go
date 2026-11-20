package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Epic 53: a composition at a constant speed shows, at frame r of the item, the frame of its timeline that
// SDD-004 section 3.3 gives, with the nested frame rate as the asset's.

func speedOf(num, den int) func(*timeline.Item) {
	return func(i *timeline.Item) { i.Speed = timeline.Rational{Num: num, Den: den} }
}

// childSourceFrame is the source frame of the first clip of the scene that the plan of the parent shows at
// frame f, 0 and false when the layer is absent.
func childSourceFrame(t *testing.T, parent *timeline.Document, opts Options, f int) (int, bool) {
	t.Helper()

	layers := FrameWith(parent, f, opts).Layers
	for i := range layers {
		if layers[i].ItemID == compID+"/itm_c1" {
			return layers[i].SourceFrame, true
		}
	}

	return 0, false
}

func TestCompositionSpeedChildFrame(t *testing.T) {
	t.Parallel()

	opts := production{tlChild: scene(tlChild)}.opts()

	for _, c := range []struct{ num, den, dur int }{
		{1, 4, 360}, {1, 2, 180}, {2, 3, 135}, {1, 1, 90}, {3, 2, 60}, {2, 1, 45}, {4, 1, 23},
	} {
		parent := parentDoc()
		put(parent, composition(compID, trkPar, tlChild, 10, c.dur, speedOf(c.num, c.den), func(i *timeline.Item) {
			i.SourceIn = 3
		}))

		for r := range c.dur {
			want := 3 + r*c.num/c.den
			got, ok := childSourceFrame(t, parent, opts, 10+r)

			if want >= 90 {
				if ok {
					t.Fatalf(
						"%d/%d frame %d: the item shows the nested frame %d, past the end of its timeline",
						c.num,
						c.den,
						r,
						want,
					)
				}

				continue
			}

			if !ok || got != want {
				t.Fatalf("%d/%d frame %d: source frame %d (present %v), want %d", c.num, c.den, r, got, ok, want)
			}
		}
	}
}

// The group form, drawn as one unit, shows the same nested frame as the flat form.
func TestCompositionSpeedGroupFollowsTheSameFrame(t *testing.T) {
	t.Parallel()

	opts := production{tlChild: scene(tlChild)}.opts()
	flat, group := parentDoc(), parentDoc()
	put(flat, composition(compID, trkPar, tlChild, 0, 45, speedOf(2, 1)))
	put(group, composition(compID, trkPar, tlChild, 0, 45, speedOf(2, 1), func(i *timeline.Item) { i.Opacity = 0.5 }))

	for r := range 45 {
		var inner []Layer

		for _, l := range FrameWith(group, r, opts).Layers {
			if l.Kind == Group {
				inner = l.Group.Layers
			}
		}

		want := FrameWith(flat, r, opts).Layers

		if len(inner) != len(want) {
			t.Fatalf("frame %d: the group has %d layers, the flat form %d", r, len(inner), len(want))
		}

		for i := range inner {
			if inner[i].ItemID != want[i].ItemID || inner[i].SourceFrame != want[i].SourceFrame {
				t.Fatalf("frame %d layer %d: group %s at %d, flat %s at %d",
					r, i, inner[i].ItemID, inner[i].SourceFrame, want[i].ItemID, want[i].SourceFrame)
			}
		}
	}
}

// A composition of ten minutes at 3/2 does not drift: frame r shows floor(r x 3/2) of the nested timeline
// whatever the frames before it, in the one place the rule lives.
func TestCompositionSpeedNoDrift(t *testing.T) {
	t.Parallel()

	const frames = 10 * 60 * 30

	long := childDoc(tlChild)
	put(long, clip("itm_c1", trkBottom, 0, frames*3/2+10))

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, frames, speedOf(3, 2)))

	it := parent.Items[compID]
	p := newPlanner(parent, production{tlChild: long}.opts())
	k := p.nested(&it)

	if k == nil {
		t.Fatal("the nested timeline was not resolved")
	}

	prev := -1

	for r := range frames {
		cf, ok := k.frameAt(&it, r, parent.Settings.FPS)
		if !ok || cf != r*3/2 {
			t.Fatalf("frame %d shows nested frame %d (present %v), want %d", r, cf, ok, r*3/2)
		}

		if prev >= 0 && cf-prev != 1 && cf-prev != 2 {
			t.Fatalf("frame %d advances the nested timeline by %d frames", r, cf-prev)
		}

		prev = cf
	}

	// The last frame of the item has read every frame of the range the item owns: ceil(frames x 3/2) of them.
	if want := it.SourceFrames(frames); want != frames*3/2 {
		t.Errorf("the item reads %d frames, want %d", want, frames*3/2)
	}
}

func TestCompositionSpeedGoldenPlans(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	opts := production{tlChild: child}.opts()
	frames := []int{0, 5, 6, 7, 20, 40, 60, 80, 99}

	for _, c := range []struct {
		name     string
		dur      int
		num, den int
		mods     []func(*timeline.Item)
	}{
		{"speed-half", 180, 1, 2, nil},
		{"speed-one", 90, 1, 1, nil},
		{"speed-double", 45, 2, 1, nil},
		{"speed-double-group", 45, 2, 1, []func(*timeline.Item){func(i *timeline.Item) { i.Opacity = 0.5 }}},
	} {
		doc := parentDoc()
		put(
			doc,
			composition(
				compID,
				trkPar,
				tlChild,
				5,
				c.dur,
				append([]func(*timeline.Item){speedOf(c.num, c.den)}, c.mods...)...),
		)
		goldenNested(t, c.name, doc, opts, frames)
	}
}
