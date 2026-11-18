package command

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.remove_silences",
		Summary: "Cuts the silent stretches out of a clip or an audio item, leaving one item per stretch of speech.",
		Description: "Replaces the clip or audio item by one item per non-silent stretch of its source range. Silences shorter than " +
			"min_silence are kept; padding of speech is kept on each side of a cut. With ripple track the pieces " +
			"close up and the later items of the track follow. The silences come from the analysis of the asset " +
			"and are supplied by the service in resolved; the command never starts a job. Captions linked to the clip move their link to the piece that holds their first word.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "min_silence": "0.4s", "padding": "0.1s", "ripple": "track"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyRemoveSilences))
}

// SilenceRange is a silence on the asset's time axis, in frames, half-open.
type SilenceRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ResolvedSilences is the analysis result the service puts in `resolved` (A.9).
type ResolvedSilences struct {
	Silences []SilenceRange `json:"silences" desc:"Sorted, non-overlapping ranges in asset frames, half-open."`
}

type removeSilencesArgs struct {
	ItemID      string            `json:"item_id"                desc:"A clip of a video asset with audio, or an audio item."                                 pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	MinSilence  Time              `json:"min_silence,omitempty"  desc:"A length; shorter silences are ignored. Default \"0.4s\"."`
	Padding     Time              `json:"padding,omitempty"      desc:"A length, may be 0; speech kept around each cut. Default \"0.1s\"."`
	ThresholdDB *int              `json:"threshold_db,omitempty" desc:"Noise level of silence: -30, -35 (default), -40 or -45."`
	Ripple      string            `json:"ripple,omitempty"       desc:"track (default) or none."                                                                                                                     enum:"track,none"`
	NewIDs      []string          `json:"new_ids,omitempty"      desc:"Ids for the second, third, ... pieces, in order."                                      pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Resolved    *ResolvedSilences `json:"resolved,omitempty"     desc:"Filled by the service from the silence analysis of the asset; never sent by a caller."`
}

// validateNewIDs checks the form and the uniqueness of a new_ids argument (R6, row 14
// of the decisions of Part C).
func validateNewIDs(ids []string) *Error {
	for i, id := range ids {
		path := fmt.Sprintf("args.new_ids[%d]", i)
		if id == "" {
			return Errorf(CodeValidation, "%s is empty.", path).
				WithHint("Send item ids like \"itm_s6t7v8w9x0\" or omit new_ids.").WithPath(path)
		}
		if err := idForm(path, timeline.PrefixItem, id); err != nil {
			return err
		}
		if j := slices.Index(ids, id); j != i {
			return Errorf(CodeValidation, "%s repeats %s, already given at args.new_ids[%d].", path, id, j).
				WithHint("Send each id once.").WithPath(path)
		}
	}
	return nil
}

// takeNewIDs returns the n ids for new items: the supplied ones first, the rest from
// the generator, each unused in the document. The supplied ids beyond n are returned
// as unused (decision 14).
func (c *Context) takeNewIDs(supplied []string, n int) (used, unused []string, err *Error) {
	used, unused = []string{}, []string{}
	for i := range n {
		given := ""
		if i < len(supplied) {
			given = supplied[i]
		}
		id, err := c.newID("items", timeline.PrefixItem, given)
		if err != nil {
			return nil, nil, err.WithPath(fmt.Sprintf("args.new_ids[%d]", i))
		}
		used = append(used, id)
	}
	if len(supplied) > n {
		unused = append(unused, supplied[n:]...)
	}
	return used, unused, nil
}

func checkSilences(r *ResolvedSilences) *Error {
	if r == nil {
		return Errorf(CodeValidation, "args.resolved is missing: the silences of the asset are not known.").
			WithHint("The service fills it from the silence analysis; run the ingest job of the asset first, then retry.").
			WithPath("args.resolved")
	}
	prevEnd := 0
	for i, s := range r.Silences {
		path := fmt.Sprintf("args.resolved.silences[%d]", i)
		switch {
		case s.Start < 0:
			return Errorf(CodeValidation, "%s starts at %d, before frame 0.", path, s.Start).
				WithHint("Send silences as sorted, non-overlapping ranges.").WithPath(path)
		case s.End < s.Start:
			return Errorf(CodeValidation, "%s ends at %d before it starts at %d.", path, s.End, s.Start).
				WithHint("Send silences as sorted, non-overlapping ranges.").WithPath(path)
		case s.End == s.Start:
			return Errorf(CodeValidation, "%s is empty: it starts and ends at %d.", path, s.Start).
				WithHint("Send silences as sorted, non-overlapping ranges.").WithPath(path)
		case s.Start < prevEnd:
			return Errorf(CodeValidation, "%s starts at %d inside the previous silence, which ends at %d.",
				path, s.Start, prevEnd).
				WithHint("Send silences as sorted, non-overlapping ranges.").WithPath(path)
		}
		prevEnd = s.End
	}
	return nil
}

// piece is one stretch of speech: source frames [a, b).
type piece struct{ a, b int }

// speechPieces applies postconditions 1 and 2: the stretches of S that stay.
func speechPieces(si, end, num, den, m, p int, silences []SilenceRange) (pieces []piece, removedAny bool) {
	cursor := si
	for _, s := range silences {
		lo, hi := max(s.Start, si), min(s.End, end)
		if hi-lo < 1 || hi-lo < m {
			continue
		}
		if lo != si {
			lo += p
		}
		if hi != end {
			hi -= p
		}
		if hi-lo < 1 {
			continue
		}
		removedAny = true
		if lo > cursor {
			pieces = append(pieces, piece{cursor, lo})
		}
		cursor = hi
	}
	if cursor < end {
		pieces = append(pieces, piece{cursor, end})
	}
	var kept []piece
	for _, pc := range pieces {
		if (pc.b-pc.a)*den/num >= 1 {
			kept = append(kept, pc)
		}
	}
	return kept, removedAny
}

// silenceParams are the arguments of clip.remove_silences once parsed.
type silenceParams struct {
	min, padding   int
	minReq, padReq string
	threshold      int
	ripple         bool
	silences       []SilenceRange
}

func parseSilenceArgs(c *Context, a *removeSilencesArgs) (silenceParams, *Error) {
	var sp silenceParams
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return sp, err
	}
	if err := validateNewIDs(a.NewIDs); err != nil {
		return sp, err
	}
	sp.threshold = -35
	if a.ThresholdDB != nil {
		sp.threshold = *a.ThresholdDB
		if !slices.Contains([]int{-30, -35, -40, -45}, sp.threshold) {
			return sp, Errorf(CodeValidation, "args.threshold_db is %d, which is not one of -30, -35, -40, -45.",
				sp.threshold).
				WithHint("Use -35, the default, or -30, -40 or -45.").WithPath("args.threshold_db")
		}
	}
	sp.minReq, sp.padReq = string(a.MinSilence), string(a.Padding)
	minStr, padStr := cmp.Or(sp.minReq, "0.4s"), cmp.Or(sp.padReq, "0.1s")
	var err *Error
	if sp.min, err = c.length("args.min_silence", minStr, false); err != nil {
		return sp, err
	}
	if sp.padding, err = c.length("args.padding", padStr, true); err != nil {
		return sp, err
	}
	sp.ripple = a.Ripple != "none"
	return sp, nil
}

// speechClip checks that an item is a clip of a video asset with audio (E_TYPE_MISMATCH).
// notClip and noAudio finish the two messages.
func (c *Context) speechClip(it *timeline.Item, notClip, noAudio string) *Error {
	if it.Type != timeline.ItemClip {
		return Errorf(CodeTypeMismatch, "Item %s is %s item, not a clip; %s", it.ID, article(it.Type), notClip).
			WithHint("Choose a clip of a video asset with audio.").WithPath("args.item_id")
	}
	if asset, ok := c.Doc.Assets[it.Asset]; !ok || asset.Kind != timeline.AssetVideo || !asset.HasAudio {
		return Errorf(CodeTypeMismatch, "Asset %s has no audio; %s", it.Asset, noAudio).
			WithHint("Choose a clip whose asset has audio.").WithPath("args.item_id")
	}
	return nil
}

// speechSource is speechClip for the commands that caption speech: it also accepts an audio
// item (a voice-over on an audio track) whose asset has sound (VV-041).
func (c *Context) speechSource(it *timeline.Item, notClip, noAudio string) *Error {
	if err := rejectRemap(it, "the speech mapping"); err != nil {
		return err
	}
	if it.Type != timeline.ItemAudio {
		return c.speechClip(it, notClip, noAudio)
	}
	if asset, ok := c.Doc.Assets[it.Asset]; !ok || asset.Kind == timeline.AssetImage ||
		asset.Kind == timeline.AssetVideo && !asset.HasAudio {
		return Errorf(CodeTypeMismatch, "Asset %s has no audio; %s", it.Asset, noAudio).
			WithHint("Choose a clip or audio item whose asset has audio.").WithPath("args.item_id")
	}
	return nil
}

// silenceClip returns the clip or audio item of the command (precondition 2 and 3).
func (c *Context) silenceClip(id string) (timeline.Item, *Error) {
	it, err := c.item(id)
	if err != nil {
		return it, err
	}
	err = c.speechSource(
		&it,
		"only a clip or an audio item can be cut by its silences.",
		"there is nothing to analyse.",
	)
	return it, err
}

// silencePieces builds the items of postconditions 3 and 4 from the pieces of
// speech; ids are those of the second and later pieces.
func silencePieces(it *timeline.Item, pieces []piece, ids []string, ripple bool) []timeline.Item {
	num, den, si, origEnd := it.Speed.Num, it.Speed.Den, it.SourceIn, itemEnd(it)
	out := make([]timeline.Item, len(pieces))
	cursor := it.Start
	for i, pc := range pieces {
		d := ceilDiv((pc.a-si)*den, num)
		dur := (pc.b - pc.a) * den / num
		start := it.Start + d
		if ripple {
			start = cursor
		}
		if i == len(pieces)-1 {
			dur = min(dur, origEnd-start)
		}
		cp := timeline.CloneItem(*it)
		cp.Start, cp.Duration, cp.SourceIn = start, dur, pc.a
		if i > 0 {
			cp.ID = ids[i-1]
			cp.FadeIn, cp.TransitionIn, cp.Slot, cp.Link = 0, nil, nil, ""
		}
		if i < len(pieces)-1 {
			cp.FadeOut, cp.TransitionOut = 0, nil
		}
		kShift(&cp, d)
		kCut(&cp, dur)
		fitFix(&cp)
		out[i] = cp
		cursor += dur
	}
	return out
}

// relinkCaptions moves the link of each caption of the cut clip to the piece that holds
// its first word, so that caption.retime_from_clip finds them on every piece (VV-152).
func (c *Context) relinkCaptions(orig *timeline.Item, pieces []piece, out []timeline.Item) {
	for _, id := range c.sortedItemIDs() {
		capt := c.Doc.Items[id]
		if capt.Type != timeline.ItemCaption || capt.LinkedItem != orig.ID || len(capt.Words) == 0 {
			continue
		}
		src := orig.SourceIn + (capt.Start+capt.Words[0].Start-orig.Start)*orig.Speed.Num/orig.Speed.Den
		target := 0
		for i, pc := range pieces {
			if pc.a <= src {
				target = i
			}
		}
		if out[target].ID != capt.LinkedItem {
			capt.LinkedItem = out[target].ID
			c.Doc.Items[id] = capt
		}
	}
}

func applyRemoveSilences(c *Context, a *removeSilencesArgs) error {
	sp, err := parseSilenceArgs(c, a)
	if err != nil {
		return err
	}
	it, err := c.silenceClip(a.ItemID)
	if err != nil {
		return err
	}
	// The service leaves resolved empty when it cannot see an asset and expects this
	// command to name the item. Check that first, then the analysis (VV-207).
	if err := checkSilences(a.Resolved); err != nil {
		return err
	}
	sp.silences = a.Resolved.Silences
	si := it.SourceIn
	end := si + ceilDiv(it.Duration*it.Speed.Num, it.Speed.Den)
	pieces, removedAny := speechPieces(si, end, it.Speed.Num, it.Speed.Den, sp.min, sp.padding, sp.silences)
	if len(pieces) == 0 {
		return Errorf(CodeInvalidState, "Clip %s is silent over its whole source range %d-%d.", a.ItemID, si, end-1).
			WithHint("Delete the clip with item.delete, or use a larger min_silence.")
	}
	ids, unused, err := c.takeNewIDs(a.NewIDs, len(pieces)-1)
	if err != nil {
		return err
	}
	if total := len(c.Doc.Items) + len(pieces) - 1; total > timeline.MaxItems {
		return Errorf(CodeLimit, "Splitting %s into %d pieces would make %d items; the limit is %d.",
			a.ItemID, len(pieces), total, timeline.MaxItems).
			WithHint("Use a larger min_silence to get fewer pieces.")
	}

	out := []timeline.Item{it}
	removed := 0
	if removedAny {
		out = silencePieces(&it, pieces, ids, sp.ripple)
		for i := range out {
			removed -= out[i].Duration
		}
		removed += it.Duration
		if sp.ripple && removed != 0 {
			for _, id := range c.trackItems(it.Track) {
				if o := c.Doc.Items[id]; id != it.ID && o.Start >= itemEnd(&it) {
					o.Start -= removed
					c.Doc.Items[id] = o
				}
			}
		}
		for i := range out {
			c.Doc.Items[out[i].ID] = out[i]
		}
		c.relinkCaptions(&it, pieces, out)
	} else {
		ids, unused = []string{}, append([]string{}, a.NewIDs...)
	}

	listed := make([]map[string]any, len(out))
	for i := range out {
		listed[i] = map[string]any{
			"id": out[i].ID, "start": out[i].Start, "duration": out[i].Duration, "source_in": out[i].SourceIn,
		}
	}
	c.SetEffectiveTime("min_silence", sp.min, sp.minReq)
	c.SetEffectiveTime("padding", sp.padding, sp.padReq)
	c.SetEffective("threshold_db", sp.threshold)
	c.SetEffective("removed", map[string]any{"frames": removed, "seconds": c.FPS().Seconds(int64(removed))})
	c.SetEffective("pieces", listed)
	c.SetEffective("new_ids", ids)
	c.SetEffective("unused_ids", unused)
	return nil
}
