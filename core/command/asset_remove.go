package command

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "asset.remove",
		Summary: "Removes an asset that no item uses.",
		Description: "Removes the asset from the project. It fails with E_IN_USE, listing the items, while an item " +
			"uses it. The file stays in the content store.",
		Example: `{"asset_id": "ast_3c5v8n2w6d"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeInUse},
		Scope:   ScopeMedia,
		Phase:   Alpha,
	}, applyAssetRemove))
}

type assetRemoveArgs struct {
	AssetID string `json:"asset_id" desc:"The id of the asset to remove." pattern:"^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyAssetRemove(c *Context, a *assetRemoveArgs) error {
	if !timeline.ValidID(timeline.PrefixAsset, a.AssetID) {
		return Errorf(CodeValidation, "args.asset_id %q is not an asset identifier.", a.AssetID).
			WithHint("Use the id of an asset listed by project_get.").WithPath("args.asset_id")
	}
	if _, ok := c.Doc.Assets[a.AssetID]; !ok {
		return notFound("asset", a.AssetID, "args.asset_id")
	}
	var users []string
	for id := range c.Doc.Items {
		if it := c.Doc.Items[id]; it.Asset == a.AssetID || it.Lut != nil && it.Lut.Asset == a.AssetID {
			users = append(users, id)
		}
	}
	if len(users) > 0 {
		slices.Sort(users)
		return Errorf(CodeInUse, "Asset %s is used by items %s.", a.AssetID, strings.Join(users, ", ")).
			WithHint("Delete those items with item.delete, then remove the asset.").
			WithDetail("used_by", users).WithPath("args.asset_id")
	}
	delete(c.Doc.Assets, a.AssetID)
	c.SetEffective("asset_id", a.AssetID)
	return nil
}
