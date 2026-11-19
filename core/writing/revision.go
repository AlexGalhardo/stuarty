package writing

import "fmt"

// Source is where a revision came from (SDD-019 section 3).
type Source string

// The sources of a revision.
const (
	SourceCreated   Source = "created"
	SourceWrite     Source = "write"
	SourceGenerated Source = "generated"
	SourceRevert    Source = "revert"
)

// Valid reports whether s is a source of a revision.
func (s Source) Valid() bool {
	switch s {
	case SourceCreated, SourceWrite, SourceGenerated, SourceRevert:
		return true
	}

	return false
}

// CheckBase is the rule of base_revision: when a writer says which revision it read, that must still be the
// latest one. A nil base is "no check".
func CheckBase(latest int, base *int) *RuleError {
	if base == nil || *base == latest {
		return nil
	}

	return &RuleError{
		Code:    "E_REVISION_CONFLICT",
		Path:    "base_revision",
		Hint:    "read the text again and write on top of its latest revision",
		Message: fmt.Sprintf("the text is at revision %d, not at the revision %d the write was made on", latest, *base),
	}
}

// NextRevision is the number of the revision a write makes after latest.
func NextRevision(latest int) int { return latest + 1 }

// PruneBefore is the number below which revisions are dropped once revision n exists: a text keeps at most
// MaxRevisions, so the 501st drops the first. Numbers are never reused.
func PruneBefore(n int) int { return max(0, n-MaxRevisions+1) }

// Unchanged reports whether writing body on top of a text whose latest body is latest changes nothing: such a write
// makes no revision.
func Unchanged(latest, body string) bool { return latest == body }

// CheckRevision is the rule of the number of a revision a request names: 1 or more.
func CheckRevision(n int) *RuleError {
	if n < 1 {
		return validation("revision", "send a revision number from 1", "the revision %d is below 1", n)
	}

	return nil
}
