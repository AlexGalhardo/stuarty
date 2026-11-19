package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// maxRepeatCopies is the most copies one item.repeat adds; a link group holds at most 20 items.
const (
	maxRepeatCopies = 200
	maxLinkGroup    = 20
)

func init() {
	Register(Define(Spec{
		Name:    "item.repeat",
		Summary: "Repeats an item back to back, a number of times or until a time, such as a looping background.",
		Description: "Adds copies of the item one after the other on its track, starting at its end: give copies " +
			"(1 to 200 extra plays) or until (an absolute time; the last copy is trimmed to end exactly there). " +
			"Keyframes, words and the name are copied; the slot mark is not. With ripple none (default) it fails " +
			"when a copy would overlap an item of the track; with ripple track the later items of the track move " +
			"later by the length added. With link the item and its copies are linked, so they move, trim and " +
			"split together (up to 20 items).",
		Example: `{"item_id": "itm_m1n2p3q4r5", "until": "120s", "ripple": "track"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState, CodeIDExists, CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Beta,
	}, applyItemRepeat))
}

type itemRepeatArgs struct {
	ItemID string   `json:"item_id"           desc:"The item to repeat."                                                                              pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Copies *int     `json:"copies,omitempty"  desc:"How many extra copies to add after the item (2 plays it three times). Give copies or until."                                                             min:"1" max:"200"`
	Until  Time     `json:"until,omitempty"   desc:"Absolute time the sequence ends at; the last copy is trimmed to end there. Give copies or until."`
	Ripple string   `json:"ripple,omitempty"  desc:"none (default) or track: the later items of the track move later by the length added."                                                                                     enum:"none,track"`
	Link   bool     `json:"link,omitempty"    desc:"Link the item and its copies so they move, trim and split together; default false."`
	NewIDs []string `json:"new_ids,omitempty" desc:"Ids for the copies, in order; the rest are generated."                                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// repeatPlan returns the duration of each copy: whole copies of the item, the last one shorter when
// the sequence has to end at until.
func (c *Context) repeatPlan(a *itemRepeatArgs, src *timeline.Item) ([]int, *Error) {
	if (a.Copies == nil) == (a.Until == "") {
		return nil, Errorf(CodeValidation, "args: give exactly one of copies and until.").
			WithHint(`Use {"copies": 3} or {"until": "120s"}.`).WithPath("args.copies")
	}
	end := itemEnd(src)
	if a.Copies != nil {
		if *a.Copies < 1 || *a.Copies > maxRepeatCopies {
			return nil, Errorf(CodeValidation, "args.copies is %d; use 1 to %d.", *a.Copies, maxRepeatCopies).
				WithHint("Send between 1 and %d copies, or use until.", maxRepeatCopies).WithPath("args.copies")
		}
		durs := make([]int, *a.Copies)
		for i := range durs {
			durs[i] = src.Duration
		}
		return durs, nil
	}
	until, err := c.ParseTime("args.until", string(a.Until))
	if err != nil {
		return nil, err
	}
	if until <= end {
		return nil, Errorf(CodeInvalidTime, "args.until %q is frame %d; the item already ends at frame %d.",
			string(a.Until), until, end).
			WithHint("Use an until after \"%df\" (%s).", end, c.seconds(end)).WithPath("args.until")
	}
	if until > timeline.MaxFrames {
		return nil, Errorf(CodeInvalidTime, "args.until %q is past the timeline limit of %d frames.",
			string(a.Until), timeline.MaxFrames).
			WithHint("Use a value up to \"%df\" (%s).", timeline.MaxFrames, c.seconds(timeline.MaxFrames)).
			WithPath("args.until")
	}
	var durs []int
	for left := until - end; left > 0; left -= src.Duration {
		durs = append(durs, min(left, src.Duration))
	}
	if len(durs) > maxRepeatCopies {
		return nil, Errorf(CodeLimit, "Repeating %s until %s needs %d copies; the limit is %d.",
			src.ID, string(a.Until), len(durs), maxRepeatCopies).
			WithHint("Use a later start of the sequence, a longer item, or two item.repeat calls.").
			WithPath("args.until")
	}
	return durs, nil
}

func applyItemRepeat(c *Context, a *itemRepeatArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if err := validateNewIDs(a.NewIDs); err != nil {
		return err
	}
	src, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	durs, err := c.repeatPlan(a, &src)
	if err != nil {
		return err
	}
	if len(a.NewIDs) > len(durs) {
		return Errorf(CodeValidation, "args.new_ids has %d ids for %d copies.", len(a.NewIDs), len(durs)).
			WithHint("Send at most one id per copy, or fewer.").WithPath("args.new_ids")
	}
	if a.Link && src.Link != "" {
		return Errorf(CodeInvalidState, "Item %s is already linked (%s).", src.ID, src.Link).
			WithHint("Unlink it with item.unlink first, or repeat without link.").WithPath("args.link")
	}
	if a.Link && len(durs) >= maxLinkGroup {
		return Errorf(
			CodeValidation,
			"args.link would link %d items; a group holds at most %d.",
			len(durs)+1,
			maxLinkGroup,
		).
			WithHint("Repeat without link, or with fewer copies.").
			WithPath("args.link")
	}
	if err := c.limit("items", len(c.Doc.Items)+len(durs)-1, timeline.MaxItems); err != nil {
		return err
	}
	if src.Type == timeline.ItemCaption {
		if total := c.wordTotal("") + len(src.Words)*len(durs); total > maxWordsPerDocument {
			return Errorf(CodeLimit, "The document would have %d caption words; the limit is %d.", total,
				maxWordsPerDocument).
				WithHint("Repeat fewer times, or remove captions you no longer need.").WithPath("args.copies")
		}
	}
	ids, _, err := c.takeNewIDs(a.NewIDs, len(durs))
	if err != nil {
		return err
	}

	total := 0
	for _, d := range durs {
		total += d
	}
	end := itemEnd(&src)
	if err := c.checkEnd("args.copies", end, total); err != nil {
		return err
	}
	if a.Ripple == "track" {
		if err := c.rippleLater(&src, total); err != nil {
			return err
		}
	}

	link := ""
	if a.Link {
		link = linkIDFrom(ids[0])
		src.Link = link
		c.Doc.Items[src.ID] = src
	}
	at := end
	for i, d := range durs {
		cp := timeline.CloneItem(src)
		cp.ID, cp.Start, cp.Slot, cp.Link = ids[i], at, nil, link
		if d < src.Duration {
			trimmed, err := c.trimmed(
				&cp,
				"end",
				at+d,
				fmt.Sprintf("Cutting the last copy of %s to %d frames", src.ID, d),
			)
			if err != nil {
				return err.WithPath("args.until")
			}
			cp = trimmed
		}
		if err := c.checkOverlap(cp.ID, cp.Track, cp.Start, cp.Duration); err != nil {
			return err
		}
		c.Doc.Items[cp.ID] = cp
		at += d
	}
	c.SetEffective("new_ids", ids)
	c.SetEffectiveTime("end", at, strconvFrames(at))
	if link != "" {
		c.SetEffective("link", link)
	}
	return nil
}

// rippleLater moves the items of the track that start at or after the end of it by shift frames
// later, and refuses a move past the timeline limit.
func (c *Context) rippleLater(it *timeline.Item, shift int) *Error {
	for _, id := range c.trackItems(it.Track) {
		if o := c.Doc.Items[id]; id != it.ID && o.Start >= itemEnd(it) {
			o.Start += shift
			if itemEnd(&o) > timeline.MaxFrames {
				return Errorf(CodeInvalidTime, "Item %s would end at frame %d; the limit is %d.", id, itemEnd(&o),
					timeline.MaxFrames).
					WithHint("Repeat fewer times, or use ripple none.").WithPath("args.ripple")
			}
			c.Doc.Items[id] = o
		}
	}
	return nil
}

func strconvFrames(f int) string { return fmt.Sprintf("%df", f) }
