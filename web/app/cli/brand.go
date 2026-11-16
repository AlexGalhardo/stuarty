package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/AlexGalhardo/vukem-video/service"
)

var fontImportCmd = &spec{
	name:       "font:import",
	summary:    "add a TrueType or OpenType font file to the font store",
	usage:      []string{"vukem-video font:import <path>"},
	example:    "vukem-video font:import Brand-Regular.ttf",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "font:import takes one path, got %d.", len(c.args.Positional))
		}

		path, err := c.rt.abs(c.args.Positional[0])
		if err != nil {
			return nil, err
		}

		res, err := c.svc.FontImport(ctx, service.FontImportRequest{Path: path})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var brandShowCmd = &spec{
	name:    "brand:show",
	summary: "show the brand kit: logo, colours and fonts",
	usage:   []string{"vukem-video brand:show"},
	example: "vukem-video brand:show --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.BrandKitGet(ctx, service.BrandKitGetRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var brandSetCmd = &spec{
	name:    "brand:set",
	summary: "replace the brand kit",
	usage:   []string{"vukem-video brand:set --kit <file|->"},
	example: "vukem-video brand:set --kit brand.json",
	write:   true,
	flags: []flagDef{
		{
			Name:  "kit",
			Kind:  kindString,
			Usage: "a JSON object with logo, colors and fonts, all optional: a file, or - for standard input (required)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("kit") {
			return nil, validation("kit", "--kit is required.")
		}

		req, err := c.readKit(c.args.str("kit"))
		if err != nil {
			return nil, err
		}

		res, err := c.svc.BrandKitSet(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// readKit reads the brand kit from a file of the current directory or from
// standard input.
func (c *call) readKit(src string) (service.BrandKitSetRequest, error) {
	var (
		raw []byte
		err error
		req service.BrandKitSetRequest
	)

	if src == "-" {
		raw, err = io.ReadAll(c.rt.Stdin)
	} else {
		raw, err = os.ReadFile(src)
	}

	if err != nil {
		return req, validation("kit", "cannot read the kit %s: %s.", src, readReason(err))
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		return req, validation("kit",
			"the kit is not valid: %s. It must be one JSON object with logo, colors and fonts, "+
				`like {"colors": {"accent": "#FF0000"}, "fonts": {"Brand": {"files": [{"weight": 400, "hash": "sha256:<64 hex digits>"}]}}}.`,
			jsonProblem(err))
	}

	return req, nil
}

// splitAll joins repeated flag values, each of which may be a comma list.
func splitAll(values []string) []string {
	var out []string

	for _, v := range values {
		out = append(out, splitList(v)...)
	}

	return out
}
