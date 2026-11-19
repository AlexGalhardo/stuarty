package command

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "slot.mark",
		Summary: "Marks an item as a template slot.",
		Description: "Makes a clip, audio, text or caption item a slot named name; the slot kind follows from the item " +
			"type (clip and audio give media). Marking the same item again with the same name updates the slot.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "name": "title", "description": "Headline"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeSlot, CodeInvalidState,
		},
		Scope: ScopeEdit,
	}, applySlotMark))

	Register(Define(Spec{
		Name:        "slot.unmark",
		Summary:     "Removes the slot marking of an item.",
		Description: "The item keeps its content. An unfilled media slot that still holds the placeholder asset cannot be unmarked.",
		Example:     `{"item_id": "itm_s6t7v8w9x0"}`,
		Errors:      []string{CodeNotFound, CodeSlot, CodeInvalidState},
		Scope:       ScopeEdit,
	}, applySlotUnmark))

	Register(Define(Spec{
		Name:    "slot.fill",
		Summary: "Fills a template slot with media, text or caption words.",
		Description: "Give exactly one of asset_id (media slot), text (text slot) or words (caption slot). A media slot " +
			"with duration policy fit keeps its duration and needs an asset long enough; with stretch_timeline it " +
			"takes the rest of the asset and the later items of every track follow.",
		Example: `{"name": "hero", "asset_id": "ast_4h6j8k2m3n", "source_in": "1s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeSlot, CodeOutOfSource, CodeLimit,
			CodeOverlap, CodeInvalidState,
		},
		Scope: ScopeEdit,
	}, applySlotFill))
}

var slotName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type slotMarkArgs struct {
	ItemID         string `json:"item_id"                   desc:"A clip, audio, text or caption item."                  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name           string `json:"name"                      desc:"Matches [a-z][a-z0-9_]{0,39}; unique among the slots."`
	Description    string `json:"description,omitempty"     desc:"Up to 200 characters."                                                                                        max:"200"`
	Required       *bool  `json:"required,omitempty"        desc:"Default true."`
	DurationPolicy string `json:"duration_policy,omitempty" desc:"Media slots only. Default fit."                                                                                         enum:"fit,stretch_timeline"`
}

func slotKind(t timeline.ItemType) string {
	switch t {
	case timeline.ItemClip, timeline.ItemAudio:
		return "media"
	case timeline.ItemText:
		return "text"
	case timeline.ItemCaption:
		return "caption"
	case timeline.ItemShape:
		return ""
	}
	return ""
}

func applySlotMark(c *Context, a *slotMarkArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if !slotName.MatchString(a.Name) {
		return Errorf(CodeValidation, "args.name %q does not match [a-z][a-z0-9_]{0,39}.", a.Name).
			WithHint("Use lower case letters, digits and underscores, for example \"title\".").WithPath("args.name")
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	kind := slotKind(it.Type)
	if kind == "" {
		return Errorf(CodeTypeMismatch, "Item %s is a %s; slots are for media, text and captions.", a.ItemID, it.Type).
			WithHint("Mark a clip, audio, text or caption item.").WithPath("args.item_id")
	}
	if a.DurationPolicy != "" && kind != "media" {
		return Errorf(CodeValidation, "args.duration_policy is only valid for media slots; %s is a %s slot.",
			a.ItemID, kind).
			WithHint("Remove duration_policy.").WithPath("args.duration_policy")
	}
	for _, id := range c.sortedItemIDs() {
		if o := c.Doc.Items[id]; id != a.ItemID && o.Slot != nil && o.Slot.Name == a.Name {
			return Errorf(CodeSlot, "Slot name %q is already used by item %s.", a.Name, id).
				WithHint("Use another name, or unmark the other slot first.").
				WithDetail("reason", "name_in_use").WithDetail("item", id).WithPath("args.name")
		}
	}
	slot := timeline.NewSlot(a.Name, kind)
	if it.Slot != nil {
		if it.Slot.Name != a.Name {
			return Errorf(CodeInvalidState, "Item %s is already slot %q.", a.ItemID, it.Slot.Name).
				WithHint("Use slot.unmark first, or reuse the name %q to update it.", it.Slot.Name)
		}
		slot.Filled = it.Slot.Filled
	}
	slot.Description = a.Description
	if a.Required != nil {
		slot.Required = *a.Required
	}
	if a.DurationPolicy != "" {
		slot.DurationPolicy = a.DurationPolicy
	}
	it.Slot = &slot
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("slot", slotView(&slot))
	return nil
}

// slotView is the stored form of a slot for effective.
func slotView(s *timeline.Slot) map[string]any {
	m := map[string]any{"name": s.Name, "kind": s.Kind}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if !s.Required {
		m["required"] = false
	}
	if s.DurationPolicy != "fit" {
		m["duration_policy"] = s.DurationPolicy
	}
	if s.Filled {
		m["filled"] = true
	}
	return m
}

type slotUnmarkArgs struct {
	ItemID string `json:"item_id" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applySlotUnmark(c *Context, a *slotUnmarkArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Slot == nil {
		return Errorf(CodeSlot, "Item %s is not a slot.", a.ItemID).
			WithHint("Mark it with slot.mark, or pick an item that is a slot.").
			WithDetail("item", a.ItemID).WithPath("args.item_id")
	}
	if it.Slot.Kind == "media" && !it.Slot.Filled && it.Asset == timeline.PlaceholderAsset {
		return Errorf(CodeInvalidState, "Slot %q on %s is unfilled and still uses %s.",
			it.Slot.Name, a.ItemID, timeline.PlaceholderAsset).
			WithHint("Fill it with slot.fill, or delete the item.")
	}
	name := it.Slot.Name
	it.Slot = nil
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("removed_slot", name)
	return nil
}

type slotFillArgs struct {
	Name     string    `json:"name"                desc:"The slot name."`
	AssetID  string    `json:"asset_id,omitempty"  desc:"For a media slot."                                                         pattern:"^ast_([0123456789abcdefghjkmnpqrstvwxyz]{10}|placeholder)$"`
	SourceIn Time      `json:"source_in,omitempty" desc:"With asset_id only. Default \"0s\"."`
	Text     *string   `json:"text,omitempty"      desc:"For a text slot."                                                                                                                               min:"1" max:"2000"`
	Words    []WordArg `json:"words,omitempty"     desc:"For a caption slot: words with times relative to the caption, as lengths."`
}

// slotItem finds the item that holds the slot name (E_SLOT with the available names).
func (c *Context) slotItem(name string) (timeline.Item, *Error) {
	var names []string
	for _, id := range c.sortedItemIDs() {
		if it := c.Doc.Items[id]; it.Slot != nil {
			if it.Slot.Name == name {
				return it, nil
			}
			names = append(names, it.Slot.Name)
		}
	}
	slices.Sort(names)
	if names == nil {
		names = []string{}
	}
	return timeline.Item{}, Errorf(CodeSlot, "Slot %q does not exist.", name).
		WithHint("Use one of the slots: %s.", strings.Join(names, ", ")).
		WithDetail("available", names).WithPath("args.name")
}

// fillMismatch is precondition 3: the argument must be the one of the slot kind.
func fillMismatch(a *slotFillArgs, slot *timeline.Slot) *Error {
	given, want := "asset_id", map[string]string{"media": "asset_id", "text": "text", "caption": "words"}[slot.Kind]
	switch {
	case a.Text != nil:
		given = "text"
	case a.Words != nil:
		given = "words"
	}
	if given != want {
		return Errorf(CodeSlot, "Slot %q is %s %s slot; it needs %s, not %s.",
			slot.Name, map[string]string{"media": "a", "text": "a", "caption": "a"}[slot.Kind], slot.Kind, want, given).
			WithHint("Send %s for slot %q.", want, slot.Name).WithPath("args." + given)
	}
	if slot.Filled {
		return Errorf(CodeSlot, "Slot %q is already filled.", slot.Name).
			WithHint("Delete the item and add a new one, or unmark and mark the slot again.").WithPath("args.name")
	}
	return nil
}

func applySlotFill(c *Context, a *slotFillArgs) error {
	if err := requireOne(
		[]string{"asset_id", "text", "words"},
		a.AssetID != "",
		a.Text != nil,
		a.Words != nil,
	); err != nil {
		return err
	}
	if a.SourceIn != "" && a.AssetID == "" {
		return Errorf(CodeValidation, "args.source_in is only valid with asset_id.").
			WithHint("Remove source_in, or fill a media slot with asset_id.").WithPath("args.source_in")
	}
	if a.AssetID != "" && a.AssetID != timeline.PlaceholderAsset {
		if err := idForm("args.asset_id", timeline.PrefixAsset, a.AssetID); err != nil {
			return err
		}
	}
	sourceIn := 0
	if a.SourceIn != "" {
		var err *Error
		if sourceIn, err = c.length("args.source_in", string(a.SourceIn), true); err != nil {
			return err
		}
	}
	var words []timeline.Word
	if a.Words != nil {
		var err *Error
		if words, err = c.parseWords("args.words", a.Words); err != nil {
			return err
		}
	}
	it, err := c.slotItem(a.Name)
	if err != nil {
		return err
	}
	slot := *it.Slot
	if err := fillMismatch(a, &slot); err != nil {
		return err
	}
	slot.Filled = true
	oldDuration := it.Duration
	var shifted []shift
	switch slot.Kind {
	case "media":
		shifted, err = c.fillMedia(&it, &slot, a.AssetID, sourceIn)
	case "text":
		it.Text = *a.Text
	default:
		err = c.fillCaption(&it, words)
	}
	if err != nil {
		return err
	}
	it.Slot = &slot
	c.Doc.Items[it.ID] = it
	if err := c.checkFill(&it, shifted); err != nil {
		return err
	}

	c.SetEffective("slot", a.Name)
	c.SetEffective("kind", slot.Kind)
	if slot.Kind == "media" {
		c.SetEffectiveTime("source_in", sourceIn, string(a.SourceIn))
	}
	c.SetEffective("duration", map[string]any{"before": oldDuration, "after": it.Duration})
	list := make([]map[string]any, len(shifted))
	for i, s := range shifted {
		list[i] = map[string]any{"id": s.id, "from": s.from, "to": s.to}
	}
	c.SetEffective("shifted", list)
	return nil
}

type shift struct {
	id       string
	from, to int
}

// fillMedia is the media part of slot.fill: preconditions 4 and 5 and postcondition 1.
func (c *Context) fillMedia(it *timeline.Item, slot *timeline.Slot, assetID string, sourceIn int) ([]shift, *Error) {
	asset, ok := c.Doc.Assets[assetID]
	if !ok {
		return nil, Errorf(CodeNotFound, "Asset %s does not exist.", assetID).
			WithHint("Import the media first, then use its asset id.").
			WithDetail("kind", "asset").WithDetail("id", assetID).WithPath("args.asset_id")
	}
	if err := fillAssetKind(it, slot, &asset); err != nil {
		return nil, err
	}
	image := asset.Kind == timeline.AssetImage
	oldEnd, oldDuration := itemEnd(it), it.Duration
	if image {
		sourceIn = 0
	}
	it.Asset, it.SourceIn = assetID, sourceIn
	if !image {
		total, _ := timeline.AssetDuration(asset, c.Doc.Settings.FPS)
		if slot.DurationPolicy == "fit" {
			if need := ceilDiv(it.Duration*it.Speed.Num, it.Speed.Den); sourceIn+need > total {
				return nil, outOfSource(slot, it, &asset, total, sourceIn, need)
			}
		} else {
			fit := maxDur(total, sourceIn, it.Speed)
			if fit < 1 {
				return nil, Errorf(CodeOutOfSource, "Slot %q: source_in %d leaves no frame of asset %s (%d frames).",
					slot.Name, sourceIn, assetID, total).
					WithHint("Use a smaller source_in.").
					WithDetail("available", map[string]any{"asset_duration": total, "source_in": sourceIn, "max_duration": 0}).
					WithPath("args.source_in")
			}
			it.Duration = fit
		}
	}
	delta := it.Duration - oldDuration
	if delta < 0 {
		kCut(it, it.Duration)
	}
	var shifted []shift
	if delta != 0 {
		for _, id := range c.sortedItemIDs() {
			if o := c.Doc.Items[id]; id != it.ID && o.Start >= oldEnd {
				shifted = append(shifted, shift{id, o.Start, o.Start + delta})
				o.Start += delta
				c.Doc.Items[id] = o
			}
		}
	}
	return shifted, nil
}

func outOfSource(slot *timeline.Slot, it *timeline.Item, asset *timeline.Asset, total, sourceIn, need int) *Error {
	avail := max(total-sourceIn, 0)
	return Errorf(
		CodeOutOfSource,
		"Slot %q needs %d frames from source frame %d; asset %s has %d frames, so %d are available.",
		slot.Name,
		need,
		sourceIn,
		asset.ID,
		total,
		avail,
	).
		WithHint("Use an asset of at least %d frames, a smaller source_in, or set duration_policy to stretch_timeline.",
			sourceIn+need).
		WithDetail("available", map[string]any{
			"asset_duration": total, "source_in": sourceIn, "max_duration": maxDur(total, sourceIn, it.Speed),
		}).
		WithPath("args.asset_id")
}

// fillAssetKind is the second half of precondition 4: the asset must suit the item.
func fillAssetKind(it *timeline.Item, slot *timeline.Slot, asset *timeline.Asset) *Error {
	word := map[timeline.AssetKind]string{
		timeline.AssetImage: "an image", timeline.AssetVideo: "a video", timeline.AssetAudio: "an audio file",
		timeline.AssetLUT: "a lookup table",
	}[asset.Kind]
	if it.Type == timeline.ItemAudio {
		if asset.Kind == timeline.AssetAudio || asset.Kind == timeline.AssetVideo && asset.HasAudio {
			return nil
		}
		return Errorf(CodeTypeMismatch, "Asset %s is %s%s; slot %q holds an audio item.",
			asset.ID, word, map[bool]string{true: " without audio", false: ""}[asset.Kind == timeline.AssetVideo],
			slot.Name).
			WithHint("Use an audio asset, or a video with audio.").WithPath("args.asset_id")
	}
	if asset.Kind == timeline.AssetAudio || asset.Kind == timeline.AssetLUT {
		return Errorf(CodeTypeMismatch, "Asset %s is %s; slot %q holds a clip.", asset.ID, word, slot.Name).
			WithHint("Use a video or image asset.").WithPath("args.asset_id")
	}
	return nil
}

// fillCaption is the caption part of slot.fill: postcondition 3 and precondition 6.
func (c *Context) fillCaption(it *timeline.Item, words []timeline.Word) *Error {
	if err := c.checkWordLimits(it.ID, len(words)); err != nil {
		return err
	}
	last := words[len(words)-1]
	duration := last.Start + last.Duration
	if duration > it.Duration {
		if err := c.checkOverlap(it.ID, it.Track, it.Start, duration); err != nil {
			return err
		}
	} else if duration < it.Duration {
		kCut(it, duration)
	}
	it.Words, it.Breaks, it.Duration = words, nil, duration
	return nil
}

// checkFill is precondition 7: the end limit, the fit of fades and transitions and the
// overlaps on the final state of the document for the filled item and the shifted ones.
func (c *Context) checkFill(it *timeline.Item, shifted []shift) *Error {
	moved := []string{it.ID}
	for _, s := range shifted {
		moved = append(moved, s.id)
	}
	for _, id := range moved {
		o := c.Doc.Items[id]
		if err := c.checkEnd("args", o.Start, o.Duration); err != nil {
			return err
		}
	}
	if err := checkFit(it, fmt.Sprintf("Filling slot %q", it.Slot.Name)); err != nil {
		return err
	}
	for _, id := range moved {
		o := c.Doc.Items[id]
		if err := c.checkOverlap(id, o.Track, o.Start, o.Duration); err != nil {
			other, _ := err.Details["conflicting_item"].(string)
			return err.WithHint("Use duration_policy \"fit\", or move %s.", other)
		}
	}
	return nil
}
