package export

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// windowDocument is a document of 18 layers scattered over a 640 by 480 canvas, as the reference document is: a
// photo, rounded boxes and ellipses, turned, with looks that grow a layer (shadow, glow), a mask of their own,
// and blend modes that read the picture below them.
func windowDocument(t *testing.T) *coreimage.Document {
	t.Helper()

	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_aaaaaaaaaa", Name: "window", Width: 640, Height: 480,
		Background: "#101820",
		Assets:     map[string]coreimage.AssetRef{testHash: {Hash: testHash, Name: "src.png", Width: 64, Height: 48}},
	}

	id := func(i int) string { return fmt.Sprintf("lyr_%010d", i) }

	for i := range 16 {
		l := coreimage.Layer{
			ID: id(i), Name: fmt.Sprintf("Layer %d", i), Visible: true, Opacity: 100, X: (i * 53) % 400,
			Y: (i * 71) % 300, W: 160 + (i%4)*20, H: 120 + (i%3)*20, Rotation: (i % 5) * 7,
		}

		switch i % 3 {
		case 0:
			l.Kind, l.Asset = coreimage.KindImage, testHash
		case 1:
			l.Kind, l.Shape, l.Fill, l.Radius = coreimage.KindShape, coreimage.ShapeRoundedRect,
				fmt.Sprintf("#%02X%02X%02X", i*12, 255-i*9, 90), 20
		default:
			l.Kind, l.Shape, l.Fill = coreimage.KindShape, coreimage.ShapeEllipse,
				fmt.Sprintf("#%02X%02X%02X", 255-i*6, i*10, 160)
		}

		d.Layers = append(d.Layers, l)
	}

	// Two text layers, which the renderer holds as bitmaps with an offset of the centre (a shadow moves it).
	for k, x := range []int{40, 300} {
		d.Layers = append(d.Layers, coreimage.Layer{
			ID: id(16 + k), Kind: coreimage.KindText, Name: fmt.Sprintf("Text %d", k), Visible: true, Opacity: 100,
			X: x, Y: 60 + k*150, W: 260, H: 90, Rotation: 4 * k, Text: "WINDOW EDIT",
		})
	}

	coreimage.ReorderAll(d)

	op := func(name, args string) command.Operation {
		return command.Operation{Op: name, Args: []byte(args)}
	}

	var ops []command.Operation

	for i := 1; i < 16; i += 3 {
		ops = append(
			ops,
			op(
				"layer.filter_add",
				fmt.Sprintf(`{"layer_id": %q, "kind": "shadow", "dx": 6, "dy": 6, "blur": 8}`, id(i)),
			),
			op("layer.blend", fmt.Sprintf(`{"layer_id": %q, "mode": "soft_light"}`, id(i))),
		)
	}

	for i := 2; i < 16; i += 3 {
		ops = append(
			ops,
			op("layer.filter_add", fmt.Sprintf(`{"layer_id": %q, "kind": "glow", "radius": 10, "amount": 120}`, id(i))),
			op(
				"layer.mask_set",
				fmt.Sprintf(`{"layer_id": %q, "kind": "rounded_rect", "radius": 30, "feather": 4}`, id(i)),
			),
		)
	}

	for k := 16; k < 18; k++ {
		ops = append(
			ops,
			op(
				"layer.filter_add",
				fmt.Sprintf(`{"layer_id": %q, "kind": "shadow", "dx": 9, "dy": 7, "blur": 4}`, id(k)),
			),
		)
	}

	for i := 0; i < 16; i += 3 {
		ops = append(ops, op("layer.adjust", fmt.Sprintf(`{"layer_id": %q, "brightness": 8, "contrast": 12}`, id(i))))
	}

	return applyLook(t, d, ops...)
}

// TestImageWindowEqualsTheFullRender: after a first render, an edit of any layer (its opacity, place, turn, size,
// blend mode or a look) is drawn as a window over the picture before, and gives the bytes of the render of the
// whole document with no cache, whichever layer it is (SDD-021 section 6.3). Edits are chained, so each starts
// from the window picture of the one before.
func TestImageWindowEqualsTheFullRender(t *testing.T) {
	d := windowDocument(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := cachedConfig(t, nil, "")

	renderWith(t, d, cfg)

	edits := []func(id string, n int) command.Operation{
		func(id string, n int) command.Operation {
			return lookOp("layer.set", fmt.Sprintf(`{"layer_id": %q, "opacity": %d}`, id, 30+n%60))
		},
		func(id string, n int) command.Operation {
			return lookOp(
				"layer.transform",
				fmt.Sprintf(`{"layer_id": %q, "x": %d, "y": %d}`, id, 20+n*7%300, 10+n*11%250),
			)
		},
		func(id string, n int) command.Operation {
			return lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "rotation": %d}`, id, n*13%90))
		},
		func(id string, _ int) command.Operation {
			return lookOp("layer.blend", fmt.Sprintf(`{"layer_id": %q, "mode": "multiply"}`, id))
		},
		func(id string, n int) command.Operation {
			return lookOp("layer.filter_add", fmt.Sprintf(`{"layer_id": %q, "kind": "blur", "radius": %d}`, id, 2+n%5))
		},
	}

	n := 0

	for round := range edits {
		for _, id := range ids {
			n++
			d = applyLook(t, d, edits[round](id, n))

			got, want := renderWith(t, d, cfg), renderWith(t, d, plain)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("edit %d of %s (kind %d): the window render differs from the full render", n, id, round)
			}
		}
	}

	t.Logf("%d edits, %d drawn as a window", n, cache.Stats().Windows)

	if s := cache.Stats(); s.Windows < n/2 {
		t.Errorf("only %d of %d edits were drawn as a window: %+v", s.Windows, n, s)
	}
}

// TestImageWindowDrawsOnlyWhatTheEditReaches: editing a small layer leaves the pixels of the picture before
// outside it as they were, and the render takes the window path.
func TestImageWindowDrawsOnlyWhatTheEditReaches(t *testing.T) {
	d := windowDocument(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")

	before := renderWith(t, d, cfg)

	edited := withOpacity(t, d, ids[8], 20)
	after := renderWith(t, edited, cfg)

	if cache.Stats().Windows != 1 {
		t.Fatalf("the edit was not drawn as a window: %+v", cache.Stats())
	}

	if bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("the edit changed no pixel")
	}

	if !bytes.Equal(after.Pix, renderWith(t, edited, cachedConfig(t, nil, "")).Pix) {
		t.Fatal("the window render differs from the full render")
	}
}

// TestImageWindowIsNotUsedForAMaskTakenFromAnotherLayer: a document with a clip falls back to the full draw and
// still gives the pixels of it.
func TestImageWindowIsNotUsedForAMaskTakenFromAnotherLayer(t *testing.T) {
	d := lookAll(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")

	renderWith(t, d, cfg)

	edited := withOpacity(t, d, ids[1], 35)

	if got, want := renderWith(
		t,
		edited,
		cfg,
	), renderWith(
		t,
		edited,
		cachedConfig(t, nil, ""),
	); !bytes.Equal(
		got.Pix,
		want.Pix,
	) {
		t.Error("the render differs from the full render")
	}

	if s := cache.Stats(); s.Windows != 0 {
		t.Errorf("a document with a clip was drawn as a window: %+v", s)
	}
}

// TestImageWindowIsNotSharedAcrossUsers: the picture before is read only by the scope that drew it.
func TestImageWindowIsNotSharedAcrossUsers(t *testing.T) {
	d := windowDocument(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)

	renderWith(t, d, cachedConfig(t, cache, "u1"))
	renderWith(t, withOpacity(t, d, ids[3], 40), cachedConfig(t, cache, "u2"))

	if s := cache.Stats(); s.Windows != 0 {
		t.Errorf("another user's picture was the start of a window: %+v", s)
	}
}

// TestImageWindowDragKeepsTheInstancesAndTheBytes: a drag edits one layer again and again, with an edit of
// another layer and another shape of document in between; every picture is the full render of its document
// (the instances of the bands keep what they hold, SDD-021 section 6.3).
func TestImageWindowDragKeepsTheInstancesAndTheBytes(t *testing.T) {
	d := windowDocument(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := cachedConfig(t, nil, "")

	renderWith(t, d, cfg)

	for n := range 10 {
		id := ids[7]
		if n == 5 {
			id = ids[12]
		}

		d = withOpacity(t, d, id, 20+n*7)

		if got, want := renderWith(t, d, cfg), renderWith(t, d, plain); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("drag step %d differs from the full render", n)
		}
	}

	other := lookBase()
	otherIDs := layerIDs(other)

	renderWith(t, other, cfg)

	other = withOpacity(t, other, otherIDs[0], 40)
	if got, want := renderWith(t, other, cfg), renderWith(t, other, plain); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("another shape differs from the full render")
	}

	d = withOpacity(t, d, ids[7], 61)
	if got, want := renderWith(t, d, cfg), renderWith(t, d, plain); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("back to the first shape differs from the full render")
	}
}

// TestImageWindowAfterACancelledRenderGivesTheBytes: a render whose context ends while its bands draw returns at
// once, and the bands in flight keep the instances of the session until they are over; the next render of the
// same shape waits for them and gives the bytes of the full render (run with -race: no band of an abandoned render
// shares an instance with the next one).
func TestImageWindowAfterACancelledRenderGivesTheBytes(t *testing.T) {
	d := windowDocument(t)
	ids := layerIDs(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := cachedConfig(t, nil, "")

	renderWith(t, d, cfg)

	for n := range 12 {
		d = withOpacity(t, d, ids[n%len(ids)], 25+n*5)

		ctx, cancel := context.WithCancel(context.Background())
		stop := time.AfterFunc(time.Duration(n%4)*time.Millisecond, cancel)
		_, _ = RenderImage(ctx, d, cfg)

		stop.Stop()
		cancel()

		d = withOpacity(t, d, ids[(n+3)%len(ids)], 30+n*4)
		if got, want := renderWith(t, d, cfg), renderWith(t, d, plain); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("edit %d after a cancelled render differs from the full render", n)
		}
	}
}

// TestImageWindowAtExtremeScales: the band of a window receives only the part of a prepared picture that its
// rectangle can read (windowCrop), and the margin of that part holds when the layer is drawn much smaller than its
// picture (the 64 by 48 photo in 6 by 5 pixels) and much larger (in 620 by 470 pixels), turned or not, at a place
// that is not a whole pixel: every edit of a small layer over them gives the bytes of the full render.
func TestImageWindowAtExtremeScales(t *testing.T) {
	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_bbbbbbbbbb", Name: "scales", Width: 640, Height: 480,
		Background: "#203040",
		Assets:     map[string]coreimage.AssetRef{testHash: {Hash: testHash, Name: "src.png", Width: 64, Height: 48}},
	}

	id := func(i int) string { return fmt.Sprintf("lyr_%010d", i) }
	photo := func(i, x, y, w, h, rot int) coreimage.Layer {
		return coreimage.Layer{
			ID: id(i), Name: fmt.Sprintf("Photo %d", i), Kind: coreimage.KindImage, Asset: testHash, Visible: true,
			Opacity: 100, X: x, Y: y, W: w, H: h, Rotation: rot,
		}
	}

	d.Layers = append(d.Layers,
		photo(0, 10, 5, 620, 470, 0),
		photo(1, -40, -30, 720, 540, 33),
		photo(2, 200, 150, 6, 5, 0),
		photo(3, 300, 200, 7, 5, 41),
		photo(4, 100, 300, 3, 2, 0),
		coreimage.Layer{
			ID: id(5), Name: "Box", Kind: coreimage.KindShape, Shape: coreimage.ShapeRoundedRect, Fill: "#E0A040",
			Radius: 6, Visible: true, Opacity: 100, X: 180, Y: 140, W: 50, H: 40, Rotation: 12,
		},
	)
	coreimage.ReorderAll(d)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := cachedConfig(t, nil, "")

	renderWith(t, d, cfg)

	moves := [][2]int{{190, 135}, {295, 195}, {97, 295}, {301, 203}, {20, 20}, {560, 400}, {199, 151}}
	n := 0

	for _, mv := range moves {
		for _, l := range []string{id(5), id(2), id(3), id(4)} {
			n++
			d = applyLook(t, d, lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "x": %d, "y": %d}`, l, mv[0], mv[1])))
			d = withOpacity(t, d, id(n%2), 60+n%40)

			if got, want := renderWith(t, d, cfg), renderWith(t, d, plain); !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("edit %d of %s: the window render differs from the full render", n, l)
			}
		}
	}

	if s := cache.Stats(); s.Windows < n/2 {
		t.Errorf("only %d of %d edits were drawn as a window: %+v", s.Windows, n, s)
	}
}
