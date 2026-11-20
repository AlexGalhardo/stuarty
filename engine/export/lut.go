package export

import (
	"fmt"
	"os"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/lut"
)

// lutExt is the extension of the normalized artifact of a lut asset: the .cube file the ingest checked and kept.
const lutExt = "cube"

// lut is the compositor.LUTSource: the table of a lut asset, parsed once per export from its normalized
// artifact (SDD-004 section 3.6). The file is untrusted, so it is parsed again here with the limits of
// engine/lut whatever the ingest did.
func (f *frameSource) lut(asset string) (*lut.Table, error) {
	if t, ok := f.luts[asset]; ok {
		return t, nil
	}

	a, ok := f.s.asset(asset)
	if !ok || a.Kind != timeline.AssetLUT {
		return nil, fmt.Errorf("the asset %s is not a lookup table of the document", asset)
	}

	path, err := f.s.artifact(&a, lutExt)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("lookup table %s: %w", asset, err)
	}

	defer func() { _ = file.Close() }()

	t, err := lut.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("lookup table %s: %w", asset, err)
	}

	f.luts[asset] = t

	return t, nil
}

// lutUpload is a table that a frame of the Rust path needs in the module.
type lutUpload struct {
	asset string
	table *lut.Table
}

// frameLUTs are the tables the layers of the frame name, each once, in plan order.
func (r *rustRenderer) frameLUTs(layers []plan.Layer) ([]lutUpload, error) {
	var out []lutUpload

	for i := range layers {
		l := &layers[i]
		if l.Lut == nil || l.Kind != plan.Media {
			continue
		}

		dup := false

		for _, u := range out {
			dup = dup || u.asset == l.Lut.Asset
		}

		if dup {
			continue
		}

		t, err := r.fs.lut(l.Lut.Asset)
		if err != nil {
			return nil, fmt.Errorf("layer %s: %w", l.ItemID, err)
		}

		out = append(out, lutUpload{l.Lut.Asset, t})
	}

	return out, nil
}
