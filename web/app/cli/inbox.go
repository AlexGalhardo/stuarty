package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/inbox"
	"github.com/AlexGalhardo/stuarty/service"
)

// The command of the unified read of what waits for a person (SDD-015 section 4.1, epic 77). It reads and decides
// nothing: a permission is answered in the chat panel and an approval in the Approvals page. Every text of a row is
// untrusted (SDD-007 section 8).

var inboxListCmd = &spec{
	name:    "inbox:list",
	summary: "list what waits for a person: the permission requests of the chat and the approvals, and the last answered",
	usage:   []string{"stuarty inbox:list"},
	example: "stuarty inbox:list --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.InboxList(ctx, service.InboxListRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: inboxText(res)}, nil
	},
}

// inboxText tells the rows in lines. Every agent, tool and item in it is cut to a line and never interpreted; an
// empty member is "not reported".
func inboxText(res *service.InboxListResult) string {
	lines := []string{fmt.Sprintf("%d request(s) waiting for a person, %d listed", res.Waiting, len(res.Rows))}

	for i := range res.Rows {
		r := &res.Rows[i]

		cost := "no cost"
		if r.Cost != nil {
			cost = fmt.Sprintf("%d micros %s", r.Cost.Micros, r.Cost.Currency)
		}

		undo := "cannot be undone or not reported"
		if r.Undoable {
			undo = "can be undone"
		}

		lines = append(
			lines,
			fmt.Sprintf("%s  %s  %s  %s", r.ID, r.State, r.Source, reported(r.Agent)),
			fmt.Sprintf(
				"  what: %s  item: %s  where: %s  cost: %s  %s",
				reported(r.Tool), reported(r.Item), reported(r.Destination), cost, undo,
			),
		)

		if r.Source == inbox.SourceApproval && r.Summary != "" {
			lines = append(lines, "  summary: "+firstLine(r.Summary))
		}
	}

	return strings.Join(lines, "\n")
}

func reported(s string) string {
	if s == "" {
		return "not reported"
	}

	return firstLine(s)
}
