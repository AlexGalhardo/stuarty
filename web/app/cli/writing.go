package cli

// The commands of the writing domain (SDD-019 section 8): the texts of a production, their revisions, the checks
// of a caption and of a script, and the draft made with a model. Each handler parses, calls one Service method and
// prints what it answers; the counts and the rules are core/writing's.

import (
	"context"
	"os"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

func writingCommands() []*spec {
	return []*spec{
		textListCmd, textShowCmd, textWriteCmd, textRevisionsCmd, textRevertCmd, textDraftCmd,
		captionValidateCmd, scriptEstimateCmd, postCheckCmd,
	}
}

var postCheckCmd = &spec{
	name:       "post:check",
	summary:    "check a blog post (SDD-019 section 13): its front matter, its images and its sanitised HTML",
	usage:      []string{"stuarty post:check <item> --project <id>"},
	example:    "stuarty post:check pit_3c5v8n2w6d --project prj_7k2m9x4q1b",
	positional: true,
	flags:      []flagDef{optionalProjectFlag},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "post:check", true)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PostCheck(ctx, service.PostCheckRequest{ProjectID: projectOf(c), ItemID: item})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// onePositional is the item id a writing command takes, or an error when there are several.
func onePositional(c *call, name string, required bool) (string, error) {
	switch n := len(c.args.Positional); {
	case n == 1:
		return c.args.Positional[0], nil
	case n == 0 && !required:
		return "", nil
	case n == 0:
		return "", validation("item_id", "%s takes the id of a text.", name)
	default:
		return "", validation("", "%s takes one text id, got %d.", name, n)
	}
}

var optionalProjectFlag = flagDef{
	Name:  "project",
	Kind:  kindString,
	Usage: "the project of the text (default $" + EnvProject + ")",
}

// projectOf is the project of a writing command: the flag, or the environment.
func projectOf(c *call) string {
	if id := c.args.str("project"); id != "" {
		return id
	}

	return os.Getenv(EnvProject)
}

var textListCmd = &spec{
	name:    "text:list",
	summary: "list the scripts, copy and captions of a project, or of every project",
	usage:   []string{"stuarty text:list [--project <id>] [--kind script|copy|caption] [--state active|trashed]"},
	example: "stuarty text:list --kind caption --json",
	flags: append([]flagDef{
		{Name: "project", Kind: kindString, Usage: "only the texts of this project"},
		{Name: "kind", Kind: kindString, Usage: "script, copy or caption"},
		{Name: "state", Kind: kindString, Usage: "active (default) or trashed"},
	}, pageFlags("50")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.TextList(ctx, service.TextListRequest{
			ProjectID: c.args.str("project"), TextKind: c.args.str("kind"), State: c.args.str("state"),
			Cursor: c.args.str("cursor"), Limit: c.args.integer("limit"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var textShowCmd = &spec{
	name:       "text:show",
	summary:    "show a text, or one of its revisions",
	usage:      []string{"stuarty text:show <item> --project <id> [--revision n]"},
	example:    "stuarty text:show pit_3c5v8n2w6d --project prj_7k2m9x4q1b",
	project:    true,
	positional: true,
	flags:      []flagDef{{Name: "revision", Kind: kindInt, Usage: "a revision number from 1; default the latest"}},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "text:show", true)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.TextGet(ctx, service.TextGetRequest{
			ProjectID: c.projectID, ItemID: item, Revision: c.args.integer("revision"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var textWriteCmd = &spec{
	name:    "text:write",
	summary: "make a text, or save a new revision of one, from a Markdown file",
	usage: []string{
		"stuarty text:write --project <id> --title <t> [--kind script|copy|caption] --text <file|->",
		"stuarty text:write <item> --project <id> --text <file|-> [--title <t>] [--base <revision>]",
	},
	example:    "stuarty text:write --project prj_7k2m9x4q1b --title Hook --kind script --text hook.md",
	project:    true,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "text", Kind: kindString, Usage: "the Markdown: a file, or - for standard input (required)"},
		{Name: "title", Kind: kindString, Usage: "the title: needed to make a text, a new title when saving one"},
		{Name: "kind", Kind: kindString, Usage: "making a text: script, copy (default) or caption"},
		{Name: "base", Kind: kindInt, Usage: "the revision you read: a newer one is refused"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "text:write", false)
		if err != nil {
			return nil, err
		}

		if !c.args.has("text") {
			return nil, validation("text", "--text is required.")
		}

		text, err := c.readText("text", c.args.str("text"))
		if err != nil {
			return nil, err
		}

		req := service.TextWriteRequest{
			ProjectID: c.projectID, ItemID: item, Title: c.args.str("title"), TextKind: c.args.str("kind"), Text: text,
			DryRun: c.args.boolean("dry-run"),
		}

		if c.args.has("base") {
			b := c.args.integer("base")
			req.BaseRevision = &b
		}

		res, err := c.svc.TextWrite(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var textRevisionsCmd = &spec{
	name:       "text:revisions",
	summary:    "list the revisions of a text, and compare two by paragraph",
	usage:      []string{"stuarty text:revisions <item> --project <id> [--from n [--until n]]"},
	example:    "stuarty text:revisions pit_3c5v8n2w6d --project prj_7k2m9x4q1b --from 1",
	project:    true,
	positional: true,
	flags: []flagDef{
		{Name: "before", Kind: kindInt, Usage: "start under this revision (the next_before of the last page)"},
		{Name: "limit", Kind: kindInt, Usage: "revisions per page, 1 to 100 (default 50)"},
		{Name: "from", Kind: kindInt, Usage: "compare this revision with --until"},
		{Name: "until", Kind: kindInt, Usage: "the revision to compare with (default the latest)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "text:revisions", true)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.TextRevisions(ctx, service.TextRevisionsRequest{
			ProjectID: c.projectID, ItemID: item, Before: c.args.integer("before"), Limit: c.args.integer("limit"),
			From: c.args.integer("from"), To: c.args.integer("until"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var textRevertCmd = &spec{
	name:       "text:revert",
	summary:    "bring an older revision of a text back as a new revision",
	usage:      []string{"stuarty text:revert <item> --project <id> --revision n"},
	example:    "stuarty text:revert pit_3c5v8n2w6d --project prj_7k2m9x4q1b --revision 2",
	project:    true,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags:      []flagDef{{Name: "revision", Kind: kindInt, Usage: "the revision whose text comes back (required)"}},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "text:revert", true)
		if err != nil {
			return nil, err
		}

		if !c.args.has("revision") {
			return nil, validation("revision", "--revision is required.")
		}

		res, err := c.svc.TextRevert(ctx, service.TextRevertRequest{
			ProjectID: c.projectID,
			ItemID:    item,
			Revision:  c.args.integer("revision"),
			DryRun:    c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var textDraftCmd = &spec{
	name:    "text:draft",
	summary: "ask a model for a draft of a text; the job shows its estimate and waits for approval",
	usage: []string{
		"stuarty text:draft <item> --project <id> --connection <id> --model <id> --instruction <text>" +
			" [--network <n>] [--max-tokens <n>] [--temperature <t>] [--dry-run]",
	},
	example: "stuarty text:draft pit_3c5v8n2w6d --project prj_7k2m9x4q1b --connection con_0123456789 --model a-model" +
		` --instruction "A hook in 20 seconds"`,
	project:    true,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection to the provider (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id, as provider:models lists it (required)"},
		{Name: "instruction", Kind: kindString, Usage: "what to write, 1 to 4000 characters (required)"},
		{Name: "network", Kind: kindString, Usage: "a caption: draft the variant of this network only"},
		{
			Name:  "max-tokens",
			Kind:  kindInt,
			Usage: "the most tokens the model may write; the estimate counts all of them",
		},
		{Name: "temperature", Kind: kindString, Usage: "0 to 2"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "text:draft", true)
		if err != nil {
			return nil, err
		}

		for _, f := range []string{"connection", "model", "instruction"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		req := service.TextDraftRequest{
			ProjectID:    c.projectID,
			ItemID:       item,
			ConnectionID: c.args.str("connection"),
			Model:        c.args.str("model"),
			Instruction: c.args.str(
				"instruction",
			),
			Network:   c.args.str("network"),
			MaxTokens: c.args.integer("max-tokens"),
			DryRun:    c.args.boolean("dry-run"),
		}

		if c.args.has("temperature") {
			t, perr := strconv.ParseFloat(c.args.str("temperature"), 64)
			if perr != nil {
				return nil, validation("temperature", "--temperature is a number from 0 to 2.")
			}

			req.Temperature = &t
		}

		job, err := c.svc.TextDraft(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}

var captionValidateCmd = &spec{
	name:    "caption:validate",
	summary: "measure a caption against the limits of each network",
	usage: []string{
		"stuarty caption:validate <item> --project <id> [--network <n>]",
		"stuarty caption:validate --body <file|->",
		"stuarty caption:validate --text <file|-> --network <n>",
	},
	example:    "stuarty caption:validate pit_3c5v8n2w6d --project prj_7k2m9x4q1b",
	positional: true,
	flags: []flagDef{
		optionalProjectFlag,
		{Name: "body", Kind: kindString, Usage: "a whole caption, saved or not: a file, or - for standard input"},
		{Name: "text", Kind: kindString, Usage: "one variant: a file, or - for standard input (needs --network)"},
		{Name: "network", Kind: kindString, Usage: "youtube, instagram, facebook, x or tiktok"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "caption:validate", false)
		if err != nil {
			return nil, err
		}

		req := service.CaptionValidateRequest{ProjectID: projectOf(c), ItemID: item, Network: c.args.str("network")}

		for flag, dest := range map[string]*string{"body": &req.Body, "text": &req.Text} {
			if !c.args.has(flag) {
				continue
			}

			if *dest, err = c.readText(flag, c.args.str(flag)); err != nil {
				return nil, err
			}
		}

		res, err := c.svc.CaptionValidate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var scriptEstimateCmd = &spec{
	name:    "script:estimate",
	summary: "estimate how long a script takes when read aloud",
	usage: []string{
		"stuarty script:estimate <item> --project <id> [--wpm n]",
		"stuarty script:estimate --text <file|-> [--wpm n]",
	},
	example:    "stuarty script:estimate pit_3c5v8n2w6d --project prj_7k2m9x4q1b --wpm 130",
	positional: true,
	flags: []flagDef{
		optionalProjectFlag,
		{Name: "text", Kind: kindString, Usage: "a script that is not saved: a file, or - for standard input"},
		{Name: "wpm", Kind: kindInt, Usage: "words per minute, 60 to 300 (default 150)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		item, err := onePositional(c, "script:estimate", false)
		if err != nil {
			return nil, err
		}

		req := service.ScriptEstimateRequest{
			ProjectID:      projectOf(c),
			ItemID:         item,
			WordsPerMinute: c.args.integer("wpm"),
		}

		if c.args.has("text") {
			if req.Text, err = c.readText("text", c.args.str("text")); err != nil {
				return nil, err
			}
		}

		res, err := c.svc.ScriptEstimate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
