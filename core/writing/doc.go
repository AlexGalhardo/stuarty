// Package writing holds the pure rules of the texts of a production (SDD-019, ADR-0031): the kinds of text, the
// structure of a script and the estimate of its duration, the variants of a caption and the table of limits of each
// network, the rules of revisions, and the difference between two bodies. It does no I/O: the service stores the
// text and its revisions and calls these functions, and no surface computes a count of its own.
package writing

import (
	"fmt"
	"slices"
)

// Kind is the kind of a text item (SDD-019 section 2).
type Kind string

// The kinds of text.
const (
	KindScript  Kind = "script"
	KindCopy    Kind = "copy"
	KindCaption Kind = "caption"
	KindPost    Kind = "post"
)

// Kinds lists the kinds in the order a list shows them.
func Kinds() []Kind { return []Kind{KindScript, KindCopy, KindCaption, KindPost} }

// Valid reports whether k is a kind of text.
func (k Kind) Valid() bool { return slices.Contains(Kinds(), k) }

// DefaultKind is the kind of a text made without one.
const DefaultKind = KindCopy

// Limits of SDD-019 section 9.
const (
	// MaxRevisions is how many revisions a text keeps; the 501st drops the oldest.
	MaxRevisions = 500
	// MaxInstruction is the longest instruction of a draft, in characters.
	MaxInstruction = 4000
)

// RuleError is a rule broken. Code is a code of SDD-000 section 8 (E_VALIDATION, E_REVISION_CONFLICT); the service
// turns it into its error.
type RuleError struct {
	Code    string
	Path    string
	Message string
	Hint    string
}

func (e *RuleError) Error() string { return e.Message }

func validation(path, hint, format string, args ...any) *RuleError {
	return &RuleError{Code: "E_VALIDATION", Path: path, Hint: hint, Message: fmt.Sprintf(format, args...)}
}

// CheckKind is the rule of the kind of a text; an empty kind is the default.
func CheckKind(kind string) (Kind, *RuleError) {
	if kind == "" {
		return DefaultKind, nil
	}

	if k := Kind(kind); k.Valid() {
		return k, nil
	}

	return "", validation("text_kind", "use script, copy, caption or post", "the text kind %q is not known", kind)
}
