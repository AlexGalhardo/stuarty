package transcript

import (
	"cmp"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Word is a transcript word on the asset's time axis, in frames, half-open.
type Word struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Range is where an item of the timeline shows a word, in timeline frames, half-open.
type Range struct {
	ItemID  string `json:"item_id"`
	TrackID string `json:"track_id"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

// WordRanges is a word with the ranges of the timeline that show it. A word with no range was cut,
// or lies outside every item.
type WordRanges struct {
	Index  int     `json:"index"`
	Text   string  `json:"text"`
	Start  int     `json:"start"`
	End    int     `json:"end"`
	Ranges []Range `json:"ranges"`
}

// View is the result of Map.
type View struct {
	Words []WordRanges `json:"words"`
	// Skipped are the items that show the asset and cannot map a frame of the timeline to a frame of the
	// source: an item with a speed curve or one that plays backwards.
	Skipped []string `json:"skipped_items"`
}

// Shows reports whether an item plays the sound or the picture of the asset: a clip or an audio item of
// it, not a freeze frame (which shows one source frame and has no sound).
func Shows(it *timeline.Item, assetID string) bool {
	return (it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio) && it.Asset == assetID && !it.Freeze
}

// Follows reports whether the item plays its source forward at a constant speed, so that a source
// frame maps to one frame of the timeline.
func Follows(it *timeline.Item) bool { return !it.Reverse && len(it.SpeedCurve) == 0 }

// SourceRange is the visible source range of an item, [SourceIn, SourceIn + ceil(duration x speed)),
// in asset frames.
func SourceRange(it *timeline.Item) (start, end int) {
	return it.SourceIn, it.SourceIn + it.SourceSpan()
}

func ceilDiv(n, d int) int { return (n + d - 1) / d }

// Locate maps the source range [from, to) of an item to the timeline, as caption.from_transcript does
// (SDD-002 C.40): the start rounds up, the end is capped at the visible range and at the end of the
// item, and the result is at least one frame. ok is false when from is outside the visible range.
func Locate(it *timeline.Item, from, to int) (start, end int, ok bool) {
	si, srcEnd := SourceRange(it)
	if from < si || from >= srcEnd {
		return 0, 0, false
	}
	num, den := it.Speed.Num, it.Speed.Den
	at := func(p int) int { return it.Start + ceilDiv((p-si)*den, num) }
	itemEnd := it.Start + it.Duration
	start = at(from)
	if start >= itemEnd {
		return 0, 0, false
	}
	end = min(max(at(min(to, srcEnd)), start+1), itemEnd)
	return start, end, true
}

// Map places the words of a transcript on the timeline: for each word, the range of every item of the
// asset that shows it. It is a pure function of the document and the transcript and is recomputed after
// every edit.
func Map(doc *timeline.Document, assetID string, words []Word) View {
	var items []timeline.Item
	var skipped []string
	for id := range doc.Items {
		it := doc.Items[id]
		if !Shows(&it, assetID) {
			continue
		}
		if !Follows(&it) {
			skipped = append(skipped, it.ID)
			continue
		}
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b timeline.Item) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(a.ID, b.ID))
	})
	slices.Sort(skipped)

	view := View{Words: make([]WordRanges, len(words)), Skipped: skipped}
	if view.Skipped == nil {
		view.Skipped = []string{}
	}
	for i, w := range words {
		wr := WordRanges{Index: i, Text: w.Text, Start: w.Start, End: w.End, Ranges: []Range{}}
		for j := range items {
			if s, e, ok := Locate(&items[j], w.Start, w.End); ok {
				wr.Ranges = append(wr.Ranges, Range{ItemID: items[j].ID, TrackID: items[j].Track, Start: s, End: e})
			}
		}
		view.Words[i] = wr
	}
	return view
}
