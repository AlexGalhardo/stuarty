package timeline

import (
	"encoding/json"
	"testing"
)

func TestStyleMapRoundTrip(t *testing.T) {
	m := map[string]any{
		"size":      json.Number("72"),
		"color":     "#ffe600",
		"uppercase": false,
		"position":  "top",
		"x_new":     "kept",
	}
	s, err := StyleFromMap(m)
	if err != nil {
		t.Fatal(err)
	}
	if *s.Size != 72 || *s.Color != "#FFE600" || *s.Uppercase || *s.Position != "top" || s.Extra["x_new"] != "kept" {
		t.Errorf("%+v", s)
	}
	back := StyleToMap(s)
	if len(back) != 5 || back["color"] != "#FFE600" || back["uppercase"] != false {
		t.Errorf("%v", back)
	}
	if StyleToMap(nil) != nil {
		t.Error("nil style")
	}
	if got := StyleToMap(&CaptionStyle{}); len(got) != 0 || got == nil {
		t.Errorf("empty style: %v", got)
	}
	if _, err := StyleFromMap(map[string]any{"size": "big"}); err == nil {
		t.Error("a wrong type must fail")
	}
}

func TestStyleKeysAndFontWeights(t *testing.T) {
	text, caption := StyleKeys()
	if len(text) != 17 || len(caption) != 4 {
		t.Errorf("%d text keys, %d caption keys: %v %v", len(text), len(caption), text, caption)
	}
	doc := &Document{Fonts: map[string]ProjectFont{"Mine": {Files: []FontFile{{Weight: 700}, {Weight: 400}}}}}
	if w, ok := FontWeights(doc, "Mine"); !ok || len(w) != 2 || w[0] != 400 {
		t.Errorf("%v %v", w, ok)
	}
	if w, ok := FontWeights(doc, "Inter"); !ok || len(w) != 2 {
		t.Errorf("%v %v", w, ok)
	}
	if _, ok := FontWeights(doc, "Comic"); ok {
		t.Error("Comic exists")
	}
}
