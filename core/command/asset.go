package command

import (
	"regexp"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

var hashForm = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func init() {
	Register(Define(Spec{
		Name:    "asset.add",
		Summary: "Adds an asset that the import flow has already ingested.",
		Description: "Adds the asset with the facts measured on the normalized media. An asset whose hash is already in " +
			"the project is not added again: its identifier is returned in effective.asset_id. Only the import flow issues it.",
		Example: `{"asset": {"id": "ast_3c5v8n2w6d", "hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", ` +
			`"kind": "video", "name": "IMG_0421.MOV", "frame_rate": {"num": 30, "den": 1}, "frame_count": 900, ` +
			`"sample_count": 1440000, "width": 1080, "height": 1920, "has_audio": true}}`,
		Errors:   []string{CodeValidation, CodeIDExists, CodeLimit},
		Scope:    ScopeMedia,
		Phase:    Prototype,
		Internal: true,
	}, applyAssetAdd))
}

type assetAddArgs struct {
	Asset timeline.Asset `json:"asset" desc:"An Asset object of SDD-001 section 3.2."`
}

func missing(field, kind, hint string) *Error {
	return Errorf(CodeValidation, "args.asset.%s is required for kind %q.", field, kind).
		WithHint("%s", hint).WithPath("args.asset." + field)
}

// checkAsset applies the shape rules of asset.add (C.4 Arguments).
func checkAsset(a *timeline.Asset) *Error {
	if !timeline.ValidID(timeline.PrefixAsset, a.ID) {
		return Errorf(CodeValidation, "args.asset.id %q is not an asset id.", a.ID).
			WithHint("Use an id like \"ast_3c5v8n2w6d\".").WithPath("args.asset.id")
	}
	if !hashForm.MatchString(a.Hash) {
		return Errorf(
			CodeValidation,
			"args.asset.hash %q is not sha256: followed by 64 lower-case hex characters.",
			a.Hash,
		).
			WithHint("Send the hash of the original file, for example sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.").
			WithPath("args.asset.hash")
	}
	const probe = "Send the facts measured on the normalized media: "
	hasAudio := a.Kind == timeline.AssetAudio || a.Kind == timeline.AssetVideo && a.HasAudio
	switch a.Kind {
	case timeline.AssetVideo:
		switch {
		case a.FrameRate.Num < 1 || a.FrameRate.Den < 1:
			return missing("frame_rate", "video", probe+"frame_rate, frame_count, width and height.")
		case a.FrameCount < 1:
			return missing("frame_count", "video", probe+"frame_rate, frame_count, width and height.")
		case a.Width < 1 || a.Height < 1:
			return missing("width", "video", probe+"frame_rate, frame_count, width and height.")
		}
	case timeline.AssetAudio:
	case timeline.AssetImage:
		if a.Width < 1 || a.Height < 1 {
			return missing("width", "image", probe+"width and height.")
		}
	case timeline.AssetLUT:
	default:
		return Errorf(CodeValidation, "args.asset.kind %q is not video, audio, image or lut.", a.Kind).
			WithHint("Use \"video\", \"audio\", \"image\" or \"lut\".").WithPath("args.asset.kind")
	}
	if hasAudio && a.SampleCount < 1 {
		return Errorf(CodeValidation, "args.asset.sample_count is required for an asset with audio (kind %q).", a.Kind).
			WithHint(probe + "sample_count at 48000 Hz.").WithPath("args.asset.sample_count")
	}
	if !hasAudio && a.SampleCount != 0 {
		return Errorf(CodeValidation, "args.asset.sample_count is given but the asset has no audio.").
			WithHint("Omit sample_count, or send has_audio true.").WithPath("args.asset.sample_count")
	}
	return nil
}

func applyAssetAdd(c *Context, a *assetAddArgs) error {
	asset := a.Asset
	if err := checkAsset(&asset); err != nil {
		return err
	}
	for _, id := range sortedAssetIDs(c.Doc) {
		if existing := c.Doc.Assets[id]; existing.Hash == asset.Hash {
			c.SetEffective("asset_id", id)
			c.SetEffective("existing", true)
			return nil
		}
	}
	if _, used := c.Doc.Assets[asset.ID]; used {
		return Errorf(CodeIDExists, "Asset id %s already exists with a different hash.", asset.ID).
			WithHint("Send a new id, or omit it so the service generates one.").WithPath("args.asset.id")
	}
	if err := c.limit("assets", len(c.Doc.Assets), timeline.MaxAssets); err != nil {
		return err
	}
	c.Doc.Assets[asset.ID] = asset
	c.SetEffective("asset_id", asset.ID)
	c.SetEffective("existing", false)
	return nil
}

func sortedAssetIDs(doc *timeline.Document) []string {
	ids := make([]string, 0, len(doc.Assets))
	for id := range doc.Assets {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
