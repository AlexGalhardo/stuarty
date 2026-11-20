package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// builtInFile records what the product installed of each built-in skill (SDD-018 section 16). It is a dot file
// of library/: no scan, import or export reads it.
const builtInFile = ".built-in.json"

// builtInSource is the stuarty-source of a built-in skill that names none of its own.
const builtInSource = "stuarty"

// BuiltInRecord is what the product installed of one built-in skill.
type BuiltInRecord struct {
	Version string `json:"version"`
	Hash    string `json:"hash"` // the content hash of the folder as it was installed
}

// BuiltInReport says what an install of the built-in set did, by skill name.
type BuiltInReport struct {
	Installed []string `json:"installed,omitempty"` // new in this workspace
	Updated   []string `json:"updated,omitempty"`   // untouched, and the embedded text changed
	Current   []string `json:"current,omitempty"`   // untouched, and already the embedded text
	Kept      []string `json:"kept,omitempty"`      // a copy that is not as the product left it: never overwritten
}

// readBuiltIn reads the record of the library. A missing or damaged file is an empty record: no skill then
// reads as built-in, which is the safe side, and the next install repairs it.
func readBuiltIn(root *os.Root) map[string]BuiltInRecord {
	data, _, err := readSmall(root, builtInFile, 256<<10)
	if err != nil {
		return map[string]BuiltInRecord{}
	}

	rec := map[string]BuiltInRecord{}
	if json.Unmarshal(data, &rec) != nil || rec == nil {
		return map[string]BuiltInRecord{}
	}

	return rec
}

// builtInHash is the content hash the product recorded for the skill, or "".
func builtInHash(root *os.Root, name string) string { return readBuiltIn(root)[name].Hash }

func writeBuiltIn(root *os.Root, rec map[string]BuiltInRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}

	return writeFile(root, builtInFile, append(data, '\n'))
}

// builtInSkills reads the skills of an embedded set: one folder per skill, each with a SKILL.md. A folder that
// is not a valid skill of its own name is an error of the build, never skipped.
func builtInSkills(set fs.FS) ([]core.Skill, error) {
	dirs, err := fs.ReadDir(set, ".")
	if err != nil {
		return nil, fmt.Errorf("library: the built-in skills cannot be read: %w", err)
	}

	var out []core.Skill

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		var files []core.File

		err := fs.WalkDir(set, d.Name(), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return err
			}

			data, err := fs.ReadFile(set, p)
			if err != nil {
				return err
			}

			files = append(files, core.File{Path: path.Clean(p[len(d.Name())+1:]), Data: data})

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("library: the built-in skill %s cannot be read: %w", d.Name(), err)
		}

		skill, err := core.ParseSkill(files)
		if err != nil {
			return nil, fmt.Errorf("library: the built-in skill %s is not valid: %w", d.Name(), err)
		}

		if skill.Name != d.Name() {
			return nil, fmt.Errorf("library: the built-in skill in %s is named %s", d.Name(), skill.Name)
		}

		out = append(out, skill)
	}

	return out, nil
}

// asBuiltIn is the skill as the product installs it: origin built-in, and a source when it names none.
func asBuiltIn(s core.Skill) core.Skill {
	s.Metadata = maps.Clone(s.Metadata)
	if s.Metadata == nil {
		s.Metadata = map[string]string{}
	}

	source := s.Source()
	if source == "" {
		source = builtInSource
	}

	s.SetOrigin(core.OriginBuiltIn, source)
	s.ClearReview()

	return s
}

// InstallBuiltInSet installs or updates the built-in skills of set (one folder per skill) for the product
// version. A folder that does not exist is written; one that is as the product left it is rewritten when the
// embedded text changed; one that a person edited is never overwritten (SDD-018 section 16). It is not an
// operation of the service.
func (m *Manager) InstallBuiltInSet(ctx context.Context, set fs.FS, version string) (BuiltInReport, error) {
	var report BuiltInReport

	skills, err := builtInSkills(set)
	if err != nil {
		return report, err
	}

	for i := range skills {
		want := asBuiltIn(skills[i])

		action, err := m.builtInAction(ctx, &want, version)
		if err != nil {
			return report, err
		}

		switch action {
		case actionKeep:
			report.Kept = append(report.Kept, want.Name)
		case actionCurrent:
			report.Current = append(report.Current, want.Name)
		case actionInstall, actionUpdate:
			if err := m.installOne(ctx, want, version); err != nil {
				return report, err
			}

			if action == actionInstall {
				report.Installed = append(report.Installed, want.Name)
			} else {
				report.Updated = append(report.Updated, want.Name)
			}
		}
	}

	return report, nil
}

type builtInDecision int

const (
	actionInstall builtInDecision = iota
	actionUpdate
	actionCurrent
	actionKeep
)

// builtInAction decides what to do with one skill; a record that only needs the new version is written here.
func (m *Manager) builtInAction(ctx context.Context, want *core.Skill, version string) (builtInDecision, error) {
	root, err := m.files.open()
	if err != nil {
		return actionKeep, err
	}

	rec := map[string]BuiltInRecord{}

	if root != nil {
		rec = readBuiltIn(root)

		_ = root.Close()
	}

	cur, _, err := m.ReadSkill(ctx, want.Name)

	if errors.Is(err, ErrNotFound) {
		return actionInstall, nil
	}

	// A folder that cannot be read as a skill is not the product's to rewrite: it is kept, and shown as invalid.
	if _, invalid := errors.AsType[*ValidationError](err); invalid {
		return actionKeep, nil
	}

	if err != nil {
		return actionKeep, err
	}

	recorded := rec[want.Name]

	// A record that is lost or damaged is repaired for a folder that is, byte for byte, the embedded text.
	if recorded.Hash == "" && cur.ContentHash() == want.ContentHash() {
		return actionCurrent, m.recordBuiltIn(ctx, want.Name, BuiltInRecord{Version: version, Hash: want.ContentHash()})
	}

	if recorded.Hash == "" || cur.ContentHash() != recorded.Hash {
		return actionKeep, nil
	}

	if recorded.Hash != want.ContentHash() {
		return actionUpdate, nil
	}

	if recorded.Version != version {
		return actionCurrent, m.recordBuiltIn(ctx, want.Name, BuiltInRecord{Version: version, Hash: recorded.Hash})
	}

	return actionCurrent, nil
}

// InstallBuiltIn writes one skill that ships with the product: origin built-in, and reviewed, because the
// project wrote and read it. It records the install the way InstallBuiltInSet does. It is not an operation of
// the service.
func (m *Manager) InstallBuiltIn(ctx context.Context, s core.Skill) error {
	return m.installOne(ctx, asBuiltIn(s), "")
}

func (m *Manager) installOne(ctx context.Context, s core.Skill, version string) error {
	hash := s.ContentHash()

	if _, _, _, err := m.finishSkill(ctx, s, true); err != nil {
		return err
	}

	return m.recordBuiltIn(ctx, s.Name, BuiltInRecord{Version: version, Hash: hash})
}

// recordBuiltIn writes the record of one skill and brings its row in step: the origin of a row depends on it.
func (m *Manager) recordBuiltIn(ctx context.Context, name string, r BuiltInRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.files.create()
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	rec := readBuiltIn(root)
	rec[name] = r

	if err := writeBuiltIn(root, rec); err != nil {
		return err
	}

	return m.putRow(ctx, root, core.KindSkill, name)
}
