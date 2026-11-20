package image

import (
	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(command.DefineIn(command.Spec{
		Name:    "layer.replace_source",
		Summary: "Gives an image layer another file and keeps the layer.",
		Description: "asset is the content hash of an image file already imported. The layer keeps its id, name, slot, " +
			"visibility, opacity, rotation, adjustments, filters, mask and blend. When the new file has the size of " +
			"the old one the box and the crop stay as they were; otherwise the crop is removed and the box stays " +
			"when the new file has its proportions, and is the largest box with the proportions of the new file that " +
			"fits inside it, centred, when it has others. effective names layer_id, box and asset.",
		Example: `{"layer_id": "lyr_a1b2c3d4e5", "asset": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}`,
		Errors: []string{
			command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			command.CodeLimit,
		},
		Scope: command.ScopeEdit,
	}, applyReplaceSource))
}

type replaceSourceArgs struct {
	LayerID string `json:"layer_id" desc:"The image layer."                                           pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Asset   string `json:"asset"    desc:"The sha256 content hash of the new image in the workspace." pattern:"^sha256:[0-9a-f]{64}$"`
}

// replaceTarget is the layer layer.replace_source changes: it must exist, not be locked and be an image layer.
func replaceTarget(d *Document, id string) (*Layer, *command.Error) {
	l, err := target(d, id, "args.layer_id")
	if err != nil {
		return nil, err
	}

	if l.Kind != KindImage {
		return nil, command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.replace_source changes image layers.", l.ID, l.Kind).
			WithHint("Use an image layer.").
			WithPath("args.layer_id")
	}

	return l, nil
}

// CheckReplaceable reports whether layer.replace_source could change the layer, as far as the layer goes: it
// exists, is an image layer and is not locked. The service asks it before an operation that costs money starts,
// so the rule is the core's and is written once.
func CheckReplaceable(d *Document, layerID string) (*Layer, error) {
	l, err := replaceTarget(d, layerID)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func applyReplaceSource(c *command.Context, d *Document, a *replaceSourceArgs) error {
	l, err := replaceTarget(d, a.LayerID)
	if err != nil {
		return err
	}

	if !hashPattern.MatchString(a.Asset) {
		return invalid("args.asset", "args.asset %q is not a content hash.", a.Asset).
			WithHint("A content hash is sha256: and 64 lower case hexadecimal digits.")
	}

	info, ok := lookup(envOf(c), a.Asset)
	if !ok {
		return command.Errorf(command.CodeNotFound, "The file %s is not a ready image of the workspace.", a.Asset).
			WithHint("Import the image with media:import and use the hash it returns.").
			WithDetail("kind", "asset").WithDetail("id", a.Asset).WithPath("args.asset")
	}

	old, hadOld := d.Assets[l.Asset]
	sameSize := hadOld && old.Width == info.Width && old.Height == info.Height

	if d.Assets == nil {
		d.Assets = map[string]AssetRef{}
	}

	d.Assets[a.Asset] = AssetRef{Hash: a.Asset, Name: info.Name, Width: info.Width, Height: info.Height}
	l.Asset = a.Asset

	if !sameSize {
		l.Crop = nil

		if l.W*info.Height != l.H*info.Width {
			b := fitBox(info.Width, info.Height, l.X, l.Y, l.W, l.H, true)
			l.X, l.Y, l.W, l.H = b.x, b.y, b.w, b.h
		}
	}

	// The old file leaves the table when no other layer uses it; the ceiling is judged after that, on the
	// table the document would be left with, and the batch is applied to a private copy, so a refusal changes nothing.
	pruneAssets(d)

	if err := limit("images", len(d.Assets)-1, MaxAssets); err != nil {
		return err
	}

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("box", boxOf(l))
	c.SetEffective("asset", a.Asset)

	return nil
}
