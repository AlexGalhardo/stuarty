package command

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "caption.split",
		Summary: "Splits a caption into two at a word boundary.",
		Description: "The right part starts with the word at before_word. Style and the linked clip are copied to it; " +
			"transition_out moves to it. The cut falls at the start of that word, so the parts cover the original range.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "before_word": 2, "new_id": "itm_c3d4e5f6g7"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCaptionSplit))

	Register(Define(Spec{
		Name:    "caption.merge",
		Summary: "Joins two captions of one track into one that spans both.",
		Description: "The caption that starts first keeps its identifier, style and position and takes the words of " +
			"the other; the other is removed. Captions with keyframes cannot be merged.",
		Example: `{"item_ids": ["itm_s6t7v8w9x0", "itm_c3d4e5f6g7"]}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCaptionMerge))
}

type captionSplitArgs struct {
	ItemID     string `json:"item_id"          desc:"A caption."                                 pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	BeforeWord int    `json:"before_word"      desc:"Index of the first word of the right part."                                                        min:"1"`
	NewID      string `json:"new_id,omitempty" desc:"The id of the right part."                  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyCaptionSplit(c *Context, a *captionSplitArgs) error {
	for _, f := range []struct{ path, id string }{{"args.item_id", a.ItemID}, {"args.new_id", a.NewID}} {
		if err := idForm(f.path, timeline.PrefixItem, f.id); err != nil {
			return err
		}
	}
	it, err := c.caption(a.ItemID, "use item.split.", "Use item.split for other items.")
	if err != nil {
		return err
	}
	n := len(it.Words)
	if a.BeforeWord >= n {
		return Errorf(
			CodeNotFound,
			"Caption %s has %d words; word %d does not exist, so the right part would be empty.",
			a.ItemID,
			n,
			a.BeforeWord,
		).
			WithHint("Use a before_word from 1 to %d.", n-1).
			WithPath("args.before_word").
			WithDetail("kind", "word").
			WithDetail("id", a.BeforeWord).
			WithDetail("word_count", n)
	}
	newID, err := c.newID("items", timeline.PrefixItem, a.NewID)
	if err != nil {
		return err.WithHint("Omit new_id or use an unused id.").WithPath("args.new_id")
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	k := a.BeforeWord
	tc := it.Words[k].Start
	left, right := splitCore(&it, newID, tc, false)
	left.Words = slices.Clone(it.Words[:k])
	right.Words = slices.Clone(it.Words[k:])
	for i := range right.Words {
		right.Words[i].Start -= tc
	}
	left.Breaks, right.Breaks = nil, nil
	for _, b := range it.Breaks {
		switch {
		case b >= 1 && b < k:
			left.Breaks = append(left.Breaks, b)
		case b > k:
			right.Breaks = append(right.Breaks, b-k)
		}
	}
	c.Doc.Items[a.ItemID], c.Doc.Items[newID] = left, right
	c.SetEffective("new_id", newID)
	c.SetEffective("left", map[string]any{"id": a.ItemID, "start": left.Start, "duration": left.Duration})
	c.SetEffective("right", map[string]any{"id": newID, "start": right.Start, "duration": right.Duration})
	return nil
}

type captionMergeArgs struct {
	ItemIDs []string `json:"item_ids" desc:"Exactly two distinct captions of one track, in any order." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// mergeDropped lists the fields of the second caption that the merge does not carry over.
func mergeDropped(first, second *timeline.Item) []string {
	dropped := []string{}
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"linked_item", second.LinkedItem != "" && first.LinkedItem != "" && second.LinkedItem != first.LinkedItem},
		{"name", second.Name != ""},
		{"opacity", second.Opacity != 1},
		{"rotation", second.Rotation != 0},
		{"scale", second.Scale != 1},
		{"slot", second.Slot != nil},
		{"style", second.Style != nil},
		{"transition_in", second.TransitionIn != nil},
		{"x", second.X != nil},
		{"y", second.Y != nil},
	} {
		if f.set {
			dropped = append(dropped, f.name)
		}
	}
	return dropped
}

func applyCaptionMerge(c *Context, a *captionMergeArgs) error {
	if len(a.ItemIDs) != 2 {
		return Errorf(CodeValidation, "args.item_ids must have exactly 2 entries, got %d.", len(a.ItemIDs)).
			WithHint("Merge two captions at a time.").WithPath("args.item_ids")
	}
	if a.ItemIDs[0] == a.ItemIDs[1] {
		return Errorf(CodeValidation, "args.item_ids[1] repeats %s.", a.ItemIDs[0]).
			WithHint("Send two different captions.").WithPath("args.item_ids[1]")
	}
	var items [2]timeline.Item
	for i, id := range a.ItemIDs {
		path := fmt.Sprintf("args.item_ids[%d]", i)
		if err := idForm(path, timeline.PrefixItem, id); err != nil {
			return err
		}
		it, err := c.caption(id, "only captions can be merged.", "Pass two caption items.")
		if err != nil {
			return err.WithPath(path)
		}
		items[i] = it
	}
	first, second := items[0], items[1]
	if second.Start < first.Start {
		first, second = second, first
	}
	if first.Track != second.Track {
		return Errorf(CodeInvalidState, "The captions %s and %s are on different tracks (%s and %s).",
			first.ID, second.ID, first.Track, second.Track).
			WithHint("Move one caption to the track of the other with item.move first.")
	}
	var between []string
	for _, id := range c.trackItems(first.Track) {
		o := c.Doc.Items[id]
		if id != first.ID && id != second.ID && o.Start >= itemEnd(&first) && o.Start < second.Start {
			between = append(between, id)
		}
	}
	if len(between) > 0 {
		o := c.Doc.Items[between[0]]
		return Errorf(CodeInvalidState, "Item %s (frames %d-%d) lies between captions %s and %s.",
			between[0], o.Start, itemEnd(&o)-1, first.ID, second.ID).
			WithHint("Move or delete %s first.", between[0]).WithDetail("between", between)
	}
	for _, it := range []*timeline.Item{&first, &second} {
		if len(it.Keyframes) > 0 {
			return Errorf(CodeInvalidState, "Caption %s has keyframes, which cannot be joined.", it.ID).
				WithHint("Remove them with keyframe.clear first.")
		}
	}
	total := len(first.Words) + len(second.Words)
	if total > maxWordsPerCaption {
		return Errorf(CodeLimit, "The merged caption would have %d words; the limit is %d.", total, maxWordsPerCaption).
			WithHint("Remove words first, or keep the captions separate.")
	}

	dropped := mergeDropped(&first, &second)
	offset := second.Start - first.Start
	merged := timeline.CloneItem(first)
	merged.Duration = offset + second.Duration
	for _, w := range second.Words {
		w.Start += offset
		merged.Words = append(merged.Words, w)
	}
	for _, b := range second.Breaks {
		merged.Breaks = append(merged.Breaks, b+len(first.Words))
	}
	merged.TransitionOut = second.TransitionOut
	if merged.LinkedItem == "" {
		merged.LinkedItem = second.LinkedItem
	}
	delete(c.Doc.Items, second.ID)
	c.Doc.Items[first.ID] = merged
	c.SetEffective("id", first.ID)
	c.SetEffective("word_count", total)
	c.SetEffective("dropped", dropped)
	return nil
}
