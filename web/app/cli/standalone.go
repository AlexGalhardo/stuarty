package cli

import (
	"context"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

var videoStandaloneListCmd = &spec{
	name:    "video:standalone-list",
	summary: "list standalone video editor sessions",
	usage:   []string{"stuarty video:standalone-list [--state active|trashed|all] [--cursor c] [--limit n]"},
	example: "stuarty video:standalone-list --json",
	flags: append([]flagDef{{Name: "state", Kind: kindString, Default: "active", Usage: "active, trashed or all"}},
		pageFlags("20")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.StandaloneVideoList(ctx, service.StandaloneVideoListRequest{
			State: c.args.str("state"),
			Page:  page(c.args),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var videoQuickOpenCmd = &spec{
	name:    "video:quick-open",
	summary: "open a standalone video editor session from a file, library asset or blank preset",
	usage: []string{
		"stuarty video:quick-open [path] [--asset sha256:...] [--preset 9:16|16:9|1:1|4:5] [--name n]",
	},
	example:    `stuarty video:quick-open ./clip.mp4 --json`,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "path", Kind: kindString, Usage: "path to a video file to import onto V1"},
		{Name: "asset", Kind: kindString, Usage: "sha256: content hash of a library asset to open"},
		{Name: "preset", Kind: kindString, Usage: "canvas preset: 9:16, 16:9, 1:1 or 4:5"},
		{Name: "fps", Kind: kindString, Usage: "frame rate, e.g. 30 or 30000/1001"},
		{Name: "name", Kind: kindString, Usage: "title for the standalone edit"},
		{Name: "wait-ms", Kind: kindInt, Usage: "milliseconds to wait for ingest (0 to 25000)"},
		{Name: "id", Kind: kindString, Usage: "optional prj_ identifier"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		p := c.args.str("path")
		if p == "" && len(c.args.Positional) > 0 {
			p = c.args.Positional[0]
		}

		res, err := c.svc.StandaloneVideoCreate(ctx, service.StandaloneVideoCreateRequest{
			Name:      c.args.str("name"),
			Preset:    c.args.str("preset"),
			FPS:       c.args.str("fps"),
			Path:      p,
			AssetHash: c.args.str("asset"),
			WaitMS:    c.args.integer("wait-ms"),
			ID:        c.args.str("id"),
			DryRun:    c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var videoPromoteCmd = &spec{
	name:              "video:promote",
	summary:           "promote a standalone video session into a full production project",
	usage:             []string{"stuarty video:promote <id> --name <n> --description <t> --specific ... --due <date>"},
	example:           `stuarty video:promote prj_7k2m9x4q1b --name "Launch teaser" --description "..." --specific "..." --measurable "..." --achievable "..." --relevant "..." --time-bound "..." --due 2027-03-31`,
	project:           true,
	projectPositional: true,
	globals:           []string{"dry-run"},
	write:             true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "project name: 4 to 32 letters, digits and single spaces (required)"},
		{Name: "description", Kind: kindString, Usage: "project description: 64 to 1024 characters (required)"},
		{Name: "specific", Kind: kindString, Usage: "SMART goal: specific, 16 to 280 characters (required)"},
		{Name: "measurable", Kind: kindString, Usage: "SMART goal: measurable, 16 to 280 characters (required)"},
		{Name: "achievable", Kind: kindString, Usage: "SMART goal: achievable, 16 to 280 characters (required)"},
		{Name: "relevant", Kind: kindString, Usage: "SMART goal: relevant, 16 to 280 characters (required)"},
		{Name: "time-bound", Kind: kindString, Usage: "SMART goal: time-bound, 16 to 280 characters (required)"},
		{Name: "due", Kind: kindString, Usage: "SMART goal due date, YYYY-MM-DD (required)"},
		{Name: "kind", Kind: kindString, Usage: "project kind: video (default), text, image or undecided"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.StandalonePromote(ctx, service.StandalonePromoteRequest{
			ProjectID:   c.projectID,
			Name:        c.args.str("name"),
			Description: c.args.str("description"),
			Goal: service.ProjectGoal{
				Specific:   c.args.str("specific"),
				Measurable: c.args.str("measurable"),
				Achievable: c.args.str("achievable"),
				Relevant:   c.args.str("relevant"),
				TimeBound:  c.args.str("time-bound"),
				Due:        c.args.str("due"),
			},
			Kind:   c.args.str("kind"),
			DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageQuickOpenCmd = &spec{
	name:       "image:quick-open",
	summary:    "open a standalone image document from a workspace asset hash or file path",
	usage:      []string{"stuarty image:quick-open [path|--asset sha256:...] [--name n] [--preset p]"},
	example:    `stuarty image:quick-open --asset sha256:... --json`,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "asset", Kind: kindString, Usage: "sha256: content hash of an image in the workspace library"},
		{Name: "path", Kind: kindString, Usage: "path to an image file on disk"},
		{Name: "name", Kind: kindString, Usage: "name for the new image document"},
		{Name: "preset", Kind: kindString, Usage: "optional canvas preset"},
		{Name: "width", Kind: kindInt, Usage: "optional canvas width (1 to 4096)"},
		{Name: "height", Kind: kindInt, Usage: "optional canvas height (1 to 4096)"},
		{Name: "wait-ms", Kind: kindInt, Usage: "milliseconds to wait for ingest (0 to 25000)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		assetHash := c.args.str("asset")
		p := c.args.str("path")

		if assetHash == "" && p == "" && len(c.args.Positional) > 0 {
			arg := c.args.Positional[0]
			if strings.HasPrefix(arg, "sha256:") {
				assetHash = arg
			} else {
				p = arg
			}
		}

		if p != "" && assetHash == "" {
			res, err := c.svc.ImageImport(ctx, service.ImageImportRequest{
				Path:   p,
				Name:   c.args.str("name"),
				Preset: c.args.str("preset"),
				Width:  c.args.integer("width"),
				Height: c.args.integer("height"),
				WaitMS: c.args.integer("wait-ms"),
				DryRun: c.args.boolean("dry-run"),
			})
			if err != nil {
				return nil, err
			}

			return &output{value: res, text: res.Text}, nil
		}

		res, err := c.svc.ImageOpenFromAsset(ctx, service.ImageOpenFromAssetRequest{
			AssetHash: assetHash,
			Name:      c.args.str("name"),
			Preset:    c.args.str("preset"),
			Width:     c.args.integer("width"),
			Height:    c.args.integer("height"),
			WaitMS:    c.args.integer("wait-ms"),
			DryRun:    c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
