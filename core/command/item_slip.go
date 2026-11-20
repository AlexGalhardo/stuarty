package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

func init() {
	Register(Define(Spec{
		Name:    "item.slip",
		Summary: "Shows another part of the source in a clip or audio item without moving or resizing it.",
		Description: "Sets source_in, the offset into the asset where the item starts reading (a slip edit). The start, " +
			"the duration, the keyframes and the neighbours do not change. The source range must stay inside the asset.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "source_in": "2s"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeOutOfSource},
		Scope:   ScopeEdit,
	}, applyItemSlip))
}

type itemSlipArgs struct {
	ItemID   string `json:"item_id"   desc:"A clip or audio item of a video or audio asset."              pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	SourceIn Time   `json:"source_in" desc:"The new offset into the asset where the item starts reading."`
}

func applyItemSlip(c *Context, a *itemSlipArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	sourceIn, err := c.length("args.source_in", string(a.SourceIn), true)
	if err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if _, reads := c.sourceAsset(&it); !reads {
		return Errorf(CodeTypeMismatch, "Item %s is a %s and has no source range to slip.", a.ItemID, it.Type).
			WithHint("Slip applies to clip and audio items of a video or audio asset, not to images, text, captions or shapes.").
			WithPath("args.item_id")
	}
	if err := c.checkSource(&it, it.Duration, sourceIn); err != nil {
		return err.WithPath("args.source_in")
	}
	it.SourceIn = sourceIn
	c.Doc.Items[a.ItemID] = it
	c.SetEffectiveTime("source_in", sourceIn, string(a.SourceIn))
	return nil
}
