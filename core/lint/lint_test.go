package lint

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	trkText  = "trk_text000001"
	trkVideo = "trk_video00001"
	trkAudio = "trk_audio00001"
	astVideo = "ast_video000001"
	astSound = "ast_sound000001"
)

// newDoc is a valid-looking 1080x1920 project at 30 fps with a text track on
// top, a video track and an audio track, and two assets. Items are added by
// the tests.
func newDoc() *timeline.Document {
	return &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion,
		ID:            "prj_7k2m9x4q1b",
		Name:          "t",
		Settings:      timeline.DefaultSettings(),
		Assets: map[string]timeline.Asset{
			astVideo: {
				ID:         astVideo,
				Kind:       timeline.AssetVideo,
				FrameRate:  timeline.Rational{Num: 30, Den: 1},
				FrameCount: 900,
				HasAudio:   true,
			},
			astSound: {
				ID:         astSound,
				Kind:       timeline.AssetAudio,
				FrameRate:  timeline.Rational{Num: 30, Den: 1},
				FrameCount: 900,
			},
		},
		Tracks: map[string]timeline.Track{
			trkText:  {ID: trkText, Kind: timeline.TrackVisual, Order: "a0"},
			trkVideo: {ID: trkVideo, Kind: timeline.TrackVisual, Order: "a1"},
			trkAudio: {ID: trkAudio, Kind: timeline.TrackAudio, Order: "a2"},
		},
		Items: map[string]timeline.Item{},
	}
}

func put(doc *timeline.Document, it timeline.Item) { doc.Items[it.ID] = it }

func clip(id string, start, dur int) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemClip, trkVideo, start, dur)
	it.Asset = astVideo

	return it
}

func caption(id string, start, dur int) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemCaption, trkText, start, dur)
	it.Words = []timeline.Word{{Text: "hello", Start: 0, Duration: dur}}

	return it
}

// safeArea is plan.SafeArea of SDD-004 §4.1, repeated for the tests because
// core/lint takes it as an input.
func safeArea(w, h int) (x, y, sw, sh int) {
	l, r, t, b := 0.05, 0.05, 0.05, 0.05
	if float64(w)/float64(h) <= 0.6 {
		l, r, t, b = 0.06, 0.06, 0.14, 0.35
	}

	x, y = int(l*float64(w)+0.5), int(t*float64(h)+0.5)

	return x, y, w - x - int(r*float64(w)+0.5), h - y - int(b*float64(h)+0.5)
}

// textSize measures every text and caption as 800x150 pixels.
func textSize(*timeline.Document, *timeline.Item) (w, h float64, ok bool) { return 800, 150, true }

func env() Env { return Env{SafeArea: safeArea, TextSize: textSize} }

func codes(fs []Finding) []string {
	out := make([]string, len(fs))
	for i := range fs {
		out[i] = fs[i].Code
	}

	return out
}

func has(fs []Finding, code string) []Finding {
	var out []Finding

	for i := range fs {
		if fs[i].Code == code {
			out = append(out, fs[i])
		}
	}

	return out
}

// fine is a document no rule objects to: a clip with sound over the whole
// timeline.
func fine() *timeline.Document {
	d := newDoc()
	put(d, clip("itm_a", 0, 90))

	return d
}

func TestAFineDocumentHasNoFinding(t *testing.T) {
	t.Parallel()

	if got := Lint(fine(), env(), Full); len(got) != 0 {
		t.Errorf("Lint(fine) = %v, want none", codes(got))
	}
}

// rule table: for every rule one document that triggers it and the fine one.
func TestEachRuleTriggersAndStaysQuiet(t *testing.T) {
	t.Parallel()

	m := func(f func(d *timeline.Document)) *timeline.Document {
		d := fine()
		f(d)

		return d
	}

	tests := []struct {
		name string
		code string
		doc  *timeline.Document
		env  Env
		mode Mode
		want int
		sev  string
	}{
		{"empty timeline", CodeEmptyTimeline, newDoc(), env(), Fast, 1, SeverityError},
		{
			"odd width",
			CodeOddDimensions,
			m(func(d *timeline.Document) { d.Settings.Width = 1081 }),
			env(),
			Fast,
			1,
			SeverityError,
		},
		{
			"small height",
			CodeOddDimensions,
			m(func(d *timeline.Document) { d.Settings.Height = 8 }),
			env(),
			Fast,
			1,
			SeverityError,
		},
		{
			"huge width",
			CodeOddDimensions,
			m(func(d *timeline.Document) { d.Settings.Width = 4098 }),
			env(),
			Fast,
			1,
			SeverityError,
		},
		{"missing media", CodeMediaMissing, m(func(d *timeline.Document) {
			it := clip("itm_b", 0, 30)
			it.Asset = "ast_gone0000001"
			put(d, it)
		}), env(), Fast, 1, SeverityError},
		{
			"unready media",
			CodeMediaUnready,
			fine(),
			Env{Unready: func(string) bool { return true }},
			Fast,
			1,
			SeverityError,
		},
		{"unfilled slot", CodeSlotUnfilled, m(func(d *timeline.Document) {
			it := clip("itm_b", 0, 30)
			s := timeline.NewSlot("hero", "media")
			it.Slot = &s
			put(d, it)
		}), env(), Fast, 1, SeverityError},
		{"placeholder asset", CodeSlotUnfilled, m(func(d *timeline.Document) {
			it := clip("itm_b", 0, 30)
			it.Asset = timeline.PlaceholderAsset
			put(d, it)
		}), env(), Fast, 1, SeverityError},
		{"gap under text", CodeGap, m(func(d *timeline.Document) {
			put(d, clip("itm_b", 120, 30))
			put(d, caption("itm_c", 80, 60))
		}), env(), Fast, 1, SeverityWarning},
		{
			"black frames",
			CodeBlackFrames,
			m(func(d *timeline.Document) { put(d, clip("itm_b", 100, 30)) }),
			env(),
			Fast,
			1,
			SeverityWarning,
		},
		{"silent export", CodeSilentExport, m(func(d *timeline.Document) {
			a := d.Assets[astVideo]
			a.HasAudio = false
			d.Assets[astVideo] = a
		}), env(), Fast, 1, SeverityWarning},
		{
			"sliver clip",
			CodeSliver,
			m(func(d *timeline.Document) { put(d, clip("itm_b", 90, 2)) }),
			env(),
			Fast,
			1,
			SeverityWarning,
		},
		{
			"sliver caption",
			CodeSliver,
			m(func(d *timeline.Document) { put(d, caption("itm_c", 10, 1)) }),
			env(),
			Fast,
			1,
			SeverityWarning,
		},
		{"caption above the safe area", CodeSafeArea, m(func(d *timeline.Document) {
			c := caption("itm_c", 0, 60)
			pos, y := "custom", 100.0
			c.Style = &timeline.CaptionStyle{Position: &pos}
			c.Y = &y
			put(d, c)
		}), env(), Fast, 1, SeverityWarning},
		{"text beside the frame", CodeSafeArea, m(func(d *timeline.Document) {
			x := 1000.0
			it := timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 60)
			it.Text, it.X = "hi", &x
			put(d, it)
		}), env(), Fast, 1, SeverityWarning},
		{
			"clip beyond the media in the store",
			CodeSourceRange,
			fine(),
			Env{MediaFrames: func(string) (int, bool) { return 60, true }},
			Full,
			1,
			SeverityError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := has(Lint(tt.doc, tt.env, tt.mode), tt.code)
			if len(got) != tt.want {
				t.Fatalf("Lint() has %d %s findings, want %d", len(got), tt.code, tt.want)
			}

			if got[0].Severity != tt.sev {
				t.Errorf("severity = %s, want %s", got[0].Severity, tt.sev)
			}

			if got[0].Message == "" || got[0].Hint == "" {
				t.Errorf("finding %+v needs a message and a hint", got[0])
			}
		})
	}
}

func TestRulesThatNearlyTrigger(t *testing.T) {
	t.Parallel()

	m := func(f func(d *timeline.Document)) []Finding {
		d := fine()
		f(d)

		return Lint(d, env(), Full)
	}

	tests := []struct {
		name string
		code string
		got  []Finding
	}{
		{
			"even small dimensions",
			CodeOddDimensions,
			m(func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 16, 4096 }),
		},
		{"a gap of 2 frames is not reported", CodeGap, m(func(d *timeline.Document) {
			put(d, clip("itm_b", 92, 30))
			put(d, caption("itm_c", 80, 43))
		})},
		{"a placeholder in a template", CodeMediaMissing, m(func(d *timeline.Document) {
			d.Template = &timeline.TemplateInfo{Description: "x"}
			it := clip("itm_b", 0, 30)
			it.Asset = timeline.PlaceholderAsset
			put(d, it)
		})},
		{"an open slot in a template", CodeSlotUnfilled, m(func(d *timeline.Document) {
			d.Template = &timeline.TemplateInfo{Description: "x"}
			it := clip("itm_b", 0, 30)
			s := timeline.NewSlot("hero", "media")
			it.Slot = &s
			put(d, it)
		})},
		{"a filled slot", CodeSlotUnfilled, m(func(d *timeline.Document) {
			it := clip("itm_b", 0, 30)
			s := timeline.NewSlot("hero", "media")
			s.Filled = true
			it.Slot = &s
			put(d, it)
		})},
		{"3 frames is not a sliver", CodeSliver, m(func(d *timeline.Document) { put(d, clip("itm_b", 90, 3)) })},
		{"a text item is never a sliver", CodeSliver, m(func(d *timeline.Document) {
			put(d, timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 1))
		})},
		{"a hidden text track is not judged", CodeSafeArea, m(func(d *timeline.Document) {
			tr := d.Tracks[trkText]
			tr.Hidden = true
			d.Tracks[trkText] = tr
			x := 1000.0
			it := timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 60)
			it.X = &x
			put(d, it)
		})},
		{
			"a caption on the bottom edge",
			CodeSafeArea,
			m(func(d *timeline.Document) { put(d, caption("itm_c", 0, 60)) }),
		},
		{"a caption in the middle", CodeSafeArea, m(func(d *timeline.Document) {
			c := caption("itm_c", 0, 60)
			pos := "middle"
			c.Style = &timeline.CaptionStyle{Position: &pos}
			put(d, c)
		})},
		{"a caption on top by its track style", CodeSafeArea, m(func(d *timeline.Document) {
			pos := "top"
			tr := d.Tracks[trkText]
			tr.CaptionStyle = &timeline.CaptionStyle{Position: &pos}
			d.Tracks[trkText] = tr
			put(d, caption("itm_c", 0, 60))
		})},
		{"a rotated text that still fits", CodeSafeArea, m(func(d *timeline.Document) {
			it := timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 60)
			y := 700.0
			it.Rotation, it.Y = 90, &y
			put(d, it)
		})},
		{"a clip within the media", CodeSourceRange, m(func(*timeline.Document) {})},
		{"an image is exempt from the source range", CodeSourceRange, func() []Finding {
			d := fine()
			a := d.Assets[astVideo]
			a.Kind = timeline.AssetImage
			d.Assets[astVideo] = a

			return Lint(d, Env{MediaFrames: func(string) (int, bool) { return 1, true }}, Full)
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := has(tt.got, tt.code); len(got) != 0 {
				t.Errorf("got %v, want no %s", got, tt.code)
			}
		})
	}
}

func TestLayerRangesAreExact(t *testing.T) {
	t.Parallel()

	d := fine() // clip 0-90
	put(d, clip("itm_b", 120, 30))
	put(d, caption("itm_c", 80, 60)) // text 80-140

	got := Lint(d, env(), Fast)

	gaps, blacks := has(got, CodeGap), has(got, CodeBlackFrames)
	if len(gaps) != 1 || *gaps[0].Range != (Range{90, 120}) {
		t.Errorf("gaps = %+v, want [90,120)", gaps)
	}

	if len(blacks) != 0 {
		t.Errorf("blacks = %+v: text covers 90-140 and the clip 120-150", blacks)
	}

	// Text over nothing is a gap, empty frames are black frames.
	d2 := fine()
	put(d2, clip("itm_b", 130, 20))

	if b := has(Lint(d2, env(), Fast), CodeBlackFrames); len(b) != 1 || *b[0].Range != (Range{90, 130}) {
		t.Errorf("black frames = %+v, want [90,130)", b)
	}

	// A hidden track counts as absent.
	d3 := fine()
	tr := d3.Tracks[trkVideo]
	tr.Hidden = true
	d3.Tracks[trkVideo] = tr

	if b := has(Lint(d3, env(), Fast), CodeBlackFrames); len(b) != 1 || *b[0].Range != (Range{0, 90}) {
		t.Errorf("hidden video track: black frames = %+v, want [0,90)", b)
	}
}

func TestAudioKeepsAnExportAudible(t *testing.T) {
	t.Parallel()

	silent := func(d *timeline.Document) []Finding { return has(Lint(d, env(), Fast), CodeSilentExport) }

	d := fine()
	a := d.Assets[astVideo]
	a.HasAudio = false
	d.Assets[astVideo] = a

	music := timeline.NewItem("itm_m", timeline.ItemAudio, trkAudio, 0, 90)
	music.Asset = astSound
	put(d, music)

	if len(silent(d)) != 0 {
		t.Error("an audio item makes the export audible")
	}

	music.Volume = 0
	put(d, music)

	if len(silent(d)) != 1 {
		t.Error("an audio item at volume 0 is silent")
	}

	music.Volume = 1
	put(d, music)

	tr := d.Tracks[trkAudio]
	tr.Muted = true
	d.Tracks[trkAudio] = tr

	if len(silent(d)) != 1 {
		t.Error("a muted track is silent")
	}

	d2 := fine()
	c := d2.Items["itm_a"]
	c.AudioMuted = true
	d2.Items["itm_a"] = c

	if len(silent(d2)) != 1 {
		t.Error("a muted clip is silent")
	}
}

func TestSafeAreaOfAWideFrameUsesFivePercent(t *testing.T) {
	t.Parallel()

	d := fine()
	d.Settings.Width, d.Settings.Height = 1920, 1080
	// 800 wide centred stays inside 5%; a text whose box crosses the right edge does not.
	x := 1700.0
	it := timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 60)
	it.X = &x
	put(d, it)

	if got := has(Lint(d, env(), Fast), CodeSafeArea); len(got) != 1 {
		t.Errorf("got %d findings, want 1", len(got))
	}
}

func TestSafeAreaRuleNeedsItsInputs(t *testing.T) {
	t.Parallel()

	d := fine()
	c := caption("itm_c", 0, 60)
	y := 10.0
	pos := "custom"
	c.Style, c.Y = &timeline.CaptionStyle{Position: &pos}, &y
	put(d, c)

	for name, e := range map[string]Env{
		"no measurer":        {SafeArea: safeArea},
		"no safe area":       {TextSize: textSize},
		"a failing measurer": {SafeArea: safeArea, TextSize: func(*timeline.Document, *timeline.Item) (float64, float64, bool) { return 0, 0, false }},
	} {
		if got := has(Lint(d, e, Fast), CodeSafeArea); len(got) != 0 {
			t.Errorf("%s: got %v, want the rule skipped", name, got)
		}
	}
}

func TestFastModeLeavesTheStoreRulesOut(t *testing.T) {
	t.Parallel()

	e := Env{MediaFrames: func(string) (int, bool) { return 10, true }}
	if got := has(Lint(fine(), e, Fast), CodeSourceRange); len(got) != 0 {
		t.Errorf("Fast ran L_SOURCE_RANGE: %v", got)
	}

	unknown := Env{MediaFrames: func(string) (int, bool) { return 0, false }}
	if got := has(Lint(fine(), unknown, Full), CodeSourceRange); len(got) != 0 {
		t.Errorf("an asset with no known media was judged: %v", got)
	}

	if got := has(Lint(fine(), Env{}, Full), CodeSourceRange); len(got) != 0 {
		t.Errorf("no MediaFrames: got %v, want the rule skipped", got)
	}
}

func TestBlockingSet(t *testing.T) {
	t.Parallel()

	for code, want := range map[string]bool{
		CodeEmptyTimeline: true, CodeMediaMissing: true, CodeMediaUnready: true, CodeSourceRange: true,
		CodeOddDimensions: true, CodeSlotUnfilled: true,
		CodeGap: false, CodeBlackFrames: false, CodeSilentExport: false, CodeSliver: false, CodeSafeArea: false,
	} {
		if got := Blocking(code); got != want {
			t.Errorf("Blocking(%s) = %v, want %v", code, got, want)
		}
	}
}

func TestOrderIsErrorsThenCodeThenRangeThenItem(t *testing.T) {
	t.Parallel()

	fs := []Finding{
		{Code: CodeSliver, Severity: SeverityWarning, Item: "itm_b", Range: &Range{5, 6}},
		{Code: CodeSliver, Severity: SeverityWarning, Item: "itm_a", Range: &Range{5, 6}},
		{Code: CodeGap, Severity: SeverityWarning, Range: &Range{9, 12}},
		{Code: CodeGap, Severity: SeverityWarning},
		{Code: CodeMediaMissing, Severity: SeverityError, Item: "itm_z", Range: &Range{1, 2}},
		{Code: CodeEmptyTimeline, Severity: SeverityError},
	}
	Sort(fs)

	want := []string{CodeEmptyTimeline, CodeMediaMissing, CodeGap, CodeGap, CodeSliver, CodeSliver}
	got := codes(fs)

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}

	if fs[2].Range != nil || fs[4].Item != "itm_a" {
		t.Errorf("ties not broken by range start then item: %+v", fs)
	}
}

func TestTotals(t *testing.T) {
	t.Parallel()

	e, w := Totals([]Finding{{Severity: SeverityError}, {Severity: SeverityWarning}, {Severity: SeverityWarning}})
	if e != 1 || w != 2 {
		t.Errorf("Totals = %d, %d, want 1, 2", e, w)
	}
}

// RF-107: a caption moved above the safe area is exactly one new finding.
func TestNewReportsOnlyWhatABatchIntroduced(t *testing.T) {
	t.Parallel()

	before := fine()
	put(before, caption("itm_c", 0, 60))

	after := fine()
	c := caption("itm_c", 0, 60)
	pos, y := "custom", 100.0
	c.Style, c.Y = &timeline.CaptionStyle{Position: &pos}, &y
	put(after, c)

	b, a := Lint(before, env(), Fast), Lint(after, env(), Fast)
	if len(b) != 0 {
		t.Fatalf("before has findings: %v", codes(b))
	}

	got := New(b, a)
	if len(got) != 1 || got[0].Code != CodeSafeArea || got[0].Item != "itm_c" {
		t.Errorf("New() = %+v, want exactly one L_SAFE_AREA on itm_c", got)
	}

	if again := New(a, a); len(again) != 0 {
		t.Errorf("New(a, a) = %v, want none: a finding that was already there is not new", again)
	}

	// A finding that disappears is not reported, and the same finding on
	// another range start is new.
	moved := []Finding{{Code: CodeGap, Severity: SeverityWarning, Range: &Range{10, 20}}}
	was := []Finding{{Code: CodeGap, Severity: SeverityWarning, Range: &Range{0, 5}}}

	if got := New(was, moved); len(got) != 1 {
		t.Errorf("a gap on a new range = %v, want 1", got)
	}

	if got := New(moved, nil); len(got) != 0 {
		t.Errorf("a fixed finding = %v, want none", got)
	}
}

func TestFindingsAreStableAcrossRuns(t *testing.T) {
	t.Parallel()

	d := referenceDoc()

	first := Lint(d, referenceEnv(), Full)
	for range 20 {
		again := Lint(d, referenceEnv(), Full)
		if len(again) != len(first) {
			t.Fatal("the number of findings changed between runs")
		}

		for i := range again {
			if again[i].Code != first[i].Code || again[i].Item != first[i].Item {
				t.Fatalf("finding %d changed between runs: %+v vs %+v", i, again[i], first[i])
			}
		}
	}
}

// referenceDoc triggers one finding of most rules, with the ids and times of
// the examples of SDD-003 §3.6.
func referenceDoc() *timeline.Document {
	d := newDoc()
	d.ID = "prj_9q8w7e6r5t"

	put(d, clip("itm_m1n2p3q4r5", 0, 120))

	gone := clip("itm_g0n3a1b2c3", 120, 60)
	gone.Asset = "ast_3c5v8n2w6d"
	put(d, gone)

	put(d, clip("itm_k4s5l6i7v8", 200, 2))

	capt := caption("itm_s6t7v8w9x0", 100, 80)
	y, pos := 100.0, "custom"
	capt.Style, capt.Y = &timeline.CaptionStyle{Position: &pos}, &y
	put(d, capt)

	return d
}

func referenceEnv() Env {
	e := env()
	e.MediaFrames = func(id string) (int, bool) { return 60, id == astVideo }

	return e
}

func fps30() timeval.FPS { return timeval.FPS{Num: 30, Den: 1} }

func TestReferenceReportEqualsTheGoldenText(t *testing.T) {
	t.Parallel()

	fs := Lint(referenceDoc(), referenceEnv(), Full)
	got, next := Report(Header{Revision: 9, ProjectID: "prj_9q8w7e6r5t", FPS: fps30()}, fs, 0)

	if next != -1 {
		t.Errorf("next = %d, want the whole report on one page", next)
	}

	path := filepath.Join("testdata", "reference.report.txt")

	if *update {
		if err := os.WriteFile(path, []byte(got+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got+"\n" != string(want) {
		t.Errorf("report differs from %s:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestReportWithNoFinding(t *testing.T) {
	t.Parallel()

	got, next := Report(Header{Revision: 3, ProjectID: "prj_9q8w7e6r5t", FPS: fps30()}, nil, 0)
	if got != "lint r3 prj_9q8w7e6r5t: errors=0 warnings=0" || next != -1 {
		t.Errorf("Report(nil) = %q, %d", got, next)
	}
}

func TestReportPagesWithinTheCap(t *testing.T) {
	t.Parallel()

	fs := make([]Finding, 0, 400)
	for i := range 400 {
		fs = append(fs, Finding{
			Code: CodeSliver, Severity: SeverityWarning, Item: "itm_0123456789",
			Range:   &Range{int64(i), int64(i + 1)},
			Message: "the clip lasts 1 frame(s), which is usually the result of a trim or split at the wrong time.",
			Hint:    "delete it or trim the neighbour",
		})
	}

	h := Header{Revision: 1, ProjectID: "prj_9q8w7e6r5t", FPS: fps30()}
	total, from, pages := 0, 0, 0

	for from >= 0 {
		text, next := Report(h, fs, from)
		if n := len([]rune(text)); n > ReportCap {
			t.Fatalf("page %d has %d characters, cap %d", pages, n, ReportCap)
		}

		if next != -1 && next <= from {
			t.Fatalf("next = %d does not advance from %d", next, from)
		}

		total += countBlocks(text)
		from = next
		pages++
	}

	if total != len(fs) || pages < 2 {
		t.Errorf("pages = %d, findings shown = %d, want every finding over several pages", pages, total)
	}
}

func countBlocks(text string) int {
	n := 0

	for _, line := range splitLines(text) {
		if line != "" && line[0] != ' ' && line[:4] != "lint" {
			n++
		}
	}

	return n
}

func splitLines(s string) []string {
	var out []string

	start := 0

	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}

	return append(out, s[start:])
}

func TestReportKeepsAtLeastOneBlockAndClampsFrom(t *testing.T) {
	t.Parallel()

	long := Finding{Code: CodeGap, Severity: SeverityWarning, Message: strings.Repeat("x", 1000), Hint: "h"}
	huge := long
	huge.Message = strings.Repeat("x", 2*ReportCap)

	text, next := Report(Header{Revision: 1, ProjectID: "prj_x", FPS: fps30()}, []Finding{huge, long}, -5)
	if next != 1 || countBlocks(text) != 1 {
		t.Errorf("next = %d, blocks = %d: an oversized first block must still be shown alone", next, countBlocks(text))
	}

	if _, next := Report(Header{FPS: fps30()}, []Finding{long}, 99); next != -1 {
		t.Errorf("from past the end: next = %d, want -1", next)
	}
}

func TestCatalogAgreesWithTheRules(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}

	for _, r := range Catalog() {
		if seen[r.Code] {
			t.Errorf("%s is listed twice", r.Code)
		}

		seen[r.Code] = true

		if r.Hint == "" || r.Triggers == "" || (r.Severity != SeverityError && r.Severity != SeverityWarning) {
			t.Errorf("incomplete entry %+v", r)
		}

		if r.Implemented && Blocking(r.Code) != (r.Severity == SeverityError) {
			t.Errorf("%s: Blocking = %v but severity is %s", r.Code, Blocking(r.Code), r.Severity)
		}
	}

	// Every finding Lint can produce carries the catalog's severity and hint.
	d := referenceDoc()
	d.Settings.Width = 1081
	put(d, timeline.NewItem("itm_t", timeline.ItemText, trkText, 0, 60))

	for _, f := range Lint(d, referenceEnv(), Full) {
		var rule *Rule

		for _, r := range Catalog() {
			if r.Code == f.Code {
				rule = &r
			}
		}

		if rule == nil || !rule.Implemented || rule.Severity != f.Severity ||
			(rule.Hint != f.Hint && f.Code != CodeSafeArea) {
			t.Errorf("finding %s disagrees with the catalog: %+v", f.Code, rule)
		}
	}
}
