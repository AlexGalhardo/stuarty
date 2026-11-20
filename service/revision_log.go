package service

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// maxRebuildSteps is how many revisions back a document is rebuilt from the log (SDD-005 section 12.4).
const maxRebuildSteps = 5000

// logRow is a row of the log of a document as the rebuild of an earlier revision reads it.
type logRow struct {
	Kind      OperationKind
	Inverse   json.RawMessage
	DocSHA256 string
}

// revisionLog is a document with its log, as the rebuild of an earlier revision sees them. A timeline and an
// image have the same walk, ceilings and checks (SDD-005 section 12.4, SDD-021 section 4), so the walk is written
// once and each kind of document says only how to read a row, how to apply the inverses and what a canonical
// document is.
type revisionLog[D any] struct {
	noun      string // "timeline" or "image", for the words of an error
	idKey     string // the name of the id of the document in the details of an error: project_id or image_id
	id        string
	current   D
	revision  int64 // the current revision
	canonical func(D) []byte
	rebuild   func(current D, inverses [][]command.Primitive) (D, error)
	row       func(ctx context.Context, revision int64) (logRow, error)
	// blocks says what a row of the kind is when the log cannot go back across it (the document changed shape
	// there, so its inverse does not describe a step back); "" when it can.
	blocks func(kind OperationKind) string
}

// rebuildAt is the document at a revision, rebuilt from the log by walking back from the current one. path
// names the member of the request that held the revision. A rebuilt document that does not match the digest of
// its row is E_INTERNAL: the log and the document disagree, which is a defect, never something to continue from.
func rebuildAt[D any](ctx context.Context, l *revisionLog[D], target int64, path string) (D, error) {
	var none D

	if target < 1 || target > l.revision {
		return none, newError(command.CodeValidation, "revision %d does not exist; the %s is at revision %d.",
			target, l.noun, l.revision).withPath(path).withHint("use a revision from 1 to %d", l.revision)
	}

	if target == l.revision {
		return l.current, nil
	}

	if l.revision-target > maxRebuildSteps {
		return none, newError(command.CodeLimit, "revision %d is more than %d revisions behind revision %d.",
			target, maxRebuildSteps, l.revision).withPath(path).
			withHint("go back in steps, through a checkpoint nearer to the current revision")
	}

	inverses := make([][]command.Primitive, 0, l.revision-target)

	for rev := l.revision; rev > target; rev-- {
		row, err := l.row(ctx, rev)
		if err != nil {
			return none, err
		}

		if what := l.blocks(row.Kind); what != "" {
			return none, newError(command.CodeInvalidState,
				"revision %d is before %s at revision %d, which the log cannot go back across.", target, what, rev).
				withPath(path).withHint("use a revision from %d on", rev)
		}

		var prims []command.Primitive
		if err := json.Unmarshal(row.Inverse, &prims); err != nil {
			return none, internalError()
		}

		inverses = append(inverses, prims)
	}

	doc, err := l.rebuild(l.current, inverses)
	if err != nil {
		return none, l.failed(target)
	}

	want, err := l.row(ctx, target)
	if err != nil {
		return none, err
	}

	if got := sha256Hex(l.canonical(doc)); !bytes.Equal([]byte(got), []byte(want.DocSHA256)) {
		return none, l.failed(target)
	}

	return doc, nil
}

// failed is the answer when the log and the document disagree: E_INTERNAL with the revisions, so that the report
// can be traced.
func (l *revisionLog[D]) failed(target int64) error {
	return internalError().withDetail(l.idKey, l.id).withDetail("revision", target).
		withDetail("current_revision", l.revision)
}
