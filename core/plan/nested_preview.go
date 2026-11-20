package plan

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The processed sound of an item inside a composition (SDD-004 section 7.1): the preview plays the file of the
// item's source with its chain and its noise reduction, like the export does, and the same key names it wherever
// the composition sits. The source is the entry compositionAudio makes for the export (the speed of the
// composition item already in it), without the envelope, the track and the position.

// nestedEntry is the entry of the sound of the composition item that itemID starts with, over the frames [from, to)
// of the timeline of doc, and the document of the timeline that holds its asset. The position is kept.
func nestedEntry(
	doc *timeline.Document, opts Options, itemID string, from, to int,
) (AudioSource, *timeline.Document, bool) {
	top, _, nested := strings.Cut(itemID, "/")
	if !nested || opts.Nested == nil {
		return AudioSource{}, nil, false
	}

	if it, ok := doc.Items[top]; !ok || it.Type != timeline.ItemComposition {
		return AudioSource{}, nil, false
	}

	srcs := compositionAudio(CompositionSubject(doc, top), from, to, opts, []string{doc.ID})

	i := slices.IndexFunc(srcs, func(s AudioSource) bool { return s.ItemID == itemID })
	if i < 0 {
		return AudioSource{}, nil, false
	}

	owner, ok := opts.Nested(srcs[i].Timeline)
	if !ok || owner == nil {
		return AudioSource{}, nil, false
	}

	return srcs[i], owner, true
}

// NestedPreviewSource is the source that the file of an item inside a composition holds over the frames [from, to)
// of the timeline of doc, with the document of the timeline that holds its asset (the document that PreviewKey and
// PreviewSubject take). itemID is the id of the entry in the plan: the composition item, a slash, and the id of the
// item in the nested timeline. Like PreviewSource it has nothing of the envelope, of the track and of the position.
// It is pure.
func NestedPreviewSource(
	doc *timeline.Document, opts Options, itemID string, from, to int,
) (AudioSource, *timeline.Document, bool) {
	src, owner, ok := nestedEntry(doc, opts, itemID, from, to)
	if !ok {
		return AudioSource{}, nil, false
	}

	src.StartSample, src.Gain, src.Voice = 0, nil, ""
	src.Track, src.BusChain = "", nil

	return src, owner, true
}

// CompositionSubject is a document that holds only a composition item and its track with no volume, solo or mute,
// so that the service keeps no whole project to plan the sound of one composition later.
func CompositionSubject(doc *timeline.Document, itemID string) *timeline.Document {
	it := doc.Items[itemID]

	sub := &timeline.Document{
		SchemaVersion: doc.SchemaVersion, ID: doc.ID, Name: doc.Name, Settings: doc.Settings,
		Assets: map[string]timeline.Asset{},
		Tracks: map[string]timeline.Track{},
		Items:  map[string]timeline.Item{it.ID: it},
	}

	if t, ok := doc.Tracks[it.Track]; ok {
		t.Muted, t.Solo, t.Volume, t.Processors = false, false, nil, nil
		sub.Tracks[t.ID] = t
	}

	return sub
}

// nestedProcessed is the ProcessedAudio of a nested source of the plan, or nil when it has no chain and no noise
// reduction. The file holds the whole sound of the item inside the composition item, in whole frames of the
// timeline of doc: it starts at the frame the source starts in and ends at the one it ends in.
func nestedProcessed(doc *timeline.Document, opts Options, s *AudioSource) *ProcessedAudio {
	if len(s.Chain) == 0 && round3(s.Denoise) <= 0 {
		return nil
	}

	top, _, _ := strings.Cut(s.ItemID, "/")

	it, ok := doc.Items[top]
	if !ok {
		return nil
	}

	whole, owner, ok := nestedEntry(doc, opts, s.ItemID, it.Start, it.Start+it.Duration)
	if !ok {
		return nil
	}

	fps := doc.Settings.FPS
	per := SampleRate * int64(fps.Den)
	start := int(whole.StartSample * int64(fps.Num) / per)
	end := int((whole.StartSample + whole.Samples) * int64(fps.Num) / per)

	if (whole.StartSample+whole.Samples)*int64(fps.Num)%per != 0 {
		end++
	}

	if end <= start {
		return nil
	}

	whole.StartSample, whole.Gain, whole.Voice = 0, nil, ""
	whole.Track, whole.BusChain = "", nil

	return &ProcessedAudio{Key: PreviewKey(owner, &whole), Start: start, Frames: end - start}
}
