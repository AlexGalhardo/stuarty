package timeline

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

const (
	clipID    = "itm_m1n2p3q4r5"
	capItemID = "itm_s6t7v8w9x0"
	capTrack  = "trk_a1b2c3d4e5"
	vidTrack  = "trk_f6g7h8j9k0"
	assetID   = "ast_3c5v8n2w6d"
)

func base(t *testing.T) *Document {
	t.Helper()
	return mustDecode(t, readFile(t, "testdata/valid/launch-teaser.json"))
}

func editItem(d *Document, key string, f func(*Item)) {
	it := d.Items[key]
	f(&it)
	d.Items[key] = it
}

func editAsset(d *Document, f func(*Asset)) {
	a := d.Assets[assetID]
	f(&a)
	d.Assets[assetID] = a
}

func editCaptionTrack(d *Document, f func(*Track)) {
	tr := d.Tracks[capTrack]
	f(&tr)
	d.Tracks[capTrack] = tr
}

func addItem(d *Document, it Item) { d.Items[it.ID] = it }

func text(d *Document, id string, start int, st *CaptionStyle) {
	it := NewItem(id, ItemText, capTrack, start, 10)
	it.Text = "hi"
	it.Style = st
	addItem(d, it)
}

type invalidCase struct {
	name string
	edit func(d *Document)
	want string // the one code expected
}

func TestValidateRejectsEveryInvariant(t *testing.T) {
	manyWords := func(n int) []Word {
		w := make([]Word, n)
		for i := range w {
			w[i] = Word{Text: "w", Start: i, Duration: 1}
		}
		return w
	}
	cases := []invalidCase{
		// V_SCHEMA_VERSION
		{"schema newer", func(d *Document) { d.SchemaVersion = 2 }, VSchemaVersion},
		{"schema zero", func(d *Document) { d.SchemaVersion = 0 }, VSchemaVersion},

		// V_ID
		{"project id", func(d *Document) { d.ID = "project" }, VID},
		{"asset id form", func(d *Document) { editAsset(d, func(a *Asset) { a.ID = "x" }) }, VID},
		{"asset id key", func(d *Document) { editAsset(d, func(a *Asset) { a.ID = "ast_0000000000" }) }, VID},
		{"track id form", func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.ID = "x" }) }, VID},
		{
			"track id key",
			func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.ID = "trk_0000000000" }) },
			VID,
		},
		{"item id key", func(d *Document) { editItem(d, clipID, func(i *Item) { i.ID = "itm_0000000000" }) }, VID},
		{"item track form", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Track = "x" }) }, VID},
		{"item asset form", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Asset = "x" }) }, VID},
		{"linked item form", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.LinkedItem = "x" }) }, VID},

		// V_SHAPE: document, settings, assets, tracks
		{"name empty", func(d *Document) { d.Name = "" }, VShape},
		{"name long", func(d *Document) { d.Name = strings.Repeat("é", 121) }, VShape},
		{"width small", func(d *Document) { d.Settings.Width = 8 }, VShape},
		{"height odd", func(d *Document) { d.Settings.Height = 1921 }, VShape},
		{"fps den", func(d *Document) { d.Settings.FPS = Rational{2997, 100} }, VShape},
		{"fps high", func(d *Document) { d.Settings.FPS = Rational{121, 1} }, VShape},
		{"fps low", func(d *Document) {
			d.Settings.FPS = Rational{1000, 1001}
			editItem(d, clipID, func(i *Item) { i.Duration, i.SourceIn = 10, 0 })
		}, VShape},
		{"loudness", func(d *Document) { d.Settings.LoudnessTarget = 0 }, VShape},
		{"background", func(d *Document) { d.Settings.Background = "red" }, VShape},
		{"background ref", func(d *Document) { d.Settings.Background = "$accent" }, VColorRef},
		{"asset hash", func(d *Document) { editAsset(d, func(a *Asset) { a.Hash = "sha256:ABC" }) }, VShape},
		{"asset kind", func(d *Document) { editAsset(d, func(a *Asset) { a.Kind = "gif" }) }, VShape},
		{"video no rate", func(d *Document) { editAsset(d, func(a *Asset) { a.FrameRate = Rational{} }) }, VShape},
		{"video no frames", func(d *Document) { editAsset(d, func(a *Asset) { a.FrameCount = 0 }) }, VShape},
		{"video no size", func(d *Document) { editAsset(d, func(a *Asset) { a.Width = 0 }) }, VShape},
		{"video samples", func(d *Document) { editAsset(d, func(a *Asset) { a.SampleCount = -1 }) }, VShape},
		{"audio no samples", func(d *Document) {
			editAsset(d, func(a *Asset) { a.Kind, a.SampleCount = AssetAudio, 0 })
			delete(d.Items, clipID)
		}, VShape},
		{
			"image no size",
			func(d *Document) { editAsset(d, func(a *Asset) { a.Kind, a.Width = AssetImage, 0 }) },
			VShape,
		},
		{"track kind", func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.Kind = "x" }) }, VShape},
		{
			"track name",
			func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.Name = strings.Repeat("n", 61) }) },
			VShape,
		},
		{
			"caption style size",
			func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.CaptionStyle.Size = new(1.0) }) },
			VShape,
		},

		// V_TRACK_ORDER
		{
			"order duplicate",
			func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.Order = "a1" }) },
			VTrackOrder,
		},
		{"order empty", func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.Order = "" }) }, VTrackOrder},
		{
			"order alphabet",
			func(d *Document) { editCaptionTrack(d, func(tr *Track) { tr.Order = "A!" }) },
			VTrackOrder,
		},

		// V_ITEM_TRACK
		{
			"track missing",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.Track = "trk_zzzzzzzzzz" }) },
			VItemTrack,
		},
		{"audio on visual", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Type = ItemAudio })
		}, VItemTrack},
		{"clip on audio track", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			editItem(d, clipID, func(i *Item) { i.Track = "trk_k000000001" })
		}, VItemTrack},

		// V_ITEM_RANGE
		{"start negative", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Start = -1 }) }, VItemRange},
		{"duration zero", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Duration = 0 })
		}, VItemRange},
		{
			"past the end",
			func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Start = MaxFrames - 10 }) },
			VItemRange,
		},
		{"item type", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Type = "video" }) }, VShape},
		{
			"item name",
			func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Name = strings.Repeat("n", 121) }) },
			VShape,
		},

		// V_OVERLAP
		{"overlap", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Track = vidTrack; i.Start = 100 })
		}, VOverlap},

		// V_ASSET_REF
		{
			"asset missing",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.Asset = "ast_zzzzzzzzzz" }) },
			VAssetRef,
		},
		{"asset empty", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Asset = "" }) }, VShape},
		{
			"placeholder outside template",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.Asset = PlaceholderAsset }) },
			VAssetRef,
		},
		{"clip of audio asset", func(d *Document) { editAsset(d, func(a *Asset) { a.Kind = AssetAudio }) }, VAssetRef},
		{"audio of silent video", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			editAsset(d, func(a *Asset) { a.HasAudio = false })
			addItem(d, NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 10))
			editItem(d, "itm_a000000001", func(i *Item) { i.Asset = assetID })
		}, VAssetRef},
		{"audio of image", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			editAsset(d, func(a *Asset) { a.Kind = AssetImage })
			addItem(d, NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 10))
			editItem(d, "itm_a000000001", func(i *Item) { i.Asset = assetID })
			delete(d.Items, clipID)
		}, VAssetRef},

		// V_SOURCE_RANGE
		{"source range", func(d *Document) { editItem(d, clipID, func(i *Item) { i.SourceIn = 800 }) }, VSourceRange},
		{"source range with speed", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Speed = Rational{4, 1}; i.Duration = 200; i.SourceIn = 150 })
		}, VSourceRange},
		{"audio source range", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			d.Assets["ast_k000000001"] = Asset{
				ID:          "ast_k000000001",
				Hash:        d.Assets[assetID].Hash,
				Kind:        AssetAudio,
				Name:        "a",
				SampleCount: 48000,
			}
			a := NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 31)
			a.Asset = "ast_k000000001"
			addItem(d, a)
		}, VSourceRange},

		// V_SPEED, V_FADES
		{"speed fast", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Speed = Rational{8, 1}; i.Duration = 100; i.SourceIn = 0 })
		}, VSpeed},
		{"speed slow", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Speed = Rational{1, 5} }) }, VSpeed},
		{"speed zero", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Speed = Rational{0, 1} }) }, VShape},
		{"fades", func(d *Document) { editItem(d, clipID, func(i *Item) { i.FadeIn, i.FadeOut = 100, 51 }) }, VFades},
		{"fade negative", func(d *Document) { editItem(d, clipID, func(i *Item) { i.FadeIn = -1 }) }, VShape},
		{"source_in negative", func(d *Document) { editItem(d, clipID, func(i *Item) { i.SourceIn = -1 }) }, VShape},
		{"volume", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Volume = 5 }) }, VShape},

		// V_SHAPE: visual and clip fields
		{"scale zero", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Scale = 0 }) }, VShape},
		{"scale big", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Scale = 21 }) }, VShape},
		{"opacity", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Opacity = 2 }) }, VShape},
		{"document marker order", func(d *Document) { d.Markers = []Marker{{Frame: 9}, {Frame: 9}} }, VMarkers},
		{"document marker frame", func(d *Document) { d.Markers = []Marker{{Frame: MaxFrames}} }, VMarkers},
		{"document marker label", func(d *Document) {
			d.Markers = []Marker{{Frame: 1, Label: strings.Repeat("é", 61)}}
		}, VShape},
		{"blend mode", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Blend = "dodge" })
		}, VShape},
		{"mask shape", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Mask = &Mask{Shape: "star", Width: 1, Height: 1} })
		}, VShape},
		{"mask outside the item", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Mask = &Mask{Shape: "rect", X: 0.6, Width: 0.6, Height: 1} })
		}, VShape},
		{"mask feather", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Mask = &Mask{Shape: "rect", Width: 1, Height: 1, Feather: 201} })
		}, VShape},
		{"chroma key colour", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.ChromaKey = &ChromaKey{Color: "green", Similarity: 0.3} })
		}, VShape},
		{"chroma key similarity", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.ChromaKey = &ChromaKey{Color: "#00FF00", Similarity: 1.5} })
		}, VShape},
		{"chroma key on audio", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			a := NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 10)
			a.Asset, a.ChromaKey = assetID, &ChromaKey{Color: "#00FF00"}
			addItem(d, a)
		}, VShape},
		{"rotation NaN", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Rotation = math.NaN() }) }, VShape},
		{"x infinite", func(d *Document) { editItem(d, clipID, func(i *Item) { i.X = new(math.Inf(1)) }) }, VShape},
		{"fit", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Fit = "stretch" }) }, VShape},
		{"crop range", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Crop = &Crop{Left: 1.5} }) }, VShape},
		{
			"crop sum",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.Crop = &Crop{Left: 0.6, Right: 0.5} }) },
			VShape,
		},
		{"brightness", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Brightness = 2 }) }, VShape},
		{"contrast", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Contrast = 3 }) }, VShape},
		{"saturation", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Saturation = -1 }) }, VShape},
		{"denoise", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Denoise = 1.5 }) }, VShape},
		{"voice", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Voice = "robot" }) }, VShape},
		{"temperature", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Temperature = 1.5 }) }, VShape},
		{"tint", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Tint = -1.5 }) }, VShape},
		{"hue", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Hue = 181 }) }, VShape},

		// Fields of another item type.
		{"text on clip", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Text = "x" }) }, VShape},
		{"words on clip", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Words = manyWords(1) }) }, VShape},
		{"crop on caption", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Crop = &Crop{} }) }, VShape},
		{"asset on caption", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Asset = assetID }) }, VShape},
		{"shape on clip", func(d *Document) { editItem(d, clipID, func(i *Item) { i.Width = 5 }) }, VShape},
		{
			"style on clip",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.Style = &CaptionStyle{} }) },
			VShape,
		},
		{"transition on audio", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			a := NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 10)
			a.Asset = assetID
			a.TransitionIn = &Transition{Kind: "fade", Duration: 1}
			addItem(d, a)
		}, VShape},

		// V_TRANSITIONS
		{"transitions sum", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				i.TransitionIn, i.TransitionOut = &Transition{
					Kind:     "fade",
					Duration: 100,
				}, &Transition{
					Kind:     "slide_up",
					Duration: 51,
				}
			})
		}, VTransitions},
		{"transition kind", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.TransitionIn = &Transition{Kind: "bounce", Duration: 5} })
		}, VShape},
		{"transition colour on a fade", func(d *Document) {
			editItem(
				d,
				clipID,
				func(i *Item) { i.TransitionIn = &Transition{Kind: "fade", Duration: 5, Color: "#FF0000"} },
			)
		}, VShape},
		{"transition colour not a colour", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.TransitionIn = &Transition{Kind: "dip", Duration: 5, Color: "red"} })
		}, VShape},
		{"transition direction on a dip", func(d *Document) {
			editItem(
				d,
				clipID,
				func(i *Item) { i.TransitionIn = &Transition{Kind: "dip", Duration: 5, Direction: "up"} },
			)
		}, VShape},
		{"transition direction unknown", func(d *Document) {
			editItem(
				d,
				clipID,
				func(i *Item) { i.TransitionIn = &Transition{Kind: "wipe", Duration: 5, Direction: "sideways"} },
			)
		}, VShape},
		{"effect kind", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Effects = []Effect{{Kind: "sparkle"}} })
		}, VShape},
		{"effect parameter of another kind", func(d *Document) {
			a, r := 0.5, 10.0
			editItem(d, clipID, func(i *Item) {
				i.Effects = []Effect{{Kind: EffectVignette, Amount: &a, Softness: &a, Radius: &r}}
			})
		}, VShape},
		{"effect parameter missing", func(d *Document) {
			a := 0.5
			editItem(d, clipID, func(i *Item) { i.Effects = []Effect{{Kind: EffectVignette, Amount: &a}} })
		}, VShape},
		{"effect parameter out of range", func(d *Document) {
			a, s := 1.5, 0.5
			editItem(
				d,
				clipID,
				func(i *Item) { i.Effects = []Effect{{Kind: EffectVignette, Amount: &a, Softness: &s}} },
			)
		}, VShape},
		{"effect twice", func(d *Document) {
			a, s := 0.5, 0.5
			e := Effect{Kind: EffectVignette, Amount: &a, Softness: &s}
			editItem(d, clipID, func(i *Item) { i.Effects = []Effect{e, e} })
		}, VShape},
		{
			"transition duration",
			func(d *Document) { editItem(d, clipID, func(i *Item) { i.TransitionOut = &Transition{Kind: "fade"} }) },
			VShape,
		},

		// V_KEYFRAMES
		{"keyframe property", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"foo": {NewKeyframe(0, 1)}} })
		}, VKeyframes},
		{"keyframe t past end", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"scale": {NewKeyframe(151, 1)}} })
		}, VKeyframes},
		{"keyframe t negative", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"x": {NewKeyframe(-1, 1)}} })
		}, VKeyframes},
		{"keyframe bezier without numbers", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				k := NewKeyframe(0, 1)
				k.Easing = EasingBezier
				i.Keyframes = Keyframes{"scale": {k}}
			})
		}, VKeyframes},
		{"keyframe bezier x out of range", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				k := NewKeyframe(0, 1)
				k.Easing, k.Bezier = EasingBezier, []float64{1.5, 0, 0.5, 1}
				i.Keyframes = Keyframes{"scale": {k}}
			})
		}, VKeyframes},
		{"keyframe bezier y out of range", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				k := NewKeyframe(0, 1)
				k.Easing, k.Bezier = EasingBezier, []float64{0.5, 3.5, 0.5, 1}
				i.Keyframes = Keyframes{"scale": {k}}
			})
		}, VKeyframes},
		{"keyframe bezier with a named easing", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				k := NewKeyframe(0, 1)
				k.Easing, k.Bezier = "ease_in", []float64{0.5, 0, 0.5, 1}
				i.Keyframes = Keyframes{"scale": {k}}
			})
		}, VKeyframes},
		{"keyframe order", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"y": {NewKeyframe(5, 1), NewKeyframe(5, 2)}} })
		}, VKeyframes},
		{"keyframe count", func(d *Document) {
			kfs := make([]Keyframe, 101)
			for i := range kfs {
				kfs[i] = NewKeyframe(i, 1)
			}
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"rotation": kfs} })
		}, VKeyframes},
		{"keyframe value", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"opacity": {NewKeyframe(0, 1.5)}} })
		}, VKeyframes},
		{"keyframe scale value", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"scale": {NewKeyframe(0, 0)}} })
		}, VKeyframes},
		{"keyframe volume value", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"volume": {NewKeyframe(0, 5)}} })
		}, VKeyframes},
		{"keyframe easing", func(d *Document) {
			k := NewKeyframe(0, 1)
			k.Easing = "bounce"
			editItem(d, clipID, func(i *Item) { i.Keyframes = Keyframes{"x": {k}} })
		}, VKeyframes},
		{"volume keyframes on caption", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Keyframes = Keyframes{"volume": {NewKeyframe(0, 1)}} })
		}, VKeyframes},
		{"scale keyframes on audio", func(d *Document) {
			d.Tracks["trk_k000000001"] = Track{ID: "trk_k000000001", Kind: TrackAudio, Order: "a2"}
			a := NewItem("itm_a000000001", ItemAudio, "trk_k000000001", 0, 10)
			a.Asset = assetID
			a.Keyframes = Keyframes{"scale": {NewKeyframe(0, 1)}}
			addItem(d, a)
		}, VKeyframes},

		// Text, shape.
		{"text empty", func(d *Document) {
			text(d, "itm_x000000001", 60, nil)
			editItem(d, "itm_x000000001", func(i *Item) { i.Text = "" })
		}, VShape},
		{"text long", func(d *Document) {
			text(d, "itm_x000000001", 60, nil)
			editItem(d, "itm_x000000001", func(i *Item) { i.Text = strings.Repeat("a", 2001) })
		}, VShape},
		{"shape kind", func(d *Document) { addShape(d, func(i *Item) { i.Shape = "star" }) }, VShape},
		{"shape size", func(d *Document) { addShape(d, func(i *Item) { i.Width = 0 }) }, VShape},
		{"shape radius", func(d *Document) { addShape(d, func(i *Item) { i.Radius = -1 }) }, VShape},
		{"shape fill", func(d *Document) { addShape(d, func(i *Item) { i.Fill = "blue" }) }, VShape},
		{"shape fill ref", func(d *Document) { addShape(d, func(i *Item) { i.Fill = "$brand" }) }, VColorRef},

		// V_WORDS
		{"words none", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Words = nil }) }, VShape},
		{
			"words 61",
			func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Duration = 100; i.Words = manyWords(61) }) },
			VShape,
		},
		{"word text empty", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Words = []Word{{Text: "", Start: 0, Duration: 1}} })
		}, VShape},
		{"word text long", func(d *Document) {
			editItem(
				d,
				capItemID,
				func(i *Item) { i.Words = []Word{{Text: strings.Repeat("w", 81), Start: 0, Duration: 1}} },
			)
		}, VShape},
		{"words overlap", func(d *Document) {
			editItem(d, capItemID, func(i *Item) {
				i.Words = []Word{{Text: "a", Start: 0, Duration: 10}, {Text: "b", Start: 5, Duration: 10}}
			})
		}, VWords},
		{"word past end", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Words = []Word{{Text: "a", Start: 40, Duration: 9}} })
		}, VWords},
		{"word negative", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Words = []Word{{Text: "a", Start: -1, Duration: 2}} })
		}, VWords},
		{"word duration", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Words = []Word{{Text: "a", Start: 0, Duration: 0}} })
		}, VWords},
		{"breaks range", func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Breaks = []int{9} }) }, VWords},
		{
			"breaks order",
			func(d *Document) { editItem(d, capItemID, func(i *Item) { i.Breaks = []int{2, 1} }) },
			VWords,
		},

		// V_FONT and style fields.
		{"font family", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{FontFamily: new("Comic Sans")})
		}, VFont},
		{"user font weight", func(d *Document) {
			d.Fonts = map[string]ProjectFont{"Mine": {Files: []FontFile{{Weight: 400, Hash: d.Assets[assetID].Hash}}}}
			text(d, "itm_x000000001", 60, &CaptionStyle{FontFamily: new("Mine"), FontWeight: new(700)})
		}, VFont},
		{"font weight step", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{FontWeight: new(750)})
		}, VShape},
		{"style size", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{Size: new(2000.0)})
		}, VShape},
		{"style align", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{Align: new("justify")})
		}, VShape},
		{"style color", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{Color: new(Color("nope"))})
		}, VShape},
		{"style color ref", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{Color: new(Color("$accent"))})
		}, VColorRef},
		{
			"style position on text",
			func(d *Document) { text(d, "itm_x000000001", 60, &CaptionStyle{Position: new("top")}) },
			VShape,
		},
		{"caption position", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Style = &CaptionStyle{Position: new("left")} })
		}, VShape},
		{"caption highlight", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Style = &CaptionStyle{Highlight: new("all")} })
		}, VShape},
		{"bundled weight missing", func(d *Document) {
			text(
				d,
				"itm_x000000001",
				60,
				&CaptionStyle{FontFamily: new("JetBrains Mono"), FontWeight: new(900)},
			)
		}, VFont},
		{"bundled weight missing", func(d *Document) {
			text(d, "itm_x000000001", 60, &CaptionStyle{FontFamily: new("Anton"), FontWeight: new(700)})
		}, VFont},
		{"fonts no files", func(d *Document) { d.Fonts = map[string]ProjectFont{"Mine": {}} }, VShape},
		{"fonts weight", func(d *Document) {
			d.Fonts = map[string]ProjectFont{"Mine": {Files: []FontFile{{Weight: 150, Hash: d.Assets[assetID].Hash}}}}
		}, VShape},
		{"fonts hash", func(d *Document) {
			d.Fonts = map[string]ProjectFont{"Mine": {Files: []FontFile{{Weight: 400, Hash: "x"}}}}
		}, VShape},

		// V_SLOT
		{"slot name", func(d *Document) {
			editItem(d, clipID, func(i *Item) { s := NewSlot("Bad Name", "media"); i.Slot = &s })
		}, VSlot},
		{
			"slot kind on clip",
			func(d *Document) { editItem(d, clipID, func(i *Item) { s := NewSlot("a", "text"); i.Slot = &s }) },
			VSlot,
		},
		{
			"media slot on caption",
			func(d *Document) { editItem(d, capItemID, func(i *Item) { s := NewSlot("a", "media"); i.Slot = &s }) },
			VSlot,
		},
		{
			"slot kind unknown",
			func(d *Document) { editItem(d, clipID, func(i *Item) { s := NewSlot("a", "video"); i.Slot = &s }) },
			VShape,
		},
		{"slot description", func(d *Document) {
			editItem(
				d,
				clipID,
				func(i *Item) { s := NewSlot("a", "media"); s.Description = strings.Repeat("d", 201); i.Slot = &s },
			)
		}, VShape},
		{"slot policy", func(d *Document) {
			editItem(d, clipID, func(i *Item) { s := NewSlot("a", "media"); s.DurationPolicy = "grow"; i.Slot = &s })
		}, VShape},
		{"slot duplicate", func(d *Document) {
			editItem(d, clipID, func(i *Item) { s := NewSlot("same", "media"); i.Slot = &s })
			editItem(d, capItemID, func(i *Item) { s := NewSlot("same", "caption"); i.Slot = &s })
		}, VSlot},

		// V_COLOR_REF and template colours.
		{
			"template colour",
			func(d *Document) { d.Template = &TemplateInfo{Colors: map[string]Color{"accent": "red"}} },
			VShape,
		},
		{"colour ref missing", func(d *Document) {
			d.Template = &TemplateInfo{Colors: map[string]Color{"other": "#FFFFFF"}}
			text(d, "itm_x000000001", 60, &CaptionStyle{Color: new(Color("$accent"))})
		}, VColorRef},

		// V_LIMITS
		{"too many tracks", func(d *Document) {
			for i := range MaxTracks {
				id := fmt.Sprintf("trk_x%09d", i)
				d.Tracks[id] = Track{ID: id, Kind: TrackVisual, Order: fmt.Sprintf("c%03d", i)}
			}
		}, VLimits},
		{"too many items", func(d *Document) {
			for i := range MaxItems {
				text(d, fmt.Sprintf("itm_x%09d", i), 100+i*10, nil)
			}
		}, VLimits},
		{"too many assets", func(d *Document) {
			for i := range MaxAssets {
				a := d.Assets[assetID]
				a.ID = fmt.Sprintf("ast_x%09d", i)
				d.Assets[a.ID] = a
			}
		}, VLimits},
		{"too many words", func(d *Document) {
			for i := range 84 {
				c := NewItem(fmt.Sprintf("itm_x%09d", i), ItemCaption, capTrack, 100+i*60, 60)
				c.Words = manyWords(60)
				addItem(d, c)
			}
		}, VLimits},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base(t)
			c.edit(d)
			if got := codesOf(Validate(d)); len(got) != 1 || got[0] != c.want {
				t.Errorf("codes %v, want only %s\n%v", got, c.want, Validate(d))
			}
		})
	}
}

func addShape(d *Document, f func(*Item)) {
	s := NewItem("itm_x000000001", ItemShape, capTrack, 60, 10)
	s.Shape, s.Width, s.Height = "rect", 10, 10
	f(&s)
	addItem(d, s)
}

func TestValidateAcceptsEdgeValues(t *testing.T) {
	d := base(t)
	// End is exclusive: an item may start where the previous one ends.
	editItem(d, clipID, func(i *Item) { i.Duration = 100; i.Start = 0 })
	next := NewItem("itm_x000000001", ItemClip, vidTrack, 100, 50)
	next.Asset = assetID
	addItem(d, next)
	// An item may end exactly at the last frame, and a speed bound is inclusive.
	last := NewItem("itm_x000000002", ItemShape, capTrack, MaxFrames-5, 5)
	last.Shape, last.Width, last.Height = "ellipse", 1, 1
	addItem(d, last)
	editItem(d, clipID, func(i *Item) { i.Speed = Rational{1, 4} })
	editItem(d, "itm_x000000001", func(i *Item) { i.Speed = Rational{4, 1}; i.Duration = 100 })
	if vs := Validate(d); len(vs) != 0 {
		t.Errorf("%v", vs)
	}
}

func TestValidateReportsEveryViolationWithPathAndID(t *testing.T) {
	d := base(t)
	d.Name = ""
	editItem(d, clipID, func(i *Item) { i.Scale = 0; i.SourceIn = 800 })
	vs := Validate(d)
	want := []Violation{
		{Code: VShape, Path: "items." + clipID + ".scale", ID: clipID},
		{Code: VSourceRange, Path: "items." + clipID + ".source_in", ID: clipID},
		{Code: VShape, Path: "name", ID: d.ID},
	}
	if len(vs) != 3 {
		t.Fatalf("%v", vs)
	}
	// Ordered by path.
	for i, w := range []int{2, 0, 1} {
		_ = i
		if vs[w].Code == "" {
			t.Fatal("empty code")
		}
	}
	for _, w := range want {
		found := false
		for _, v := range vs {
			found = found || v.Code == w.Code && v.Path == w.Path && v.ID == w.ID && v.Message != ""
		}
		if !found {
			t.Errorf("missing %+v in %v", w, vs)
		}
	}
	if vs[0].String() == "" || !strings.HasPrefix(vs[0].String(), vs[0].Code) {
		t.Error("String")
	}
}

func TestValidateTemplatePlaceholderAndColourRef(t *testing.T) {
	d := mustDecode(t, readFile(t, "testdata/valid/template.json"))
	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("%v", vs)
	}
}

func TestValidateOverlapNamesTheFrames(t *testing.T) {
	d := base(t)
	editItem(d, capItemID, func(i *Item) { i.Track = vidTrack; i.Start = 100 })
	vs := Validate(d)
	if len(vs) != 1 || vs[0].Code != VOverlap || !strings.Contains(vs[0].Message, "frames 100 to 147") {
		t.Errorf("%v", vs)
	}
}

func TestValidateOverlapNestedItems(t *testing.T) {
	d := base(t)
	editItem(d, clipID, func(i *Item) { i.Duration = 100; i.SourceIn = 0 })
	for i, start := range []int{10, 20} { // both inside the first item
		it := NewItem(fmt.Sprintf("itm_x00000000%d", i), ItemClip, vidTrack, start, 5)
		it.Asset = assetID
		addItem(d, it)
	}
	if n := len(Validate(d)); n != 2 {
		t.Errorf("want 2 overlap violations, got %v", Validate(d))
	}
}

func TestValidateTransparentBackground(t *testing.T) {
	d := base(t)
	d.Settings.Background = BackgroundTransparent
	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("transparent is a valid background, got %v", vs)
	}
	d.Settings.Background = "clear"
	if got := codesOf(Validate(d)); len(got) != 1 || got[0] != VShape {
		t.Errorf("an unknown background keyword gives %v, want only %s", got, VShape)
	}
}

func TestValidateShadowAndOutline(t *testing.T) {
	cases := []invalidCase{
		{"shadow opacity above 1", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Shadow = &Shadow{Color: "#000000", Opacity: 2} })
		}, VShape},
		{"outline too thin", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Outline = &Outline{Color: "#000000", Width: 0.1} })
		}, VShape},
		{"outline colour unknown", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Outline = &Outline{Color: "red", Width: 2} })
		}, VShape},
		{"shadow on a text item", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Shadow = &Shadow{Color: "#000000", Opacity: 0.5} })
		}, VShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base(t)
			c.edit(d)
			if got := codesOf(Validate(d)); len(got) != 1 || got[0] != c.want {
				t.Errorf("codes %v, want only %s\n%v", got, c.want, Validate(d))
			}
		})
	}

	d := base(t)
	editItem(d, clipID, func(i *Item) {
		i.Shadow = &Shadow{Color: "#000000", Opacity: 0.5, DX: 8, DY: 8, Blur: 12}
		i.Outline = &Outline{Color: "#FFFFFF", Width: 4}
	})
	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("a shadow and an outline on a clip are valid, got %v", vs)
	}
}
