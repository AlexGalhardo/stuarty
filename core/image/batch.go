package image

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// BatchResult is the outcome of a batch: the new document, what each operation applied, and the net change.
type BatchResult struct {
	Doc       *Document
	Effective []map[string]any // one entry per operation, in order
	Inverse   []command.Primitive
	Diff      []command.Change
}

// NoOp reports whether the batch left the document as it was: nothing to commit (VV-050).
func (b BatchResult) NoOp() bool { return len(b.Diff) == 0 && len(b.Inverse) == 0 }

// ApplyBatch applies the operations in order, each to the document the previous one produced, on one private
// copy. If one fails the whole batch fails with that error and op_index set, and doc is untouched. It is pure:
// the same document, operations and environment give the same result.
func ApplyBatch(doc *Document, ops []command.Operation, env Env) (BatchResult, error) {
	if len(ops) == 0 {
		return BatchResult{}, command.Errorf(command.CodeValidation, "The batch has no operations.").
			WithPath("ops").WithHint("Send at least one operation.")
	}

	reg := Registry()
	work := Clone(doc)
	cenv := env.command()
	effective := make([]map[string]any, 0, len(ops))

	for i, op := range ops {
		eff, err := reg.ApplyIn(work, op, cenv)
		if err == nil {
			ReorderAll(work)
			err = firstViolation(op.Op, work)
		}

		if err != nil {
			e := *command.AsError(err)
			idx := i
			e.OpIndex = &idx

			return BatchResult{}, &e
		}

		effective = append(effective, eff)
	}

	res := BatchResult{Doc: work, Effective: effective}
	res.Diff, res.Inverse = Compare(doc, work)

	return res, nil
}

// firstViolation turns the first way in which an operation left the document invalid into the error of the
// batch: the commands check what they can name, and this catches the rest (a coordinate out of range, a depth).
func firstViolation(op string, d *Document) error {
	vs := Validate(d)
	if len(vs) == 0 {
		return nil
	}

	v := vs[0]
	code := command.CodeValidation

	if v.Code == "limit" || v.Code == "depth" {
		code = command.CodeLimit
	}

	e := command.Errorf(code, "The result of %s would be invalid: %s.", op, v.Message).
		WithHint("Change the arguments so that the layer stays inside the limits of the document.").
		WithPath(v.Path)
	if v.Layer != "" {
		e = e.WithDetail("layer", v.Layer)
	}

	return e.WithDetail("violations", len(vs))
}

// String names a violation for logs.
func (v Violation) String() string { return fmt.Sprintf("%s %s: %s", v.Code, v.Path, v.Message) }
