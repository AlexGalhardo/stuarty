package service

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The text of the day of the calendar, for the CLI and for the text part of the MCP tool publication. The note is the
// words of a person or of an agent, so it is delimited as untrusted.

// CalendarDayText is one day on a line.
func CalendarDayText(d *CalendarDay) string {
	line := d.Date + "  " + d.Zone

	if d.Theme != "" {
		line += "  theme " + d.Theme
	}

	if d.Quiet {
		line += "  quiet: no posts"
	}

	if d.Note != "" {
		line += "  note " + untrusted.Inline(untrusted.SourceProjectText, nil, d.Note, 200)
	}

	if d.Empty() {
		line += "  nothing set"
	}

	return line
}

// CalendarSlotText is one slot on a line.
func CalendarSlotText(s *CalendarSlot) string {
	line := fmt.Sprintf("%s  %s  every %s at %s %s", s.ID, s.Network, s.Weekday, s.Time, s.Zone)

	if s.ConnectionID != "" {
		line += "  account " + s.ConnectionID
	}

	return line
}

// CalendarPlanText is a range of the calendar: the days that hold something, the slots with the target that fills
// each, and the publications when they were asked for.
func CalendarPlanText(p *CalendarPlanResult) string {
	lines := []string{fmt.Sprintf("calendar %s to %s (%s)", p.From.Format("2006-01-02T15:04:05Z"),
		p.To.Format("2006-01-02T15:04:05Z"), p.Zone)}

	for i := range p.Days {
		lines = append(lines, "day  "+CalendarDayText(&p.Days[i]))
	}

	for i := range p.Slots {
		s := &p.Slots[i]
		line := fmt.Sprintf("slot  %s  %s  %s %s %s (%s)", s.SlotID, s.Network, s.Date, s.Time, s.Zone,
			s.At.Format("2006-01-02T15:04:05Z"))

		if s.Filled != nil {
			line += "  filled by " + s.Filled.TargetID + " of " + s.Filled.PublicationID
		} else {
			line += "  empty"
		}

		lines = append(lines, line)
	}

	lines = append(lines, fmt.Sprintf("%d slot rule(s), %d publication(s)", len(p.Rules), p.Total))

	for i := range p.Publications {
		lines = append(lines, PublicationText(&p.Publications[i]))
	}

	return strings.Join(lines, "\n")
}
