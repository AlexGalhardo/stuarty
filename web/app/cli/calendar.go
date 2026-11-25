package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// The commands of the day of the publishing calendar (SDD-020 section 30A). Like the calendar commands they are for
// the skills of publishing; publicationCommands includes them.
func calendarCommands() []*spec {
	return []*spec{
		calendarPlanCmd,
		calendarDayCmd,
		calendarQuietCmd,
		calendarSlotCmd,
		calendarSlotRemoveCmd,
		calendarExportCmd,
		publicationDuplicateCmd,
	}
}

func rangeFlags() []flagDef {
	return []flagDef{
		{
			Name:  "from",
			Kind:  kindString,
			Usage: "the start of the range: a date such as 2026-12-01, a local time or an instant",
		},
		{Name: "until", Kind: kindString, Usage: "the end of the range, exclusive, in the same forms"},
		{Name: "zone", Kind: kindString, Usage: "the zone dates and local times are read in (default UTC)"},
	}
}

var calendarPlanCmd = &spec{
	name: "calendar:plan",
	summary: "read a range of the calendar: its days with notes, themes and quiet marks, its recurring slots and " +
		"the target that fills each one",
	usage: []string{
		"stuarty calendar:plan --from <date> --until <date> [--zone <zone>] [--publications] [--project <id>]",
	},
	example: "stuarty calendar:plan --from 2026-12-01 --until 2026-12-02 --zone America/Sao_Paulo --publications",
	flags: append(rangeFlags(),
		flagDef{Name: "publications", Kind: kindBool, Usage: "also list the publications of the range"},
		flagDef{Name: "project", Kind: kindString, Usage: "only the publications of this project"},
		flagDef{Name: "limit", Kind: kindInt, Usage: "the page size of the publications, 1 to 500 (default 100)"},
		flagDef{Name: "offset", Kind: kindInt, Usage: "how many publications to skip"},
	),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CalendarPlan(ctx, service.CalendarPlanRequest{
			From: c.args.str("from"), To: c.args.str("until"), Zone: c.args.str("zone"),
			ProjectID: c.args.str("project"), Publications: c.args.boolean("publications"),
			Limit: c.args.integer("limit"), Offset: c.args.integer("offset"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CalendarPlanText(res)}, nil
	},
}

var calendarDayCmd = &spec{
	name: "calendar:day",
	summary: "write the note and the theme of a day (a flag you leave out stays; an empty one clears it); " +
		"a person marks a day as quiet with calendar:quiet",
	usage: []string{"stuarty calendar:day <date> [--note <text>] [--theme <name>] [--zone <zone>]"},
	example: "stuarty calendar:day 2026-12-01 --note \"long video day\" --theme amber " +
		"--zone America/Sao_Paulo",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "note", Kind: kindString, Usage: "the note, at most 2000 characters; empty clears it"},
		{
			Name:  "theme",
			Kind:  kindString,
			Usage: "blue, green, amber, red, teal or gray; empty clears it",
		},
		{Name: "zone", Kind: kindString, Usage: "the zone the day is planned in (default: the one it has, or UTC)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		date, err := oneArg(c, "calendar:day", "date")
		if err != nil {
			return nil, err
		}

		req := service.CalendarDaySetRequest{Date: date, Zone: c.args.str("zone")}

		if c.args.has("note") {
			v := c.args.str("note")
			req.Note = &v
		}

		if c.args.has("theme") {
			v := c.args.str("theme")
			req.Theme = &v
		}

		res, err := c.svc.CalendarDaySet(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CalendarDayText(res)}, nil
	},
}

var calendarQuietCmd = &spec{
	name: "calendar:quiet",
	summary: "mark a day as quiet (no posts) or clear the mark; only a person in the app does, every other caller " +
		"is refused",
	usage:      []string{"stuarty calendar:quiet <date> [--off] [--zone <zone>]"},
	example:    "stuarty calendar:quiet 2026-12-25 --zone America/Sao_Paulo",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "off", Kind: kindBool, Usage: "clear the mark instead of setting it"},
		{Name: "zone", Kind: kindString, Usage: "the zone the day is planned in (default: the one it has, or UTC)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		date, err := oneArg(c, "calendar:quiet", "date")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.CalendarQuietSet(ctx, service.CalendarQuietSetRequest{
			Date: date, Zone: c.args.str("zone"), Quiet: !c.args.boolean("off"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CalendarDayText(res)}, nil
	},
}

var calendarSlotCmd = &spec{
	name: "calendar:slot",
	summary: "create or change a recurring slot: a weekday and a local time on a network, shown as an empty slot " +
		"to fill; it never sends anything",
	usage: []string{
		"stuarty calendar:slot --network <n> --weekday mon..sun --time HH:MM [--zone <zone>] [--connection <id>] [--id <slot>]",
	},
	example: "stuarty calendar:slot --network instagram --weekday tue --time 18:00 --zone America/Sao_Paulo",
	write:   true,
	flags: []flagDef{
		{Name: "id", Kind: kindString, Usage: "the slot to change; leave out to make a new one"},
		{Name: "network", Kind: kindString, Usage: "the network, for example instagram (required)"},
		{Name: "weekday", Kind: kindString, Usage: "mon, tue, wed, thu, fri, sat or sun (required)"},
		{Name: "time", Kind: kindString, Usage: "the local time, 24 hours, such as 18:00 (required)"},
		{Name: "zone", Kind: kindString, Usage: "the zone of the time (default UTC)"},
		{Name: "connection", Kind: kindString, Usage: "only this account of the network (default any)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CalendarSlotSave(ctx, service.CalendarSlotSaveRequest{
			ID: c.args.str("id"), Network: c.args.str("network"), Weekday: c.args.str("weekday"),
			Time: c.args.str("time"), Zone: c.args.str("zone"), ConnectionID: c.args.str("connection"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CalendarSlotText(res)}, nil
	},
}

var calendarSlotRemoveCmd = &spec{
	name:       "calendar:slot-remove",
	summary:    "remove a recurring slot",
	usage:      []string{"stuarty calendar:slot-remove <slot id>"},
	example:    "stuarty calendar:slot-remove slt_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "calendar:slot-remove", "slot id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.CalendarSlotDelete(ctx, service.CalendarSlotDeleteRequest{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "removed " + res.ID}, nil
	},
}

var calendarExportCmd = &spec{
	name: "calendar:export",
	summary: "print the targets of a range as an .ics file (read only, times in UTC, no credential); redirect it " +
		"to a file to open it in another calendar app",
	usage: []string{
		"stuarty calendar:export --from <date> --until <date> [--zone <zone>] [--project <id>] > plan.ics",
	},
	example: "stuarty calendar:export --from 2026-12-01 --until 2027-01-01 > plan.ics",
	flags: append(rangeFlags(),
		flagDef{Name: "project", Kind: kindString, Usage: "only the publications of this project"},
	),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CalendarExport(ctx, service.CalendarExportRequest{
			From: c.args.str(
				"from",
			),
			To:        c.args.str("until"),
			Zone:      c.args.str("zone"),
			ProjectID: c.args.str("project"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Body}, nil
	},
}

var publicationDuplicateCmd = &spec{
	name: "publication:duplicate",
	summary: "copy a publication to another day or hour as a new draft; nothing is asked or sent until you " +
		"schedule the copy",
	usage: []string{
		"stuarty publication:duplicate <publication id> --at <time> [--zone <zone>] [--target <id>]",
	},
	example:    "stuarty publication:duplicate pub_0123456789 --at 2026-12-02T18:00 --zone America/Sao_Paulo",
	positional: true,
	write:      true,
	flags: []flagDef{
		{
			Name:  "at",
			Kind:  kindString,
			Usage: "the new time: 2026-12-02T18:00 read in --zone, or an instant (required)",
		},
		{Name: "zone", Kind: kindString, Usage: "the zone the time is read in (default: the zone of each target)"},
		{
			Name:  "target",
			Kind:  kindString,
			Usage: "copy only this target (default: every target that is not cancelled)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "publication:duplicate", "publication id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.PublicationDuplicate(ctx, service.PublicationDuplicateRequest{
			ID: id, TargetID: c.args.str("target"), At: c.args.str("at"), Zone: c.args.str("zone"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: publicationText(res)}, nil
	},
}
