package cli

// The product commands of SDD-003 section 5.4. Each handler parses, calls one
// Service method and formats; no editing rule lives here (ADR-0007).

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// registry lists the commands in the order the short list prints them.
func registry() []*spec {
	return append([]*spec{
		projectList, projectCreate, projectShow, projectTrash, projectRestore,
		edit, history, undo, redo, lintCmd, version, mcpCmd,
		mediaImport, mediaInspect, mediaList, mediaTrash, mediaRestore, view, export,
		exportPresets, exportCheck, exportBatch,
		jobShow, jobCancel, jobList, eventsCmd, commandHelp,
		templateList, templateSave, templateNew, templateExport, templateImport, bundleExport, bundleImport,
		agentDocs, transcribeCmd, doctorCmd, fontsCmd, auditCmd, toolsInstallCmd, tokenRotate,
		fontImportCmd, brandShowCmd, brandSetCmd, otioExport, otioImport,
		projectDescribeCmd, projectDuplicateCmd, itemAddCmd, itemMoveCmd, itemRenameCmd, itemTrashCmd, itemRestoreCmd,
		briefShowCmd, briefSetCmd,
		connectionListCmd, connectionShowCmd, connectionAddCmd, connectionRemoveCmd, connectionTestCmd,
		approvalListCmd, approvalShowCmd, approvalDecideCmd, approvalRuleListCmd, approvalRuleSetCmd,
		approvalRuleRemoveCmd, costShowCmd, costCeilingCmd,
		imageCreate, imageList, imageShow, imageEdit, imageUndo, imageRedo, imageExport,
	}, libraryCommands()...)
}

// CommandNames lists the commands the binary knows, and those among them that
// still answer with the task that delivers them. The parity test of
// web/contract reads it.
func CommandNames() (all, pending []string) {
	for _, s := range registry() {
		all = append(all, s.name)

		if s.pending != "" {
			pending = append(pending, s.name)
		}
	}

	return all, pending
}

func lookup(name string) *spec {
	for _, s := range registry() {
		if s.name == name {
			return s
		}
	}

	return nil
}

func pageFlags(limit string) []flagDef {
	return []flagDef{
		{Name: "cursor", Kind: kindString, Usage: "continue a list from this cursor"},
		{Name: "limit", Kind: kindInt, Default: limit, Usage: "items per page"},
	}
}

func page(p *parsed) service.Page {
	return service.Page{Cursor: p.str("cursor"), Limit: p.integer("limit")}
}

var projectList = &spec{
	name:    "project:list",
	summary: "list the projects of the workspace",
	usage:   []string{"stuarty project:list [--state active|trashed|all] [--cursor c] [--limit n]"},
	example: "stuarty project:list --json",
	flags: append([]flagDef{{Name: "state", Kind: kindString, Default: "active", Usage: "active, trashed or all"}},
		pageFlags("20")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectList(ctx, service.ProjectListRequest{State: c.args.str("state"), Page: page(c.args)})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var projectCreate = &spec{
	name:    "project:create",
	summary: "create a project",
	usage: []string{
		"stuarty project:create --name <name> [--preset 9:16|1:1|16:9|4:5] [--fps 30] [--id prj_...]",
	},
	example: `stuarty project:create --name "Launch teaser" --json`,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "project name (required)"},
		{Name: "preset", Kind: kindString, Default: "9:16", Usage: "9:16, 1:1, 16:9 or 4:5"},
		{Name: "fps", Kind: kindString, Default: "30", Usage: "frames per second, for example 30 or 30000/1001"},
		{
			Name: "id", Kind: kindString,
			Usage: "identifier for the project (omit it in network mode: the server chooses)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("name") {
			return nil, validation("name", "--name is required.")
		}

		res, err := c.svc.ProjectCreate(ctx, service.ProjectCreateRequest{
			Name: c.args.str("name"), Preset: c.args.str("preset"), FPS: c.args.str("fps"),
			ID: c.args.str("id"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var projectShow = &spec{
	name:     "project:show",
	timeline: true,
	summary:  "show a project",
	usage: []string{
		"stuarty project:show [<project id>] [--detail concise|full|document] [--cursor c] [--limit n]",
	},
	example:           "stuarty project:show --project prj_7k2m9x4q1b",
	project:           true,
	projectPositional: true,
	flags: append([]flagDef{{Name: "detail", Kind: kindString, Default: "concise", Usage: "concise, full or document"}},
		pageFlags("")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectGet(ctx, service.ProjectGetRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Detail: c.args.str("detail"), Page: page(c.args),
		})
		if err != nil {
			return nil, err
		}

		text := res.Text

		if c.args.str("detail") == "document" {
			raw, merr := json.Marshal(res.Document)
			if merr != nil {
				return nil, merr
			}

			text = string(raw)
		}

		return &output{value: res, text: text}, nil
	},
}

func projectStateCommand(
	name, summary string,
	fn func(service.Service, context.Context, service.ProjectTrashRequest) (*service.ProjectStateResult, error),
) *spec {
	return &spec{
		name:              name,
		summary:           summary,
		usage:             []string{"stuarty " + name + " <project id>"},
		example:           "stuarty " + name + " prj_7k2m9x4q1b",
		project:           true,
		projectPositional: true,
		globals:           []string{"dry-run"},
		write:             true,
		run: func(ctx context.Context, c *call) (*output, error) {
			res, err := fn(
				c.svc,
				ctx,
				service.ProjectTrashRequest{ProjectID: c.projectID, DryRun: c.args.boolean("dry-run")},
			)
			if err != nil {
				return nil, err
			}

			return &output{value: res, text: res.Text}, nil
		},
	}
}

var projectTrash = projectStateCommand("project:trash", "move a project to the trash",
	func(s service.Service, ctx context.Context, r service.ProjectTrashRequest) (*service.ProjectStateResult, error) {
		return s.ProjectTrash(ctx, r)
	})

var projectRestore = projectStateCommand("project:restore", "restore a trashed project",
	func(s service.Service, ctx context.Context, r service.ProjectTrashRequest) (*service.ProjectStateResult, error) {
		return s.ProjectRestore(ctx, r)
	})

var history = &spec{
	name:     "history",
	timeline: true,
	summary:  "list the steps of a project history",
	usage:    []string{"stuarty history --project <id> [--by a] [--cursor c] [--limit n]"},
	example:  "stuarty history --project prj_7k2m9x4q1b --json",
	project:  true,
	flags: append([]flagDef{{Name: "by", Kind: kindString, Usage: "only the steps of this actor"}},
		pageFlags("20")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.HistoryList(ctx, service.HistoryListRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Actor: c.args.str("by"), Page: page(c.args),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

func undoCommand(name, summary string, redo bool) *spec {
	return &spec{
		name:     name,
		timeline: true,
		summary:  summary,
		usage:    []string{"stuarty " + name + " --project <id>"},
		example:  "stuarty " + name + " --project prj_7k2m9x4q1b",
		project:  true,
		globals:  []string{"dry-run"},
		write:    true,
		run: func(ctx context.Context, c *call) (*output, error) {
			req := service.UndoRequest{
				ProjectID:  c.projectID,
				TimelineID: c.timelineID,
				DryRun:     c.args.boolean("dry-run"),
			}

			var (
				res *service.UndoResult
				err error
			)

			if redo {
				res, err = c.svc.Redo(ctx, req)
			} else {
				res, err = c.svc.Undo(ctx, req)
			}

			if err != nil {
				return nil, err
			}

			return &output{value: res, text: service.UndoText(res)}, nil
		},
	}
}

var (
	undo = undoCommand("undo", "undo the last step of the caller", false)
	redo = undoCommand("redo", "redo the last undone step of the caller", true)
)

var lintCmd = &spec{
	name:     "lint",
	timeline: true,
	summary:  "check a project against every lint rule (an edit result runs only the fast rules, lint_scope fast)",
	usage:    []string{"stuarty lint --project <id> [--severity error|warning] [--cursor c] [--limit n] [--strict]"},
	example:  "stuarty lint --project prj_7k2m9x4q1b --json",
	project:  true,
	flags: append([]flagDef{
		{
			Name:    "severity",
			Kind:    kindString,
			Default: "warning",
			Usage:   "error lists only errors; warning lists everything",
		},
		{Name: "strict", Kind: kindBool, Default: "false", Usage: "exit 1 when there is at least one error finding"},
	}, pageFlags("25")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.Lint(ctx, service.LintRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Severity: c.args.str("severity"), Page: page(c.args),
		})
		if err != nil {
			return nil, err
		}

		out := &output{value: res, text: res.Text}
		if c.args.boolean("strict") && res.Errors > 0 {
			out.exit = ExitFailure
		}

		return out, nil
	},
}

var commandHelp = &spec{
	name:       "command:help",
	summary:    "list the editing commands, or describe one",
	usage:      []string{"stuarty command:help [<name>]"},
	example:    "stuarty command:help item.move",
	positional: true,
	flags: []flagDef{
		{
			Name:    "domain",
			Kind:    kindString,
			Default: "video",
			Usage:   "video, or image for the commands of image documents",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) > 1 {
			return nil, validation("", "command:help takes one command name, got %d.", len(c.args.Positional))
		}

		req := service.CommandHelpRequest{Domain: c.args.str("domain")}
		if len(c.args.Positional) == 1 {
			req.Name = c.args.Positional[0]
		}

		res, err := c.svc.CommandHelp(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// versionDoc is the result of version.
type versionDoc struct {
	Version string `json:"version"`
	Text    string `json:"text"`
}

var version = &spec{
	name:    "version",
	summary: "print the version of this binary",
	usage:   []string{"stuarty version [--json]"},
	example: "stuarty version",
	offline: true,
	run: func(_ context.Context, _ *call) (*output, error) {
		v := agentdocs.Version
		text := "stuarty " + v

		return &output{value: versionDoc{Version: v, Text: text}, text: text}, nil
	},
}

// mcpDescription tells the client which workspace and server its tools act on, because a server
// started without STUARTY_HOME or STUARTY_ADDR opens the default workspace of the user (VV-114).
func mcpDescription(workspace, server string) string {
	text := "Stuarty MCP server"

	if workspace != "" {
		text += " for the workspace " + workspace
	}

	if server != "" {
		text += ", server " + server
	}

	return text
}

// mcpCmd serves the MCP tools on standard input and output (SDD-003 section
// 4.5). The Service is the one the resolver found: the owner of the workspace
// over HTTP, or this process. It stops when the client closes standard input.
// The listener, worker and runtime.json of the owner model (rule 3) arrive with
// the owner startup of the server; until then a stdio server without an owner
// runs the Service in process and the UI and CLI cannot reach it.
var mcpCmd = &spec{
	name:    "mcp",
	summary: "serve the MCP tools on standard input and output",
	usage:   []string{"stuarty mcp [--actor <label>]"},
	example: "claude mcp add stuarty -- stuarty mcp",
	stream:  true,
	run: func(ctx context.Context, c *call) (*output, error) {
		o := mcp.Options{Description: mcpDescription(c.workspace, c.server)}
		if c.args.has("actor") {
			o.Actor = c.args.str("actor")
		}

		if err := mcp.RunStdio(ctx, c.svc, o); err != nil {
			return nil, err
		}

		return &output{}, nil
	},
}
