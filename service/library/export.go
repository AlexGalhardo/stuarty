package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// ExportResult says what an export wrote.
type ExportResult struct {
	Names []string // the skills written
	Dir   string   // the folder that holds them: exports/library/ of the workspace
}

// ExportSkills writes skills as folders in the Agent Skills layout under
// exports/library/<name>/ of the workspace (SDD-018 section 8). With no names it
// writes every reviewed skill; with names it writes those, reviewed or not. The
// review mark is left out, because it is the word of this workspace about this
// content; the origin and the source stay, so a later import still shows where
// the skill came from. There is no destination argument.
func (m *Manager) ExportSkills(ctx context.Context, names []string) (ExportResult, error) {
	for _, n := range names {
		if err := core.ValidName(n); err != nil {
			return ExportResult{}, invalid("name", "%v", err)
		}
	}

	if len(names) == 0 {
		rows, err := m.List(ctx, core.KindSkill, Filter{})
		if err != nil {
			return ExportResult{}, err
		}

		for i := range rows {
			if rows[i].State == StateOK && rows[i].Reviewed {
				names = append(names, rows[i].Name)
			}
		}
	}

	res := ExportResult{Dir: m.exportDir}
	if len(names) == 0 {
		return res, nil
	}

	skills := make([]core.Skill, 0, len(names))

	for _, n := range names {
		s, _, err := m.ReadSkill(ctx, n)
		if err != nil {
			return ExportResult{}, err
		}

		skills = append(skills, s)
	}

	if err := os.MkdirAll(m.exportDir, 0o700); err != nil {
		return ExportResult{}, err
	}

	root, err := os.OpenRoot(m.exportDir)
	if err != nil {
		return ExportResult{}, err
	}

	defer func() { _ = root.Close() }()

	for i := range skills {
		s := skills[i]
		s.ClearReview()

		all, err := s.AllFiles()
		if err != nil {
			return ExportResult{}, fmt.Errorf("%s: %w", s.Name, err)
		}

		if err := writeDirAtomic(root, ".", s.Name, all); err != nil {
			return ExportResult{}, err
		}

		res.Names = append(res.Names, s.Name)
	}

	return res, nil
}

// exportDirOf is where the exports of a library go: exports/library/ next to library/.
func exportDirOf(libraryDir string) string {
	return filepath.Join(filepath.Dir(libraryDir), "exports", "library")
}
