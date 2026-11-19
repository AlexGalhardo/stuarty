package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// providerCommands are the commands of the AI provider layer.
func providerCommands() []*spec {
	return []*spec{providerListCmd, providerModelsCmd, providerPriceCmd, textGenerateCmd}
}

var providerListCmd = &spec{
	name:    "provider:list",
	summary: "list the AI providers, the connections of each and the price table",
	usage:   []string{"stuarty provider:list"},
	example: "stuarty provider:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProviderList(ctx, service.ProviderListRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: providerListText(res)}, nil
	},
}

func providerListText(r *service.ProviderListResult) string {
	lines := []string{fmt.Sprintf("%d provider(s)", len(r.Providers))}

	for i := range r.Providers {
		p := &r.Providers[i]

		conns := "no connection (connect one with connection:add --kind " + p.ID + ")"
		if len(p.Connections) > 0 {
			conns = "connections: " + strings.Join(p.Connections, ", ")
		}

		lines = append(lines, fmt.Sprintf("%s  %s  %s  %s", p.ID, p.Name, strings.Join(p.Capabilities, ","), conns))
	}

	for i := range r.Prices {
		e := &r.Prices[i]
		lines = append(lines, fmt.Sprintf("price %s %s: in %s, out %s per million tokens, %s",
			e.Provider, e.Model, micros(e.InputMicrosPerMTok), micros(e.OutputMicrosPerMTok), e.Currency))
	}

	return strings.Join(lines, "\n")
}

// micros writes an amount in millionths of the currency unit as a decimal.
func micros(v int64) string { return fmt.Sprintf("%d.%06d", v/1_000_000, v%1_000_000) }

var providerModelsCmd = &spec{
	name:       "provider:models",
	summary:    "list the models a provider offers through a connection, read from the provider now, with their price",
	usage:      []string{"stuarty provider:models <connection id> [--refresh]"},
	example:    "stuarty provider:models con_0123456789",
	positional: true,
	flags: []flagDef{
		{Name: "refresh", Kind: kindBool, Default: "false", Usage: "read the list from the provider again"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneID(c, "provider:models")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ProviderModels(ctx, service.ProviderModelsRequest{
			ConnectionID: id, Refresh: c.args.boolean("refresh"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: providerModelsText(res)}, nil
	},
}

func providerModelsText(r *service.ProviderModelsResult) string {
	lines := []string{fmt.Sprintf("%d model(s) of %s through %s", len(r.Models), r.Provider, r.ConnectionID)}

	for i := range r.Models {
		m := &r.Models[i]

		price := "no price: set one with provider:price before generating with it"
		if m.Price != nil {
			price = fmt.Sprintf(
				"in %s, out %s per million tokens, %s (%s)",
				micros(
					m.Price.InputMicrosPerMTok,
				),
				micros(m.Price.OutputMicrosPerMTok),
				m.Price.Currency,
				m.PriceSource,
			)
		}

		// The display name is text the provider wrote: delimited, never bare.
		name := ""
		if m.Name != "" && m.Name != m.ID {
			name = "  " + untrusted.Inline(untrusted.SourceNetworkText, nil, m.Name, 80)
		}

		lines = append(lines, fmt.Sprintf("%s%s  %s", m.ID, name, price))
	}

	return strings.Join(lines, "\n")
}

var providerPriceCmd = &spec{
	person:  true,
	name:    "provider:price",
	summary: "set or remove the price of a model (for a person in the app; the local token is refused)",
	usage: []string{
		"stuarty provider:price --provider <id> --model <id> --currency USD --price-in <micros> --price-out <micros>",
	},
	example: "stuarty provider:price --provider openai --model a-model --currency USD --price-in 2500000 --price-out 10000000",
	write:   true,
	flags: []flagDef{
		{Name: "provider", Kind: kindString, Usage: "the provider id (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id (required)"},
		{Name: "currency", Kind: kindString, Usage: "three letters, such as USD; needed to set a price"},
		{
			Name:  "price-in",
			Kind:  kindString,
			Usage: "the price of a million input tokens, in millionths of the currency unit",
		},
		{
			Name:  "price-out",
			Kind:  kindString,
			Usage: "the price of a million output tokens; leave both prices out to remove the entry",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"provider", "model"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		in, err := microsFlag(c, "price-in")
		if err != nil {
			return nil, err
		}

		out, err := microsFlag(c, "price-out")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ProviderPriceSet(ctx, service.ProviderPriceSetRequest{
			Provider: c.args.str("provider"), Model: c.args.str("model"), Currency: c.args.str("currency"),
			InputMicrosPerMTok: in, OutputMicrosPerMTok: out,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: providerListText(&service.ProviderListResult{Prices: res.Prices})}, nil
	},
}

var textGenerateCmd = &spec{
	name:    "text:generate",
	summary: "generate text with the provider of a connection; the job shows its estimate and waits for approval",
	usage: []string{
		"stuarty text:generate --project <id> --connection <id> --model <id> [--prompt <text> | --prompt-file <file>]" +
			" [--system <text>] [--max-tokens <n>] [--temperature <t>] [--dry-run]",
	},
	example: "stuarty text:generate --project prj_7k2m9x4q1b --connection con_0123456789 --model a-model --prompt \"Write a caption\"",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection to the provider (required)"},
		{Name: "model", Kind: kindString, Usage: "the model id, as provider:models lists it (required)"},
		{
			Name:  "prompt",
			Kind:  kindString,
			Usage: "what to write; without it and without --prompt-file the prompt is read from standard input",
		},
		{Name: "prompt-file", Kind: kindString, Usage: "a file that holds the prompt"},
		{Name: "system", Kind: kindString, Usage: "instructions for the model"},
		{
			Name:  "max-tokens",
			Kind:  kindInt,
			Usage: "the most tokens the model may write (default 1024); the estimate counts all of them",
		},
		{Name: "temperature", Kind: kindString, Usage: "0 to 2"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"connection", "model"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		prompt, err := c.readPrompt()
		if err != nil {
			return nil, err
		}

		req := service.TextGenerateRequest{
			ProjectID: c.projectID, ConnectionID: c.args.str("connection"), DryRun: c.args.boolean("dry-run"),
			Model: c.args.str(
				"model",
			),
			Prompt:    prompt,
			System:    c.args.str("system"),
			MaxTokens: c.args.integer("max-tokens"),
		}

		if c.args.has("temperature") {
			t, perr := strconv.ParseFloat(c.args.str("temperature"), 64)
			if perr != nil {
				return nil, validation("temperature", "--temperature is a number from 0 to 2.")
			}

			req.Temperature = &t
		}

		job, err := c.svc.TextGenerate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}

// readPrompt reads the prompt of text:generate from the flag, a file or standard input.
func (c *call) readPrompt() (string, error) {
	switch {
	case c.args.has("prompt"):
		return c.args.str("prompt"), nil
	case c.args.has("prompt-file"):
		raw, err := os.ReadFile(c.args.str("prompt-file"))
		if err != nil {
			return "", validation("prompt-file", "cannot read the prompt: %s.", readReason(err))
		}

		return string(raw), nil
	}

	raw, err := io.ReadAll(io.LimitReader(c.rt.Stdin, provider.MaxPromptChars*4+1))
	if err != nil {
		return "", validation("prompt", "cannot read the prompt: %s.", readReason(err))
	}

	return strings.TrimRight(string(raw), "\r\n"), nil
}
