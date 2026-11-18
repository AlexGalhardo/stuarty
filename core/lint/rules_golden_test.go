package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

// Golden cases of the ALPHA rules (P2-ENG-019 to P2-ENG-021): a case is a directory
// testdata/<code>/<case>/ with doc.json, env.json (what the rule reads besides the
// document) and want.txt, the report of the findings of that one code in the format
// of SDD-003 §3.6. A case named trigger_* has a finding, quiet_* has none. `go test
// ./core/lint -update` writes the files from the table below.

// envSpec is env.json: a box per item for the fake Measurer, and the artifacts of
// the fake ArtifactReader.
type envSpec struct {
	Measure    map[string][4]float64        `json:"measure,omitempty"`
	Peaks      map[string][]float64         `json:"peaks,omitempty"`
	Loudness   map[string]float64           `json:"loudness,omitempty"`
	Transcript map[string][]transcriptEntry `json:"transcript,omitempty"`
}

type transcriptEntry struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type fakeMeasurer map[string][4]float64

func (m fakeMeasurer) Measure(_ *timeline.Document, it *timeline.Item) (Rect, bool) {
	b, ok := m[it.ID]

	return Rect{b[0], b[1], b[2], b[3]}, ok
}

type fakeArtifacts struct{ spec envSpec }

func (f fakeArtifacts) Peaks(a string) (Peaks, bool) { p, ok := f.spec.Peaks[a]; return p, ok }

func (f fakeArtifacts) Loudness(a string) (Loudness, bool) {
	l, ok := f.spec.Loudness[a]

	return Loudness{LUFS: l}, ok
}

func (f fakeArtifacts) Transcript(a string) ([]TranscriptWord, bool) {
	t, ok := f.spec.Transcript[a]
	out := make([]TranscriptWord, len(t))

	for i, w := range t {
		out[i] = TranscriptWord(w)
	}

	return out, ok
}

// planSafeArea adapts plan.SafeArea, the single function of SDD-004 §4.1, to the Env.
func planSafeArea(w, h int) (x, y, sw, sh int) {
	r := plan.SafeArea(w, h)

	return int(r.X), int(r.Y), int(r.W), int(r.H)
}

func (e envSpec) env() Env {
	out := Env{SafeArea: planSafeArea}
	if len(e.Measure) > 0 {
		out.Measurer = fakeMeasurer(e.Measure)
	}

	if len(e.Peaks)+len(e.Loudness)+len(e.Transcript) > 0 {
		out.Artifacts = fakeArtifacts{e}
	}

	return out
}

type ruleCase struct {
	code, name string
	doc        *timeline.Document
	env        envSpec
}

var ruleCases []ruleCase

func rc(code, name string, doc *timeline.Document, env ...envSpec) {
	var e envSpec
	if len(env) > 0 {
		e = env[0]
	}

	ruleCases = append(ruleCases, ruleCase{code, name, doc, e})
}

const (
	trkVideo2 = "trk_video00002"
	itmUp     = "itm_up0000001"
	itmLow    = "itm_low000001"
)

// sized gives the video asset the size of the frame, so that a clip can cover it.
func sized(d *timeline.Document) *timeline.Document {
	a := d.Assets[astVideo]
	a.Width, a.Height = 1080, 1920
	d.Assets[astVideo] = a

	return d
}

// withVideoTrack adds a second video track under the first one.
func withVideoTrack(d *timeline.Document) *timeline.Document {
	d.Tracks[trkVideo2] = timeline.Track{ID: trkVideo2, Kind: timeline.TrackVisual, Order: "a15"}

	return d
}

func onTrack(it timeline.Item, track string) timeline.Item {
	it.Track = track

	return it
}

// occlusion is a lower clip on trkVideo2 under an upper clip on trkVideo.
func occlusion(mod func(up *timeline.Item)) *timeline.Document {
	d := withVideoTrack(sized(newDoc()))
	up := onTrack(clip(itmUp, 0, 120), trkVideo)
	mod(&up)
	put(d, up)
	put(d, onTrack(clip(itmLow, 30, 60), trkVideo2))

	return d
}

func init() {
	none := func(*timeline.Item) {}

	rc(CodeOccluded, "trigger_clip_covers_lower_clip", occlusion(none))
	rc(CodeOccluded, "trigger_contain_on_same_aspect", occlusion(func(up *timeline.Item) { up.Fit = "contain" }))
	rc(CodeOccluded, "trigger_opaque_shape_covers_lower_clip", func() *timeline.Document {
		d := occlusion(none)
		sh := timeline.NewItem(itmUp, timeline.ItemShape, trkVideo, 0, 120)
		sh.Shape, sh.Width, sh.Height = "rect", 1080, 1920
		put(d, sh)

		return d
	}())
	rc(CodeOccluded, "quiet_opacity_below_1", occlusion(func(up *timeline.Item) { up.Opacity = 0.9 }))
	rc(CodeOccluded, "quiet_upper_does_not_contain_the_range", occlusion(func(up *timeline.Item) { up.Duration = 60 }))
	rc(CodeOccluded, "quiet_upper_has_a_transition", occlusion(func(up *timeline.Item) {
		up.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10}
	}))
	rc(
		CodeOccluded,
		"quiet_upper_is_cropped",
		occlusion(func(up *timeline.Item) { up.Crop = &timeline.Crop{Left: 0.1} }),
	)
	rc(CodeOccluded, "quiet_upper_scaled_down", occlusion(func(up *timeline.Item) { up.Scale = 0.9 }))
	rc(CodeOccluded, "quiet_contain_on_another_aspect", func() *timeline.Document {
		d := occlusion(func(up *timeline.Item) { up.Fit = "contain" })
		a := d.Assets[astVideo]
		a.Width, a.Height = 1920, 1080
		d.Assets[astVideo] = a

		return d
	}())
	rc(CodeOccluded, "quiet_upper_is_below", func() *timeline.Document {
		d := occlusion(none)
		d.Tracks[trkVideo2] = timeline.Track{ID: trkVideo2, Kind: timeline.TrackVisual, Order: "a05"}

		return d
	}())
}

func init() {
	sound := func(id, track string, start, dur int) timeline.Item { return onTrack(clip(id, start, dur), track) }
	two := func(second func(*timeline.Item)) *timeline.Document {
		d := withVideoTrack(newDoc())
		put(d, sound("itm_a", trkVideo, 0, 90))

		b := sound("itm_b", trkVideo2, 60, 90)
		second(&b)
		put(d, b)

		return d
	}

	rc(CodeAudioOverlap, "trigger_two_clips_with_sound", two(func(*timeline.Item) {}))
	rc(CodeAudioOverlap, "quiet_back_to_back", two(func(b *timeline.Item) { b.Start = 90 }))
	rc(CodeAudioOverlap, "quiet_one_clip_muted", two(func(b *timeline.Item) { b.AudioMuted = true }))
	rc(CodeAudioOverlap, "quiet_music_under_a_clip", func() *timeline.Document {
		d := two(func(b *timeline.Item) { b.Start = 90 })
		put(d, timeline.NewItem("itm_m", timeline.ItemAudio, trkAudio, 0, 150))

		return d
	}())
}

func init() {
	// A clip on the lower track from 0 to 300, and a visible item on top with a transition.
	over := func(start int, mod func(*timeline.Item)) *timeline.Document {
		d := newDoc()
		d.Tracks[trkText] = timeline.Track{ID: trkText, Kind: timeline.TrackVisual, Order: "a0"}
		it := onTrack(clip("itm_top", start, 60), trkText)
		mod(&it)
		put(d, it)

		return d
	}
	fade := func() *timeline.Transition { return &timeline.Transition{Kind: "fade", Duration: 10} }
	underneath := func(d *timeline.Document) *timeline.Document {
		put(d, clip("itm_base", 0, 300))

		return d
	}

	rc(CodeTransitionEmpty, "trigger_transition_in_over_the_background",
		over(30, func(it *timeline.Item) { it.TransitionIn = fade() }))
	rc(CodeTransitionEmpty, "trigger_transition_out_before_the_end", func() *timeline.Document {
		d := over(30, func(it *timeline.Item) { it.TransitionOut = fade() })
		put(d, onTrack(clip("itm_late", 200, 100), trkVideo))

		return d
	}())
	rc(CodeTransitionEmpty, "quiet_transition_in_at_frame_0",
		over(0, func(it *timeline.Item) { it.TransitionIn = fade() }))
	rc(CodeTransitionEmpty, "quiet_transition_out_at_the_timeline_end",
		over(30, func(it *timeline.Item) { it.TransitionOut = fade() }))
	rc(CodeTransitionEmpty, "quiet_transition_out_of_the_last_picture_while_audio_runs_on", func() *timeline.Document {
		d := over(30, func(it *timeline.Item) { it.TransitionOut = fade() })
		put(d, timeline.NewItem("itm_music", timeline.ItemAudio, trkAudio, 0, 400))

		return d
	}())
	rc(CodeTransitionEmpty, "quiet_transition_out_of_the_last_clip_while_a_title_runs_on", func() *timeline.Document {
		d := over(40, func(it *timeline.Item) { it.TransitionOut = fade() })
		text := timeline.NewItem("itm_title", timeline.ItemText, trkText, 100, 300)
		text.Text = "Fin"
		put(d, text)

		return d
	}())
	rc(CodeTransitionEmpty, "quiet_something_lies_beneath",
		underneath(over(30, func(it *timeline.Item) { it.TransitionIn = fade() })))
	rc(CodeTransitionEmpty, "trigger_beneath_ends_inside_the_transition", func() *timeline.Document {
		d := over(30, func(it *timeline.Item) { it.TransitionIn = fade() })
		put(d, clip("itm_base", 0, 35))

		return d
	}())
}

func init() {
	slotted := func(mod func(it *timeline.Item, d *timeline.Document)) *timeline.Document {
		d := newDoc()
		it := clip("itm_hero", 0, 90)
		s := timeline.NewSlot("hero", "media")
		it.Slot = &s
		mod(&it, d)
		put(d, it)

		return d
	}

	rc(CodeSlotUnfilled, "trigger_required_slot_open", slotted(func(*timeline.Item, *timeline.Document) {}))
	rc(CodeSlotUnfilled, "trigger_placeholder_asset", slotted(func(it *timeline.Item, _ *timeline.Document) {
		it.Asset = timeline.PlaceholderAsset
		it.Slot = nil
	}))
	rc(
		CodeSlotUnfilled,
		"quiet_slot_filled",
		slotted(func(it *timeline.Item, _ *timeline.Document) { it.Slot.Filled = true }),
	)
	rc(
		CodeSlotUnfilled,
		"quiet_slot_not_required",
		slotted(func(it *timeline.Item, _ *timeline.Document) { it.Slot.Required = false }),
	)
	rc(CodeSlotUnfilled, "quiet_optional_slot_on_placeholder", slotted(func(it *timeline.Item, _ *timeline.Document) {
		it.Asset = timeline.PlaceholderAsset
		it.Slot.Required = false
	}))
	rc(CodeSlotUnfilled, "quiet_template_document", slotted(func(_ *timeline.Item, d *timeline.Document) {
		d.Template = &timeline.TemplateInfo{Description: "t"}
	}))
	rc(CodeSlotUnfilled, "trigger_one_of_two_required_slots_filled", func() *timeline.Document {
		d := slotted(func(it *timeline.Item, _ *timeline.Document) { it.Slot.Filled = true })
		other := timeline.NewItem("itm_title", timeline.ItemText, trkText, 0, 60)
		s := timeline.NewSlot("title", "text")
		other.Slot = &s
		put(d, other)

		return d
	}())
}

func init() {
	long := func(frames int) *timeline.Document {
		d := newDoc()
		put(d, clip("itm_a", 0, frames))

		return d
	}

	rc(CodeDurationLimit, "trigger_5401_frames", long(5401))
	rc(CodeDurationLimit, "quiet_5400_frames", long(5400))
	rc(CodeDurationLimit, "trigger_one_hour", long(108000))
	rc(CodeDurationLimit, "quiet_landscape_project", func() *timeline.Document {
		d := long(5401)
		d.Settings.Width, d.Settings.Height = 1920, 1080

		return d
	}())
	rc(CodeDurationLimit, "trigger_square_project", func() *timeline.Document {
		d := long(5401)
		d.Settings.Width, d.Settings.Height = 1080, 1080

		return d
	}())
}

func init() {
	const id = "itm_s6t7v8w9x0"
	frame := func(w, h int) *timeline.Document {
		d := newDoc()
		d.Settings.Width, d.Settings.Height = w, h
		c := caption(id, 12, 48)
		put(d, c)

		return d
	}
	spec := func(x0, y0, x1, y1 float64) envSpec {
		return envSpec{Measure: map[string][4]float64{id: {x0, y0, x1, y1}}}
	}

	rc(CodeSafeArea, "trigger_example_of_the_sdd", frame(1080, 1920), spec(140, 1180, 940, 1330))
	rc(CodeSafeArea, "quiet_box_ends_at_1248", frame(1080, 1920), spec(140, 1098, 940, 1248))
	rc(CodeSafeArea, "trigger_box_ends_at_1249", frame(1080, 1920), spec(140, 1099, 940, 1249))
	rc(CodeSafeArea, "trigger_top_margin", frame(1080, 1920), spec(140, 100, 940, 250))
	rc(CodeSafeArea, "trigger_right_margin", frame(1080, 1920), spec(300, 600, 1030, 750))
	rc(CodeSafeArea, "quiet_landscape_inside_five_percent", frame(1920, 1080), spec(960, 540, 1800, 700))
	rc(CodeSafeArea, "trigger_four_five_uses_plan_safe_area", frame(1080, 1350), spec(140, 1130, 940, 1283))
	rc(CodeSafeArea, "quiet_four_five_inside", frame(1080, 1350), spec(140, 1130, 940, 1282))
}

func init() {
	const id = "itm_s6t7v8w9x0"
	text := func() *timeline.Document {
		d := newDoc()
		put(d, caption(id, 0, 60))

		return d
	}
	shape := func() *timeline.Document {
		d := newDoc()
		sh := timeline.NewItem("itm_sh", timeline.ItemShape, trkVideo, 0, 60)
		sh.Shape, sh.Width, sh.Height = "rect", 400, 400
		put(d, sh)

		return d
	}
	spec := func(id string, x0, y0, x1, y1 float64) envSpec {
		return envSpec{Measure: map[string][4]float64{id: {x0, y0, x1, y1}}}
	}

	rc(CodeOffFrame, "trigger_text_one_pixel_outside", text(), spec(id, 200, 500, 1081, 650))
	rc(CodeOffFrame, "quiet_text_inside", text(), spec(id, 200, 500, 1080, 650))
	rc(CodeOffFrame, "trigger_shape_wholly_outside", shape(), spec("itm_sh", 1100, 100, 1500, 500))
	rc(CodeOffFrame, "trigger_shape_more_than_half_outside", shape(), spec("itm_sh", 900, 100, 1300, 500))
	rc(CodeOffFrame, "quiet_shape_exactly_half_outside", shape(), spec("itm_sh", 880, 100, 1280, 500))
	rc(CodeOffFrame, "quiet_shape_slightly_outside", shape(), spec("itm_sh", 700, 100, 1100, 500))
	// VV-179: a shape wholly inside the frame (origin top-left, x and y the centre) is quiet.
	rc(CodeOffFrame, "quiet_shape_inside_the_frame", shape(), spec("itm_sh", 340, 760, 740, 1160))
	rc(CodeOffFrame, "quiet_box_covers_the_frame", shape(), spec("itm_sh", -600, -100, 1700, 2000))
	rc(CodeOffFrame, "quiet_without_a_measurer", text())
	rc(CodeOffFrame, "quiet_text_enters_by_a_slide", func() *timeline.Document {
		d := text()
		it := d.Items[id]
		it.TransitionIn = &timeline.Transition{Kind: "slide_left", Duration: 10}
		d.Items[id] = it

		return d
	}(), spec(id, 1100, 500, 1500, 650))
	rc(CodeOffFrame, "quiet_text_with_position_keyframes", func() *timeline.Document {
		d := text()
		it := d.Items[id]
		it.Keyframes = timeline.Keyframes{"x": {{T: 0, Value: 1500}, {T: 20, Value: 540}}}
		d.Items[id] = it

		return d
	}(), spec(id, 1100, 500, 1500, 650))
	rc(CodeOffFrame, "trigger_text_with_a_fade_only", func() *timeline.Document {
		d := text()
		it := d.Items[id]
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10}
		d.Items[id] = it

		return d
	}(), spec(id, 1100, 500, 1500, 650))
}

func init() {
	// A clip on trkVideo from frame 0 for 300 frames with a caption linked to it.
	linked := func(captionStart int) *timeline.Document {
		d := newDoc()
		put(d, clip("itm_clip", 0, 300))

		c := caption("itm_cap", captionStart, 40)
		c.LinkedItem = "itm_clip"
		c.Words = []timeline.Word{{Text: "Hello", Start: 0, Duration: 40}}
		put(d, c)

		return d
	}
	said := func(start int) envSpec {
		return envSpec{
			Transcript: map[string][]transcriptEntry{astVideo: {{Text: "hello", Start: start, End: start + 10}}},
		}
	}

	rc(CodeCaptionDesync, "trigger_caption_extends_beyond_its_clip", linked(280))
	rc(CodeCaptionDesync, "quiet_caption_inside_its_clip", linked(100))
	rc(CodeCaptionDesync, "quiet_first_word_3_frames_off", linked(103), said(100))
	rc(CodeCaptionDesync, "trigger_first_word_4_frames_off", linked(104), said(100))
	rc(CodeCaptionDesync, "quiet_without_a_transcript", linked(150))
}

// flat is n windows of the same peak.
func flat(n int, v float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}

	return out
}

func init() {
	// Two clips of one second on different tracks, both from the start.
	pair := func(volume float64) *timeline.Document {
		d := withVideoTrack(newDoc())
		a, b := clip("itm_a", 0, 30), onTrack(clip("itm_b", 0, 30), trkVideo2)
		a.Volume, b.Volume = volume, volume
		put(d, a)
		put(d, b)

		return d
	}
	peaks := func(v float64) envSpec { return envSpec{Peaks: map[string][]float64{astVideo: flat(50, v)}} }

	rc(CodeAudioClip, "trigger_two_sources_sum_above_1", pair(1), peaks(0.6))
	rc(CodeAudioClip, "quiet_volume_lowered", pair(0.5), peaks(0.6))
	rc(CodeAudioClip, "quiet_sum_exactly_1", pair(1), peaks(0.5))
	rc(CodeAudioClip, "quiet_without_peaks", pair(1))

	loud := func(lufs float64) envSpec { return envSpec{Loudness: map[string]float64{astVideo: lufs}} }
	one := func() *timeline.Document {
		d := newDoc()
		put(d, clip("itm_a", 0, 90))

		return d
	}

	rc(CodeLoudness, "quiet_6_lu_from_the_target", one(), loud(-20.0))
	rc(CodeLoudness, "trigger_6_1_lu_from_the_target", one(), loud(-20.1))
	rc(CodeLoudness, "trigger_too_loud", one(), loud(-5.0))
	rc(CodeLoudness, "quiet_without_loudness", one())
}

func (c ruleCase) dir() string { return filepath.Join("testdata", c.code, c.name) }

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// reportOf lints the document and renders the findings of one code as the lint text.
func reportOf(doc *timeline.Document, env Env, code string) string {
	fs := has(Lint(doc, env, Full), code)
	text, _ := Report(
		Header{
			Revision:  1,
			ProjectID: doc.ID,
			FPS:       timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)},
		},
		fs,
		0,
	)

	return text + "\n"
}

func TestWriteRuleGoldens(t *testing.T) {
	if !*update {
		t.Skip("run with -update to write the golden inputs and reports")
	}

	for _, c := range ruleCases {
		if err := os.MkdirAll(c.dir(), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(c.dir(), "doc.json"),
			append(timeline.Canonical(c.doc), '\n'),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		if len(c.env.Measure)+len(c.env.Peaks)+len(c.env.Loudness)+len(c.env.Transcript) > 0 {
			writeJSON(t, filepath.Join(c.dir(), "env.json"), c.env)
		}

		if err := os.WriteFile(
			filepath.Join(c.dir(), "want.txt"),
			[]byte(reportOf(c.doc, c.env.env(), c.code)),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
}

// Every case directory reports exactly its want.txt; trigger_ cases have a finding,
// quiet_ cases none, and every rule has both kinds.
func TestRuleGoldens(t *testing.T) {
	t.Parallel()

	codes, kinds := map[string]bool{}, map[string]map[string]bool{}

	entries, err := filepath.Glob(filepath.Join("testdata", "L_*", "*"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no golden cases: %v", err)
	}

	for _, dir := range entries {
		code, name := filepath.Base(filepath.Dir(dir)), filepath.Base(dir)

		t.Run(code+"/"+name, func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(filepath.Join(dir, "doc.json"))
			if err != nil {
				t.Fatal(err)
			}

			doc, err := timeline.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}

			var spec envSpec
			if b, err := os.ReadFile(filepath.Join(dir, "env.json")); err == nil {
				if err := json.Unmarshal(b, &spec); err != nil {
					t.Fatal(err)
				}
			}

			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}

			got := reportOf(doc, spec.env(), code)
			if got != string(want) {
				t.Errorf("report differs:\n--- got\n%s--- want\n%s", got, want)
			}

			if quiet := strings.HasPrefix(name, "quiet_"); quiet != strings.Contains(got, "errors=0 warnings=0") {
				t.Errorf(
					"%s: a %s case must %shave a finding",
					name,
					strings.Split(name, "_")[0],
					map[bool]string{true: "not ", false: ""}[quiet],
				)
			}
		})

		codes[code] = true
		if kinds[code] == nil {
			kinds[code] = map[string]bool{}
		}

		kinds[code][strings.SplitN(name, "_", 2)[0]] = true
	}

	for _, r := range Catalog() {
		if r.Phase != "A" {
			continue
		}

		if !codes[r.Code] || !kinds[r.Code]["trigger"] || !kinds[r.Code]["quiet"] {
			t.Errorf("%s needs a trigger and a quiet golden case", r.Code)
		}
	}

	if !slices.Contains(entries, filepath.Join("testdata", CodeDurationLimit, "trigger_5401_frames")) {
		t.Error("the boundary case of L_DURATION_LIMIT is missing")
	}
}
