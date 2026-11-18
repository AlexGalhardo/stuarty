package command

func init() {
	Register(Define(Spec{
		Name:    "clip.set_filter",
		Summary: "Sets the colour filter of a visual item and how much of it is mixed in.",
		Description: "Sets one filter of the fixed list (none, grayscale, sepia, invert, warm, cool) and its amount, " +
			"0 (the original) to 1 (the full filter), rounded to 3 decimals; a value equal to the default (none, 1) " +
			"clears the stored one.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "filter": "sepia", "amount": 0.6}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyClipSetFilter))

	Register(Define(Spec{
		Name:        "clip.set_blur",
		Summary:     "Sets the blur radius of a visual item.",
		Description: "Sets the Gaussian blur radius in pixels at project resolution, 0 to 100; 0 removes the blur.",
		Example:     `{"item_id": "itm_m1n2p3q4r5", "radius": 12.5}`,
		Errors:      []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:       ScopeEdit,
		Phase:       Beta,
	}, applyClipSetBlur))
}

type clipFilterArgs struct {
	ItemID string   `json:"item_id"          desc:"A clip, text, caption or shape item."                    pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Filter string   `json:"filter,omitempty" desc:"none (default), grayscale, sepia, invert, warm or cool."                                                        enum:"none,grayscale,sepia,invert,warm,cool"`
	Amount *float64 `json:"amount,omitempty" desc:"0 to 1; default 1."                                                                                                                                          min:"0" max:"1"`
}

func applyClipSetFilter(c *Context, a *clipFilterArgs) error {
	if err := atLeastOne([]string{"filter", "amount"}, a.Filter != "", a.Amount != nil); err != nil {
		return err
	}
	it, err := c.visualItem(a.ItemID, "filter")
	if err != nil {
		return err
	}
	if a.Filter != "" {
		it.Filter = a.Filter
	}
	if a.Amount != nil {
		it.FilterAmount = round3(*a.Amount)
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("filter", it.Filter)
	c.SetEffective("amount", it.FilterAmount)
	return nil
}

type clipBlurArgs struct {
	ItemID string  `json:"item_id" desc:"A clip, text, caption or shape item."                 pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Radius float64 `json:"radius"  desc:"Blur radius in pixels, 0 to 100; 0 removes the blur."                                                        min:"0" max:"100"`
}

func applyClipSetBlur(c *Context, a *clipBlurArgs) error {
	it, err := c.visualItem(a.ItemID, "blur")
	if err != nil {
		return err
	}
	it.Blur = round3(a.Radius)
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("blur", it.Blur)
	return nil
}
