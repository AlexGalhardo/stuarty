package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

var connectionListCmd = &spec{
	name:    "connection:list",
	summary: "list the destinations Stuarty may reach",
	usage:   []string{"stuarty connection:list"},
	example: "stuarty connection:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ConnectionList(ctx, service.ConnectionListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d connection(s)", len(res.Connections))}
		for i := range res.Connections {
			lines = append(lines, res.Connections[i].Summary())
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var connectionShowCmd = &spec{
	name:       "connection:show",
	summary:    "show one connection: destinations, what it accepts and its last use",
	usage:      []string{"stuarty connection:show <id>"},
	example:    "stuarty connection:show con_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneID(c, "connection:show")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ConnectionGet(ctx, service.ConnectionRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Summary()}, nil
	},
}

var connectionAddCmd = &spec{
	name:    "connection:add",
	summary: "connect a destination; the secret is read from standard input or a file, never from an argument",
	usage:   []string{"stuarty connection:add --kind <k> --label <t> --endpoint <url> --accepts <list>"},
	example: "echo $TOKEN | stuarty connection:add --kind test --label Blog --endpoint https://video.example.com --accepts text",
	write:   true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "the kind of connection (required)"},
		{Name: "label", Kind: kindString, Usage: "a name for the person, 1 to 60 characters (required)"},
		{Name: "endpoint", Kind: kindString, Usage: "the URL the connector calls first, https (required)"},
		{
			Name:  "accepts",
			Kind:  kindString,
			Usage: "what it may receive, a comma list of text, image, video, audio, metadata, document (required)",
		},
		{
			Name:  "hosts",
			Kind:  kindString,
			Usage: "further hosts it may reach, a comma list; the endpoint host is always allowed",
		},
		{
			Name:  "secret-file",
			Kind:  kindString,
			Usage: "a file that holds the secret; without it the secret is read from standard input",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"kind", "label", "endpoint", "accepts"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		secret, err := c.readSecret()
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ConnectionCreate(ctx, service.ConnectionCreateRequest{
			Kind: c.args.str("kind"), Label: c.args.str("label"), Endpoint: c.args.str("endpoint"),
			Hosts: splitList(c.args.str("hosts")), Accepts: splitList(c.args.str("accepts")), Secret: secret,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "connected: " + res.Summary()}, nil
	},
}

// readSecret reads the credential of connection:add from a file or from standard input, without the
// trailing line break.
func (c *call) readSecret() (string, error) {
	var (
		raw []byte
		err error
	)

	if c.args.has("secret-file") {
		raw, err = os.ReadFile(c.args.str("secret-file"))
	} else {
		raw, err = io.ReadAll(io.LimitReader(c.rt.Stdin, connections.MaxSecret+1))
	}

	if err != nil {
		return "", validation("secret", "cannot read the secret: %s.", readReason(err))
	}

	secret := strings.TrimRight(string(raw), "\r\n")
	if secret == "" {
		return "", validation("secret", "the secret is empty: pipe it to standard input or use --secret-file.")
	}

	return secret, nil
}

var connectionRemoveCmd = &spec{
	name:       "connection:remove",
	summary:    "remove a connection and its credential",
	usage:      []string{"stuarty connection:remove <id>"},
	example:    "stuarty connection:remove con_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneID(c, "connection:remove")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ConnectionRemove(ctx, service.ConnectionRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "removed " + res.ID}, nil
	},
}

var connectionTestCmd = &spec{
	name:       "connection:test",
	summary:    "make one request through a connection and show how the destination answered",
	usage:      []string{"stuarty connection:test <id>"},
	example:    "stuarty connection:test con_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneID(c, "connection:test")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ConnectionTest(ctx, service.ConnectionRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: fmt.Sprintf("ok: %t, status: %d\n%s", res.OK, res.Status, res.Message)}, nil
	},
}

func oneID(c *call, name string) (string, error) {
	if len(c.args.Positional) != 1 {
		return "", validation("", "%s takes one connection id, got %d.", name, len(c.args.Positional))
	}

	return c.args.Positional[0], nil
}
