package plan

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	tlChild = "prj_c1c1c1c1c1"
	tlInner = "prj_d2d2d2d2d2"
	tlDeep  = "prj_e3e3e3e3e3"
	trkPar  = "trk_parent00001"
	trkOver = "trk_overlay0001"
	compID  = "itm_comp0000001"
)

// production is the resolver of a test: the timelines of the project by id.
type production map[string]*timeline.Document

func (pr production) resolve(id string) (*timeline.Document, bool) {
	d, ok := pr[id]

	return d, ok
}

func (pr production) opts() Options { return Options{Measure: fakeMeasure, Nested: pr.resolve} }

// childDoc is a timeline of the project with the tracks and assets of newDoc and the given id.
func childDoc(id string) *timeline.Document {
	d := newDoc()
	d.ID = id

	return d
}

// parentDoc is the timeline that uses compositions: a track for them and one above for an overlay.
func parentDoc() *timeline.Document {
	d := newDoc()
	d.Tracks = map[string]timeline.Track{
		trkOver:  {ID: trkOver, Kind: timeline.TrackVisual, Order: "a0"},
		trkPar:   {ID: trkPar, Kind: timeline.TrackVisual, Order: "a1"},
		trkSound: {ID: trkSound, Kind: timeline.TrackAudio, Order: "a2"},
	}

	return d
}

func composition(id, track, nested string, start, dur int, mods ...func(*timeline.Item)) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemComposition, track, start, dur)
	it.Timeline = nested

	for _, m := range mods {
		m(&it)
	}

	return it
}

// scene is a child timeline with a clip on each of its two tracks and a text over them, 90 frames long.
func scene(id string) *timeline.Document {
	d := childDoc(id)
	c1 := clip("itm_c1", trkBottom, 0, 90)
	c2 := clip("itm_c2", trkTop, 10, 60)
	c2.Scale = 0.5
	tx := timeline.NewItem("itm_tx", timeline.ItemText, trkTop, 70, 20)
	tx.Text = "Hello"
	put(d, c1)
	put(d, c2)
	put(d, tx)

	return d
}

// stripComp removes the path of the composition item from the layer ids, to compare nested layers with the
// layers of the timeline they came from.
func stripComp(layers []Layer) []Layer {
	out := slices.Clone(layers)
	for i := range out {
		out[i].ItemID = strings.TrimPrefix(out[i].ItemID, compID+"/")
	}

	return out
}

func TestNestedFlatteningEqualsInlining(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	parent := parentDoc()
	overlay := clip("itm_ov", trkOver, 20, 100)
	put(parent, overlay)
	put(parent, composition(compID, trkPar, tlChild, 30, 90))

	// The same project with the items of the child put on the parent's timeline, shifted by the start of the
	// composition, under the overlay in the order they had.
	inlined := parentDoc()
	put(inlined, overlay)
	inlined.Tracks[trkTop] = timeline.Track{ID: trkTop, Kind: timeline.TrackVisual, Order: "b0"}
	inlined.Tracks[trkBottom] = timeline.Track{ID: trkBottom, Kind: timeline.TrackVisual, Order: "b1"}

	for _, it := range child.Items {
		it.Start += 30
		put(inlined, it)
	}

	opts := production{tlChild: child}.opts()
	plain := Options{Measure: fakeMeasure}

	for f := range 140 {
		got := FrameWith(parent, f, opts)
		want := FrameWith(inlined, f, plain)

		got.Layers = stripComp(got.Layers)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf(
				"frame %d: flattened plan differs from the inlined one\n got %+v\nwant %+v",
				f,
				got.Layers,
				want.Layers,
			)
		}
	}

	// And the window plan: the same layers with the same series, in the same order.
	gotW := WindowWith(parent, 0, 140, opts)
	wantW := WindowWith(inlined, 0, 140, plain)

	if len(gotW.Layers) != len(wantW.Layers) {
		t.Fatalf("window has %d layers, want %d", len(gotW.Layers), len(wantW.Layers))
	}

	for i := range gotW.Layers {
		g, w := gotW.Layers[i], wantW.Layers[i]
		g.Item = strings.TrimPrefix(g.Item, compID+"/")

		if !reflect.DeepEqual(g, w) {
			t.Errorf("window layer %d: got %+v, want %+v", i, g, w)
		}
	}
}

func TestNestedFlatLayersAreThePlanOfTheChild(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90, func(i *timeline.Item) { i.SourceIn = 20 }))

	opts := production{tlChild: child}.opts()

	// source_in 20: frame r of the item shows frame 20+r of the child.
	for _, r := range []int{0, 5, 50, 69} {
		got := stripComp(FrameWith(parent, r, opts).Layers)
		want := FrameWith(child, 20+r, opts).Layers

		if !reflect.DeepEqual(got, want) {
			t.Errorf("frame %d: got %+v, want the child's frame %d %+v", r, got, 20+r, want)
		}
	}

	// Past the end of the child (90 frames, 20 skipped): nothing.
	if l := FrameWith(parent, 70, opts).Layers; len(l) != 0 {
		t.Errorf("frame 70 is past the end of the nested timeline and drew %+v", l)
	}
}

func TestNestedFrameRateIsConverted(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	child.Settings.FPS = timeline.Rational{Num: 24, Den: 1}

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90))

	opts := production{tlChild: child}.opts()

	// 30 fps over 24 fps: parent frame 25 is child frame floor(25 * 24 / 30) = 20.
	got := stripComp(FrameWith(parent, 25, opts).Layers)
	want := FrameWith(child, 20, opts).Layers

	if !reflect.DeepEqual(got, want) {
		t.Errorf("frame 25: got %+v, want the child's frame 20 %+v", got, want)
	}
}

func TestNestedOpacityMakesOneGroup(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90, func(i *timeline.Item) { i.Opacity = 0.5 }))

	opts := production{tlChild: child}.opts()
	fp := FrameWith(parent, 15, opts)

	if len(fp.Layers) != 1 {
		t.Fatalf("got %d layers, want one group: %+v", len(fp.Layers), fp.Layers)
	}

	g := fp.Layers[0]
	if g.Kind != Group || g.ItemID != compID || g.Opacity != 0.5 || g.Group == nil {
		t.Fatalf("not a group at half opacity: %+v", g)
	}

	// The nested frame of the child, in the child's own pixels, with its own layers.
	want := stripComp(FrameWith(child, 15, opts).Layers)
	got := stripComp(g.Group.Layers)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("group layers = %+v, want the child's frame %+v", got, want)
	}

	if g.Group.Width != 1080 || g.Group.Height != 1920 || g.Crop != (Rect{W: 1080, H: 1920}) {
		t.Errorf("group frame is %dx%d with crop %+v", g.Group.Width, g.Group.Height, g.Crop)
	}

	if g.W != 1080 || g.H != 1920 || g.CX != 540 || g.CY != 960 {
		t.Errorf("group is placed at %v,%v, %vx%v", g.CX, g.CY, g.W, g.H)
	}
}

func TestNestedGroupPlacementFollowsTheFit(t *testing.T) {
	t.Parallel()

	// A 1920x1080 timeline in a 1080x1920 frame.
	child := scene(tlChild)
	child.Settings.Width, child.Settings.Height = 1920, 1080

	cases := []struct {
		fit          string
		scale        float64
		wantW, wantH float64
	}{
		{"cover", 1, 3413.333, 1920},
		{"contain", 1, 1080, 607.5},
		{"none", 1, 1920, 1080},
		{"contain", 0.5, 540, 303.75},
	}

	for _, c := range cases {
		parent := parentDoc()
		put(
			parent,
			composition(compID, trkPar, tlChild, 0, 90, func(i *timeline.Item) { i.Fit, i.Scale = c.fit, c.scale }),
		)

		fp := FrameWith(parent, 5, production{tlChild: child}.opts())
		if len(fp.Layers) != 1 || fp.Layers[0].Kind != Group {
			t.Fatalf("%s: want one group, got %+v", c.fit, fp.Layers)
		}

		g := fp.Layers[0]
		near(t, c.fit+" w", g.W, c.wantW)
		near(t, c.fit+" h", g.H, c.wantH)

		if g.Group.Width != 1920 || g.Group.Height != 1080 {
			t.Errorf("%s: group frame %dx%d, want 1920x1080", c.fit, g.Group.Width, g.Group.Height)
		}
	}
}

func TestNestedTransformMakesAGroupWhenTheFrameDoesNotLandOnTheParent(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	small := scene(tlInner)
	small.Settings.Width, small.Settings.Height = 540, 960

	cases := map[string]func(*timeline.Item){
		"scale":    func(i *timeline.Item) { i.Scale = 0.9 },
		"rotation": func(i *timeline.Item) { i.Rotation = 5 },
		"x":        func(i *timeline.Item) { x := 100.0; i.X = &x },
		"blend":    func(i *timeline.Item) { i.Blend = "multiply" },
		"mask":     func(i *timeline.Item) { i.Mask = &timeline.Mask{Shape: "rect", Width: 0.5, Height: 0.5} },
		"blur":     func(i *timeline.Item) { i.Blur = 3 },
		"effect":   func(i *timeline.Item) { i.Effects = []timeline.Effect{{Kind: "vignette"}} },
		"shadow":   func(i *timeline.Item) { i.Shadow = &timeline.Shadow{Color: "#000000", Opacity: 1} },
		"flip":     func(i *timeline.Item) { i.FlipX = true },
		"filter":   func(i *timeline.Item) { i.Filter = "sepia" },
		"keyframe": func(i *timeline.Item) {
			i.Keyframes = timeline.Keyframes{"x": {{T: 0, Value: 540}, {T: 10, Value: 560}}}
		},
		"transition": func(i *timeline.Item) { i.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10} },
	}

	for name, mod := range cases {
		parent := parentDoc()
		put(parent, composition(compID, trkPar, tlChild, 0, 90, mod))

		fp := FrameWith(parent, 20, production{tlChild: child}.opts())
		if len(fp.Layers) != 1 || fp.Layers[0].Kind != Group {
			t.Errorf("%s: want one group layer, got %+v", name, fp.Layers)
		}
	}

	// A nested frame of another size is a group too, even with nothing of its own on the item.
	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlInner, 0, 90))

	if fp := FrameWith(
		parent,
		20,
		production{tlInner: small}.opts(),
	); len(fp.Layers) != 1 ||
		fp.Layers[0].Kind != Group {
		t.Errorf("a nested frame of another size should be a group, got %+v", fp.Layers)
	}
}

func TestNestedTwoLevelsAndTheSameTimelineTwice(t *testing.T) {
	t.Parallel()

	inner := scene(tlInner)
	middle := childDoc(tlChild)
	put(middle, composition("itm_mid", trkTop, tlInner, 0, 90))
	put(middle, composition("itm_mid2", trkBottom, tlInner, 0, 90))

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90))

	pr := production{tlChild: middle, tlInner: inner}
	fp := FrameWith(parent, 15, pr.opts())

	seen := map[string]bool{}
	for _, l := range fp.Layers {
		if seen[l.ItemID] {
			t.Errorf("two layers have the id %q", l.ItemID)
		}

		seen[l.ItemID] = true

		if !strings.HasPrefix(l.ItemID, compID+"/itm_mid") {
			t.Errorf("layer %q is not under a nested composition item", l.ItemID)
		}
	}

	// itm_c1 (90 frames), itm_c2 (frames 10 to 69) at frame 15, each in both uses of the timeline.
	if want := 4; len(fp.Layers) != want {
		t.Errorf("got %d layers, want %d: %+v", len(fp.Layers), want, fp.Layers)
	}
}

func TestNestedAMissingTimelineDrawsNothing(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90))

	for name, opts := range map[string]Options{
		"no resolver": {Measure: fakeMeasure},
		"unknown id":  production{}.opts(),
	} {
		if l := FrameWith(parent, 10, opts).Layers; len(l) != 0 {
			t.Errorf("%s: drew %+v", name, l)
		}
	}

	d := childDoc(tlChild)
	put(d, clip("itm_x", trkTop, 0, 90))

	off := parentDoc()
	put(off, composition(compID, trkPar, tlChild, 0, 90, func(i *timeline.Item) { i.Disabled = true }))

	if l := FrameWith(off, 10, production{tlChild: d}.opts()).Layers; len(l) != 0 {
		t.Errorf("a disabled composition drew %+v", l)
	}
}

func TestNestedLoopsAndDepthAreBounded(t *testing.T) {
	t.Parallel()

	// A holds itself through B, which a stored project cannot, but a document edited by hand can.
	a := childDoc(tlChild)
	b := childDoc(tlInner)
	put(a, composition("itm_a2b", trkTop, tlInner, 0, 90))
	put(b, composition("itm_b2a", trkTop, tlChild, 0, 90))
	put(b, clip("itm_bclip", trkBottom, 0, 90))

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90))

	fp := FrameWith(parent, 5, production{tlChild: a, tlInner: b}.opts())
	if len(fp.Layers) != 1 || !strings.HasSuffix(fp.Layers[0].ItemID, "/itm_bclip") {
		t.Errorf("the loop should end after B's own clip, got %+v", fp.Layers)
	}

	// A chain of 6 distinct timelines: the plan stops at depth 4, the clip of the last is not drawn.
	pr := production{}
	var ids []string

	for i := range 6 {
		ids = append(ids, "prj_k"+strings.Repeat(string(rune('a'+i)), 9))
	}

	for i, id := range ids {
		d := childDoc(id)
		if i+1 < len(ids) {
			put(d, composition("itm_next", trkTop, ids[i+1], 0, 90))
		} else {
			put(d, clip("itm_leaf", trkTop, 0, 90))
		}

		pr[id] = d
	}

	deep := parentDoc()
	put(deep, composition(compID, trkPar, ids[0], 0, 90))

	if l := FrameWith(deep, 5, pr.opts()).Layers; len(l) != 0 {
		t.Errorf("a chain deeper than %d drew %+v", timeline.MaxCompositionDepth, l)
	}

	// Four levels of nesting under the top do draw.
	four := parentDoc()
	put(four, composition(compID, trkPar, ids[2], 0, 90))

	if l := FrameWith(four, 5, pr.opts()).Layers; len(l) != 1 || !strings.Contains(l[0].ItemID, "/itm_leaf") {
		t.Errorf("a chain of four compositions should draw its leaf, got %+v", l)
	}
}

func TestNestedWindowOfAGroup(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)
	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 10, 60, func(i *timeline.Item) { i.Opacity = 0.5 }))

	w := WindowWith(parent, 0, 80, production{tlChild: child}.opts())
	if len(w.Layers) != 1 {
		t.Fatalf("got %d window layers, want the one group: %+v", len(w.Layers), w.Layers)
	}

	g := w.Layers[0]
	if g.Kind != Group || g.Group == nil || g.First != 10 || g.Last != 69 {
		t.Fatalf("group window layer is %+v", g)
	}

	if g.Group.Width != 1080 || g.Group.Height != 1920 {
		t.Errorf("group frame %dx%d", g.Group.Width, g.Group.Height)
	}

	var ids []string

	for _, l := range g.Group.Layers {
		ids = append(ids, l.Item)
	}

	// Bottom first: the clip of the lower track, then the clip of the upper one (the text starts at child frame 70).
	want := []string{compID + "/itm_c1", compID + "/itm_c2"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("nested window layers %v, want %v", ids, want)
	}

	// itm_c2 is in the child from frame 10 to 69; the composition starts at 10 and shows child frames 0 to 59,
	// so it is present on parent frames 20 to 69.
	c2 := g.Group.Layers[1]
	if c2.First != 20 || c2.Last != 69 || len(c2.Opacity) != 50 {
		t.Errorf("itm_c2 window layer: first %d last %d, %d values", c2.First, c2.Last, len(c2.Opacity))
	}

	if len(g.Opacity) != 60 {
		t.Errorf("group opacity has %d values, want 60", len(g.Opacity))
	}
}

// goldenNested compares the plan of a scene with its golden file.
func goldenNested(t *testing.T, name string, doc *timeline.Document, opts Options, frames []int) {
	t.Helper()

	type golden struct {
		Frames []FramePlan `json:"frames"`
		Window WindowPlan  `json:"window"`
	}

	g := golden{Window: WindowWith(doc, 0, 100, opts)}
	for _, f := range frames {
		g.Frames = append(g.Frames, FrameWith(doc, f, opts))
	}

	got, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	got = append(got, '\n')
	path := filepath.Join("testdata", "nested", name+".plan.json")

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("plan of %s differs from %s (run with -update after reviewing)", name, path)
	}
}

func TestNestedGoldenPlans(t *testing.T) {
	t.Parallel()

	child := scene(tlChild)

	flat := parentDoc()
	put(flat, composition(compID, trkPar, tlChild, 5, 90))
	put(flat, clip("itm_ov", trkOver, 20, 60))
	goldenNested(t, "flat", flat, production{tlChild: child}.opts(), []int{0, 5, 20, 40, 80, 99})

	half := parentDoc()
	put(half, composition(compID, trkPar, tlChild, 5, 90, func(i *timeline.Item) { i.Opacity = 0.5 }))
	goldenNested(t, "group-opacity", half, production{tlChild: child}.opts(), []int{0, 5, 20, 40, 80, 99})

	wide := childDoc(tlInner)
	wide.Settings.Width, wide.Settings.Height = 1920, 1080
	put(wide, clip("itm_w", trkTop, 0, 90))

	mid := childDoc(tlDeep)
	put(mid, composition("itm_mid", trkTop, tlInner, 0, 90))

	two := parentDoc()
	put(two, composition(compID, trkPar, tlDeep, 0, 90, func(i *timeline.Item) {
		i.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8}
	}))
	goldenNested(t, "two-levels-mask", two, production{tlDeep: mid, tlInner: wide}.opts(), []int{0, 30, 89})
}

// A random child, a random parent placement: the group and the flat form never panic and always hold the
// ids of the layers they came from.
func TestNestedRandomStaysConsistent(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(7))

	for range 200 {
		child := scene(tlChild)
		if rng.Intn(2) == 0 {
			child.Settings.Width, child.Settings.Height = 1920, 1080
		}

		parent := parentDoc()
		put(parent, composition(compID, trkPar, tlChild, rng.Intn(30), 10+rng.Intn(100), func(i *timeline.Item) {
			i.SourceIn = rng.Intn(60)
			i.Opacity = []float64{1, 0.5}[rng.Intn(2)]
			i.Fit = []string{"cover", "contain", "none"}[rng.Intn(3)]
		}))

		opts := production{tlChild: child}.opts()
		w := WindowWith(parent, rng.Intn(50), 50+rng.Intn(100), opts)

		for _, l := range w.Layers {
			if !strings.HasPrefix(l.Item, compID) {
				t.Fatalf("window layer %q is not the composition or under it", l.Item)
			}
		}

		for f := range 120 {
			for _, l := range FrameWith(parent, f, opts).Layers {
				if !strings.HasPrefix(l.ItemID, compID) {
					t.Fatalf("frame %d layer %q is not the composition or under it", f, l.ItemID)
				}
			}
		}
	}
}
