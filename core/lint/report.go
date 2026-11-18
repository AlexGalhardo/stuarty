package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

// ReportCap is the character cap of the lint text (SDD-003 §3.6).
const ReportCap = 9600

// Header is what the first line of the report names.
type Header struct {
	Revision  int64
	ProjectID string
	FPS       timeval.FPS
	// Limit is the most findings on a page; 0 means as many as the cap allows.
	Limit int
}

// cursorReserve is room kept for the "next_cursor: ..." line the caller appends.
const cursorReserve = 160

// Report renders findings[from:] as the lint text of SDD-003 §3.6: a header
// with the totals of every finding, then the three-line blocks that fit in
// ReportCap characters (at least one). findings must be in report order (Lint
// and New return it). next is the index of the first finding left out, or -1
// when the page reaches the end; the caller writes the `next_cursor:` line
// and owns the cursor format (SDD-003 §1.5).
func Report(h Header, findings []Finding, from int) (text string, next int) {
	errs, warns := Totals(findings)
	head := fmt.Sprintf("lint r%d %s: errors=%d warnings=%d", h.Revision, h.ProjectID, errs, warns)

	if len(findings) == 0 {
		return head, -1
	}

	from = min(max(from, 0), len(findings)-1)
	// The count in "(showing a of b)" is at most b, so b's width bounds it.
	showing := fmt.Sprintf(" (showing %d of %d)", len(findings), len(findings))
	budget := ReportCap - cursorReserve - utf8.RuneCountInString(head) - len(showing)

	var blocks []string

	used := 0
	next = -1

	for i := from; i < len(findings); i++ {
		b := block(h.FPS, &findings[i])

		cost := utf8.RuneCountInString(b) + 1 // its line feed
		if len(blocks) > 0 && (used+cost > budget || (h.Limit > 0 && len(blocks) >= h.Limit)) {
			next = i
			break
		}

		blocks = append(blocks, b)
		used += cost
	}

	head += fmt.Sprintf(" (showing %d of %d)", len(blocks), len(findings))

	return head + "\n" + strings.Join(blocks, "\n"), next
}

func block(fps timeval.FPS, f *Finding) string {
	item, rng := "-", "-"
	if f.Item != "" {
		item = f.Item
	}

	if f.Range != nil {
		rng = cell(fps, f.Range.Start) + "-" + cell(fps, f.Range.End)
	}

	return fmt.Sprintf("%s %s %s %s\n  %s\n  hint: %s", f.Severity, f.Code, item, rng, f.Message, f.Hint)
}

// cell is the time cell of SDD-003 §3.1: seconds then frames.
func cell(fps timeval.FPS, frame int64) string {
	return fps.FormatSeconds(frame) + "/" + timeval.FormatFrames(frame)
}
