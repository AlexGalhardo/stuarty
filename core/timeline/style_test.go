package timeline

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strconv"
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
	if w, ok := FontWeights(doc, "Inter"); !ok || len(w) != 9 {
		t.Errorf("%v %v", w, ok)
	}
	if _, ok := FontWeights(doc, "Comic"); ok {
		t.Error("Comic exists")
	}
}

// FONTS.md is the source of the shipped weights; BundledWeights must say the same.
func TestBundledWeightsMatchFontsMD(t *testing.T) {
	b, err := os.ReadFile("../../renderer/fonts/FONTS.md")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\| ([A-Za-z ]+?)(?: \(default\))? \| [^|]+ \| (\d+)(?: to (\d+))?`)
	rows := re.FindAllStringSubmatch(string(b), -1)
	if len(rows) != len(BundledWeights) {
		t.Fatalf("FONTS.md has %d families, BundledWeights %d", len(rows), len(BundledWeights))
	}
	for _, r := range rows {
		lo, _ := strconv.Atoi(r[2])
		hi := lo
		if r[3] != "" {
			hi, _ = strconv.Atoi(r[3])
		}
		var want []int
		for w := lo; w <= hi; w += 100 {
			want = append(want, w)
		}
		if !slices.Equal(BundledWeights[r[1]], want) {
			t.Errorf("%s: FONTS.md says %v, BundledWeights %v", r[1], want, BundledWeights[r[1]])
		}
	}
}

func TestDefaultFontWeightFallsToTheNearestAvailable(t *testing.T) {
	doc := &Document{Fonts: map[string]ProjectFont{
		"Regular":  {Files: []FontFile{{Weight: 400}}},
		"Light":    {Files: []FontFile{{Weight: 300}, {Weight: 500}}},
		"HasBold":  {Files: []FontFile{{Weight: 400}, {Weight: 700}}},
		"Light300": {Files: []FontFile{{Weight: 200}, {Weight: 300}}},
	}}

	for family, want := range map[string]int{
		"Inter": 700, "Anton": 400, "Regular": 400, "Light": 500, "HasBold": 700, "Light300": 300, "Unknown": 700,
	} {
		if got := DefaultFontWeight(doc, family); got != want {
			t.Errorf("%s: %d, want %d", family, got, want)
		}
	}
}
