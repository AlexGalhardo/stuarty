package library

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// Caller is what the library needs to know about whoever acts: the actor
// string, whether it is a person at the browser, and the project a key is
// limited to.
type Caller struct {
	Actor     string
	Person    bool   // a person at the browser (SDD-007 section 5.3)
	ProjectID string // a key limited to one project; empty means any
}

// MemoryInput is a note as a caller writes it. The actor, the times and, for a
// caller that is not a person, the kept mark are not the caller's to set.
type MemoryInput struct {
	Name        string // empty: made from the description
	Description string
	Scope       string // empty is workspace
	Tags        []string
	Body        string
	Keep        *bool // a person may keep or release a note; nil keeps it for a person
}

func (m *Manager) stamp() string { return m.now().UTC().Format(time.RFC3339) }

// SaveMemory writes a note (SDD-018 section 5): it is kept at once when a
// person writes it and pending otherwise, a note that a person kept is not
// rewritten by anyone else, and the number of notes is bounded as a whole and
// per actor and day.
func (m *Manager) SaveMemory(ctx context.Context, c Caller, in MemoryInput) (core.Memory, Entry, error) {
	if in.Keep != nil && *in.Keep && !c.Person {
		return core.Memory{}, Entry{}, fmt.Errorf("%w: only a person keeps a note", ErrForbidden)
	}

	// A key limited to a project writes only into its project, so a note with no scope goes there: the MCP tool
	// has no scope argument, and the workspace default would refuse every note of such a key.
	scope := in.Scope

	switch {
	case scope == "" && c.ProjectID != "":
		scope = c.ProjectID
	case scope == "":
		scope = "workspace"
	}

	if c.ProjectID != "" && scope != c.ProjectID {
		return core.Memory{}, Entry{}, fmt.Errorf("%w: the key is limited to project %s", ErrForbidden, c.ProjectID)
	}

	rows, err := m.refresh(ctx)
	if err != nil {
		return core.Memory{}, Entry{}, err
	}

	name := in.Name
	if name == "" {
		name = uniqueName(core.Slug(in.Description), rows, core.KindMemory)
		if name == "" {
			return core.Memory{}, Entry{}, invalid("description", "it needs letters or digits to make a name from")
		}
	}

	mem := core.Memory{Name: name, Created: m.stamp()}

	existing, _, err := m.ReadMemory(ctx, name)

	switch {
	case err == nil:
		if existing.Kept && !c.Person {
			return core.Memory{}, Entry{}, fmt.Errorf("%w: a person kept this note; write a new one", ErrForbidden)
		}

		mem = existing
	case errors.Is(err, ErrNotFound):
		if err := m.checkNoteLimits(rows, c); err != nil {
			return core.Memory{}, Entry{}, err
		}
	default:
		// An invalid file of that name is replaced by a valid one.
		if _, ok := errors.AsType[*ValidationError](err); !ok {
			return core.Memory{}, Entry{}, err
		}
	}

	mem.Description, mem.Scope, mem.Tags, mem.Body = in.Description, scope, in.Tags, in.Body
	mem.Actor, mem.Updated = c.Actor, m.stamp()

	switch {
	case !c.Person:
		mem.Kept = false
	case in.Keep != nil:
		mem.Kept = *in.Keep
	default:
		mem.Kept = true
	}

	if err := m.WriteMemory(ctx, mem); err != nil {
		return core.Memory{}, Entry{}, err
	}

	got, entry, err := m.ReadMemory(ctx, name)

	return got, entry, err
}

// uniqueName adds -2, -3 and so on to a derived name until no entry of the kind has it.
func uniqueName(base string, rows []Entry, kind core.Kind) string {
	if base == "" {
		return ""
	}

	taken := map[string]bool{}

	for i := range rows {
		if rows[i].Kind == kind {
			taken[rows[i].Name] = true
		}
	}

	name := base

	for n := 2; taken[name]; n++ {
		suffix := "-" + strconv.Itoa(n)
		name = base
		if len(name)+len(suffix) > core.MaxNameLength {
			name = name[:core.MaxNameLength-len(suffix)]
		}

		name += suffix
	}

	return name
}

// checkNoteLimits applies the ceilings on a new note: the notes of the
// workspace, and the notes one actor that is not a person wrote today (UTC).
// The day count is derived from the files, so it survives a rebuilt index.
func (m *Manager) checkNoteLimits(rows []Entry, c Caller) error {
	total, today := 0, 0

	start := m.now().UTC().Truncate(24 * time.Hour).UnixMilli()

	for i := range rows {
		if rows[i].Kind != core.KindMemory {
			continue
		}

		total++

		if rows[i].Actor == c.Actor && rows[i].CreatedMs >= start {
			today++
		}
	}

	if total >= core.MaxNotes {
		return &LimitError{Msg: fmt.Sprintf("the workspace already has %d notes, the most it keeps", core.MaxNotes)}
	}

	if !c.Person && today >= core.MaxNotesPerDay {
		return &LimitError{Msg: fmt.Sprintf(
			"%s already wrote %d notes today, the most one actor writes in a day", c.Actor, core.MaxNotesPerDay)}
	}

	return nil
}

// DeleteMemory removes a note. Only a person removes a note that a person kept.
func (m *Manager) DeleteMemory(ctx context.Context, c Caller, name string) error {
	mem, _, err := m.ReadMemory(ctx, name)

	var ve *ValidationError
	if err != nil && !errors.As(err, &ve) {
		return err
	}

	if err == nil && mem.Kept && !c.Person {
		return fmt.Errorf("%w: a person kept this note", ErrForbidden)
	}

	if c.ProjectID != "" && err == nil && mem.Scope != c.ProjectID {
		return fmt.Errorf("%w: the key is limited to project %s", ErrForbidden, c.ProjectID)
	}

	return m.RemoveMemory(ctx, name)
}

// PromptInput is a prompt as a caller writes it.
type PromptInput struct {
	Name        string
	Description string
	Tags        []string
	Defaults    map[string]string // nil keeps the defaults of a prompt that exists; an empty map removes them
	Body        string
}

// SavePrompt writes a prompt, keeping the creation time of one that exists.
func (m *Manager) SavePrompt(ctx context.Context, c Caller, in PromptInput) (core.Prompt, Entry, error) {
	if c.ProjectID != "" {
		return core.Prompt{}, Entry{}, fmt.Errorf("%w: a key limited to a project does not write prompts", ErrForbidden)
	}

	if err := core.ValidName(in.Name); err != nil {
		return core.Prompt{}, Entry{}, invalid("name", "%v", err)
	}

	rows, err := m.refresh(ctx)
	if err != nil {
		return core.Prompt{}, Entry{}, err
	}

	p := core.Prompt{Name: in.Name, Created: m.stamp()}

	if old, _, err := m.ReadPrompt(ctx, in.Name); err == nil {
		// The prompt that exists is the start, so the keys the product does not know are written back (SDD-018
		// section 3), and nil defaults keep the defaults it has: a page that edits the text does not wipe them.
		p = old

		if in.Defaults == nil {
			in.Defaults = old.Defaults
		}
	} else {
		var ve *ValidationError
		if !errors.Is(err, ErrNotFound) && !errors.As(err, &ve) {
			return core.Prompt{}, Entry{}, err
		}

		if errors.Is(err, ErrNotFound) {
			count := 0

			for i := range rows {
				if rows[i].Kind == core.KindPrompt {
					count++
				}
			}

			if count >= core.MaxPrompts {
				return core.Prompt{}, Entry{}, &LimitError{
					Msg: fmt.Sprintf("the workspace already has %d prompts, the most it keeps", core.MaxPrompts),
				}
			}
		}
	}

	p.Description, p.Tags, p.Defaults, p.Body = in.Description, in.Tags, in.Defaults, in.Body
	p.Actor, p.Updated = c.Actor, m.stamp()

	if err := m.WritePrompt(ctx, p); err != nil {
		return core.Prompt{}, Entry{}, err
	}

	return m.ReadPrompt(ctx, in.Name)
}

// Render replaces the variables of a prompt. A failure names the variables.
func (m *Manager) Render(ctx context.Context, name string, values map[string]string) (string, core.Prompt, error) {
	p, _, err := m.ReadPrompt(ctx, name)
	if err != nil {
		return "", core.Prompt{}, err
	}

	if err := p.Validate(); err != nil {
		return "", core.Prompt{}, fromCore(err)
	}

	out, err := core.Render(&p, values)
	if err != nil {
		if re, ok := errors.AsType[*core.RenderError](err); ok {
			ve := &ValidationError{Field: "values", Msg: re.Error(), Details: map[string]any{}}
			if len(re.Missing) > 0 {
				ve.Details["missing"] = re.Missing
			}

			if len(re.Unknown) > 0 {
				ve.Details["unknown"] = re.Unknown
			}

			if len(re.TooLong) > 0 {
				ve.Details["too_long"] = re.TooLong
			}

			return "", core.Prompt{}, ve
		}

		return "", core.Prompt{}, err
	}

	return out, p, nil
}
