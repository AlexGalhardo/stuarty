package cli

import (
	"context"
	"io"
	"os"

	"github.com/AlexGalhardo/stuarty/service"
)

// The commands on the production of a project (SDD-003 section 5.4): its description, its copy, its items and
// its brief.

var projectDescribeCmd = &spec{
	name:              "project:describe",
	summary:           "describe a project as text: brief, items and state",
	usage:             []string{"stuarty project:describe <project id>"},
	example:           "stuarty project:describe prj_7k2m9x4q1b",
	project:           true,
	projectPositional: true,
	flags: []flagDef{
		{Name: "max-tokens", Kind: kindInt, Usage: "the budget of the text in tokens, 200 to 6000 (default 1500)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectDescribe(ctx, service.ProjectDescribeRequest{
			ProjectID: c.projectID, MaxTokens: c.args.integer("max-tokens"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var projectDuplicateCmd = &spec{
	name:              "project:duplicate",
	summary:           "copy a project: its brief, texts and timelines",
	usage:             []string{"stuarty project:duplicate <project id> [--name n]"},
	example:           `stuarty project:duplicate prj_7k2m9x4q1b --name "Teaser, second cut"`,
	project:           true,
	projectPositional: true,
	globals:           []string{"dry-run"},
	write:             true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "the name of the copy (default: the name and the word copy)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectDuplicate(ctx, service.ProjectDuplicateRequest{
			ProjectID: c.projectID, Name: c.args.str("name"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// readText reads a Markdown text from a file of the current directory, or from standard input for -.
func (c *call) readText(flag, src string) (string, error) {
	var (
		raw []byte
		err error
	)

	if src == "-" {
		raw, err = io.ReadAll(c.rt.Stdin)
	} else {
		raw, err = os.ReadFile(src)
	}

	if err != nil {
		return "", validation(flag, "cannot read the text %s: %s.", src, readReason(err))
	}

	return string(raw), nil
}

var itemAddCmd = &spec{
	name:    "item:add",
	summary: "add a text, a timeline or an output to a project",
	usage: []string{
		"stuarty item:add --project <id> --kind text|timeline|output --title <t>",
		"stuarty item:add --project <id> --kind text --title <t> --text <file|->",
		"stuarty item:add --project <id> --kind output --title <t> --job <export job id>",
	},
	example: `stuarty item:add --project prj_7k2m9x4q1b --kind timeline --title "Wide cut" --preset 16:9`,
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "text, timeline or output (required)"},
		{Name: "title", Kind: kindString, Usage: "1 to 120 characters (required)"},
		{Name: "position", Kind: kindInt, Usage: "where in the order of the items, from 0 (default: the end)"},
		{Name: "text", Kind: kindString, Usage: "a text item: a file with its Markdown, or - for standard input"},
		{Name: "preset", Kind: kindString, Usage: "a timeline: 9:16 (default), 1:1, 16:9 or 4:5"},
		{Name: "fps", Kind: kindString, Usage: "a timeline: 30 (default), or n/1001"},
		{Name: "job", Kind: kindString, Usage: "an output: a succeeded export job of this project"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"kind", "title"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		req := service.ProjectItemAddRequest{
			ProjectID: c.projectID, Kind: c.args.str("kind"), Title: c.args.str("title"), Preset: c.args.str("preset"),
			FPS: c.args.str("fps"), JobID: c.args.str("job"), DryRun: c.args.boolean("dry-run"),
		}

		if c.args.has("position") {
			p := c.args.integer("position")
			req.Position = &p
		}

		if c.args.has("text") {
			text, err := c.readText("text", c.args.str("text"))
			if err != nil {
				return nil, err
			}

			req.Text = text
		}

		res, err := c.svc.ProjectItemAdd(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// itemCommand is an item command that takes the item id as its positional argument.
func itemCommand(
	name, summary, extraUsage string,
	flags []flagDef,
	run func(ctx context.Context, c *call, item string) (*output, error),
) *spec {
	return &spec{
		name:       name,
		summary:    summary,
		usage:      []string{"stuarty " + name + " <item id> --project <id>" + extraUsage},
		example:    "stuarty " + name + " pit_3c5v8n2w6d --project prj_7k2m9x4q1b" + extraUsage,
		project:    true,
		positional: true,
		globals:    []string{"dry-run"},
		write:      true,
		flags:      flags,
		run: func(ctx context.Context, c *call) (*output, error) {
			if len(c.args.Positional) != 1 {
				return nil, validation("", "%s takes one item id, got %d.", name, len(c.args.Positional))
			}

			return run(ctx, c, c.args.Positional[0])
		},
	}
}

var itemMoveCmd = itemCommand("item:move", "move an item to a position in the order of the project", " --position n",
	[]flagDef{{Name: "position", Kind: kindInt, Usage: "where in the order of the items, from 0 (required)"}},
	func(ctx context.Context, c *call, item string) (*output, error) {
		if !c.args.has("position") {
			return nil, validation("position", "--position is required.")
		}

		res, err := c.svc.ProjectItemMove(ctx, service.ProjectItemMoveRequest{
			ProjectID: c.projectID,
			ItemID:    item,
			Position:  c.args.integer("position"),
			DryRun:    c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	})

var itemRenameCmd = itemCommand("item:rename", "change the title of an item", ` --title "..."`,
	[]flagDef{{Name: "title", Kind: kindString, Usage: "the new title, 1 to 120 characters (required)"}},
	func(ctx context.Context, c *call, item string) (*output, error) {
		if !c.args.has("title") {
			return nil, validation("title", "--title is required.")
		}

		res, err := c.svc.ProjectItemRename(ctx, service.ProjectItemRenameRequest{
			ProjectID: c.projectID, ItemID: item, Title: c.args.str("title"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	})

var itemTrashCmd = itemCommand("item:trash", "move an item to the trash of its project", "", nil,
	func(ctx context.Context, c *call, item string) (*output, error) {
		res, err := c.svc.ProjectItemTrash(ctx, service.ProjectItemTrashRequest{
			ProjectID: c.projectID, ItemID: item, DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	})

var itemRestoreCmd = itemCommand("item:restore", "restore an item from the trash of its project", "", nil,
	func(ctx context.Context, c *call, item string) (*output, error) {
		res, err := c.svc.ProjectItemRestore(ctx, service.ProjectItemTrashRequest{
			ProjectID: c.projectID, ItemID: item, DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	})

// briefText is the text of a brief for the terminal: the Markdown as it is, with the note that it is data.
func briefText(res *service.BriefResult) string {
	if res.ItemID == "" {
		return "the project has no brief; write one with brief:set"
	}

	return res.Text
}

var briefShowCmd = &spec{
	name:    "brief:show",
	summary: "show the brief of a project",
	usage:   []string{"stuarty brief:show --project <id>"},
	example: "stuarty brief:show --project prj_7k2m9x4q1b",
	project: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.BriefGet(ctx, service.BriefGetRequest{ProjectID: c.projectID})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: briefText(res)}, nil
	},
}

var briefSetCmd = &spec{
	name:    "brief:set",
	summary: "write the brief of a project from a Markdown file",
	usage:   []string{"stuarty brief:set --project <id> --text <file|->"},
	example: "stuarty brief:set --project prj_7k2m9x4q1b --text brief.md",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{
			Name:  "text",
			Kind:  kindString,
			Usage: "the Markdown: a file, or - for standard input; an empty text clears the brief (required)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("text") {
			return nil, validation("text", "--text is required.")
		}

		text, err := c.readText("text", c.args.str("text"))
		if err != nil {
			return nil, err
		}

		res, err := c.svc.BriefSet(ctx, service.BriefSetRequest{
			ProjectID: c.projectID, Text: text, DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: briefText(res)}, nil
	},
}
