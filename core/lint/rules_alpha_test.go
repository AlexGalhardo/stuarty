package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func countCode(doc *timeline.Document, env Env, code string) int {
	return len(has(Lint(doc, env, Full), code))
}

// A 30 fps timeline of 5400 frames is 180 s and is fine; 5401 frames is not.
func TestDurationLimitBoundary(t *testing.T) {
	t.Parallel()

	for frames, want := range map[int]int{5400: 0, 5401: 1} {
		d := newDoc()
		put(d, clip("itm_a", 0, frames))

		if got := countCode(d, Env{}, CodeDurationLimit); got != want {
			t.Errorf("%d frames: %d findings, want %d", frames, got, want)
		}
	}
}

// L_SLOT_UNFILLED is an error. A template draft is exempt; an instance (template.instance)
// reports its open slots and its placeholder, and the colours of the template still resolve.
func TestTemplateExempt(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_hero", 0, 90)
	s := timeline.NewSlot("hero", "media")
	it.Slot = &s
	put(d, it)

	got := has(Lint(d, Env{}, Fast), CodeSlotUnfilled)
	if len(got) != 1 || got[0].Severity != SeverityError || !Blocking(CodeSlotUnfilled) {
		t.Fatalf("a project with an open slot: %+v", got)
	}

	d.Template = &timeline.TemplateInfo{Description: "t"}

	if got := has(Lint(d, Env{}, Fast), CodeSlotUnfilled); len(got) != 0 {
		t.Errorf("a template draft is exempt, got %+v", got)
	}

	d.Template.Instance = true

	got = has(Lint(d, Env{}, Fast), CodeSlotUnfilled)
	if len(got) != 1 || got[0].Item != "itm_hero" {
		t.Errorf("an instance reports its open slot, got %+v", got)
	}
}

func TestInstanceWithPlaceholderIsBlocked(t *testing.T) {
	t.Parallel()

	for _, instance := range []bool{false, true} {
		d := newDoc()
		d.Template = &timeline.TemplateInfo{Description: "t", Instance: instance}
		it := clip("itm_ph", 0, 90)
		it.Asset = timeline.PlaceholderAsset
		put(d, it)

		f := Lint(d, Env{}, Fast)
		if got := has(f, CodeSlotUnfilled); (len(got) == 1) != instance {
			t.Errorf("instance=%v: L_SLOT_UNFILLED findings %+v", instance, got)
		}

		if got := has(f, CodeMediaMissing); len(got) != 0 {
			t.Errorf("instance=%v: the placeholder is not a missing file: %+v", instance, got)
		}
	}
}

// RF-051: with two required slots and one filled, the other one is still reported.
func TestOneUnfilledSlotRemains(t *testing.T) {
	t.Parallel()

	d := newDoc()

	for _, c := range []struct {
		id     string
		filled bool
	}{{"itm_one", true}, {"itm_two", false}} {
		it := clip(c.id, 0, 30)
		s := timeline.NewSlot("slot_"+c.id[4:], "media")
		s.Filled = c.filled
		it.Slot = &s
		put(d, onTrack(it, trkVideo))
		d.Tracks[trkVideo2] = timeline.Track{ID: trkVideo2, Kind: timeline.TrackVisual, Order: "a15"}
	}

	if got := has(Lint(d, Env{}, Fast), CodeSlotUnfilled); len(got) != 1 || got[0].Item != "itm_two" {
		t.Errorf("findings %+v, want only itm_two", got)
	}
}

// A caption box ending at y 1248 is inside the safe area; at 1249 it is not.
func TestSafeAreaBoundary(t *testing.T) {
	t.Parallel()

	for bottom, want := range map[float64]int{1248: 0, 1249: 1} {
		d := newDoc()
		put(d, caption("itm_c", 0, 60))

		env := envSpec{Measure: map[string][4]float64{"itm_c": {140, bottom - 150, 940, bottom}}}.env()
		if got := countCode(d, env, CodeSafeArea); got != want {
			t.Errorf("box ending at %v: %d findings, want %d", bottom, got, want)
		}
	}
}

// On 1080x1350 the rule uses the rectangle of plan.SafeArea and no other margins.
func TestSafeAreaFourFive(t *testing.T) {
	t.Parallel()

	safe := plan.SafeArea(1080, 1350)

	for _, c := range []struct {
		name string
		box  [4]float64
		want int
	}{
		{"on the edges", [4]float64{safe.X, safe.Y, safe.X + safe.W, safe.Y + safe.H}, 0},
		{"one pixel below", [4]float64{safe.X, safe.Y, safe.X + safe.W, safe.Y + safe.H + 1}, 1},
		{"one pixel above", [4]float64{safe.X, safe.Y - 1, safe.X + safe.W, safe.Y + safe.H}, 1},
		{"one pixel to the right", [4]float64{safe.X, safe.Y, safe.X + safe.W + 1, safe.Y + safe.H}, 1},
		{"a 9:16 margin would object", [4]float64{safe.X, 200, safe.X + safe.W, 300}, 0},
	} {
		d := newDoc()
		d.Settings.Width, d.Settings.Height = 1080, 1350
		put(d, caption("itm_c", 0, 60))

		env := envSpec{Measure: map[string][4]float64{"itm_c": c.box}}.env()
		if got := countCode(d, env, CodeSafeArea); got != c.want {
			t.Errorf("%s: %d findings, want %d", c.name, got, c.want)
		}
	}
}

// A first word 3 frames from the transcript is fine, 4 frames is not.
func TestCaptionDesyncBoundary(t *testing.T) {
	t.Parallel()

	for off, want := range map[int]int{0: 0, 3: 0, 4: 1, -4: 1} {
		d := newDoc()
		put(d, clip("itm_clip", 0, 300))

		c := caption("itm_cap", 100+off, 40)
		c.LinkedItem = "itm_clip"
		c.Words[0].Text = "Hello"
		put(d, c)

		env := envSpec{
			Transcript: map[string][]transcriptEntry{astVideo: {{Text: "hello", Start: 100, End: 110}}},
		}.env()
		if got := countCode(d, env, CodeCaptionDesync); got != want {
			t.Errorf("%d frames off: %d findings, want %d", off, got, want)
		}
	}
}

// A target of -14 LUFS with an estimate of -20.0 gives no finding, -20.1 gives one that
// states the gain.
func TestLoudnessBoundary(t *testing.T) {
	t.Parallel()

	for lufs, want := range map[float64]int{-20.0: 0, -20.1: 1, -8.0: 0, -7.9: 1} {
		d := newDoc()
		put(d, clip("itm_a", 0, 90))

		env := envSpec{Loudness: map[string]float64{astVideo: lufs}}.env()

		got := has(Lint(d, env, Full), CodeLoudness)
		if len(got) != want {
			t.Errorf("%v LUFS: %d findings, want %d", lufs, len(got), want)
		}

		if want == 1 && got[0].Message == "" {
			t.Error("the message must state the gain")
		}
	}
}

// A missing artifact makes a rule silent. The three artifact rules are warnings, so
// they never block an export.
func TestArtifactRulesAreSilentAndNeverBlock(t *testing.T) {
	t.Parallel()

	for _, code := range []string{CodeCaptionDesync, CodeAudioClip, CodeLoudness} {
		if Blocking(code) {
			t.Errorf("%s must not block an export", code)
		}
	}

	d := withVideoTrack(newDoc())
	put(d, clip("itm_a", 0, 30))
	put(d, onTrack(clip("itm_b", 0, 30), trkVideo2))

	for _, code := range []string{CodeCaptionDesync, CodeAudioClip, CodeLoudness} {
		if got := countCode(d, Env{}, code); got != 0 {
			t.Errorf("%s without artifacts: %d findings", code, got)
		}
	}

	loud := envSpec{
		Loudness: map[string]float64{astVideo: -5},
		Peaks:    map[string][]float64{astVideo: flat(50, 0.9)},
	}.env()
	for _, f := range Lint(d, loud, Full) {
		if (f.Code == CodeAudioClip || f.Code == CodeLoudness) && f.Severity != SeverityWarning {
			t.Errorf("%s is a %s", f.Code, f.Severity)
		}
	}
}

// Three findings render in the format of SDD-003 §3.6, errors first.
func TestThreeFindingsReport(t *testing.T) {
	t.Parallel()

	d := occlusion(func(up *timeline.Item) {
		s := timeline.NewSlot("hero", "media")
		up.Slot = &s
	})

	fs := Lint(d, Env{}, Full)
	if len(fs) != 3 || fs[0].Severity != SeverityError {
		t.Fatalf("findings %v", codes(fs))
	}

	text, _ := Report(Header{Revision: 1, ProjectID: d.ID, FPS: fps30()}, fs, 0)
	path := filepath.Join("testdata", "report.txt")

	if *update {
		if err := os.WriteFile(path, []byte(text+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if text+"\n" != string(want) {
		t.Errorf("report differs:\n--- got\n%s\n--- want\n%s", text, want)
	}
}

// The transition rule costs the sort of the spans, not the length of the items: a
// 2-hour item with a transition among 300 items lints well under 100 ms.
func TestTransitionEmptyCostDoesNotGrowWithLength(t *testing.T) {
	t.Parallel()

	d := withVideoTrack(newDoc())

	for i := range 300 {
		it := onTrack(clip(fmt.Sprintf("itm_s%09d", i), i*10, 10), trkVideo2)
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 5}
		put(d, it)
	}

	long := clip("itm_long0001", 0, 216000)
	long.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 100000}
	long.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 100000}
	put(d, long)

	start := time.Now()
	Lint(d, Env{}, Full)

	// The race detector slows this loop about tenfold; the limit holds for a
	// normal build, which CI runs too.
	if took := time.Since(start); took > 100*time.Millisecond && !raceEnabled {
		t.Errorf("lint took %v", took)
	}
}
