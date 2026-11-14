package timeline

import (
	"bytes"
	"testing"
)

func TestCloneIsDeepAndEqual(t *testing.T) {
	for _, f := range []string{"unknown-fields", "all-item-types", "template"} {
		doc := mustDecode(t, readFile(t, "testdata/valid/"+f+".json"))
		before := Canonical(doc)
		c := Clone(doc)
		if !bytes.Equal(Canonical(c), before) {
			t.Fatalf("%s: clone differs", f)
		}
		c.Name = "changed"
		if top, ok := c.Extra["x_top"].(map[string]any); ok {
			top["a"] = "changed"
		}
		for k, it := range c.Items {
			it.Start += 1000
			if it.X != nil {
				*it.X = 12345
			}
			if len(it.Words) > 0 {
				it.Words[0].Text = "changed"
			}
			if it.Style != nil && it.Style.Size != nil {
				*it.Style.Size = 99
			}
			c.Items[k] = it
		}
		for k, tr := range c.Tracks {
			if tr.CaptionStyle != nil {
				tr.CaptionStyle.Extra = Extra{"y": true}
			}
			c.Tracks[k] = tr
		}
		if !bytes.Equal(Canonical(doc), before) {
			t.Errorf("%s: mutating the clone changed the original", f)
		}
	}
	if Clone(nil) != nil {
		t.Error("Clone(nil)")
	}
}

func TestRawIsCanonicalTree(t *testing.T) {
	doc := mustDecode(t, readFile(t, "testdata/valid/launch-teaser.json"))
	raw := Raw(doc)
	if _, ok := raw["items"].(map[string]any); !ok || raw["name"] != "Launch teaser" {
		t.Errorf("%v", raw)
	}
	back, err := FromRaw(raw)
	if err != nil || !bytes.Equal(Canonical(back), Canonical(doc)) {
		t.Errorf("round trip: %v", err)
	}
}

func TestCloneItemIsDeep(t *testing.T) {
	doc := mustDecode(t, readFile(t, "testdata/valid/all-item-types.json"))
	it := doc.Items["itm_p000000001"]
	c := CloneItem(it)
	c.Keyframes["scale"][0].Value = 99
	*c.X = 1
	c.Crop.Left = 0.9
	if it.Keyframes["scale"][0].Value == 99 || *it.X == 1 || it.Crop.Left == 0.9 {
		t.Error("CloneItem shares memory with the original")
	}
}

func TestAssetDuration(t *testing.T) {
	fps := Rational{Num: 30, Den: 1}
	video := Asset{Kind: AssetVideo, FrameRate: Rational{Num: 24, Den: 1}, FrameCount: 240}
	audio := Asset{Kind: AssetAudio, SampleCount: 96000}
	for _, c := range []struct {
		a    Asset
		fps  Rational
		want int
		ok   bool
	}{
		{video, fps, 300, true},
		{audio, fps, 60, true},
		{Asset{Kind: AssetImage}, fps, 0, false},
		{video, Rational{}, 0, false},
		{Asset{Kind: AssetVideo}, fps, 0, false},
		{Asset{Kind: AssetAudio}, fps, 0, false},
	} {
		if got, ok := AssetDuration(c.a, c.fps); got != c.want || ok != c.ok {
			t.Errorf("%+v: %d %v", c.a, got, ok)
		}
	}
}
