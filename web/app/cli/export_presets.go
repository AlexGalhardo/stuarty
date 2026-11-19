package cli

// The export presets, the check against one and the batch of exports (SDD-004 sections 8.2 to 8.4). Each
// handler parses, calls one Service method and formats; the rules live in core/export and in the service.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

var exportPresets = &spec{
	name:    "export:presets",
	summary: "list the export presets of the platforms",
	usage:   []string{"stuarty export:presets [--codecs]"},
	example: "stuarty export:presets --codecs --json",
	flags: []flagDef{
		{
			Name: "codecs", Kind: kindBool, Default: "false",
			Usage: "also tell which codecs this machine can write (runs the test encodes the first time)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ExportPresetList(ctx, service.ExportPresetListRequest{Codecs: c.args.boolean("codecs")})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var exportCheck = &spec{
	name:    "export:check",
	summary: "check that a project fits an export preset, before any encoding",
	usage:   []string{"stuarty export:check --project <id> --preset <id> [--start t] [--end t] [--codec c]"},
	example: "stuarty export:check --project prj_7k2m9x4q1b --preset tiktok --json",
	project: true,
	flags: []flagDef{
		{Name: "preset", Kind: kindString, Usage: "the preset id, as export:presets lists them"},
		{Name: "start", Kind: kindString, Usage: "first time to export (default: the start)"},
		{Name: "end", Kind: kindString, Usage: "end of the export (default: the end of the timeline)"},
		{
			Name:  "codec",
			Kind:  kindString,
			Usage: "h264, hevc or av1, for the estimate of the size (default: the preset's first)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ExportCheck(ctx, service.ExportCheckRequest{
			ProjectID: c.projectID, Preset: c.args.str("preset"), Start: c.args.str("start"), End: c.args.str("end"),
			Codec: c.args.str("codec"),
		})
		if err != nil {
			return nil, err
		}

		out := &output{value: res, text: res.Text}
		if !res.OK {
			out.exit = ExitFailure
		}

		return out, nil
	},
}

var exportBatch = &spec{
	name:    "export:batch",
	summary: "export a project several times, one after the other, as one job",
	usage: []string{
		// The first line is the one of the agent skill, whose size is capped: --preset repeats, and --timeout is global.
		"stuarty export:batch --project <id> --preset <id>... [--codec c] [--start t] [--end t] [--wait]",
		"stuarty export:batch --project <id> --items '<json array of {preset, format, codec, start, end}>' [--wait]",
	},
	example: "stuarty export:batch --project prj_7k2m9x4q1b --preset tiktok --preset youtube-shorts --wait",
	project: true,
	globals: []string{"wait", "timeout"},
	write:   true,
	flags: []flagDef{
		{Name: "preset", Kind: kindString, Repeat: true, Usage: "a preset id; give the flag once for each export"},
		{
			Name:  "items",
			Kind:  kindString,
			Usage: "the exports as a JSON array of {preset, format, codec, start, end}, instead of --preset",
		},
		{Name: "format", Kind: kindString, Usage: "the format of every --preset export that is not mp4 (default mp4)"},
		{
			Name:  "codec",
			Kind:  kindString,
			Usage: "h264, hevc or av1 for every --preset export (default: the choice of the preset)",
		},
		{
			Name:  "start",
			Kind:  kindString,
			Usage: "first time to export, for every --preset export (default: the start)",
		},
		{
			Name:  "end",
			Kind:  kindString,
			Usage: "end of the export, for every --preset export (default: the end of the timeline)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		items, err := batchItems(c.args)
		if err != nil {
			return nil, err
		}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait")

		return c.runJob(ctx, wait, deadline, func(ctx context.Context, waitMS int) (*service.Job, error) {
			return c.svc.ExportBatch(ctx, service.ExportBatchRequest{
				ProjectID: c.projectID, Items: items, WaitMS: waitMS,
			})
		})
	},
}

// batchItems reads the items of a batch: the JSON array of --items, or one item for each --preset.
func batchItems(p *parsed) ([]service.ExportBatchItem, error) {
	if p.has("items") {
		if p.has("preset") {
			return nil, validation("items", "give --items or --preset, not both.")
		}

		var items []service.ExportBatchItem
		if err := json.Unmarshal([]byte(p.str("items")), &items); err != nil {
			return nil, validation("items", "--items is not a JSON array of exports: %v.", err)
		}

		return items, nil
	}

	presets := p.all("preset")
	if len(presets) == 0 {
		return nil, validation("preset", "give at least one --preset, or --items.")
	}

	items := make([]service.ExportBatchItem, len(presets))
	for i, id := range presets {
		items[i] = service.ExportBatchItem{
			Preset: id, Format: p.str("format"), Codec: p.str("codec"), Start: p.str("start"), End: p.str("end"),
		}
	}

	return items, nil
}
