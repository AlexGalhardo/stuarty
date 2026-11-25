package service

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// PublicationText is what a person or an agent reads: one line for the publication and one for each target, with
// the time in its zone and in UTC, the state, the address when it is published and the error when it failed. The
// CLI and the MCP tool publication print it.
func PublicationText(p *Publication) string {
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

// PublicationListText is the text of a page of publications.
func PublicationListText(res *PublicationListResult) string {
	lines := []string{fmt.Sprintf("%d publication(s)", res.Total)}
	for i := range res.Publications {
		lines = append(lines, PublicationText(&res.Publications[i]))
	}

	return strings.Join(lines, "\n")
}
