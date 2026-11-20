package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service"
)

var imageModelEditCmd = &spec{
	name: "image:model-edit",
	summary: "remove the background of a layer, or generate an image into a document, with the provider of a connection; " +
		"the job says that the picture is sent, shows its estimate, waits for approval and puts the result in the document as one undoable step",
	usage: []string{
		"stuarty image:model-edit --project <id> --image <id> --operation remove_background|generate --connection <id> --model <id>" +
			" [--layer <id>] [--prompt <text> | --prompt-file <file>] [--size <width>x<height>] [--dry-run]",
	},
	example: "stuarty image:model-edit --project prj_7k2m9x4q1b --image img_7k2m9x4q1b --operation remove_background --connection con_0123456789 --model a-model --layer lyr_7k2m9x4q1b",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "image", Kind: kindString, Usage: "the image document (required)"},
		{Name: "operation", Kind: kindString, Usage: "remove_background or generate (required)"},
		{Name: "connection", Kind: kindString, Usage: "the connection to the provider (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id, as provider:models lists it (required)"},
		{
			Name:  "layer",
			Kind:  kindString,
			Usage: "remove_background: the image layer (required); generate: a layer whose box the picture must cover",
		},
		{
			Name:  "prompt",
			Kind:  kindString,
			Usage: "generate only: what to draw; without it and without --prompt-file a generation reads it from standard input",
		},
		{Name: "prompt-file", Kind: kindString, Usage: "generate only: a file that holds the prompt"},
		{Name: "size", Kind: kindString, Usage: "generate only: width and height in pixels joined by x"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"image", "operation", "connection", "model"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		var prompt string

		// A removal takes no prompt, so standard input is read only for a generation, or when a prompt was named.
		if c.args.str("operation") == provider.OperationGenerate || c.args.has("prompt") || c.args.has("prompt-file") {
			var err error

			if prompt, err = c.readInput("prompt", "prompt-file", provider.MaxImagePrompt); err != nil {
				return nil, err
			}
		}

		job, err := c.svc.ImageModelEdit(ctx, service.ImageModelEditRequest{
			ProjectID: c.projectID, DryRun: c.args.boolean("dry-run"),
			ConnectionID: c.args.str(
				"connection",
			),
			ImageID:   c.args.str("image"),
			Operation: c.args.str("operation"),
			Model:     c.args.str("model"),
			LayerID:   c.args.str("layer"),
			Prompt:    prompt,
			Size:      c.args.str("size"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}
