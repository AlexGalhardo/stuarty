package image

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// DescribeDiff is a diff as sentences an agent reads, one line per changed object (SDD-003 section 2.4 rule 6):
// what was added, what changed field by field, what was removed. The order keys are not shown; a layer whose
// only change is its place is said to have moved.
func DescribeDiff(doc *Document, diff []command.Change) string {
	if len(diff) == 0 {
		return "nothing changed"
	}

	var lines []string

	for _, ch := range diff {
		switch ch.Kind {
		case KindDocumentUpdated:
			lines = append(lines, "document: "+pairs(ch.Changes, nil))
		case KindLayerAdded:
			lines = append(
				lines,
				fmt.Sprintf("added %s %s %s at %v,%v %vx%v", str(ch.Object["kind"]),
					quote(str(ch.Object["name"]), maxLineName),
					ch.ID, num(ch.Object["x"]), num(ch.Object["y"]), num(ch.Object["w"]), num(ch.Object["h"])),
			)
		case KindLayerRemoved:
			lines = append(
				lines,
				fmt.Sprintf("removed %s %s %s", str(ch.Object["kind"]), quote(str(ch.Object["name"]), maxLineName), ch.ID),
			)
		case KindLayerUpdated:
			name := ch.ID

			if l, ok := Find(doc, ch.ID); ok {
				name = quote(l.Name, maxLineName) + " " + ch.ID
			}

			if text := pairs(ch.Changes, []string{"order"}); text != "" {
				lines = append(lines, "changed "+name+": "+text)
			} else {
				lines = append(lines, "moved "+name)
			}
		}
	}

	if len(lines) == 0 {
		return "nothing changed"
	}

	return strings.Join(lines, "\n")
}

// pairs lists the changed fields as "field before -> after", sorted, leaving out the fields in skip.
func pairs(changes map[string]command.Pair, skip []string) string {
	keys := make([]string, 0, len(changes))

	for k := range changes {
		if !slices.Contains(skip, k) {
			keys = append(keys, k)
		}
	}

	slices.Sort(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		p := changes[k]
		parts[i] = fmt.Sprintf("%s %s -> %s", k, show(p[0]), show(p[1]))
	}

	return strings.Join(parts, ", ")
}

func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "none"
	case string:
		return quote(x, maxLineText) // a name, a text or a slot: untrusted
	case map[string]any:
		return "(set)"
	default:
		return fmt.Sprint(x)
	}
}

func str(v any) string {
	s, _ := v.(string)

	return s
}

func num(v any) any {
	if v == nil {
		return 0
	}

	return v
}
