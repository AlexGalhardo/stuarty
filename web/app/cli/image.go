package cli

// The image documents of SDD-021: the CLI face of the seven image operations of the Service. Each handler parses,
// calls one Service method and formats; no editing rule lives here (ADR-0007).

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/service"
)

// imageID takes the image id, the first positional argument, off the arguments.
func imageID(c *call) (string, error) {
	if len(c.args.Positional) == 0 {
		return "", validation("image", "give the image id, such as img_7k2m9x4q1b.")
	}

	id := c.args.Positional[0]
	c.args.Positional = c.args.Positional[1:]

	return id, nil
}

var imageCreate = &spec{
	name:    "image:create",
	summary: "create an image document, blank or from a template",
	usage:   []string{"stuarty image:create --preset p"},
	example: `stuarty image:create --preset video_thumbnail --name "Episode 12" --background "#101820"`,
	write:   true,
	flags: []flagDef{
		{
			Name:  "preset",
			Kind:  kindString,
			Usage: "video_thumbnail (1280x720), square_post (1080x1080) or vertical_cover (1080x1920)",
		},
		{Name: "width", Kind: kindInt, Usage: "pixels, 1 to 4096, with --height"},
		{Name: "height", Kind: kindInt, Usage: "pixels, 1 to 4096, with --width"},
		{Name: "name", Kind: kindString, Usage: "name of the image"},
		{Name: "background", Kind: kindString, Usage: "a colour or transparent (default #FFFFFF)"},
		{
			Name:    "template",
			Kind:    kindBool,
			Default: "false",
			Usage:   "make a template whose text and image layers may be slots",
		},
		{Name: "from-template", Kind: kindString, Usage: "make the image from this template"},
		{
			Name:  "values",
			Kind:  kindString,
			Usage: `a JSON object of slot name to {"text": "..."} or {"asset": "sha256:..."}: a file, or - for standard input`,
		},
		{Name: "id", Kind: kindString, Usage: "identifier for the image (omit it in network mode: the server chooses)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		req := service.ImageCreateRequest{
			Name:   c.args.str("name"),
			Preset: c.args.str("preset"),
			Width:  c.args.integer("width"),
			Height: c.args.integer(
				"height",
			),
			Background: c.args.str("background"),
			Template:   c.args.boolean("template"),
			TemplateID: c.args.str("from-template"),
			ID:         c.args.str("id"),
		}

		if c.args.has("values") {
			raw, err := c.readOps(c.args.str("values"))
			if err != nil {
				return nil, err
			}

			if err := json.Unmarshal(raw, &req.Values); err != nil {
				return nil, validation("values", "the values are not a JSON object of slot values: %s.", jsonProblem(err))
			}
		}

		res, err := c.svc.ImageCreate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageList = &spec{
	name:    "image:list",
	summary: "list the image documents and templates",
	usage:   []string{"stuarty image:list"},
	example: "stuarty image:list --templates only --json",
	flags: append(
		[]flagDef{{Name: "templates", Kind: kindString, Usage: "only for the templates, none for the others"}},
		pageFlags("20")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ImageList(
			ctx,
			service.ImageListRequest{Templates: c.args.str("templates"), Page: page(c.args)},
		)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageShow = &spec{
	name:       "image:show",
	summary:    "describe an image document: its layers, its findings and its revision",
	usage:      []string{"stuarty image:show <id>"},
	example:    "stuarty image:show img_7k2m9x4q1b",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageEdit = &spec{
	name:       "image:edit",
	summary:    "apply a batch of image operations (stuarty command:help --domain image lists them)",
	usage:      []string{"stuarty image:edit <id> <op> k=v..."},
	example:    `stuarty image:edit img_7k2m9x4q1b layer.add_text text="HOW IT WORKS" slot=title`,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags:      edit.flags,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		req, err := editRequest(c)
		if err != nil {
			return nil, err
		}

		if !c.args.has("base") && !req.baseGiven {
			cur, gerr := c.svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: id})
			if gerr != nil {
				return nil, gerr
			}

			req.BaseRevision = cur.Revision
		}

		res, err := c.svc.ImageEdit(ctx, service.ImageEditRequest{
			ImageID: id, BaseRevision: req.BaseRevision, Operations: req.Operations,
			IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.ImageEditText(res)}, nil
	},
}

func imageStepCommand(name, summary string, redo bool) *spec {
	return &spec{
		name:       name,
		summary:    summary,
		usage:      []string{"stuarty " + name + " <id>"},
		example:    "stuarty " + name + " img_7k2m9x4q1b",
		positional: true,
		globals:    []string{"dry-run"},
		write:      true,
		run: func(ctx context.Context, c *call) (*output, error) {
			id, err := imageID(c)
			if err != nil {
				return nil, err
			}

			req := service.ImageStepRequest{ImageID: id, DryRun: c.args.boolean("dry-run")}

			var res *service.ImageStepResult
			if redo {
				res, err = c.svc.ImageRedo(ctx, req)
			} else {
				res, err = c.svc.ImageUndo(ctx, req)
			}

			if err != nil {
				return nil, err
			}

			return &output{value: res, text: service.ImageStepText(res)}, nil
		},
	}
}

var (
	imageUndo = imageStepCommand("image:undo", "undo the last step of the caller on an image", false)
	imageRedo = imageStepCommand("image:redo", "redo the last undone step of the caller on an image", true)
)

var imageExport = &spec{
	name:       "image:export",
	summary:    "export an image document to a PNG, JPEG or WebP file",
	usage:      []string{"stuarty image:export <id> [--format f]"},
	example:    "stuarty image:export img_7k2m9x4q1b --format jpeg --quality 85",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "format", Kind: kindString, Default: "png", Usage: "png, jpeg or webp"},
		{Name: "quality", Kind: kindInt, Usage: "1 to 100, for jpeg and lossy webp"},
		{Name: "preset", Kind: kindString, Usage: "resample to video_thumbnail, square_post or vertical_cover"},
		{Name: "width", Kind: kindInt, Usage: "resample to this width (0 keeps the aspect)"},
		{Name: "height", Kind: kindInt, Usage: "resample to this height (0 keeps the aspect)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageExport(ctx, service.ImageExportRequest{
			ImageID: id, Format: c.args.str("format"), Quality: c.args.integer("quality"), Preset: c.args.str("preset"),
			Width: c.args.integer("width"), Height: c.args.integer("height"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.ImageExportText(res)}, nil
	},
}

var imageImport = &spec{
	name:    "image:import",
	summary: "bring an image file into a new image document, or into an existing one as a layer",
	usage: []string{
		"stuarty image:import --path <file>",
		"stuarty image:import --hash <sha256:...> [--id <image>] [--name n] [--preset p | --width w --height h]",
	},
	example: `stuarty image:import --path /home/ana/photos/face.jpg --preset video_thumbnail`,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "path", Kind: kindString, Usage: "an absolute path to an image file on the machine of the server"},
		{Name: "hash", Kind: kindString, Usage: "an image already in the workspace (asset:list shows its hash)"},
		{Name: "id", Kind: kindString, Usage: "put the file in this image as a layer (default: a new image)"},
		{Name: "name", Kind: kindString, Usage: "the name of the new image, or of the layer with --id"},
		{
			Name:  "preset",
			Kind:  kindString,
			Usage: "the canvas of a new image: video_thumbnail, square_post or vertical_cover",
		},
		{
			Name:  "width",
			Kind:  kindInt,
			Usage: "the canvas of a new image, with --height (default: the size of the file, up to 4096)",
		},
		{Name: "height", Kind: kindInt, Usage: "the canvas of a new image, with --width"},
		{
			Name:  "wait-ms",
			Kind:  kindInt,
			Usage: "how long to wait for the file to be ingested, 0 to 25000 (default 10000)",
		},
		{
			Name:  "base",
			Kind:  kindInt,
			Usage: "with --id: the revision the layer was planned against (default: the current one)",
		},
		{Name: "key", Kind: kindString, Usage: "with --id: idempotency key"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		req := service.ImageImportRequest{
			Path: c.args.str("path"), Hash: c.args.str("hash"), ImageID: c.args.str("id"), Name: c.args.str("name"),
			Preset: c.args.str("preset"), Width: c.args.integer("width"), Height: c.args.integer("height"),
			WaitMS: c.args.integer("wait-ms"), IdempotencyKey: c.args.str("key"), DryRun: c.args.boolean("dry-run"),
		}

		if req.ImageID != "" {
			req.BaseRevision = int64(c.args.integer("base"))

			if !c.args.has("base") {
				cur, err := c.svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: req.ImageID})
				if err != nil {
					return nil, err
				}

				req.BaseRevision = cur.Revision
			}
		} else if c.args.has("base") {
			req.BaseRevision = int64(c.args.integer("base"))
		}

		res, err := c.svc.ImageImport(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// Revert, the diff and the checkpoints of an image (epic 50) are those of a timeline (revert.go), with the id of
// the image as the first argument, as image:undo has it.

var imageRevert = &spec{
	name:       "image:revert",
	summary:    "go back to the document of an earlier revision of an image, as a new step that image:undo takes back",
	usage:      []string{"stuarty image:revert <id> (--revision n | --checkpoint name) [--base n] [--dry-run]"},
	example:    `stuarty image:revert img_7k2m9x4q1b --checkpoint "before the agent" --dry-run`,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "revision", Kind: kindInt, Usage: "the revision to go back to"},
		{Name: "checkpoint", Kind: kindString, Usage: "the name of a checkpoint to go back to"},
		{
			Name:  "base",
			Kind:  kindInt,
			Usage: "the revision you read (default the current one); an older one conflicts with other actors' changes",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageRevert(ctx, service.ImageRevertRequest{
			ImageID: id, Revision: int64(c.args.integer("revision")), Checkpoint: c.args.str("checkpoint"),
			BaseRevision: int64(c.args.integer("base")), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageDiff = &spec{
	name:       "image:diff",
	summary:    "list what changed between two revisions of an image, most significant first",
	usage:      []string{"stuarty image:diff <id> (--from n | --from-checkpoint name) [--until n]"},
	example:    "stuarty image:diff img_7k2m9x4q1b --from 12",
	positional: true,
	flags: []flagDef{
		{Name: "from", Kind: kindInt, Usage: "the revision the diff starts from"},
		{Name: "from-checkpoint", Kind: kindString, Usage: "the name of a checkpoint the diff starts from"},
		{Name: "until", Kind: kindInt, Usage: "the revision the diff ends at (default the current one)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageDiff(ctx, service.ImageDiffRequest{
			ImageID: id, From: int64(c.args.integer("from")), FromCheckpoint: c.args.str("from-checkpoint"),
			Until: int64(c.args.integer("until")),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageCheckpointCreate = &spec{
	name:       "image:checkpoint:create",
	summary:    "name a revision of an image so that it can be gone back to",
	usage:      []string{"stuarty image:checkpoint:create <id> --name <name> [--note t] [--revision n]"},
	example:    `stuarty image:checkpoint:create img_7k2m9x4q1b --name "before the agent"`,
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "1 to 60 characters, no slash"},
		{Name: "note", Kind: kindString, Usage: "up to 300 characters"},
		{Name: "revision", Kind: kindInt, Usage: "the revision to name (default the current one)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageCheckpointCreate(ctx, service.ImageCheckpointCreateRequest{
			ImageID:  id,
			Name:     c.args.str("name"),
			Note:     c.args.str("note"),
			Revision: int64(c.args.integer("revision")),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("created", res)}, nil
	},
}

var imageCheckpointList = &spec{
	name:       "image:checkpoint:list",
	summary:    "list the checkpoints of an image, newest revision first",
	usage:      []string{"stuarty image:checkpoint:list <id>"},
	example:    "stuarty image:checkpoint:list img_7k2m9x4q1b --json",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageCheckpointList(ctx, service.ImageCheckpointListRequest{ImageID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var imageCheckpointRename = &spec{
	name:       "image:checkpoint:rename",
	summary:    "rename a checkpoint of an image, and change its note",
	usage:      []string{"stuarty image:checkpoint:rename <id> --name <name> --new-name <name> [--note t]"},
	example:    `stuarty image:checkpoint:rename img_7k2m9x4q1b --name draft --new-name "first cut"`,
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "the checkpoint to rename"},
		{Name: "new-name", Kind: kindString, Usage: "the new name"},
		{Name: "note", Kind: kindString, Usage: "replaces the note when given"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		req := service.ImageCheckpointRenameRequest{
			ImageID: id,
			Name:    c.args.str("name"),
			NewName: c.args.str("new-name"),
		}

		if c.args.has("note") {
			note := c.args.str("note")
			req.Note = &note
		}

		res, err := c.svc.ImageCheckpointRename(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("renamed", res)}, nil
	},
}

var imageCheckpointRemove = &spec{
	name:       "image:checkpoint:remove",
	summary:    "remove a checkpoint of an image; no revision is removed",
	usage:      []string{"stuarty image:checkpoint:remove <id> --name <name>"},
	example:    "stuarty image:checkpoint:remove img_7k2m9x4q1b --name draft",
	positional: true,
	write:      true,
	flags:      []flagDef{{Name: "name", Kind: kindString, Usage: "the checkpoint to remove"}},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := imageID(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ImageCheckpointRemove(
			ctx,
			service.ImageCheckpointRemoveRequest{ImageID: id, Name: c.args.str("name")},
		)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("removed", res)}, nil
	},
}
