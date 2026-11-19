package command

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The verbs of a DiffEntry (SDD-002 A.5, "Diff of two documents").
const (
	VerbAdded   = "added"
	VerbRemoved = "removed"
	VerbMoved   = "moved"
	VerbTrimmed = "trimmed"
	VerbChanged = "changed"
)

// DiffEntry is one change between two documents, classified for people and agents.
type DiffEntry struct {
	Verb   string `json:"verb"`
	Noun   string `json:"noun"` // track, an item type (clip, audio, text, caption, shape), asset, font, settings or project
	Change Change `json:"change"`
	Line   string `json:"line"` // the DescribeDiff line of the change
	Weight int    `json:"-"`    // significance; larger first
}

// Diff is the change that turns document a into document b, in the form of a batch diff
// (SDD-002 A.5). It reads the two documents and never a log, so it is the same whatever
// sequence of edits, undos and redos led to them. Equal documents give no change.
func Diff(a, b *timeline.Document) []Change {
	return diffOf(fullPair(a, b))
}

// DiffEntries is Diff classified and sorted, most significant first (SDD-002 A.5): a track
// added or removed, an item added or removed, a change of the settings or of the project, an
// item trimmed, an item moved, any other change of an item or a track, then assets and fonts.
// Ties are ordered by kind and identifier.
func DiffEntries(a, b *timeline.Document) []DiffEntry {
	changes := Diff(a, b)
	out := make([]DiffEntry, 0, len(changes))

	for _, ch := range changes {
		verb, noun, weight := classify(ch, a, b)
		out = append(out, DiffEntry{
			Verb: verb, Noun: noun, Change: ch, Weight: weight, Line: lineOf(b, ch, verb),
		})
	}

	slices.SortStableFunc(out, func(x, y DiffEntry) int {
		if x.Weight != y.Weight {
			return y.Weight - x.Weight
		}

		if c := strings.Compare(x.Change.Kind, y.Change.Kind); c != 0 {
			return c
		}

		return strings.Compare(x.Change.ID, y.Change.ID)
	})

	return out
}

// lineOf is the DescribeDiff line of one change with its leading mark replaced by the verb:
// "~ item itm_a: start 0s -> 2s" reads "moved item itm_a: start 0s -> 2s".
func lineOf(doc *timeline.Document, ch Change, verb string) string {
	line := DescribeDiff(doc, []Change{ch})
	if len(line) > 2 && strings.ContainsAny(line[:1], "+-~") && line[1] == ' ' {
		line = line[2:]
	}

	return verb + " " + line
}

func classify(ch Change, a, b *timeline.Document) (verb, noun string, weight int) {
	scope, kind, _ := strings.Cut(ch.Kind, ".")

	switch scope {
	case "track":
		noun = "track"
	case "asset":
		noun = "asset"
	case "font":
		noun = "font"
	case "settings":
		noun = "settings"
	case "project":
		noun = "project"
	case "item":
		noun = itemNoun(ch.ID, a, b)
	}

	switch kind {
	case "added":
		verb = VerbAdded
	case "removed":
		verb = VerbRemoved
	default:
		verb = VerbChanged
		if scope == "item" {
			verb = updatedVerb(ch.Changes)
		}
	}

	return verb, noun, weightOf(scope, verb)
}

func itemNoun(id string, a, b *timeline.Document) string {
	if it, ok := b.Items[id]; ok {
		return string(it.Type)
	}

	if it, ok := a.Items[id]; ok {
		return string(it.Type)
	}

	return "clip"
}

// updatedVerb: trimmed when the source range or the length changed, moved when only the place
// did, changed for any other property.
func updatedVerb(changes map[string]Pair) string {
	if _, ok := changes["duration"]; ok {
		return VerbTrimmed
	}

	if _, ok := changes["source_in"]; ok {
		return VerbTrimmed
	}

	_, start := changes["start"]
	_, track := changes["track"]

	if (start || track) && len(changes) == countKeys(changes, "start", "track") {
		return VerbMoved
	}

	return VerbChanged
}

func countKeys(m map[string]Pair, keys ...string) int {
	n := 0

	for _, k := range keys {
		if _, ok := m[k]; ok {
			n++
		}
	}

	return n
}

func weightOf(scope, verb string) int {
	whole := verb == VerbAdded || verb == VerbRemoved

	switch {
	case scope == "track" && whole:
		return 100
	case scope == "item" && whole:
		return 90
	case scope == "settings" || scope == "project":
		return 80
	case scope == "item" && verb == VerbTrimmed:
		return 70
	case scope == "item" && verb == VerbMoved:
		return 60
	case scope == "item" || scope == "track":
		return 50
	}

	return 30 // assets and fonts
}
