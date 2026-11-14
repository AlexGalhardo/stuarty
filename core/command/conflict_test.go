package command

import "testing"

func TestConflicts(t *testing.T) {
	const other = "itm_k000000001"
	op := func(args string) Operation { return opJSON("x.y", args) }
	cases := []struct {
		name         string
		ops          []Operation
		intervening  []Change
		wantID, kind string
	}{
		{
			"no intervening change", []Operation{op(`{"item_id":"` + clipKey + `"}`)}, nil, "", "",
		},
		{
			"a different object does not conflict",
			[]Operation{op(`{"item_id":"` + clipKey + `"}`)},
			[]Change{{Kind: KindItemUpdated, ID: other}},
			"", "",
		},
		{
			"the same object conflicts",
			[]Operation{op(`{"item_id":"` + clipKey + `","start":"1s"}`)},
			[]Change{{Kind: KindItemUpdated, ID: clipKey}},
			clipKey, KindItemUpdated,
		},
		{
			"added, removed",
			[]Operation{op(`{"track_id":"` + capTrack + `"}`)},
			[]Change{{Kind: KindTrackRemoved, ID: capTrack}},
			capTrack, KindTrackRemoved,
		},
		{
			"asset reference nested in an object",
			[]Operation{op(`{"asset":{"id":"` + assetKey + `"}}`)},
			[]Change{{Kind: KindAssetAdded, ID: assetKey}},
			assetKey, KindAssetAdded,
		},
		{
			"ids in a list",
			[]Operation{op(`{"item_ids":["` + other + `","` + clipKey + `"]}`)},
			[]Change{{Kind: KindItemRemoved, ID: clipKey}},
			clipKey, KindItemRemoved,
		},
		{
			"the first object of the first operation wins",
			[]Operation{op(`{"a_item":"` + clipKey + `"}`), op(`{"z_item":"` + other + `"}`)},
			[]Change{{Kind: KindItemUpdated, ID: other}, {Kind: KindItemUpdated, ID: clipKey}},
			clipKey, KindItemUpdated,
		},
		{
			"the id has the wrong collection prefix",
			[]Operation{op(`{"item_id":"` + clipKey + `"}`)},
			[]Change{{Kind: KindTrackUpdated, ID: clipKey}},
			"", "",
		},
		{
			"settings, project and fonts are not referenced by id",
			[]Operation{op(`{"item_id":"` + clipKey + `","family":"Mine"}`)},
			[]Change{
				{Kind: KindSettingsUpdated, ID: "prj_7k2m9x4q1b"},
				{Kind: KindProjectUpdated, ID: "prj_7k2m9x4q1b"},
				{Kind: KindFontAdded, ID: "Mine"},
			},
			"",
			"",
		},
		{
			"strings that only look like ids",
			[]Operation{op(`{"name":"itm_short","other":"itm_m1n2p3q4r5x","n":5,"b":true,"z":null}`)},
			[]Change{{Kind: KindItemUpdated, ID: "itm_short"}, {Kind: KindItemUpdated, ID: "itm_m1n2p3q4r5x"}},
			"", "",
		},
		{
			"no args and invalid args",
			[]Operation{{Op: "x.y"}, {Op: "x.y", Args: []byte(`{`)}},
			[]Change{{Kind: KindItemUpdated, ID: clipKey}},
			"", "",
		},
	}
	for _, c := range cases {
		got := Conflicts(c.ops, c.intervening)
		switch {
		case c.wantID == "" && got != nil:
			t.Errorf("%s: unexpected conflict %+v", c.name, got)
		case c.wantID != "" && (got == nil || got.ID != c.wantID || got.Kind != c.kind):
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
	got := Conflicts(
		[]Operation{opJSON("x.y", `{"item_id":"`+clipKey+`"}`)},
		[]Change{{Kind: KindItemUpdated, ID: clipKey}},
	)
	if got.Collection != "items" {
		t.Errorf("%+v", got)
	}
}
