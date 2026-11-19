package timeline

import (
	"fmt"
	"slices"
	"testing"
)

// compDoc is a timeline with one visual track and one composition item per nested timeline id.
func compDoc(id string, nested ...string) *Document {
	d := mustDecodeDoc(id)
	for i, n := range nested {
		it := NewItem(fmt.Sprintf("itm_c%09d", i), ItemComposition, "trk_t000000001", i*10, 10)
		it.Timeline = n
		d.Items[it.ID] = it
	}

	return d
}

func mustDecodeDoc(id string) *Document {
	d, err := Decode([]byte(`{"schema_version":1,"id":"` + id + `","name":"T","settings":{},"assets":{},` +
		`"tracks":{"trk_t000000001":{"id":"trk_t000000001","kind":"visual","order":"a0"}},"items":{}}`))
	if err != nil {
		panic(err)
	}

	return d
}

type mapGraph map[string]*Document

func (g mapGraph) Doc(id string) (*Document, bool) { d, ok := g[id]; return d, ok }

func (g mapGraph) Users(id string) []string {
	var out []string

	for uid, d := range g {
		if slices.Contains(CompositionTimelines(d), id) {
			out = append(out, uid)
		}
	}

	slices.Sort(out)

	return out
}

const (
	tA = "prj_a000000001"
	tB = "prj_b000000002"
	tC = "prj_c000000003"
	tD = "prj_d000000004"
	tE = "prj_e000000005"
	tF = "prj_f000000006"
)

func TestCompositionValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Document)
		want string // "" is valid
	}{
		{"valid", func(*Document) {}, ""},
		{
			"itself",
			func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Timeline = d.ID }) },
			VComposition,
		},
		{"no timeline", func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Timeline = "" }) }, VShape},
		{"timeline form", func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Timeline = "x" }) }, VID},
		{
			"speed",
			func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Speed = Rational{2, 1} }) },
			VComposition,
		},
		{
			"asset",
			func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Asset = "ast_3c5v8n2w6d" }) },
			VShape,
		},
		{
			"crop",
			func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Crop = &Crop{Left: 0.1} }) },
			VShape,
		},
		{"chroma key", func(d *Document) {
			editItem(d, "itm_c000000000", func(i *Item) { i.ChromaKey = &ChromaKey{Color: "#00FF00"} })
		}, VShape},
		{"denoise", func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Denoise = 0.5 }) }, VShape},
		{
			"fades",
			func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.FadeIn, i.FadeOut = 8, 8 }) },
			VFades,
		},
		{"volume", func(d *Document) { editItem(d, "itm_c000000000", func(i *Item) { i.Volume = 5 }) }, VShape},
		{"audio track", func(d *Document) {
			d.Tracks["trk_a000000001"] = Track{ID: "trk_a000000001", Kind: TrackAudio, Order: "b0"}
			editItem(d, "itm_c000000000", func(i *Item) { i.Track = "trk_a000000001" })
		}, VItemTrack},
		{"slot", func(d *Document) {
			editItem(
				d,
				"itm_c000000000",
				func(i *Item) { i.Slot = &Slot{Name: "intro", Kind: "media", DurationPolicy: "fit"} },
			)
		}, VSlot},
		{"on a clip", func(d *Document) {
			editItem(d, "itm_c000000000", func(i *Item) { i.Type, i.Timeline = ItemText, tB; i.Text = "x" })
		}, VShape},
		{"opacity and a mask are fine", func(d *Document) {
			editItem(d, "itm_c000000000", func(i *Item) {
				i.Opacity = 0.5
				i.Mask = &Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5}
				i.Volume = 0.5
				i.Shadow = &Shadow{Color: "#000000", Opacity: 0.5}
			})
		}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := compDoc(tA, tB)
			c.edit(d)

			got := codesOf(Validate(d))
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want valid, got %v", Validate(d))
				}

				return
			}

			if len(got) != 1 || got[0] != c.want {
				t.Fatalf("got %v, want %s (%v)", got, c.want, Validate(d))
			}
		})
	}
}

func TestCompositionRoundTrip(t *testing.T) {
	d := compDoc(tA, tB)
	editItem(d, "itm_c000000000", func(i *Item) { i.SourceIn, i.Opacity = 12, 0.5 })

	back, err := Decode(Canonical(d))
	if err != nil {
		t.Fatal(err)
	}

	if got := back.Items["itm_c000000000"]; got.Type != ItemComposition || got.Timeline != tB || got.SourceIn != 12 {
		t.Fatalf("round trip lost the composition: %+v", got)
	}
}

func TestNestingCheckPlacement(t *testing.T) {
	// A holds B, B holds C, C holds D, D holds E: A has depth 4, E and F none.
	chain := mapGraph{
		tA: compDoc(tA, tB), tB: compDoc(tB, tC), tC: compDoc(tC, tD), tD: compDoc(tD, tE),
		tE: compDoc(tE), tF: compDoc(tF),
	}

	cases := []struct {
		name        string
		g           mapGraph
		host, child string
		kind        NestingKind
		depth       int
		path        []string
	}{
		{"leaf in leaf", chain, tF, tE, "", 0, nil},
		{"itself", chain, tA, tA, NestingCycle, 0, []string{tA}},
		{"direct", chain, tB, tA, NestingCycle, 0, []string{tA, tB}},
		{"indirect", chain, tE, tA, NestingCycle, 0, []string{tA, tB, tC, tD, tE}},
		{"depth five by the child", chain, tF, tA, NestingDepth, 5, nil},
		{"depth five by the timelines above", chain, tE, tF, NestingDepth, 5, nil},
		{"depth four exactly", chain, tF, tB, "", 0, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := CheckPlacement(c.g, c.host, c.child)
			if c.kind == "" {
				if n != nil {
					t.Fatalf("want a legal placement, got %+v", n)
				}

				return
			}

			if n == nil || n.Kind != c.kind || n.Depth != c.depth || !slices.Equal(n.Path, c.path) {
				t.Fatalf("got %+v, want %s depth %d path %v", n, c.kind, c.depth, c.path)
			}
		})
	}
}

func TestNestingDepthHelpers(t *testing.T) {
	g := mapGraph{tA: compDoc(tA, tB, tC), tB: compDoc(tB, tC), tC: compDoc(tC, tD), tD: compDoc(tD)}

	if got := DepthOf(g, tA); got != 3 {
		t.Errorf("DepthOf(A) = %d, want 3", got)
	}

	if got := DepthAbove(g, tD); got != 3 {
		t.Errorf("DepthAbove(D) = %d, want 3", got)
	}

	if got := DepthOf(g, "prj_z000000009"); got != 0 {
		t.Errorf("an unknown timeline has depth %d, want 0", got)
	}

	// A graph that already holds a loop ends the walk.
	loop := mapGraph{tA: compDoc(tA, tB), tB: compDoc(tB, tA)}
	if got := DepthOf(loop, tA); got < depthGuard {
		t.Errorf("a loop gave depth %d, want at least the guard", got)
	}
}
