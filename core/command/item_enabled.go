package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.set_enabled",
		Summary: "Turns an item off or on without removing it from the timeline.",
		Description: "enabled false keeps the item, its place and its settings, and makes the preview, the export and the " +
			"audio mix ignore it; true turns it back on. Works on every item type.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "enabled": false}`,
		Errors:  []string{CodeValidation, CodeNotFound},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyItemSetEnabled))
}

type itemEnabledArgs struct {
	ItemID  string `json:"item_id" desc:"Any item."                                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Enabled *bool  `json:"enabled" desc:"false turns the item off, true turns it on."`
}

func applyItemSetEnabled(c *Context, a *itemEnabledArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if a.Enabled == nil {
		return invalidArg("args.enabled", "args.enabled is required.").WithHint("Send true or false.")
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	it.Disabled = !*a.Enabled
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("enabled", *a.Enabled)
	return nil
}
