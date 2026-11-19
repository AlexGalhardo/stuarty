package library

import (
	"context"
	"os"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// openFor refreshes the index and opens the library folder; ErrNotFound when
// there is none yet.
func (m *Manager) openFor(ctx context.Context, name string) (*os.Root, error) {
	if err := core.ValidName(name); err != nil {
		return nil, invalid("name", "%v", err)
	}

	if _, err := m.refresh(ctx); err != nil {
		return nil, err
	}

	root, err := m.files.open()
	if err != nil {
		return nil, err
	}

	if root == nil {
		return nil, ErrNotFound
	}

	return root, nil
}

// ReadMemory returns a note and its row. The file is read, not the index.
func (m *Manager) ReadMemory(ctx context.Context, name string) (core.Memory, Entry, error) {
	root, err := m.openFor(ctx, name)
	if err != nil {
		return core.Memory{}, Entry{}, err
	}

	defer func() { _ = root.Close() }()

	data, _, err := readSmall(root, entryPath(core.KindMemory, name), maxEntryFile)
	if isMissing(err) {
		return core.Memory{}, Entry{}, ErrNotFound
	}

	if err != nil {
		return core.Memory{}, Entry{}, invalid("", "the note cannot be read: %v", err)
	}

	mem, err := core.ParseMemory(string(data))
	if err != nil {
		return core.Memory{}, Entry{}, fromCore(err)
	}

	entry, _ := m.loadFlat(root, core.KindMemory, name)

	return mem, entry, nil
}

// WriteMemory checks a note and writes its file, replacing one of the same name.
func (m *Manager) WriteMemory(ctx context.Context, mem core.Memory) error {
	if err := mem.Validate(); err != nil {
		return fromCore(err)
	}

	text, err := mem.Marshal()
	if err != nil {
		return invalid("", "%v", err)
	}

	return m.writeFlat(ctx, core.KindMemory, mem.Name, text)
}

func (m *Manager) writeFlat(ctx context.Context, kind core.Kind, name, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.files.create()
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	if err := writeFile(root, entryPath(kind, name), []byte(text)); err != nil {
		return err
	}

	return m.putRow(ctx, root, kind, name)
}

// RemoveMemory deletes the file of a note.
func (m *Manager) RemoveMemory(ctx context.Context, name string) error {
	root, err := m.openFor(ctx, name)
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := root.Remove(entryPath(core.KindMemory, name)); err != nil {
		if isMissing(err) {
			return ErrNotFound
		}

		return err
	}

	return m.idx.Delete(ctx, core.Key(core.KindMemory, name))
}

// ReadPrompt returns a prompt and its row.
func (m *Manager) ReadPrompt(ctx context.Context, name string) (core.Prompt, Entry, error) {
	root, err := m.openFor(ctx, name)
	if err != nil {
		return core.Prompt{}, Entry{}, err
	}

	defer func() { _ = root.Close() }()

	data, _, err := readSmall(root, entryPath(core.KindPrompt, name), maxEntryFile)
	if isMissing(err) {
		return core.Prompt{}, Entry{}, ErrNotFound
	}

	if err != nil {
		return core.Prompt{}, Entry{}, invalid("", "the prompt cannot be read: %v", err)
	}

	p, err := core.ParsePrompt(string(data))
	if err != nil {
		return core.Prompt{}, Entry{}, fromCore(err)
	}

	entry, _ := m.loadFlat(root, core.KindPrompt, name)

	return p, entry, nil
}

// WritePrompt checks a prompt and writes its file.
func (m *Manager) WritePrompt(ctx context.Context, p core.Prompt) error {
	if err := p.Validate(); err != nil {
		return fromCore(err)
	}

	text, err := p.Marshal()
	if err != nil {
		return invalid("", "%v", err)
	}

	return m.writeFlat(ctx, core.KindPrompt, p.Name, text)
}

// ReadSkill returns a skill, its files and its row.
func (m *Manager) ReadSkill(ctx context.Context, name string) (core.Skill, Entry, error) {
	root, err := m.openFor(ctx, name)
	if err != nil {
		return core.Skill{}, Entry{}, err
	}

	defer func() { _ = root.Close() }()

	if _, err := root.Lstat(entryPath(core.KindSkill, name)); isMissing(err) {
		return core.Skill{}, Entry{}, ErrNotFound
	}

	entry, _, skill := m.loadSkill(root, name)
	if entry.State == StateInvalid {
		return core.Skill{}, entry, &ValidationError{Field: "skill", Msg: entry.Error}
	}

	return skill, entry, nil
}

// WriteSkill writes the whole folder of a skill, replacing one of the same
// name: the new folder is complete before the old one is let go.
func (m *Manager) WriteSkill(ctx context.Context, s core.Skill) error {
	if err := core.ValidName(s.Name); err != nil {
		return invalid("name", "%v", err)
	}

	all, err := s.AllFiles()
	if err != nil {
		return invalid("", "%v", err)
	}

	return m.writeSkillFiles(ctx, s.Name, all)
}

func (m *Manager) writeSkillFiles(ctx context.Context, name string, all []core.File) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.files.create()
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	if err := writeSkillDir(root, name, all); err != nil {
		return err
	}

	// The time of the last write is the time of the library clock, so that the skill says when the product
	// wrote it the way a note does; a file edited by hand keeps the real time of that edit.
	now := m.now()
	if err := root.Chtimes(entryPath(core.KindSkill, name)+"/"+core.SkillFile, now, now); err != nil {
		return err
	}

	return m.putRow(ctx, root, core.KindSkill, name)
}
