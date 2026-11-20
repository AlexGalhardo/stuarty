package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The approval commands (SDD-015 section 10). Reading the queue is open to every caller. Deciding,
// writing a rule and setting a ceiling are for a person in the app: the commands exist so that an
// agent that tries gets E_PRIVILEGED_SESSION and its hint instead of "unknown command"; run with the
// local token they always fail.

var approvalListCmd = &spec{
	name:    "approval:list",
	summary: "list the approval queue: what waits for a person (pending by default)",
	usage:   []string{"stuarty approval:list"},
	example: "stuarty approval:list --state all",
	flags: append([]flagDef{
		{Name: "state", Kind: kindString, Usage: "pending (default), approved, rejected, expired or all"},
		{Name: "project", Kind: kindString, Usage: "only the requests of one project"},
	}, pageFlags("50")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ApprovalList(ctx, service.ApprovalListRequest{
			State: c.args.str("state"), ProjectID: c.args.str("project"), Page: page(c.args),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var approvalShowCmd = &spec{
	name:       "approval:show",
	summary:    "show one approval request: what will be sent, where, the cost and its state",
	usage:      []string{"stuarty approval:show <id>"},
	example:    "stuarty approval:show apr_0123456789",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "approval:show", "approval request id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ApprovalGet(ctx, service.ApprovalRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Line()}, nil
	},
}

var approvalDecideCmd = &spec{
	person:     true,
	name:       "approval:decide",
	summary:    "approve or reject a request (for a person in the app; the local token is refused)",
	usage:      []string{"stuarty approval:decide <id> --approve|--reject"},
	example:    "stuarty approval:decide apr_0123456789 --reject --reason \"wrong channel\"",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "approve", Kind: kindBool, Usage: "approve the request"},
		{Name: "reject", Kind: kindBool, Usage: "reject the request"},
		{Name: "reason", Kind: kindString, Usage: "why a request is rejected"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "approval:decide", "approval request id")
		if err != nil {
			return nil, err
		}

		if c.args.boolean("approve") == c.args.boolean("reject") {
			return nil, validation("decision", "give exactly one of --approve and --reject.")
		}

		decision := "approve"
		if c.args.boolean("reject") {
			decision = "reject"
		}

		res, err := c.svc.ApprovalDecide(ctx, service.ApprovalDecideRequest{
			ID: id, Decision: decision, Reason: c.args.str("reason"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Line()}, nil
	},
}

var approvalRuleListCmd = &spec{
	name:    "approval:rule:list",
	summary: "list the rules that approve without asking",
	usage:   []string{"stuarty approval:rule:list"},
	example: "stuarty approval:rule:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ApprovalRuleList(ctx, service.ApprovalRuleListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d rule(s)", len(res.Rules))}

		for i := range res.Rules {
			r := &res.Rules[i]
			state := "enabled"

			if !r.Enabled {
				state = "disabled"
			}

			lines = append(lines, fmt.Sprintf("%s  %s  %s  connection %s  project %s  %s", r.ID, r.Kind, state,
				orAny(r.ConnectionID), orAny(r.ProjectID), limits(r.PerItem, r.PerDay, r.Currency)))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var approvalRuleSetCmd = &spec{
	person:  true,
	name:    "approval:rule:set",
	summary: "create or replace a rule that approves a kind of action (for a person in the app; the local token is refused)",
	usage:   []string{"stuarty approval:rule:set --kind <k>"},
	example: "stuarty approval:rule:set --kind publish.upload --connection con_0123456789",
	write:   true,
	flags: []flagDef{
		{Name: "id", Kind: kindString, Usage: "the rule to replace; leave out to create one"},
		{
			Name:  "kind",
			Kind:  kindString,
			Usage: "the kind of action: publish.upload, publish.scheduled, text.generate, ... (required)",
		},
		{Name: "connection", Kind: kindString, Usage: "only this destination"},
		{Name: "project", Kind: kindString, Usage: "only this project"},
		{Name: "currency", Kind: kindString, Usage: "three upper-case letters, needed with a limit"},
		{Name: "per-item", Kind: kindString, Usage: "the most one action may cost, in millionths of the currency unit"},
		{Name: "per-day", Kind: kindString, Usage: "the most the rule may approve in a UTC day, in millionths"},
		{Name: "disabled", Kind: kindBool, Usage: "keep the rule but stop it"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("kind") {
			return nil, validation("kind", "--kind is required.")
		}

		perItem, err := microsFlag(c, "per-item")
		if err != nil {
			return nil, err
		}

		perDay, err := microsFlag(c, "per-day")
		if err != nil {
			return nil, err
		}

		enabled := !c.args.boolean("disabled")

		res, err := c.svc.ApprovalRuleSet(ctx, service.ApprovalRuleSetRequest{
			ID: c.args.str("id"), Kind: c.args.str("kind"), ConnectionID: c.args.str("connection"),
			ProjectID: c.args.str("project"), Currency: c.args.str("currency"), PerItemMicros: perItem,
			PerDayMicros: perDay, Enabled: &enabled,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "rule " + res.ID + " for " + res.Kind}, nil
	},
}

var approvalRuleRemoveCmd = &spec{
	person:     true,
	name:       "approval:rule:remove",
	summary:    "remove a rule (for a person in the app; the local token is refused)",
	usage:      []string{"stuarty approval:rule:remove <id>"},
	example:    "stuarty approval:rule:remove rul_0123456789",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "approval:rule:remove", "rule id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ApprovalRuleRemove(ctx, service.ApprovalRef{ID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: "removed " + res.ID}, nil
	},
}

var costShowCmd = &spec{
	name:    "cost:show",
	summary: "show the cost ceilings and what was spent today and over the last 30 days",
	usage:   []string{"stuarty cost:show"},
	example: "stuarty cost:show --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CostSummary(ctx, service.CostSummaryRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: costText(res)}, nil
	},
}

var costCeilingCmd = &spec{
	person:  true,
	name:    "cost:ceiling",
	summary: "set or remove a cost ceiling (for a person in the app; the local token is refused)",
	usage:   []string{"stuarty cost:ceiling --currency USD"},
	example: "stuarty cost:ceiling --currency USD --per-day 5000000",
	write:   true,
	flags: []flagDef{
		{Name: "currency", Kind: kindString, Usage: "three upper-case letters (required)"},
		{Name: "project", Kind: kindString, Usage: "the project the ceiling is for; leave out for the whole workspace"},
		{Name: "per-item", Kind: kindString, Usage: "the most one action may cost, in millionths of the currency unit"},
		{
			Name:  "per-day",
			Kind:  kindString,
			Usage: "the most that may be spent in a UTC day, in millionths; with neither limit the ceiling is removed",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("currency") {
			return nil, validation("currency", "--currency is required.")
		}

		perItem, err := microsFlag(c, "per-item")
		if err != nil {
			return nil, err
		}

		perDay, err := microsFlag(c, "per-day")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.CostCeilingSet(ctx, service.CostCeilingSetRequest{
			ProjectID: c.args.str(
				"project",
			),
			Currency:      c.args.str("currency"),
			PerItemMicros: perItem,
			PerDayMicros:  perDay,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: costText(res)}, nil
	},
}

func oneArg(c *call, name, what string) (string, error) {
	if len(c.args.Positional) != 1 {
		return "", validation("", "%s takes one %s, got %d.", name, what, len(c.args.Positional))
	}

	return c.args.Positional[0], nil
}

// microsFlag reads a limit given in micros; an absent flag is nil.
func microsFlag(c *call, name string) (*int64, error) {
	if !c.args.has(name) {
		return nil, nil
	}

	n, err := strconv.ParseInt(c.args.str(name), 10, 64)
	if err != nil {
		return nil, validation(name, "--%s is a whole number of millionths of the currency unit.", name)
	}

	return &n, nil
}

func orAny(p *string) string {
	if p == nil {
		return "any"
	}

	return *p
}

func limits(perItem, perDay *int64, currency string) string {
	if perItem == nil && perDay == nil {
		return "no limits"
	}

	part := func(label string, v *int64) string {
		if v == nil {
			return label + " -"
		}

		return fmt.Sprintf("%s %d.%06d", label, *v/1_000_000, *v%1_000_000)
	}

	return part("per item", perItem) + ", " + part("per day", perDay) + " " + currency
}

func costText(r *service.CostSummaryResult) string {
	lines := []string{fmt.Sprintf("cost on %s (UTC), in millionths of the currency unit", r.Day)}

	if len(r.Ceilings) == 0 {
		lines = append(lines, "no ceilings")
	}

	for i := range r.Ceilings {
		c := &r.Ceilings[i]
		lines = append(
			lines,
			fmt.Sprintf(
				"ceiling %s project %s: %s",
				c.Currency,
				orAny(c.ProjectID),
				limits(c.PerItem, c.PerDay, c.Currency),
			),
		)
	}

	for _, l := range r.Today {
		lines = append(lines, fmt.Sprintf("today %s %s %s: %d", l.Scope, l.ProjectID, l.Currency, l.Micros))
	}

	return strings.Join(lines, "\n")
}
