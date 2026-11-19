package textview

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// CharsPerToken is the conservative estimate of SDD-003 section 3.10 that turns a token budget into characters.
const CharsPerToken = 3

// ProductionLine is one item of a production in the description.
type ProductionLine struct {
	ID      string
	Kind    string
	Title   string
	Trashed bool
	// Detail is what follows the title: for a timeline its id, size, rate, duration and revision; for an output
	// its format and job. It is made of values the service computed, never of user text.
	Detail string
}

// ProductionView is what Production prints.
type ProductionView struct {
	ID        string
	Name      string
	State     string
	HasBrief  bool
	HasScript bool
	Timelines int
	Outputs   int
	Pending   int
	Published int
	// Brief is the Markdown of the brief, cut to what the budget leaves.
	Brief string
	Lines []ProductionLine
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}

// Production renders the description of a production (SDD-003 section 2.3.1): the name and state, one line of
// production state, the brief, then the items. maxTokens is the budget of the whole text; items are listed in
// order and the list is cut with a line that says how many are left, and the brief takes what remains, so the
// text never exceeds the budget by more than one line.
func Production(v *ProductionView, maxTokens int) string {
	limit := maxTokens * CharsPerToken

	var head strings.Builder

	head.WriteString(Note + "\n")
	fmt.Fprintf(&head, "project %s %s %s\n", v.ID, Quote(v.Name, 60), v.State)
	fmt.Fprintf(&head, "production: brief %s, script %s, timelines %d, outputs %d, pending approvals %d, published %d",
		yesNo(v.HasBrief), yesNo(v.HasScript), v.Timelines, v.Outputs, v.Pending, v.Published)

	var items strings.Builder

	shown := 0

	// Reserve a third of the budget for the brief, so that a long list of items cannot hide it.
	itemBudget := limit - runeLen(head.String()) - limit/3

	for i := range v.Lines {
		l := &v.Lines[i]

		line := fmt.Sprintf("\n%s %s %s", l.ID, l.Kind, Quote(l.Title, 60))
		if l.Trashed {
			line += " trashed"
		}

		if l.Detail != "" {
			line += " " + l.Detail
		}

		if shown > 0 && runeLen(items.String())+runeLen(line) > itemBudget {
			break
		}

		items.WriteString(line)

		shown++
	}

	var b strings.Builder

	b.WriteString(head.String())

	if v.Brief != "" {
		b.WriteString("\nbrief:\n")

		room := max(limit-runeLen(head.String())-runeLen(items.String())-120, 200)
		attrs := untrusted.Attrs{"project": v.ID, "item": "brief"}
		block := untrusted.Block(untrusted.SourceProjectText, attrs, v.Brief, room)
		b.WriteString(block)

		if strings.Contains(block, `truncated="true"`) {
			b.WriteString("\nthe brief is cut; read all of it with brief")
		}
	}

	b.WriteString("\nitems:")
	b.WriteString(items.String())

	if shown < len(v.Lines) {
		fmt.Fprintf(&b, "\n… %d more items; read them with project_get", len(v.Lines)-shown)
	}

	return b.String()
}
