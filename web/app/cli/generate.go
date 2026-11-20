package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

var imageGenerateCmd = &spec{
	name:    "image:generate",
	summary: "generate images with the provider of a connection; the job shows its estimate, waits for approval and stores the images as assets",
	usage: []string{
		"stuarty image:generate --project <id> --connection <id> --model <id> [--prompt <text> | --prompt-file <file>]" +
			" [--size <width>x<height>] [--count <n>] [--dry-run]",
	},
	example: "stuarty image:generate --project prj_7k2m9x4q1b --connection con_0123456789 --model a-model --prompt \"A red fox\" --count 2",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection to the provider (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id, as provider:models lists it (required)"},
		{
			Name:  "prompt",
			Kind:  kindString,
			Usage: "what to draw; without it and without --prompt-file the prompt is read from standard input",
		},
		{Name: "prompt-file", Kind: kindString, Usage: "a file that holds the prompt"},
		{Name: "size", Kind: kindString, Usage: "width and height in pixels joined by x (default 1024x1024)"},
		{Name: "count", Kind: kindInt, Usage: "how many images, 1 to 10 (default 1); the estimate counts all of them"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"connection", "model"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		prompt, err := c.readInput("prompt", "prompt-file", provider.MaxImagePrompt)
		if err != nil {
			return nil, err
		}

		job, err := c.svc.ImageGenerate(ctx, service.ImageGenerateRequest{
			ProjectID: c.projectID, ConnectionID: c.args.str("connection"), DryRun: c.args.boolean("dry-run"),
			Model: c.args.str("model"), Prompt: prompt, Size: c.args.str("size"), Count: c.args.integer("count"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}

var voiceGenerateCmd = &spec{
	name:    "voice:generate",
	summary: "generate speech with the provider of a connection; the job shows its estimate, waits for approval and stores the audio as an asset",
	usage: []string{
		"stuarty voice:generate --project <id> --connection <id> --model <id> --voice <id> [--text <text> | --text-file <file>]" +
			" [--language <tag>] [--format mp3|wav|flac|opus|aac] [--dry-run]",
	},
	example: "stuarty voice:generate --project prj_7k2m9x4q1b --connection con_0123456789 --model a-model --voice alloy --text \"Welcome\"",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection to the provider (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id, as provider:models lists it (required)"},
		{Name: "voice", Kind: kindString, Usage: "the voice id, as voice:list names it (required)"},
		{
			Name:  "text",
			Kind:  kindString,
			Usage: "what to read out; without it and without --text-file the text is read from standard input",
		},
		{Name: "text-file", Kind: kindString, Usage: "a file that holds the text"},
		{Name: "language", Kind: kindString, Usage: "a language tag such as pt-BR"},
		{Name: "format", Kind: kindString, Usage: "mp3 (default), wav, flac, opus or aac"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"connection", "model", "voice"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		text, err := c.readInput("text", "text-file", provider.DefaultVoiceChars)
		if err != nil {
			return nil, err
		}

		job, err := c.svc.VoiceGenerate(ctx, service.VoiceGenerateRequest{
			ProjectID: c.projectID, ConnectionID: c.args.str("connection"), DryRun: c.args.boolean("dry-run"),
			Model: c.args.str("model"), Text: text, Voice: c.args.str("voice"),
			Language: c.args.str("language"), Format: c.args.str("format"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}

var voiceListCmd = &spec{
	name:       "voice:list",
	summary:    "list the voices of the provider of a connection",
	usage:      []string{"stuarty voice:list <connection id>"},
	example:    "stuarty voice:list con_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneID(c, "voice:list")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.VoiceList(ctx, service.VoiceListRequest{ConnectionID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: voiceListText(res)}, nil
	},
}

func voiceListText(r *service.VoiceListResult) string {
	lines := []string{fmt.Sprintf("%d voice(s) of %s through %s", len(r.Voices), r.Provider, r.ConnectionID)}

	for i := range r.Voices {
		v := &r.Voices[i]

		// The name is text the provider wrote: delimited, never bare.
		name := ""
		if v.Name != "" && v.Name != v.ID {
			name = "  " + untrusted.Inline(untrusted.SourceNetworkText, nil, v.Name, 80)
		}

		line := v.ID + name
		if v.Language != "" {
			line += "  " + v.Language
		}

		lines = append(lines, line+"  ("+v.Source+")")
	}

	return strings.Join(lines, "\n")
}

// readInput reads the text of a generation from its flag, from a file, or from standard input, to the most
// characters the request may have (four bytes each, and one more to see that it is over).
func (c *call) readInput(flag, fileFlag string, chars int) (string, error) {
	switch {
	case c.args.has(flag):
		return c.args.str(flag), nil
	case c.args.has(fileFlag):
		raw, err := os.ReadFile(c.args.str(fileFlag))
		if err != nil {
			return "", validation(fileFlag, "cannot read the %s: %s.", flag, readReason(err))
		}

		return string(raw), nil
	}

	raw, err := io.ReadAll(io.LimitReader(c.rt.Stdin, int64(chars)*4+1))
	if err != nil {
		return "", validation(flag, "cannot read the %s: %s.", flag, readReason(err))
	}

	return strings.TrimRight(string(raw), "\r\n"), nil
}
