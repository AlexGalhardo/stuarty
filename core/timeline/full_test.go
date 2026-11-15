package timeline

import "testing"

func TestFullWritesTheDefaultsAndKeepsUnknownFields(t *testing.T) {
	t.Parallel()

	doc := &Document{
		SchemaVersion: CurrentSchemaVersion, ID: "prj_7k2m9x4q1b", Name: "x", Settings: DefaultSettings(),
		Assets: map[string]Asset{}, Tracks: map[string]Track{}, Items: map[string]Item{},
		Extra: Extra{"future": "kept"},
	}
	doc.Items["itm_a000000001"] = NewItem("itm_a000000001", ItemClip, "trk_t000000001", 0, 10)

	canon, full := Raw(doc), Full(doc)

	if _, has := canon["settings"].(map[string]any)["width"]; has {
		t.Fatal("the canonical form must omit a default")
	}

	if got := full["settings"].(map[string]any)["width"]; got == nil {
		t.Error("Full must write settings.width")
	}

	if full["future"] != "kept" {
		t.Error("Full must keep an unknown field")
	}

	item := full["items"].(map[string]any)["itm_a000000001"].(map[string]any)
	for _, k := range []string{"scale", "opacity", "filter", "filter_amount", "speed", "volume", "fit", "contrast", "saturation"} {
		if _, ok := item[k]; !ok {
			t.Errorf("the full item has no %s", k)
		}
	}

	one := FullValue(doc.Items["itm_a000000001"]).(map[string]any)
	if one["filter"] != "none" {
		t.Errorf("FullValue filter = %v", one["filter"])
	}
}
