package command

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func rawChanges(t *testing.T, before, after *timeline.Document) []Change {
	t.Helper()
	return diffOf(timeline.Raw(before), timeline.Raw(after))
}

func describe(t *testing.T, before, after *timeline.Document) string {
	t.Helper()
	return DescribeDiff(after, rawChanges(t, before, after))
}

func TestDescribeDiffEmpty(t *testing.T) {
	d := baseDoc(t)
	if got := DescribeDiff(d, nil); got != "no changes" {
		t.Error(got)
	}
	if got := describe(t, d, timeline.Clone(d)); got != "no changes" {
		t.Error(got)
	}
}

// Every one of the 12 change kinds of SDD-003 §3.4.
func TestDescribeDiffAllKinds(t *testing.T) {
	before := baseDoc(t)
	after := timeline.Clone(before)

	after.Name = "New"
	after.Settings.Width = 1920
	after.Settings.Height = 1080
	delete(after.Assets, assetID)
	after.Assets["ast_k000000001"] = timeline.Asset{
		ID:          "ast_k000000001",
		Hash:        before.Assets[assetID].Hash,
		Kind:        timeline.AssetAudio,
		Name:        "voice.wav",
		SampleCount: 96000,
	}
	after.Assets["ast_k000000002"] = timeline.Asset{
		ID:     "ast_k000000002",
		Hash:   before.Assets[assetID].Hash,
		Kind:   timeline.AssetImage,
		Name:   "logo",
		Width:  64,
		Height: 32,
	}
	delete(after.Tracks, capTrack)
	after.Tracks["trk_k000000001"] = timeline.Track{
		ID:    "trk_k000000001",
		Kind:  timeline.TrackVisual,
		Name:  `Over "lay"`,
		Order: "a0",
	}
	tr := after.Tracks[vidTrack]
	tr.Hidden, tr.Name = true, "Main"
	after.Tracks[vidTrack] = tr
	delete(after.Items, capItemID)
	clip := after.Items[clipID]
	clip.Start, clip.Duration = 30, 60
	after.Items[clipID] = clip
	text := timeline.NewItem("itm_k000000001", timeline.ItemText, "trk_k000000001", 100, 10)
	text.Text = "Hello <b> & more text"
	after.Items[text.ID] = text
	shape := timeline.NewItem("itm_k000000002", timeline.ItemShape, "trk_k000000001", 200, 30)
	shape.Shape, shape.Width, shape.Height = "rect", 10, 20
	after.Items[shape.ID] = shape
	after.Fonts = map[string]timeline.ProjectFont{
		"Mine": {Files: []timeline.FontFile{{Weight: 400, Hash: before.Assets[assetID].Hash}}},
	}

	want := strings.Join([]string{
		`~ project: name "Launch teaser" -> "New"`,
		`~ settings: height none -> 1080; width none -> 1920`,
		`- asset ast_3c5v8n2w6d "IMG_0421.MOV"`,
		`+ asset ast_k000000001 audio "voice.wav" 2s/60f`,
		`+ asset ast_k000000002 image "logo" 64x32`,
		`- track trk_a1b2c3d4e5 "Captions"`,
		`~ track trk_f6g7h8j9k0: hidden none -> true; name "Video" -> "Main"`,
		`+ track trk_k000000001 visual "Over \"lay\"" top`,
		`+ item itm_k000000001 text on trk_k000000001 3.333s/100f +0.333s/10f "Hello &lt;b> &amp; mo…"`,
		`+ item itm_k000000002 shape on trk_k000000001 6.667s/200f +1s/30f rect 10x20`,
		`~ item itm_m1n2p3q4r5: duration 5s/150f -> 2s/60f; start 0s/0f -> 1s/30f`,
		`- item itm_s6t7v8w9x0 caption was 0.4s/12f +1.6s/48f on trk_a1b2c3d4e5 4w "Meet the new e…"`,
		`+ font "Mine"`,
	}, "\n")
	if got := describe(t, before, after); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	// Removed lines and an updated font.
	back := describe(t, after, before)
	for _, line := range []string{
		`- track trk_k000000001 "Over \"lay\""`,
		`+ track trk_a1b2c3d4e5 visual "Captions" top`,
		`+ asset ast_3c5v8n2w6d video "IMG_0421.MOV" 30s/900f 1080x1920 audio`,
		`- asset ast_k000000001 "voice.wav"`,
		`+ item itm_s6t7v8w9x0 caption on trk_a1b2c3d4e5 0.4s/12f +1.6s/48f 4w "Meet the new e…"`,
		`- item itm_k000000001 text was 3.333s/100f +0.333s/10f on trk_k000000001 "Hello &lt;b> &amp; mo…"`,
		`- font "Mine"`,
	} {
		if !strings.Contains(back, line) {
			t.Errorf("missing %q in:\n%s", line, back)
		}
	}
	changed := timeline.Clone(after)
	changed.Fonts["Mine"] = timeline.ProjectFont{
		Files: []timeline.FontFile{{Weight: 700, Hash: before.Assets[assetID].Hash}},
	}
	if got := describe(t, after, changed); got != "- font \"Mine\"\n+ font \"Mine\"" {
		t.Errorf("font update: %q", got)
	}
}

func TestDescribeDiffItemDetails(t *testing.T) {
	before := baseDoc(t)
	after := timeline.Clone(before)
	clip := timeline.NewItem("itm_k000000001", timeline.ItemClip, vidTrack, 200, 30)
	clip.Asset, clip.SourceIn, clip.Fit = assetID, 45, "contain"
	clip.Speed = timeline.Rational{Num: 3, Den: 2}
	slow := timeline.NewItem("itm_k000000002", timeline.ItemClip, vidTrack, 300, 30)
	slow.Asset = assetID
	slow.Speed = timeline.Rational{Num: 1, Den: 3}
	plain := timeline.NewItem("itm_k000000003", timeline.ItemClip, vidTrack, 400, 30)
	plain.Asset = assetID
	after.Items[clip.ID], after.Items[slow.ID], after.Items[plain.ID] = clip, slow, plain
	got := describe(t, before, after)
	for _, want := range []string{
		`+ item itm_k000000001 clip on trk_f6g7h8j9k0 6.667s/200f +1s/30f ast_3c5v8n2w6d srcin=1.5s/45f x1.5 fit=contain`,
		`+ item itm_k000000002 clip on trk_f6g7h8j9k0 10s/300f +1s/30f ast_3c5v8n2w6d x0.333`,
		`+ item itm_k000000003 clip on trk_f6g7h8j9k0 13.333s/400f +1s/30f ast_3c5v8n2w6d`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestDescribeDiffMiddleAndBottomTracks(t *testing.T) {
	before := baseDoc(t)
	after := timeline.Clone(before)
	after.Tracks["trk_k000000001"] = timeline.Track{ID: "trk_k000000001", Kind: timeline.TrackVisual, Order: "a05"}
	after.Tracks["trk_k000000002"] = timeline.Track{ID: "trk_k000000002", Kind: timeline.TrackAudio, Order: "z"}
	got := describe(t, before, after)
	want := `+ track trk_k000000001 visual "" mid` + "\n" + `+ track trk_k000000002 audio "" bottom`
	if got != want {
		t.Errorf("got:\n%s", got)
	}
	// A track that is not in the document after the change is placed at the top.
	if got := DescribeDiff(
		before,
		[]Change{{Kind: KindTrackAdded, ID: "trk_zzzzzzzzzz", Object: map[string]any{"kind": "audio"}}},
	); !strings.HasSuffix(
		got,
		" top",
	) {
		t.Error(got)
	}
}

// rlo is U+202E, a bidirectional override that compact must drop.
var rlo = string(rune(0x202e))

func TestDescribeDiffValues(t *testing.T) {
	doc := baseDoc(t)
	d := describer{fps: timelineFPS(doc), positions: nil}
	long := strings.Repeat("é", 60)
	cases := []struct {
		field string
		v     any
		want  string
	}{
		{"x", nil, "none"},
		{"x", true, "true"},
		{"x", false, "false"},
		{"x", jn("12.5"), "12.5"},
		{
			"markers",
			[]any{map[string]any{"frame": jn("19")}, map[string]any{"frame": jn("36")}},
			"2 markers at 19f 36f",
		},
		{"start", jn("375"), "12.5s/375f"},
		{"fade_in", jn("0"), "0s/0f"},
		{"name", "a\"b\\c<d&e" + rlo + "\x07", `"a\"b\\c&lt;d&amp;e"`},
		{"name", long, `"` + strings.Repeat("é", 40) + `…"`},
		{"crop", map[string]any{"left": jn("0.1")}, `{"left":0.1}`},
		{"words", []any{"x", jn("1")}, `["x",1]`},
	}
	for _, c := range cases {
		got := d.value(c.field, c.v)
		if got != c.want {
			t.Errorf("value(%s, %v) = %s, want %s", c.field, c.v, got, c.want)
		}
	}
	if got := d.value("start", "text"); got != `"text"` {
		t.Error(got)
	}
	if got := d.value("start", jn("1.5")); got != "1.5" {
		t.Errorf("a non-integer frame value is written as is: %s", got)
	}
	if got := d.value(
		"blob",
		[]any{strings.Repeat("a", 80)},
	); utf8.RuneCountInString(got) != maxValue ||
		!strings.HasSuffix(got, "…") {
		t.Errorf("%s", got)
	}
	if got := d.cell("x", false); got != "none" {
		t.Error(got)
	}
}

func TestCompactForm(t *testing.T) {
	cases := []struct {
		in     string
		budget int
		want   string
	}{
		{"plain", 10, `"plain"`},
		{"", 10, `""`},
		{"a\nb\tc", 10, `"abc"`},
		{`say "hi" \ <tag> & more`, 100, `"say \"hi\" \\ &lt;tag> &amp; more"`},
		{"abcdefghij", 4, `"abcd…"`},
		{rlo + "evil" + string(rune(0x2066)) + "x", 10, `"evilx"`},
	}
	for _, c := range cases {
		if got := compact(c.in, c.budget); got != c.want {
			t.Errorf("compact(%q, %d) = %s, want %s", c.in, c.budget, got, c.want)
		}
	}
}

func TestDescribeDiffCapsALineAtThreeHundredCharacters(t *testing.T) {
	doc := baseDoc(t)
	changes := map[string]Pair{}
	for i := range 30 {
		changes["field_"+strings.Repeat("a", 3)+string(rune('a'+i%26))+string(rune('a'+i/26))] = Pair{jn("1"), jn("2")}
	}
	got := DescribeDiff(doc, []Change{{Kind: KindItemUpdated, ID: clipID, Changes: changes}})
	if n := utf8.RuneCountInString(got); n != maxLine || !strings.HasSuffix(got, "…") {
		t.Errorf("line has %d characters", n)
	}
}

func TestDescribeDiffUnknownKindAndInvalidFPS(t *testing.T) {
	doc := baseDoc(t)
	doc.Settings.FPS = timeline.Rational{}
	got := DescribeDiff(doc, []Change{
		{Kind: "weird.kind", ID: "x"},
		{Kind: KindItemUpdated, ID: clipID, Changes: map[string]Pair{"start": {jn("30"), jn("60")}}},
	})
	if got != "? weird.kind x\n~ item itm_m1n2p3q4r5: start 1s/30f -> 2s/60f" {
		t.Error(got)
	}
}
