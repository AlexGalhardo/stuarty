package library

import (
	"context"
	"errors"
	"fmt"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// SkillInput is a skill as a caller writes it. The origin, the source and the
// review are not the caller's: the product records them (SDD-018 section 7).
type SkillInput struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	AllowedTools  string
	// Metadata keys of the product (stuarty-*) are dropped. Nil keeps the metadata of the skill as it is; an
	// empty map removes it.
	Metadata map[string]string
	Body     string
	// Files are the other files of the folder. Nil keeps the files of the skill as they are;
	// an empty list removes them.
	Files []core.File
}

// SaveSkill writes a skill. It is checked by the lint first: an error finding
// refuses it. A person at the browser has read what they wrote, so the skill is
// reviewed; for any other caller it is not.
func (m *Manager) SaveSkill(ctx context.Context, c Caller, in SkillInput) (core.Skill, Entry, []core.Finding, error) {
	if c.ProjectID != "" {
		return core.Skill{}, Entry{}, nil, fmt.Errorf(
			"%w: a key limited to a project does not write skills",
			ErrForbidden,
		)
	}

	if err := core.ValidName(in.Name); err != nil {
		return core.Skill{}, Entry{}, nil, invalid("name", "%v", err)
	}

	rows, err := m.refresh(ctx)
	if err != nil {
		return core.Skill{}, Entry{}, nil, err
	}

	skill := core.Skill{}
	existed := false

	if old, _, err := m.ReadSkill(ctx, in.Name); err == nil {
		skill, existed = old, true
	} else if !isNotFoundOrInvalid(err) {
		return core.Skill{}, Entry{}, nil, err
	}

	if !existed {
		if n := countKind(rows, core.KindSkill); n >= core.MaxSkills {
			return core.Skill{}, Entry{}, nil, &LimitError{
				Msg: fmt.Sprintf("the workspace already has %d skills, the most it keeps", n),
			}
		}
	}

	files := skill.Files
	if in.Files != nil {
		files = in.Files
	}

	for _, f := range files {
		if err := core.ValidFilePath(f.Path); err != nil {
			return core.Skill{}, Entry{}, nil, invalid("files", "%q: %v", f.Path, err)
		}

		if f.Path == core.SkillFile {
			return core.Skill{}, Entry{}, nil, invalid("files", "%s is written from the other fields", core.SkillFile)
		}
	}

	// Nil metadata keeps what the skill has, as nil files do: a caller that edits the text (the page does)
	// does not wipe the keys it was not shown.
	meta := withoutProduct(in.Metadata)
	if in.Metadata == nil {
		meta = withoutProduct(skill.Metadata)
	}

	origin, source := core.OriginUser, ""

	if existed && skill.Origin() == core.OriginImported {
		origin, source = core.OriginImported, skill.Source()
	}

	// The skill that exists is the start, so the keys of its front matter the product does not know are written
	// back unchanged (SDD-018 section 3).
	next := skill
	next.Name, next.Description, next.License = in.Name, in.Description, in.License
	next.Compatibility, next.AllowedTools, next.Metadata = in.Compatibility, in.AllowedTools, meta
	next.Body, next.Files = in.Body, files
	next.SetOrigin(origin, source)

	return m.finishSkill(ctx, next, c.Person)
}

// finishSkill lints the skill, marks it reviewed when asked, and writes it.
func (m *Manager) finishSkill(
	ctx context.Context,
	s core.Skill,
	review bool,
) (core.Skill, Entry, []core.Finding, error) {
	all, err := s.AllFiles()
	if err != nil {
		return core.Skill{}, Entry{}, nil, invalid("", "%v", err)
	}

	findings := core.LintSkill(s.Name, all)
	if core.HasError(findings) {
		return core.Skill{}, Entry{}, findings, &ValidationError{
			Field:    "skill",
			Msg:      "the skill breaks the lint",
			Findings: findings,
		}
	}

	if review {
		if err := m.sealReview(&s); err != nil {
			return core.Skill{}, Entry{}, nil, err
		}
	}

	if err := m.WriteSkill(ctx, s); err != nil {
		return core.Skill{}, Entry{}, nil, err
	}

	got, entry, err := m.ReadSkill(ctx, s.Name)

	return got, entry, findings, err
}

// MarkReviewed records that a person read the skill. It is for a person at the
// browser, and refused while the lint reports an error. contentHash is the
// content hash of the skill as the person read it (SkillGet gives it): when the
// skill changed since, the mark is refused with ErrChanged, so a review never
// covers a text that was swapped while the person was reading the old one.
func (m *Manager) MarkReviewed(
	ctx context.Context,
	c Caller,
	name, contentHash string,
) (Entry, []core.Finding, error) {
	if !c.Person {
		return Entry{}, nil, fmt.Errorf("%w: only a person at the browser marks a skill as read", ErrForbidden)
	}

	if contentHash == "" {
		return Entry{}, nil, invalid("content_hash", "it is required: the content hash of the skill as it was read")
	}

	s, _, err := m.ReadSkill(ctx, name)
	if err != nil {
		return Entry{}, nil, err
	}

	if s.ContentHash() != contentHash {
		return Entry{}, nil, ErrChanged
	}

	_, entry, findings, err := m.finishSkill(ctx, s, true)

	return entry, findings, err
}

// LintSkill reads the folder of a skill and returns the findings of the lint. A
// folder that cannot be read as a skill is reported as findings too.
func (m *Manager) LintSkill(ctx context.Context, name string) ([]core.Finding, error) {
	root, err := m.openFor(ctx, name)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Close() }()

	rel := entryPath(core.KindSkill, name)
	if _, err := root.Lstat(rel); isMissing(err) {
		return nil, ErrNotFound
	}

	all, _, err := readSkillFiles(root, rel)
	if err != nil {
		return nil, &ValidationError{Field: "skill", Msg: err.Error()}
	}

	return core.LintSkill(name, all), nil
}

func isNotFoundOrInvalid(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}

	_, ok := errors.AsType[*ValidationError](err)

	return ok
}

func countKind(rows []Entry, kind core.Kind) int {
	n := 0

	for i := range rows {
		if rows[i].Kind == kind {
			n++
		}
	}

	return n
}
