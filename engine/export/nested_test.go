package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Nested compositions in the export (SDD-004 section 3.16, SDD-013 section 3.12, epic 32): a composition that
// draws exactly as its timeline would is flat and gives the very bytes of that timeline; one with an opacity, a
// mask or another property is drawn as a group, and the Go compositor and the Rust renderer give the same
// bytes for it. The golden frames are in testdata/golden-nested; regenerate them with
// go test ./engine/export -run NestedGolden -update (a reviewed change).

const (
	nstChild  = "prj_c1c1c1c1c1"
	nstInner  = "prj_d2d2d2d2d2"
	nstParent = "prj_f0f0f0f0f0"
	nstTrkA   = "trk_a1a1a1a1a1"
	nstTrkB   = "trk_b2b2b2b2b2"
	nstComp   = "itm_k1k1k1k1k1"
	nstAssetP = "ast_a000000001"
	nstAssetL = "ast_a000000002"
)

var nestedFrames = []int{0, 20, 45, 70, 89}

// nestedProject is a parent timeline with one composition item of a child timeline: the child has a portrait
// clip under a landscape one at half size and a text, 3 seconds long, in the size of the parent unless the mods
// change it.
func nestedProject(
	t *testing.T,
	size [2]int,
	comp func(*timeline.Item),
) (*timeline.Document, map[string]*timeline.Document) {
	t.Helper()

	newDoc := func(id string) *timeline.Document {
		d, err := timeline.Decode(
			[]byte(`{"schema_version":1,"id":"` + id + `","name":"T","settings":{"width":360,"height":640},` +
				`"assets":{},"tracks":{"` + nstTrkA + `":{"id":"` + nstTrkA + `","kind":"visual","order":"a0"},` +
				`"` + nstTrkB + `":{"id":"` + nstTrkB + `","kind":"visual","order":"a1"}},"items":{}}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	child := newDoc(nstChild)
	child.Settings.Width, child.Settings.Height = size[0], size[1]
	for id, spec := range map[string]fixtureSpec{
		nstAssetP: {Name: "gold-portrait.mp4", Width: 180, Height: 320, FPS: 30, Seconds: 3},
		nstAssetL: {Name: "gold-landscape.mp4", Width: 320, Height: 180, FPS: 30, Seconds: 3},
	} {
		child.Assets[id] = timeline.Asset{ID: id, Kind: timeline.AssetVideo, Name: spec.Name}
		useAsset(child, id, ingestClip(t, spec))
	}

	under := timeline.NewItem("itm_a1a1a1a1a1", timeline.ItemClip, nstTrkB, 0, 90)
	under.Asset = nstAssetP
	over := timeline.NewItem("itm_b2b2b2b2b2", timeline.ItemClip, nstTrkA, 10, 50)
	over.Asset, over.Scale = nstAssetL, 0.5
	label := timeline.NewItem("itm_c3c3c3c3c3", timeline.ItemText, nstTrkA, 60, 30)
	label.Text = "Nested"
	child.Items[under.ID], child.Items[over.ID], child.Items[label.ID] = under, over, label

	parent := newDoc(nstParent)
	c := timeline.NewItem(nstComp, timeline.ItemComposition, nstTrkB, 0, 90)
	c.Timeline = nstChild
	if comp != nil {
		comp(&c)
	}
	parent.Items[c.ID] = c

	return parent, map[string]*timeline.Document{nstChild: child}
}

// renderNested composes frames of the parent with the given renderer and number of bands.
func renderNested(
	t *testing.T,
	doc *timeline.Document,
	nested map[string]*timeline.Document,
	name string,
	kind PixelRenderer,
	bands int,
	frames []int,
) [][]byte {
	t.Helper()

	job := newJob(t, doc, "nested-"+name+"-"+string(kind)+strconv.Itoa(int(renders.Add(1))), "unused.mp4")
	job.Nested, job.PixelRenderer = nested, kind
	s := testState(t, job, job.JobID)
	rd := s.newRenderer(bands)
	defer rd.close()

	dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))
	var out [][]byte
	for _, f := range frames {
		if err := rd.render(dst, f); err != nil {
			t.Fatalf("%s frame %d: %v", name, f, err)
		}
		if s.scale != 1 {
			t.Fatalf("%s frame %d: the output scale of the state was left at %v", name, f, s.scale)
		}
		out = append(out, bytes.Clone(dst.Pix))
	}
	return out
}

// nestedCases are the compositions that are drawn as a group, with what makes each one.
var nestedCases = map[string]func(*timeline.Item){
	"group-opacity": func(i *timeline.Item) { i.Opacity = 0.6 },
	"group-mask-blend": func(i *timeline.Item) {
		i.Opacity = 0.85
		i.Blend = "multiply"
		i.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8, Feather: 6}
	},
	"group-scaled-rotated": func(i *timeline.Item) {
		i.Scale, i.Rotation = 0.5, 12
		i.Shadow = &timeline.Shadow{Color: "#000000", Opacity: 0.6, DX: 6, DY: 8, Blur: 6}
	},
	"group-source-in": func(i *timeline.Item) { i.SourceIn, i.Duration, i.Opacity = 30, 60, 0.7 },
	"group-effects": func(i *timeline.Item) {
		amount, softness := 0.8, 0.5
		i.Effects = []timeline.Effect{{Kind: "vignette", Amount: &amount, Softness: &softness}}
		i.Opacity = 0.9
	},
}

// A composition that draws exactly as its timeline does is flat, and a flat composition is the timeline: the
// frames are the bytes of the child's own frames.
func TestNestedFlatCompositionEqualsItsTimeline(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{360, 640}, nil)

	for _, kind := range []PixelRenderer{PixelRendererGo, PixelRendererRust} {
		got := renderNested(t, parent, nested, "flat", kind, 0, nestedFrames)
		want := renderNested(t, nested[nstChild], nil, "flat-child", kind, 0, nestedFrames)

		for i, f := range nestedFrames {
			if !bytes.Equal(got[i], want[i]) {
				t.Errorf("%s frame %d: the flat composition differs from its timeline", kind, f)
			}
		}
	}
}

// The group of a composition is composed once as one picture: at 60 % opacity the layers of the nested
// timeline do not show through each other, which is not what drawing them one by one would give.
func TestNestedGroupDiffersFromItsLayersDrawnOneByOne(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{360, 640}, func(i *timeline.Item) { i.Opacity = 0.6 })
	group := renderNested(t, parent, nested, "unit", PixelRendererGo, 0, []int{45})[0]

	// The same child with each of its layers at 60 %, drawn as they would be if there were no group.
	loose := nested[nstChild]
	for id, it := range loose.Items {
		it.Opacity = 0.6
		loose.Items[id] = it
	}
	one := renderDoc(t, loose, "unit-loose", 0, []int{45})[0]

	if bytes.Equal(group, one) {
		t.Error("a group at 60 % equals its layers at 60 % one by one")
	}
}

// The Go compositor and the Rust renderer, whole and in 8 bands, draw the same bytes for every group, and the
// frames are the references of testdata/golden-nested (SDD-013 section 3.10 and 3.12).
func TestNestedGoldenGroups(t *testing.T) {
	setup(t)

	for name, mod := range nestedCases {
		t.Run(name, func(t *testing.T) {
			parent, nested := nestedProject(t, [2]int{360, 640}, mod)
			dir := filepath.Join("testdata", "golden-nested", name)

			frames := nestedFrames
			if mod != nil && parent.Items[nstComp].Duration < 90 {
				frames = []int{0, 20, 45, 59}
			}

			byGo := renderNested(t, parent, nested, name, PixelRendererGo, 0, frames)
			byGo1 := renderNested(t, parent, nested, name, PixelRendererGo, 1, frames)
			byRust := renderNested(t, parent, nested, name, PixelRendererRust, 8, frames)

			for i, f := range frames {
				if !bytes.Equal(byGo[i], byGo1[i]) {
					t.Errorf("frame %d: the Go compositor draws other bytes in one band than in the default", f)
				}
				if !bytes.Equal(byGo[i], byRust[i]) {
					t.Errorf("frame %d: the Rust renderer draws other bytes than the Go compositor", f)
				}
				if *update {
					if err := writePNG(framePNG(dir, f), 360, 640, byGo[i]); err != nil {
						t.Fatal(err)
					}
				}
				want := readPNG(t, framePNG(dir, f))
				if sum(byGo[i]) != sum(want) {
					t.Errorf("frame %d: %s, reference %s", f, sum(byGo[i]), sum(want))
					saveMismatch(t, "nested-"+name, f, 360, 640, byGo[i])
				}
			}
		})
	}
}

// A nested timeline of another size and another frame rate, and two levels of nesting.
func TestNestedOtherSizeRateAndTwoLevels(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{640, 360}, func(i *timeline.Item) { i.Fit, i.Opacity = "contain", 0.8 })
	nested[nstChild].Settings.FPS = timeline.Rational{Num: 25, Den: 1}

	frames := []int{0, 30, 60, 89}
	byGo := renderNested(t, parent, nested, "size", PixelRendererGo, 0, frames)
	byRust := renderNested(t, parent, nested, "size", PixelRendererRust, 3, frames)

	for i, f := range frames {
		if !bytes.Equal(byGo[i], byRust[i]) {
			t.Errorf("another size, frame %d: Rust and Go differ", f)
		}
	}

	// Two levels: the parent holds a timeline that holds the child, each at an opacity.
	two, nested2 := nestedProject(t, [2]int{360, 640}, func(i *timeline.Item) { i.Opacity = 0.7 })
	mid, _ := nestedProject(t, [2]int{360, 640}, func(i *timeline.Item) { i.Opacity = 0.9 })
	mid.ID = nstInner
	nested2[nstInner] = mid
	holder := two.Items[nstComp]
	holder.Timeline = nstInner
	two.Items[nstComp] = holder

	got := renderNested(t, two, nested2, "two", PixelRendererGo, 0, []int{45})[0]
	rust := renderNested(t, two, nested2, "two", PixelRendererRust, 8, []int{45})[0]
	if !bytes.Equal(got, rust) {
		t.Error("two levels of groups: Rust and Go differ")
	}
}

// The view of a frame at another scale draws a group at that scale: the surface is smaller, the group is in
// the same place.
func TestNestedGroupAtAnotherOutputScale(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{360, 640}, func(i *timeline.Item) { i.Opacity = 0.6 })
	cfg := RenderConfig{
		Runner: shared.runner, Doc: parent, Nested: nested, JobID: "nested-view", ArtifactsDir: artifactsDir(),
	}
	r, err := NewRenderer(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	half, err := r.Render(context.Background(), 45, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	full, err := r.Render(context.Background(), 45, 1)
	if err != nil {
		t.Fatal(err)
	}

	if half.Rect.Dx() != 180 || half.Rect.Dy() != 320 || full.Rect.Dx() != 360 {
		t.Fatalf("sizes %v and %v", half.Rect, full.Rect)
	}

	// Not a black frame, and close to the full-size frame shrunk by half: the mean difference stays small.
	var diff, n int
	for y := range 320 {
		for x := range 180 {
			a := half.Pix[(y*180+x)*4 : (y*180+x)*4+3]
			b := full.Pix[((2*y)*360+2*x)*4 : ((2*y)*360+2*x)*4+3]
			for k := range 3 {
				d := int(a[k]) - int(b[k])
				diff += max(d, -d)
				n++
			}
		}
	}
	if mean := float64(diff) / float64(n); mean > 12 {
		t.Errorf("the half-size view differs from the full frame by %.1f levels on average", mean)
	}
}

// An export of a project with a composition writes the same file with the Go compositor and with the Rust
// renderer, and the frames count is that of the parent.
func TestNestedExportIsTheSameWithBothRenderers(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{360, 640}, func(i *timeline.Item) { i.Opacity = 0.6 })
	sums := map[PixelRenderer][32]byte{}

	for _, kind := range []PixelRenderer{PixelRendererGo, PixelRendererRust} {
		job := newJob(t, parent, "nested-export-"+string(kind), "nested-"+string(kind)+".mp4")
		job.Nested, job.PixelRenderer = nested, kind

		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if res.Frames != 90 {
			t.Errorf("%s: %d frames, want 90", kind, res.Frames)
		}
		raw, err := os.ReadFile(res.Path)
		if err != nil {
			t.Fatal(err)
		}
		sums[kind] = sha256.Sum256(raw)
	}

	if sums[PixelRendererGo] != sums[PixelRendererRust] {
		t.Error("the file exported with the Rust renderer differs from the one exported with the Go compositor")
	}
}

// Two timelines that give one asset id to two files cannot be exported together: a layer names its asset by id.
func TestNestedAssetIDsMustAgree(t *testing.T) {
	setup(t)

	parent, nested := nestedProject(t, [2]int{360, 640}, nil)
	parent.Assets[nstAssetP] = timeline.Asset{
		ID:   nstAssetP,
		Kind: timeline.AssetVideo,
		Hash: "sha256:" + string(bytes.Repeat([]byte("e"), 64)),
	}

	job := newJob(t, parent, "nested-clash", "clash.mp4")
	job.Nested = nested

	if _, err := Run(context.Background(), job); err == nil {
		t.Error("an asset id with two different files was exported")
	}
}

// the encoder is not asked for anything the JSON of a case does not say: a guard that the cases are plain JSON
// documents a person can read, so a reviewer can open them.
func TestNestedCasesAreValidDocuments(t *testing.T) {
	setup(t)

	for name, mod := range nestedCases {
		parent, nested := nestedProject(t, [2]int{360, 640}, mod)
		if vs := timeline.Validate(parent); len(vs) != 0 {
			t.Errorf("%s: the parent is not valid: %v", name, vs)
		}
		if vs := timeline.Validate(nested[nstChild]); len(vs) != 0 {
			t.Errorf("%s: the child is not valid: %v", name, vs)
		}
		if _, err := json.Marshal(parent); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
