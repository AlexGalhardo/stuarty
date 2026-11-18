package plan

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Kinds of SnapPoint (SDD-012 §7.8).
const (
	SnapTimelineStart = "timeline_start"
	SnapItemStart     = "item_start"
	SnapItemEnd       = "item_end"
	SnapCaptionWord   = "caption_word"
)

// SnapPoint is a frame a dragged edge may snap to.
type SnapPoint struct {
	Frame int    `json:"frame"`
	Kind  string `json:"kind"`
	Item  string `json:"item,omitempty"`
	Track string `json:"track,omitempty"`
}

// SnapLimit bounds a trim of one item: the earliest frame its start edge can move
// to and the latest frame its end edge can reach.
type SnapLimit struct {
	TrimStartMin int `json:"trim_start_min"`
	TrimEndMax   int `json:"trim_end_max"`
}

// Snap is the snap structure of SDD-012 §7.8, minus the revision, which the service
// adds.
type Snap struct {
	Points []SnapPoint          `json:"points"`
	Limits map[string]SnapLimit `json:"limits"`
}

var snapKindOrder = map[string]int{SnapTimelineStart: 0, SnapItemStart: 1, SnapItemEnd: 2, SnapCaptionWord: 3}

// SnapPoints lists the points of the document sorted by frame, then kind, then
// item, and the trim limits of every item: bounded by the neighbours on its track
// and, for a clip or audio item of a video or audio asset, by the source (the start
// edge cannot go earlier than source_in allows, the end edge not past the end of the
// asset). Caption word starts and ends are included only with words.
func SnapPoints(doc *timeline.Document, words bool) Snap {
	out := Snap{
		Points: []SnapPoint{{Frame: 0, Kind: SnapTimelineStart}},
		Limits: make(map[string]SnapLimit, len(doc.Items)),
	}
	byTrack := map[string][]string{}
	for id := range doc.Items {
		it := doc.Items[id]
		byTrack[it.Track] = append(byTrack[it.Track], id)
		out.Points = append(out.Points,
			SnapPoint{Frame: it.Start, Kind: SnapItemStart, Item: id, Track: it.Track},
			SnapPoint{Frame: it.Start + it.Duration, Kind: SnapItemEnd, Item: id, Track: it.Track},
		)
		if words && it.Type == timeline.ItemCaption {
			for _, w := range it.Words {
				out.Points = append(out.Points,
					SnapPoint{Frame: it.Start + w.Start, Kind: SnapCaptionWord, Item: id, Track: it.Track},
					SnapPoint{Frame: it.Start + w.Start + w.Duration, Kind: SnapCaptionWord, Item: id, Track: it.Track},
				)
			}
		}
	}
	slices.SortFunc(out.Points, func(a, b SnapPoint) int {
		if a.Frame != b.Frame {
			return a.Frame - b.Frame
		}
		if a.Kind != b.Kind {
			return snapKindOrder[a.Kind] - snapKindOrder[b.Kind]
		}
		if c := strings.Compare(a.Item, b.Item); c != 0 {
			return c
		}
		return 0
	})
	out.Points = slices.CompactFunc(out.Points, func(a, b SnapPoint) bool { return a == b })

	for _, ids := range byTrack {
		slices.SortFunc(ids, func(a, b string) int {
			if d := doc.Items[a].Start - doc.Items[b].Start; d != 0 {
				return d
			}
			return strings.Compare(a, b)
		})
		for i, id := range ids {
			it := doc.Items[id]
			lim := SnapLimit{TrimStartMin: 0, TrimEndMax: timeline.MaxFrames}
			if i > 0 {
				prev := doc.Items[ids[i-1]]
				lim.TrimStartMin = prev.Start + prev.Duration
			}
			if i+1 < len(ids) {
				lim.TrimEndMax = doc.Items[ids[i+1]].Start
			}
			lim = sourceLimit(doc, &it, lim)
			out.Limits[id] = lim
		}
	}
	return out
}

// sourceLimit narrows the trim limits of a clip or audio item to the asset: how far its start can
// move earlier and its end later before the source range leaves the asset.
func sourceLimit(doc *timeline.Document, it *timeline.Item, lim SnapLimit) SnapLimit {
	total, ok := sourceFrames(doc, it)
	if !ok || it.Freeze {
		return lim
	}

	sp := it.Speed
	back, forward := it.SourceIn, total-it.SourceIn

	if it.Reverse {
		// The start of the output is the end of the source range (SDD-004 section 3.12).
		back, forward = total-it.SourceIn-it.SourceSpan(), it.SourceIn+it.SourceSpan()
	}

	lim.TrimStartMin = max(lim.TrimStartMin, it.Start-max(back, 0)*sp.Den/sp.Num)
	lim.TrimEndMax = min(lim.TrimEndMax, it.Start+max(forward*sp.Den/sp.Num, 0))

	return lim
}

// sourceFrames is the asset duration in project frames of a clip or audio item that
// reads from a video or audio asset.
func sourceFrames(doc *timeline.Document, it *timeline.Item) (int, bool) {
	if it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio || it.Speed.Num < 1 || it.Speed.Den < 1 {
		return 0, false
	}
	a, ok := doc.Assets[it.Asset]
	if !ok || a.Kind == timeline.AssetImage {
		return 0, false
	}
	return timeline.AssetDuration(a, doc.Settings.FPS)
}
