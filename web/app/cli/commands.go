package cli

// The product commands of SDD-003 section 5.4. Each handler parses, calls one
// Service method and formats; no editing rule lives here (ADR-0007).

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// registry lists the commands in the order the short list prints them.
func registry() []*spec {
	return append([]*spec{
		projectList,
		projectCreate,
		projectUpdate,
		projectShow,
		projectTrash,
		projectRestore,
		edit,
		history,
		undo,
		redo,
		lintCmd,
		version,
		mcpCmd,
		revertCmd,
		diffCmd,
		checkpointCreateCmd,
		checkpointListCmd,
		checkpointRenameCmd,
		checkpointRemoveCmd,
		mediaImport,
		mediaInspect,
		mediaList,
		mediaTrash,
		mediaRestore,
		view,
		export,
		exportPresets,
		exportCheck,
		exportBatch,
		jobShow,
		jobCancel,
		jobList,
		eventsCmd,
		commandHelp,
		templateList,
		templateSave,
		templateNew,
		templateExport,
		templateImport,
		bundleExport,
		bundleImport,
		agentDocs,
		transcribeCmd,
		doctorCmd,
		fontsCmd,
		auditCmd,
		toolsInstallCmd,
		tokenRotate,
		fontImportCmd,
		brandShowCmd,
		brandSetCmd,
		otioExport,
		otioImport,
		projectDescribeCmd,
		projectDuplicateCmd,
		itemAddCmd,
		itemMoveCmd,
		itemRenameCmd,
		itemTrashCmd,
		itemRestoreCmd,
		briefShowCmd,
		briefSetCmd,
		briefingQuestionsCmd,
		briefingAnswerCmd,
		connectionListCmd,
		connectionShowCmd,
		connectionAddCmd,
		connectionRemoveCmd,
		connectionTestCmd,
		approvalListCmd,
		approvalShowCmd,
		approvalDecideCmd,
		approvalRuleListCmd,
		approvalRuleSetCmd,
		approvalRuleRemoveCmd,
		costShowCmd,
		costCeilingCmd,
		providerListCmd,
		providerModelsCmd,
		providerPriceCmd,
		textGenerateCmd,
		imageGenerateCmd,
		voiceGenerateCmd,
		videoGenerateCmd,
		voiceListCmd,
		imageModelEditCmd,
		imageCreate,
		imageList,
		imageShow,
		imageEdit,
		imageUndo,
		imageRedo,
		imageExport,
		imageImport,
		imageRevert,
		imageDiff,
		imageCheckpointCreate,
		imageCheckpointList,
		imageCheckpointRename,
		imageCheckpointRemove,
		assetListCmd,
		assetShowCmd,
		assetUsesCmd,
		assetSetCmd,
		assetTagCmd,
		motionBriefCmd,
		motionCheckCmd,
		motionReplaceCmd,
		agentProfilesCmd,
		chatSessionsCmd,
		chatSessionCmd,
		chatTranscriptCmd,
	}, append(append(libraryCommands(), publishingCommands()...), writingCommands()...)...)
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
		"stuarty project:create --name <n> --description <t> --specific|--measurable|--achievable|--relevant|" +
			"--time-bound <t> --due <date>",
	},
	example: `stuarty project:create --name "Launch teaser" --description "A 30 second vertical teaser for the launch of ` +
		`my course, for people who follow me on short video, with captions and a thumbnail." ` +
		`--specific "Publish one teaser that explains the course in one sentence" ` +
		`--measurable "At least 1000 views and 50 sign-ups in the first week" ` +
		`--achievable "I already have the footage and edit one video a day" ` +
		`--relevant "The launch needs an audience before the doors open" ` +
		`--time-bound "Script on Monday, edit on Tuesday, publish on Wednesday" --due 2027-03-31 --json`,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "project name: 4 to 32 letters, digits and single spaces (required)"},
		{
			Name:  "description",
			Kind:  kindString,
			Usage: "what the project is, for whom, what it makes: 64 to 1024 characters (required)",
		},
		{
			Name:  "specific",
			Kind:  kindString,
			Usage: "goal: what exactly it will achieve, 16 to 280 characters (required)",
		},
		{Name: "measurable", Kind: kindString, Usage: "goal: how success is counted, 16 to 280 characters (required)"},
		{
			Name:  "achievable",
			Kind:  kindString,
			Usage: "goal: why it can be done with what you have, 16 to 280 characters (required)",
		},
		{Name: "relevant", Kind: kindString, Usage: "goal: why it matters, 16 to 280 characters (required)"},
		{
			Name:  "time-bound",
			Kind:  kindString,
			Usage: "goal: the schedule and the milestones, 16 to 280 characters (required)",
		},
		{Name: "due", Kind: kindString, Usage: "goal: the due date YYYY-MM-DD, today or later (required)"},
		{
			Name:  "preset",
			Kind:  kindString,
			Usage: "deprecated, creates nothing: the size is asked when a video is added",
		},
		{
			Name:  "fps",
			Kind:  kindString,
			Usage: "deprecated, creates nothing: the frame rate is asked when a video is added",
		},
		{
			Name: "id", Kind: kindString,
			Usage: "identifier for the project (omit it in network mode: the server chooses)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectCreate(ctx, service.ProjectCreateRequest{
			Name: c.args.str("name"), Description: c.args.str("description"),
			Goal: service.ProjectGoal{
				Specific: c.args.str("specific"), Measurable: c.args.str("measurable"),
				Achievable: c.args.str("achievable"), Relevant: c.args.str("relevant"),
				TimeBound: c.args.str("time-bound"), Due: c.args.str("due"),
			},
			Preset: c.args.str("preset"), FPS: c.args.str("fps"),
			ID: c.args.str("id"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var projectUpdate = &spec{
	name:    "project:update",
	summary: "change the name, the description or the goal of a project",
	usage: []string{
		"stuarty project:update <id> [any flag of project:create]",
	},
	example:           `stuarty project:update prj_7k2m9x4q1b --name "Launch teaser v2"`,
	project:           true,
	projectPositional: true,
	globals:           []string{"dry-run"},
	write:             true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "project name: 4 to 32 letters, digits and single spaces"},
		{Name: "description", Kind: kindString, Usage: "64 to 1024 characters"},
		{
			Name:  "specific",
			Kind:  kindString,
			Usage: "goal: 16 to 280 characters (a goal is sent whole: give all six goal flags)",
		},
		{Name: "measurable", Kind: kindString, Usage: "goal: 16 to 280 characters"},
		{Name: "achievable", Kind: kindString, Usage: "goal: 16 to 280 characters"},
		{Name: "relevant", Kind: kindString, Usage: "goal: 16 to 280 characters"},
		{Name: "time-bound", Kind: kindString, Usage: "goal: 16 to 280 characters"},
		{Name: "due", Kind: kindString, Usage: "goal: the due date YYYY-MM-DD"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		req := service.ProjectUpdateRequest{ProjectID: c.projectID, DryRun: c.args.boolean("dry-run")}

		if c.args.has("name") {
			v := c.args.str("name")
			req.Name = &v
		}

		if c.args.has("description") {
			v := c.args.str("description")
			req.Description = &v
		}

		goalFlags := []string{"specific", "measurable", "achievable", "relevant", "time-bound", "due"}
		if slices.ContainsFunc(goalFlags, c.args.has) {
			req.Goal = &service.ProjectGoal{
				Specific: c.args.str("specific"), Measurable: c.args.str("measurable"),
				Achievable: c.args.str("achievable"), Relevant: c.args.str("relevant"),
				TimeBound: c.args.str("time-bound"), Due: c.args.str("due"),
			}
		}

		res, err := c.svc.ProjectUpdate(ctx, req)
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
	usage: []string{"stuarty lint --project <id> [--severity error|warning] [--cursor c] [--limit n] [--strict]",
		"stuarty lint --project <id> --preset <id> [--start t] [--end t]"},
	example: "stuarty lint --project prj_7k2m9x4q1b --json",
	project: true,
	flags: append([]flagDef{
		{
			Name:    "severity",
			Kind:    kindString,
			Default: "warning",
			Usage:   "error lists only errors; warning lists everything",
		},
		{Name: "strict", Kind: kindBool, Default: "false", Usage: "exit 1 when there is at least one error finding"},
		{Name: "preset", Kind: kindString, Usage: "the fit check of an export preset instead of the lint, as export:check"},
		{Name: "start", Kind: kindString, Usage: "with --preset: first time of the range to check"},
		{Name: "end", Kind: kindString, Usage: "with --preset: end of the range to check"},
	}, pageFlags("25")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		// The lint tool of MCP takes a preset as the fit check; the CLI takes the same (SDD-003 section 4).
		if c.args.str("preset") != "" {
			chk, err := c.svc.ExportCheck(ctx, service.ExportCheckRequest{
				ProjectID: c.projectID, TimelineID: c.timelineID, Preset: c.args.str("preset"),
				Start: c.args.str("start"), End: c.args.str("end"),
			})
			if err != nil {
				return nil, err
			}

			return &output{value: chk, text: chk.Text}, nil
		}

		if c.args.str("start") != "" || c.args.str("end") != "" {
			return nil, validation("start", "--start and --end are the range of the fit check: they need --preset.")
		}

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
	summary:    "list the editing commands and the domains, or describe one command",
	usage:      []string{"stuarty command:help [<name>]", "stuarty command:help --domain <domain>"},
	example:    "stuarty command:help item.move",
	positional: true,
	flags: []flagDef{
		{
			Name:  "domain",
			Kind:  kindString,
			Usage: "a domain of the product: its commands (without it, the video commands and the list of domains)",
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
