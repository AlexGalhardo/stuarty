package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The social network and publishing commands (SDD-020 section 13). They are for the skills of
// publishing, not for the editor skill, whose size is limited (SDD-003 section 8.1), as the library
// commands are: publishingCommands is read by AgentDocsInputs.
func publishingCommands() []*spec {
	return append([]*spec{
		socialNetworkListCmd,
		socialConnectCmd,
		socialAccountListCmd,
		publishValidateCmd,
		publishNowCmd,
		publishStatusCmd,
	}, publicationCommands()...)
}

var socialNetworkListCmd = &spec{
	name:    "social:network:list",
	summary: "list the social networks this build can publish to, with their limits and where they were read",
	usage:   []string{"stuarty social:network:list"},
	example: "stuarty social:network:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.SocialNetworkList(ctx, service.SocialNetworkListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d network(s); the redirect path is %s", len(res.Networks), res.RedirectPath)}
		for i := range res.Networks {
			n := &res.Networks[i]
			lines = append(lines, fmt.Sprintf("%s  %s  publishes: %s  limits read %s from %s",
				n.Network, n.Name, strings.Join(n.Publishes, ", "), n.Limits.Read, n.Limits.Source))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var socialConnectCmd = &spec{
	name: "social:connect",
	summary: "start the sign-in to a network with your own developer app; the client secret is read from " +
		"standard input or a file, never from an argument",
	usage: []string{
		"stuarty social:connect --network <n> --client-id <id> --redirect-uri <url> [--label <t>]",
		"stuarty social:connect --network <n> --connection <id> --redirect-uri <url>",
	},
	example: "echo $CLIENT_SECRET | stuarty social:connect --network youtube --client-id 123.apps.googleusercontent.com " +
		"--redirect-uri http://127.0.0.1:7878/social/callback",
	write: true,
	flags: []flagDef{
		{Name: "network", Kind: kindString, Usage: "the network, for example youtube (required)"},
		{Name: "client-id", Kind: kindString, Usage: "the client id of your developer project"},
		{
			Name:  "redirect-uri",
			Kind:  kindString,
			Usage: "the address of this app followed by /social/callback (required)",
		},
		{Name: "label", Kind: kindString, Usage: "a name for the connection"},
		{Name: "connection", Kind: kindString, Usage: "sign in again an existing connection instead of making one"},
		{
			Name:  "secret-file",
			Kind:  kindString,
			Usage: "a file that holds the client secret; without it it is read from standard input",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"network", "redirect-uri"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		req := service.SocialConnectBeginRequest{
			Network: c.args.str("network"), Label: c.args.str("label"), RedirectURI: c.args.str("redirect-uri"),
			ClientID: c.args.str("client-id"), ConnectionID: c.args.str("connection"),
		}

		if req.ConnectionID == "" {
			secret, err := c.readSecret()
			if err != nil {
				return nil, err
			}

			req.ClientSecret = secret
		}

		res, err := c.svc.SocialConnectBegin(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: fmt.Sprintf(
			"connection %s\nopen this address in your browser, sign in and accept; the app finishes the sign-in:\n%s\n"+
				"then check it with social:account:list", res.ConnectionID, res.AuthorizeURL)}, nil
	},
}

var socialAccountListCmd = &spec{
	name:    "social:account:list",
	summary: "list the connected social accounts and whether each is signed in",
	usage:   []string{"stuarty social:account:list"},
	example: "stuarty social:account:list",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.SocialAccountList(ctx, service.SocialAccountListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d account(s)", len(res.Accounts))}
		for i := range res.Accounts {
			a := &res.Accounts[i]
			lines = append(lines, fmt.Sprintf("%s  %s  %s  channel %s  %s", a.ConnectionID, a.Network,
				untrusted.Inline(untrusted.SourceProjectText, nil, a.Label, 60),
				untrusted.Inline(untrusted.SourceNetworkText, nil, a.AccountTitle, 80), a.State))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

func publishFlags() []flagDef {
	return []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection the video goes to (required)"},
		{
			Name:  "export",
			Kind:  kindString,
			Usage: "the id of a succeeded export job of the project: the video (required)",
		},
		{
			Name:  "thumbnail",
			Kind:  kindString,
			Usage: "the cover: the export job of a still, or the job_id of image:export",
		},
		{Name: "title", Kind: kindString, Usage: "the title (required)"},
		{Name: "description", Kind: kindString, Usage: "the description"},
		{Name: "tags", Kind: kindString, Usage: "the tags, a comma list"},
		{Name: "visibility", Kind: kindString, Usage: "private (the default), unlisted or public"},
		{Name: "made-for-kids", Kind: kindString, Usage: "yes or no: whether the video is made for kids (required)"},
		{
			Name:    "ai-disclosure",
			Kind:    kindBool,
			Default: "false",
			Usage:   "the video has realistic altered or synthetic content",
		},
	}
}

func publishArgs(c *call) (service.PublishArgs, error) {
	a := service.PublishArgs{
		ConnectionID: c.args.str("connection"), Export: c.args.str("export"), Thumbnail: c.args.str("thumbnail"),
		Title: c.args.str("title"), Description: c.args.str("description"), Tags: splitList(c.args.str("tags")),
		Visibility: c.args.str("visibility"), AIDisclosure: c.args.boolean("ai-disclosure"),
	}

	switch strings.ToLower(c.args.str("made-for-kids")) {
	case "":
	case "yes", "true":
		a.MadeForKids = new(true)
	case "no", "false":
		a.MadeForKids = new(false)
	default:
		return a, validation("made-for-kids", "--made-for-kids is yes or no.")
	}

	return a, nil
}

var publishValidateCmd = &spec{
	name:    "publish:validate",
	summary: "check a post against the limits of its network and show what a person would approve; sends nothing",
	usage: []string{
		"stuarty publish:validate --project <id> --connection <id> --export <job> --title <t> --made-for-kids yes|no",
	},
	example: "stuarty publish:validate --project prj_7k2m9x4q1b --connection con_0123456789 --export job_0123456789 " +
		"--title \"My short\" --made-for-kids no",
	project: true,
	flags:   publishFlags(),
	run: func(ctx context.Context, c *call) (*output, error) {
		args, err := publishArgs(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublishValidate(
			ctx,
			service.PublishValidateRequest{ProjectID: c.projectID, PublishArgs: args},
		)
		if err != nil {
			return nil, err
		}

		lines := []string{"state: " + res.State}
		for _, i := range res.Issues {
			lines = append(lines, fmt.Sprintf("  %s (%s): %s", i.Path, i.Code, i.Message))
		}

		if res.Summary != "" {
			lines = append(lines, untrusted.Inline(untrusted.SourceProjectText, nil, res.Summary, 500))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var publishNowCmd = &spec{
	name: "publish:now",
	summary: "prepare the publication of an exported video: it waits for a person to approve it in the app, " +
		"then it is sent; you cannot approve it",
	usage: []string{
		"stuarty publish:now --project <id> --connection <id> --export <job> --title <t> --made-for-kids yes|no",
	},
	example: "stuarty publish:now --project prj_7k2m9x4q1b --connection con_0123456789 --export job_0123456789 " +
		"--title \"My short\" --made-for-kids no --visibility private",
	project:  true,
	timeline: true,
	write:    true,
	globals:  []string{"dry-run"},
	flags:    publishFlags(),
	run: func(ctx context.Context, c *call) (*output, error) {
		args, err := publishArgs(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublishNow(ctx, service.PublishNowRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, PublishArgs: args, DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var publishStatusCmd = &spec{
	name:       "publish:status",
	summary:    "show the state of a publication: waiting_approval, uploading, processing, published, sent_as_draft or failed",
	usage:      []string{"stuarty publish:status <job id>"},
	example:    "stuarty publish:status job_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "publish:status", "job id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublishStatus(ctx, service.PublishStatusRequest{JobID: id})
		if err != nil {
			return nil, err
		}

		text := res.JobID + " " + res.State
		if res.Result != nil {
			text += " " + untrusted.Inline(untrusted.SourceNetworkText, nil, res.Result.URL, 200)
		}

		if res.Error != nil {
			text += "\nerror " + res.Error.Code + ": " + res.Error.Message
			if res.Error.Hint != "" {
				text += "\nhint: " + res.Error.Hint
			}
		}

		return &output{value: res, text: text}, nil
	},
}
