package command

import (
	"reflect"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	// Set here, not in the literal of namedSchemas: schemaOf reads that map.
	namedSchemas["mask_or_null"] = maskSchema

	Register(Define(Spec{
		Name:    "clip.set_mask",
		Summary: "Limits where a visual item is drawn to a rectangle or an ellipse.",
		Description: "Sets a mask in the item's own frame: x, y, width and height are fractions of the item (0 to 1, " +
			"x plus width and y plus height at most 1), feather is the soft edge in pixels (0 to 200, default 0) and " +
			"invert keeps the outside instead. Replaces the current mask and clears its keyframes (mask_x, mask_y, " +
			"mask_width, mask_height, mask_feather; see keyframe.set). Works on clip, text, caption and shape " +
			"items. mask null removes it.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "mask": {"shape": "ellipse", "x": 0.1, "y": 0.1, "width": 0.8, "height": 0.8, "feather": 12}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyClipSetMask))
}

type maskArgs struct {
	Shape   string  `json:"shape"             desc:"rect or ellipse."                                      enum:"rect,ellipse"`
	X       float64 `json:"x"                 desc:"Left edge, a fraction of the item width."                                  min:"0"     max:"1"`
	Y       float64 `json:"y"                 desc:"Top edge, a fraction of the item height."                                  min:"0"     max:"1"`
	Width   float64 `json:"width"             desc:"A fraction of the item width."                                             min:"0.001" max:"1"`
	Height  float64 `json:"height"            desc:"A fraction of the item height."                                            min:"0.001" max:"1"`
	Feather *int    `json:"feather,omitempty" desc:"Soft edge in pixels; default 0."                                           min:"0"     max:"200"`
	Invert  bool    `json:"invert,omitempty"  desc:"Keep the outside of the shape instead; default false."`
}

type clipMaskArgs struct {
	ItemID string    `json:"item_id" desc:"A visual item."                  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Mask   *maskArgs `json:"mask"    desc:"The mask, or null to remove it."                                                        schema:"mask_or_null"`
}

func applyClipSetMask(c *Context, a *clipMaskArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type == timeline.ItemAudio {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; a mask applies to visual items.", a.ItemID).
			WithHint("Use item.set_audio for audio items.").WithPath("args.item_id")
	}
	// The keyframes of a mask that is replaced or removed do not apply to the new one.
	for _, p := range timeline.MaskProps {
		if len(it.Keyframes[p]) > 0 {
			dropKeyframes(&it, p, 0)
		}
	}
	if a.Mask == nil {
		it.Mask = nil
		c.Doc.Items[a.ItemID] = it
		c.SetEffective("mask", nil)
		return nil
	}
	m := a.Mask
	mask := timeline.Mask{
		Shape: m.Shape, X: round3(m.X), Y: round3(m.Y), Width: round3(m.Width), Height: round3(m.Height),
		Invert: m.Invert,
	}
	if m.Feather != nil {
		mask.Feather = *m.Feather
	}
	if mask.Width < 0.001 || mask.Height < 0.001 {
		return invalidArg("args.mask", "The mask has no area after rounding to 3 decimals.").
			WithHint("Use a width and a height of at least 0.001.")
	}
	if mask.X+mask.Width > 1.0005 || mask.Y+mask.Height > 1.0005 {
		return invalidArg("args.mask", "The mask goes outside the item: x + width or y + height is above 1.").
			WithHint("Keep x + width and y + height at most 1.")
	}
	if cur := it.Mask; cur != nil {
		mask.Extra = cur.Extra
	}
	it.Mask = &mask
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("mask", map[string]any{
		"shape": mask.Shape, "x": mask.X, "y": mask.Y, "width": mask.Width, "height": mask.Height,
		"feather": mask.Feather, "invert": mask.Invert,
	})
	return nil
}

func maskSchema() map[string]any {
	s := schemaOf(reflect.TypeFor[maskArgs]())
	s["type"] = []string{"object", "null"}
	return s
}
