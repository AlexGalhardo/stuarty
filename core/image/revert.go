package image

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Rebuild returns the document that came before a run of revisions (SDD-005 section 12.4). current is the
// document after them and inverses holds the stored inverse of each revision, newest first. The lists are
// applied one after the other on one working copy, with no guard, and the result is checked once. A primitive
// that does not apply means the log and the document disagree, which is an error, never a partial document.
func Rebuild(current *Document, inverses [][]command.Primitive) (*Document, error) {
	var prims []command.Primitive

	for _, inv := range inverses {
		prims = append(prims, slices.Clone(inv)...)
	}

	res, skipped := ApplyPrimitives(current, prims, nil)
	if len(skipped) > 0 {
		s := skipped[0]

		return nil, fmt.Errorf("primitive %d (%s %s %s) was skipped: %s", s.PrimitiveIndex, s.Primitive.Type,
			s.Primitive.Collection, s.Primitive.ID, s.Reason)
	}

	return res.Doc, nil
}

// Revert is the batch that turns the current document into target (SDD-021 section 4): Doc is target, Diff the
// net change and Inverse the primitives that turn target back into current, so the batch is undone like any
// other. The inputs are not modified. A revert to a document equal to the current one is a no-op.
func Revert(current, target *Document) BatchResult {
	work := Clone(target)
	diff, inverse := Compare(current, work)

	return BatchResult{Doc: work, Diff: diff, Inverse: inverse}
}
