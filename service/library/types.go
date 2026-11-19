// Package library keeps the memory, the skills and the prompts of a workspace
// as Markdown files under library/ and an index of them for search (ADR-0040,
// SDD-018). The files are the truth: the index is rebuilt from them at any
// time. The pure rules are in core/library; this package reads and writes the
// files, scans them, imports and exports skills, and searches.
package library

import (
	"context"
	"errors"
	"fmt"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// Entry is one row of the index: what the lists show, with no body.
type Entry struct {
	Key          string // "<kind>/<name>"
	Kind         core.Kind
	Name         string
	Description  string
	Scope        string // memory: workspace or a project id; empty for the others
	Origin       string // skills
	Reviewed     bool   // skills: the review mark holds for the content now
	Kept         bool   // memory: a person kept it
	Actor        string
	Tags         []string
	LintErrors   int // skills: findings of the lint by severity
	LintWarnings int
	HasScripts   bool   // skills: the folder holds a script, stored inert
	State        string // StateOK or StateInvalid
	Error        string // why a file is invalid
	Path         string // relative to library/, slash separated
	Sig          string // what the scan compares: file count, bytes and latest modification
	CreatedMs    int64
	UpdatedMs    int64
}

// The states of an entry.
const (
	StateOK      = "ok"
	StateInvalid = "invalid"
)

// Index is the repository of the index (SDD-018 section 9). Its content can be
// thrown away: Reindex fills it again from the files.
type Index interface {
	// All returns every row.
	All(ctx context.Context) ([]Entry, error)
	// Put inserts or replaces the row of e.Key and its searchable text.
	Put(ctx context.Context, e Entry, body string) error
	// Delete removes the row and the text of a key; a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// Clear removes every row.
	Clear(ctx context.Context) error
	// Search returns the keys whose name, description, tags or body have a word
	// that starts with each of the terms (all of them), best match first, at most
	// limit keys.
	Search(ctx context.Context, terms []string, limit int) ([]string, error)
}

// The errors of the package. The service turns them into its own.
var (
	ErrNotFound  = errors.New("library: not found")
	ErrExists    = errors.New("library: the name is taken")
	ErrForbidden = errors.New("library: not allowed for this caller")
	// ErrChanged is a review of a skill whose content is no longer the one the person read.
	ErrChanged = errors.New("library: the skill changed since it was read")
)

// ValidationError is a refusal of the content: a field that breaks a rule, an
// archive entry that is not allowed, a lint error.
type ValidationError struct {
	Field    string
	Msg      string
	Findings []core.Finding
	Details  map[string]any
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Msg
	}

	return e.Msg
}

// LimitError is a refusal because a ceiling is reached.
type LimitError struct{ Msg string }

func (e *LimitError) Error() string { return e.Msg }

func invalid(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// fromCore turns an error of core/library into a ValidationError when it names a field.
func fromCore(err error) error {
	if err == nil {
		return nil
	}

	if fe, ok := errors.AsType[*core.FieldError](err); ok {
		return &ValidationError{Field: fe.Field, Msg: fe.Msg}
	}

	if fm, ok := errors.AsType[*core.FormatError](err); ok {
		return &ValidationError{Field: "front_matter", Msg: fm.Error()}
	}

	return err
}
