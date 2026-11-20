package cli

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// The bundle commands have no MCP tool: a bundle moves a whole project as a file, which is for the
// person or the script at the terminal (SDD-003 section 7.1, exception E-3).

var bundleExport = &spec{
	name:     "bundle:export",
	timeline: true,
	summary:  "export a project as a .vvbundle to share or to back up",
	usage:    []string{"stuarty bundle:export --project <id> [--no-log] [--wait] [--out f] [--force]"},
	example:  "stuarty bundle:export --project prj_7k2m9x4q1b --out ~/teaser.vvbundle",
	project:  true,
	globals:  []string{"wait", "timeout"},
	write:    true,
	flags: append([]flagDef{
		{
			Name: "log", Kind: kindBool, Default: "true",
			Usage: "include the operation log (--no-log to share without history)",
		},
	}, outFlags()...),
	run: func(ctx context.Context, c *call) (*output, error) {
		dest, err := c.outPath()
		if err != nil {
			return nil, err
		}

		includeLog := c.args.boolean("log")
		req := service.ProjectBundleExportRequest{
			ProjectID:  c.projectID,
			TimelineID: c.timelineID,
			IncludeLog: &includeLog,
		}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait") || dest != ""

		out, err := c.runJob(ctx, wait, deadline, func(ctx context.Context, waitMS int) (*service.Job, error) {
			req.WaitMS = waitMS

			return c.svc.ProjectBundleExport(ctx, req)
		})
		if err != nil {
			return nil, err
		}

		return c.saveJobFile(ctx, out, dest)
	},
}

var bundleImport = &spec{
	name:       "bundle:import",
	summary:    "create a project from a .vvbundle file",
	usage:      []string{"stuarty bundle:import <path> [--as-new]"},
	example:    "stuarty bundle:import launch-teaser.vvbundle --as-new",
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{
			Name: "as-new", Kind: kindBool, Default: "false",
			Usage: "give the project a new id and leave the log out (required on a server with accounts)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "bundle:import takes one path, got %d.", len(c.args.Positional))
		}

		path, err := c.rt.abs(c.args.Positional[0])
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ProjectBundleImport(ctx, service.ProjectBundleImportRequest{
			Path: path, AsNew: c.args.boolean("as-new"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
