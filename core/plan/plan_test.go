package plan

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	trkTop    = "trk_top0000001"
	trkBottom = "trk_bottom0001"
	trkSound  = "trk_sound00001"
	astVideo  = "ast_video000001"
	astImage  = "ast_image000001"
	astSound  = "ast_sound000001"
)

func newDoc() *timeline.Document {
	return &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion,
		ID:            "prj_7k2m9x4q1b",
		Settings:      timeline.DefaultSettings(),
		Assets: map[string]timeline.Asset{
			astVideo: {
				ID:         astVideo,
				Kind:       timeline.AssetVideo,
				FrameRate:  timeline.Rational{Num: 30, Den: 1},
				FrameCount: 900,
				Width:      1920,
				Height:     1080,
				HasAudio:   true,
			},
			astImage: {ID: astImage, Kind: timeline.AssetImage, Width: 1000, Height: 1000},
			astSound: {
				ID:         astSound,
				Kind:       timeline.AssetAudio,
				FrameRate:  timeline.Rational{Num: 30, Den: 1},
				FrameCount: 900,
			},
		},
		Tracks: map[string]timeline.Track{
			trkTop:    {ID: trkTop, Kind: timeline.TrackVisual, Order: "a0"},
			trkBottom: {ID: trkBottom, Kind: timeline.TrackVisual, Order: "a1"},
			trkSound:  {ID: trkSound, Kind: timeline.TrackAudio, Order: "a2"},
		},
		Items: map[string]timeline.Item{},
	}
}

func put(d *timeline.Document, it timeline.Item) { d.Items[it.ID] = it }

func clip(id, track string, start, dur int) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemClip, track, start, dur)
	it.Asset = astVideo

	return it
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > 0.0005 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestSafeAreaOfTheSDDExample(t *testing.T) {
	t.Parallel()

	got := SafeArea(1080, 1920)
	if want := (Rect{65, 269, 950, 979}); got != want {
		t.Errorf("SafeArea(1080, 1920) = %+v, want %+v (SDD-004 §4.1: [65, 269, 950, 979])", got, want)
	}
}

func TestSafeAreaMargins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		want          Rect
	}{
		{"vertical 9:16 edge of the 0.6 aspect", 600, 1000, Rect{36, 140, 528, 510}},
		{"just wider than 0.6", 601, 1000, Rect{30, 50, 541, 900}},
		{"landscape", 1920, 1080, Rect{96, 54, 1728, 972}},
		{"square", 1000, 1000, Rect{50, 50, 900, 900}},
		{"a degenerate frame", 100, 0, Rect{5, 0, 90, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SafeArea(tt.width, tt.height); got != tt.want {
				t.Errorf("SafeArea(%d, %d) = %+v, want %+v", tt.width, tt.height, got, tt.want)
			}
		})
	}
}

func TestEasing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		u    float64
		want float64
	}{
		{"linear", 0.25, 0.25},
		{"", 0.25, 0.25},
		{"ease_in", 0.5, 0.125},
		{"ease_out", 0.5, 0.875},
		{"ease_in_out", 0.25, 0.0625},
		{"ease_in_out", 0.5, 0.5},
		{"ease_in_out", 0.75, 0.9375},
		{"hold", 0.99, 0},
	}

	for _, tt := range tests {
		near(t, tt.name, timeline.Ease(tt.name, tt.u), tt.want)
	}
}

func TestKeyframesAtTheirBoundaries(t *testing.T) {
	t.Parallel()

	kfs := []timeline.Keyframe{
		{T: 10, Value: 1, Easing: "linear"},
		{T: 20, Value: 3, Easing: "hold"},
		{T: 30, Value: 5, Easing: "ease_in"},
	}

	tests := []struct {
		r    float64
		want float64
	}{
		{0, 1}, {10, 1}, {15, 2}, {19, 2.8}, {20, 3}, {25, 3}, {29, 3}, {30, 5}, {99, 5},
	}

	for _, tt := range tests {
		near(t, "keyframeValue", keyframeValue(kfs, tt.r), tt.want)
	}

	// Unsorted input is read in order of t, and the caller's slice is kept.
	unsorted := []timeline.Keyframe{{T: 30, Value: 5}, {T: 10, Value: 1}}
	near(t, "unsorted", keyframeValue(unsorted, 20), 3)

	if unsorted[0].T != 30 {
		t.Error("keyframeValue reordered the caller's slice")
	}

	// ease_in between the last two: u=0.5 gives 0.125 of the span.
	two := []timeline.Keyframe{{T: 0, Value: 0, Easing: "ease_in"}, {T: 10, Value: 8}}
	near(t, "ease_in mid", keyframeValue(two, 5), 1)
}

func TestTransitionsAtTheirEdges(t *testing.T) {
	t.Parallel()

	d := newDoc()
	// 100 frames, transitions of 4 frames each way on a frame of 1080x1920.
	kinds := []string{"fade", "slide_left", "slide_right", "slide_up", "slide_down"}

	for _, kind := range kinds {
		it := clip("itm_a", trkBottom, 0, 100)
		it.TransitionIn = &timeline.Transition{Kind: kind, Duration: 4}
		it.TransitionOut = &timeline.Transition{Kind: kind, Duration: 4}
		put(d, it)

		at := func(r int) animated { return evaluate(&it, r, 1080, 1920, 540, 960) }

		// First frame in: p = 1/5. Frame 3 (last in): p = 4/5. Frame 4: no transition.
		first, last, mid := at(0), at(3), at(4)
		s := func(p float64) float64 { return 1 - (1-p)*(1-p)*(1-p) }

		switch kind {
		case "fade":
			near(t, kind+" first", first.opacity, 0.2)
			near(t, kind+" last", last.opacity, 0.8)
			near(t, kind+" out first", at(96).opacity, 4.0/5)
			near(t, kind+" out last", at(99).opacity, 1.0/5)
		case "slide_left":
			near(t, kind+" first", first.x, 540-(1-s(0.2))*1080)
			near(t, kind+" out", at(99).x, 540-(1-s(0.2))*1080)
		case "slide_right":
			near(t, kind+" first", first.x, 540+(1-s(0.2))*1080)
		case "slide_up":
			near(t, kind+" first", first.y, 960-(1-s(0.2))*1920)
		case "slide_down":
			near(t, kind+" last", last.y, 960+(1-s(0.8))*1920)
		}

		if mid.opacity != 1 || mid.x != 540 || mid.y != 960 {
			t.Errorf("%s: frame 4 is outside the transition: %+v", kind, mid)
		}
	}

	// An unknown kind leaves the values alone, a missing transition too.
	it := clip("itm_b", trkBottom, 0, 10)
	it.TransitionIn = &timeline.Transition{Kind: "wipe", Duration: 3}

	if a := evaluate(&it, 0, 1080, 1920, 540, 960); a.opacity != 1 || a.x != 540 {
		t.Errorf("unknown transition changed the item: %+v", a)
	}
}

func TestSourceFrame(t *testing.T) {
	t.Parallel()

	fps30 := timeline.Rational{Num: 30, Den: 1}
	video := timeline.Asset{Kind: timeline.AssetVideo, FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 100}

	at := func(it timeline.Item, a timeline.Asset, fps timeline.Rational, r int) int {
		return sourceFrame(&it, &a, fps, r)
	}

	base := timeline.NewItem("itm_a", timeline.ItemClip, trkBottom, 0, 50)
	base.SourceIn = 10

	if got := at(base, video, fps30, 0); got != 10 {
		t.Errorf("first frame = %d, want source_in 10", got)
	}

	if got := at(base, video, fps30, 7); got != 17 {
		t.Errorf("frame 7 = %d, want 17", got)
	}

	fast := base
	fast.Speed = timeline.Rational{Num: 2, Den: 1}

	if got := at(fast, video, fps30, 7); got != 24 {
		t.Errorf("speed 2, frame 7 = %d, want 24", got)
	}

	slow := base
	slow.Speed = timeline.Rational{Num: 1, Den: 2}

	for r, want := range []int{10, 10, 11, 11, 12} {
		if got := at(slow, video, fps30, r); got != want {
			t.Errorf("speed 1/2, frame %d = %d, want %d (a frame is reused)", r, got, want)
		}
	}

	// A source at 25 fps in a 30 fps project: 30 project frames are 25 source frames.
	pal := video
	pal.FrameRate = timeline.Rational{Num: 25, Den: 1}

	if got := at(base, pal, fps30, 30); got != 33 {
		t.Errorf("25 fps source, frame 30 = %d, want floor(40*25/30) = 33", got)
	}

	// A 30000/1001 project on a 30000/1001 source is one to one.
	ntsc := timeline.Rational{Num: 30000, Den: 1001}
	video.FrameRate = ntsc

	if got := at(base, video, ntsc, 12); got != 22 {
		t.Errorf("30000/1001 project and source, frame 12 = %d, want 22", got)
	}

	// The last frame of the media holds.
	if got := at(base, video, ntsc, 49); got != 59 {
		t.Errorf("frame 49 = %d, want 59", got)
	}

	short := video
	short.FrameCount = 20

	if got := at(base, short, ntsc, 40); got != 19 {
		t.Errorf("past the end = %d, want frame_count-1 = 19", got)
	}

	img := timeline.Asset{Kind: timeline.AssetImage}
	if got := at(base, img, fps30, 30); got != 0 {
		t.Errorf("image = %d, want 0", got)
	}
}

func TestLayoutFit(t *testing.T) {
	t.Parallel()

	// A 1920x1080 asset in a 1080x1920 frame.
	tests := []struct {
		name       string
		fit        string
		scale      float64
		crop       *timeline.Crop
		wantW      float64
		wantH      float64
		wantCropW  float64
		wantCropXY [2]float64
	}{
		{"cover", "cover", 1, nil, 3413.333, 1920, 1920, [2]float64{0, 0}},
		{"contain", "contain", 1, nil, 1080, 607.5, 1920, [2]float64{0, 0}},
		{"none", "none", 1, nil, 1920, 1080, 1920, [2]float64{0, 0}},
		{"contain at half scale", "contain", 0.5, nil, 540, 303.75, 1920, [2]float64{0, 0}},
		{
			"cover with a crop to the middle",
			"cover",
			1,
			&timeline.Crop{Left: 0.25, Right: 0.25},
			1080,
			1920 * 1080 / 540 * 0.5 * 2 / 2 * 2,
			960,
			[2]float64{480, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := newDoc()
			it := clip("itm_a", trkBottom, 0, 30)
			it.Fit, it.Scale, it.Crop = tt.fit, tt.scale, tt.crop
			put(d, it)

			l := Frame(d, 0).Layers[0]
			if tt.name != "cover with a crop to the middle" {
				near(t, "W", l.W, tt.wantW)
				near(t, "H", l.H, tt.wantH)
			} else {
				// crop 960x1080 covering 1080x1920: b = max(1080/960, 1920/1080) = 1.7778.
				near(t, "W", l.W, 960*1920.0/1080)
				near(t, "H", l.H, 1920)
			}

			near(t, "crop w", l.Crop.W, tt.wantCropW)
			near(t, "crop x", l.Crop.X, tt.wantCropXY[0])
			near(t, "cx", l.CX, 540)
			near(t, "cy", l.CY, 960)
		})
	}
}

func TestMediaLayerFieldsAndAnimation(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 10, 60)
	x, y := 100.0, 200.0
	it.X, it.Y, it.Rotation, it.Opacity = &x, &y, 15, 0.5
	it.Keyframes = timeline.Keyframes{
		"opacity": {{T: 0, Value: 0, Easing: "linear"}, {T: 10, Value: 1, Easing: "linear"}},
	}
	put(d, it)

	l := Frame(d, 15).Layers[0]
	if l.Kind != Media || l.AssetID != astVideo || l.AssetKind != "video" || l.ItemID != "itm_a" {
		t.Errorf("layer = %+v", l)
	}

	near(t, "opacity from keyframes", l.Opacity, 0.5)
	near(t, "rotation", l.Rotation, 15)
	near(t, "cx", l.CX, 100)
	near(t, "cy", l.CY, 200)

	if l.SourceFrame != 5 || l.ColorMatrix != nil {
		t.Errorf("source frame %d, matrix %v; want 5 and nil", l.SourceFrame, l.ColorMatrix)
	}

	// A clip whose asset is gone draws nothing.
	delete(d.Assets, astVideo)

	if got := Frame(d, 15).Layers; len(got) != 0 {
		t.Errorf("a clip with a missing asset gave layers: %+v", got)
	}
}

func TestColorMatrix(t *testing.T) {
	t.Parallel()

	neutral := timeline.NewItem("itm_a", timeline.ItemClip, trkBottom, 0, 10)
	if colorMatrix(&neutral) != nil {
		t.Error("neutral adjustment must give nil")
	}

	it := neutral
	it.Brightness, it.Contrast, it.Saturation = 0.1, 1.2, 0.5

	m := colorMatrix(&it)
	if m == nil {
		t.Fatal("matrix is nil")
	}

	// Apply the matrix and the formulas of SDD-004 §3.5 to a colour.
	in := [3]float64{0.8, 0.4, 0.2}

	var cr [3]float64
	for i := range in {
		cr[i] = (in[i]-0.5)*it.Contrast + 0.5 + it.Brightness
	}

	l := 0.2126*cr[0] + 0.7152*cr[1] + 0.0722*cr[2]

	for i := range in {
		want := l + (cr[i]-l)*it.Saturation
		got := m[i*4]*in[0] + m[i*4+1]*in[1] + m[i*4+2]*in[2] + m[i*4+3]

		if math.Abs(got-want) > 0.002 {
			t.Errorf("channel %d: matrix gives %v, formula gives %v", i, got, want)
		}
	}

	d := newDoc()
	adj := clip("itm_b", trkBottom, 0, 10)
	adj.Brightness = 0.2
	put(d, adj)

	if Frame(d, 0).Layers[0].ColorMatrix == nil {
		t.Error("the layer lost its colour matrix")
	}
}

func TestColorMatrixWhiteBalanceAndHue(t *testing.T) {
	t.Parallel()

	apply := func(m *[12]float64, in [3]float64) [3]float64 {
		var out [3]float64
		for i := range out {
			out[i] = m[i*4]*in[0] + m[i*4+1]*in[1] + m[i*4+2]*in[2] + m[i*4+3]
		}

		return out
	}

	it := timeline.NewItem("itm_a", timeline.ItemClip, trkBottom, 0, 10)
	it.Temperature = 1

	m := colorMatrix(&it)
	if m == nil {
		t.Fatal("temperature alone must give a matrix")
	}

	near3 := func(got, want [3]float64) bool {
		return math.Abs(got[0]-want[0])+math.Abs(got[1]-want[1])+math.Abs(got[2]-want[2]) < 0.002
	}

	if got, want := apply(m, [3]float64{0.4, 0.4, 0.4}), [3]float64{0.5, 0.4, 0.3}; !near3(got, want) {
		t.Errorf("warm grey = %v, want %v", got, want)
	}

	it.Temperature, it.Tint = 0, 1
	if got, want := apply(colorMatrix(&it), [3]float64{0.4, 0.4, 0.4}), [3]float64{0.4, 0.3, 0.4}; !near3(got, want) {
		t.Errorf("magenta grey = %v, want %v", got, want)
	}

	it.Tint, it.Hue = 0, 120
	m = colorMatrix(&it)

	// A rotation about the grey axis keeps grey; the W3C coefficients turn red into
	// (-0.365, 0.443, -0.362) at 120 degrees, so green is the strongest channel.
	if got := apply(m, [3]float64{0.5, 0.5, 0.5}); !near3(got, [3]float64{0.5, 0.5, 0.5}) {
		t.Errorf("hue moved grey: %v", got)
	}

	if red := apply(m, [3]float64{1, 0, 0}); !near3(red, [3]float64{-0.365, 0.443, -0.362}) {
		t.Errorf("red rotated by 120 degrees = %v", red)
	}

	it.Hue = 0
	if colorMatrix(&it) != nil {
		t.Error("neutral values must give nil again")
	}
}

func TestLayerOrderHiddenTracksAndActiveRange(t *testing.T) {
	t.Parallel()

	d := newDoc()
	put(d, clip("itm_low", trkBottom, 0, 10))
	put(d, clip("itm_high", trkTop, 5, 10))
	put(d, clip("itm_next", trkBottom, 10, 10))

	ids := func(t int) []string {
		var out []string
		for _, l := range Frame(d, t).Layers {
			out = append(out, l.ItemID)
		}

		return out
	}

	tests := []struct {
		t    int
		want string
	}{
		{0, "itm_low"},
		{5, "itm_low,itm_high"}, // the track first in order is the top layer: drawn last
		{9, "itm_low,itm_high"},
		{10, "itm_next,itm_high"}, // an item ends before start+duration
		{14, "itm_next,itm_high"},
		{15, "itm_next"},
		{19, "itm_next"},
		{20, ""},
		{-1, ""},
	}

	for _, tt := range tests {
		if got := strings.Join(ids(tt.t), ","); got != tt.want {
			t.Errorf("frame %d: layers = %q, want %q", tt.t, got, tt.want)
		}
	}

	hidden := d.Tracks[trkTop]
	hidden.Hidden = true
	d.Tracks[trkTop] = hidden

	if got := strings.Join(ids(7), ","); got != "itm_low" {
		t.Errorf("hidden track still drawn: %q", got)
	}

	// An audio item or an audio track makes no layer.
	snd := timeline.NewItem("itm_snd", timeline.ItemAudio, trkSound, 0, 30)
	snd.Asset = astSound
	put(d, snd)

	if got := strings.Join(ids(7), ","); got != "itm_low" {
		t.Errorf("audio made a layer: %q", got)
	}

	fp := Frame(d, 3)
	if fp.Frame != 3 || fp.Width != 1080 || fp.Height != 1920 || fp.Background != "#000000" {
		t.Errorf("plan header = %+v", fp)
	}

	if empty := Frame(newDoc(), 0); empty.Layers == nil || len(empty.Layers) != 0 {
		t.Errorf("an empty frame has Layers = %#v, want an empty list", empty.Layers)
	}
}

// fakeMeasure is a stand-in for the rasterizer: 0.5 of the size per
// character, one line height per line.
func fakeMeasure(s BitmapSpec) (w, h float64) {
	lines := strings.Split(s.Text, "\n")
	longest := 0

	for _, l := range lines {
		longest = max(longest, len([]rune(l)))
	}

	return float64(longest) * s.Style.Size * 0.5, float64(len(lines)) * s.Style.Size * s.Style.LineHeight
}

func caption(start, dur int) timeline.Item {
	it := timeline.NewItem("itm_c", timeline.ItemCaption, trkTop, start, dur)
	it.Words = []timeline.Word{
		{Text: "Meet", Start: 0, Duration: 5},
		{Text: "the", Start: 8, Duration: 4},
		{Text: "editor", Start: 12, Duration: 6},
	}

	return it
}

func TestCaptionBottomEdge(t *testing.T) {
	t.Parallel()

	d := newDoc()
	put(d, caption(0, 30))

	l := FrameWith(d, 0, Options{Measure: fakeMeasure}).Layers[0]
	safe := SafeArea(1080, 1920)

	near(t, "bottom edge", l.CY+l.H/2, safe.Y+safe.H)
	near(t, "cx", l.CX, 540)

	if l.H == 0 || l.W == 0 {
		t.Errorf("size = %vx%v, want the measured size", l.W, l.H)
	}

	// Scale grows the box around the anchor, not away from it.
	scaled := d.Items["itm_c"]
	scaled.Scale = 2
	d.Items["itm_c"] = scaled

	l2 := FrameWith(d, 0, Options{Measure: fakeMeasure}).Layers[0]
	near(t, "scaled bottom edge", l2.CY+l2.H/2, safe.Y+safe.H)
	near(t, "scaled height", l2.H, 2*l.H)
}

// A caption with one word wider than max_width is scaled down to max_width instead of leaving the frame.
func TestCaptionLongWordFitsMaxWidth(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := timeline.NewItem("itm_c", timeline.ItemCaption, trkTop, 0, 30)
	it.Words = []timeline.Word{{Text: "www.americanoprogress.org/a-very-long-link-to-a-page", Start: 0, Duration: 30}}
	put(d, it)

	short := d.Items["itm_c"]
	short.Words = []timeline.Word{{Text: "hi", Start: 0, Duration: 30}}
	d2 := newDoc()
	put(d2, short)

	l := FrameWith(d, 0, Options{Measure: fakeMeasure}).Layers[0]
	ref := FrameWith(d2, 0, Options{Measure: fakeMeasure}).Layers[0]

	if l.W > 0.8*1080+40 {
		t.Errorf("width %v, want about max_width (864) plus the padding", l.W)
	}

	if l.CX-l.W/2 < 0 || l.CX+l.W/2 > 1080 {
		t.Errorf("the caption spans x %v to %v, outside the frame", l.CX-l.W/2, l.CX+l.W/2)
	}

	if ref.W >= l.W {
		t.Errorf("a short caption is %v wide, a long one %v", ref.W, l.W)
	}
}

func TestCaptionPositions(t *testing.T) {
	t.Parallel()

	safe := SafeArea(1080, 1920)
	opts := Options{Measure: fakeMeasure}

	place := func(position string) Layer {
		d := newDoc()
		c := caption(0, 30)
		x, y := 200.0, 300.0
		c.X, c.Y = &x, &y
		c.Style = &timeline.CaptionStyle{Position: &position}
		put(d, c)

		return FrameWith(d, 0, opts).Layers[0]
	}

	top, mid, custom := place("top"), place("middle"), place("custom")
	near(t, "top edge", top.CY-top.H/2, safe.Y)
	near(t, "top cx", top.CX, 540)
	near(t, "middle cy", mid.CY, 960)
	near(t, "custom cx", custom.CX, 200)
	near(t, "custom cy", custom.CY, 300)

	// Without a Measurer the caption is anchored by its centre on the edge.
	d := newDoc()
	put(d, caption(0, 30))

	l := Frame(d, 0).Layers[0]
	near(t, "unmeasured cy", l.CY, safe.Y+safe.H)

	if l.W != 0 || l.H != 0 {
		t.Errorf("unmeasured size = %vx%v, want 0", l.W, l.H)
	}
}

func TestCaptionStyleCascadeAndHighlight(t *testing.T) {
	t.Parallel()

	d := newDoc()
	d.Template = &timeline.TemplateInfo{Description: "x", Colors: map[string]timeline.Color{"brand": "#112233"}}

	pos, hl, size, weight := "top", "word", 80.0, 900
	track := d.Tracks[trkTop]
	track.CaptionStyle = &timeline.CaptionStyle{Position: &pos, Highlight: &hl}
	track.CaptionStyle.Size = &size
	d.Tracks[trkTop] = track

	c := caption(0, 30)
	brand := timeline.Color("$brand")
	c.Style = &timeline.CaptionStyle{HighlightColor: &brand}
	c.Style.FontWeight = &weight
	c.Style.Color = &brand
	c.Breaks = []int{2}
	put(d, c)

	spec := Frame(d, 0).Layers[0].Bitmap
	st := spec.Style

	if st.Size != 80 || st.FontWeight != 900 || st.Color != "#112233" || st.HighlightColor != "#112233" {
		t.Errorf("cascade: %+v", st)
	}

	if st.FontFamily != "Inter" || st.MaxWidth != 864 || st.Highlight != "word" || st.LineHeight != 1.2 ||
		st.Align != "center" {
		t.Errorf("defaults: %+v", st)
	}

	if spec.Kind != "caption" || spec.Text != "Meet the\neditor" || len(spec.Words) != 3 || spec.PixelScale != 1 {
		t.Errorf("spec = %+v", spec)
	}

	// The active word: the last word that started, while it lasts.
	for r, want := range map[int]int{0: 0, 4: 0, 5: -1, 7: -1, 8: 1, 11: 1, 12: 2, 17: 2, 18: -1, 29: -1} {
		if got := Frame(d, r).Layers[0].Bitmap.ActiveWord; got != want {
			t.Errorf("frame %d: active word %d, want %d", r, got, want)
		}
	}

	// With the default highlight "none" no word is active.
	plain := newDoc()
	put(plain, caption(0, 30))

	if got := Frame(plain, 0).Layers[0].Bitmap; got.ActiveWord != -1 || got.Style.Size != 64 ||
		got.Style.MaxWidth != 864 {
		t.Errorf(
			"default caption = active %d, size %v, max width %v",
			got.ActiveWord,
			got.Style.Size,
			got.Style.MaxWidth,
		)
	}
}

func TestTextAndShapeLayers(t *testing.T) {
	t.Parallel()

	d := newDoc()
	txt := timeline.NewItem("itm_t", timeline.ItemText, trkTop, 0, 30)
	txt.Text = "Hello"
	x := 100.0
	txt.X = &x
	txt.Scale = 2
	size, upper := 50.0, true
	txt.Style = &timeline.CaptionStyle{}
	txt.Style.Size, txt.Style.Uppercase = &size, &upper
	put(d, txt)

	shp := timeline.NewItem("itm_s", timeline.ItemShape, trkBottom, 0, 30)
	shp.Shape, shp.Width, shp.Height, shp.Radius = "rounded_rect", 200, 100, 12
	shp.Scale = 1.5
	put(d, shp)

	fp := FrameWith(d, 0, Options{Measure: fakeMeasure})
	if len(fp.Layers) != 2 {
		t.Fatalf("layers = %d, want 2", len(fp.Layers))
	}

	s, tx := fp.Layers[0], fp.Layers[1]
	if s.Bitmap.Kind != "shape" || s.Bitmap.Shape.Shape != "rounded_rect" || s.Bitmap.Shape.Fill != "#FFFFFF" {
		t.Errorf("shape = %+v", s.Bitmap.Shape)
	}

	near(t, "shape W", s.W, 300)
	near(t, "shape H", s.H, 150)
	near(t, "shape cx", s.CX, 540)
	near(t, "shape pixel scale", s.Bitmap.PixelScale, 1.5)

	if tx.Bitmap.Kind != "text" || tx.Bitmap.Text != "Hello" || tx.Bitmap.Style.Size != 50 ||
		!tx.Bitmap.Style.Uppercase {
		t.Errorf("text = %+v", tx.Bitmap)
	}

	near(t, "text cx", tx.CX, 100)
	near(t, "text cy", tx.CY, 960)
	near(t, "text W", tx.W, 2*5*50*0.5)
	near(t, "text pixel scale", tx.Bitmap.PixelScale, 2)
	near(t, "default text max width", tx.Bitmap.Style.MaxWidth, 0.86*1080)
}

func TestPixelScale(t *testing.T) {
	t.Parallel()

	it := timeline.NewItem("itm_t", timeline.ItemText, trkTop, 0, 30)

	for _, tt := range []struct{ scale, kf, want float64 }{
		{1, 0, 1}, {1.1, 0, 1.25}, {0.1, 0, 0.25}, {6, 0, 4}, {1, 2.3, 2.5},
	} {
		it.Scale = tt.scale
		it.Keyframes = nil

		if tt.kf > 0 {
			it.Keyframes = timeline.Keyframes{"scale": {{T: 0, Value: tt.kf}}}
		}

		if got := pixelScale(&it, 0); got != tt.want {
			t.Errorf("pixelScale(scale %v, keyframe %v) = %v, want %v", tt.scale, tt.kf, got, tt.want)
		}
	}
}

func TestBitmapKey(t *testing.T) {
	t.Parallel()

	a := BitmapSpec{Kind: "caption", Text: "x", ActiveWord: 0, PixelScale: 1}
	first, second := a.Key(), a.Key()
	if first != second || len(first) != 64 {
		t.Errorf("Key() = %q then %q, want 64 hex characters and stable", first, second)
	}

	b := a
	b.ActiveWord = 1

	c := a
	c.Style.Size = 10

	if a.Key() == b.Key() || a.Key() == c.Key() {
		t.Error("the key must change with the active word and the style")
	}
}

func TestWindowSeriesAndBitmaps(t *testing.T) {
	t.Parallel()

	d := newDoc()
	vid := clip("itm_v", trkBottom, 0, 20)
	vid.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 3}
	put(d, vid)

	spoken := caption(4, 20)
	hl := "word"
	spoken.Style = &timeline.CaptionStyle{Highlight: &hl}
	put(d, spoken)

	w := WindowWith(d, 0, 10, Options{Measure: fakeMeasure})
	if w.From != 0 || w.To != 10 || w.EndFrame != 24 || w.SafeArea != [4]float64{65, 269, 950, 979} || w.Width != 1080 {
		t.Errorf("header = %+v", w)
	}

	if len(w.Layers) != 2 || w.Layers[0].Item != "itm_v" || w.Layers[1].Item != "itm_c" {
		t.Fatalf("layers = %+v, want the clip then the caption (bottom first)", w.Layers)
	}

	v, c := w.Layers[0], w.Layers[1]
	if v.First != 0 || v.Last != 9 || len(v.SourceSeconds) != 10 || v.ColorMatrix == nil ||
		string(v.ColorMatrix) != "null" {
		t.Errorf("media layer = %+v", v)
	}

	if c.First != 4 || c.Last != 9 || len(c.Bitmaps.Index) != 6 || c.Natural[0] == 0 {
		t.Errorf("bitmap layer = %+v %+v", c, c.Bitmaps)
	}

	// Word 0 is active in frames 4-8 (relative 0-4) and none at 9 (relative 5): two keys.
	if len(c.Bitmaps.Keys) != 2 || c.Bitmaps.Index[0] != 0 || c.Bitmaps.Index[5] != 1 {
		t.Errorf("bitmaps = %+v", c.Bitmaps)
	}

	js, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}

	out := string(js)
	// Constant values are one number, the faded opacity is an array.
	for _, want := range []string{`"cx":540`, `"rotation":0`, `"opacity":[0.25,0.5,0.75,1,1,1,1,1,1,1]`, `"safe_area":[65,269,950,979]`, `"crop":[0,0,1920,1080]`} {
		if !strings.Contains(out, want) {
			t.Errorf("window JSON lacks %s:\n%s", want, out)
		}
	}

	if strings.Contains(out, `"asset_kind":""`) {
		t.Error("a bitmap layer must not carry an empty asset kind")
	}
}

func TestWindowBoundsAndEmptyProject(t *testing.T) {
	t.Parallel()

	empty := Window(newDoc(), 0, 50)
	if empty.EndFrame != 0 || len(empty.Layers) != 0 || len(empty.Audio) != 0 || empty.Layers == nil {
		t.Errorf("empty window = %+v", empty)
	}

	if w := Window(newDoc(), 10, 1000); w.To != 310 {
		t.Errorf("To = %d, want the window cut to 300 frames: 310", w.To)
	}

	if w := Window(newDoc(), -5, 3); w.From != 0 || w.To != 3 {
		t.Errorf("negative from: %d-%d", w.From, w.To)
	}

	if w := Window(newDoc(), 9, 4); w.To != 9 {
		t.Errorf("inverted window: %d-%d, want empty", w.From, w.To)
	}

	d := newDoc()
	put(d, clip("itm_v", trkBottom, 100, 10))

	if w := Window(d, 0, 50); len(w.Layers) != 0 || w.EndFrame != 110 {
		t.Errorf("window before the clip = %+v", w)
	}

	js, err := Series{}.MarshalJSON()
	if err != nil || string(js) != "[]" {
		t.Errorf("empty series = %s, %v", js, err)
	}
}

func TestWindowAudioGainPerFrame(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 10, 30)
	c.SourceIn = 30
	c.FadeIn = 4
	put(d, c)

	w := Window(d, 8, 16)
	if len(w.Audio) != 1 {
		t.Fatalf("audio = %+v", w.Audio)
	}

	a := w.Audio[0]
	if a.First != 10 || a.Last != 15 || a.Speed != 1 || a.SourceSeconds != 1 {
		t.Errorf("audio = %+v", a)
	}

	want := []float64{0, 0.25, 0.5, 0.75, 1, 1}
	for i, g := range want {
		near(t, "gain", a.Gain[i], g)
	}
}

func TestAudioSources(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 10, 30)
	c.SourceIn = 30
	c.Volume = 0.5
	put(d, c)

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = astSound
	put(d, m)

	got := Audio(d, 0, 60)
	if len(got) != 2 {
		t.Fatalf("sources = %+v", got)
	}

	// Bottom first: the audio track sorts after the video tracks in order.
	byID := map[string]AudioSource{}
	for _, s := range got {
		byID[s.ItemID] = s
	}

	v := byID["itm_v"]
	if v.StartSample != 16000 || v.SourceStart != 48000 || v.Samples != 48000 {
		t.Errorf(
			"clip source = %+v, want start 16000 (10 frames), source 48000 (30 frames), 48000 samples (30 frames)",
			v,
		)
	}

	if len(v.Gain) != 2 || v.Gain[0] != (GainPoint{0, 0.5}) || v.Gain[1] != (GainPoint{48000, 0.5}) {
		t.Errorf("flat gain = %+v", v.Gain)
	}

	// A window cuts the source to the frames asked for.
	cut := Audio(d, 20, 30)
	if len(cut) != 2 {
		t.Fatalf("cut = %+v", cut)
	}

	for _, s := range cut {
		if s.ItemID == "itm_v" && (s.StartSample != 32000 || s.SourceStart != 64000 || s.Samples != 16000) {
			t.Errorf("cut clip = %+v", s)
		}
	}

	if none := Audio(d, 100, 120); len(none) != 0 {
		t.Errorf("audio past the end = %+v", none)
	}
}

func TestMutedProducesNothing(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 10, 30)
	put(d, c)

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = astSound
	put(d, m)

	// Muted: the item, the track, a clip without audio.
	muted := d.Items["itm_v"]
	muted.AudioMuted = true
	d.Items["itm_v"] = muted

	tr := d.Tracks[trkSound]
	tr.Muted = true
	d.Tracks[trkSound] = tr

	if got := Audio(d, 0, 60); len(got) != 0 {
		t.Errorf("muted sources = %+v", got)
	}

	silent := clip("itm_q", trkBottom, 0, 10)
	silent.Asset = astImage
	d2 := newDoc()
	put(d2, silent)

	if got := Audio(d2, 0, 10); len(got) != 0 {
		t.Errorf("a clip without audio sounded: %+v", got)
	}
}

func TestAudioCarriesDenoise(t *testing.T) {
	t.Parallel()

	d := newDoc()
	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = astSound
	put(d, m)

	if got := Audio(d, 0, 60); len(got) != 1 || got[0].Denoise != 0 {
		t.Fatalf("an item with no denoise must give none: %+v", got)
	}

	m.Denoise = 0.6
	put(d, m)

	if got := Audio(d, 0, 60); len(got) != 1 || got[0].Denoise != 0.6 {
		t.Errorf("source lost the denoise: %+v", got)
	}

	if w := Window(d, 0, 10); len(w.Audio) != 1 || w.Audio[0].Denoise != 0.6 {
		t.Errorf("window audio lost the denoise: %+v", w.Audio)
	}
}

func TestAudioCarriesTheVoice(t *testing.T) {
	t.Parallel()

	d := newDoc()
	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = astSound
	put(d, m)

	if got := Audio(d, 0, 60); len(got) != 1 || got[0].Voice != "" {
		t.Fatalf("an item with no voice must give none: %+v", got)
	}

	m.Voice = "deep"
	put(d, m)

	if got := Audio(d, 0, 60); len(got) != 1 || got[0].Voice != "deep" {
		t.Errorf("source lost the voice: %+v", got)
	}

	if w := Window(d, 0, 10); len(w.Audio) != 1 || w.Audio[0].Voice != "deep" {
		t.Errorf("window audio lost the voice: %+v", w.Audio)
	}
}

func TestDisabledItemsLeaveThePlan(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 0, 30)
	put(d, c)

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = astSound
	put(d, m)

	if len(Frame(d, 5).Layers) != 1 || len(Window(d, 0, 10).Layers) != 1 || len(Audio(d, 0, 60)) != 2 {
		t.Fatal("the enabled items must be planned")
	}

	c.Disabled, m.Disabled = true, true
	put(d, c)
	put(d, m)

	if l := Frame(d, 5).Layers; len(l) != 0 {
		t.Errorf("a disabled clip is drawn: %+v", l)
	}

	if l := Window(d, 0, 10).Layers; len(l) != 0 {
		t.Errorf("a disabled clip is in the window: %+v", l)
	}

	if a := Audio(d, 0, 60); len(a) != 0 {
		t.Errorf("a disabled item sounds: %+v", a)
	}
}

func TestAudioEnvelopeFadesAndKeyframes(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 0, 30)
	c.FadeIn, c.FadeOut = 6, 6
	put(d, c)

	pts := Audio(d, 0, 30)[0].Gain
	// 0 at the start, 1 after the fade in (6 frames = 9600 samples), 1 before the fade out, 0 at the end.
	want := []GainPoint{{0, 0}, {9600, 1}, {38400, 1}, {48000, 0}}

	if len(pts) != len(want) {
		t.Fatalf("points = %+v, want %+v", pts, want)
	}

	for i := range want {
		if pts[i] != want[i] {
			t.Errorf("point %d = %+v, want %+v", i, pts[i], want[i])
		}
	}

	// Keyframed volume: ease_in is approximated by straight segments, hold by a step.
	k := clip("itm_k", trkTop, 0, 20)
	k.Keyframes = timeline.Keyframes{"volume": {
		{T: 0, Value: 0, Easing: "ease_in"}, {T: 10, Value: 1, Easing: "hold"}, {T: 20, Value: 0.5, Easing: "linear"},
	}}
	d2 := newDoc()
	put(d2, k)

	got := Audio(d2, 0, 20)[0].Gain

	for i := 1; i < len(got); i++ {
		if got[i].Sample <= got[i-1].Sample {
			t.Fatalf("envelope samples not increasing: %+v", got)
		}
	}

	if got[0] != (GainPoint{0, 0}) || got[len(got)-1] != (GainPoint{32000, 0.5}) {
		t.Errorf("envelope ends = %+v ... %+v", got[0], got[len(got)-1])
	}

	// Mid of the ease_in segment (frame 5): u = 0.5 gives 0.125.
	mid := 5 * 1600
	for _, p := range got {
		if p.Sample == int64(mid) {
			near(t, "ease_in at frame 5", p.Gain, 0.125)
		}
	}

	// The hold segment keeps 1 until one sample before frame 20... of its own span.
	var held bool

	for _, p := range got {
		if p.Sample == 20*1600-1 || p.Sample == 10*1600-1 {
			held = true
		}
	}

	if !held {
		t.Errorf("no point before the held keyframe edge: %+v", got)
	}
}

func TestSampleConversionAt2997(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30000, Den: 1001}
	// 30 frames at 30000/1001 are 1.001 s: 48048 samples.
	if got := frameSample(fps, 30); got != 48048 {
		t.Errorf("frameSample(30) = %d, want 48048", got)
	}

	if got := frameSample(fps, 1); got != 1601 {
		t.Errorf("frameSample(1) = %d, want floor(1601.6) = 1601", got)
	}
}

func loadScene(t *testing.T, name string) *timeline.Document {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "scenes", name+".json"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(data)
	if err != nil {
		t.Fatalf("scene %s: %v", name, err)
	}

	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Fatalf("scene %s is not a valid document: %v", name, vs)
	}

	return doc
}

// sceneGolden is what the golden files pin: a few frames and the first window.
type sceneGolden struct {
	Frames []FramePlan   `json:"frames"`
	Window WindowPlan    `json:"window"`
	Audio  []AudioSource `json:"audio"`
}

// SDD-004 §11, determinism: the plan of each reference scene is the same bytes
// on every operating system.
func TestReferenceScenesEqualTheirGoldenPlans(t *testing.T) {
	t.Parallel()

	scenes := map[string][]int{
		"clip-caption":  {0, 11, 12, 15, 30, 59, 60, 89},
		"two-clips-cut": {0, 59, 60, 61, 119},
		"contain-text":  {0, 10, 12, 40, 70, 79, 80},
	}

	for name, frames := range scenes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			doc := loadScene(t, name)
			opts := Options{Measure: fakeMeasure}

			g := sceneGolden{Window: WindowWith(doc, 0, 120, opts), Audio: Audio(doc, 0, 120)}
			for _, f := range frames {
				g.Frames = append(g.Frames, FrameWith(doc, f, opts))
			}

			got, err := json.MarshalIndent(g, "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			got = append(got, '\n')
			path := filepath.Join("testdata", name+".plan.json")

			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got, want) {
				t.Errorf("plan of %s differs from %s (run with -update after reviewing)", name, path)
			}
		})
	}
}

func scale200() *timeline.Document {
	d := newDoc()

	for i := range 100 {
		d.Tracks["trk_a"+string(rune('a'+i/26))+string(rune('a'+i%26))+"00000"] = timeline.Track{
			ID: "trk_a" + string(
				rune('a'+i/26),
			) + string(
				rune('a'+i%26),
			) + "00000",
			Kind:  timeline.TrackVisual,
			Order: "b" + string(rune('a'+i/26)) + string(rune('a'+i%26)),
		}
	}

	n := 0

	for id := range d.Tracks {
		if d.Tracks[id].Kind != timeline.TrackVisual {
			continue
		}

		for k := range 2 {
			it := clip("itm_"+id[4:]+string(rune('0'+k)), id, k*150, 150)
			it.Keyframes = timeline.Keyframes{"scale": {{T: 0, Value: 1}, {T: 100, Value: 1.5, Easing: "ease_in_out"}}}
			put(d, it)

			n++
		}
	}

	return d
}

func TestScale200HasTwoHundredItems(t *testing.T) {
	t.Parallel()

	if n := len(scale200().Items); n < 200 {
		t.Fatalf("items = %d, want at least 200", n)
	}
}

// BenchmarkWindow300 computes a 300-frame window of the 200-item project
// (SDD-004 §12; the budget is 20 ms).
func BenchmarkWindow300(b *testing.B) {
	d := scale200()

	b.ReportAllocs()

	for b.Loop() {
		Window(d, 0, 300)
	}
}

func TestLayerReportsInterpolatedScaleAndOpacity(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 40)
	it.Keyframes = timeline.Keyframes{
		"scale":   {{T: 10, Value: 1, Easing: "linear"}, {T: 20, Value: 2}},
		"opacity": {{T: 10, Value: 0.2, Easing: "linear"}, {T: 20, Value: 0.6}},
	}
	put(d, it)

	l := Frame(d, 15).Layers[0]
	near(t, "scale", l.Scale, 1.5)
	near(t, "opacity", l.Opacity, 0.4)

	// The scale reported is the one that sizes the layer.
	base := Frame(d, 10).Layers[0]
	near(t, "width ratio", l.W/base.W, l.Scale)

	w := Window(d, 10, 21).Layers[0]
	near(t, "window scale", w.Scale[5], 1.5)
	near(t, "window opacity", w.Opacity[5], 0.4)
}

// The placeholder of an unfilled optional slot is no audio source and no layer (SDD-004 §7):
// the export skips it.
func TestPlaceholderItemsAreSkipped(t *testing.T) {
	t.Parallel()

	d := newDoc()
	d.Template = &timeline.TemplateInfo{Description: "t", Instance: true}

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 60)
	m.Asset = timeline.PlaceholderAsset
	put(d, m)

	v := clip("itm_v", trkBottom, 0, 60)
	v.Asset = timeline.PlaceholderAsset
	put(d, v)

	if got := Audio(d, 0, 60); len(got) != 0 {
		t.Errorf("sources = %+v", got)
	}

	if got := Window(d, 0, 10); len(got.Layers) != 0 {
		t.Errorf("layers = %+v", got.Layers)
	}
}

// VV-036: the volume of a track is a factor on its items, solo keeps only the soloed tracks,
// and a solo visual track hides the picture of the others.
func TestTrackVolumeAndSolo(t *testing.T) {
	t.Parallel()

	d := newDoc()
	put(d, clip("itm_v", trkBottom, 0, 30))

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 30)
	m.Asset = astSound
	put(d, m)

	half := 0.5
	tr := d.Tracks[trkSound]
	tr.Volume = &half
	d.Tracks[trkSound] = tr

	for _, s := range Audio(d, 0, 30) {
		want := 1.0
		if s.ItemID == "itm_m" {
			want = 0.5
		}

		if g := s.Gain[0].Gain; g != want {
			t.Errorf("%s gain %v, want %v", s.ItemID, g, want)
		}
	}

	if w := Window(d, 0, 30); w.Audio[0].Item == "itm_m" && w.Audio[0].Gain[0] != 0.5 &&
		(len(w.Audio) < 2 || w.Audio[1].Gain[0] != 0.5) {
		t.Errorf("window gain ignores the track volume: %+v", w.Audio)
	}

	tr.Solo = true
	d.Tracks[trkSound] = tr

	if got := Audio(d, 0, 30); len(got) != 1 || got[0].ItemID != "itm_m" {
		t.Errorf("solo audio track: sources = %+v", got)
	}

	if len(Frame(d, 0).Layers) != 1 {
		t.Error("a solo audio track must not hide the picture")
	}

	top := timeline.NewItem("itm_t", timeline.ItemShape, trkTop, 0, 30)
	top.Shape, top.Width, top.Height = "rect", 10, 10
	put(d, top)

	st := d.Tracks[trkTop]
	st.Solo = true
	d.Tracks[trkTop] = st

	layers := Frame(d, 0).Layers
	if len(layers) != 1 || layers[0].ItemID != "itm_t" {
		t.Errorf("solo visual track: layers = %+v", layers)
	}
}

func TestSpeedCurveChoosesSourceFramesAndIsCarried(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 10, 4)
	c.SourceIn = 20
	c.SpeedCurve = []timeline.SpeedPoint{
		{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 3, Den: 1}},
	}
	c.Speed = timeline.CurveMean(c.SpeedCurve)
	put(d, c)

	// Offsets 0, 1, 3, 5 (SDD-004 section 3.11); the asset and the project share a frame rate.
	for r, off := range []int{0, 1, 3, 5} {
		l := Frame(d, 10+r).Layers
		if len(l) != 1 || l[0].SourceFrame != 20+off {
			t.Errorf("frame %d: want source frame %d, got %+v", r, 20+off, l)
		}
	}

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 4)
	m.Asset, m.SpeedCurve, m.Speed = astSound, c.SpeedCurve, c.Speed
	put(d, m)

	got := Audio(d, 2, 4)
	if len(got) != 1 || got[0].Curve == nil || got[0].Curve.ItemFrames != 4 || got[0].Curve.FirstFrame != 2 {
		t.Fatalf("the audio source does not carry the ramp: %+v", got)
	}

	if w := Window(d, 2, 4); len(w.Audio) != 1 || w.Audio[0].Curve == nil {
		t.Errorf("the window audio does not carry the ramp: %+v", w.Audio)
	}
}

func TestReverseChoosesBackwardsSourceFrames(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_v", trkBottom, 0, 4)
	c.SourceIn = 20
	c.Speed, c.Reverse = timeline.Rational{Num: 2, Den: 1}, true
	put(d, c)

	for r, want := range []int{27, 25, 23, 21} {
		if l := Frame(d, r).Layers; len(l) != 1 || l[0].SourceFrame != want {
			t.Errorf("frame %d: want source frame %d, got %+v", r, want, l)
		}
	}

	m := timeline.NewItem("itm_m", timeline.ItemAudio, trkSound, 0, 4)
	m.Asset, m.Speed, m.Reverse = astSound, c.Speed, true
	put(d, m)

	for _, a := range Audio(d, 0, 4) {
		if !a.Reverse {
			t.Errorf("the audio source does not carry reverse: %+v", a)
		}
	}
}
