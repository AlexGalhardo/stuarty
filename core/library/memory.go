package library

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Memory is one note: one fact in one file (SDD-018 section 5).
type Memory struct {
	Name        string
	Description string
	Scope       string // "workspace" or a project id
	Tags        []string
	Actor       string // who wrote it; set by the service
	Kept        bool   // a person kept it
	Created     string // RFC 3339, UTC
	Updated     string
	Body        string

	fm *FrontMatter
}

// ParseMemory reads the text of a note file. It checks the shape of the file
// and not the rules of the fields: Validate does that.
func ParseMemory(text string) (Memory, error) {
	fm, body, err := ParseDocument(text)
	if err != nil {
		return Memory{}, err
	}

	scope := fm.Scalar("scope")
	if scope == "" {
		scope = "workspace"
	}

	return Memory{
		Name: fm.Scalar("name"), Description: fm.Scalar("description"), Scope: scope, Tags: fm.List("tags"),
		Actor: fm.Scalar("actor"), Kept: fm.Scalar("kept") == "true",
		Created: fm.Scalar("created"), Updated: fm.Scalar("updated"), Body: body, fm: fm,
	}, nil
}

// Validate applies the rules of a note.
func (m *Memory) Validate() error {
	if err := ValidName(m.Name); err != nil {
		return fieldError("name", err)
	}

	if err := checkLine("description", m.Description, MaxDescription, true); err != nil {
		return err
	}

	if err := ValidScope(m.Scope); err != nil {
		return fieldError("scope", err)
	}

	if err := ValidTags(m.Tags); err != nil {
		return fieldError("tags", err)
	}

	if len(m.Body) > MaxMemoryBody {
		return fieldError("body", fmt.Errorf("the body is longer than %d bytes", MaxMemoryBody))
	}

	if !utf8.ValidString(m.Body) {
		return fieldError("body", errors.New("the text is not valid UTF-8"))
	}

	for _, t := range []struct{ name, v string }{{"created", m.Created}, {"updated", m.Updated}} {
		if t.v == "" {
			continue
		}

		if _, err := time.Parse(time.RFC3339, t.v); err != nil {
			return fieldError(t.name, errors.New("the time must be RFC 3339"))
		}
	}

	return nil
}

// Marshal writes the file of the note: the keys in the order of the table of
// SDD-018 section 5, then any key the file had that the product does not know.
func (m Memory) Marshal() (string, error) {
	fm := baseFrontMatter(m.fm)
	fm.SetScalar("name", m.Name)
	fm.SetScalar("description", m.Description)
	fm.SetScalar("scope", m.Scope)
	fm.SetList("tags", m.Tags)
	fm.SetScalar("actor", m.Actor)
	fm.SetBool("kept", m.Kept)
	fm.SetScalar("created", m.Created)
	fm.SetScalar("updated", m.Updated)

	return Compose(fm, m.Body)
}

// baseFrontMatter starts from the original keys, so unknown ones survive a rewrite.
func baseFrontMatter(orig *FrontMatter) *FrontMatter {
	if orig == nil {
		return NewFrontMatter()
	}

	return orig.Clone()
}

func fieldError(field string, err error) error {
	return &FieldError{Field: field, Msg: err.Error()}
}

// FieldError names the field that broke a rule.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

// checkLine checks a one line text field.
func checkLine(field, v string, limit int, required bool) error {
	if strings.TrimSpace(v) == "" {
		if required {
			return fieldError(field, errors.New("it is required"))
		}

		return nil
	}

	if strings.ContainsAny(v, "\n\r") {
		return fieldError(field, errors.New("it must be one line"))
	}

	if utf8.RuneCountInString(v) > limit {
		return fieldError(field, fmt.Errorf("it is longer than %d characters", limit))
	}

	if !utf8.ValidString(v) {
		return fieldError(field, errors.New("the text is not valid UTF-8"))
	}

	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return fieldError(field, errors.New("it has a control character"))
		}
	}

	return nil
}
