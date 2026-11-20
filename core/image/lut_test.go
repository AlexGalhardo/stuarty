package image

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// The lookup table of a layer (epic 49, SDD-021 section 3.1). The golden cases of layer.adjust hold the bytes of
// each result and each refusal; these tests hold the rules.

func adjustLUT(t *testing.T, d *Document, id, args string) (*Document, *command.Error) {
	t.Helper()

	res, err := ApplyBatch(d, []command.Operation{{Op: "layer.adjust", Args: json.RawMessage(
		`{"layer_id": "` + id + `", ` + args + `}`)}}, testEnv())
	if err != nil {
		return nil, command.AsError(err)
	}

	return res.Doc, nil
}

func lutOf(t *testing.T, d *Document, id string) *LUTRef {
	t.Helper()

	l, ok := Find(d, id)
	if !ok {
		t.Fatalf("no layer %s", id)
	}

	if l.Adjust == nil {
		return nil
	}

	return l.Adjust.LUT
}

func TestAdjustLUTStoredForm(t *testing.T) {
	photo := lid('b')

	d, err := adjustLUT(t, baseDoc(), photo, `"lut": {"asset": "`+hashCube+`"}`)
	if err != nil {
		t.Fatal(err)
	}

	if got := lutOf(t, d, photo); got == nil || *got != (LUTRef{Asset: hashCube, Intensity: 100}) {
		t.Fatalf("the default intensity is 100: got %+v", got)
	}

	if a := d.Assets[hashCube]; a.Kind != AssetLUT || a.Name != "warm.cube" || a.Width != 0 || a.Height != 0 {
		t.Errorf("assets lists the table with its kind and no size: %+v", a)
	}

	if vs := Validate(d); len(vs) > 0 {
		t.Fatalf("the result is not valid: %v", vs)
	}

	// an intensity of 0 leaves no key, no adjust and no entry of assets
	d, err = adjustLUT(t, d, photo, `"lut": {"intensity": 0}`)
	if err != nil {
		t.Fatal(err)
	}

	l, _ := Find(d, photo)
	if l.Adjust != nil {
		t.Errorf("an adjust with no value is not stored: %+v", l.Adjust)
	}

	if _, ok := d.Assets[hashCube]; ok {
		t.Error("the table no layer uses is still in assets")
	}

	raw, merr := json.Marshal(l)
	if merr != nil {
		t.Fatal(merr)
	}

	if strings.Contains(string(raw), "lut") {
		t.Errorf("the stored layer has a lut key: %s", raw)
	}
}

func TestAdjustLUTChangesAndKeepsTheOtherKeys(t *testing.T) {
	photo := lid('b')

	d, err := adjustLUT(t, withLUT(), photo, `"lut": {"intensity": 30}`)
	if err != nil {
		t.Fatal(err)
	}

	if got := lutOf(t, d, photo); got == nil || *got != (LUTRef{Asset: hashCube, Intensity: 30}) {
		t.Fatalf("the intensity alone changes the strength: got %+v", got)
	}

	l, _ := Find(d, photo)
	if l.Adjust.Brightness != 10 {
		t.Errorf("the other adjustments stay: %+v", l.Adjust)
	}

	// another table replaces it, keeps the strength, and the first leaves assets
	d, err = adjustLUT(t, d, photo, `"lut": {"asset": "`+hashCube2+`"}`)
	if err != nil {
		t.Fatal(err)
	}

	if got := lutOf(t, d, photo); got == nil || *got != (LUTRef{Asset: hashCube2, Intensity: 30}) {
		t.Fatalf("got %+v", got)
	}

	if _, ok := d.Assets[hashCube]; ok || d.Assets[hashCube2].Kind != AssetLUT {
		t.Errorf("assets follows the table: %+v", d.Assets)
	}

	// a reset removes the adjustments and the entry
	d, err = adjustLUT(t, d, photo, `"reset": true`)
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Assets) != 1 {
		t.Errorf("only the photo is left in assets: %+v", d.Assets)
	}
}

// A document at the ceiling of files can swap the table of a layer for another one: the ceiling is judged on the
// files the document is left with, as layer.replace_source does, and a new table is still refused.
func TestAdjustLUTSwapAtTheCeilingOfFiles(t *testing.T) {
	photo := lid('b')
	d := fillAssets(withLUT())

	if vs := Validate(d); len(vs) > 0 {
		t.Fatalf("the fixture is not a valid document: %v", vs)
	}

	got, err := adjustLUT(t, d, photo, `"lut": {"asset": "`+hashCube2+`"}`)
	if err != nil {
		t.Fatalf("swapping the only table at %d files was refused: %v", MaxAssets, err)
	}

	if _, ok := got.Assets[hashCube]; ok || len(got.Assets) != MaxAssets {
		t.Errorf("the first table leaves assets and the count stays %d: %d", MaxAssets, len(got.Assets))
	}

	// a second layer with a table of its own is one file more
	_, err = adjustLUT(t, got, lid('a'), `"lut": {"asset": "`+hashCube+`"}`)
	if err == nil || err.Code != command.CodeLimit {
		t.Fatalf("a new table past the ceiling is E_LIMIT: %v", err)
	}
}

func TestAdjustLUTRefusals(t *testing.T) {
	photo, group, locked := lid('b'), lid('e'), lid('h')

	cases := []struct {
		name string
		id   string
		args string
		code string
		path string
	}{
		{
			"an image is not a table",
			photo,
			`"lut": {"asset": "` + hashFace + `"}`,
			command.CodeTypeMismatch,
			"args.lut.asset",
		},
		{"an unknown file", photo, `"lut": {"asset": "` + hashNone + `"}`, command.CodeNotFound, "args.lut.asset"},
		{"not a hash", photo, `"lut": {"asset": "../../etc/passwd"}`, command.CodeValidation, "args.lut.asset"},
		{
			"an intensity over 100",
			photo,
			`"lut": {"asset": "` + hashCube + `", "intensity": 101}`,
			command.CodeValidation,
			"args.lut.intensity",
		},
		{
			"a negative intensity",
			photo,
			`"lut": {"asset": "` + hashCube + `", "intensity": -1}`,
			command.CodeValidation,
			"args.lut.intensity",
		},
		{"an intensity with no table", photo, `"lut": {"intensity": 50}`, command.CodeValidation, "args.lut.asset"},
		{"an empty lut", photo, `"lut": {}`, command.CodeValidation, "args.lut"},
		{"a group", group, `"lut": {"asset": "` + hashCube + `"}`, command.CodeTypeMismatch, "args.layer_id"},
		{"a locked layer", locked, `"lut": {"asset": "` + hashCube + `"}`, command.CodeInvalidState, "args.layer_id"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := baseDoc()
			before := Canonical(d)

			_, err := adjustLUT(t, d, c.id, c.args)
			if err == nil {
				t.Fatal("the batch was accepted")
			}

			if err.Code != c.code || err.Path != c.path {
				t.Errorf("got %s at %q, want %s at %q: %s", err.Code, err.Path, c.code, c.path, err.Message)
			}

			if !bytes.Equal(before, Canonical(d)) {
				t.Error("a refused batch changed its input")
			}
		})
	}

	t.Run("an image layer cannot be added from a table", func(t *testing.T) {
		_, err := ApplyBatch(baseDoc(), []command.Operation{{Op: "layer.add_image", Args: json.RawMessage(
			`{"asset": "` + hashCube + `"}`)}}, testEnv())
		if e := command.AsError(err); e == nil || e.Code != command.CodeNotFound {
			t.Fatalf("a table is not a ready image: %v", err)
		}
	})
}

func TestAdjustLUTValidateCatchesAStoredDocument(t *testing.T) {
	table := AssetRef{Hash: hashCube, Name: "warm.cube", Kind: AssetLUT}

	cases := []struct {
		name   string
		edit   func(d *Document)
		reason string
	}{
		{"a table with no entry", func(d *Document) {
			d.Layers[1].Adjust = &Adjust{LUT: &LUTRef{Asset: hashCube, Intensity: 100}}
		}, "not in assets"},
		{"a table that is an image", func(d *Document) {
			d.Layers[1].Adjust = &Adjust{LUT: &LUTRef{Asset: hashFace, Intensity: 100}}
		}, "is an image"},
		{"an entry nobody uses", func(d *Document) { d.Assets[hashCube] = table }, "no layer uses"},
		{"an image layer on a table", func(d *Document) {
			d.Layers[1].Asset = hashCube
			d.Assets[hashCube] = table
		}, "is a lookup table"},
		{"a table with a size", func(d *Document) {
			d.Layers[1].Adjust = &Adjust{LUT: &LUTRef{Asset: hashCube, Intensity: 100}}
			d.Assets[hashCube] = AssetRef{Hash: hashCube, Name: "x", Kind: AssetLUT, Width: 4, Height: 4}
		}, "is not valid"},
		{"a kind that is not known", func(d *Document) {
			d.Assets[hashFace] = AssetRef{Hash: hashFace, Name: "x", Kind: "video", Width: 4, Height: 4}
		}, "is not valid"},
		{"an intensity of 0", func(d *Document) {
			d.Layers[1].Adjust = &Adjust{LUT: &LUTRef{Asset: hashCube, Intensity: 0}}
			d.Assets[hashCube] = table
		}, "lut.intensity"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := baseDoc()
			c.edit(d)

			vs := Validate(d)

			found := false

			for _, v := range vs {
				found = found || strings.Contains(v.Message, c.reason)
			}

			if !found {
				t.Errorf("no violation mentions %q: %v", c.reason, vs)
			}
		})
	}
}

func TestAdjustLUTCloneAndPlan(t *testing.T) {
	d := withLUT()

	l, _ := Find(d, lid('b'))
	cp := cloneLayer(l)

	cp.Adjust.LUT.Intensity = 5

	if l.Adjust.LUT.Intensity != 80 {
		t.Error("the copy of a layer shares its lookup table with the original")
	}

	fp := planOf(d)

	var got *struct {
		asset string
		mix   float64
	}

	for i := range fp.Layers {
		if fp.Layers[i].ItemID == lid('b') && fp.Layers[i].Lut != nil {
			got = &struct {
				asset string
				mix   float64
			}{fp.Layers[i].Lut.Asset, fp.Layers[i].Lut.Intensity}
		}
	}

	if got == nil || got.asset != strings.TrimPrefix(hashCube, "sha256:") || got.mix != 0.8 {
		t.Fatalf("the plan layer carries the table as a fraction: %+v", got)
	}

	for i := range fp.Layers {
		if fp.Layers[i].ItemID != lid('b') && fp.Layers[i].Lut != nil {
			t.Errorf("layer %s has a table it was not given", fp.Layers[i].ItemID)
		}
	}
}
