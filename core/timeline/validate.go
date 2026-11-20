package timeline

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"unicode/utf8"
)

// Violation codes (SDD-001 §4).
const (
	VSchemaVersion = "V_SCHEMA_VERSION"
	VShape         = "V_SHAPE"
	VID            = "V_ID"
	VTrackOrder    = "V_TRACK_ORDER"
	VItemTrack     = "V_ITEM_TRACK"
	VItemRange     = "V_ITEM_RANGE"
	VOverlap       = "V_OVERLAP"
	VAssetRef      = "V_ASSET_REF"
	VSourceRange   = "V_SOURCE_RANGE"
	VSpeed         = "V_SPEED"
	VFades         = "V_FADES"
	VTransitions   = "V_TRANSITIONS"
	VKeyframes     = "V_KEYFRAMES"
	VWords         = "V_WORDS"
	VMarkers       = "V_MARKERS"
	VFont          = "V_FONT"
	VSlot          = "V_SLOT"
	VColorRef      = "V_COLOR_REF"
	VLimits        = "V_LIMITS"
	VComposition   = "V_COMPOSITION"
)

var (
	hashPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	slotNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	colorRefPattern = regexp.MustCompile(`^\$[a-z][a-z0-9_]{0,39}$`)
)

type validator struct {
	doc *Document
	vs  []Violation
}

func (v *validator) add(code, path, id, format string, args ...any) {
	v.vs = append(v.vs, Violation{Code: code, Path: path, ID: id, Message: fmt.Sprintf(format, args...)})
}

// related records the other objects the last violation involves.
func (v *validator) related(ids ...string) {
	last := &v.vs[len(v.vs)-1]
	last.Related = append(last.Related, ids...)
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func inRange(f, lo, hi float64) bool { return finite(f) && f >= lo && f <= hi }

func runes(s string) int { return utf8.RuneCountInString(s) }

// Validate returns every invariant of SDD-001 §4 that doc violates, ordered by path
// and code. An empty result means the document is valid.
func Validate(doc *Document) []Violation {
	v := &validator{doc: doc}
	v.document()
	sort.SliceStable(v.vs, func(i, j int) bool {
		if v.vs[i].Path != v.vs[j].Path {
			return v.vs[i].Path < v.vs[j].Path
		}
		return v.vs[i].Code < v.vs[j].Code
	})
	return v.vs
}

func (v *validator) document() {
	d := v.doc
	switch {
	case d.SchemaVersion > CurrentSchemaVersion:
		v.add(
			VSchemaVersion,
			"schema_version",
			d.ID,
			"schema_version %d is newer than the %d this program supports; upgrade the program",
			d.SchemaVersion,
			CurrentSchemaVersion,
		)
	case d.SchemaVersion != CurrentSchemaVersion:
		v.add(
			VSchemaVersion,
			"schema_version",
			d.ID,
			"schema_version %d is not supported (current is %d)",
			d.SchemaVersion,
			CurrentSchemaVersion,
		)
	}
	if !ValidID(PrefixProject, d.ID) {
		v.add(VID, "id", d.ID, "%q is not a valid project identifier", d.ID)
	}
	if n := runes(d.Name); n < 1 || n > 120 {
		v.add(VShape, "name", d.ID, "name must have 1 to 120 characters")
	}
	v.settings()
	v.docMarkers(d)
	if len(d.Tracks) > MaxTracks {
		v.add(VLimits, "tracks", d.ID, "%d tracks exceed the limit of %d", len(d.Tracks), MaxTracks)
	}
	if len(d.Items) > MaxItems {
		v.add(VLimits, "items", d.ID, "%d items exceed the limit of %d", len(d.Items), MaxItems)
	}
	if len(d.Assets) > MaxAssets {
		v.add(VLimits, "assets", d.ID, "%d assets exceed the limit of %d", len(d.Assets), MaxAssets)
	}
	words := 0
	for k := range d.Items {
		words += len(d.Items[k].Words)
	}
	if words > MaxCaptionWords {
		v.add(VLimits, "items", d.ID, "%d caption words exceed the limit of %d", words, MaxCaptionWords)
	}
	for _, k := range sortedKeys(d.Assets) {
		v.asset(k, d.Assets[k])
	}
	v.tracks()
	for _, k := range sortedKeys(d.Fonts) {
		v.projectFont(k, d.Fonts[k])
	}
	v.template()
	for _, k := range sortedKeys(d.Items) {
		v.item(k, d.Items[k])
	}
	v.overlaps()
	v.slotNames()
}

func (v *validator) settings() {
	s := v.doc.Settings
	id := v.doc.ID
	for _, dim := range []struct {
		name string
		n    int
	}{{"width", s.Width}, {"height", s.Height}} {
		if dim.n < 16 || dim.n > 4096 || dim.n%2 != 0 {
			v.add(VShape, "settings."+dim.name, id, "%s must be even and between 16 and 4096", dim.name)
		}
	}
	f := s.FPS
	if f.Den != 1 && f.Den != 1001 {
		v.add(VShape, "settings.fps", id, "den must be 1 or 1001")
	} else if f.Num < f.Den || f.Num > 120*f.Den {
		v.add(VShape, "settings.fps", id, "fps must be between 1 and 120")
	}
	if !inRange(s.LoudnessTarget, -30, -5) {
		v.add(VShape, "settings.loudness_target", id, "loudness_target must be between -30 and -5")
	}
	if s.Background != BackgroundTransparent {
		v.color("settings.background", id, s.Background, true)
	}
}

// color checks a colour value; refs says whether "$name" is accepted.
func (v *validator) color(path, id string, c Color, refs bool) {
	switch {
	case isHexColor(string(c)):
	case refs && colorRefPattern.MatchString(string(c)):
		name := string(c)[1:]
		if v.doc.Template == nil {
			v.add(VColorRef, path, id, "colour reference %s but the document is not a template", c)
		} else if _, ok := v.doc.Template.Colors[name]; !ok {
			v.add(VColorRef, path, id, "colour reference %s is not in template.colors", c)
		}
	default:
		v.add(VShape, path, id, "%q is not a colour (#RRGGBB or #RRGGBBAA)", string(c))
	}
}

func (v *validator) asset(key string, a Asset) {
	path := "assets." + key
	v.animation(path, key, a)
	switch {
	case !ValidID(PrefixAsset, a.ID):
		v.add(VID, path+".id", key, "%q is not a valid asset identifier", a.ID)
	case a.ID != key:
		v.add(VID, path+".id", key, "map key %q differs from id %q", key, a.ID)
	}
	if !hashPattern.MatchString(a.Hash) {
		v.add(VShape, path+".hash", key, "hash must be sha256: followed by 64 lower-case hex characters")
	}
	switch a.Kind {
	case AssetVideo:
		if a.FrameRate.Num < 1 || a.FrameRate.Den < 1 {
			v.add(VShape, path+".frame_rate", key, "a video asset needs a positive frame_rate")
		}
		if a.FrameCount < 1 {
			v.add(VShape, path+".frame_count", key, "a video asset needs frame_count of at least 1")
		}
		if a.Width < 1 || a.Height < 1 {
			v.add(VShape, path+".width", key, "a video asset needs width and height")
		}
		if a.SampleCount < 0 {
			v.add(VShape, path+".sample_count", key, "sample_count cannot be negative")
		}
	case AssetAudio:
		if a.SampleCount < 1 {
			v.add(VShape, path+".sample_count", key, "an audio asset needs sample_count of at least 1")
		}
	case AssetImage:
		if a.Width < 1 || a.Height < 1 {
			v.add(VShape, path+".width", key, "an image asset needs width and height")
		}
	case AssetLUT:
		// A lookup table has no picture facts.
	default:
		v.add(VShape, path+".kind", key, "kind must be video, audio, image or lut")
	}
}

func (v *validator) tracks() {
	d := v.doc
	byOrder := map[string]string{}
	for _, k := range sortedKeys(d.Tracks) {
		t := d.Tracks[k]
		path := "tracks." + k
		switch {
		case !ValidID(PrefixTrack, t.ID):
			v.add(VID, path+".id", k, "%q is not a valid track identifier", t.ID)
		case t.ID != k:
			v.add(VID, path+".id", k, "map key %q differs from id %q", k, t.ID)
		}
		if t.Kind != TrackVisual && t.Kind != TrackAudio {
			v.add(VShape, path+".kind", k, "kind must be visual or audio")
		}
		if runes(t.Name) > 60 {
			v.add(VShape, path+".name", k, "name has more than 60 characters")
		}
		if t.Volume != nil && !inRange(*t.Volume, 0, 4) {
			v.add(VShape, path+".volume", k, "volume must be between 0 and 4")
		}
		if !ValidOrder(t.Order) {
			v.add(VTrackOrder, path+".order", k, "order must be a non-empty string of 0-9a-z")
		} else if other, dup := byOrder[t.Order]; dup {
			v.add(VTrackOrder, path+".order", k, "order %q is also used by track %s", t.Order, other)
		} else {
			byOrder[t.Order] = k
		}
		if t.CaptionStyle != nil {
			v.style(path+".caption_style", k, t.CaptionStyle, true)
		}
		v.processors(path, k, t.Processors, false)
	}
}

func (v *validator) template() {
	t := v.doc.Template
	if t == nil {
		return
	}
	for _, k := range sortedKeys(t.Colors) {
		v.color("template.colors."+k, v.doc.ID, t.Colors[k], false)
	}
}

func (v *validator) style(path, id string, s *CaptionStyle, caption bool) {
	v.styleFont(path, id, s)
	v.styleNumbers(path, id, s)
	v.styleColors(path, id, s)
	v.styleCaption(path, id, s, caption)
}

func (v *validator) styleFont(path, id string, s *CaptionStyle) {
	if s.FontFamily == nil && s.FontWeight == nil {
		return
	}
	family, weight := "Inter", 0
	if s.FontFamily != nil {
		family = *s.FontFamily
	}
	if s.FontWeight != nil {
		weight = *s.FontWeight
	}
	weights, ok := FontWeights(v.doc, family)
	switch {
	case !ok:
		v.add(VFont, path+".font_family", id, "font family %q is not bundled and not in fonts", family)
	case s.FontWeight != nil && weight%100 == 0 && weight >= 100 && weight <= 900 && !slices.Contains(weights, weight):
		v.add(VFont, path+".font_weight", id, "font %q has no weight %d; it has %v", family, weight, weights)
	}
}

func (v *validator) styleNumbers(path, id string, s *CaptionStyle) {
	if s.FontWeight != nil && (*s.FontWeight < 100 || *s.FontWeight > 900 || *s.FontWeight%100 != 0) {
		v.add(VShape, path+".font_weight", id, "font_weight must be 100 to 900 in steps of 100")
	}
	for _, r := range []struct {
		name   string
		val    *float64
		lo, hi float64
	}{
		{"size", s.Size, 8, 1000},
		{"stroke_width", s.StrokeWidth, 0, 40},
		{"shadow_dx", s.ShadowDX, -math.MaxFloat64, math.MaxFloat64},
		{"shadow_dy", s.ShadowDY, -math.MaxFloat64, math.MaxFloat64},
		{"shadow_blur", s.ShadowBlur, 0, 100},
		{"background_padding", s.BackgroundPadding, 0, math.MaxFloat64},
		{"background_radius", s.BackgroundRadius, 0, math.MaxFloat64},
		{"line_height", s.LineHeight, 0.8, 3},
		{"max_width", s.MaxWidth, math.SmallestNonzeroFloat64, math.MaxFloat64},
	} {
		if r.val != nil && !inRange(*r.val, r.lo, r.hi) {
			v.add(VShape, path+"."+r.name, id, "%s is out of range", r.name)
		}
	}
	if s.Align != nil && *s.Align != "left" && *s.Align != "center" && *s.Align != "right" {
		v.add(VShape, path+".align", id, "align must be left, center or right")
	}
}

func (v *validator) styleColors(path, id string, s *CaptionStyle) {
	for _, c := range []struct {
		name string
		val  *Color
	}{
		{"color", s.Color},
		{"stroke_color", s.StrokeColor},
		{"shadow_color", s.ShadowColor},
		{"background_color", s.BackgroundColor},
		{"highlight_color", s.HighlightColor},
		{"highlight_background", s.HighlightBackground},
	} {
		if c.val != nil {
			v.color(path+"."+c.name, id, *c.val, true)
		}
	}
}

func (v *validator) styleCaption(path, id string, s *CaptionStyle, caption bool) {
	if s.Position != nil {
		switch *s.Position {
		case "bottom", "middle", "top", "custom":
		default:
			v.add(VShape, path+".position", id, "position must be bottom, middle, top or custom")
		}
	}
	if s.Highlight != nil && *s.Highlight != "none" && *s.Highlight != "word" {
		v.add(VShape, path+".highlight", id, "highlight must be none or word")
	}
	if !caption &&
		(s.Position != nil || s.Highlight != nil || s.HighlightColor != nil || s.HighlightBackground != nil) {
		v.add(VShape, path, id, "position and highlight fields apply to caption items only")
	}
}

func (v *validator) item(key string, it Item) {
	d := v.doc
	path := "items." + key
	switch {
	case !ValidID(PrefixItem, it.ID):
		v.add(VID, path+".id", key, "%q is not a valid item identifier", it.ID)
	case it.ID != key:
		v.add(VID, path+".id", key, "map key %q differs from id %q", key, it.ID)
	}
	switch it.Type {
	case ItemClip, ItemAudio, ItemText, ItemCaption, ItemShape, ItemComposition:
	default:
		v.add(VShape, path+".type", key, "type must be clip, audio, text, caption, shape or composition")
		return
	}
	if runes(it.Name) > 120 {
		v.add(VShape, path+".name", key, "name has more than 120 characters")
	}

	// Track.
	if !ValidID(PrefixTrack, it.Track) {
		v.add(VID, path+".track", key, "%q is not a valid track identifier", it.Track)
	} else if tr, ok := d.Tracks[it.Track]; !ok {
		v.add(VItemTrack, path+".track", key, "track %s does not exist", it.Track)
		v.related(it.Track)
	} else if (it.Type == ItemAudio) != (tr.Kind == TrackAudio) {
		v.add(VItemTrack, path+".track", key, "a %s track does not accept %s items", tr.Kind, it.Type)
		v.related(it.Track)
	}

	// Range.
	if it.Start < 0 || it.Duration < 1 || it.Start+it.Duration > MaxFrames {
		v.add(
			VItemRange,
			path,
			key,
			"start %d and duration %d must satisfy start >= 0, duration >= 1 and end <= %d",
			it.Start,
			it.Duration,
			MaxFrames,
		)
	}

	v.itemFieldsForType(path, key, it)
	if it.Type.IsVisual() {
		v.visual(path, key, it)
		v.filter(path, key, it)
	}
	v.markers(path, key, it)
	if it.Type == ItemClip || it.Type == ItemAudio {
		v.media(path, key, it)
	}
	switch it.Type {
	case ItemClip:
		v.clip(path, key, it)
	case ItemAudio:
		// Fully checked by media above.
	case ItemText:
		if n := runes(it.Text); n < 1 || n > 2000 {
			v.add(VShape, path+".text", key, "text must have 1 to 2000 characters")
		}
		if it.Style != nil {
			v.style(path+".style", key, it.Style, false)
		}
	case ItemCaption:
		v.caption(path, key, it)
	case ItemShape:
		v.shape(path, key, it)
	case ItemComposition:
		v.composition(path, key, it)
	}
	v.transitions(path, key, it)
	v.keyframes(path, key, it)
	v.slot(path, key, it)
	v.link(path, key, it)
	v.effects(path, key, it)
}

// clipGrading says the item carries a field of the framing and colour work that only a clip has.
func clipGrading(it *Item) bool {
	return it.Crop != nil || it.Lut != nil || it.Wheels != nil || it.Curves != nil || len(it.Hsl) > 0 ||
		it.Brightness != 0 || it.Temperature != 0 || it.Tint != 0 || it.Hue != 0
}

// compositionGrading says a composition carries a colour field its picture cannot have: contrast and
// saturation are 1 by default, and a chroma key needs pixels of a source.
func compositionGrading(it *Item) bool {
	return it.Contrast != 1 || it.Saturation != 1 || it.ChromaKey != nil
}

// compositionExtras says a composition carries what it may not have: an asset and the audio shaping of one.
func compositionExtras(it *Item) bool {
	return it.Asset != "" || it.Denoise != 0 || it.Voice != "" || len(it.SpeedCurve) > 0 || it.Reverse
}

// mediaFieldsForType reports the media fields of an item that is not a clip, an audio item or a composition,
// and what a composition has no use for.
func mediaFieldsForType(bad func(string), it *Item) {
	media := it.Type == ItemClip || it.Type == ItemAudio
	comp := it.Type == ItemComposition
	if !media && !comp &&
		(it.Asset != "" || it.SourceIn != 0 || it.FadeIn != 0 || it.FadeOut != 0 || it.Denoise != 0 || it.Voice != "" ||
			len(it.SpeedCurve) > 0 || it.Reverse || len(it.Processors) > 0) {
		bad("asset")
	}
	// A composition plays its nested timeline: it keeps source_in and the fades, and has no asset of its own.
	if comp && compositionExtras(it) {
		bad("asset")
	}
	if !comp && it.Timeline != "" {
		bad("timeline")
	}
	if it.Type != ItemClip && (clipGrading(it) || (comp && compositionGrading(it))) {
		bad("crop")
	}
	if it.Type != ItemClip && !comp && (it.Shadow != nil || it.Outline != nil || it.AudioMuted) {
		bad("shadow")
	}
}

// itemFieldsForType reports fields that belong to another item type.
func (v *validator) itemFieldsForType(path, key string, it Item) {
	bad := func(name string) { v.add(VShape, path+"."+name, key, "%s does not apply to %s items", name, it.Type) }
	mediaFieldsForType(bad, &it)
	if it.Freeze &&
		(it.Type != ItemClip || it.Speed != (Rational{Num: 1, Den: 1}) || len(it.SpeedCurve) > 0 || it.Reverse) {
		bad("freeze")
	}
	if it.Type != ItemText && it.Text != "" {
		bad("text")
	}
	if it.Type != ItemText && it.Type != ItemCaption && it.Style != nil {
		bad("style")
	}
	if it.Type != ItemCaption && (len(it.Words) > 0 || len(it.Breaks) > 0 || it.LinkedItem != "") {
		bad("words")
	}
	if it.Type != ItemShape &&
		(it.Shape != "" || it.Width != 0 || it.Height != 0 || it.Radius != 0 || it.StrokeWidth != 0) {
		bad("shape")
	}
	if it.Type == ItemAudio &&
		(it.Blur != 0 || (it.Filter != "" && it.Filter != "none") || (it.FilterAmount != 0 && it.FilterAmount != 1) ||
			it.ChromaKey != nil || it.Blend != "" || it.Mask != nil || it.FlipX || it.FlipY || len(it.Effects) > 0) {
		bad("filter")
	}
	if it.Type == ItemAudio && (it.TransitionIn != nil || it.TransitionOut != nil || it.X != nil || it.Y != nil) {
		bad("transition_in")
	}
}

func (v *validator) visual(path, key string, it Item) {
	for _, p := range []struct {
		name string
		val  *float64
	}{{"x", it.X}, {"y", it.Y}} {
		if p.val != nil && !finite(*p.val) {
			v.add(VShape, path+"."+p.name, key, "%s must be a finite number", p.name)
		}
	}
	if !finite(it.Scale) || it.Scale <= 0 || it.Scale > 20 {
		v.add(VShape, path+".scale", key, "scale must be greater than 0 and at most 20")
	}
	if !finite(it.Rotation) {
		v.add(VShape, path+".rotation", key, "rotation must be a finite number")
	}
	if !inRange(it.Opacity, 0, 1) {
		v.add(VShape, path+".opacity", key, "opacity must be between 0 and 1")
	}
}

func (v *validator) media(path, key string, it Item) {
	if !inRange(it.Volume, 0, 4) {
		v.add(VShape, path+".volume", key, "volume must be between 0 and 4")
	}
	if !inRange(it.Denoise, 0, 1) {
		v.add(VShape, path+".denoise", key, "denoise must be between 0 and 1")
	}
	v.voice(path, key, it)
	v.processors(path, key, it.Processors, true)
	if it.SourceIn < 0 {
		v.add(VShape, path+".source_in", key, "source_in cannot be negative")
	}
	if it.FadeIn < 0 || it.FadeOut < 0 {
		v.add(VShape, path+".fade_in", key, "fades cannot be negative")
	} else if it.FadeIn+it.FadeOut > it.Duration {
		v.add(VFades, path, key, "fade_in %d plus fade_out %d exceeds duration %d", it.FadeIn, it.FadeOut, it.Duration)
	}

	sp := it.Speed
	switch {
	case sp.Num < 1 || sp.Den < 1:
		v.add(VShape, path+".speed", key, "speed num and den must be positive")
	case sp.Num*4 < sp.Den || sp.Num > 4*sp.Den:
		v.add(VSpeed, path+".speed", key, "speed %d/%d is outside 1/4 to 4", sp.Num, sp.Den)
	}

	if len(it.SpeedCurve) > 0 {
		if msg := CurveError(it.SpeedCurve); msg != "" {
			v.add(VSpeed, path+".speed_curve", key, "%s", msg)
		} else if m := CurveMean(it.SpeedCurve); m != sp {
			v.add(
				VSpeed,
				path+".speed",
				key,
				"speed %d/%d is not the mean %d/%d of the speed curve",
				sp.Num,
				sp.Den,
				m.Num,
				m.Den,
			)
		}
	}

	// Asset reference and source range.
	if it.Asset == "" {
		v.add(VShape, path+".asset", key, "a %s item needs an asset", it.Type)
		return
	}
	if it.Asset == PlaceholderAsset {
		if v.doc.Template == nil {
			v.add(VAssetRef, path+".asset", key, "the placeholder asset is allowed only in a template")
		}
		return
	}
	if !ValidID(PrefixAsset, it.Asset) {
		v.add(VID, path+".asset", key, "%q is not a valid asset identifier", it.Asset)
		return
	}
	a, ok := v.doc.Assets[it.Asset]
	if !ok {
		v.add(VAssetRef, path+".asset", key, "asset %s does not exist", it.Asset)
		v.related(it.Asset)
		return
	}
	switch {
	case it.Type == ItemClip && (a.Kind == AssetAudio || a.Kind == AssetLUT):
		v.add(VAssetRef, path+".asset", key, "a clip needs a video or image asset, %s is %s", it.Asset, a.Kind)
		v.related(it.Asset)
		return
	case it.Type == ItemAudio && (a.Kind == AssetImage || a.Kind == AssetLUT || a.Kind == AssetVideo && !a.HasAudio):
		v.add(
			VAssetRef,
			path+".asset",
			key,
			"an audio item needs an audio asset or a video with audio, %s is not",
			it.Asset,
		)
		v.related(it.Asset)
		return
	}
	if sp.Num < 1 || sp.Den < 1 || it.SourceIn < 0 || it.Duration < 1 {
		return
	}
	if total, ok := assetFrames(a, v.doc.Settings.FPS); ok {
		// source_in + ceil(duration * speed.num / speed.den) <= total
		need := sourceEnd(it)
		if need.Cmp(total) > 0 {
			v.add(
				VSourceRange,
				path+".source_in",
				key,
				"source range ends at frame %s but asset %s has %s frames",
				need,
				it.Asset,
				total,
			)
			v.related(it.Asset)
		}
	}
}

// CompositionIDOK is the form check of the nested timeline id of a composition item.
func CompositionIDOK(id string) bool { return ValidID(PrefixProject, id) }

// composition checks what one document can say about a composition item (SDD-001 §3.5.1): the nested
// timeline is named well and is not the document itself, and the fields it keeps have valid values. That
// the timeline is in the project, that there is no cycle and that the depth is bounded are checked by the
// command core and the service, which know the other documents.
func (v *validator) composition(path, key string, it Item) {
	if !inRange(it.Volume, 0, 4) {
		v.add(VShape, path+".volume", key, "volume must be between 0 and 4")
	}
	if it.SourceIn < 0 {
		v.add(VShape, path+".source_in", key, "source_in cannot be negative")
	}
	if it.FadeIn < 0 || it.FadeOut < 0 {
		v.add(VShape, path+".fade_in", key, "fades cannot be negative")
	} else if it.FadeIn+it.FadeOut > it.Duration {
		v.add(VFades, path, key, "fade_in %d plus fade_out %d exceeds duration %d", it.FadeIn, it.FadeOut, it.Duration)
	}
	switch {
	case it.Timeline == "":
		v.add(VShape, path+".timeline", key, "a composition item needs a timeline")
	case !CompositionIDOK(it.Timeline):
		v.add(VID, path+".timeline", key, "%q is not a valid timeline identifier", it.Timeline)
	case it.Timeline == v.doc.ID:
		v.add(VComposition, path+".timeline", key, "a timeline cannot contain itself")
	}
	// A composition has a constant speed, with the range of a clip (SDD-001 §3.5.1, epic 53).
	switch sp := it.Speed; {
	case sp.Num < 1 || sp.Den < 1:
		v.add(VShape, path+".speed", key, "speed num and den must be positive")
	case sp.Num*4 < sp.Den || sp.Num > 4*sp.Den:
		v.add(VSpeed, path+".speed", key, "speed %d/%d is outside 1/4 to 4", sp.Num, sp.Den)
	}
	if it.Freeze {
		v.add(VComposition, path+".freeze", key, "a composition cannot be a freeze frame")
	}
}

// sourceEnd is the first source frame after the range an item reads: source_in + ceil(duration * speed.num /
// speed.den), or source_in + 1 for a freeze frame.
func sourceEnd(it Item) *big.Int {
	sp := it.Speed
	need := new(big.Int).Mul(big.NewInt(int64(it.Duration)), big.NewInt(int64(sp.Num)))
	need.Add(need, big.NewInt(int64(sp.Den-1)))
	need.Div(need, big.NewInt(int64(sp.Den)))
	if it.Freeze {
		need.SetInt64(1)
	}
	return need.Add(need, big.NewInt(int64(it.SourceIn)))
}

// AssetDuration is the length of an asset in project frames (SDD-001 §3.2); ok is
// false for an image, which has no limit, or for an asset whose facts are unusable.
func AssetDuration(a Asset, fps Rational) (int, bool) {
	n, ok := assetFrames(a, fps)
	if !ok || !n.IsInt64() {
		return 0, false
	}
	return int(n.Int64()), true
}

// assetFrames is the asset duration in project frames (SDD-001 §3.2); ok is false
// for an image, which has no limit, or for an asset whose facts are unusable.
func assetFrames(a Asset, fps Rational) (*big.Int, bool) {
	if fps.Num < 1 || fps.Den < 1 {
		return nil, false
	}
	switch a.Kind {
	case AssetVideo:
		if a.FrameRate.Num < 1 || a.FrameRate.Den < 1 || a.FrameCount < 1 {
			return nil, false
		}
		n := new(big.Int).Mul(big.NewInt(int64(a.FrameCount)), big.NewInt(int64(fps.Num)))
		n.Mul(n, big.NewInt(int64(a.FrameRate.Den)))
		d := new(big.Int).Mul(big.NewInt(int64(fps.Den)), big.NewInt(int64(a.FrameRate.Num)))
		return n.Div(n, d), true
	case AssetAudio:
		if a.SampleCount < 1 {
			return nil, false
		}
		n := new(big.Int).Mul(big.NewInt(int64(a.SampleCount)), big.NewInt(int64(fps.Num)))
		d := new(big.Int).Mul(big.NewInt(48000), big.NewInt(int64(fps.Den)))
		return n.Div(n, d), true
	case AssetImage, AssetLUT:
		// An image has no duration limit and a lookup table has no duration.
	}
	return nil, false
}

func (v *validator) clip(path, key string, it Item) {
	switch it.Fit {
	case "cover", "contain", "none":
	default:
		v.add(VShape, path+".fit", key, "fit must be cover, contain or none")
	}
	if c := it.Crop; c != nil {
		for _, s := range []struct {
			name string
			val  float64
		}{{"left", c.Left}, {"top", c.Top}, {"right", c.Right}, {"bottom", c.Bottom}} {
			if !inRange(s.val, 0, 1) {
				v.add(VShape, path+".crop."+s.name, key, "crop %s must be between 0 and 1", s.name)
			}
		}
		if c.Left+c.Right >= 1 || c.Top+c.Bottom >= 1 {
			v.add(VShape, path+".crop", key, "left + right and top + bottom must each be below 1")
		}
	}
	if !inRange(it.Brightness, -1, 1) {
		v.add(VShape, path+".brightness", key, "brightness must be between -1 and 1")
	}
	if !inRange(it.Contrast, 0, 2) {
		v.add(VShape, path+".contrast", key, "contrast must be between 0 and 2")
	}
	if !inRange(it.Saturation, 0, 2) {
		v.add(VShape, path+".saturation", key, "saturation must be between 0 and 2")
	}
	if !inRange(it.Temperature, -1, 1) {
		v.add(VShape, path+".temperature", key, "temperature must be between -1 and 1")
	}
	if !inRange(it.Tint, -1, 1) {
		v.add(VShape, path+".tint", key, "tint must be between -1 and 1")
	}
	if !inRange(it.Hue, -180, 180) {
		v.add(VShape, path+".hue", key, "hue must be between -180 and 180")
	}
	v.lut(path, key, it)
	if p, m := GradeProblem(it.Hsl, it.Wheels, it.Curves); m != "" {
		v.add(VShape, path+"."+p, key, "%s", m)
	}
}

// lut checks the lookup table reference of a clip: the asset is a lut asset of the project and the
// intensity is between 0 and 1.
func (v *validator) lut(path, key string, it Item) {
	l := it.Lut
	if l == nil {
		return
	}
	if !inRange(l.Intensity, 0, 1) {
		v.add(VShape, path+".lut.intensity", key, "lut intensity must be between 0 and 1")
	}
	if !ValidID(PrefixAsset, l.Asset) {
		v.add(VID, path+".lut.asset", key, "%q is not a valid asset identifier", l.Asset)
		return
	}
	a, ok := v.doc.Assets[l.Asset]
	switch {
	case !ok:
		v.add(VAssetRef, path+".lut.asset", key, "asset %s does not exist", l.Asset)
		v.related(l.Asset)
	case a.Kind != AssetLUT:
		v.add(VAssetRef, path+".lut.asset", key, "a lut needs an asset of kind lut, %s is %s", l.Asset, a.Kind)
		v.related(l.Asset)
	}
}

func (v *validator) shape(path, key string, it Item) {
	switch it.Shape {
	case "rect", "rounded_rect", "ellipse":
	default:
		v.add(VShape, path+".shape", key, "shape must be rect, rounded_rect or ellipse")
	}
	if !finite(it.Width) || it.Width <= 0 || !finite(it.Height) || it.Height <= 0 {
		v.add(VShape, path+".width", key, "width and height must be greater than 0")
	}
	if !finite(it.Radius) || it.Radius < 0 || !finite(it.StrokeWidth) || it.StrokeWidth < 0 {
		v.add(VShape, path+".radius", key, "radius and stroke_width cannot be negative")
	}
	v.color(path+".fill", key, it.Fill, true)
	v.color(path+".stroke_color", key, it.StrokeColor, true)
}

func (v *validator) caption(path, key string, it Item) {
	if it.Style != nil {
		v.style(path+".style", key, it.Style, true)
	}
	if n := len(it.Words); n < 1 || n > 60 {
		v.add(VShape, path+".words", key, "a caption has 1 to 60 words, not %d", n)
	}
	end := 0
	for i, w := range it.Words {
		wp := fmt.Sprintf("%s.words[%d]", path, i)
		if n := runes(w.Text); n < 1 || n > 80 {
			v.add(VShape, wp+".text", key, "word text must have 1 to 80 characters")
		}
		switch {
		case w.Start < 0 || w.Duration < 1:
			v.add(VWords, wp, key, "word start must be at least 0 and duration at least 1")
		case w.Start < end:
			v.add(
				VWords,
				wp,
				key,
				"words overlap or are out of order: word starts at %d, the previous one ends at %d",
				w.Start,
				end,
			)
		case w.Start+w.Duration > it.Duration:
			v.add(VWords, wp, key, "word ends at %d, after the item duration %d", w.Start+w.Duration, it.Duration)
		}
		if w.Start+w.Duration > end {
			end = w.Start + w.Duration
		}
	}
	prev := -1
	for i, b := range it.Breaks {
		if b < 0 || b >= len(it.Words) || b <= prev {
			v.add(VWords, fmt.Sprintf("%s.breaks[%d]", path, i), key, "breaks must be increasing word indexes")
		}
		prev = b
	}
	if it.LinkedItem != "" && !ValidID(PrefixItem, it.LinkedItem) {
		v.add(VID, path+".linked_item", key, "%q is not a valid item identifier", it.LinkedItem)
	}
}

func (v *validator) link(path, key string, it Item) {
	switch {
	case it.Link == "":
	case !ValidID(PrefixLink, it.Link):
		v.add(VID, path+".link", key, "%q is not a valid link identifier", it.Link)
	case it.Type == ItemCaption:
		v.add(VShape, path+".link", key, "link does not apply to caption items")
	}
}

func (v *validator) transitions(path, key string, it Item) {
	sum := 0
	for _, t := range []struct {
		name string
		val  *Transition
	}{{"transition_in", it.TransitionIn}, {"transition_out", it.TransitionOut}} {
		if t.val == nil {
			continue
		}
		switch t.val.Kind {
		case "fade",
			"slide_left",
			"slide_right",
			"slide_up",
			"slide_down",
			"zoom",
			"whip",
			"dip",
			"wipe",
			"spin",
			"shrink",
			"grow":
		default:
			v.add(VShape, path+"."+t.name+".kind", key, "unknown transition kind %q", t.val.Kind)
		}
		v.transitionParams(path+"."+t.name, key, t.val)
		if t.val.Duration < 1 {
			v.add(VShape, path+"."+t.name+".duration", key, "transition duration must be at least 1")
		}
		sum += t.val.Duration
	}
	if sum > it.Duration {
		v.add(VTransitions, path, key, "transition durations sum to %d, more than the duration %d", sum, it.Duration)
	}
}

// transitionParams checks the colour of a dip and the direction of a whip or a wipe.
func (v *validator) transitionParams(path, key string, t *Transition) {
	if t.Color != "" {
		if t.Kind != "dip" {
			v.add(VShape, path+".color", key, "color applies to dip transitions only")
		} else {
			v.color(path+".color", key, t.Color, true)
		}
	}
	if t.Direction != "" {
		if t.Kind != "whip" && t.Kind != "wipe" {
			v.add(VShape, path+".direction", key, "direction applies to whip and wipe transitions only")
		} else if !slices.Contains([]string{"left", "right", "up", "down"}, t.Direction) {
			v.add(VShape, path+".direction", key, "direction must be left, right, up or down")
		}
	}
}

// effects checks the effects of an item: known kinds, each once, only the parameters of the kind,
// all of them present and in range.
func (v *validator) effects(path, key string, it Item) {
	if len(it.Effects) == 0 {
		return
	}
	if len(it.Effects) > MaxEffects {
		v.add(VShape, path+".effects", key, "an item has at most %d effects", MaxEffects)
	}
	seen := map[string]bool{}
	for i, e := range it.Effects {
		p := fmt.Sprintf("%s.effects[%d]", path, i)
		params, ok := EffectParams[e.Kind]
		switch {
		case !ok:
			v.add(VShape, p+".kind", key, "unknown effect kind %q", e.Kind)
			continue
		case seen[e.Kind]:
			v.add(VShape, p+".kind", key, "effect %s is listed twice", e.Kind)
		}
		seen[e.Kind] = true
		for _, name := range []string{"amount", "angle", "radius", "threshold", "softness", "size", "speed", "seed"} {
			val, present := e.Param(name)
			switch {
			case present != slices.Contains(params, name):
				v.add(
					VShape,
					p+"."+name,
					key,
					"%s %s the parameter %s",
					e.Kind,
					map[bool]string{true: "does not take", false: "needs"}[present],
					name,
				)
			case present && !finite(val):
				v.add(VShape, p+"."+name, key, "%s must be a finite number", name)
			case present && !effectInRange(name, val):
				v.add(VShape, p+"."+name, key, "%s is out of range", name)
			}
		}
	}
}

// Param is the value of a named parameter and whether it is set.
func (e Effect) Param(name string) (float64, bool) {
	var f *float64
	var n *int
	switch name {
	case "amount":
		f = e.Amount
	case "angle":
		f = e.Angle
	case "radius":
		f = e.Radius
	case "threshold":
		f = e.Threshold
	case "softness":
		f = e.Softness
	case "size":
		n = e.Size
	case "speed":
		n = e.Speed
	case "seed":
		n = e.Seed
	}
	switch {
	case f != nil:
		return *f, true
	case n != nil:
		return float64(*n), true
	}
	return 0, false
}

// EffectRange is the range of a parameter of an effect.
func EffectRange(name string) (lo, hi float64) {
	switch name {
	case "angle":
		return -180, 180
	case "radius":
		return 1, 100
	case "size":
		return 1, 8
	case "speed":
		return 1, 30
	case "seed":
		return 0, 65535
	}
	return 0, 1
}

func effectInRange(name string, v float64) bool {
	lo, hi := EffectRange(name)
	return inRange(v, lo, hi)
}

// keyframeRange is the range of the value of an animatable property, and whether it is one.
func keyframeRange(prop string) (lo, hi float64, ok bool) {
	switch prop {
	case "x", "y", "rotation":
		return -math.MaxFloat64, math.MaxFloat64, true
	case "scale":
		return math.SmallestNonzeroFloat64, 20, true
	case "opacity":
		return 0, 1, true
	case "volume":
		return 0, 4, true
	}
	if lo, hi, ok := MaskRange(prop); ok {
		return lo, hi, true
	}
	if lo, hi, ok := CropRange(prop); ok {
		return lo, hi, true
	}
	return ColourRange(prop)
}

// keyframeTarget checks that an animatable property applies to the item.
func (v *validator) keyframeTarget(pp, key, prop string, it Item) {
	if _, _, colour := ColourRange(prop); colour && it.Type != ItemClip {
		v.add(VKeyframes, pp, key, "%s applies to clips", prop)
	}
	if _, _, mask := MaskRange(prop); mask && it.Mask == nil {
		v.add(VKeyframes, pp, key, "%s needs a mask on the item", prop)
	}
	if _, _, crop := CropRange(prop); crop && it.Type != ItemClip {
		v.add(VKeyframes, pp, key, "%s applies to clips", prop)
	}
	switch {
	case prop == "volume" && it.Type != ItemClip && it.Type != ItemAudio && it.Type != ItemComposition:
		v.add(VKeyframes, pp, key, "volume applies to items with audio")
	case prop != "volume" && it.Type == ItemAudio:
		v.add(VKeyframes, pp, key, "%s applies to visual items", prop)
	}
}

func (v *validator) keyframes(path, key string, it Item) {
	for _, prop := range sortedKeys(it.Keyframes) {
		pp := path + ".keyframes." + prop
		lo, hi, known := keyframeRange(prop)
		if !known {
			v.add(VKeyframes, pp, key, "%q is not an animatable property", prop)
			continue
		}
		v.keyframeTarget(pp, key, prop, it)
		kfs := it.Keyframes[prop]
		if len(kfs) > 100 {
			v.add(VKeyframes, pp, key, "%d keyframes exceed the limit of 100", len(kfs))
		}
		for i, k := range kfs {
			kp := fmt.Sprintf("%s[%d]", pp, i)
			if k.T < 0 || k.T > it.Duration {
				v.add(VKeyframes, kp+".t", key, "t %d is outside 0 to %d", k.T, it.Duration)
			}
			if i > 0 && k.T <= kfs[i-1].T {
				v.add(VKeyframes, kp+".t", key, "t must be strictly increasing")
			}
			if !inRange(k.Value, lo, hi) {
				v.add(VKeyframes, kp+".value", key, "value %v is not valid for %s", k.Value, prop)
			}
			switch k.Easing {
			case "linear", "ease_in", "ease_out", "ease_in_out", "hold":
				if len(k.Bezier) > 0 {
					v.add(VKeyframes, kp+".bezier", key, "bezier needs the easing %q", EasingBezier)
				}
			case EasingBezier:
				if !BezierValid(k.Bezier) {
					v.add(VKeyframes, kp+".bezier", key,
						"bezier must be four numbers, x1 and x2 from 0 to 1, y1 and y2 from %v to %v",
						BezierMinY, BezierMaxY)
				}
			default:
				v.add(VKeyframes, kp+".easing", key, "unknown easing %q", k.Easing)
			}
		}
	}
}

func (v *validator) slot(path, key string, it Item) {
	s := it.Slot
	if s == nil {
		return
	}
	if !slotNamePattern.MatchString(s.Name) {
		v.add(VSlot, path+".slot.name", key, "slot name %q must match [a-z][a-z0-9_]{0,39}", s.Name)
	}
	var want ItemType
	switch s.Kind {
	case "media":
		if it.Type != ItemClip && it.Type != ItemAudio {
			v.add(VSlot, path+".slot.kind", key, "a media slot belongs on a clip or audio item, not %s", it.Type)
		}
	case "text":
		want = ItemText
	case "caption":
		want = ItemCaption
	default:
		v.add(VShape, path+".slot.kind", key, "slot kind must be media, text or caption")
	}
	if want != "" && it.Type != want {
		v.add(VSlot, path+".slot.kind", key, "a %s slot belongs on a %s item, not %s", s.Kind, want, it.Type)
	}
	if runes(s.Description) > 200 {
		v.add(VShape, path+".slot.description", key, "description has more than 200 characters")
	}
	if s.DurationPolicy != "fit" && s.DurationPolicy != "stretch_timeline" {
		v.add(VShape, path+".slot.duration_policy", key, "duration_policy must be fit or stretch_timeline")
	}
}

func (v *validator) slotNames() {
	seen := map[string]string{}
	for _, k := range sortedKeys(v.doc.Items) {
		s := v.doc.Items[k].Slot
		if s == nil {
			continue
		}
		if other, dup := seen[s.Name]; dup {
			v.add(VSlot, "items."+k+".slot.name", k, "slot name %q is already used by item %s", s.Name, other)
		} else {
			seen[s.Name] = k
		}
	}
}

func (v *validator) overlaps() {
	byTrack := map[string][]string{}
	for _, k := range sortedKeys(v.doc.Items) {
		it := v.doc.Items[k]
		if it.Start < 0 || it.Duration < 1 {
			continue
		}
		byTrack[it.Track] = append(byTrack[it.Track], k)
	}
	for _, track := range sortedKeys(byTrack) {
		keys := byTrack[track]
		sort.SliceStable(keys, func(i, j int) bool { return v.doc.Items[keys[i]].Start < v.doc.Items[keys[j]].Start })
		blocker, blockEnd := "", 0
		for _, k := range keys {
			it := v.doc.Items[k]
			if blocker != "" && it.Start < blockEnd {
				last := min(blockEnd, it.Start+it.Duration) - 1
				v.add(
					VOverlap,
					"items."+k,
					k,
					"frames %d to %d on track %s are occupied by both %s and %s",
					it.Start,
					last,
					track,
					blocker,
					k,
				)
				v.related(blocker, it.Track)
			}
			if end := it.Start + it.Duration; blocker == "" || end > blockEnd {
				blocker, blockEnd = k, end
			}
		}
	}
}
