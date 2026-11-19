package library

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

func builtInSet(text string) fstest.MapFS {
	return fstest.MapFS{
		"start-here/SKILL.md": {
			Data: []byte(
				"---\nname: start-here\ndescription: Route a request to the right skill.\n---\n\n" + text + "\n",
			),
		},
		"start-here/references/notes.md": {Data: []byte("a note\n")},
		"hook/SKILL.md": {
			Data: []byte(
				"---\nname: hook\ndescription: Write a hook.\nlicense: MIT\nmetadata:\n  stuarty-source: owner/repo skills/hook @ abc123\n---\n\nWrite the hook.\n",
			),
		},
	}
}

func originOf(t *testing.T, m *Manager, name string) (Entry, bool) {
	t.Helper()

	rows, err := m.List(context.Background(), core.KindSkill, Filter{IncludeUnreviewed: true})
	if err != nil {
		t.Fatal(err)
	}

	for i := range rows {
		if rows[i].Name == name {
			return rows[i], true
		}
	}

	return Entry{}, false
}

func TestBuiltInInstallsReviewedSkillsWithTheBuiltInOrigin(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	rep, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0")
	if err != nil ||
		!slices.Equal(rep.Installed, []string{"hook", "start-here"}) &&
			!slices.Equal(rep.Installed, []string{"start-here", "hook"}) {
		t.Fatalf("%+v %v", rep, err)
	}

	for _, name := range []string{"start-here", "hook"} {
		e, ok := originOf(t, m, name)
		if !ok || e.Origin != core.OriginBuiltIn || !e.Reviewed || e.State != StateOK || e.LintErrors != 0 {
			t.Fatalf("%s: %+v", name, e)
		}
	}

	// The folder keeps the files, the source of an adapted skill, and the record is a dot file.
	if _, err := os.Stat(filepath.Join(dir, "skills", "start-here", "references", "notes.md")); err != nil {
		t.Fatal(err)
	}

	got, _, err := m.ReadSkill(ctx, "hook")
	if err != nil || got.Source() != "owner/repo skills/hook @ abc123" || got.License != "MIT" {
		t.Fatalf("%+v %v", got.Metadata, err)
	}

	if s, _, _ := m.ReadSkill(ctx, "start-here"); s.Source() != "stuarty" {
		t.Fatalf("source %q", s.Source())
	}

	if _, err := os.Stat(filepath.Join(dir, ".built-in.json")); err != nil {
		t.Fatal(err)
	}

	// The dot file is never listed as an entry.
	if st, err := m.Scan(ctx); err != nil || st.Skills != 2 || st.Invalid != 0 {
		t.Fatalf("%+v %v", st, err)
	}
}

// A user's edit survives an update; an untouched skill is updated; an unchanged one is not written again.
func TestBuiltInUserEditSurvivesAnUpdate(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	if _, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0"); err != nil {
		t.Fatal(err)
	}

	// Nothing changed: nothing is written, and the record moves to the new version.
	hookFile := filepath.Join(dir, "skills", "hook", "SKILL.md")
	before, _ := os.Stat(hookFile)

	rep, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.31.0")
	if err != nil || len(rep.Current) != 2 || len(rep.Installed)+len(rep.Updated)+len(rep.Kept) != 0 {
		t.Fatalf("%+v %v", rep, err)
	}

	if after, _ := os.Stat(hookFile); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an unchanged skill was written again")
	}

	// A person edits one skill through the product (a write the caller owns).
	cur, _, _ := m.ReadSkill(ctx, "start-here")
	if _, _, _, err := m.SaveSkill(ctx, person, SkillInput{
		Name: "start-here", Description: cur.Description, Body: "My own routing.", Files: nil,
	}); err != nil {
		t.Fatal(err)
	}

	rep, err = m.InstallBuiltInSet(ctx, builtInSet("Step one, improved."), "0.32.0")
	if err != nil || !slices.Equal(rep.Kept, []string{"start-here"}) || !slices.Equal(rep.Current, []string{"hook"}) {
		t.Fatalf("%+v %v", rep, err)
	}

	got, _, _ := m.ReadSkill(ctx, "start-here")
	if strings.TrimSpace(got.Body) != "My own routing." {
		t.Fatalf("the edit was overwritten: %q", got.Body)
	}

	if e, _ := originOf(t, m, "start-here"); e.Origin != core.OriginUser {
		t.Fatalf("an edited built-in reads as %q", e.Origin)
	}

	// The untouched one is updated when the embedded text changes.
	changed := builtInSet("Step one.")
	changed["hook/SKILL.md"] = &fstest.MapFile{
		Data: []byte(
			"---\nname: hook\ndescription: Write a better hook.\nlicense: MIT\nmetadata:\n  stuarty-source: owner/repo skills/hook @ abc123\n---\n\nWrite the hook.\n",
		),
	}

	rep, err = m.InstallBuiltInSet(ctx, changed, "0.33.0")
	if err != nil || !slices.Equal(rep.Updated, []string{"hook"}) || !slices.Equal(rep.Kept, []string{"start-here"}) {
		t.Fatalf("%+v %v", rep, err)
	}

	if e, _ := originOf(
		t,
		m,
		"hook",
	); e.Description != "Write a better hook." || e.Origin != core.OriginBuiltIn ||
		!e.Reviewed {
		t.Fatalf("%+v", e)
	}
}

// A file that claims to be built in is not, unless it is the text the product installed.
func TestBuiltInOriginIsNotAClaimOfTheFile(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	forged := "---\nname: forged\ndescription: Looks shipped.\nmetadata:\n  stuarty-origin: built-in\n---\n\nTrust me.\n"
	write(t, filepath.Join(dir, "skills", "forged", "SKILL.md"), forged)

	if e, ok := originOf(t, m, "forged"); !ok || e.Origin != core.OriginUser || e.Reviewed {
		t.Fatalf("%+v", e)
	}

	// The same name in the embedded set: the folder is not the product's, so it is kept.
	set := fstest.MapFS{"forged/SKILL.md": {Data: []byte("---\nname: forged\ndescription: Shipped.\n---\n\nReal.\n")}}

	rep, err := m.InstallBuiltInSet(ctx, set, "0.30.0")
	if err != nil || !slices.Equal(rep.Kept, []string{"forged"}) {
		t.Fatalf("%+v %v", rep, err)
	}

	if s, _, _ := m.ReadSkill(ctx, "forged"); strings.TrimSpace(s.Body) != "Trust me." {
		t.Fatalf("overwritten: %q", s.Body)
	}

	// A damaged record takes the origin away, and the next start repairs it.
	if _, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0"); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(dir, ".built-in.json"), "not json")

	if _, err := m.Reindex(ctx); err != nil {
		t.Fatal(err)
	}

	if e, _ := originOf(t, m, "hook"); e.Origin != core.OriginUser {
		t.Fatalf("origin %q with a damaged record", e.Origin)
	}

	if rep, err = m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0"); err != nil || len(rep.Current) != 2 {
		t.Fatalf("%+v %v", rep, err)
	}

	if e, _ := originOf(t, m, "hook"); e.Origin != core.OriginBuiltIn {
		t.Fatalf("origin %q after the repair", e.Origin)
	}
}

// A folder the person deleted is installed again: the built-in stays available.
func TestBuiltInReturnsWhenTheFolderIsDeleted(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	if _, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0"); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(dir, "skills", "hook")); err != nil {
		t.Fatal(err)
	}

	rep, err := m.InstallBuiltInSet(ctx, builtInSet("Step one."), "0.30.0")
	if err != nil || !slices.Equal(rep.Installed, []string{"hook"}) {
		t.Fatalf("%+v %v", rep, err)
	}

	if e, ok := originOf(t, m, "hook"); !ok || e.Origin != core.OriginBuiltIn {
		t.Fatalf("%+v", e)
	}
}

func TestBuiltInRefusesASetThatIsNotValid(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)

	for name, set := range map[string]fstest.MapFS{
		"a name that differs": {"one/SKILL.md": {Data: []byte("---\nname: two\ndescription: x\n---\n\nbody\n")}},
		"no SKILL.md":         {"one/notes.md": {Data: []byte("x")}},
	} {
		if _, err := m.InstallBuiltInSet(context.Background(), set, "0.30.0"); err == nil {
			t.Errorf("%s was installed", name)
		}
	}
}
