package cli

// The commands of the library: memory notes, skills and prompts (SDD-018 section 14). Each handler parses,
// calls one Service method and formats; the rules are in core/library.

import (
	"context"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// libraryCommands are the commands of the library, after those of the editor in the short list.
func libraryCommands() []*spec {
	return []*spec{
		memoryList, memoryShow, memoryWrite, memoryRemove,
		skillList, skillShow, skillWrite, skillImport, skillLint, skillExport,
		promptList, promptShow, promptWrite, promptRender,
		librarySearch, libraryReindex,
	}
}

var tagFlag = flagDef{Name: "tag", Kind: kindString, Repeat: true, Usage: "a tag; repeat it for several"}

var allFlag = flagDef{
	Name: "all", Kind: kindBool, Default: "false",
	Usage: "also what is not reviewed (skills) or not kept (notes): an agent is not offered those",
}

var bodyFlags = []flagDef{
	{Name: "body", Kind: kindString, Usage: "the text of the entry"},
	{Name: "body-file", Kind: kindString, Usage: "read the text from a file, or - for standard input"},
}

// readBody reads the text of an entry from --body or --body-file.
func (c *call) readBody() (string, error) {
	if c.args.has("body") && c.args.has("body-file") {
		return "", validation("body", "give --body or --body-file, not both.")
	}

	if !c.args.has("body-file") {
		return c.args.str("body"), nil
	}

	return c.readText("body-file", c.args.str("body-file"))
}

// oneName is the single positional argument that names an entry.
func oneName(c *call, command string) (string, error) {
	if len(c.args.Positional) != 1 {
		return "", validation("", "%s takes one name, got %d.", command, len(c.args.Positional))
	}

	return c.args.Positional[0], nil
}

var memoryList = &spec{
	name:    "memory:list",
	summary: "list the memory notes",
	usage:   []string{"stuarty memory:list [--scope workspace|<project id>] [--tag t]... [--all]"},
	example: "stuarty memory:list --tag hooks --json",
	flags: []flagDef{
		{Name: "scope", Kind: kindString, Usage: "workspace or a project id"}, tagFlag, allFlag,
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.MemoryList(ctx, service.MemoryListRequest{
			Scope: c.args.str("scope"), Tags: c.args.all("tag"), IncludeUnreviewed: c.args.boolean("all"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var memoryShow = &spec{
	name:       "memory:show",
	summary:    "show a memory note",
	usage:      []string{"stuarty memory:show <name>"},
	example:    "stuarty memory:show viewers-skip-intros",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "memory:show")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.MemoryGet(ctx, service.LibraryRef{Name: name})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var memoryWrite = &spec{
	name:    "memory:write",
	summary: "write a memory note: one fact, with what was learned",
	usage: []string{
		"stuarty memory:write [<name>] --description <one line> [--body <text>|--body-file <file|->] [--scope s] [--tag t]...",
		"a note written here is pending until a person keeps it in the Memory section",
	},
	example:    `stuarty memory:write --description "Viewers skip long intros" --body "Cut the intro to two seconds."`,
	positional: true,
	write:      true,
	flags: append([]flagDef{
		{Name: "description", Kind: kindString, Usage: "the fact, in one line (required)"},
		{Name: "scope", Kind: kindString, Usage: "workspace (default) or a project id"},
		tagFlag,
	}, bodyFlags...),
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) > 1 {
			return nil, validation("", "memory:write takes at most one name, got %d.", len(c.args.Positional))
		}

		body, err := c.readBody()
		if err != nil {
			return nil, err
		}

		req := service.MemoryWriteRequest{
			Description: c.args.str("description"), Scope: c.args.str("scope"), Tags: c.args.all("tag"), Body: body,
		}

		if len(c.args.Positional) == 1 {
			req.Name = c.args.Positional[0]
		}

		res, err := c.svc.MemoryWrite(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var memoryRemove = &spec{
	name:       "memory:remove",
	summary:    "delete a memory note (a note a person kept stays)",
	usage:      []string{"stuarty memory:remove <name>"},
	example:    "stuarty memory:remove old-note",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "memory:remove")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.MemoryRemove(ctx, service.LibraryRef{Name: name})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillList = &spec{
	name:    "skill:list",
	summary: "list the skills that are reviewed",
	usage:   []string{"stuarty skill:list [--origin built-in|user|imported] [--all]"},
	example: "stuarty skill:list --all --json",
	flags:   []flagDef{{Name: "origin", Kind: kindString, Usage: "built-in, user or imported"}, allFlag},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.SkillList(ctx, service.SkillListRequest{
			Origin: c.args.str("origin"), IncludeUnreviewed: c.args.boolean("all"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillShow = &spec{
	name:       "skill:show",
	summary:    "show a skill: its text, its files and the findings of the lint",
	usage:      []string{"stuarty skill:show <name>"},
	example:    "stuarty skill:show hook-writer",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "skill:show")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.SkillGet(ctx, service.LibraryRef{Name: name})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillWrite = &spec{
	name:    "skill:write",
	summary: "write a skill; it stays unreviewed until a person reads it in the Skills section",
	usage: []string{
		"stuarty skill:write <name> --description <text> [--body <text>|--body-file <file|->] [--license l]",
		"                    [--file <path in the skill>=<local file>]...",
	},
	example:    "stuarty skill:write hook-writer --description \"Write the first three seconds.\" --body-file hook.md",
	positional: true,
	write:      true,
	flags: append([]flagDef{
		{Name: "description", Kind: kindString, Usage: "what the skill does and when to use it (required)"},
		{Name: "license", Kind: kindString, Usage: "the licence of the text"},
		{
			Name:   "file",
			Kind:   kindString,
			Repeat: true,
			Usage:  "another file of the folder: <path in the skill>=<local file>",
		},
	}, bodyFlags...),
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "skill:write")
		if err != nil {
			return nil, err
		}

		body, err := c.readBody()
		if err != nil {
			return nil, err
		}

		req := service.SkillWriteRequest{
			Name: name, Description: c.args.str("description"), License: c.args.str("license"), Body: body,
		}

		if c.args.has("file") {
			req.Files = []service.SkillFileInput{}

			for _, spec := range c.args.all("file") {
				dest, src, ok := strings.Cut(spec, "=")
				if !ok || dest == "" || src == "" {
					return nil, validation("file", "--file is <path in the skill>=<local file>.")
				}

				text, err := c.readText("file", src)
				if err != nil {
					return nil, err
				}

				req.Files = append(req.Files, service.SkillFileInput{Path: dest, Text: text})
			}
		}

		res, err := c.svc.SkillWrite(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillImport = &spec{
	name:    "skill:import",
	summary: "import a skill from a folder or an archive (.zip, .tar.gz) you already have; it starts unreviewed",
	usage:   []string{"stuarty skill:import <folder|archive> [--source <text>] [--replace]"},
	example: "stuarty skill:import ./downloads/hook-writer.zip --source \"a friend\"",
	// Stuarty opens no connection to fetch a skill: download it first.
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "source", Kind: kindString, Usage: "where it came from; the name of the folder or file if omitted"},
		{Name: "replace", Kind: kindBool, Default: "false", Usage: "replace a skill of the same name"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		path, err := oneName(c, "skill:import")
		if err != nil {
			return nil, err
		}

		abs, err := c.rt.abs(path)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.SkillImport(ctx, service.SkillImportRequest{
			Path: abs, Source: c.args.str("source"), Replace: c.args.boolean("replace"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillLint = &spec{
	name:       "skill:lint",
	summary:    "run the lint on a skill",
	usage:      []string{"stuarty skill:lint <name>"},
	example:    "stuarty skill:lint hook-writer --json",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "skill:lint")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.SkillLint(ctx, service.LibraryRef{Name: name})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var skillExport = &spec{
	name:    "skill:export",
	summary: "write skills as folders in the Agent Skills layout, to install them in your own agent",
	usage: []string{
		"stuarty skill:export [<name>...]",
		"with no name, every reviewed skill; the folders are written under exports/library/ of the workspace",
	},
	example:    "stuarty skill:export hook-writer",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.SkillExport(ctx, service.SkillExportRequest{Names: c.args.Positional})
		if err != nil {
			return nil, err
		}

		text := res.Text
		if res.Path != "" {
			text += ": " + res.Path
		}

		return &output{value: res, text: text}, nil
	},
}

var promptList = &spec{
	name:    "prompt:list",
	summary: "list the prompts",
	usage:   []string{"stuarty prompt:list [--tag t]..."},
	example: "stuarty prompt:list --json",
	flags:   []flagDef{tagFlag},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.PromptList(ctx, service.PromptListRequest{Tags: c.args.all("tag")})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var promptShow = &spec{
	name:       "prompt:show",
	summary:    "show a prompt and its variables",
	usage:      []string{"stuarty prompt:show <name>"},
	example:    "stuarty prompt:show video-intro",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "prompt:show")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PromptGet(ctx, service.LibraryRef{Name: name})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// keyValues reads repeated name=value flags into a map.
func keyValues(c *call, flag string) (map[string]string, error) {
	var out map[string]string

	for _, kv := range c.args.all(flag) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, validation(flag, "--%s is <variable>=<value>.", flag)
		}

		if out == nil {
			out = map[string]string{}
		}

		out[k] = v
	}

	return out, nil
}

var promptWrite = &spec{
	name:    "prompt:write",
	summary: "write a prompt: a reusable text with {{variables}}",
	usage: []string{
		"stuarty prompt:write <name> --description <text> [--body <text>|--body-file <file|->] [--default <variable>=<value>]... [--tag t]...",
	},
	example:    `stuarty prompt:write video-intro --description "An intro" --body "Write an intro about {{topic}}."`,
	positional: true,
	write:      true,
	flags: append([]flagDef{
		{Name: "description", Kind: kindString, Usage: "what the prompt is for (required)"},
		{Name: "default", Kind: kindString, Repeat: true, Usage: "a default value: <variable>=<value>"},
		tagFlag,
	}, bodyFlags...),
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "prompt:write")
		if err != nil {
			return nil, err
		}

		body, err := c.readBody()
		if err != nil {
			return nil, err
		}

		defaults, err := keyValues(c, "default")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PromptWrite(ctx, service.PromptWriteRequest{
			Name: name, Description: c.args.str("description"), Tags: c.args.all("tag"), Defaults: defaults, Body: body,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var promptRender = &spec{
	name:       "prompt:render",
	summary:    "replace the variables of a prompt and print the text; nothing is sent anywhere",
	usage:      []string{"stuarty prompt:render <name> [--value <variable>=<value>]..."},
	example:    "stuarty prompt:render video-intro --value topic=cats",
	positional: true,
	flags:      []flagDef{{Name: "value", Kind: kindString, Repeat: true, Usage: "a value: <variable>=<value>"}},
	run: func(ctx context.Context, c *call) (*output, error) {
		name, err := oneName(c, "prompt:render")
		if err != nil {
			return nil, err
		}

		values, err := keyValues(c, "value")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PromptRender(ctx, service.PromptRenderRequest{Name: name, Values: values})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var librarySearch = &spec{
	name:    "library:search",
	summary: "search memory, skills and prompts with one query",
	usage: []string{
		"stuarty library:search <word>... [--kind memory|skill|prompt]... [--scope s] [--tag t]... [--origin o] [--all]",
		"                       [--limit n] [--budget characters]",
	},
	example:    "stuarty library:search intro --kind prompt --json",
	positional: true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Repeat: true, Usage: "memory, skill or prompt; repeat it for several"},
		{Name: "scope", Kind: kindString, Usage: "memory: workspace or a project id"},
		tagFlag,
		{Name: "origin", Kind: kindString, Usage: "skills: built-in, user or imported"},
		allFlag,
		{Name: "limit", Kind: kindInt, Default: "20", Usage: "results, at most 50"},
		{Name: "budget", Kind: kindInt, Default: "4000", Usage: "characters of the answer, at most 16000"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) == 0 {
			return nil, validation("", "library:search needs at least one word.")
		}

		res, err := c.svc.LibrarySearch(ctx, service.LibrarySearchRequest{
			Query: strings.Join(c.args.Positional, " "), Kinds: c.args.all("kind"), Scope: c.args.str("scope"),
			Tags: c.args.all("tag"), Origin: c.args.str("origin"), IncludeUnreviewed: c.args.boolean("all"),
			Limit: c.args.integer("limit"), Budget: c.args.integer("budget"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var libraryReindex = &spec{
	name:    "library:reindex",
	summary: "rebuild the index of the library from its files",
	usage:   []string{"stuarty library:reindex"},
	example: "stuarty library:reindex",
	write:   true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.LibraryReindex(ctx, service.LibraryReindexRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
