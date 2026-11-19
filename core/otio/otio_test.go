package otio

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	vidAsset = "ast_a000000001"
	audAsset = "ast_a000000002"
	imgAsset = "ast_a000000003"
	trkTop   = "trk_t000000001"
	trkVid   = "trk_t000000002"
	trkAud   = "trk_t000000003"
)

func assets() map[string]timeline.Asset {
	return map[string]timeline.Asset{
		vidAsset: {
			ID: vidAsset, Hash: "sha256:" + strings.Repeat("a", 64), Kind: timeline.AssetVideo, Name: "clips/a.mp4",
			FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 900, Width: 1080, Height: 1920, HasAudio: true,
		},
		audAsset: {
			ID: audAsset, Hash: "sha256:" + strings.Repeat("b", 64), Kind: timeline.AssetAudio, Name: "music.wav",
			SampleCount: 48000 * 40,
		},
		imgAsset: {
			ID: imgAsset, Hash: "sha256:" + strings.Repeat("c", 64), Kind: timeline.AssetImage, Name: "logo.png",
			Width: 100, Height: 100,
		},
	}
}

func newDoc(fps timeline.Rational) *timeline.Document {
	d := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: "prj_7k2m9x4q1b", Name: "Demo",
		Settings: timeline.DefaultSettings(),
		Assets:   assets(), Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}
	d.Settings.FPS = fps

	return d
}

func sampleDoc() *timeline.Document {
	d := newDoc(timeline.Rational{Num: 30, Den: 1})
	d.Markers = []timeline.Marker{{Frame: 0}, {Frame: 45, Label: "chorus"}}
	d.Tracks[trkTop] = timeline.Track{ID: trkTop, Kind: timeline.TrackVisual, Name: "Top", Order: "a0"}
	d.Tracks[trkVid] = timeline.Track{ID: trkVid, Kind: timeline.TrackVisual, Name: "Video", Order: "a1"}
	d.Tracks[trkAud] = timeline.Track{ID: trkAud, Kind: timeline.TrackAudio, Name: "Music", Order: "a2"}

	c1 := timeline.NewItem("itm_c000000001", timeline.ItemClip, trkVid, 0, 90)
	c1.Asset, c1.SourceIn, c1.Name = vidAsset, 30, "intro"
	c1.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 12}
	c1.Markers = []timeline.Marker{{Frame: 10, Label: "hit"}}

	c2 := timeline.NewItem("itm_c000000002", timeline.ItemClip, trkVid, 120, 60)
	c2.Asset, c2.SourceIn = vidAsset, 300
	c2.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 8}

	logo := timeline.NewItem("itm_c000000003", timeline.ItemClip, trkVid, 200, 30)
	logo.Asset = imgAsset

	music := timeline.NewItem("itm_c000000004", timeline.ItemAudio, trkAud, 15, 150)
	music.Asset = audAsset

	title := timeline.NewItem("itm_c000000005", timeline.ItemText, trkTop, 10, 40)
	title.Text = "Hello"

	for _, it := range []*timeline.Item{&c1, &c2, &logo, &music, &title} {
		d.Items[it.ID] = *it
	}

	return d
}

func TestExportGolden(t *testing.T) {
	out, _, err := Export(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join("testdata", "sample.otio")
	if *update {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), out) {
		t.Errorf("the export differs from %s:\n%s", path, out)
	}
}

func seqIDs() func(string) string {
	n := 0

	return func(prefix string) string {
		n++

		return prefix + "_" + strings.Repeat("0", 7) + strconv.Itoa(1000 + n)[1:]
	}
}

// build applies the batch of an import to a project that has the assets.
func build(t *testing.T, im *Import, d *timeline.Document) *timeline.Document {
	t.Helper()

	byRef := map[string]string{}
	for id := range d.Assets {
		byRef[d.Assets[id].Name] = id
	}

	ops, lost := im.Operations(byRef, seqIDs())

	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}

	res, err := command.Default().ApplyBatch(d, ops, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	return res.Doc
}

func TestRoundTrip(t *testing.T) {
	d := sampleDoc()
	first, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(lost) != 1 || lost[0].Feature != "text item" || lost[0].ID != "itm_c000000005" {
		t.Errorf("lost = %+v, want only the text item", lost)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	// The markers of a clip come back through marker.add with item_id (TASK-0254).
	if len(im.Lost) != 0 {
		t.Errorf("import lost = %+v, want nothing", im.Lost)
	}

	if im.FPS != (timeline.Rational{Num: 30, Den: 1}) || im.Name != "Demo" {
		t.Errorf("fps %v name %q", im.FPS, im.Name)
	}

	doc := build(t, im, newDoc(im.FPS))
	if errs := timeline.Validate(doc); len(errs) != 0 {
		t.Fatalf("the imported document is invalid: %v", errs)
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(doc.Markers, d.Markers) {
		t.Errorf("document markers did not come back: %+v, want %+v", doc.Markers, d.Markers)
	}

	if !bytes.Equal(second, first) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}

func withoutMarkers(t *testing.T, data []byte) []byte {
	t.Helper()

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}

	var strip func(any)

	strip = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			if _, ok := x["markers"]; ok {
				x["markers"] = []any{}
			}

			for _, c := range x {
				strip(c)
			}
		case []any:
			for _, c := range x {
				strip(c)
			}
		}
	}
	strip(v)

	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func TestRoundTripAt2997(t *testing.T) {
	d := newDoc(timeline.Rational{Num: 30000, Den: 1001})
	d.Tracks[trkVid] = timeline.Track{ID: trkVid, Kind: timeline.TrackVisual, Name: "V", Order: "a1"}
	c := timeline.NewItem("itm_c000000001", timeline.ItemClip, trkVid, 7, 61)
	c.Asset, c.SourceIn = vidAsset, 13
	d.Items[c.ID] = c

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	if im.FPS != (timeline.Rational{Num: 30000, Den: 1001}) {
		t.Fatalf("fps %v", im.FPS)
	}

	second, _, err := Export(build(t, im, newDoc(im.FPS)))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("round trip differs:\n%s\n%s", first, second)
	}
}

func TestExportReportsEveryLoss(t *testing.T) {
	d := newDoc(timeline.Rational{Num: 30, Den: 1})
	d.Tracks[trkTop] = timeline.Track{
		ID: trkTop, Kind: timeline.TrackVisual, Order: "a0", Hidden: true, Muted: true,
		CaptionStyle: &timeline.CaptionStyle{},
	}
	c := timeline.NewItem("itm_c000000001", timeline.ItemClip, trkTop, 0, 30)
	c.Asset = vidAsset
	c.Keyframes = timeline.Keyframes{"scale": {timeline.NewKeyframe(0, 1)}}
	c.Speed = timeline.Rational{Num: 2, Den: 1}
	c.Brightness = 0.2
	c.Filter, c.Blur = "sepia", 3
	c.Fit = "contain"
	x := 10.0
	c.X = &x
	c.Volume = 2
	c.TransitionIn = &timeline.Transition{Kind: "slide_left", Duration: 5}
	s := timeline.NewSlot("main", "media")
	c.Slot = &s
	ph := timeline.NewItem("itm_c000000002", timeline.ItemClip, trkTop, 40, 10)
	ph.Asset = timeline.PlaceholderAsset
	sh := timeline.NewItem("itm_c000000003", timeline.ItemShape, trkTop, 60, 10)
	capt := timeline.NewItem("itm_c000000004", timeline.ItemCaption, trkTop, 80, 10)

	for _, it := range []*timeline.Item{&c, &ph, &sh, &capt} {
		d.Items[it.ID] = *it
	}

	out, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, l := range lost {
		got[l.Feature] = true
	}

	for _, f := range []string{
		"hidden track", "muted track", "caption style", "keyframes", "speed", "colour adjustment", "filter and blur",
		"framing", "transform and opacity", "audio settings", "template slot", "transition slide_left",
		"placeholder or missing asset", "shape item", "caption item",
	} {
		if !got[f] {
			t.Errorf("no loss reported for %q", f)
		}
	}

	if strings.Contains(string(out), "Transition.1") {
		t.Error("a slide transition must not be written")
	}
}

func TestImportReportsMediaWithoutOpeningIt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ssrf.otio"))
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"http://169.254.169.254/x.mp4": Unresolved,
		`\\evil\share\a.mp4`:           Unresolved,
		"//evil/share/a.mp4":           Unresolved,
		`\\?\GLOBALROOT\a.mp4`:         Unresolved,
		"file:///etc/passwd":           Unresolved,
		"smb://host/a.mp4":             Unresolved,
		"data:video/mp4;base64,AAAA":   Unresolved,
		"concat:a.mp4|b.mp4":           Unresolved,
		"":                             Unresolved,
		"bad\x00name.mp4":              Unresolved,
		`C:\media\a.mp4`:               Candidate,
		"media/b.mp4":                  Candidate,
	}

	if len(im.Media) != len(want) {
		t.Fatalf("%d media references, want %d: %+v", len(im.Media), len(want), im.Media)
	}

	for _, m := range im.Media {
		if m.Status != want[m.Ref] {
			t.Errorf("%q is %s, want %s", m.Ref, m.Status, want[m.Ref])
		}

		if m.Status == Unresolved && m.Reason == "" {
			t.Errorf("%q is unresolved without a reason", m.Ref)
		}
	}
}

// The package cannot reach the network or the file system: it imports neither.
func TestImportOpensNoConnection(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}

		for _, imp := range af.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "os" || p == "io/ioutil" || p == "net" || strings.HasPrefix(p, "net/") ||
				strings.HasPrefix(p, "os/") {
				t.Errorf("%s imports %s", f, p)
			}
		}
	}
}

func TestOperationsLeaveOutUnknownAssets(t *testing.T) {
	first, _, err := Export(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	ops, lost := im.Operations(map[string]string{"clips/a.mp4": vidAsset}, seqIDs())

	if len(lost) != 2 {
		t.Errorf("lost = %+v, want the logo and the music", lost)
	}

	for _, op := range ops {
		if op.Op == "audio.add" {
			t.Error("an audio clip without its asset must not be added")
		}
	}
}

func TestImportLossesAndOddInput(t *testing.T) {
	const doc = `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
	  {"OTIO_SCHEMA":"Stack.1","children":[]},
	  {"OTIO_SCHEMA":"Track.1","kind":"Subtitle","children":[]},
	  {"OTIO_SCHEMA":"Track.1","kind":"Audio","name":"a","children":[
	    {"OTIO_SCHEMA":"Gap.1","source_range":{"start_time":{"value":0,"rate":24},"duration":{"value":24,"rate":24}}},
	    {"OTIO_SCHEMA":"Clip.2","name":"snd","source_range":{"start_time":{"value":0,"rate":24},"duration":{"value":48,"rate":24}},
	      "media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"s.wav"},"effects":[{"OTIO_SCHEMA":"LinearTimeWarp.1"}],
	      "markers":[{}]},
	    {"OTIO_SCHEMA":"Transition.1","transition_type":"SMPTE_Dissolve","in_offset":{"value":6,"rate":24},"out_offset":{"value":0,"rate":24}},
	    {"OTIO_SCHEMA":"Transition.1","transition_type":"Wipe","in_offset":{"value":6,"rate":24},"out_offset":{"value":0,"rate":24}},
	    {"OTIO_SCHEMA":"Clip.2","name":"nosrc","media_reference":{"OTIO_SCHEMA":"MissingReference.1"}},
	    {"OTIO_SCHEMA":"Clip.2","name":"miss","source_range":{"start_time":{"value":0,"rate":24},"duration":{"value":5,"rate":24}},
	      "media_reference":{"OTIO_SCHEMA":"MissingReference.1"}},
	    {"OTIO_SCHEMA":"Generator.1"}
	  ]}]}}`

	im, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	if im.FPS != (timeline.Rational{Num: 24, Den: 1}) {
		t.Errorf("fps %v", im.FPS)
	}

	feats := map[string]bool{}
	for _, l := range im.Lost {
		feats[strings.SplitN(l.Feature, " ", 2)[0]] = true
	}

	for _, f := range []string{"nested", "track", "marker", "effects", "transition", "clip"} {
		if !feats[f] {
			t.Errorf("no loss reported starting with %q: %+v", f, im.Lost)
		}
	}

	ops, lost := im.Operations(map[string]string{"s.wav": audAsset, "": audAsset}, seqIDs())

	// A dissolve next to an audio clip cannot become a transition.
	if len(lost) == 0 {
		t.Error("the transition on an audio clip must be reported")
	}

	if ops[0].Op != "track.add" || !strings.Contains(string(ops[1].Args), `"start":"24f"`) {
		t.Errorf("unexpected batch: %+v", ops)
	}
}

func TestImportTransitionsBetweenClips(t *testing.T) {
	const doc = `{"OTIO_SCHEMA":"Timeline.1","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
	  {"OTIO_SCHEMA":"Track.1","kind":"Video","children":[
	    {"OTIO_SCHEMA":"Clip.2","source_range":{"start_time":{"value":0,"rate":30},"duration":{"value":60,"rate":30}},
	      "media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"a.mp4"}},
	    {"OTIO_SCHEMA":"Transition.1","transition_type":"SMPTE_Dissolve","in_offset":{"value":10,"rate":30},"out_offset":{"value":5,"rate":30}},
	    {"OTIO_SCHEMA":"Clip.2","source_range":{"start_time":{"value":0,"rate":30},"duration":{"value":60,"rate":30}},
	      "media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"a.mp4"}}]}]}}`

	im, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	ops, lost := im.Operations(map[string]string{"a.mp4": vidAsset}, seqIDs())
	if len(lost) != 0 {
		t.Fatalf("%v", lost)
	}

	var edges []string

	for _, op := range ops {
		if op.Op == "item.set_transition" {
			edges = append(edges, string(op.Args))
		}
	}

	if len(edges) != 2 || !strings.Contains(edges[0], `"edge":"in"`) || !strings.Contains(edges[0], `"5f"`) ||
		!strings.Contains(edges[1], `"edge":"out"`) || !strings.Contains(edges[1], `"10f"`) {
		t.Errorf("transitions = %v", edges)
	}
}

func TestParseRejects(t *testing.T) {
	deep := strings.Repeat(`{"OTIO_SCHEMA":"Stack.1","children":[`, MaxDepth+2) + strings.Repeat(`]}`, MaxDepth+2)

	for name, in := range map[string]string{
		"not json":       `{`,
		"not a timeline": `{"OTIO_SCHEMA":"Clip.2"}`,
		"no stack":       `{"OTIO_SCHEMA":"Timeline.1","tracks":{"OTIO_SCHEMA":"Track.1"}}`,
		"too deep":       `{"OTIO_SCHEMA":"Timeline.1","tracks":` + deep + `}`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	nested := strings.Repeat(`{"OTIO_SCHEMA":"Stack.1","children":[`, MaxDepth-1) + strings.Repeat(`]}`, MaxDepth-1)
	if _, err := Parse([]byte(`{"OTIO_SCHEMA":"Timeline.1","tracks":` + nested + `}`)); err != nil {
		t.Errorf("a nesting of %d is allowed: %v", MaxDepth, err)
	}
}

func TestRateToFPS(t *testing.T) {
	for rate, want := range map[float64]timeline.Rational{
		24: {Num: 24, Den: 1}, 29.97002997002997: {Num: 30000, Den: 1001}, 23.976023976023978: {Num: 24000, Den: 1001},
		59.94005994005994: {Num: 60000, Den: 1001},
	} {
		if got, err := rateToFPS(rate); err != nil || got != want {
			t.Errorf("rate %v: %v %v, want %v", rate, got, err, want)
		}
	}

	for _, bad := range []float64{0, -1, 0.5, 500, 29.5} {
		if _, err := rateToFPS(bad); err == nil {
			t.Errorf("rate %v must be refused", bad)
		}
	}
}

func TestFramesRefuseBadTimes(t *testing.T) {
	r := &reader{fps: timeline.Rational{Num: 30, Den: 1}}

	for _, v := range []any{
		nil, "x",
		map[string]any{"value": 1.0},
		map[string]any{"value": 1.0, "rate": 0.0},
		map[string]any{"value": 1e12, "rate": 30.0},
	} {
		if _, ok := r.frames(v); ok {
			t.Errorf("%v must be refused", v)
		}
	}
}

func FuzzImport(f *testing.F) {
	for _, name := range []string{"sample.otio", "ssrf.otio"} {
		if b, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(b)
		}
	}

	f.Add(
		[]byte(
			`{"OTIO_SCHEMA":"Timeline.1","tracks":{"OTIO_SCHEMA":"Stack.1","children":[{"OTIO_SCHEMA":"Track.1","kind":"Video","children":[{"OTIO_SCHEMA":"Clip.2"}]}]}}`,
		),
	)

	f.Fuzz(func(_ *testing.T, data []byte) {
		im, err := Parse(data)
		if err != nil {
			return
		}

		assets := map[string]string{}
		for _, m := range im.Media {
			assets[m.Ref] = vidAsset
		}

		im.Operations(assets, seqIDs())
	})
}

// VV-025: the properties OTIO has no field for travel in the vukem metadata and come back
// as commands.
func TestRoundTripKeepsVukemProperties(t *testing.T) {
	d := sampleDoc()

	trk := d.Tracks[trkVid]
	trk.Muted = true
	d.Tracks[trkVid] = trk

	c1 := d.Items["itm_c000000001"]
	x := 120.0
	c1.X, c1.Scale, c1.Rotation, c1.Opacity = &x, 1.5, 12, 0.8
	c1.Volume, c1.FadeIn, c1.AudioMuted = 0.5, 6, true
	c1.Brightness, c1.Contrast, c1.Saturation = 0.1, 1.2, 0.9
	c1.Temperature, c1.Tint, c1.Hue = 0.3, -0.2, 45
	c1.Fit, c1.Crop = "contain", &timeline.Crop{Left: 0.1, Bottom: 0.2}
	c1.Filter, c1.FilterAmount, c1.Blur = "sepia", 0.6, 4
	c1.SpeedCurve = []timeline.SpeedPoint{
		{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}},
		{T: 0.4, Speed: timeline.Rational{Num: 3, Den: 2}},
		{T: 1, Speed: timeline.Rational{Num: 3, Den: 4}},
	}
	c1.Speed, c1.Reverse = timeline.CurveMean(c1.SpeedCurve), true
	amt, rad, thr, sz, seed := 0.4, 20.0, 0.5, 2, 99
	c1.Effects = []timeline.Effect{
		{Kind: timeline.EffectGlow, Amount: &amt, Radius: &rad, Threshold: &thr},
		{Kind: timeline.EffectGrain, Amount: &amt, Size: &sz, Seed: &seed},
	}
	d.Items[c1.ID] = c1

	music := d.Items["itm_c000000004"]
	music.Volume, music.FadeOut = 2, 10
	d.Items[music.ID] = music

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	var got timeline.Item

	for _, it := range doc.Items {
		if it.Name == "intro" {
			got = it
		}
	}

	if got.X == nil || *got.X != 120 || got.Scale != 1.5 || got.Rotation != 12 || got.Opacity != 0.8 ||
		got.Volume != 0.5 || got.FadeIn != 6 || !got.AudioMuted || got.Brightness != 0.1 || got.Contrast != 1.2 ||
		got.Saturation != 0.9 || got.Temperature != 0.3 || got.Tint != -0.2 || got.Hue != 45 || got.Fit != "contain" || got.Crop == nil || got.Crop.Left != 0.1 ||
		got.Crop.Bottom != 0.2 || got.Filter != "sepia" || got.FilterAmount != 0.6 || got.Blur != 4 {
		t.Errorf("properties did not come back: %+v", got)
	}

	if len(got.SpeedCurve) != 3 || got.SpeedCurve[1] != c1.SpeedCurve[1] || got.Speed != c1.Speed ||
		got.Duration != c1.Duration || !got.Reverse || len(got.Effects) != 2 || *got.Effects[1].Seed != 99 ||
		*got.Effects[0].Radius != 20 {
		t.Errorf("speed curve did not come back: %+v (want %+v)", got.SpeedCurve, c1.SpeedCurve)
	}

	if !doc.Tracks[findTrack(doc, "Video")].Muted {
		t.Error("the muted flag of the track did not come back")
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withoutMarkers(t, second), withoutMarkers(t, first)) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}

func findTrack(d *timeline.Document, name string) string {
	for id, t := range d.Tracks {
		if t.Name == name {
			return id
		}
	}

	return ""
}

// The metadata of a file is untrusted: a value out of range is reported and left out, and
// the rest of the batch still applies.
func TestImportReportsBadVukemMetadata(t *testing.T) {
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
	{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"V","metadata":{"vukem":{"hidden":"yes"}},"children":[
	{"OTIO_SCHEMA":"Clip.2","name":"c","metadata":{"vukem":{"properties":{"scale":99,"opacity":"x","blur":3,
	"crop":{"left":5},"filter":"vhs","fit":"cover","blend":"dodge"}}},
	"source_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":0},
	"duration":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":30}},
	"media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"clips/a.mp4"}}]}]}}`

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	lost := strings.Builder{}
	for _, l := range im.Lost {
		lost.WriteString(l.Feature + ";")
	}

	for _, want := range []string{"scale", "opacity", "crop", "filter", "fit", "blend"} {
		if !strings.Contains(lost.String(), "vukem property "+want) {
			t.Errorf("%s was not reported: %s", want, lost.String())
		}
	}

	doc := build(t, im, newDoc(im.FPS))
	for _, it := range doc.Items {
		if it.Blur != 3 || it.Scale != 1 {
			t.Errorf("the valid blur was not kept or a bad value got through: %+v", it)
		}
	}
}

// byName indexes the items of a document by name, with the id and the track left out, so that a
// document and its round trip compare with reflect.DeepEqual.
func byName(d *timeline.Document) map[string]timeline.Item {
	out := map[string]timeline.Item{}

	for id := range d.Items {
		it := d.Items[id]
		it.ID, it.Track = "", ""
		out[it.Name] = it
	}

	return out
}

// Speed, keyframes, slots, texts with their style and the chroma key travel in the vukem
// metadata: the items of the imported document are the items of the exported one.
//
//go:fix inline
//go:fix inline
func TestRoundTripKeepsSpeedKeyframesSlotsTextsAndKey(t *testing.T) {
	d := sampleDoc()
	delete(d.Items, "itm_c000000005")

	slow := timeline.NewItem("itm_d000000001", timeline.ItemClip, trkVid, 300, 60)
	slow.Asset, slow.SourceIn, slow.Name = vidAsset, 30, "sped"
	slow.Speed = timeline.Rational{Num: 3, Den: 2}
	slow.Keyframes = timeline.Keyframes{
		"scale":   {timeline.NewKeyframe(0, 1), {T: 30, Value: 1.5, Easing: "ease_out"}, timeline.NewKeyframe(60, 1)},
		"opacity": {{T: 10, Value: 0.5, Easing: "hold"}},
		"volume":  {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(20, 1)},
	}
	slow.Slot = &timeline.Slot{
		Name: "hero", Kind: "media", Description: "Main shot", Required: false, DurationPolicy: "fit", Filled: true,
	}
	slow.ChromaKey = &timeline.ChromaKey{Color: "#00FF00", Similarity: 0.35, Smoothness: 0.05, Spill: 0.25}
	slow.Blend = "screen"
	slow.FlipX = true
	slow.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.2, Width: 0.5, Height: 0.6, Feather: 12, Invert: true}

	// 7 frames at 1/4 speed cannot come from any whole source length: the clip is trimmed to it.
	crawl := timeline.NewItem("itm_d000000002", timeline.ItemClip, trkVid, 400, 7)
	crawl.Asset, crawl.SourceIn, crawl.Name = vidAsset, 0, "crawl"
	crawl.Speed = timeline.Rational{Num: 1, Den: 4}

	voice := timeline.NewItem("itm_d000000003", timeline.ItemAudio, trkAud, 200, 80)
	voice.Asset, voice.Name = audAsset, "voice"
	voice.Speed = timeline.Rational{Num: 2, Den: 1}
	voice.Keyframes = timeline.Keyframes{"volume": {timeline.NewKeyframe(0, 1), timeline.NewKeyframe(80, 0.25)}}

	title := timeline.NewItem("itm_d000000004", timeline.ItemText, trkTop, 10, 40)
	title.Name, title.Text = "title", "Hello\nworld"
	title.Style = &timeline.CaptionStyle{
		FontFamily: new("Inter"), Size: new(72.0), StrokeWidth: new(4.0),
	}
	color := timeline.Color("#FFCC00")
	title.Style.Color = &color
	title.X, title.Y, title.Scale, title.Rotation, title.Opacity = new(540.0), new(300.0), 1.2, 5, 0.9
	title.Keyframes = timeline.Keyframes{"opacity": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(10, 0.9)}}
	title.Slot = &timeline.Slot{Name: "headline", Kind: "text", Required: true, DurationPolicy: "fit", Filled: true}
	title.Blend = "multiply"
	title.FlipX, title.FlipY = true, true
	voice.Disabled, title.Disabled = true, true
	voice.Denoise = 0.35
	voice.Voice = "radio"
	title.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0.25, Width: 1, Height: 0.5}

	plain := timeline.NewItem("itm_d000000005", timeline.ItemText, trkTop, 60, 20)
	plain.Name, plain.Text = "plain", "Bye"

	for _, it := range []*timeline.Item{&slow, &crawl, &voice, &title, &plain} {
		d.Items[it.ID] = *it
	}

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range im.Lost {
		if !strings.Contains(l.Feature, "markers") {
			t.Errorf("import lost %q", l.Feature)
		}
	}

	doc := build(t, im, newDoc(im.FPS))
	if errs := timeline.Validate(doc); len(errs) != 0 {
		t.Fatalf("the imported document is invalid: %v", errs)
	}

	want, got := byName(d), byName(doc)
	for _, name := range []string{"sped", "crawl", "voice", "title", "plain"} {
		if !reflect.DeepEqual(got[name], want[name]) {
			t.Errorf("%s did not come back:\n got  %+v\n want %+v", name, got[name], want[name])
		}
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withoutMarkers(t, second), withoutMarkers(t, first)) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}

func TestExportReportsWhatTextItemsDoNotCarry(t *testing.T) {
	d := newDoc(timeline.Rational{Num: 30, Den: 1})
	d.Tracks[trkTop] = timeline.Track{ID: trkTop, Kind: timeline.TrackVisual, Name: "Top", Order: "a0"}
	title := timeline.NewItem("itm_c000000001", timeline.ItemText, trkTop, 0, 30)
	title.Text, title.Blur = "Hi", 3
	title.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 5}
	title.Markers = []timeline.Marker{{Frame: 4}}
	d.Items[title.ID] = title

	_, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, l := range lost {
		got[l.Feature] = true
	}

	for _, f := range []string{"text item", "markers of a text item"} {
		if !got[f] {
			t.Errorf("no loss reported for %q: %+v", f, lost)
		}
	}

	// The blur and the transition travel in the metadata now (VV-025).
	for _, f := range []string{"filter and blur", "transition of a text item"} {
		if got[f] {
			t.Errorf("%q is still reported as lost: %+v", f, lost)
		}
	}
}

// A file nobody vouches for: bad speed, keyframes, slot, key and text style are reported and
// left out, and the rest of the batch still applies.
func TestImportReportsBadAnimationSlotAndTextMetadata(t *testing.T) {
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
	{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"V","children":[
	{"OTIO_SCHEMA":"Gap.1","name":"t1","metadata":{"vukem":{"text":"ok","style":{"size":20,"font_family":"Nope"},
	"properties":{"slot":{"name":"Bad Name"}}}},
	"source_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":0},
	"duration":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":30}}},
	{"OTIO_SCHEMA":"Gap.1","name":"t2","metadata":{"vukem":{"text":""}},
	"source_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":0},
	"duration":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":10}}},
	{"OTIO_SCHEMA":"Clip.2","name":"c","metadata":{"vukem":{"properties":{"speed":{"num":9,"den":1},
	"keyframes":{"scale":[{"t":5,"value":1,"easing":"linear"},{"t":5,"value":2,"easing":"linear"}],
	"opacity":[{"t":0,"value":0.5,"easing":"linear"}]},"chroma_key":{"color":"green"},
	"mask":{"shape":"star","x":0,"y":0,"width":1,"height":1,"feather":0},
	"slot":{"name":"main","required":true}}}},
	"source_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":0},
	"duration":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":30}},
	"media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"clips/a.mp4"}}]}]}}`

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	lost := strings.Builder{}
	for _, l := range im.Lost {
		lost.WriteString(l.Feature + ";")
	}

	for _, want := range []string{
		"vukem property slot", "vukem property text", "vukem property speed",
		"vukem property keyframes.scale", "vukem property chroma_key", "vukem property mask",
	} {
		if !strings.Contains(lost.String(), want) {
			t.Errorf("%q was not reported: %s", want, lost.String())
		}
	}

	ops, dropped := im.Operations(map[string]string{}, seqIDs())
	if len(dropped) != 2 || !strings.Contains(dropped[0].Feature+dropped[1].Feature, "style font Nope") {
		t.Errorf("the unknown font and the clip without an asset are reported: %+v", dropped)
	}

	res, err := command.Default().ApplyBatch(newDoc(im.FPS), ops, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	doc := res.Doc

	texts := 0

	for id := range doc.Items {
		it := doc.Items[id]

		switch it.Type {
		case timeline.ItemText:
			texts++

			if it.Text != "ok" || it.Style == nil || it.Style.FontFamily != nil || it.Style.Size == nil ||
				*it.Style.Size != 20 {
				t.Errorf("the text did not come back without its unknown font: %+v", it)
			}
		case timeline.ItemClip:
			if it.Speed.Num != 1 || it.Slot != nil || it.ChromaKey != nil || it.Mask != nil ||
				len(it.Keyframes["scale"]) != 0 ||
				len(it.Keyframes["opacity"]) != 1 {
				t.Errorf("a bad value got through or a valid one was lost: %+v", it)
			}
		default:
			t.Errorf("unexpected item %+v", it)
		}
	}

	if texts != 1 {
		t.Errorf("%d text items, want 1", texts)
	}
}

func TestExportDoesNotListMaskAsLost(t *testing.T) {
	d := sampleDoc()
	it := d.Items["itm_c000000001"]
	it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5}
	d.Items[it.ID] = it

	_, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range lost {
		if strings.Contains(l.Feature, "mask") {
			t.Errorf("the mask is carried by the metadata, got a loss: %+v", l)
		}
	}
}

func TestExportDoesNotListBlendAsLost(t *testing.T) {
	d := sampleDoc()
	it := d.Items["itm_c000000001"]
	it.Blend = "add"
	d.Items[it.ID] = it

	_, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range lost {
		if l.Feature == "blend mode" {
			t.Errorf("blend is carried by the metadata, got a loss: %+v", l)
		}
	}
}

func TestImportReadsNativeStackMarkersAndReportsBadOnes(t *testing.T) {
	rt := func(v int) string {
		return fmt.Sprintf(`{"OTIO_SCHEMA":"RationalTime.1","value":%d,"rate":30}`, v)
	}
	mk := func(name string, v int) string {
		return `{"OTIO_SCHEMA":"Marker.2","name":"` + name +
			`","marked_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":` + rt(v) + `,"duration":` + rt(0) + `}}`
	}
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[],
	"markers":[` + mk("a", 5) + `,` + mk("dup", 5) + `,` + mk("neg", -3) + `,` + mk(strings.Repeat("x", 61), 9) + `]}}`

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	if len(im.Lost) != 3 {
		t.Errorf("lost = %+v, want the duplicate, the negative and the long label", im.Lost)
	}

	doc := build(t, im, newDoc(im.FPS))
	if want := []timeline.Marker{{Frame: 5, Label: "a"}}; !reflect.DeepEqual(doc.Markers, want) {
		t.Errorf("markers = %+v, want %+v", doc.Markers, want)
	}
}

func TestImportReadsClipMarkersRelativeToTheSourceRangeAndReportsBadOnes(t *testing.T) {
	rt := func(v int) string {
		return fmt.Sprintf(`{"OTIO_SCHEMA":"RationalTime.1","value":%d,"rate":30}`, v)
	}
	mk := func(name string, v int) string {
		return `{"OTIO_SCHEMA":"Marker.2","name":"` + name +
			`","marked_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":` + rt(v) + `,"duration":` + rt(0) + `}}`
	}
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
	{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"v","children":[
	{"OTIO_SCHEMA":"Clip.2","name":"c","source_range":{"start_time":` + rt(30) + `,"duration":` + rt(60) + `},
	"media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"clips/a.mp4"},
	"markers":[` + mk("a", 40) + `,` + mk("dup", 40) + `,` + mk("before", 29) + `,` + mk("after", 90) + `,` + mk("b", 89) + `]}]}]}}`

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	if len(im.Lost) != 3 {
		t.Errorf("lost = %+v, want the duplicate and the two outside the clip", im.Lost)
	}

	doc := build(t, im, newDoc(im.FPS))
	want := []timeline.Marker{{Frame: 10, Label: "a"}, {Frame: 59, Label: "b"}}

	for _, it := range doc.Items {
		if !reflect.DeepEqual(it.Markers, want) {
			t.Errorf("markers = %+v, want %+v", it.Markers, want)
		}
	}
}

func TestImportReportsBadVukemTimelineMarkers(t *testing.T) {
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","metadata":{"vukem":{"markers":[
	{"frame":4,"label":"ok"},{"frame":"x"},{"frame":1.5},{"frame":6,"label":7},{"frame":4}]}},
	"tracks":{"OTIO_SCHEMA":"Stack.1","children":[]}}`

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	if len(im.Lost) != 4 {
		t.Errorf("lost = %+v, want 4", im.Lost)
	}

	if want := []timeline.Marker{{Frame: 4, Label: "ok"}}; !reflect.DeepEqual(im.markers, want) {
		t.Errorf("markers = %+v, want %+v", im.markers, want)
	}
}

// VV-025: shapes, captions, the caption style of a track, every kind of transition, and the blur and
// filter of a text come back from the vukem metadata; a project font is kept when the caller names it.
func TestRoundTripKeepsShapesCaptionsAndTransitions(t *testing.T) {
	d := sampleDoc()

	size, pos, fam := 64.0, "middle", "Project Sans"
	top := d.Tracks[trkTop]
	top.CaptionStyle = &timeline.CaptionStyle{
		Size: &size, FontFamily: &fam, Position: &pos,
	}
	d.Tracks[trkTop] = top
	d.Fonts = map[string]timeline.ProjectFont{fam: {}}

	c1 := d.Items["itm_c000000001"]
	c1.TransitionIn = &timeline.Transition{Kind: "slide_up", Duration: 6}
	d.Items[c1.ID] = c1

	title := d.Items["itm_c000000005"]
	title.Blur, title.Filter, title.FilterAmount = 4, "sepia", 0.5
	title.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 5}
	title.TransitionOut = &timeline.Transition{Kind: "slide_left", Duration: 7}
	title.Style = &timeline.CaptionStyle{FontFamily: &fam}
	d.Items[title.ID] = title

	shape := timeline.NewItem("itm_c000000006", timeline.ItemShape, trkTop, 60, 30)
	shape.Name, shape.Shape, shape.Width, shape.Height, shape.Radius = "badge", "rounded_rect", 300, 120, 24
	shape.Fill, shape.StrokeColor, shape.StrokeWidth = "#FF8800", "#112233", 3
	shape.Opacity, shape.Blur = 0.7, 2
	shape.TransitionOut = &timeline.Transition{Kind: "slide_down", Duration: 4}
	d.Items[shape.ID] = shape

	sub := timeline.NewItem("itm_c000000007", timeline.ItemCaption, trkTop, 100, 40)
	sub.Name = "subtitle"
	sub.Words = []timeline.Word{
		{
			Text:     "one",
			Start:    0,
			Duration: 10,
		},
		{Text: "two", Start: 10, Duration: 10},
		{Text: "three", Start: 20, Duration: 10},
	}
	sub.Breaks = []int{2}
	sub.Style = &timeline.CaptionStyle{Position: &pos}
	sub.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 3}
	d.Items[sub.ID] = sub

	first, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range lost {
		if l.Feature == "filter and blur" || l.Feature == "transition of a text item" {
			t.Errorf("still reported as lost: %+v", l)
		}
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	im.KnownFonts = []string{fam}

	byRef := map[string]string{}
	for id := range d.Assets {
		byRef[d.Assets[id].Name] = id
	}

	empty := newDoc(im.FPS)
	empty.Fonts = d.Fonts

	ops, dropped := im.Operations(byRef, seqIDs())
	for _, l := range dropped {
		t.Errorf("dropped: %+v", l)
	}

	res, err := command.Default().ApplyBatch(empty, ops, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	doc := res.Doc

	byName := map[string]timeline.Item{}
	for _, it := range doc.Items {
		byName[it.Name+"/"+string(it.Type)] = it
	}

	if got := byName["intro/clip"]; got.TransitionIn == nil || got.TransitionIn.Kind != "slide_up" ||
		got.TransitionIn.Duration != 6 || got.TransitionOut == nil || got.TransitionOut.Kind != "fade" {
		t.Errorf("clip transitions: %+v / %+v", got.TransitionIn, got.TransitionOut)
	}

	if got := byName["/text"]; got.Blur != 4 || got.Filter != "sepia" || got.FilterAmount != 0.5 ||
		got.TransitionIn == nil || got.TransitionIn.Kind != "fade" || got.TransitionOut == nil ||
		got.TransitionOut.Kind != "slide_left" || got.Style == nil || got.Style.FontFamily == nil ||
		*got.Style.FontFamily != fam {
		t.Errorf("text: %+v", got)
	}

	got := byName["badge/shape"]
	if got.Shape != "rounded_rect" || got.Width != 300 || got.Height != 120 || got.Radius != 24 ||
		got.Fill != "#FF8800" || got.StrokeColor != "#112233" || got.StrokeWidth != 3 || got.Opacity != 0.7 ||
		got.Blur != 2 ||
		got.TransitionOut == nil ||
		got.TransitionOut.Kind != "slide_down" ||
		got.Start != 60 ||
		got.Duration != 30 {
		t.Errorf("shape: %+v", got)
	}

	got = byName["subtitle/caption"]
	if len(got.Words) != 3 || got.Words[2].Text != "three" || got.Words[2].Start != 20 || len(got.Breaks) != 1 ||
		got.Breaks[0] != 2 || got.Style == nil || got.Style.Position == nil || *got.Style.Position != "middle" ||
		got.TransitionIn == nil || got.Start != 100 || got.Duration != 40 {
		t.Errorf("caption: %+v", got)
	}

	var ts *timeline.CaptionStyle

	for _, tr := range doc.Tracks {
		if tr.Name == "Top" {
			ts = tr.CaptionStyle
		}
	}

	if ts == nil || ts.Size == nil || *ts.Size != 64 || ts.Position == nil || *ts.Position != "middle" ||
		ts.FontFamily == nil || *ts.FontFamily != fam {
		t.Errorf("caption style of the track: %+v", ts)
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withoutMarkers(t, second), withoutMarkers(t, first)) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}

// A font that is neither bundled nor named by the caller is dropped from a style and reported, as
// before; the rest of the style stays.
func TestImportDropsAnUnknownProjectFont(t *testing.T) {
	d := sampleDoc()
	fam, size := "Project Sans", 30.0
	title := d.Items["itm_c000000005"]
	title.Style = &timeline.CaptionStyle{FontFamily: &fam, Size: &size}
	d.Items[title.ID] = title

	data, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	_, dropped := im.Operations(
		map[string]string{"clips/a.mp4": vidAsset, "music.wav": audAsset, "logo.png": imgAsset},
		seqIDs(),
	)

	found := false

	for _, l := range dropped {
		found = found || strings.Contains(l.Feature, "Project Sans")
	}

	if !found {
		t.Errorf("the unknown font was not reported: %+v", dropped)
	}
}

func TestRoundTripKeepsMagnetAndReportsLutAndFreezeOnImport(t *testing.T) {
	d := sampleDoc()
	tr := d.Tracks[trkVid]
	tr.Magnet = true
	d.Tracks[trkVid] = tr

	c := d.Items["itm_c000000001"]
	c.Lut = &timeline.Lut{Asset: "ast_l000000001", Intensity: 0.5}
	c.Freeze = true
	d.Items[c.ID] = c

	data, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, l := range lost {
		got[l.Feature] = true
	}

	for _, f := range []string{"track magnet", "lookup table", "freeze frame"} {
		if !got[f] {
			t.Errorf("export did not report %q: %+v", f, lost)
		}
	}

	im, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	imported := map[string]bool{}
	for _, l := range im.Lost {
		imported[strings.SplitN(l.Feature, " of clip", 2)[0]] = true
	}

	if !imported["freeze frame"] || !imported["lookup table (the .cube asset is not part of the file)"] {
		t.Errorf("import did not report the lookup table and the freeze frame: %+v", im.Lost)
	}

	ops, _ := im.Operations(map[string]string{"clips/a.mp4": vidAsset, "logo.png": imgAsset}, seqIDs())

	found := false

	for _, op := range ops {
		if op.Op != "track.update" {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal(op.Args, &args); err != nil {
			t.Fatal(err)
		}

		if args["magnet"] == true {
			found = true
		}
	}

	if !found {
		t.Errorf("the magnet of the track was not restored: %+v", ops)
	}
}

func TestRoundTripKeepsTheAdvancedColour(t *testing.T) {
	d := sampleDoc()
	c1 := d.Items["itm_c000000001"]
	c1.Hsl = []timeline.HslBand{{Range: "red", Hue: 0.1}, {Range: "blue", Saturation: -0.4, Lightness: 0.2}}
	c1.Wheels = &timeline.Wheels{Shadows: &timeline.Wheel{Hue: 240, Amount: 0.3, Luma: -0.1}}
	c1.Curves = &timeline.Curves{Master: []timeline.CurvePoint{{X: 0, Y: 0}, {X: 0.5, Y: 0.4}, {X: 1, Y: 1}}}
	d.Items[c1.ID] = c1

	first, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, l := range lost {
		found = found || l.Feature == "advanced colour"
	}

	if !found {
		t.Errorf("export did not list the advanced colour as lost to other tools: %+v", lost)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	var got timeline.Item

	for _, it := range doc.Items {
		if it.Name == "intro" {
			got = it
		}
	}

	if len(got.Hsl) != 2 ||
		!reflect.DeepEqual(got.Hsl[1], c1.Hsl[1]) ||
		got.Wheels == nil ||
		!reflect.DeepEqual(got.Wheels.Shadows, c1.Wheels.Shadows) ||
		got.Curves == nil ||
		len(got.Curves.Master) != 3 ||
		!reflect.DeepEqual(got.Curves.Master[1], c1.Curves.Master[1]) {
		t.Errorf("the advanced colour did not come back: %+v", got)
	}
}

func TestImportReportsABadAdvancedColour(t *testing.T) {
	p := &propReader{
		props: map[string]any{
			"hsl":    []any{map[string]any{"range": "pink"}},
			"curves": map[string]any{"red": []any{map[string]any{"x": 0.0, "y": 0.0}}},
		},
		lose: func(string) {},
	}

	var lost []string

	p.lose = func(f string) { lost = append(lost, f) }

	if got := gradeArgs(p); len(got) != 0 || len(lost) != 2 {
		t.Errorf("got %v and lost %v, want nothing kept and two losses", got, lost)
	}
}

func TestRoundTripKeepsColourKeyframes(t *testing.T) {
	d := sampleDoc()
	c1 := d.Items["itm_c000000001"]
	c1.Keyframes = timeline.Keyframes{"hue": {
		{T: 0, Value: 0, Easing: "linear"}, {T: 30, Value: 90, Easing: "ease_out"},
	}}
	d.Items[c1.ID] = c1

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	for _, it := range doc.Items {
		if it.Name == "intro" {
			if kfs := it.Keyframes["hue"]; len(kfs) != 2 || kfs[1].Value != 90 || kfs[1].T != 30 {
				t.Errorf("hue keyframes did not come back: %+v", it.Keyframes)
			}
		}
	}
}

func TestRoundTripKeepsMaskAndCropKeyframes(t *testing.T) {
	d := sampleDoc()
	c1 := d.Items["itm_c000000001"]
	c1.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5}
	c1.Keyframes = timeline.Keyframes{
		"mask_x":    {{T: 0, Value: 0, Easing: "linear"}, {T: 30, Value: 0.4, Easing: "linear"}},
		"crop_left": {{T: 0, Value: 0, Easing: "linear"}, {T: 30, Value: 0.2, Easing: "linear"}},
	}
	d.Items[c1.ID] = c1

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	for _, it := range doc.Items {
		if it.Name == "intro" {
			if kfs := it.Keyframes["mask_x"]; len(kfs) != 2 || kfs[1].Value != 0.4 {
				t.Errorf("mask keyframes did not come back: %+v", it.Keyframes)
			}

			if kfs := it.Keyframes["crop_left"]; len(kfs) != 2 || kfs[1].Value != 0.2 {
				t.Errorf("crop keyframes did not come back: %+v", it.Keyframes)
			}
		}
	}
}

// A keyframe with a Bezier easing (epic 31) travels with its four numbers; a curve that is not valid is
// reported and its property left out, like any other bad keyframe list.
func TestRoundTripKeepsBezierKeyframes(t *testing.T) {
	d := sampleDoc()
	c1 := d.Items["itm_c000000001"]
	c1.Keyframes = timeline.Keyframes{"scale": {
		{T: 0, Value: 1, Easing: timeline.EasingBezier, Bezier: []float64{0.34, 1.56, 0.64, 1}},
		{T: 30, Value: 1.5, Easing: "linear"},
	}}
	d.Items[c1.ID] = c1

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range im.Lost {
		if strings.Contains(l.Feature, "keyframes") {
			t.Errorf("the curve was reported as lost: %+v", l)
		}
	}

	doc := build(t, im, newDoc(im.FPS))
	found := false

	for _, it := range doc.Items {
		if it.Name == "intro" {
			found = true
			kfs := it.Keyframes["scale"]

			if len(kfs) != 2 || kfs[0].Easing != timeline.EasingBezier ||
				!reflect.DeepEqual(kfs[0].Bezier, []float64{0.34, 1.56, 0.64, 1}) || kfs[1].Value != 1.5 {
				t.Errorf("the Bezier keyframes did not come back: %+v", it.Keyframes)
			}
		}
	}

	if !found {
		t.Fatal("no item named intro after the import")
	}

	for _, bad := range []any{nil, []any{0.5, 0.0, 1.5, 1.0}, []any{0.5, 0.0, 1.0}, "0.5"} {
		entry := map[string]any{"t": 0.0, "value": 1.0, "easing": "bezier"}
		if bad != nil {
			entry["bezier"] = bad
		}

		if _, ok := keyframeList("scale", []any{entry}, modeClip, 30); ok {
			t.Errorf("a curve %v was accepted", bad)
		}
	}

	linearWithCurve := map[string]any{"t": 0.0, "value": 1.0, "easing": "linear", "bezier": []any{0.5, 0.0, 0.5, 1.0}}
	if _, ok := keyframeList("scale", []any{linearWithCurve}, modeClip, 30); ok {
		t.Error("a curve on a linear keyframe was accepted")
	}
}

func TestRoundTripKeepsEveryTransitionKind(t *testing.T) {
	d := sampleDoc()
	c1 := d.Items["itm_c000000001"]
	c1.TransitionIn = &timeline.Transition{Kind: "wipe", Duration: 10, Direction: "up"}
	c1.TransitionOut = &timeline.Transition{Kind: "spin", Duration: 8}
	d.Items[c1.ID] = c1

	first, _, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	for _, it := range doc.Items {
		if it.Name != "intro" {
			continue
		}

		in, out := it.TransitionIn, it.TransitionOut
		if in == nil || in.Kind != "wipe" || in.Direction != "up" || in.Duration != 10 || out == nil ||
			out.Kind != "spin" {
			t.Errorf("transitions did not come back: %+v %+v", in, out)
		}
	}
}

// VV-025: the lock, the solo flag and the volume of a track come back, and what the file still
// cannot carry (a link, the template information) is reported on export.
func TestRoundTripKeepsTrackLockSoloVolumeAndReportsTheRest(t *testing.T) {
	d := sampleDoc()

	vol := 0.5
	vid, aud := d.Tracks[trkVid], d.Tracks[trkAud]
	vid.Locked = true
	aud.Solo, aud.Volume = true, &vol
	d.Tracks[trkVid], d.Tracks[trkAud] = vid, aud

	c1 := d.Items["itm_c000000001"]
	c1.Link = "lnk_0000000001"
	d.Items[c1.ID] = c1
	d.Template = &timeline.TemplateInfo{}

	first, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	features := map[string]bool{}
	for _, l := range lost {
		features[l.Feature] = true
	}

	for _, want := range []string{"link", "template information of the project"} {
		if !features[want] {
			t.Errorf("export did not report %q: %+v", want, lost)
		}
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	got := map[string]timeline.Track{}
	for _, tr := range doc.Tracks {
		got[tr.Name] = tr
	}

	if !got["Video"].Locked || !got["Music"].Solo || got["Music"].Volume == nil || *got["Music"].Volume != 0.5 {
		t.Errorf("tracks = %+v", got)
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withoutMarkers(t, second), withoutMarkers(t, first)) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}
