package command

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	tx "github.com/AlexGalhardo/stuarty/core/transcript"
)

func init() {
	Register(Define(Spec{
		Name:    "transcript.delete_range",
		Summary: "Removes the media of a range of transcript words from the timeline and closes the gap.",
		Description: "Cuts the source range of the words from_word to to_word (inclusive indices into the transcript of the " +
			"asset) out of every clip and audio item that shows the asset, so the video clip and its linked sound are cut " +
			"together. The pieces close up from the start of each item and the later items of the same track move " +
			"earlier. Captions linked to a cut clip move their link to the piece that holds their first word; " +
			"caption.retime_from_clip re-syncs them. The words come from the transcript of the asset and are supplied " +
			"by the service in resolved; the command never starts a job.",
		Example: `{"asset_id": "ast_3c5v8n2w6d", "from_word": 4, "to_word": 9}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyTranscriptDeleteRange))
}

type transcriptDeleteArgs struct {
	AssetID  string         `json:"asset_id"           desc:"A video asset with audio, or an audio asset, that was transcribed."              pattern:"^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	FromWord int            `json:"from_word"          desc:"Index of the first word to remove, in the transcript of the asset."                                                                     min:"0"`
	ToWord   int            `json:"to_word"            desc:"Index of the last word to remove, inclusive."                                                                                           min:"0"`
	NewIDs   []string       `json:"new_ids,omitempty"  desc:"Ids for the new pieces, in order of cut."                                        pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Resolved *ResolvedWords `json:"resolved,omitempty" desc:"Filled by the service from the transcript of the asset; never sent by a caller."                                                                internal:"true"`
}

// deleteCut is what the command does to one item.
type deleteCut struct {
	item   timeline.Item
	pieces []piece
	// frames are the local frames of the item each piece keeps, one entry per piece.
	frames []localFrames
}

// localFrames is a stretch of an item's own frames: the frame it starts at and how many it lasts.
type localFrames struct{ offset, length int }

// keptFrames decides the cut in the item's own frames, because that is what the timeline loses. Local frame k shows
// source frame floor(k x speed), so the frames that show [from, to) are exactly [ceil(a / speed), ceil(b / speed)):
// removing them loses the range within one frame at any speed. Rounding each kept side down instead loses up to one
// frame per side. A range that falls between two shown frames still removes one, so that its word leaves the timeline.
// ok is false when the item shows no frame of the range. The piece after the cut starts at source frame `to`: its
// frames then read no source past the end of the item.
func keptFrames(it *timeline.Item, from, to int) (pieces []piece, frames []localFrames, ok bool) {
	si, end := tx.SourceRange(it)
	num, den := it.Speed.Num, it.Speed.Den
	first := ceilDiv(max(from-si, 0)*den, num)
	if to <= si || first >= it.Duration {
		return nil, nil, false
	}
	last := min(max(ceilDiv((to-si)*den, num), first+1), it.Duration)
	if first > 0 {
		pieces = append(pieces, piece{si, from})
		frames = append(frames, localFrames{0, first})
	}
	if last < it.Duration {
		pieces = append(pieces, piece{to, end})
		frames = append(frames, localFrames{last, it.Duration - last})
	}
	return pieces, frames, true
}

// cutPieces builds the items of the kept stretches, closed up from the item's start (C.87 postcondition 2).
func cutPieces(it *timeline.Item, cut *deleteCut, ids []string) []timeline.Item {
	out := make([]timeline.Item, len(cut.pieces))
	cursor := it.Start
	for i, pc := range cut.pieces {
		kept := cut.frames[i]
		cp := timeline.CloneItem(*it)
		cp.Start, cp.Duration, cp.SourceIn = cursor, kept.length, pc.a
		if i > 0 {
			cp.ID = ids[i-1]
			cp.FadeIn, cp.TransitionIn, cp.Slot, cp.Link = 0, nil, nil, ""
		}
		if i < len(cut.pieces)-1 {
			cp.FadeOut, cp.TransitionOut = 0, nil
		}
		kShift(&cp, kept.offset)
		kCut(&cp, kept.length)
		fitFix(&cp)
		out[i] = cp
		cursor += kept.length
	}
	return out
}

// checkDeleteArgs validates the arguments and returns the source range to remove, in asset frames.
func checkDeleteArgs(c *Context, a *transcriptDeleteArgs) (from, to int, err *Error) {
	if err := idForm("args.asset_id", timeline.PrefixAsset, a.AssetID); err != nil {
		return 0, 0, err
	}
	if err := validateNewIDs(a.NewIDs); err != nil {
		return 0, 0, err
	}
	asset, err := c.asset(a.AssetID, "args.asset_id")
	if err != nil {
		return 0, 0, err
	}
	if asset.Kind == timeline.AssetImage || asset.Kind == timeline.AssetLUT ||
		asset.Kind == timeline.AssetVideo && !asset.HasAudio {
		return 0, 0, Errorf(CodeTypeMismatch, "Asset %s has no audio; there is no transcript to cut.", a.AssetID).
			WithHint("Choose an asset whose audio was transcribed.").WithPath("args.asset_id")
	}
	// Name a missing asset before the internal resolved field (VV-207).
	if err := checkResolvedWords(a.Resolved); err != nil {
		return 0, 0, err
	}
	words := a.Resolved.Words
	switch {
	case a.FromWord < 0:
		return 0, 0, Errorf(CodeValidation, "args.from_word is %d, before the first word.", a.FromWord).
			WithHint("Use indices from the transcript view of the asset.").WithPath("args.from_word")
	case a.ToWord < a.FromWord:
		return 0, 0, Errorf(CodeValidation, "args.to_word is %d, before args.from_word %d.", a.ToWord, a.FromWord).
			WithHint("Send the first word, then the last, both inclusive.").WithPath("args.to_word")
	case a.ToWord >= len(words):
		return 0, 0, Errorf(
			CodeValidation,
			"args.to_word is %d but the transcript has %d words.",
			a.ToWord,
			len(words),
		).
			WithHint("Use indices from the transcript view of the asset.").
			WithPath("args.to_word")
	}
	return words[a.FromWord].Start, words[a.ToWord].End, nil
}

// planCuts finds the items the range meets, latest start first so that the ripple of one cut never
// moves an item not yet cut.
func planCuts(c *Context, assetID string, from, to int) ([]*deleteCut, *Error) {
	var cuts []*deleteCut
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if !tx.Shows(&it, assetID) {
			continue
		}
		si, end := tx.SourceRange(&it)
		if from >= end || to <= si {
			continue
		}
		if !tx.Follows(&it) {
			return nil, rejectRemap(&it, "the words of the transcript").WithPath("args.asset_id")
		}
		pieces, frames, ok := keptFrames(&it, from, to)
		if !ok {
			continue
		}
		cuts = append(cuts, &deleteCut{item: it, pieces: pieces, frames: frames})
	}
	if len(cuts) == 0 {
		return nil, Errorf(CodeInvalidState, "No clip or audio item shows source frames %d to %d of asset %s.",
			from, to-1, assetID).
			WithHint("Check the indices against the transcript view; words already cut have no range.").
			WithPath("args.from_word")
	}
	slices.SortFunc(cuts, func(x, y *deleteCut) int {
		return cmp.Or(cmp.Compare(y.item.Start, x.item.Start), cmp.Compare(y.item.ID, x.item.ID))
	})
	return cuts, nil
}

// applyCut replaces one item by its pieces and ripples its track; it returns the frames removed and the
// ids of the pieces.
func (c *Context) applyCut(cut *deleteCut, ids []string) (removed int, pieceIDs []string) {
	it := cut.item
	var out []timeline.Item
	removed = it.Duration
	if len(cut.pieces) > 0 {
		out = cutPieces(&it, cut, ids)
		for i := range out {
			removed -= out[i].Duration
		}
	}
	if removed != 0 {
		for _, id := range c.trackItems(it.Track) {
			if o := c.Doc.Items[id]; id != it.ID && o.Start >= itemEnd(&it) {
				o.Start -= removed
				c.Doc.Items[id] = o
			}
		}
	}
	pieceIDs = []string{}
	if len(out) == 0 {
		delete(c.Doc.Items, it.ID)
		return removed, pieceIDs
	}
	for i := range out {
		c.Doc.Items[out[i].ID] = out[i]
		pieceIDs = append(pieceIDs, out[i].ID)
	}
	c.relinkCaptions(&it, cut.pieces, out)
	return removed, pieceIDs
}

func applyTranscriptDeleteRange(c *Context, a *transcriptDeleteArgs) error {
	from, to, err := checkDeleteArgs(c, a)
	if err != nil {
		return err
	}
	cuts, err := planCuts(c, a.AssetID, from, to)
	if err != nil {
		return err
	}

	added, deleted := 0, 0
	for _, cut := range cuts {
		added += max(len(cut.pieces)-1, 0)
		if len(cut.pieces) == 0 {
			deleted++
		}
	}
	if total := len(c.Doc.Items) + added - deleted; total > timeline.MaxItems {
		return Errorf(CodeLimit, "Cutting would make %d items; the limit is %d.", total, timeline.MaxItems).
			WithHint("Delete unused items first.")
	}
	ids, unused, err := c.takeNewIDs(a.NewIDs, added)
	if err != nil {
		return err
	}

	report := make([]map[string]any, 0, len(cuts))
	longest, next := 0, 0
	for _, cut := range cuts {
		own := ids[next : next+max(len(cut.pieces)-1, 0)]
		next += len(own)
		removed, pieceIDs := c.applyCut(cut, own)
		longest = max(longest, removed)
		report = append(report, map[string]any{
			"item_id": cut.item.ID, "track_id": cut.item.Track, "removed": removed, "pieces": pieceIDs,
		})
	}
	slices.SortFunc(report, func(x, y map[string]any) int {
		return cmp.Compare(fmt.Sprint(x["item_id"]), fmt.Sprint(y["item_id"]))
	})

	c.SetEffective("range", map[string]any{"start": from, "end": to})
	c.SetEffective("cuts", report)
	c.SetEffective("removed", map[string]any{"frames": longest, "seconds": c.FPS().Seconds(int64(longest))})
	c.SetEffective("new_ids", ids)
	c.SetEffective("unused_ids", unused)
	return nil
}
