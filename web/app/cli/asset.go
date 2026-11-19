package cli

// The asset library commands of SDD-011 section 14.5, and the library flags of media:import.

import (
	"context"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
)

// libraryFlags are the flags of media:import that say where the files came from and what they are for.
func libraryFlags() []flagDef {
	return []flagDef{
		{
			Name: "hash", Kind: kindString, Repeat: true,
			Usage: "the hash of a file of the library to use in the project, in place of a path",
		},
		{
			Name:  "origin-kind",
			Kind:  kindString,
			Usage: "imported (default), generated, fetched_by_agent or rendered_externally",
		},
		{
			Name:  "origin-source",
			Kind:  kindString,
			Usage: "a path, a URL or a tool name, as plain text (required for fetched_by_agent and rendered_externally)",
		},
		{Name: "provider", Kind: kindString, Usage: "the provider that generated the files (required for generated)"},
		{Name: "model", Kind: kindString, Usage: "the model that generated the files (required for generated)"},
		{Name: "prompt", Kind: kindString, Usage: "the prompt that generated the files"},
		{Name: "cost-micros", Kind: kindInt, Usage: "what the files cost, in millionths of a US dollar"},
		{
			Name: "rights-note", Kind: kindString,
			Usage: "your own record of the licence or the permission (required for fetched_by_agent)",
		},
		{Name: "tag", Kind: kindString, Repeat: true, Usage: "a tag for the files; repeat it or give a comma list"},
		{Name: "reusable", Kind: kindBool, Usage: "mark the files to be offered again"},
		{Name: "description", Kind: kindString, Usage: "what the files are, in your words"},
	}
}

// importBlocks reads the origin and the info of media:import; nil when no flag asks for them.
func importBlocks(p *parsed) (*asset.Origin, *service.AssetInfo) {
	var (
		origin *asset.Origin
		info   *service.AssetInfo
	)

	for _, f := range []string{
		"origin-kind", "origin-source", "provider", "model", "prompt", "cost-micros", "rights-note",
	} {
		if !p.has(f) {
			continue
		}

		o := asset.Origin{
			Kind: asset.OriginKind(p.str("origin-kind")), Source: p.str("origin-source"),
			Provider: p.str("provider"), Model: p.str("model"), Prompt: p.str("prompt"),
			RightsNote: p.str("rights-note"),
		}

		if p.has("cost-micros") {
			o.CostMicros = new(int64(p.integer("cost-micros")))
		}

		origin = &o

		break
	}

	if p.has("tag") || p.has("reusable") || p.has("description") {
		info = &service.AssetInfo{Tags: splitAll(p.all("tag")), Description: p.str("description")}

		if p.has("reusable") {
			info.Reusable = new(p.boolean("reusable"))
		}
	}

	return origin, info
}

var assetListCmd = &spec{
	name:    "asset:list",
	summary: "list the files of the library, with origin, tags and reuse",
	usage: []string{
		"stuarty asset:list",
	},
	example: "stuarty asset:list --tag logo --reusable --json",
	flags: append([]flagDef{
		{Name: "section", Kind: kindString, Usage: "visual (images and LUTs) or video (video and audio)"},
		{Name: "kind", Kind: kindString, Usage: "image, video, audio or lut"},
		{Name: "tag", Kind: kindString, Usage: "only the files with this tag"},
		{Name: "project", Kind: kindString, Usage: "only the files this project, or one of its timelines, uses"},
		{Name: "origin", Kind: kindString, Usage: "imported, generated, fetched_by_agent or rendered_externally"},
		{Name: "reusable", Kind: kindBool, Usage: "only the files marked reusable; --no-reusable keeps the others"},
		{Name: "text", Kind: kindString, Usage: "a full-text query over name, tags, description and origin"},
	}, pageFlags("50")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		req := service.AssetListRequest{
			Kind: c.args.str("kind"), Section: c.args.str("section"), Tag: c.args.str("tag"),
			Project: c.args.str("project"), Origin: c.args.str("origin"), Text: c.args.str("text"),
			Limit: c.args.integer("limit"), Cursor: c.args.str("cursor"),
		}

		if c.args.has("reusable") {
			req.Reusable = new(c.args.boolean("reusable"))
		}

		res, err := c.svc.AssetList(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

func assetHash(c *call, name string) (string, error) {
	if len(c.args.Positional) != 1 {
		return "", validation("", "%s takes one hash, got %d.", name, len(c.args.Positional))
	}

	return c.args.Positional[0], nil
}

var assetShowCmd = &spec{
	name:       "asset:show",
	summary:    "show a file of the library: its origin, its tags and the projects that use it",
	usage:      []string{"stuarty asset:show <hash>"},
	example:    "stuarty asset:show sha256:3b4c0a1d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b --json",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		hash, err := assetHash(c, "asset:show")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.AssetGet(ctx, service.AssetRef{Hash: hash})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var assetUsesCmd = &spec{
	name:       "asset:uses",
	summary:    "list the projects that use a file of the library",
	usage:      []string{"stuarty asset:uses <hash>"},
	example:    "stuarty asset:uses sha256:3b4c0a1d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b --json",
	positional: true,
	flags:      pageFlags("50"),
	run: func(ctx context.Context, c *call) (*output, error) {
		hash, err := assetHash(c, "asset:uses")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.AssetUses(ctx, service.AssetUsesRequest{Hash: hash, Page: page(c.args)})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var assetSetCmd = &spec{
	name:    "asset:set",
	summary: "set the description, the rights note and the reuse mark of a file of the library",
	usage: []string{
		"stuarty asset:set <hash>",
	},
	example:    "stuarty asset:set sha256:3b4c0a1d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b --reusable",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "description", Kind: kindString, Usage: "what the file is, in your words; an empty value clears it"},
		{
			Name: "rights-note", Kind: kindString,
			Usage: "your own record of the licence or the permission; the product does not check it",
		},
		{
			Name:  "reusable",
			Kind:  kindBool,
			Usage: "mark the file to be offered again; --no-reusable takes the mark off",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		hash, err := assetHash(c, "asset:set")
		if err != nil {
			return nil, err
		}

		req := service.AssetSetInfoRequest{Hash: hash}

		if c.args.has("description") {
			req.Description = new(c.args.str("description"))
		}

		if c.args.has("rights-note") {
			req.RightsNote = new(c.args.str("rights-note"))
		}

		if c.args.has("reusable") {
			req.Reusable = new(c.args.boolean("reusable"))
		}

		if req.Description == nil && req.RightsNote == nil && req.Reusable == nil {
			return nil, validation("", "asset:set needs --description, --rights-note or --reusable.")
		}

		res, err := c.svc.AssetSetInfo(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var assetTagCmd = &spec{
	name:       "asset:tag",
	summary:    "add and remove the tags of a file of the library",
	usage:      []string{"stuarty asset:tag <hash>"},
	example:    "stuarty asset:tag sha256:3b4c0a1d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b --add logo,brand",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "add", Kind: kindString, Repeat: true, Usage: "tags to add; repeat it or give a comma list"},
		{Name: "remove", Kind: kindString, Repeat: true, Usage: "tags to take off"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		hash, err := assetHash(c, "asset:tag")
		if err != nil {
			return nil, err
		}

		add, remove := splitAll(c.args.all("add")), splitAll(c.args.all("remove"))
		if len(add) == 0 && len(remove) == 0 {
			return nil, validation("", "asset:tag needs --add or --remove.")
		}

		res, err := c.svc.AssetTag(ctx, service.AssetTagRequest{Hash: hash, Add: add, Remove: remove})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
