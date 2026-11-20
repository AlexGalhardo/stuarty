package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The calendar commands (SDD-020 section 29). Like the publishing commands they are for the skills of
// publishing, not for the editor skill; publishingCommands includes them.
func publicationCommands() []*spec {
	return []*spec{
		publicationCreateCmd,
		publicationListCmd,
		publicationShowCmd,
		publicationUpdateCmd,
		publicationScheduleCmd,
		publicationSendNowCmd,
		publicationRetryCmd,
		publicationCancelCmd,
	}
}

// publicationText is what a person reads: one line for the publication and one for each target, with the time
// in its zone and in UTC, the state, the address when it is published and the error when it failed.
func publicationText(p *service.Publication) string {
	lines := []string{fmt.Sprintf("%s  %s  project %s  %d target(s)", p.ID,
		untrusted.Inline(untrusted.SourceProjectText, nil, p.Title, 80), p.ProjectID, len(p.Targets))}

	for i := range p.Targets {
		t := &p.Targets[i]
		line := fmt.Sprintf("  %s  %s  %s  %s", t.ID, t.Network, t.ConnectionID, t.State)

		if t.ScheduledAt != nil {
			line += fmt.Sprintf("  %s %s (%s)", t.LocalTime, t.Zone, t.ScheduledAt.UTC().Format("2006-01-02T15:04:05Z"))
		}

		if t.ApprovalID != "" && (t.State == "waiting_approval") {
			line += "  waits for " + t.ApprovalID
		}

		if t.URL != "" {
			line += "  " + untrusted.Inline(untrusted.SourceNetworkText, nil, t.URL, 200)
		}

		lines = append(lines, line)

		if t.Error != nil {
			lines = append(lines, "    "+t.Error.Code+": "+t.Error.Message)

			if t.Error.Hint != "" {
				lines = append(lines, "    hint: "+t.Error.Hint)
			}
		}
	}

	for _, n := range p.Notes {
		lines = append(lines, "note: "+n)
	}

	return strings.Join(lines, "\n")
}

// targetFlags are the flags that describe one target on the command line; --targets takes a JSON list for
// several.
func targetFlags() []flagDef {
	return []flagDef{
		{Name: "connection", Kind: kindString, Usage: "the connection of the account the post goes to"},
		{
			Name:  "at",
			Kind:  kindString,
			Usage: "the time: 2026-11-20T09:00 read in --zone, or an instant such as 2026-11-20T12:00:00Z",
		},
		{
			Name:  "zone",
			Kind:  kindString,
			Usage: "the IANA zone the time is read in, for example America/Sao_Paulo (default UTC)",
		},
		{
			Name:  "late-window",
			Kind:  kindString,
			Usage: "minutes, 0 to 1440, a late post may still be sent without asking (default 0)",
		},
		{Name: "target-title", Kind: kindString, Usage: "a title for this account instead of the publication's"},
		{Name: "target-description", Kind: kindString, Usage: "a description for this account instead of the caption"},
		{Name: "tags", Kind: kindString, Usage: "the tags, a comma list"},
		{Name: "visibility", Kind: kindString, Usage: "private (the default), unlisted or public"},
		{Name: "made-for-kids", Kind: kindString, Usage: "yes or no: whether the video is made for kids"},
		{
			Name:    "ai-disclosure",
			Kind:    kindBool,
			Default: "false",
			Usage:   "the video has realistic altered or synthetic content",
		},
		{Name: "targets", Kind: kindString, Usage: "a JSON list of targets, for several accounts at once"},
	}
}

func lateWindow(c *call) (*int, error) {
	if !c.args.has("late-window") {
		return nil, nil
	}

	var n int
	if _, err := fmt.Sscanf(c.args.str("late-window"), "%d", &n); err != nil {
		return nil, validation("late-window", "--late-window is a number of minutes.")
	}

	return &n, nil
}

// targetsOf reads the targets of the flags: one from --connection and its companions, or a list from --targets.
func targetsOf(c *call) ([]pub.TargetInput, error) {
	if c.args.has("targets") {
		if c.args.has("connection") {
			return nil, validation("targets", "give --targets or --connection, not both.")
		}

		var list []pub.TargetInput
		if err := json.Unmarshal([]byte(c.args.str("targets")), &list); err != nil {
			return nil, validation("targets", "--targets is a JSON list of targets: %v", err)
		}

		return list, nil
	}

	if !c.args.has("connection") {
		return nil, nil
	}

	late, err := lateWindow(c)
	if err != nil {
		return nil, err
	}

	t := pub.TargetInput{
		ConnectionID: c.args.str("connection"), At: c.args.str("at"), Zone: c.args.str("zone"), LateWindowMinutes: late,
		Fields: pub.Fields{
			Title: c.args.str("target-title"), Description: c.args.str("target-description"),
			Tags: splitList(c.args.str("tags")), Visibility: c.args.str("visibility"),
			AIDisclosure: c.args.boolean("ai-disclosure"),
		},
	}

	switch strings.ToLower(c.args.str("made-for-kids")) {
	case "":
	case "yes", "true":
		t.Fields.MadeForKids = new(true)
	case "no", "false":
		t.Fields.MadeForKids = new(false)
	default:
		return nil, validation("made-for-kids", "--made-for-kids is yes or no.")
	}

	return []pub.TargetInput{t}, nil
}

var publicationCreateCmd = &spec{
	name: "publication:create",
	summary: "plan a publication: a video for one or more accounts, each with its own time; nothing is asked or " +
		"sent until you schedule it",
	usage: []string{
		"stuarty publication:create --project <id> --title <t> --export <job> --connection <id> --at <time> " +
			"--zone <zone> --made-for-kids yes|no",
		"stuarty publication:create --project <id> --title <t> --export <job> --targets <json>",
	},
	example: "stuarty publication:create --project prj_7k2m9x4q1b --title \"Week one\" --export job_0123456789 " +
		"--connection con_0123456789 --at 2026-11-20T09:00 --zone America/Sao_Paulo --made-for-kids no",
	project: true,
	write:   true,
	flags: append([]flagDef{
		{Name: "title", Kind: kindString, Usage: "the title (required)"},
		{Name: "text", Kind: kindString, Usage: "the caption the accounts use unless they have their own"},
		{
			Name:  "export",
			Kind:  kindString,
			Usage: "the id of a succeeded export job of the project: the video (required)",
		},
		{Name: "thumbnail", Kind: kindString, Usage: "the id of a succeeded export job of a still: the cover"},
	}, targetFlags()...),
	run: func(ctx context.Context, c *call) (*output, error) {
		targets, err := targetsOf(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublicationCreate(ctx, service.PublicationCreateRequest{
			ProjectID: c.projectID, Title: c.args.str("title"), Text: c.args.str("text"), Export: c.args.str("export"),
			Thumbnail: c.args.str("thumbnail"), Targets: targets,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: publicationText(res)}, nil
	},
}

var publicationListCmd = &spec{
	name:    "publication:list",
	summary: "list the publications of the calendar, by project, state, network or time range",
	usage: []string{
		"stuarty publication:list [--project <id>] [--state <state>] [--from <time>] [--until <time>] [--zone <zone>]",
	},
	example: "stuarty publication:list --from 2026-11-16T00:00:00Z --until 2026-11-23T00:00:00Z --json",
	flags: []flagDef{
		{Name: "project", Kind: kindString, Usage: "only the publications of this project"},
		{
			Name:  "state",
			Kind:  kindString,
			Usage: "only those with a target in this state, for example waiting_approval or failed",
		},
		{Name: "network", Kind: kindString, Usage: "only those with a target on this network"},
		{Name: "from", Kind: kindString, Usage: "only those with a target at or after this time (RFC 3339) or date"},
		{Name: "until", Kind: kindString, Usage: "only those with a target before this time (RFC 3339) or date"},
		{
			Name:  "zone",
			Kind:  kindString,
			Usage: "the zone a date or local time of --from and --until is read in (default UTC)",
		},
		{Name: "limit", Kind: kindInt, Usage: "the page size, 1 to 500 (default 100)"},
		{Name: "offset", Kind: kindInt, Usage: "how many to skip"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.PublicationList(ctx, service.PublicationListRequest{
			ProjectID: c.args.str("project"),
			State:     c.args.str("state"),
			Network:   c.args.str("network"),
			From:      c.args.str("from"),
			To:        c.args.str("until"),
			Zone:      c.args.str("zone"),
			Limit:     c.args.integer("limit"),
			Offset:    c.args.integer("offset"),
		})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d publication(s)", res.Total)}
		for i := range res.Publications {
			lines = append(lines, publicationText(&res.Publications[i]))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var publicationShowCmd = &spec{
	name:       "publication:show",
	summary:    "show one publication with each target: its time, its state, its approval, its address or its error",
	usage:      []string{"stuarty publication:show <publication id>"},
	example:    "stuarty publication:show pub_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "publication:show", "publication id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublicationGet(ctx, service.PublicationGetRequest{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: publicationText(res)}, nil
	},
}

var publicationUpdateCmd = &spec{
	name: "publication:update",
	summary: "change a publication or its targets; a post that waited for approval or was approved goes back " +
		"to waiting for approval",
	usage: []string{
		"stuarty publication:update <publication id> [--title <t>] [--text <t>] [--export <job>] [--targets <json>]",
	},
	example:    "stuarty publication:update pub_0123456789 --title \"A better title\"",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "title", Kind: kindString, Usage: "the new title"},
		{Name: "text", Kind: kindString, Usage: "the new caption"},
		{Name: "export", Kind: kindString, Usage: "the new video: a succeeded export job of the project"},
		{Name: "thumbnail", Kind: kindString, Usage: "the new cover; an empty text removes it"},
		{
			Name: "targets", Kind: kindString,
			Usage: "a JSON list of targets to change (with an id), add (without) or remove (\"remove\":true)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "publication:update", "publication id")
		if err != nil {
			return nil, err
		}

		req := service.PublicationUpdateRequest{ID: id}

		for _, f := range []struct {
			name string
			dst  **string
		}{{"title", &req.Title}, {"text", &req.Text}, {"export", &req.Export}, {"thumbnail", &req.Thumbnail}} {
			if c.args.has(f.name) {
				v := c.args.str(f.name)
				*f.dst = &v
			}
		}

		if c.args.has("targets") {
			if err := json.Unmarshal([]byte(c.args.str("targets")), &req.Targets); err != nil {
				return nil, validation("targets", "--targets is a JSON list of targets: %v", err)
			}
		}

		res, err := c.svc.PublicationUpdate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: publicationText(res)}, nil
	},
}

var publicationScheduleCmd = &spec{
	name: "publication:schedule",
	summary: "give a target its time and ask for approval: it is sent at that time only after a person approves it " +
		"in the app; you cannot approve it",
	usage: []string{
		"stuarty publication:schedule <publication id> [--target <id>] [--at <time>] [--zone <zone>] [--late-window <min>]",
	},
	example:    "stuarty publication:schedule pub_0123456789 --target tgt_0123456789 --at 2026-11-20T09:00 --zone America/Sao_Paulo",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "target", Kind: kindString, Usage: "the target; leave out for every draft target that has a time"},
		{
			Name:  "at",
			Kind:  kindString,
			Usage: "the time: 2026-11-20T09:00 read in --zone, or an instant such as 2026-11-20T12:00:00Z",
		},
		{Name: "zone", Kind: kindString, Usage: "the IANA zone the time is read in (default UTC)"},
		{
			Name:  "late-window",
			Kind:  kindString,
			Usage: "minutes, 0 to 1440, a late post may still be sent without asking (default 0)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "publication:schedule", "publication id")
		if err != nil {
			return nil, err
		}

		late, err := lateWindow(c)
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublicationSchedule(ctx, service.PublicationScheduleRequest{
			ID:                id,
			TargetID:          c.args.str("target"),
			At:                c.args.str("at"),
			Zone:              c.args.str("zone"),
			LateWindowMinutes: late,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: publicationText(res)}, nil
	},
}

func targetCommand(
	name, summary, example string,
	send func(context.Context, *call, string, string) (*service.Publication, error),
) *spec {
	return &spec{
		name:       name,
		summary:    summary,
		usage:      []string{"stuarty " + name + " <publication id> --target <id>"},
		example:    example,
		positional: true,
		write:      true,
		flags:      []flagDef{{Name: "target", Kind: kindString, Usage: "the target, tgt_ and 10 characters"}},
		run: func(ctx context.Context, c *call) (*output, error) {
			id, err := oneArg(c, name, "publication id")
			if err != nil {
				return nil, err
			}

			res, err := send(ctx, c, id, c.args.str("target"))
			if err != nil {
				return nil, err
			}

			return &output{value: res, text: publicationText(res)}, nil
		},
	}
}

var publicationSendNowCmd = targetCommand("publication:send-now",
	"ask for approval to send a target as soon as it is approved; use it for a missed target too",
	"stuarty publication:send-now pub_0123456789 --target tgt_0123456789",
	func(ctx context.Context, c *call, id, target string) (*service.Publication, error) {
		return c.svc.PublicationSendNow(ctx, service.PublicationSendNowRequest{ID: id, TargetID: target})
	})

var publicationRetryCmd = targetCommand("publication:retry",
	"ask for approval to send a failed target again; read why it failed with publication:show",
	"stuarty publication:retry pub_0123456789 --target tgt_0123456789",
	func(ctx context.Context, c *call, id, target string) (*service.Publication, error) {
		return c.svc.PublicationRetry(ctx, service.PublicationRetryRequest{ID: id, TargetID: target})
	})

var publicationCancelCmd = targetCommand("publication:cancel",
	"cancel a target, or every target that has not gone out when --target is left out",
	"stuarty publication:cancel pub_0123456789 --target tgt_0123456789",
	func(ctx context.Context, c *call, id, target string) (*service.Publication, error) {
		return c.svc.PublicationCancel(ctx, service.PublicationCancelRequest{ID: id, TargetID: target})
	})
