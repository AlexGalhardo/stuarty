package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/AlexGalhardo/stuarty/service"
)

var templateList = &spec{
	name:    "template:list",
	summary: "list the templates of the library",
	usage:   []string{"stuarty template:list [--cursor c] [--limit n]"},
	example: "stuarty template:list --json",
	flags:   pageFlags("20"),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.TemplateList(ctx, service.TemplateListRequest{Page: page(c.args)})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var templateSave = &spec{
	name:    "template:save",
	summary: "make a template from a project",
	usage:   []string{"stuarty template:save --project <id> --name <name> [--description d]"},
	example: `stuarty template:save --project prj_7k2m9x4q1b --name "Spring sale"`,
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "template name (required)"},
		{Name: "description", Kind: kindString, Usage: "what the template is for"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("name") {
			return nil, validation("name", "--name is required.")
		}

		res, err := c.svc.TemplateSave(ctx, service.TemplateSaveRequest{
			ProjectID: c.projectID, Name: c.args.str("name"), Description: c.args.str("description"),
			DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var templateNew = &spec{
	name:    "template:new",
	summary: "create a project from a template",
	usage: []string{
		"stuarty template:new <template id> --name <name> [--fill <file|->] [--id prj_...]",
	},
	example:    `stuarty template:new tpl_5n8b2v6c1x --name "Spring sale" --fill fill.json`,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "project name (required)"},
		{Name: "fill", Kind: kindString, Usage: "a JSON object of slot name to value: a file, or - for standard input"},
		{
			Name: "id", Kind: kindString,
			Usage: "identifier for the project (omit it in network mode: the server chooses)",
		},
		{
			Name: "brand-color", Kind: kindString, Repeat: true,
			Usage: "a colour of the template that takes its value from the brand kit",
		},
		{
			Name: "brand-font", Kind: kindString, Repeat: true,
			Usage: "a family of the brand kit to add to the project's fonts",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "template:new takes one template id, got %d.", len(c.args.Positional))
		}

		if !c.args.has("name") {
			return nil, validation("name", "--name is required.")
		}

		req := service.TemplateInstantiateRequest{
			TemplateID: c.args.Positional[0], Name: c.args.str("name"), ID: c.args.str("id"),
			DryRun:      c.args.boolean("dry-run"),
			BrandColors: splitAll(c.args.all("brand-color")), BrandFonts: splitAll(c.args.all("brand-font")),
		}

		if c.args.has("fill") {
			fill, err := c.readFill(c.args.str("fill"))
			if err != nil {
				return nil, err
			}

			req.Fill = fill
		}

		res, err := c.svc.TemplateInstantiate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var templateExport = &spec{
	name:       "template:export",
	summary:    "write a template to a file the workspace keeps",
	usage:      []string{"stuarty template:export <template id> [--out f] [--force]"},
	example:    "stuarty template:export tpl_5n8b2v6c1x --out ~/spring-sale.stuarty-template.zip",
	positional: true,
	flags:      outFlags(),
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "template:export takes one template id, got %d.", len(c.args.Positional))
		}

		dest, err := c.outPath()
		if err != nil {
			return nil, err
		}

		res, err := c.svc.TemplateExport(ctx, service.TemplateRef{TemplateID: c.args.Positional[0]})
		if err != nil {
			return nil, err
		}

		if dest != "" {
			return c.saveResultFile(ctx, res, dest)
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var templateImport = &spec{
	name:       "template:import",
	summary:    "add a template file to the library",
	usage:      []string{"stuarty template:import <path>"},
	example:    "stuarty template:import spring-sale.stuarty-template.zip",
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "template:import takes one path, got %d.", len(c.args.Positional))
		}

		path, err := c.rt.abs(c.args.Positional[0])
		if err != nil {
			return nil, err
		}

		req := service.TemplateImportRequest{Path: path, DryRun: c.args.boolean("dry-run")}

		res, err := c.svc.TemplateImport(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// readFill reads the slot values from a file of the current directory or from
// standard input.
func (c *call) readFill(src string) (map[string]service.SlotValue, error) {
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
		return nil, validation("fill", "cannot read the fill %s: %s.", src, readReason(err))
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var fill map[string]service.SlotValue
	if err := dec.Decode(&fill); err != nil || fill == nil {
		problem := "it is null"
		if err != nil {
			problem = jsonProblem(err)
		}

		return nil, validation("fill",
			"the fill is not valid: %s. It must be one JSON object of slot name to value, "+
				`like {"headline": {"text": "Spring sale"}}; a slot takes text, words, or asset_id with source_in.`,
			problem)
	}

	return fill, nil
}
