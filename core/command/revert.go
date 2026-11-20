package command

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Revert is the batch that turns the current document into target (SDD-002 A.6, "Revert"). Doc
// is target, Diff is Diff(current, target) and Inverse the primitives that turn target back
// into current, so the batch is undone like any other. The inputs are not modified. A revert
// to a document equal to the current one is a no-op (BatchResult.NoOp).
func Revert(current, target *timeline.Document) BatchResult {
	work := timeline.Clone(target)
	before, after := rawPair(current, work)

	return BatchResult{
		Doc:     work,
		Inverse: inverseOf(before, after),
		Diff:    diffOf(fullPair(current, work)),
	}
}

// RebuildAt returns the document that came before a run of revisions. current is the document
// after them and inverses holds the stored inverse of each revision, newest first. Each list is
// applied in reverse order, one after the other, on one working copy with no guard and no
// validation in between; the result is validated once. A primitive that does not apply means
// the log and the document disagree, which is an error, never a partial document.
func RebuildAt(current *timeline.Document, inverses [][]Primitive) (*timeline.Document, error) {
	var prims []Primitive

	for _, inv := range inverses {
		prims = append(prims, reversePrimitives(inv)...)
	}

	res, skipped := ApplyPrimitives(current, prims, nil)
	if len(skipped) > 0 {
		s := skipped[0]

		return nil, fmt.Errorf("primitive %d (%s %s %s) was skipped: %s", s.PrimitiveIndex, s.Primitive.Type,
			s.Primitive.Collection, s.Primitive.ID, s.Reason)
	}

	return res.Doc, nil
}

func reversePrimitives(prims []Primitive) []Primitive {
	out := slices.Clone(prims)
	slices.Reverse(out)

	return out
}
