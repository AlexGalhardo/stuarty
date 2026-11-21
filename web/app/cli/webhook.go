package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The webhook commands (SDD-022 section 7). A webhook is a destination a person registers: the address is given to
// webhook:add and webhook:update with --endpoint, as connection:add does, and no other command names one.

func webhookLine(w *service.Webhook) string {
	state := "on"
	if !w.Enabled {
		state = "off"
	}

	last := "no delivery yet"
	if w.LastDelivery != nil {
		last = fmt.Sprintf("last %s (%d)", w.LastDelivery.State, w.LastDelivery.Status)
	}

	return fmt.Sprintf("%s  %s  %s  %s  [%s]  %s", w.ID, state, w.Name, w.URL, strings.Join(w.Events, ","), last)
}

var webhookEventsCmd = &spec{
	name:    "webhook:events",
	summary: "list the events a webhook may choose and what each one carries",
	usage:   []string{"stuarty webhook:events"},
	example: "stuarty webhook:events --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.WebhookEventList(ctx, service.WebhookEventListRequest{})
		if err != nil {
			return nil, err
		}

		lines := make([]string, 0, len(res.Events))
		for _, e := range res.Events {
			lines = append(lines, e.Name+": "+e.Description)
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var webhookListCmd = &spec{
	name:    "webhook:list",
	summary: "list the outbound webhooks",
	usage:   []string{"stuarty webhook:list"},
	example: "stuarty webhook:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.WebhookList(ctx, service.WebhookListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d webhook(s)", len(res.Webhooks))}
		for i := range res.Webhooks {
			lines = append(lines, webhookLine(&res.Webhooks[i]))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var webhookShowCmd = &spec{
	name:       "webhook:show",
	summary:    "show one webhook",
	usage:      []string{"stuarty webhook:show <id>"},
	example:    "stuarty webhook:show whk_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:show", "webhook id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.WebhookGet(ctx, service.WebhookRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: webhookLine(res)}, nil
	},
}

var webhookAddCmd = &spec{
	name:    "webhook:add",
	summary: "register a webhook; the signing secret is printed once and cannot be shown again",
	usage:   []string{"stuarty webhook:add --name <t> --endpoint <url> --events <list> [--disable]"},
	example: "stuarty webhook:add --name n8n --endpoint https://stuarty.example.com/hooks --events export.ready,job.failed",
	write:   true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "a name for the person, 1 to 60 characters (required)"},
		{
			Name:  "endpoint",
			Kind:  kindString,
			Usage: "where the events are sent: https, or http for localhost (required)",
		},
		{
			Name:  "events",
			Kind:  kindString,
			Usage: "a comma list of the events to receive (required); see webhook:events",
		},
		{Name: "disable", Kind: kindBool, Default: "false", Usage: "register it turned off"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		for _, f := range []string{"name", "endpoint", "events"} {
			if !c.args.has(f) {
				return nil, validation(f, "--%s is required.", f)
			}
		}

		req := service.WebhookCreateRequest{
			Name: c.args.str("name"), URL: c.args.str("endpoint"), Events: splitList(c.args.str("events")),
		}

		if c.args.boolean("disable") {
			off := false
			req.Enabled = &off
		}

		res, err := c.svc.WebhookCreate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{
			value: res,
			text:  "registered:\n" + webhookLine(&res.Webhook) + "\n\nsigning secret (shown once): " + res.Secret,
		}, nil
	},
}

var webhookUpdateCmd = &spec{
	name:    "webhook:update",
	summary: "change the name, the address, the events or the state of a webhook",
	usage: []string{
		"stuarty webhook:update <id> [--name <t>] [--endpoint <url>] [--events <list>] [--enable|--disable]",
	},
	example:    "stuarty webhook:update whk_0123456789 --disable",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "the new name"},
		{Name: "endpoint", Kind: kindString, Usage: "the new address"},
		{Name: "events", Kind: kindString, Usage: "the new comma list of events"},
		{Name: "enable", Kind: kindBool, Default: "false", Usage: "turn the webhook on"},
		{Name: "disable", Kind: kindBool, Default: "false", Usage: "turn the webhook off"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:update", "webhook id")
		if err != nil {
			return nil, err
		}

		if c.args.boolean("enable") && c.args.boolean("disable") {
			return nil, validation("enable", "--enable and --disable cannot be used together.")
		}

		req := service.WebhookUpdateRequest{ID: id}

		if c.args.has("name") {
			v := c.args.str("name")
			req.Name = &v
		}

		if c.args.has("endpoint") {
			v := c.args.str("endpoint")
			req.URL = &v
		}

		if c.args.has("events") {
			v := splitList(c.args.str("events"))
			req.Events = &v
		}

		if c.args.boolean("enable") || c.args.boolean("disable") {
			v := c.args.boolean("enable")
			req.Enabled = &v
		}

		res, err := c.svc.WebhookUpdate(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "updated:\n" + webhookLine(res)}, nil
	},
}

var webhookRemoveCmd = &spec{
	name:       "webhook:remove",
	summary:    "remove a webhook, its deliveries and its secret",
	usage:      []string{"stuarty webhook:remove <id>"},
	example:    "stuarty webhook:remove whk_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:remove", "webhook id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.WebhookRemove(ctx, service.WebhookRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "removed " + res.ID}, nil
	},
}

var webhookRotateCmd = &spec{
	name:       "webhook:rotate-secret",
	summary:    "make a new signing secret; it is printed once and the old one stops verifying",
	usage:      []string{"stuarty webhook:rotate-secret <id>"},
	example:    "stuarty webhook:rotate-secret whk_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:rotate-secret", "webhook id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.WebhookRotateSecret(ctx, service.WebhookRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "new signing secret (shown once): " + res.Secret}, nil
	},
}

func deliveryLine(d *service.WebhookDelivery) string {
	return fmt.Sprintf("%s  %s  %s  attempts %d/%d  status %d  %s", d.ID, d.State, d.Event, d.Attempts, d.MaxAttempts,
		d.LastStatus, d.LastError)
}

var webhookTestCmd = &spec{
	name:       "webhook:test",
	summary:    "send a webhook.test event now, one attempt, and show how the receiver answered",
	usage:      []string{"stuarty webhook:test <id>"},
	example:    "stuarty webhook:test whk_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:test", "webhook id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.WebhookTest(ctx, service.WebhookRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: deliveryLine(res)}, nil
	},
}

var webhookDeliveriesCmd = &spec{
	name:       "webhook:deliveries",
	summary:    "list the deliveries of a webhook, newest first, with their attempts",
	usage:      []string{"stuarty webhook:deliveries <id> [--limit n] [--offset n]"},
	example:    "stuarty webhook:deliveries whk_0123456789 --json",
	positional: true,
	flags: []flagDef{
		{Name: "limit", Kind: kindInt, Default: "50", Usage: "deliveries in the page, 1 to 100"},
		{Name: "offset", Kind: kindInt, Default: "0", Usage: "deliveries to skip"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "webhook:deliveries", "webhook id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.WebhookDeliveryList(ctx, service.WebhookDeliveriesRequest{
			ID: id, Limit: c.args.integer("limit"), Offset: c.args.integer("offset"),
		})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d delivery(ies)", len(res.Deliveries))}
		for i := range res.Deliveries {
			lines = append(lines, deliveryLine(&res.Deliveries[i]))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var webhookRedeliverCmd = &spec{
	name:       "webhook:redeliver",
	summary:    "send the stored body of a delivery again, with a fresh signature",
	usage:      []string{"stuarty webhook:redeliver <webhook id> <delivery id>"},
	example:    "stuarty webhook:redeliver whk_0123456789 wdl_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 2 {
			return nil, validation(
				"",
				"webhook:redeliver takes a webhook id and a delivery id, got %d.",
				len(c.args.Positional),
			)
		}

		res, err := c.svc.WebhookRedeliver(ctx, service.WebhookRedeliverRequest{
			ID: c.args.Positional[0], DeliveryID: c.args.Positional[1],
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "queued " + deliveryLine(res)}, nil
	},
}
