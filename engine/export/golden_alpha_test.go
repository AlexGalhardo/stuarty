package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The golden cases of the ALPHA scenes (SDD-004 section 11, P2-RND-004). Each case is a folder of
// testdata/golden holding the document (project.json), the description of its generated media
// (fixtures.json), the list of the composed frames (frames.json) and one frame-<n>.png for each of
// them. The test composes the frames and compares the SHA-256 of the pixels with that of the PNG.
// The folders are written by go test ./engine/export -run Alpha -update from the builders below; every
// regeneration is a reviewed change.

// requiredCases are the scenes of SDD-004 section 11 that this suite adds to the three reference scenes.
var requiredCases = []string{
	"contain-background",
	"crop",
	"pip-opacity",
	"rotation",
	"transition-fade",
	"transition-slide-left",
	"transition-slide-right",
	"transition-slide-up",
	"transition-slide-down",
	"scale-linear",
	"scale-ease-in",
	"scale-ease-out",
	"scale-ease-in-out",
	"scale-hold",
	"caption-highlight",
	"text-style",
	"shape",
	"color-adjust",
	"speed-half",
	"speed-double",
	"fps-30000-1001",
	"source-fps-24",
}

const (
	goldenW = 180
	goldenH = 320
)

var (
	portrait   = fixtureSpec{Name: "gold-portrait.mp4", Width: 180, Height: 320, FPS: 30, Seconds: 3}
	landscape  = fixtureSpec{Name: "gold-landscape.mp4", Width: 320, Height: 180, FPS: 30, Seconds: 3}
	portrait24 = fixtureSpec{Name: "gold-portrait24.mp4", Width: 180, Height: 320, FPS: 24, Seconds: 3}
)

// alphaCase is what a builder returns.
type alphaCase struct {
	doc      *timeline.Document
	fixtures map[string]fixtureSpec
	frames   []int
}

// caseBuilder makes the document of a case with ids from counters, so that the documents are the same
// on every run.
type caseBuilder struct {
	doc      *timeline.Document
	fixtures map[string]fixtureSpec
	assets   int
	tracks   int
	items    int
}

func newCase(name string, fps timeline.Rational) *caseBuilder {
	st := timeline.DefaultSettings()
	st.Width, st.Height, st.FPS = goldenW, goldenH, fps
	return &caseBuilder{
		doc: &timeline.Document{
			SchemaVersion: timeline.CurrentSchemaVersion,
			ID:            "prj_gg00000000",
			Name:          name,
			Settings:      st,
			Assets:        map[string]timeline.Asset{},
			Tracks:        map[string]timeline.Track{},
			Items:         map[string]timeline.Item{},
		},
		fixtures: map[string]fixtureSpec{},
	}
}

// video adds an asset for the fixture; the hash and the facts are filled in when the case is loaded.
func (b *caseBuilder) video(spec fixtureSpec) string {
	b.assets++
	id := fmt.Sprintf("ast_a%09d", b.assets)
	b.doc.Assets[id] = timeline.Asset{
		ID: id, Hash: "sha256:" + strings.Repeat("0", 64), Kind: timeline.AssetVideo, Name: spec.Name,
		FrameRate:  timeline.Rational{Num: spec.FPS, Den: 1},
		FrameCount: spec.FPS * int(spec.Seconds), SampleCount: 44100 * int(spec.Seconds),
		Width: spec.Width, Height: spec.Height, HasAudio: true,
	}
	b.fixtures[id] = spec
	return id
}

// track adds a visual track above the ones before it: the smaller order is drawn on top.
func (b *caseBuilder) track() string {
	b.tracks++
	id := fmt.Sprintf("trk_t%09d", b.tracks)
	b.doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Order: fmt.Sprintf("a%02d", 50-b.tracks)}
	return id
}

// add puts the item in the document.
func (b *caseBuilder) add(typ timeline.ItemType, track string, start, dur int, set func(*timeline.Item)) {
	b.items++
	id := fmt.Sprintf("itm_i%09d", b.items)
	it := timeline.NewItem(id, typ, track, start, dur)
	if set != nil {
		set(&it)
	}
	b.doc.Items[id] = it
}

// clip adds a clip of the asset.
func (b *caseBuilder) clip(track, asset string, start, dur int, set func(*timeline.Item)) {
	b.add(timeline.ItemClip, track, start, dur, func(it *timeline.Item) {
		it.Asset = asset
		if set != nil {
			set(it)
		}
	})
}

func (b *caseBuilder) build(frames ...int) alphaCase {
	return alphaCase{doc: b.doc, fixtures: b.fixtures, frames: frames}
}

func rate(n, d int) timeline.Rational { return timeline.Rational{Num: n, Den: d} }

// backdrop is a portrait clip under the rest of a case.
func (b *caseBuilder) backdrop(dur int) {
	b.clip(b.track(), b.video(portrait), 0, dur, nil)
}

func transitionCase(kind string) alphaCase {
	b := newCase("transition "+kind, rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit = "contain"
		it.TransitionIn = &timeline.Transition{Kind: kind, Duration: 8}
		it.TransitionOut = &timeline.Transition{Kind: kind, Duration: 8}
	})
	// First, middle and last frame of the entry (duration 8) and of the exit (from frame 22).
	return b.build(0, 4, 7, 22, 26, 29)
}

func scaleCase(easing string) alphaCase {
	b := newCase("scale "+easing, rate(30, 1))
	b.clip(b.track(), b.video(portrait), 0, 30, func(it *timeline.Item) {
		k0 := timeline.NewKeyframe(0, 0.3)
		k0.Easing = easing
		it.Keyframes = timeline.Keyframes{"scale": {k0, timeline.NewKeyframe(20, 1)}}
	})
	return b.build(0, 5, 10, 15, 20)
}

func speedCase(name string, speed timeline.Rational, dur int, frames ...int) alphaCase {
	b := newCase(name, rate(30, 1))
	b.clip(b.track(), b.video(portrait), 0, dur, func(it *timeline.Item) { it.Speed = speed })
	return b.build(frames...)
}

// alphaCases are the builders of the cases, by folder name.
var alphaCases = map[string]func() alphaCase{
	"contain-background": func() alphaCase {
		b := newCase("contain with a background", rate(30, 1))
		b.doc.Settings.Background = "#1E3A8A"
		b.clip(b.track(), b.video(landscape), 0, 60, func(it *timeline.Item) { it.Fit = "contain" })
		return b.build(0, 30, 59)
	},
	"crop": func() alphaCase {
		b := newCase("crop", rate(30, 1))
		b.clip(b.track(), b.video(portrait), 0, 40, func(it *timeline.Item) {
			it.Crop = &timeline.Crop{Left: 0.25, Top: 0.1, Right: 0.25, Bottom: 0.3}
		})
		return b.build(0, 20, 39)
	},
	"pip-opacity": func() alphaCase {
		b := newCase("picture in picture", rate(30, 1))
		b.backdrop(30)
		b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
			it.Fit, it.Scale, it.Opacity = "contain", 0.7, 0.6
			it.X, it.Y = new(100.0), new(220.0)
		})
		return b.build(0, 15, 29)
	},
	"rotation": func() alphaCase {
		b := newCase("rotation", rate(30, 1))
		b.doc.Settings.Background = "#202020"
		b.clip(b.track(), b.video(portrait), 0, 31, func(it *timeline.Item) {
			it.Fit, it.Scale = "contain", 0.5
			it.Keyframes = timeline.Keyframes{"rotation": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(30, 90)}}
		})
		return b.build(0, 10, 20, 30)
	},
	"transition-fade":        func() alphaCase { return transitionCase("fade") },
	"transition-slide-left":  func() alphaCase { return transitionCase("slide_left") },
	"transition-slide-right": func() alphaCase { return transitionCase("slide_right") },
	"transition-slide-up":    func() alphaCase { return transitionCase("slide_up") },
	"transition-slide-down":  func() alphaCase { return transitionCase("slide_down") },
	"scale-linear":           func() alphaCase { return scaleCase("linear") },
	"scale-ease-in":          func() alphaCase { return scaleCase("ease_in") },
	"scale-ease-out":         func() alphaCase { return scaleCase("ease_out") },
	"scale-ease-in-out":      func() alphaCase { return scaleCase("ease_in_out") },
	"scale-hold":             func() alphaCase { return scaleCase("hold") },
	"caption-highlight": func() alphaCase {
		b := newCase("caption with word highlight", rate(30, 1))
		b.backdrop(60)
		b.add(timeline.ItemCaption, b.track(), 0, 60, func(it *timeline.Item) {
			it.Words = []timeline.Word{
				{Text: "Meet", Start: 0, Duration: 20},
				{Text: "the", Start: 20, Duration: 20},
				{Text: "editor", Start: 40, Duration: 20},
			}
			it.Style = &timeline.CaptionStyle{
				Size: new(26.0), Color: new(timeline.Color("#FFFFFF")),
				Highlight: new("word"), HighlightColor: new(timeline.Color("#FFD400")),
			}
		})
		// One frame inside each word.
		return b.build(10, 30, 50)
	},
	"text-style": func() alphaCase {
		b := newCase("text with stroke, shadow and background", rate(30, 1))
		b.backdrop(30)
		b.add(timeline.ItemText, b.track(), 0, 30, func(it *timeline.Item) {
			it.Text = "Stylish"
			it.Style = &timeline.CaptionStyle{
				Size: new(30.0), Color: new(timeline.Color("#FFFFFF")),
				StrokeColor: new(timeline.Color("#E11D48")), StrokeWidth: new(2.0),
				ShadowColor: new(timeline.Color("#000000")), ShadowDX: new(3.0), ShadowDY: new(3.0),
				ShadowBlur:      new(2.0),
				BackgroundColor: new(timeline.Color("#003366")), BackgroundPadding: new(8.0),
				BackgroundRadius: new(6.0),
			}
		})
		return b.build(0, 15, 29)
	},
	"shape": func() alphaCase {
		b := newCase("shapes", rate(30, 1))
		b.doc.Settings.Background = "#101820"
		for _, s := range []struct {
			kind string
			x, y float64
		}{{"rect", 60, 70}, {"rounded_rect", 120, 160}, {"ellipse", 70, 250}} {
			b.add(timeline.ItemShape, b.track(), 0, 20, func(it *timeline.Item) {
				it.Shape, it.Width, it.Height, it.Radius = s.kind, 90, 60, 14
				it.Fill, it.StrokeColor, it.StrokeWidth = "#E11D48", "#FFFFFF", 3
				it.X, it.Y = new(s.x), new(s.y)
			})
		}
		return b.build(0, 10, 19)
	},
	"color-adjust": func() alphaCase {
		b := newCase("colour adjustment", rate(30, 1))
		track, asset := b.track(), b.video(portrait)
		b.clip(track, asset, 0, 10, func(it *timeline.Item) { it.Brightness = 0.25 })
		b.clip(track, asset, 10, 10, func(it *timeline.Item) { it.SourceIn, it.Contrast = 10, 1.6 })
		b.clip(track, asset, 20, 10, func(it *timeline.Item) { it.SourceIn, it.Saturation = 20, 0.2 })
		return b.build(5, 15, 25)
	},
	"speed-half":   func() alphaCase { return speedCase("speed 0.5", rate(1, 2), 40, 0, 13, 26, 39) },
	"speed-double": func() alphaCase { return speedCase("speed 2", rate(2, 1), 30, 0, 10, 20, 29) },
	"fps-30000-1001": func() alphaCase {
		b := newCase("a 30000/1001 project", rate(30000, 1001))
		b.clip(b.track(), b.video(portrait), 0, 60, nil)
		return b.build(0, 15, 30, 45, 59)
	},
	"source-fps-24": func() alphaCase {
		b := newCase("a source at 24 fps in a 30 fps project", rate(30, 1))
		b.clip(b.track(), b.video(portrait24), 0, 60, nil)
		return b.build(0, 7, 22, 45, 59)
	},
}

func goldenDir(name string) string { return filepath.Join("testdata", "golden", name) }

func framePNG(dir string, f int) string { return filepath.Join(dir, "frame-"+strconv.Itoa(f)+".png") }

func sum(pix []byte) string {
	s := sha256.Sum256(pix)
	return hex.EncodeToString(s[:])
}

// readFrames is the list of frames of a case folder.
func readFrames(tb testing.TB, dir string) []int {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "frames.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var frames []int
	if err := json.Unmarshal(raw, &frames); err != nil {
		tb.Fatal(err)
	}
	return frames
}

// readPNG decodes a reference into the pixels of an opaque RGBA frame.
func readPNG(tb testing.TB, path string) []byte {
	tb.Helper()
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	im, err := png.Decode(f)
	if err != nil {
		tb.Fatalf("%s: %v", path, err)
	}
	rgba := image.NewRGBA(im.Bounds())
	draw.Draw(rgba, rgba.Bounds(), im, im.Bounds().Min, draw.Src)
	return rgba.Pix
}

// writeCase writes the folder of a case, with the references composed now.
func writeCase(t *testing.T, name string, c alphaCase) {
	t.Helper()
	dir := goldenDir(name)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(file string, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, file), append(bytes.TrimSpace(data), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("project.json", timeline.Canonical(c.doc))
	fixtures, err := json.MarshalIndent(c.fixtures, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write("fixtures.json", fixtures)
	frames, err := json.Marshal(c.frames)
	if err != nil {
		t.Fatal(err)
	}
	write("frames.json", frames)

	doc := loadDir(t, dir)
	for i, pix := range renderDoc(t, doc, name, 0, c.frames) {
		if err := writePNG(framePNG(dir, c.frames[i]), doc.Settings.Width, doc.Settings.Height, pix); err != nil {
			t.Fatal(err)
		}
	}
}

// alphaHashes is the SHA-256 of every listed frame of every case, one line each.
func alphaHashes(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, name := range requiredCases {
		dir := goldenDir(name)
		frames := readFrames(t, dir)
		for i, pix := range renderDoc(t, loadDir(t, dir), name, 0, frames) {
			out = append(out, name+" "+strconv.Itoa(frames[i])+" "+sum(pix))
		}
	}
	return out
}

// Every scene of SDD-004 section 11 has its folder, and the folder has its files.
func TestAlphaCasesAreComplete(t *testing.T) {
	for _, name := range requiredCases {
		dir := goldenDir(name)
		for _, file := range []string{"project.json", "fixtures.json", "frames.json"} {
			if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
				t.Errorf("case %s: %v", name, err)
			}
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "frames.json"))
		if err != nil {
			continue
		}
		var frames []int
		if err := json.Unmarshal(raw, &frames); err != nil || len(frames) == 0 {
			t.Errorf("case %s: frames.json holds no frames (%v)", name, err)
		}
		for _, f := range frames {
			if _, err := os.Stat(framePNG(dir, f)); err != nil {
				t.Errorf("case %s: %v", name, err)
			}
		}
	}
	for name := range alphaCases {
		if !slices.Contains(requiredCases, name) {
			t.Errorf("the builder %s is not in requiredCases", name)
		}
	}
	for _, name := range requiredCases {
		if alphaCases[name] == nil {
			t.Errorf("requiredCases lists %s, which has no builder", name)
		}
	}
}

// SDD-004 section 11, NFR-010: the composed frames of every case are the bytes of their references.
func TestGoldenAlphaCases(t *testing.T) {
	setup(t)
	for _, name := range requiredCases {
		t.Run(name, func(t *testing.T) {
			if *update {
				writeCase(t, name, alphaCases[name]())
			}
			dir := goldenDir(name)
			doc := loadDir(t, dir)
			frames := readFrames(t, dir)
			for i, pix := range renderDoc(t, doc, name, 0, frames) {
				want := readPNG(t, framePNG(dir, frames[i]))
				if got := sum(pix); got != sum(want) {
					t.Errorf("frame %d: %s, reference %s", frames[i], got, sum(want))
					saveMismatch(t, name, frames[i], doc.Settings.Width, doc.Settings.Height, pix)
				}
			}
		})
	}
}

const alphaHashesEnv = "VUKEM_VIDEO_TEST_ALPHA_HASHES"

// TestAlphaHashesChild is the second process of TestGoldenAlphaTwice.
func TestAlphaHashesChild(t *testing.T) {
	if os.Getenv(alphaHashesEnv) == "" {
		t.Skip("only runs as the child of TestGoldenAlphaTwice")
	}
	for _, line := range alphaHashes(t) {
		_, _ = os.Stdout.WriteString("HASH " + line + "\n")
	}
}

// NFR-010: two runs in one process and one in another give the same hash for every frame of every case.
func TestGoldenAlphaTwice(t *testing.T) {
	setup(t)
	first, second := alphaHashes(t), alphaHashes(t)
	if !slices.Equal(first, second) {
		t.Fatal("two runs in one process differ")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestAlphaHashesChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), alphaHashesEnv+"=1")
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("the second process failed: %v", err)
	}
	var other []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "HASH "); ok {
			other = append(other, rest)
		}
	}
	if !slices.Equal(first, other) {
		t.Fatalf("the hashes of a second process differ (%d against %d lines)", len(other), len(first))
	}
}

// Section 8 on each case with libopenh264: the file passes the verification (Run fails otherwise) and
// the listed frames, decoded, look like the composed frames (SSIM of at least 0.95 on luma, section 11).
func TestExportAlphaCases(t *testing.T) {
	setup(t)
	if testing.Short() {
		t.Skip("one export per case")
	}
	for _, name := range requiredCases {
		t.Run(name, func(t *testing.T) {
			dir := goldenDir(name)
			doc := loadDir(t, dir)
			frames := readFrames(t, dir)
			job := newJob(t, doc, "export-"+name, name+".mp4")
			res, err := Run(context.Background(), job)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(res.Path) }()
			for _, f := range frames {
				got := decodeFrame(t, res.Path, doc, f)
				s := ssim(luma(readPNG(t, framePNG(dir, f))), luma(got), doc.Settings.Width, doc.Settings.Height)
				if s < 0.95 {
					t.Errorf("frame %d: SSIM %.4f against the composed frame, want at least 0.95", f, s)
				}
			}
		})
	}
}
