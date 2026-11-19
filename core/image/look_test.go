package image

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

func TestAdjustToneTableIsIdentityWhenNeutral(t *testing.T) {
	if _, ok := ToneTable(nil); ok {
		t.Fatal("a nil adjustment has a tone table")
	}

	if _, ok := ToneTable(&Adjust{Brightness: 20, Hue: 5}); ok {
		t.Fatal("the colour matrix keys alone make a tone table")
	}

	tab, ok := ToneTable(&Adjust{Curve: []CurvePoint{{X: 0, Y: 0}, {X: 255, Y: 255}}})
	if !ok {
		t.Fatal("a curve makes a table")
	}

	for v := range tab {
		if int(tab[v]) != v {
			t.Fatalf("the identity curve moves %d to %d", v, tab[v])
		}
	}
}

func TestAdjustToneTableStages(t *testing.T) {
	// One stop of exposure doubles, clamped at 255.
	tab, _ := ToneTable(&Adjust{Exposure: 100})
	if tab[10] != 20 || tab[127] != 254 || tab[128] != 255 || tab[255] != 255 {
		t.Errorf("exposure +1 stop: 10->%d 127->%d 128->%d", tab[10], tab[127], tab[128])
	}

	// Levels map the black and white points to 0 and 255.
	tab, _ = ToneTable(&Adjust{Levels: &Levels{Black: 50, White: 200, Gamma: 100}})
	if tab[50] != 0 || tab[200] != 255 || tab[20] != 0 || tab[220] != 255 || tab[125] != 128 {
		t.Errorf("levels 50..200: 50->%d 200->%d 125->%d", tab[50], tab[200], tab[125])
	}

	// A gamma above 100 brightens the middle.
	tab, _ = ToneTable(&Adjust{Levels: &Levels{Black: 0, White: 255, Gamma: 200}})
	if tab[64] <= 64 || tab[0] != 0 || tab[255] != 255 {
		t.Errorf("gamma 2.0: 64->%d", tab[64])
	}

	// A curve is straight between its points and has (0,0) and (255,255) as the ends.
	tab, _ = ToneTable(&Adjust{Curve: []CurvePoint{{X: 100, Y: 50}}})
	if tab[0] != 0 || tab[50] != 25 || tab[100] != 50 || tab[255] != 255 || tab[178] != 153 {
		t.Errorf("one point curve: 50->%d 100->%d 178->%d", tab[50], tab[100], tab[178])
	}
}

// TestAdjustToneTableProperties is the property test of the tables: a rising curve gives a monotonic table,
// every entry is a byte, and the same adjustment gives the same table.
func TestAdjustToneTableProperties(t *testing.T) {
	seed := uint32(1)
	next := func(n int) int {
		seed = seed*1664525 + 1013904223

		return int(seed>>8) % n
	}

	for range 300 {
		var curve []CurvePoint

		x, y := 0, 0
		for range 1 + next(8) {
			x += 1 + next(30)
			y += next(40)

			if x > 255 || y > 255 {
				break
			}

			curve = append(curve, CurvePoint{X: x, Y: y})
		}

		a := &Adjust{Curve: curve, Exposure: next(601) - 300}
		if next(2) == 0 {
			b := next(200)
			a.Levels = &Levels{Black: b, White: b + 1 + next(255-b), Gamma: 10 + next(990)}
		}

		if v := violations(func(add func(string, string, ...any)) { validateAdjust(a, "a", add) }); v != nil {
			t.Fatalf("the generated adjustment is invalid: %v", v)
		}

		t1, ok := ToneTable(a)
		if !ok {
			continue
		}

		t2, _ := ToneTable(a)
		if t1 != t2 {
			t.Fatal("the table is not deterministic")
		}

		for v := 1; v < 256; v++ {
			if t1[v] < t1[v-1] {
				t.Fatalf("the table of %+v falls at %d: %d then %d", a, v, t1[v-1], t1[v])
			}
		}
	}
}

func TestAdjustCommandStoresOnlyWhatIsNotNeutral(t *testing.T) {
	d := baseDoc()
	res := mustApply(t, d, op("layer.adjust", `{"layer_id": "`+lid('b')+`", "brightness": 5, "levels": {"black": 0}}`))

	l, _ := Find(res.Doc, lid('b'))
	if l.Adjust == nil || l.Adjust.Brightness != 5 || l.Adjust.Levels != nil {
		t.Fatalf("the stored adjustment is %+v", l.Adjust)
	}

	res = mustApply(t, res.Doc, op("layer.adjust", `{"layer_id": "`+lid('b')+`", "brightness": 0}`))

	l, _ = Find(res.Doc, lid('b'))
	if l.Adjust != nil {
		t.Fatalf("a neutral adjustment is stored: %+v", l.Adjust)
	}
}

func TestFilterOrderIsTheOrderOfTheList(t *testing.T) {
	d := withFilters()
	res := mustApply(t, d,
		op("layer.filter_move", `{"layer_id": "`+lid('b')+`", "kind": "vignette", "index": 0}`),
		op("layer.filter_add", `{"layer_id": "`+lid('b')+`", "kind": "grain", "index": 1}`),
	)

	l, _ := Find(res.Doc, lid('b'))

	var kinds []string
	for _, f := range l.Filters {
		kinds = append(kinds, f.Kind)
	}

	if want := []string{"vignette", "grain", "blur", "shadow"}; !slices.Equal(kinds, want) {
		t.Fatalf("the filters are %v, want %v", kinds, want)
	}

	// The plan carries them in that order.
	fp := Plan(res.Doc, plan.Options{})

	var got []string

	for _, f := range planLayer(t, fp, lid('b')).Filters {
		got = append(got, f.Kind)
	}

	if want := []string{"vignette", "grain", "blur", "shadow"}; !slices.Equal(got, want) {
		t.Fatalf("the plan filters are %v, want %v", got, want)
	}
}

func TestFilterDefaultsAreValid(t *testing.T) {
	for _, k := range FilterKinds {
		f := NewFilter(k)

		if v := violations(func(add func(string, string, ...any)) { validateFilter(&f, "f", add) }); v != nil {
			t.Errorf("the default %s filter is invalid: %v", k, v)
		}
	}
}

func planLayer(t *testing.T, fp plan.FramePlan, id string) plan.Layer {
	t.Helper()

	for i := range fp.Layers {
		if fp.Layers[i].ItemID == id {
			return fp.Layers[i]
		}
	}

	t.Fatalf("the plan has no layer %s", id)

	return plan.Layer{}
}

func TestMaskFromALayerBecomesAClipAndItsSourceStaysInThePlan(t *testing.T) {
	d := withLayerMask()
	fp := Plan(d, plan.Options{})

	photo := planLayer(t, fp, lid('b'))
	if photo.Clip == nil || photo.Clip.Source != lid('d') || photo.Mask != nil {
		t.Fatalf("the photo has clip %+v mask %+v", photo.Clip, photo.Mask)
	}

	if src := planLayer(t, fp, lid('d')); src.Stencil {
		t.Fatal("a visible source is a stencil")
	}

	// A hidden source is added as a stencil with opacity 1.
	hid := mustApply(t, d, op("layer.set", `{"layer_id": "`+lid('d')+`", "visible": false, "opacity": 40}`))
	fp = Plan(hid.Doc, plan.Options{})

	src := planLayer(t, fp, lid('d'))
	if !src.Stencil || src.Opacity != 1 {
		t.Fatalf("the hidden source is %+v", src)
	}

	if planLayer(t, fp, lid('b')).Clip == nil {
		t.Fatal("the clip went with the hidden source")
	}

	// A source with no pixels (an image slot with no file) takes the layer it clips with it.
	empty := withLayerMask()
	empty.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('c')}
	empty.Layers[2].Kind = KindImage
	empty.Layers[2].Text, empty.Layers[2].Style, empty.Layers[2].Asset = "", nil, ""

	for _, l := range Plan(empty, plan.Options{}).Layers {
		if l.ItemID == lid('b') || l.ItemID == lid('c') {
			t.Fatalf("layer %s is in the plan though its mask source draws nothing", l.ItemID)
		}
	}
}

// A source whose opacity rounds to none through its group is not drawn, like a hidden one, and still gives
// its shape: the renderer drops a layer of no opacity, so it must be a stencil (SDD-021 section 5).
func TestMaskSourceInATransparentGroupIsAStencil(t *testing.T) {
	d := baseDoc()
	d.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('f')}

	res := mustApply(t, d, op("layer.set", `{"layer_id": "`+lid('e')+`", "opacity": 0}`))
	fp := Plan(res.Doc, plan.Options{})

	src := planLayer(t, fp, lid('f'))
	if !src.Stencil || src.Opacity != 1 {
		t.Fatalf("the source in a group of opacity 0 is %+v", src)
	}

	if planLayer(t, fp, lid('b')).Clip == nil {
		t.Fatal("the clip went with the source")
	}

	// A group of 1 per cent and a source of 1 per cent: 0.01 per cent, which the plan rounds to 0.
	nested := baseDoc()
	nested.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('f')}
	nested.Layers[4].Opacity = 1
	nested.Layers[4].Children[0].Opacity = 1

	if src := planLayer(t, Plan(nested, plan.Options{}), lid('f')); !src.Stencil {
		t.Fatalf("a source of 0.01 per cent is %+v", src)
	}

	// Two groups of 4 per cent give 0.16 per cent, which the renderer still draws.
	faint := baseDoc()
	faint.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('f')}
	faint.Layers[4].Opacity = 4
	faint.Layers[4].Children[0].Opacity = 4

	if src := planLayer(t, Plan(faint, plan.Options{}), lid('f')); src.Stencil || src.Opacity != 0.002 {
		t.Fatalf("a source of 0.16 per cent is %+v", src)
	}

	// A source that is drawn keeps its opacity.
	res = mustApply(t, d, op("layer.set", `{"layer_id": "`+lid('e')+`", "opacity": 50}`))
	if src := planLayer(t, Plan(res.Doc, plan.Options{}), lid('f')); src.Stencil || src.Opacity != 0.5 {
		t.Fatalf("the drawn source is %+v", src)
	}
}

func TestMaskSourceRemovedClearsTheMaskAndUndoRestoresIt(t *testing.T) {
	d := withLayerMask()
	res := mustApply(t, d, op("layer.remove", `{"layer_id": "`+lid('d')+`"}`))

	photo, _ := Find(res.Doc, lid('b'))
	if photo.Mask != nil {
		t.Fatalf("the mask of the photo is %+v after its source was removed", photo.Mask)
	}

	if v := Validate(res.Doc); len(v) > 0 {
		t.Fatalf("the result is invalid: %v", v)
	}

	undone, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
	if len(skipped) > 0 {
		t.Fatalf("undo skipped %v", skipped)
	}

	photo, _ = Find(undone.Doc, lid('b'))
	if photo.Mask == nil || photo.Mask.Source != lid('d') {
		t.Fatal("undo did not bring the mask back")
	}
}

func TestMaskShapeInThePlan(t *testing.T) {
	res := mustApply(t, baseDoc(), op(
		"layer.mask_set",
		`{"layer_id": "`+lid(
			'b',
		)+`", "kind": "rounded_rect", "x": 100, "w": 500, "radius": 30, "feather": 5, "invert": true}`,
	))

	m := planLayer(t, Plan(res.Doc, plan.Options{}), lid('b')).Mask
	want := plan.MaskSpec{
		Shape:   "rounded_rect",
		X:       0.1,
		Y:       0,
		Width:   0.5,
		Height:  1,
		Feather: 5,
		Invert:  true,
		Radius:  30,
	}

	if m == nil || *m != want {
		t.Fatalf("the plan mask is %+v, want %+v", m, want)
	}
}

func TestBlendEveryModeIsAccepted(t *testing.T) {
	for _, m := range BlendModes {
		res := mustApply(t, baseDoc(), op("layer.blend", `{"layer_id": "`+lid('d')+`", "mode": "`+m+`"}`))
		l, _ := Find(res.Doc, lid('d'))

		want := m
		if m == "normal" {
			want = ""
		}

		if l.Blend != want {
			t.Errorf("mode %s is stored as %q", m, l.Blend)
		}

		if got := planLayer(t, Plan(res.Doc, plan.Options{}), lid('d')).Blend; got != want {
			t.Errorf("mode %s is %q in the plan", m, got)
		}
	}
}

func TestAdjustAndFiltersInThePlan(t *testing.T) {
	d := baseDoc()
	res := mustApply(
		t,
		d,
		op("layer.adjust", `{"layer_id": "`+lid('b')+`", "brightness": 10, "contrast": 20, "exposure": 100}`),
		op("layer.filter_add", `{"layer_id": "`+lid('b')+`", "kind": "sepia", "amount": 50}`),
		op(
			"layer.filter_add",
			`{"layer_id": "`+lid('b')+`", "kind": "glow", "radius": 9, "amount": 150, "color": "#ffaa00"}`,
		),
		op("layer.filter_add", `{"layer_id": "`+lid('b')+`", "kind": "sharpen"}`),
	)

	l := planLayer(t, Plan(res.Doc, plan.Options{}), lid('b'))

	if l.ColorMatrix == nil || l.Tone == nil || l.Tone[10] != 20 {
		t.Fatalf("matrix %v tone %v", l.ColorMatrix, l.Tone)
	}

	want, err := json.Marshal([]plan.FilterSpec{
		{Kind: "matrix", Matrix: plan.FilterMatrix("sepia", 0.5)},
		{Kind: "outer_glow", Color: "#FFAA00", Radius: 9, Amount: 1.5},
		{Kind: "sharpen", Amount: 1, Radius: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := json.Marshal(l.Filters)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("filters\n%s\nwant\n%s", got, want)
	}

	// A layer with no look has no field of it, so the plan did not change for an old document.
	raw, err := json.Marshal(planLayer(t, Plan(baseDoc(), plan.Options{}), lid('d')))
	if err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"tone", "filters", "clip", "stencil", "mask", "blend", "color_matrix"} {
		if strings.Contains(string(raw), `"`+k+`"`) {
			t.Errorf("a layer without a look has %s: %s", k, raw)
		}
	}
}

func TestDescribeNamesTheLook(t *testing.T) {
	res := mustApply(t, baseDoc(),
		op("layer.adjust", `{"layer_id": "`+lid('b')+`", "brightness": 10}`),
		op("layer.filter_add", `{"layer_id": "`+lid('b')+`", "kind": "blur"}`),
		op("layer.mask_set", `{"layer_id": "`+lid('b')+`", "kind": "layer", "source": "`+lid('d')+`", "invert": true}`),
		op("layer.blend", `{"layer_id": "`+lid('b')+`", "mode": "multiply"}`),
	)

	text := Describe(res.Doc, 2, nil)
	if !strings.Contains(text, " blend multiply adjust brightness=10 filters blur mask layer "+lid('d')+" inverted") {
		t.Fatalf("the text does not name the look:\n%s", text)
	}
}

func TestLookValidateRefusesAGroupWithALookAndAMaskCycle(t *testing.T) {
	d := baseDoc()
	d.Layers[4].Blend = "multiply"

	if v := Validate(d); len(v) == 0 {
		t.Fatal("a group with a blend mode is valid")
	}

	d = baseDoc()
	d.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('d')}
	d.Layers[3].Mask = &Mask{Kind: MaskLayer, Source: lid('b')}

	if v := Validate(d); len(v) == 0 {
		t.Fatal("two layers masked by each other are valid")
	}
}

func TestLookCommandsAreDeterministic(t *testing.T) {
	ops := []command.Operation{
		op("layer.adjust", `{"layer_id": "`+lid('b')+`", "curve": [{"x": 10, "y": 20}], "exposure": -40}`),
		op("layer.filter_add", `{"layer_id": "`+lid('b')+`", "kind": "grain"}`),
		op("layer.mask_set", `{"layer_id": "`+lid('b')+`", "kind": "ellipse"}`),
	}

	a, err := ApplyBatch(baseDoc(), ops, testEnv())
	if err != nil {
		t.Fatal(err)
	}

	b, _ := ApplyBatch(baseDoc(), ops, testEnv())

	if !bytes.Equal(Canonical(a.Doc), Canonical(b.Doc)) {
		t.Fatal("the same batch gave two documents")
	}
}

// The error of a refused batch is part of what is the same every time (SDD-021 section 4): a mask of kind layer
// with two shape members names the first of them, in the order of the arguments.
func TestMaskSetErrorIsDeterministic(t *testing.T) {
	bad := op(
		"layer.mask_set",
		`{"layer_id": "`+lid('b')+`", "kind": "layer", "source": "`+lid('d')+`", "feather": 4, "radius": 8, "x": 1}`,
	)

	for range 50 {
		_, err := ApplyBatch(baseDoc(), []command.Operation{bad}, testEnv())

		var ce *command.Error
		if !errors.As(err, &ce) || ce.Path != "args.x" {
			t.Fatalf("the error is %v, not the one of args.x", err)
		}
	}
}
