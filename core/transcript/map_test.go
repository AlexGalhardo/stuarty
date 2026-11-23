package transcript

import (
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	assetA = "ast_3c5v8n2w6d"
	trackV = "trk_f6g7h8j9k0"
	trackA = "trk_a1b2c3d4e5"
)

func docWith(items ...timeline.Item) *timeline.Document {
	doc := &timeline.Document{Items: map[string]timeline.Item{}}
	for i := range items {
		doc.Items[items[i].ID] = items[i]
	}
	return doc
}

func clip(id string, start, duration, sourceIn int) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemClip, trackV, start, duration)
	it.Asset, it.SourceIn = assetA, sourceIn
	return it
}

func ranges(v View) [][]Range {
	out := make([][]Range, len(v.Words))
	for i, w := range v.Words {
		out[i] = w.Ranges
	}
	return out
}

// The words of the worked example of SDD-002 C.41: a clip sped up to 1.5x.
func TestMapSpeedChangeGolden(t *testing.T) {
	it := clip("itm_m1n2p3q4r5", 90, 80, 60)
	it.Speed = timeline.Rational{Num: 3, Den: 2}
	words := []Word{
		{"Hello", 62, 70},
		{"everyone,", 72, 86},
		{"welcome", 90, 100},
		{"to", 100, 104},
		{"the", 104, 108},
		{"channel", 110, 128},
		{"today", 170, 182},
	}
	got := ranges(Map(docWith(it), assetA, words))
	r := func(s, e int) []Range { return []Range{{ItemID: it.ID, TrackID: trackV, Start: s, End: e}} }
	want := [][]Range{r(92, 97), r(98, 108), r(110, 117), r(117, 120), r(120, 122), r(124, 136), r(164, 170)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges\n got %v\nwant %v", got, want)
	}
}

func TestMapWordsOutsideTheVisibleRangeHaveNoRange(t *testing.T) {
	it := clip("itm_m1n2p3q4r5", 30, 100, 60) // shows [60,160)
	words := []Word{{"before", 40, 55}, {"in", 60, 80}, {"straddles", 150, 170}, {"after", 160, 190}}
	got := ranges(Map(docWith(it), assetA, words))
	want := [][]Range{
		{},
		{{ItemID: it.ID, TrackID: trackV, Start: 30, End: 50}},
		{{ItemID: it.ID, TrackID: trackV, Start: 120, End: 130}},
		{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges\n got %v\nwant %v", got, want)
	}
}

// A clip already cut into two pieces: the words in the cut have no range, the others keep their place.
func TestMapAnAlreadyCutClip(t *testing.T) {
	left := clip("itm_a111111111", 30, 12, 60)   // shows [60,72)
	right := clip("itm_b222222222", 42, 280, 80) // shows [80,360)
	words := []Word{{"hello", 62, 70}, {"um", 72, 80}, {"world", 90, 110}}
	v := Map(docWith(left, right), assetA, words)
	if len(v.Words[1].Ranges) != 0 {
		t.Fatalf("the cut word has ranges: %v", v.Words[1].Ranges)
	}
	if got := v.Words[2].Ranges; !reflect.DeepEqual(
		got,
		[]Range{{ItemID: right.ID, TrackID: trackV, Start: 52, End: 72}},
	) {
		t.Fatalf("world: %v", got)
	}
}

// The video clip and its linked audio show the same words on two tracks; a clip placed twice shows
// them in two places.
func TestMapSeveralItemsOfOneAsset(t *testing.T) {
	video := clip("itm_a111111111", 0, 100, 0)
	sound := timeline.NewItem("itm_b222222222", timeline.ItemAudio, trackA, 0, 100)
	sound.Asset = assetA
	again := clip("itm_c333333333", 200, 50, 20) // shows [20,70)
	other := clip("itm_d444444444", 300, 50, 0)
	other.Asset = "ast_zzzzzzzzzz"
	v := Map(docWith(video, sound, again, other), assetA, []Word{{"a", 30, 40}})
	want := []Range{
		{ItemID: "itm_a111111111", TrackID: trackV, Start: 30, End: 40},
		{ItemID: "itm_b222222222", TrackID: trackA, Start: 30, End: 40},
		{ItemID: "itm_c333333333", TrackID: trackV, Start: 210, End: 220},
	}
	if !reflect.DeepEqual(v.Words[0].Ranges, want) {
		t.Fatalf("ranges\n got %v\nwant %v", v.Words[0].Ranges, want)
	}
}

func TestMapSkipsItemsThatCannotFollowTheSource(t *testing.T) {
	curve := clip("itm_a111111111", 0, 100, 0)
	curve.SpeedCurve = []timeline.SpeedPoint{
		{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}},
		{T: 1000, Speed: timeline.Rational{Num: 2, Den: 1}},
	}
	back := clip("itm_b222222222", 100, 100, 0)
	back.Reverse = true
	freeze := clip("itm_c333333333", 200, 30, 5)
	freeze.Freeze = true
	v := Map(docWith(curve, back, freeze), assetA, []Word{{"a", 30, 40}})
	if len(v.Words[0].Ranges) != 0 {
		t.Fatalf("ranges: %v", v.Words[0].Ranges)
	}
	if want := []string{"itm_a111111111", "itm_b222222222"}; !reflect.DeepEqual(v.Skipped, want) {
		t.Fatalf("skipped %v, want %v", v.Skipped, want)
	}
}

func TestMapEmptyTranscriptAndNoItems(t *testing.T) {
	v := Map(docWith(), assetA, nil)
	if len(v.Words) != 0 || v.Skipped == nil || len(v.Skipped) != 0 {
		t.Fatalf("view: %+v", v)
	}
}
