// Package motion holds the pure rules of the hand-off from an external motion engine (SDD-011 section 15, epic 33):
// what a render must fit (the brief), what does not fit in a file that came back (the check) and which clips of a
// timeline read an earlier render of the same animation. It does no I/O: the service reads the project, the brand
// kit and the frames and gives them in.
package motion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// MaxFrames is the longest range a brief covers: a motion clip is a short piece (SDD-011 section 15.1).
const MaxFrames = 3600

// RuleError is a refusal with a stable reason; the service maps it to the error code of the surface.
type RuleError struct {
	Path   string // from, until or track_id
	Reason string // range_empty, range_negative, range_too_long, track_unknown or track_not_visual
	Detail string
}

func (e *RuleError) Error() string { return fmt.Sprintf("%s: %s (%s)", e.Path, e.Detail, e.Reason) }

// Rect is a rectangle of the frame in project pixels.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Margins are the pixels at each side of the frame that the platforms cover.
type Margins struct {
	Left   int `json:"left"`
	Right  int `json:"right"`
	Top    int `json:"top"`
	Bottom int `json:"bottom"`
}

// TrackInfo is the visual track a clip will go on. Overlay is true when another visual track lies below it, which
// is when the render needs an alpha channel.
type TrackInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Overlay bool   `json:"overlay"`
}

// Caption is a caption that shows inside the range; Start and End are frames relative to the start of the range
// and Text is untrusted wherever it is shown.
type Caption struct {
	ItemID string `json:"item_id"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Text   string `json:"text"`
}

// Brief is what a render by another tool must fit (the parts the document can tell; the service adds the brand kit
// and the reference frames).
type Brief struct {
	FromFrame      int               `json:"from_frame"`
	UntilFrame     int               `json:"until_frame"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	FrameRate      timeline.Rational `json:"frame_rate"`
	DurationFrames int               `json:"duration_frames"`
	SafeArea       Rect              `json:"safe_area"`
	Margins        Margins           `json:"margins"`
	Track          *TrackInfo        `json:"track,omitempty"`
	NeedsAlpha     bool              `json:"needs_alpha"`
	Captions       []Caption         `json:"captions"`
}

// Build makes the brief of the range [from, until) of the document, for a clip that goes on trackID (empty: none).
func Build(doc *timeline.Document, from, until int, trackID string) (Brief, *RuleError) {
	switch {
	case from < 0:
		return Brief{}, &RuleError{Path: "from", Reason: "range_negative", Detail: "the range starts before the timeline"}
	case until <= from:
		return Brief{}, &RuleError{
			Path: "until", Reason: "range_empty",
			Detail: fmt.Sprintf("the range from frame %d to frame %d has no frames", from, until),
		}
	case until-from > MaxFrames:
		return Brief{}, &RuleError{
			Path: "until", Reason: "range_too_long",
			Detail: fmt.Sprintf("the range has %d frames; a motion clip has at most %d", until-from, MaxFrames),
		}
	}

	st := doc.Settings
	area := plan.SafeArea(st.Width, st.Height)
	sa := Rect{X: int(area.X), Y: int(area.Y), Width: int(area.W), Height: int(area.H)}

	b := Brief{
		FromFrame: from, UntilFrame: until, Width: st.Width, Height: st.Height, FrameRate: st.FPS,
		DurationFrames: until - from, SafeArea: sa,
		Margins:  Margins{Left: sa.X, Right: st.Width - sa.X - sa.Width, Top: sa.Y, Bottom: st.Height - sa.Y - sa.Height},
		Captions: []Caption{},
	}

	if trackID != "" {
		t, ok := doc.Tracks[trackID]
		if !ok {
			return Brief{}, &RuleError{Path: "track_id", Reason: "track_unknown", Detail: "the project has no track " + trackID}
		}

		if t.Kind != timeline.TrackVisual {
			return Brief{}, &RuleError{
				Path: "track_id", Reason: "track_not_visual", Detail: "track " + trackID + " is an audio track",
			}
		}

		overlay := false

		for id := range doc.Tracks {
			if o := doc.Tracks[id]; o.Kind == timeline.TrackVisual && o.Order > t.Order {
				overlay = true

				break
			}
		}

		b.Track = &TrackInfo{ID: t.ID, Name: t.Name, Overlay: overlay}
		b.NeedsAlpha = overlay
	}

	b.Captions = captionsIn(doc, from, until)

	return b, nil
}

// captionsIn lists the caption items that show inside [from, until), by start then id.
func captionsIn(doc *timeline.Document, from, until int) []Caption {
	out := []Caption{}

	for id := range doc.Items {
		it := doc.Items[id]
		if it.Type != timeline.ItemCaption || it.Disabled || it.Start >= until || it.Start+it.Duration <= from {
			continue
		}

		words := make([]string, len(it.Words))
		for i, w := range it.Words {
			words[i] = w.Text
		}

		out = append(out, Caption{
			ItemID: it.ID, Start: it.Start - from, End: it.Start + it.Duration - from, Text: strings.Join(words, " "),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}

		return out[i].ItemID < out[j].ItemID
	})

	return out
}
