package lint

import (
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Smallest ranges the layer rules report, in frames.
const (
	minGapFrames   = 3
	minBlackFrames = 1
	maxSliver      = 2
	minDimension   = 16
	maxDimension   = 4096
	// edgeSlack absorbs the rounding of the safe rectangle to whole pixels:
	// a caption placed on its edge must not be reported.
	edgeSlack = 0.5
)

type check struct {
	doc   *timeline.Document
	env   Env
	out   []Finding
	items []*timeline.Item // by id
}

func (c *check) add(code, item string, r *Range, msg, hint string) {
	sev := SeverityWarning
	if Blocking(code) {
		sev = SeverityError
	}

	c.out = append(c.out, Finding{Code: code, Severity: sev, Item: item, Range: r, Message: msg, Hint: hint})
}

func (c *check) sorted() []*timeline.Item {
	if c.items == nil {
		for _, id := range slices.Sorted(maps.Keys(c.doc.Items)) {
			it := c.doc.Items[id]
			c.items = append(c.items, &it)
		}
	}

	return c.items
}

func itemRange(it *timeline.Item) *Range {
	return &Range{Start: int64(it.Start), End: int64(it.Start + it.Duration)}
}

func (c *check) visibleTrack(id string) bool {
	t, ok := c.doc.Tracks[id]
	return ok && c.doc.TrackVisible(id) && !t.Hidden
}

func (c *check) emptyTimeline() {
	if len(c.doc.Items) == 0 {
		c.add(CodeEmptyTimeline, "", nil, "the project has no item, so there is nothing to export.",
			"import media and place it with clip.add")
	}
}

func (c *check) dimensions() {
	w, h := c.doc.Settings.Width, c.doc.Settings.Height
	for _, d := range []struct {
		name string
		v    int
	}{{"width", w}, {"height", h}} {
		if d.v%2 != 0 || d.v < minDimension || d.v > maxDimension {
			c.add(
				CodeOddDimensions,
				"",
				nil,
				fmt.Sprintf(
					"settings.%s is %d; it must be even and between %d and %d.",
					d.name,
					d.v,
					minDimension,
					maxDimension,
				),
				"use settings.set with even values",
			)
		}
	}
}

// media reports items whose asset is missing from the project or not ready.
func (c *check) media() {
	for _, it := range c.sorted() {
		if it.Asset == "" || (c.doc.Template != nil && it.Asset == timeline.PlaceholderAsset) {
			continue
		}

		if _, ok := c.doc.Assets[it.Asset]; !ok {
			c.add(CodeMediaMissing, it.ID, itemRange(it),
				fmt.Sprintf("asset %s is not in the project; the item cannot be rendered.", it.Asset),
				"import the file again, then replace the item's asset")

			continue
		}

		if c.env.Unready != nil && c.env.Unready(it.Asset) {
			c.add(CodeMediaUnready, it.ID, itemRange(it),
				fmt.Sprintf("the normalized media of asset %s is not ready; it is being rebuilt.", it.Asset),
				"wait for the asset to be ready and run lint again")
		}
	}
}

// slots reports required slots that were never filled. A template draft is exempt:
// its slots are meant to be open. An instance (template.instance) is not. An optional
// slot (required: false) left on the placeholder is quiet: the plan skips its item.
func (c *check) slots() {
	if c.doc.Template != nil && !c.doc.Template.Instance {
		return
	}

	for _, it := range c.sorted() {
		open := it.Slot != nil && it.Slot.Required && !it.Slot.Filled
		optional := it.Slot != nil && !it.Slot.Required
		if open || (it.Asset == timeline.PlaceholderAsset && !optional) {
			c.add(
				CodeSlotUnfilled,
				it.ID,
				itemRange(it),
				"a required template slot is not filled.",
				"fill it with slot.fill",
			)
		}
	}
}

type segment struct{ start, end int64 }

// layers finds the ranges where no video layer shows (L_GAP, with text over
// the background) and where nothing shows at all (L_BLACK_FRAMES).
func (c *check) layers() {
	var end int64

	cuts := map[int64]bool{0: true}

	for _, it := range c.sorted() {
		end = max(end, int64(it.Start+it.Duration))
	}

	if end == 0 {
		return
	}

	cuts[end] = true

	for _, it := range c.sorted() {
		if it.Type != timeline.ItemAudio && c.visibleTrack(it.Track) {
			cuts[min(int64(it.Start), end)] = true
			cuts[min(int64(it.Start+it.Duration), end)] = true
		}
	}

	points := slices.Sorted(maps.Keys(cuts))

	var gaps, blacks []segment

	for i := 0; i+1 < len(points); i++ {
		video, text := c.activeAt(points[i])
		seg := segment{points[i], points[i+1]}

		switch {
		case video > 0:
		case text > 0:
			gaps = appendMerged(gaps, seg)
		default:
			blacks = appendMerged(blacks, seg)
		}
	}

	bg := string(c.doc.Settings.Background)

	for _, s := range gaps {
		if s.end-s.start >= minGapFrames {
			c.add(
				CodeGap,
				"",
				&Range{s.start, s.end},
				fmt.Sprintf(
					"frames %d-%d have no visible clip or shape; only text shows over the background color %s.",
					s.start,
					s.end-1,
					bg,
				),
				"fill the range, or close it with timeline.close_gap",
			)
		}
	}

	if !c.hasVisualTrack() {
		blacks = nil // an audio-only project has no picture to be black
	}

	for _, s := range blacks {
		if s.end-s.start >= minBlackFrames {
			c.add(
				CodeBlackFrames,
				"",
				&Range{s.start, s.end},
				fmt.Sprintf(
					"frames %d-%d have no visible item; they render as the background color %s.",
					s.start,
					s.end-1,
					bg,
				),
				"fill the range with a clip or a shape, close it with timeline.close_gap, or shorten the timeline",
			)
		}
	}
}

func (c *check) hasVisualTrack() bool {
	for _, t := range c.doc.Tracks {
		if t.Kind == timeline.TrackVisual {
			return true
		}
	}

	return false
}

func appendMerged(list []segment, s segment) []segment {
	if n := len(list); n > 0 && list[n-1].end == s.start {
		list[n-1].end = s.end
		return list
	}

	return append(list, s)
}

// activeAt counts the visible video layers (clip, shape) and text layers
// (text, caption) active at frame t. A hidden track counts as absent.
func (c *check) activeAt(t int64) (video, text int) {
	for _, it := range c.sorted() {
		if it.Type == timeline.ItemAudio || !c.visibleTrack(it.Track) {
			continue
		}

		if t < int64(it.Start) || t >= int64(it.Start+it.Duration) {
			continue
		}

		if it.Type == timeline.ItemText || it.Type == timeline.ItemCaption {
			text++
		} else {
			video++
		}
	}

	return video, text
}

// silent reports a project in which no source would be audible.
func (c *check) silent() {
	if len(c.doc.Items) == 0 {
		return
	}

	if slices.ContainsFunc(c.sorted(), c.audible) {
		return
	}

	c.add(CodeSilentExport, "", nil, "no source would be audible: the export has no sound.",
		"add an audio item or unmute a clip")
}

func (c *check) audible(it *timeline.Item) bool {
	if it.Volume <= 0 || it.AudioMuted {
		return false
	}

	if _, ok := c.doc.Tracks[it.Track]; ok && (!c.doc.TrackSounds(it.Track) || c.doc.TrackGain(it.Track) <= 0) {
		return false
	}

	if it.Type == timeline.ItemAudio {
		return true
	}

	return it.Type == timeline.ItemClip && c.doc.Assets[it.Asset].HasAudio
}

func (c *check) slivers() {
	for _, it := range c.sorted() {
		switch it.Type {
		case timeline.ItemClip, timeline.ItemAudio, timeline.ItemCaption:
		default:
			continue
		}

		if it.Duration <= maxSliver {
			c.add(
				CodeSliver,
				it.ID,
				itemRange(it),
				fmt.Sprintf(
					"the %s lasts %d frame(s), which is usually the result of a trim or split at the wrong time.",
					it.Type,
					it.Duration,
				),
				"delete it or trim the neighbour",
			)
		}
	}
}

// sourceRange reports clips that read past the end of the normalized media
// that is really in the store (an inconsistent store, never a valid document).
func (c *check) sourceRange() {
	if c.env.MediaFrames == nil {
		return
	}

	fps := c.doc.Settings.FPS

	for _, it := range c.sorted() {
		a, ok := c.doc.Assets[it.Asset]
		if it.Type != timeline.ItemClip || !ok || a.Kind == timeline.AssetImage || a.FrameRate.Num == 0 ||
			it.Speed.Den == 0 {
			continue
		}

		actual, known := c.env.MediaFrames(it.Asset)
		if !known {
			continue
		}

		span := (int64(it.Duration)*int64(it.Speed.Num) + int64(it.Speed.Den) - 1) / int64(it.Speed.Den)
		if it.Freeze {
			span = 1
		}
		last := int64(it.SourceIn) + span - 1
		srcLast := last * int64(fps.Den) * int64(a.FrameRate.Num) / (int64(fps.Num) * int64(a.FrameRate.Den))

		if srcLast >= int64(actual) {
			c.add(
				CodeSourceRange,
				it.ID,
				itemRange(it),
				fmt.Sprintf(
					"the clip reads source frame %d of asset %s, but the media in the store has %d frames.",
					srcLast,
					it.Asset,
					actual,
				),
				"re-import the file",
			)
		}
	}
}

// box is a rectangle in project pixels.
type box struct{ x0, y0, x1, y1 float64 }

// itemBox is the bounding box of the item after placement and rotation.
func (c *check) itemBox(it *timeline.Item, w, h float64, safe box) box {
	st := c.doc.Settings
	cx, cy := float64(st.Width)/2, float64(st.Height)/2

	if it.X != nil {
		cx = *it.X
	}

	if it.Y != nil {
		cy = *it.Y
	}

	if it.Type == timeline.ItemCaption {
		switch c.captionPosition(it) {
		case "bottom":
			cx, cy = float64(st.Width)/2, safe.y1-h/2
		case "top":
			cx, cy = float64(st.Width)/2, safe.y0+h/2
		case "middle":
			cx, cy = float64(st.Width)/2, float64(st.Height)/2
		}
	}

	rad := it.Rotation * math.Pi / 180
	bw := math.Abs(w*math.Cos(rad)) + math.Abs(h*math.Sin(rad))
	bh := math.Abs(w*math.Sin(rad)) + math.Abs(h*math.Cos(rad))

	return box{cx - bw/2, cy - bh/2, cx + bw/2, cy + bh/2}
}

// captionPosition is the effective position: the item's style over the
// track's caption_style over "bottom" (SDD-001 §3.7).
func (c *check) captionPosition(it *timeline.Item) string {
	if it.Style != nil && it.Style.Position != nil {
		return *it.Style.Position
	}

	if t, ok := c.doc.Tracks[it.Track]; ok && t.CaptionStyle != nil && t.CaptionStyle.Position != nil {
		return *t.CaptionStyle.Position
	}

	return "bottom"
}
