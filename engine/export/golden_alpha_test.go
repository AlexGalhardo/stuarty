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
	"crop-animated",
	"crop-still",
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
	"color-keyframes",
	"grade-hsl",
	"grade-wheels",
	"grade-curves",
	"grade-all",
	"lut-warm",
	"lut-teal-half",
	"lut-with-grade",
	"speed-half",
	"speed-double",
	"fps-30000-1001",
	"source-fps-24",
	"filter-grayscale",
	"filter-sepia",
	"filter-invert",
	"filter-warm",
	"filter-cool",
	"blur-2",
	"blur-6",
	"blur-16",
	"blur-still",
	"sticker-png",
	"sticker-anim",
	"chroma-key",
	"blend-multiply",
	"blend-screen",
	"blend-overlay",
	"blend-add",
	"mask-rect",
	"mask-animated",
	"mask-ellipse-feather",
	"mask-invert-rotated",
	"flip-x",
	"flip-y",
	"flip-rotated-masked",
	"effect-motion-blur",
	"effect-glow",
	"effect-vignette",
	"effect-grain",
	"effect-rgb-split",
	"effects-combined",
	"transition-zoom",
	"transition-whip",
	"transition-dip",
	"transition-wipe-left",
	"transition-wipe-up",
	"transition-spin",
	"transition-shrink",
	"transition-grow",
	"shadow",
	"outline",
	"transparent",
	"opaque-control",
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

// image adds an image asset for the fixture: an animated one when the spec has frame delays.
func (b *caseBuilder) image(spec fixtureSpec) string {
	b.assets++
	id := fmt.Sprintf("ast_a%09d", b.assets)
	a := timeline.Asset{
		ID: id, Hash: "sha256:" + strings.Repeat("0", 64), Kind: timeline.AssetImage, Name: spec.Name,
		Width: spec.Width, Height: spec.Height,
	}
	for _, cs := range spec.DelaysCS {
		a.FrameDelays = append(a.FrameDelays, cs*10)
	}
	a.FrameCount = len(a.FrameDelays)
	b.doc.Assets[id] = a
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
	return transitionOf(timeline.Transition{Kind: kind, Duration: 8})
}

// transitionOf plays a transition on the entry and on the exit of a clip over the portrait clip (SDD-004
// section 3.2, TASK-0279 for zoom, whip, dip and wipe).
func transitionOf(tr timeline.Transition) alphaCase {
	b := newCase("transition "+tr.Kind, rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit = "contain"
		in, out := tr, tr
		it.TransitionIn, it.TransitionOut = &in, &out
	})
	// First, middle and last frame of the entry (duration 8) and of the exit (from frame 22).
	return b.build(0, 4, 7, 22, 26, 29)
}

// effectCase draws an effect on a layer that stays (the portrait clip) and on one that moves and rotates (a
// picture in picture sliding across it), both from the same clips (SDD-004 section 3.13, TASK-0279).
func effectCase(e timeline.Effect) alphaCase {
	b := newCase("effect "+e.Kind, rate(30, 1))
	b.clip(b.track(), b.video(portrait), 0, 30, func(it *timeline.Item) {
		it.Effects = []timeline.Effect{e}
	})
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale = "contain", 0.6
		it.Y = new(220.0)
		it.Keyframes = timeline.Keyframes{
			"x":        {timeline.NewKeyframe(0, 60), timeline.NewKeyframe(29, 120)},
			"rotation": {timeline.NewKeyframe(0, -12), timeline.NewKeyframe(29, 18)},
		}
		it.Effects = []timeline.Effect{e}
	})
	return b.build(0, 15, 29)
}

// combinedEffects is the five pixel effects and the shake on one item, with a whip on the way in and a dip
// on the way out, a mask and a flip: every stage of SDD-004 section 3.13 in one frame.
func combinedEffects() alphaCase {
	b := newCase("all the effects", rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.FlipX = "contain", 0.9, true
		it.X, it.Y = new(90.0), new(160.0)
		it.Mask = &timeline.Mask{Shape: "ellipse", X: 0.05, Y: 0.05, Width: 0.9, Height: 0.9, Feather: 6}
		it.TransitionIn = &timeline.Transition{Kind: "whip", Duration: 8, Direction: "down"}
		it.TransitionOut = &timeline.Transition{Kind: "dip", Duration: 8, Color: "#FF8000"}
		it.Effects = []timeline.Effect{
			{Kind: timeline.EffectMotionBlur, Amount: new(0.3), Angle: new(20.0)},
			{Kind: timeline.EffectGlow, Amount: new(0.5), Radius: new(6.0), Threshold: new(0.5)},
			{Kind: timeline.EffectVignette, Amount: new(0.6), Softness: new(0.5)},
			{Kind: timeline.EffectGrain, Amount: new(0.3), Size: new(2), Seed: new(7)},
			{Kind: timeline.EffectRGBSplit, Amount: new(0.3), Angle: new(-30.0)},
			{Kind: timeline.EffectShake, Amount: new(0.4), Speed: new(8), Seed: new(3)},
		}
	})
	return b.build(0, 4, 15, 26, 29)
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

// filterCase draws the filter on a layer that stays (the portrait clip) and on one that moves (a
// picture in picture sliding across it), both from the same clip, so a filter is checked on pixels
// that change place and on pixels that do not (SDD-004 section 3.6, P3-RND-003).
func filterCase(name string) alphaCase {
	b := newCase("filter "+name, rate(30, 1))
	b.clip(b.track(), b.video(portrait), 0, 30, func(it *timeline.Item) {
		it.Filter, it.FilterAmount = name, 1
	})
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.Filter, it.FilterAmount = "contain", 0.6, name, 1
		it.Y = new(220.0)
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 60), timeline.NewKeyframe(29, 120)}}
	})
	return b.build(0, 15, 29)
}

// blurCase blurs a picture in picture that slides across the portrait clip.
func blurCase(radius float64) alphaCase {
	b := newCase("blur", rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.Blur = "contain", 0.7, radius
		it.Y = new(160.0)
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 70), timeline.NewKeyframe(29, 110)}}
	})
	return b.build(0, 15, 29)
}

// blendCase draws two clips with a blend mode over the portrait clip, one at full opacity and one at half.
func blendCase(mode string) alphaCase {
	b := newCase("blend "+mode, rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.Blend = "contain", 0.6, mode
		it.X, it.Y = new(60.0), new(110.0)
	})
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.Blend, it.Opacity = "contain", 0.6, mode, 0.5
		it.X, it.Y = new(120.0), new(220.0)
	})
	return b.build(0, 15, 29)
}

// maskCase draws a clip through a mask over the portrait clip; edit sets what else the item has.
func maskCase(name string, m timeline.Mask, edit func(it *timeline.Item)) alphaCase {
	b := newCase("mask "+name, rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.Mask = "contain", 0.9, &m
		it.X, it.Y = new(90.0), new(160.0)
		if edit != nil {
			edit(it)
		}
	})
	return b.build(0, 15, 29)
}

// flipCase draws a clip mirrored over the portrait clip; edit sets what else the item has.
func flipCase(name string, fx, fy bool, edit func(it *timeline.Item)) alphaCase {
	b := newCase("flip "+name, rate(30, 1))
	b.backdrop(30)
	b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
		it.Fit, it.Scale, it.FlipX, it.FlipY = "contain", 0.9, fx, fy
		it.X, it.Y = new(90.0), new(160.0)
		if edit != nil {
			edit(it)
		}
	})
	return b.build(0, 15, 29)
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
	// A frozen source, so frame 0 and frame 10 differ only when each frame keeps its own crop. The window
	// keeps a constant size (the two sides swap), which is what makes a baked first crop identical.
	"crop-animated": func() alphaCase {
		b := newCase("an animated crop", rate(30, 1))
		b.clip(b.track(), b.video(portrait), 0, 11, func(it *timeline.Item) {
			it.Freeze, it.Fit = true, "none"
			it.Keyframes = timeline.Keyframes{
				"crop_left":  {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(10, 0.4)},
				"crop_right": {timeline.NewKeyframe(0, 0.4), timeline.NewKeyframe(10, 0)},
			}
		})
		return b.build(0, 10)
	},
	"crop-still": func() alphaCase {
		b := newCase("an animated crop of a still", rate(30, 1))
		b.clip(b.track(), b.image(fixtureSpec{Name: "gold-sticker.png", Width: 120, Height: 120, Sticker: true}), 0, 11,
			func(it *timeline.Item) {
				it.Fit = "none"
				it.Keyframes = timeline.Keyframes{
					"crop_left":  {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(10, 0.5)},
					"crop_right": {timeline.NewKeyframe(0, 0.5), timeline.NewKeyframe(10, 0)},
				}
			})
		return b.build(0, 10)
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
	"color-keyframes": colourKeyframesCase,
	"grade-hsl":       func() alphaCase { return gradeCase("grade hsl", gradeHSL) },
	"grade-wheels":    func() alphaCase { return gradeCase("grade wheels", gradeWheels) },
	"grade-curves":    func() alphaCase { return gradeCase("grade curves", gradeCurves) },
	"grade-all":       func() alphaCase { return gradeCase("grade all", gradeAll) },
	"lut-warm":        func() alphaCase { return lutCase("lut warm", "warm", 1, nil) },
	"lut-teal-half":   func() alphaCase { return lutCase("lut teal half", "teal", 0.5, nil) },
	"lut-with-grade":  func() alphaCase { return lutCase("lut with grade", "teal", 0.8, gradeAll) },
	"speed-half":      func() alphaCase { return speedCase("speed 0.5", rate(1, 2), 40, 0, 13, 26, 39) },
	"speed-double":    func() alphaCase { return speedCase("speed 2", rate(2, 1), 30, 0, 10, 20, 29) },
	"fps-30000-1001": func() alphaCase {
		b := newCase("a 30000/1001 project", rate(30000, 1001))
		b.clip(b.track(), b.video(portrait), 0, 60, nil)
		return b.build(0, 15, 30, 45, 59)
	},
	"filter-grayscale": func() alphaCase { return filterCase("grayscale") },
	"filter-sepia":     func() alphaCase { return filterCase("sepia") },
	"filter-invert":    func() alphaCase { return filterCase("invert") },
	"filter-warm":      func() alphaCase { return filterCase("warm") },
	"filter-cool":      func() alphaCase { return filterCase("cool") },
	"blur-2":           func() alphaCase { return blurCase(2) },
	"blur-6":           func() alphaCase { return blurCase(6) },
	"blur-16":          func() alphaCase { return blurCase(16) },
	"blur-still": func() alphaCase {
		b := newCase("a blurred still layer", rate(30, 1))
		b.clip(b.track(), b.video(portrait), 0, 30, func(it *timeline.Item) { it.Blur = 8 })
		return b.build(0, 15, 29)
	},
	"sticker-png": func() alphaCase {
		b := newCase("a PNG sticker with alpha over the portrait clip", rate(30, 1))
		b.backdrop(30)
		b.clip(b.track(), b.image(fixtureSpec{Name: "gold-sticker.png", Width: 120, Height: 120, Sticker: true}), 0, 30,
			func(it *timeline.Item) {
				it.Fit, it.Scale = "contain", 0.8
				it.X, it.Y = new(90.0), new(200.0)
			})
		return b.build(0, 15, 29)
	},
	"sticker-anim": func() alphaCase {
		b := newCase("an animated GIF sticker that grows, over the portrait clip", rate(30, 1))
		b.backdrop(30)
		spec := fixtureSpec{Name: "gold-sticker.gif", Width: 80, Height: 80, DelaysCS: []int{7, 3, 10}}
		b.clip(b.track(), b.image(spec), 0, 30, func(it *timeline.Item) {
			it.Fit = "contain"
			it.X, it.Y = new(90.0), new(200.0)
			it.Keyframes = timeline.Keyframes{"scale": {timeline.NewKeyframe(0, 0.6), timeline.NewKeyframe(29, 1.4)}}
		})
		// Item frames 0, 1, 2, 3, 5 and 6 choose the image frames 0, 0, 0, 2, 2, 0 of delays 70, 30 and 100 ms at 30 fps
		// (core/plan, AnimImageSourceFrame); 8, 10 and 29 are in later loops and choose 0, 2 and 2.
		return b.build(0, 1, 2, 3, 5, 6, 8, 10, 29)
	},
	"chroma-key": func() alphaCase {
		b := newCase("a clip keyed on its green over the portrait clip", rate(30, 1))
		b.backdrop(30)
		b.clip(b.track(), b.video(landscape), 0, 30, func(it *timeline.Item) {
			it.Fit = "contain"
			it.ChromaKey = &timeline.ChromaKey{Color: "#00FF00", Similarity: 0.35, Smoothness: 0.2, Spill: 0.5}
		})
		return b.build(0, 15, 29)
	},
	"blend-multiply": func() alphaCase { return blendCase("multiply") },
	"blend-screen":   func() alphaCase { return blendCase("screen") },
	"blend-overlay":  func() alphaCase { return blendCase("overlay") },
	"blend-add":      func() alphaCase { return blendCase("add") },
	"mask-rect": func() alphaCase {
		return maskCase("rect", timeline.Mask{Shape: "rect", X: 0.2, Y: 0.1, Width: 0.6, Height: 0.7}, nil)
	},
	// The source is frozen, so the two frames differ only when each frame draws its own mask.
	"mask-animated": func() alphaCase {
		b := newCase("an animated mask", rate(30, 1))
		b.clip(b.track(), b.video(portrait), 0, 11, func(it *timeline.Item) {
			it.Freeze = true
			it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 1}
			it.Keyframes = timeline.Keyframes{
				"mask_x": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(10, 0.5)},
			}
		})
		return b.build(0, 10)
	},
	"mask-ellipse-feather": func() alphaCase {
		return maskCase(
			"ellipse feather",
			timeline.Mask{Shape: "ellipse", X: 0.05, Y: 0.05, Width: 0.9, Height: 0.9, Feather: 12},
			nil,
		)
	},
	"mask-invert-rotated": func() alphaCase {
		return maskCase(
			"invert rotated",
			timeline.Mask{Shape: "ellipse", X: 0.25, Y: 0.15, Width: 0.5, Height: 0.7, Feather: 4, Invert: true},
			func(it *timeline.Item) {
				it.Scale, it.Rotation = 0.8, 20
			},
		)
	},
	"effect-motion-blur": func() alphaCase {
		return effectCase(timeline.Effect{Kind: timeline.EffectMotionBlur, Amount: new(0.5), Angle: new(20.0)})
	},
	"effect-glow": func() alphaCase {
		return effectCase(timeline.Effect{
			Kind: timeline.EffectGlow, Amount: new(0.7), Radius: new(8.0), Threshold: new(0.45),
		})
	},
	"effect-vignette": func() alphaCase {
		return effectCase(timeline.Effect{Kind: timeline.EffectVignette, Amount: new(0.8), Softness: new(0.5)})
	},
	"effect-grain": func() alphaCase {
		return effectCase(timeline.Effect{
			Kind: timeline.EffectGrain, Amount: new(0.3), Size: new(4), Seed: new(11),
		})
	},
	"effect-rgb-split": func() alphaCase {
		return effectCase(timeline.Effect{Kind: timeline.EffectRGBSplit, Amount: new(0.5), Angle: new(30.0)})
	},
	"effects-combined":  combinedEffects,
	"transition-zoom":   func() alphaCase { return transitionCase("zoom") },
	"transition-spin":   func() alphaCase { return transitionCase("spin") },
	"transition-shrink": func() alphaCase { return transitionCase("shrink") },
	"transition-grow":   func() alphaCase { return transitionCase("grow") },
	"transition-whip": func() alphaCase {
		return transitionOf(timeline.Transition{Kind: "whip", Duration: 8, Direction: "right"})
	},
	"transition-dip": func() alphaCase {
		return transitionOf(timeline.Transition{Kind: "dip", Duration: 8, Color: "#FF8000"})
	},
	"transition-wipe-left": func() alphaCase {
		return transitionOf(timeline.Transition{Kind: "wipe", Duration: 8, Direction: "left"})
	},
	"transition-wipe-up": func() alphaCase {
		return transitionOf(timeline.Transition{Kind: "wipe", Duration: 8, Direction: "up"})
	},
	"flip-x": func() alphaCase { return flipCase("x", true, false, nil) },
	"flip-y": func() alphaCase { return flipCase("y", false, true, nil) },
	"flip-rotated-masked": func() alphaCase {
		return flipCase("rotated masked", true, true, func(it *timeline.Item) {
			it.Scale, it.Rotation = 0.8, 20
			it.Mask = &timeline.Mask{Shape: "rect", X: 0.1, Y: 0.1, Width: 0.5, Height: 0.6, Feather: 4}
		})
	},
	"source-fps-24": func() alphaCase {
		b := newCase("a source at 24 fps in a 30 fps project", rate(30, 1))
		b.clip(b.track(), b.video(portrait24), 0, 60, nil)
		return b.build(0, 7, 22, 45, 59)
	},
	// One frame of a still with alpha, centred, so the pad is on the picture and the corner stays the background.
	"shadow": func() alphaCase {
		b := newCase("a sticker with a drop shadow", rate(30, 1))
		b.doc.Settings.Background = "#202020"
		b.clip(b.track(), b.image(fixtureSpec{Name: "gold-sticker.png", Width: 120, Height: 120, Sticker: true}), 0, 1,
			func(it *timeline.Item) {
				it.Fit = "none"
				it.X, it.Y = new(90.0), new(160.0)
				it.Shadow = &timeline.Shadow{Color: "#00AAFF", Opacity: 0.8, DX: 8, DY: 6, Blur: 4}
			})
		return b.build(0)
	},
	"outline": func() alphaCase {
		b := newCase("a sticker with an outline", rate(30, 1))
		b.doc.Settings.Background = "#202020"
		b.clip(b.track(), b.image(fixtureSpec{Name: "gold-sticker.png", Width: 120, Height: 120, Sticker: true}), 0, 1,
			func(it *timeline.Item) {
				it.Fit = "none"
				it.X, it.Y = new(90.0), new(160.0)
				it.Outline = &timeline.Outline{Color: "#FFFFFF", Width: 3}
			})
		return b.build(0)
	},
	// A landscape clip contained in the portrait frame leaves the corner uncovered. Fit none would
	// spill past the frame and the export would scale only the visible window, whose pixels the
	// parity harness cannot place. The picture is fully opaque, so the PNG round trip keeps every
	// byte (a soft alpha edge would not: the encoder stores straight alpha).
	"transparent": func() alphaCase {
		return uncoveredClip("a clip on a transparent frame", timeline.BackgroundTransparent)
	},
	"opaque-control": func() alphaCase {
		return uncoveredClip("a clip on an opaque frame", "#202020")
	},
}

func uncoveredClip(name, background string) alphaCase {
	b := newCase(name, rate(30, 1))
	b.doc.Settings.Background = timeline.Color(background)
	b.clip(b.track(), b.video(landscape), 0, 1, func(it *timeline.Item) { it.Fit = "contain" })
	return b.build(0)
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

// readPNG decodes a reference into the pixels of an RGBA frame.
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

// SDD-004 section 12 rule 3 on the blur and filter cases: 0 differing bytes between 1 and 8 bands.
func TestCompositorBandsEqualBlurAndFilter(t *testing.T) {
	setup(t)
	for _, name := range requiredCases {
		if !strings.HasPrefix(name, "blur-") && !strings.HasPrefix(name, "filter-") &&
			!strings.HasPrefix(name, "sticker-") && !strings.HasPrefix(name, "chroma-") &&
			!strings.HasPrefix(name, "blend-") && !strings.HasPrefix(name, "mask-") &&
			!strings.HasPrefix(name, "crop-") &&
			!strings.HasPrefix(name, "shadow") && !strings.HasPrefix(name, "outline") &&
			!strings.HasPrefix(name, "transparent") && !strings.HasPrefix(name, "opaque") &&
			!strings.HasPrefix(
				name,
				"flip-",
			) && !strings.HasPrefix(name, "grade-") && !strings.HasPrefix(name, "lut-") && !strings.HasPrefix(name, "effect") &&
			!strings.HasPrefix(name, "transition-") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := goldenDir(name)
			doc := loadDir(t, dir)
			frames := readFrames(t, dir)
			one := renderDoc(t, doc, name, 1, frames)
			eight := renderDoc(t, doc, name, 8, frames)
			for i, f := range frames {
				if !bytes.Equal(one[i], eight[i]) {
					t.Fatalf("frame %d differs between 1 band and 8 bands", f)
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

// colourKeyframesCase animates brightness, saturation and hue on one clip, so every frame has its own colour
// matrix (SDD-004 section 3.5, animated colour): the planner gives each frame its matrix and both renderers draw it.
func colourKeyframesCase() alphaCase {
	b := newCase("animated colour", rate(30, 1))
	b.clip(b.track(), b.video(portrait), 0, 30, func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{
			"brightness": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(29, 0.3)},
			"saturation": {timeline.NewKeyframe(0, 1), timeline.NewKeyframe(29, 0.2)},
			"hue":        {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(29, 120)},
		}
	})
	return b.build(0, 10, 20, 29)
}

// The grades of SDD-004 section 3.14, set on the item as clip.set_color stores them.
func gradeHSL(it *timeline.Item) {
	it.Hsl = []timeline.HslBand{
		{Range: "red", Hue: 0.6, Saturation: 0.3},
		{Range: "yellow", Hue: -0.5, Lightness: 0.3},
		{Range: "blue", Saturation: 0.8, Lightness: -0.2},
		{Range: "green", Hue: 1, Saturation: -0.5},
	}
}

func gradeWheels(it *timeline.Item) {
	it.Wheels = &timeline.Wheels{
		Shadows:    &timeline.Wheel{Hue: 220, Amount: 0.5, Luma: -0.1},
		Midtones:   &timeline.Wheel{Hue: 30, Amount: 0.25, Luma: 0.05},
		Highlights: &timeline.Wheel{Hue: 50, Amount: 0.6, Luma: 0.2},
	}
}

func gradeCurves(it *timeline.Item) {
	it.Curves = &timeline.Curves{
		Master: []timeline.CurvePoint{{X: 0, Y: 0.08}, {X: 0.4, Y: 0.3}, {X: 0.75, Y: 0.85}, {X: 1, Y: 1}},
		Red:    []timeline.CurvePoint{{X: 0, Y: 0}, {X: 0.5, Y: 0.62}, {X: 1, Y: 1}},
		Blue:   []timeline.CurvePoint{{X: 0, Y: 0.15}, {X: 1, Y: 0.85}},
	}
}

func gradeAll(it *timeline.Item) {
	gradeHSL(it)
	gradeWheels(it)
	gradeCurves(it)
	it.Brightness, it.Filter, it.FilterAmount = 0.1, "warm", 0.5
}

// gradeCase draws a grade on a layer that stays and on a picture in picture that slides and rotates, both from
// the same clip, so the grade is checked on pixels that change place and on pixels that do not.
func gradeCase(name string, set func(*timeline.Item)) alphaCase {
	b := newCase(name, rate(30, 1))
	asset := b.video(portrait)
	b.clip(b.track(), asset, 0, 30, set)
	b.clip(b.track(), asset, 0, 30, func(it *timeline.Item) {
		set(it)
		it.Scale, it.Rotation = 0.5, 12
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 40), timeline.NewKeyframe(29, 140)}}
	})
	return b.build(0, 15, 29)
}
