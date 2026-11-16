package timeline

import (
	"strings"
	"testing"
)

const lutID = "ast_2b3c4d5f6g"

func withLut(d *Document, l *Lut) {
	d.Assets[lutID] = Asset{
		ID:   lutID,
		Hash: "sha256:" + strings.Repeat("0", 63) + "1",
		Kind: AssetLUT,
		Name: "grade.cube",
	}
	editItem(d, clipID, func(i *Item) { i.Lut = l })
}

func TestValidateLut(t *testing.T) {
	cases := []invalidCase{
		{"lut asset missing", func(d *Document) {
			withLut(d, &Lut{Asset: "ast_zzzzzzzzzz", Intensity: 1})
		}, VAssetRef},
		{"lut asset is a video", func(d *Document) {
			withLut(d, &Lut{Asset: assetID, Intensity: 1})
		}, VAssetRef},
		{"lut asset id form", func(d *Document) { withLut(d, &Lut{Asset: "grade", Intensity: 1}) }, VID},
		{"lut intensity above 1", func(d *Document) { withLut(d, &Lut{Asset: lutID, Intensity: 1.5}) }, VShape},
		{"lut intensity below 0", func(d *Document) { withLut(d, &Lut{Asset: lutID, Intensity: -0.1}) }, VShape},
		{"lut on a text item", func(d *Document) {
			withLut(d, nil)
			editItem(d, capItemID, func(i *Item) { i.Lut = &Lut{Asset: lutID, Intensity: 1} })
		}, VShape},
		{"clip placed from a lut asset", func(d *Document) {
			withLut(d, nil)
			editItem(d, clipID, func(i *Item) { i.Asset = lutID })
		}, VAssetRef},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base(t)
			c.edit(d)
			if got := codesOf(Validate(d)); len(got) != 1 || got[0] != c.want {
				t.Errorf("codes %v, want only %s\n%v", got, c.want, Validate(d))
			}
		})
	}
}

func TestValidateAcceptsALut(t *testing.T) {
	d := base(t)
	withLut(d, &Lut{Asset: lutID, Intensity: 0.5})
	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("a clip with a lut asset reference is valid, got %v", vs)
	}
}
