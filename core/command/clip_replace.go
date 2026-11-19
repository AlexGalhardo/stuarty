package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

func init() {
	Register(Define(Spec{
		Name:    "clip.replace_media",
		Summary: "Swaps the asset of a clip for another of the same kind and keeps everything else.",
		Description: "Replaces the asset a clip plays, a video by a video or an image by an image, and keeps its position, " +
			"duration, trim, speed, transform, keyframes and effects: the animation an external engine rendered again takes " +
			"the place of the one it replaces. Nothing is stretched or trimmed: a video the clip's range does not fit is refused.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "asset_id": "ast_k8m2n4p6q9"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeOutOfSource},
		Scope:   ScopeEdit,
	}, applyClipReplaceMedia))
}

type clipReplaceMediaArgs struct {
	ItemID  string `json:"item_id"  desc:"A clip."                                                                       pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	AssetID string `json:"asset_id" desc:"A video or image asset of the document, of the same kind as the clip's asset." pattern:"^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyClipReplaceMedia(c *Context, a *clipReplaceMediaArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if err := idForm("args.asset_id", timeline.PrefixAsset, a.AssetID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemClip {
		return Errorf(
			CodeTypeMismatch,
			"Item %s is %s item; clip.replace_media applies to clips.",
			a.ItemID,
			article(it.Type),
		).
			WithHint("Use audio.add for the sound of an audio item.").
			WithPath("args.item_id")
	}
	if it.Asset == timeline.PlaceholderAsset {
		return Errorf(CodeInvalidState, "Item %s is a template slot holder.", a.ItemID).
			WithHint("Use slot.fill to put media in a slot.").WithPath("args.item_id")
	}
	next, err := c.asset(a.AssetID, "args.asset_id")
	if err != nil {
		return err
	}
	current := c.Doc.Assets[it.Asset]
	if next.Kind != timeline.AssetVideo && next.Kind != timeline.AssetImage {
		return Errorf(
			CodeTypeMismatch,
			"Asset %s is %s asset; clip.replace_media places video or images.",
			a.AssetID,
			assetArticle(next.Kind),
		).
			WithHint("Use audio.add on an audio track.").
			WithPath("args.asset_id")
	}
	if next.Kind != current.Kind {
		return Errorf(
			CodeTypeMismatch,
			"Asset %s is %s asset; clip %s plays %s asset.",
			a.AssetID,
			assetArticle(next.Kind),
			a.ItemID,
			assetArticle(current.Kind),
		).
			WithHint("Replace a video with a video and an image with an image; to change the kind, add a new clip with clip.add.").
			WithPath("args.asset_id")
	}
	c.SetEffective("asset_id", a.AssetID)
	c.SetEffective("previous_asset_id", it.Asset)
	if it.Asset == a.AssetID {
		return nil
	}
	it.Asset = a.AssetID
	if err := c.checkSource(&it, it.Duration, it.SourceIn); err != nil {
		return err.WithPath("args.asset_id")
	}
	c.Doc.Items[a.ItemID] = it
	return nil
}

// assetArticle writes an asset kind with its article: "a video", "an image".
func assetArticle(k timeline.AssetKind) string {
	if k == timeline.AssetAudio || k == timeline.AssetImage {
		return "an " + string(k)
	}
	return "a " + string(k)
}
