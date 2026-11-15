package cli

import (
	"context"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// bundleExporter is the part of the Service the bundle:export command calls. SDD-000 section 1
// routes the operation outside the 15 MCP tools (exception E-3); until it is on service.Service
// the command reaches it through the connection when the connection offers it.
type bundleExporter interface {
	ProjectBundleExport(ctx context.Context, req service.ProjectBundleExportRequest) (*service.Job, error)
}

var bundleExport = &spec{
	name:    "bundle:export",
	summary: "export a project as a .vvbundle to share or to back up",
	usage:   []string{"vukem-video bundle:export --project <id> [--no-log] [--wait]"},
	example: "vukem-video bundle:export --project prj_7k2m9x4q1b --wait",
	project: true,
	globals: []string{"wait", "timeout"},
	write:   true,
	flags: []flagDef{
		{Name: "log", Kind: kindBool, Default: "true", Usage: "include the operation log (--no-log to share without history)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		be, ok := c.svc.(bundleExporter)
		if !ok {
			return nil, &service.Error{
				Code: command.CodeInvalidState, Message: "this connection cannot export a bundle.",
				Hint: "run the command on the machine that owns the workspace",
			}
		}

		includeLog := c.args.boolean("log")
		req := service.ProjectBundleExportRequest{ProjectID: c.projectID, IncludeLog: &includeLog}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait")

		if wait {
			req.WaitMS = waitSlice(deadline)
		}

		job, err := be.ProjectBundleExport(ctx, req)
		if err != nil {
			return nil, err
		}

		return jobOutput(ctx, c, job, wait, deadline)
	},
}
