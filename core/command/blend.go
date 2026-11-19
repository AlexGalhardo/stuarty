package command

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_blend",
		Summary: "Sets how a visual item is mixed with what is below it.",
		Description: "Sets one blend mode of the fixed list (normal, multiply, screen, overlay, add). normal is the default " +
			"and clears the stored one. Works on clip, text, caption and shape items.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "mode": "screen"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyClipSetBlend))
}

type clipBlendArgs struct {
	ItemID string `json:"item_id" desc:"A visual item."                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Mode   string `json:"mode"    desc:"normal, multiply, screen, overlay or add."                                                        enum:"normal,multiply,screen,overlay,add"`
}

func applyClipSetBlend(c *Context, a *clipBlendArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type == timeline.ItemAudio {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; blend applies to visual items.", a.ItemID).
			WithHint("Use item.set_audio for audio items.").WithPath("args.item_id")
	}
	if !slices.Contains(timeline.Blends, a.Mode) {
		return invalidArg("args.mode", "args.mode %q is not a blend mode.", a.Mode).
			WithHint("Use normal, multiply, screen, overlay or add.")
	}
	it.Blend = a.Mode
	if a.Mode == "normal" {
		it.Blend = ""
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("blend", a.Mode)
	return nil
}
