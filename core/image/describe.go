package image

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
)

// The budget of the text view (SDD-021 section 9): the whole text, and what the findings keep of it.
const (
	DescribeBudget   = 4000
	DescribeFindings = 1000
	maxQuoted        = 60 // the characters of a name or a text kept on one line of a view
)

// oneLine makes untrusted text safe for one line of the text view: control characters and line breaks become
// spaces and a long text is cut with an ellipsis.
func oneLine(s string, limit int) string {
	r := []rune(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return ' '
		}

		return c
	}, s))

	if len(r) > limit {
		return string(r[:limit]) + "..."
	}

	return string(r)
}

// quote is the compact form of an untrusted name or text on one line of a view (SDD-007 section 8.2): line breaks
// become spaces, then it is quoted, cleaned, escaped and cut as the diff of a timeline does it.
func quote(s string) string { return command.Compact(oneLine(s, len(s)), maxQuoted) }

// Describe is the text view of an image document: a header, the tree of layers one line each with the top layer
// first, then the findings of the lint, inside DescribeBudget characters. A tree that does not fit is cut at a
// line and says how many layers it left out; the findings keep at least DescribeFindings characters. The names
// and the texts of the document are untrusted: they are quoted in the compact form and the view starts with the
// note that says so.
func Describe(d *Document, revision int64, findings []lint.Finding) string {
	head := fmt.Sprintf("%s\nimage %s r%d %s %dx%d background %s template=%t layers=%d", command.UntrustedNote,
		d.ID, revision, quote(d.Name), d.Width, d.Height, d.Background, d.Template, Count(d))

	var fl []string

	for _, f := range findings {
		fl = append(fl, fmt.Sprintf("%s %s %s %s", f.Severity, f.Code, f.Item, oneLine(f.Message, 200)))
	}

	findingText := ""
	if len(fl) > 0 {
		findingText = "findings:\n" + strings.Join(fl, "\n")
	}

	if len(findingText) > DescribeFindings && len(head)+len(findingText) > DescribeBudget {
		findingText = cutLines(
			findingText,
			max(DescribeFindings, DescribeBudget-len(head)-DescribeFindings),
			"findings",
		)
	}

	var lines []string

	describeLayers(d.Layers, 0, &lines)

	room := DescribeBudget - len(head) - len(findingText) - 2
	body, left := fit(lines, room)

	out := head
	if body != "" {
		out += "\n" + body
	}

	if left > 0 {
		out += fmt.Sprintf("\n... and %d more layers", left)
	}

	if findingText != "" {
		out += "\n" + findingText
	}

	return out
}

// describeLayers appends one line per layer, the top layer first, children indented under their group.
func describeLayers(ls []Layer, depth int, out *[]string) {
	for i := len(ls) - 1; i >= 0; i-- {
		l := &ls[i]
		*out = append(*out, strings.Repeat("  ", depth)+describeLayer(l))

		if l.Kind == KindGroup {
			describeLayers(l.Children, depth+1, out)
		}
	}
}

func describeLayer(l *Layer) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s %s", l.ID, l.Kind, quote(l.Name))

	if l.Kind == KindGroup {
		fmt.Fprintf(&b, " offset %d,%d children %d", l.X, l.Y, len(l.Children))
	} else {
		fmt.Fprintf(&b, " at %d,%d %dx%d", l.X, l.Y, l.W, l.H)

		if l.Rotation != 0 {
			fmt.Fprintf(&b, " rotation %d", l.Rotation)
		}
	}

	fmt.Fprintf(&b, " opacity %d", l.Opacity)

	if !l.Visible {
		b.WriteString(" hidden")
	}

	if l.Locked {
		b.WriteString(" locked")
	}

	if l.Slot != "" {
		fmt.Fprintf(&b, " slot=%s", l.Slot)
	}

	b.WriteString(describeLook(l))

	switch l.Kind {
	case KindImage:
		if l.Asset == "" {
			b.WriteString(" no file")
		} else {
			fmt.Fprintf(&b, " asset %.19s", l.Asset)
		}

		if l.Crop != nil {
			fmt.Fprintf(&b, " crop %d,%d %dx%d", l.Crop.X, l.Crop.Y, l.Crop.W, l.Crop.H)
		}
	case KindText:
		fmt.Fprintf(&b, " %s", quote(l.Text))

		if l.Style != nil && l.Style.Size != nil {
			fmt.Fprintf(&b, " size %g", *l.Style.Size)
		}
	case KindShape:
		fmt.Fprintf(&b, " %s fill %s", l.Shape, l.Fill)
	case KindFill:
		fmt.Fprintf(&b, " fill %s", l.Fill)
	}

	return b.String()
}

// fit joins as many lines as fit in room characters and says how many were left out.
func fit(lines []string, room int) (string, int) {
	used, n := 0, 0

	for _, l := range lines {
		if used+len(l)+1 > room && n > 0 || room <= 0 {
			break
		}

		used += len(l) + 1
		n++
	}

	return strings.Join(lines[:n], "\n"), len(lines) - n
}

func cutLines(text string, room int, what string) string {
	lines := strings.Split(text, "\n")
	kept, left := fit(lines[1:], room)

	out := lines[0]
	if kept != "" {
		out += "\n" + kept
	}

	if left > 0 {
		out += fmt.Sprintf("\n... and %d more %s", left, what)
	}

	return out
}
