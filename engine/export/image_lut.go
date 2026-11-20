package export

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/lut"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// putImageLUT uploads the table a layer names before the layer is drawn (SDD-004 section 3.6), once per table:
// the layer is never drawn without it. A layer with no table asks for nothing.
func putImageLUT(
	ctx context.Context, host *renderhost.Host, l *plan.Layer, uploaded map[string]bool, cfg ImageConfig,
) error {
	if l.Lut == nil || uploaded[l.Lut.Asset] {
		return nil
	}

	t, err := imageLUT(l, cfg)
	if err != nil {
		return err
	}

	if err := host.PutLUT(ctx, l.Lut.Asset, t); err != nil {
		return rendererError(err)
	}

	uploaded[l.Lut.Asset] = true

	return nil
}

// imageLUT is the table a layer of an image document names (SDD-021 section 3.1): the normalized .cube file of
// its asset, parsed by engine/lut, the only parser (SDD-004 section 3.6). The file is untrusted, so it is parsed
// again here with the limits of the parser whatever the ingest did, and a file that no longer parses fails the
// export: a layer is never drawn without the table it asked for.
func imageLUT(l *plan.Layer, cfg ImageConfig) (*lut.Table, error) {
	if cfg.LUTPath == nil {
		return nil, exportErr(ReasonRender, "No lookup table files are available.", errors.New("no lut source"), "")
	}

	path, err := cfg.LUTPath("sha256:" + l.Lut.Asset) // the plan names a table by the digits of its hash
	if err != nil {
		return nil, exportErr(
			ReasonRender,
			"A lookup table of the image is not available.",
			fmt.Errorf("layer %s: %w", l.ItemID, err),
			"",
		)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, exportErr(
			ReasonRender,
			"A lookup table of the image is not available.",
			fmt.Errorf("layer %s: %w", l.ItemID, err),
			"",
		)
	}

	defer func() { _ = file.Close() }()

	t, err := lut.Parse(file)
	if err != nil {
		return nil, exportErr(
			ReasonRender,
			"A lookup table of the image cannot be read.",
			fmt.Errorf("layer %s: %w", l.ItemID, err),
			"",
		)
	}

	return t, nil
}
