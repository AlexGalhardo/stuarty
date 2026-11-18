package command

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "caption.from_transcript",
		Summary: "Creates captions for the speech of a clip from its transcript.",
		Description: "Groups the transcript words of the visible range of the clip into captions of at most max_words " +
			"words and max_chars characters, split at pauses longer than max_gap, and links them to the clip. With " +
			"replace (the default) the captions already linked to the clip on that track are removed first. The " +
			"transcript is supplied by the service in resolved; the command never starts a job.",
		Example: `{"clip_item_id": "itm_m1n2p3q4r5", "track_id": "trk_a1b2c3d4e5", "max_words": 5}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists,
			CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCaptionFromTranscript))

	Register(Define(Spec{
		Name:    "caption.retime_from_clip",
		Summary: "Retimes the captions linked to a clip after the clip was moved, trimmed or sped up.",
		Description: "Maps the transcript words of the visible range to the timeline again and gives them to the " +
			"linked captions in order, each taking as many words as it has. Edited texts are kept when the word " +
			"count is unchanged. The transcript is supplied by the service in resolved.",
		Example: `{"clip_item_id": "itm_m1n2p3q4r5"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCaptionRetime))
}

// ResolvedWord is a transcript word on the asset's time axis, in frames, half-open.
type ResolvedWord struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// ResolvedWords is the transcript the service puts in `resolved` (A.9).
type ResolvedWords struct {
	Words []ResolvedWord `json:"words" desc:"Words in order, not overlapping, in asset frames, half-open."`
}

func checkResolvedWords(r *ResolvedWords) *Error {
	if r == nil {
		return Errorf(CodeValidation, "args.resolved is required.").
			WithHint("Run this through the service, which fills resolved from the transcript; " +
				"run the transcribe job first if there is none.").WithPath("args.resolved")
	}
	prevEnd := 0
	for i, w := range r.Words {
		path := fmt.Sprintf("args.resolved.words[%d]", i)
		bad := func(format string, a ...any) *Error {
			return Errorf(CodeValidation, format, a...).WithHint("Send words in order without overlap.").WithPath(path)
		}
		switch {
		case chars(w.Text) < 1 || chars(w.Text) > 80:
			return bad("%s has a text of %d characters; a word has 1 to 80.", path, chars(w.Text))
		case strings.ContainsFunc(w.Text, isSpace):
			return bad("%s text contains whitespace; a word has none.", path)
		case w.Start < 0:
			return bad("%s starts at %d, before frame 0.", path, w.Start)
		case w.End <= w.Start:
			return bad("%s ends at %d before it starts at %d.", path, w.End, w.Start)
		case w.Start < prevEnd:
			return bad("%s starts at %d before the previous word ends at %d.", path, w.Start, prevEnd)
		}
		prevEnd = w.End
	}
	return nil
}

// mappedWord is a transcript word on the timeline: absolute frames, half-open.
type mappedWord struct {
	text       string
	start, end int
}

// mapTranscript is postcondition 1 of C.40: the words that start inside the visible
// source range of the clip, on the timeline.
func mapTranscript(it *timeline.Item, words []ResolvedWord) []mappedWord {
	num, den, si := it.Speed.Num, it.Speed.Den, it.SourceIn
	srcEnd := si + ceilDiv(it.Duration*num, den)
	clipEnd := itemEnd(it)
	at := func(p int) int { return it.Start + ceilDiv((p-si)*den, num) }
	var out []mappedWord
	prevEnd := 0
	for _, w := range words {
		if w.Start < si || w.Start >= srcEnd {
			continue
		}
		start := max(at(w.Start), prevEnd)
		if start >= clipEnd {
			continue
		}
		end := min(max(at(min(w.End, srcEnd)), start+1), clipEnd)
		out = append(out, mappedWord{w.Text, start, end})
		prevEnd = end
	}
	return out
}

// toWords converts mapped words to caption words relative to the first one.
func toWords(ws []mappedWord) (start, duration int, words []timeline.Word) {
	start, duration = ws[0].start, ws[len(ws)-1].end-ws[0].start
	for _, w := range ws {
		words = append(words, timeline.Word{Text: w.text, Start: w.start - start, Duration: w.end - w.start})
	}
	return start, duration, words
}

func noWordInRange(it *timeline.Item) *Error {
	end := it.SourceIn + ceilDiv(it.Duration*it.Speed.Num, it.Speed.Den) - 1
	return Errorf(CodeInvalidState, "The transcript has no word between source frames %d and %d of clip %s.",
		it.SourceIn, end, it.ID).
		WithHint("Check that the clip contains speech, or widen the clip with item.trim.")
}

type captionFromTranscriptArgs struct {
	ClipItemID string         `json:"clip_item_id"        desc:"A clip of a video asset with audio, or an audio item."                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID    string         `json:"track_id"            desc:"A visual track that receives the captions."                                      pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	MaxWords   *int           `json:"max_words,omitempty" desc:"Words per caption, default 5."                                                                                                          min:"1" max:"60"`
	MaxChars   *int           `json:"max_chars,omitempty" desc:"Characters per caption, default 28."                                                                                                    min:"1" max:"500"`
	MaxGap     Time           `json:"max_gap,omitempty"   desc:"A length, may be 0; a longer pause starts a caption. Default \"0.6s\"."`
	Replace    *bool          `json:"replace,omitempty"   desc:"Remove the captions linked to this clip on this track first. Default true."`
	NewIDs     []string       `json:"new_ids,omitempty"   desc:"Ids for the created captions, in order."                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Resolved   *ResolvedWords `json:"resolved,omitempty"  desc:"Filled by the service from the transcript of the asset; never sent by a caller."`
}

// groupWords is postcondition 2 of C.40.
func groupWords(ws []mappedWord, maxWords, maxChars, maxGap int) [][]mappedWord {
	var groups [][]mappedWord
	var cur []mappedWord
	size := 0
	for _, w := range ws {
		if len(cur) > 0 {
			total := size + 1 + chars(w.text)
			if len(cur)+1 > maxWords || total > maxChars || w.start-cur[len(cur)-1].end > maxGap {
				groups = append(groups, cur)
				cur, size = nil, 0
			}
		}
		if len(cur) > 0 {
			size++
		}
		size += chars(w.text)
		cur = append(cur, w)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

func applyCaptionFromTranscript(c *Context, a *captionFromTranscriptArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.clip_item_id", timeline.PrefixItem, a.ClipItemID}, {"args.track_id", timeline.PrefixTrack, a.TrackID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	if err := validateNewIDs(a.NewIDs); err != nil {
		return err
	}
	maxGapStr := cmp.Or(string(a.MaxGap), "0.6s")
	maxGap, err := c.length("args.max_gap", maxGapStr, true)
	if err != nil {
		return err
	}
	maxWords, maxChars, replace := 5, 28, true
	if a.MaxWords != nil {
		maxWords = *a.MaxWords
	}
	if a.MaxChars != nil {
		maxChars = *a.MaxChars
	}
	if a.Replace != nil {
		replace = *a.Replace
	}

	clip, err := c.item(a.ClipItemID)
	if err != nil {
		return err.WithPath("args.clip_item_id")
	}
	track, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if err := c.speechSource(
		&clip,
		"captions follow the speech of a clip or an audio item.",
		"there is no speech to caption.",
	); err != nil {
		return err.WithPath("args.clip_item_id")
	}
	if track.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; captions need a visual track.", a.TrackID).
			WithHint("Use a visual track, for example the one from track.add.").WithPath("args.track_id")
	}
	// Name a missing or wrong clip or track before the internal resolved field (VV-207).
	if err := checkResolvedWords(a.Resolved); err != nil {
		return err
	}
	mapped := mapTranscript(&clip, a.Resolved.Words)
	if len(mapped) == 0 {
		return noWordInRange(&clip)
	}
	groups := groupWords(mapped, maxWords, maxChars, maxGap)

	removed := []string{}
	if replace {
		for _, id := range c.trackItems(a.TrackID) {
			if it := c.Doc.Items[id]; it.Type == timeline.ItemCaption && it.LinkedItem == a.ClipItemID {
				removed = append(removed, id)
				delete(c.Doc.Items, id)
			}
		}
		slices.Sort(removed)
	}
	ids, unused, err := c.takeNewIDs(a.NewIDs, len(groups))
	if err != nil {
		return err
	}
	words := len(mapped)
	if total := c.wordTotal("") + words; total > maxWordsPerDocument {
		return Errorf(
			CodeLimit,
			"The document would have %d caption words; the limit is %d.",
			total,
			maxWordsPerDocument,
		).
			WithHint("Remove captions you no longer need.")
	}
	if total := len(c.Doc.Items) + len(groups); total > timeline.MaxItems {
		return Errorf(CodeLimit, "The document would have %d items; the limit is %d.", total, timeline.MaxItems).
			WithHint("Delete unused items first.")
	}
	for i, g := range groups {
		start, duration, ws := toWords(g)
		if err := c.checkOverlap(ids[i], a.TrackID, start, duration); err != nil {
			return err.WithHint(
				"replace true removes only the captions of this clip on this track, not the item in the way; " +
					"clear that item or use another track.",
			)
		}
		it := timeline.NewItem(ids[i], timeline.ItemCaption, a.TrackID, start, duration)
		it.Words, it.LinkedItem = ws, a.ClipItemID
		c.Doc.Items[ids[i]] = it
	}
	c.SetEffectiveTime("max_gap", maxGap, string(a.MaxGap))
	c.SetEffective("created", ids)
	c.SetEffective("new_ids", ids)
	c.SetEffective("unused_ids", unused)
	c.SetEffective("removed", removed)
	c.SetEffective("words_used", words)
	c.SetEffective("words_skipped", len(a.Resolved.Words)-words)
	return nil
}

type captionRetimeArgs struct {
	ClipItemID string         `json:"clip_item_id"       desc:"A clip of a video asset with audio, or an audio item."                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Resolved   *ResolvedWords `json:"resolved,omitempty" desc:"Filled by the service from the transcript of the asset; never sent by a caller."`
}

// linkedCaptions returns the captions linked to a clip, per track, each list in
// order of start then id; tracks are in sorted order.
func (c *Context) linkedCaptions(clip string) (tracks []string, byTrack map[string][]string) {
	byTrack = map[string][]string{}
	for _, id := range c.sortedItemIDs() {
		if it := c.Doc.Items[id]; it.Type == timeline.ItemCaption && it.LinkedItem == clip {
			byTrack[it.Track] = append(byTrack[it.Track], id)
		}
	}
	for t, ids := range byTrack {
		slices.SortStableFunc(ids, func(x, y string) int {
			return cmp.Or(c.Doc.Items[x].Start-c.Doc.Items[y].Start, strings.Compare(x, y))
		})
		tracks = append(tracks, t)
	}
	slices.Sort(tracks)
	return tracks, byTrack
}

func applyCaptionRetime(c *Context, a *captionRetimeArgs) error {
	if err := idForm("args.clip_item_id", timeline.PrefixItem, a.ClipItemID); err != nil {
		return err
	}
	clip, err := c.item(a.ClipItemID)
	if err != nil {
		return err.WithPath("args.clip_item_id")
	}
	if err := c.speechSource(&clip, "captions follow the speech of a clip or an audio item.",
		"there is no transcript to follow."); err != nil {
		return err.WithPath("args.clip_item_id")
	}
	// Name a missing or wrong clip before the internal resolved field (VV-207).
	if err := checkResolvedWords(a.Resolved); err != nil {
		return err
	}
	tracks, byTrack := c.linkedCaptions(a.ClipItemID)
	if len(tracks) == 0 {
		return Errorf(CodeInvalidState, "No caption is linked to clip %s.", a.ClipItemID).
			WithHint("Create captions with caption.from_transcript first.")
	}
	mapped := mapTranscript(&clip, a.Resolved.Words)
	if len(mapped) == 0 {
		return noWordInRange(&clip)
	}

	var out retimeResult
	for _, track := range tracks {
		next, err := c.retimeTrack(byTrack[track], mapped, &out)
		if err != nil {
			return err
		}
		out.unassigned = max(out.unassigned, len(mapped)-next)
	}
	for _, id := range out.retimed {
		it := c.Doc.Items[id]
		if err := c.checkOverlap(id, it.Track, it.Start, it.Duration); err != nil {
			other, _ := err.Details["conflicting_item"].(string)
			return err.WithHint("Move %s away, or put it on another track.", other)
		}
	}
	c.SetEffective("retimed", sortedIDs(out.retimed))
	c.SetEffective("removed", sortedIDs(out.removed))
	c.SetEffective("text_reset", sortedIDs(out.reset))
	c.SetEffective("unassigned", out.unassigned)
	return nil
}

type retimeResult struct {
	retimed, removed, reset []string
	unassigned              int
}

func sortedIDs(ids []string) []string {
	out := slices.Clone(ids)
	if out == nil {
		out = []string{}
	}
	slices.Sort(out)
	return out
}

// retimeTrack gives the mapped words, in order, to the linked captions of one track
// (postconditions 2 to 4 of C.41) and returns the number of words used.
func (c *Context) retimeTrack(ids []string, mapped []mappedWord, out *retimeResult) (int, *Error) {
	next := 0
	for _, id := range ids {
		it := c.Doc.Items[id]
		take := min(len(it.Words), len(mapped)-next)
		if take == 0 {
			delete(c.Doc.Items, id)
			out.removed = append(out.removed, id)
			continue
		}
		start, duration, words := toWords(mapped[next : next+take])
		next += take
		if take == len(it.Words) {
			for i := range words {
				words[i].Text = it.Words[i].Text
			}
		} else {
			out.reset = append(out.reset, id)
		}
		it.Start, it.Words = start, words
		it.Breaks = slices.DeleteFunc(slices.Clone(it.Breaks), func(b int) bool { return b >= len(words) })
		if len(it.Breaks) == 0 {
			it.Breaks = nil
		}
		if duration < it.Duration {
			kCut(&it, duration)
		}
		it.Duration = duration
		if err := checkFit(&it, "Retiming caption "+id); err != nil {
			return 0, err
		}
		c.Doc.Items[id] = it
		out.retimed = append(out.retimed, id)
	}
	return next, nil
}
