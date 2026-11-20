package image

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// The nouns of an image DiffEntry; the verbs are those of core/command.
const (
	NounLayer    = "layer"
	NounAsset    = "asset"
	NounDocument = "document"
)

// Diff is the change that turns document a into document b, in the form of a batch diff. It reads the two
// documents and never a log, so it is the same whatever sequence of edits, undos and redos led to them.
func Diff(a, b *Document) []command.Change {
	diff, _ := Compare(a, b)

	return diff
}

// moveFields are the fields of a layer that only say where it is.
var moveFields = []string{"x", "y", "order", "parent"}

// DiffEntries is Diff classified and sorted, most significant first (SDD-021 section 4): a layer added or
// removed, a change of the canvas or of the name, a layer moved, any other change of a layer, then assets. Ties
// are ordered by kind and identifier. Each entry has the line an agent reads.
func DiffEntries(a, b *Document) []command.DiffEntry {
	changes := Diff(a, b)
	out := make([]command.DiffEntry, 0, len(changes))

	for _, ch := range changes {
		verb, noun, weight := classify(ch)
		out = append(out, command.DiffEntry{
			Verb: verb, Noun: noun, Change: ch, Weight: weight, Line: entryLine(b, ch, verb),
		})
	}

	slices.SortStableFunc(out, func(x, y command.DiffEntry) int {
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

func classify(ch command.Change) (verb, noun string, weight int) {
	switch ch.Kind {
	case KindLayerAdded:
		return command.VerbAdded, NounLayer, 100
	case KindLayerRemoved:
		return command.VerbRemoved, NounLayer, 100
	case KindDocumentUpdated:
		return command.VerbChanged, NounDocument, 90
	case KindLayerUpdated:
		for k := range ch.Changes {
			if !slices.Contains(moveFields, k) {
				return command.VerbChanged, NounLayer, 50
			}
		}

		return command.VerbMoved, NounLayer, 60
	case KindAssetAdded:
		return command.VerbAdded, NounAsset, 10
	default:
		return command.VerbRemoved, NounAsset, 10
	}
}

// entryLine is the DescribeDiff line of a change with its leading word replaced by the verb: a layer whose
// place and size changed reads "moved ..." and the canvas "changed document: ...". An asset is a file the
// document holds, named by its hash and its untrusted name.
func entryLine(doc *Document, ch command.Change, verb string) string {
	if ch.Kind == KindAssetAdded || ch.Kind == KindAssetRemoved {
		kind := str(ch.Object["kind"])
		if kind == "" {
			kind = AssetImage
		}

		line := fmt.Sprintf("%s asset %s %s %s", verb, ch.ID, quote(str(ch.Object["name"])), kind)
		if w, h := num(ch.Object["width"]), num(ch.Object["height"]); w != 0 && h != 0 {
			line += fmt.Sprintf(" %vx%v", w, h)
		}

		return line
	}

	line := DescribeDiff(doc, []command.Change{ch})

	switch ch.Kind {
	case KindDocumentUpdated:
		return verb + " " + line
	case KindLayerUpdated:
		_, rest, _ := strings.Cut(line, " ")

		return verb + " " + rest
	}

	return line
}
