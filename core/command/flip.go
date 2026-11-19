package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_flip",
		Summary: "Mirrors a visual item horizontally, vertically or both.",
		Description: "Sets the mirror of an item: horizontal flips left to right, vertical flips top to bottom. A side " +
			"not given keeps its value; false clears it. Works on clip, text, caption and shape items.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "horizontal": true}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyClipSetFlip))
}

type clipFlipArgs struct {
	ItemID     string `json:"item_id"              desc:"A visual item."        pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Horizontal *bool  `json:"horizontal,omitempty" desc:"Mirror left to right."`
	Vertical   *bool  `json:"vertical,omitempty"   desc:"Mirror top to bottom."`
}

func applyClipSetFlip(c *Context, a *clipFlipArgs) error {
	if err := atLeastOne([]string{"horizontal", "vertical"}, a.Horizontal != nil, a.Vertical != nil); err != nil {
		return err
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type == timeline.ItemAudio {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; flip applies to visual items.", a.ItemID).
			WithHint("Use item.set_audio for audio items.").WithPath("args.item_id")
	}
	if a.Horizontal != nil {
		it.FlipX = *a.Horizontal
	}
	if a.Vertical != nil {
		it.FlipY = *a.Vertical
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("horizontal", it.FlipX)
	c.SetEffective("vertical", it.FlipY)
	return nil
}
