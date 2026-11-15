package otio

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
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
	first, lost, err := Export(sampleDoc())
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

	// Markers have no command yet: they are reported, not silently dropped.
	if len(im.Lost) != 1 || !strings.Contains(im.Lost[0].Feature, "markers") {
		t.Errorf("import lost = %+v, want the markers", im.Lost)
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

	// The marker and the text item are the declared losses; compare without them.
	want := withoutMarkers(t, first)
	if !bytes.Equal(withoutMarkers(t, second), want) {
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

	for _, f := range []string{"nested", "track", "markers", "effects", "transition", "clip"} {
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
