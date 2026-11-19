package library

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

var testNow = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

func newManager(t *testing.T) (*Manager, *memIndex, string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "library")
	idx := newMemIndex()

	return New(dir, idx, func() time.Time { return testNow }), idx, dir
}

func sampleMemory(name string) core.Memory {
	return core.Memory{
		Name: name, Description: "Open with the result", Scope: "workspace", Tags: []string{"hooks"},
		Actor: "local:ui", Kept: true, Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z",
		Body: "The first three seconds matter.\n",
	}
}

func samplePrompt(name string) core.Prompt {
	return core.Prompt{
		Name: name, Description: "An intro", Tags: []string{"intro"}, Actor: "local:ui",
		Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z", Body: "Write an intro about {{topic}}.\n",
	}
}

func sampleSkill(name string) core.Skill {
	return core.Skill{
		Name: name, Description: "Write a hook for a short video.", License: "MIT",
		Body:  "# Hook\n\nSee [patterns](references/patterns.md).\n",
		Files: []core.File{{Path: "references/patterns.md", Data: []byte("Open with the result.\n")}},
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustList(t *testing.T, m *Manager, kind core.Kind) []Entry {
	t.Helper()

	got, err := m.List(context.Background(), kind, Filter{IncludeUnreviewed: true})
	if err != nil {
		t.Fatal(err)
	}

	return got
}

func TestStoreWritesFilesWhereTheLayoutSays(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	s := sampleSkill("hook-writer")
	s.SetOrigin(core.OriginUser, "")

	for _, err := range []error{
		m.WriteMemory(ctx, sampleMemory("hook-first")), m.WritePrompt(ctx, samplePrompt("intro")), m.WriteSkill(ctx, s),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, rel := range []string{
		"memory/hook-first.md", "prompts/intro.md", "skills/hook-writer/SKILL.md", "skills/hook-writer/references/patterns.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "memory", "hook-first.md"))
	if !strings.HasPrefix(string(raw), "---\nname: hook-first\n") ||
		!strings.Contains(string(raw), "The first three seconds matter.") {
		t.Fatalf("the file is plain markdown with front matter:\n%s", raw)
	}

	if got := mustList(
		t,
		m,
		core.KindMemory,
	); len(got) != 1 || got[0].Name != "hook-first" || got[0].Scope != "workspace" ||
		!got[0].Kept {
		t.Fatalf("%+v", got)
	}

	if got := mustList(t, m, core.KindSkill); len(got) != 1 || got[0].Origin != core.OriginUser || got[0].Reviewed {
		t.Fatalf("%+v", got)
	}

	mem, entry, err := m.ReadMemory(ctx, "hook-first")
	if err != nil || mem.Body != "The first three seconds matter.\n" || entry.State != StateOK {
		t.Fatalf("%v %+v", err, mem)
	}

	sk, _, err := m.ReadSkill(ctx, "hook-writer")
	if err != nil || len(sk.Files) != 1 || sk.Files[0].Path != "references/patterns.md" {
		t.Fatalf("%v %+v", err, sk)
	}

	p, _, err := m.ReadPrompt(ctx, "intro")
	if err != nil || !reflect.DeepEqual(core.Variables(p.Body), []string{"topic"}) {
		t.Fatalf("%v %+v", err, p)
	}

	// nothing temporary is left behind
	entries, _ := os.ReadDir(filepath.Join(dir, "skills"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("a temporary folder was left: %s", e.Name())
		}
	}
}

func TestStoreNamesAreNeverPaths(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	for _, name := range []string{"../x", "a/b", `a\b`, "..", "", "A", "con"} {
		var ve *ValidationError

		if _, _, err := m.ReadMemory(ctx, name); !errors.As(err, &ve) {
			t.Errorf("read %q: %v", name, err)
		}

		mem := sampleMemory("ok")
		mem.Name = name

		if err := m.WriteMemory(ctx, mem); !errors.As(err, &ve) {
			t.Errorf("write %q: %v", name, err)
		}

		if err := m.RemoveMemory(ctx, name); !errors.As(err, &ve) {
			t.Errorf("remove %q: %v", name, err)
		}
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "x.md")); err == nil {
		t.Fatal("a file was written outside the library")
	}
}

func TestStoreMissingEntryIsNotFound(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	if _, _, err := m.ReadMemory(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	if _, _, err := m.ReadSkill(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	if err := m.WriteMemory(ctx, sampleMemory("a")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.ReadPrompt(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	if err := m.RemoveMemory(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	if err := m.RemoveMemory(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	if got := mustList(t, m, core.KindMemory); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// RF-391: every change made outside the product is picked up.
func TestStoreScanPicksUpEditsMadeOutsideTheProduct(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	if err := m.WriteMemory(ctx, sampleMemory("hook-first")); err != nil {
		t.Fatal(err)
	}

	// edited
	write(t, filepath.Join(dir, "memory", "hook-first.md"),
		"---\nname: hook-first\ndescription: A new description that is longer\n---\nbody\n")

	if got := mustList(
		t,
		m,
		core.KindMemory,
	); len(got) != 1 ||
		got[0].Description != "A new description that is longer" {
		t.Fatalf("an edited file: %+v", got)
	}

	// added
	write(t, filepath.Join(dir, "memory", "by-hand.md"), "---\nname: by-hand\ndescription: Added by hand\n---\nx\n")
	write(t, filepath.Join(dir, "skills", "tool", "SKILL.md"), "---\nname: tool\ndescription: A tool.\n---\nUse it.\n")
	write(t, filepath.Join(dir, "prompts", "p.md"), "---\nname: p\ndescription: P\n---\nHi {{x}}\n")

	if got := mustList(t, m, core.KindMemory); len(got) != 2 {
		t.Fatalf("an added note: %+v", got)
	}

	if got := mustList(t, m, core.KindSkill); len(got) != 1 || got[0].Reviewed {
		t.Fatalf("a skill added by hand is unreviewed: %+v", got)
	}

	if got := mustList(t, m, core.KindPrompt); len(got) != 1 {
		t.Fatalf("%+v", got)
	}

	// a file added to the folder of a skill
	write(t, filepath.Join(dir, "skills", "tool", "references", "x.md"), "more")

	st, err := m.Scan(ctx)
	if err != nil || st.Examined != 1 {
		t.Fatalf("a changed skill folder is read again: %+v %v", st, err)
	}

	// removed
	if err := os.Remove(filepath.Join(dir, "memory", "by-hand.md")); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(dir, "skills", "tool")); err != nil {
		t.Fatal(err)
	}

	if got := mustList(t, m, core.KindMemory); len(got) != 1 {
		t.Fatalf("a removed note: %+v", got)
	}

	if got := mustList(t, m, core.KindSkill); len(got) != 0 {
		t.Fatalf("a removed skill: %+v", got)
	}

	// nothing changed: nothing is read again
	if st, err := m.Scan(ctx); err != nil || st.Examined != 0 || st.Removed != 0 {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestStoreInvalidFilesAreListedWithTheirReason(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)

	write(t, filepath.Join(dir, "memory", "broken.md"), "no front matter at all\n")
	write(t, filepath.Join(dir, "memory", "renamed.md"), "---\nname: other\ndescription: d\n---\nx\n")
	write(t, filepath.Join(dir, "memory", "empty.md"), "---\nname: empty\n---\nx\n")
	write(t, filepath.Join(dir, "prompts", "dup.md"), "---\nname: dup\ndescription: d\nname: dup\n---\nx\n")
	write(t, filepath.Join(dir, "skills", "nofile", "README.md"), "x")

	for kind, want := range map[core.Kind]int{core.KindMemory: 3, core.KindPrompt: 1, core.KindSkill: 1} {
		got := mustList(t, m, kind)
		if len(got) != want {
			t.Fatalf("%s: %+v", kind, got)
		}

		for _, e := range got {
			if e.State != StateInvalid || e.Error == "" {
				t.Errorf("%s: an invalid file must say why: %+v", e.Key, e)
			}
		}
	}
}

func TestStoreIgnoresWhatIsNotAnEntry(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)

	write(t, filepath.Join(dir, "memory", "README.txt"), "x")
	write(t, filepath.Join(dir, "memory", "Bad Name.md"), "---\nname: x\ndescription: d\n---\n")
	write(t, filepath.Join(dir, "memory", ".hidden.md"), "---\nname: hidden\ndescription: d\n---\n")
	write(t, filepath.Join(dir, "memory", ".tmp-1234"), "x")
	write(t, filepath.Join(dir, "skills", "Bad Dir", "SKILL.md"), "x")
	write(t, filepath.Join(dir, "skills", ".tmp-abc", "SKILL.md"), "x")
	write(t, filepath.Join(dir, "skills", "loose.md"), "x")

	st, err := m.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if st.Memory != 0 || st.Skills != 0 || len(st.Ignored) != 4 {
		t.Fatalf("%+v", st)
	}
}

func TestStoreDoesNotFollowALink(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	write(t, outside, "---\nname: leak\ndescription: from outside\n---\nsecret\n")
	write(t, filepath.Join(dir, "memory", "keep.md"), "---\nname: keep\ndescription: d\n---\n")

	if err := os.Symlink(outside, filepath.Join(dir, "memory", "leak.md")); err != nil {
		t.Skipf( // verify:env-skip creating a symbolic link needs a privilege on Windows
			"this system does not allow a symbolic link here: %v", err)
	}

	write(t, filepath.Join(dir, "skills", "s", "SKILL.md"), "---\nname: s\ndescription: d\n---\nx\n")

	if err := os.Symlink(outside, filepath.Join(dir, "skills", "s", "link.md")); err != nil {
		t.Skipf( // verify:env-skip creating a symbolic link needs a privilege on Windows
			"this system does not allow a symbolic link here: %v", err)
	}

	mem := mustList(t, m, core.KindMemory)
	if len(mem) != 1 || mem[0].Name != "keep" {
		t.Fatalf("a link is never indexed: %+v", mem)
	}

	sk := mustList(t, m, core.KindSkill)
	if len(sk) != 1 || sk[0].State != StateInvalid || !strings.Contains(sk[0].Error, "link") {
		t.Fatalf("a skill folder with a link is refused whole: %+v", sk)
	}
}

func TestStoreReplacingASkillKeepsNoOldFile(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	if err := m.WriteSkill(ctx, sampleSkill("s")); err != nil {
		t.Fatal(err)
	}

	next := sampleSkill("s")
	next.Files = []core.File{{Path: "references/patterns.md", Data: []byte("v2")}}

	if err := m.WriteSkill(ctx, next); err != nil {
		t.Fatal(err)
	}

	sk, _, err := m.ReadSkill(ctx, "s")
	if err != nil || string(sk.Files[0].Data) != "v2" {
		t.Fatalf("%v %+v", err, sk)
	}

	list, _ := os.ReadDir(filepath.Join(dir, "skills"))
	if len(list) != 1 {
		t.Fatalf("%v", list)
	}
}

func TestStoreRefusesAFilePathThatEscapes(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)

	s := sampleSkill("s")
	s.Files = []core.File{{Path: "../../escape.md", Data: []byte("x")}}

	var ve *ValidationError
	if err := m.WriteSkill(context.Background(), s); !errors.As(err, &ve) {
		t.Fatalf("%v", err)
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); err == nil {
		t.Fatal("a file was written outside")
	}

	if got := mustList(t, m, core.KindSkill); len(got) != 0 {
		t.Fatalf("a failed write leaves no skill: %+v", got)
	}
}

// The index is disposable: with it deleted, a reindex gives the same results.
func TestReindexRestoresEveryResult(t *testing.T) {
	t.Parallel()

	m, idx, _ := newManager(t)
	ctx := context.Background()

	s := sampleSkill("hook-writer")
	s.SetOrigin(core.OriginUser, "")
	s.MarkReviewed()

	for _, err := range []error{
		m.WriteMemory(ctx, sampleMemory("hook-first")), m.WriteMemory(ctx, sampleMemory("second-note")),
		m.WritePrompt(ctx, samplePrompt("intro")), m.WriteSkill(ctx, s),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	before, _ := idx.All(ctx)
	textBefore := maps.Clone(idx.text)

	if len(before) != 4 {
		t.Fatalf("%+v", before)
	}

	if err := idx.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	st, err := m.Reindex(ctx)
	if err != nil || !st.Rebuilt || st.Memory != 2 || st.Skills != 1 || st.Prompts != 1 {
		t.Fatalf("%+v %v", st, err)
	}

	after, _ := idx.All(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rows differ:\n%+v\n%+v", before, after)
	}

	if !reflect.DeepEqual(textBefore, idx.text) {
		t.Fatal("searchable text differs")
	}

	// the same through a fresh manager over a new, empty index (the database file is gone)
	other := New(m.files.dir, newMemIndex(), nil)
	if _, err := other.Scan(ctx); err != nil {
		t.Fatal(err)
	}

	again, _ := other.idx.All(ctx)
	if !reflect.DeepEqual(before, again) {
		t.Fatal("a fresh index does not match")
	}
}

func TestReindexDropsRowsWithoutAFileAndKeepsNothingStale(t *testing.T) {
	t.Parallel()

	m, idx, _ := newManager(t)
	ctx := context.Background()

	ghost := Entry{Key: "memory/ghost", Kind: core.KindMemory, Name: "ghost", State: StateOK}
	if err := idx.Put(ctx, ghost, "ghost"); err != nil {
		t.Fatal(err)
	}

	if err := m.WriteMemory(ctx, sampleMemory("real")); err != nil {
		t.Fatal(err)
	}

	st, err := m.Reindex(ctx)
	if err != nil || st.Memory != 1 {
		t.Fatalf("%+v %v", st, err)
	}

	if rows, _ := idx.All(ctx); len(rows) != 1 || rows[0].Name != "real" {
		t.Fatalf("%+v", rows)
	}

	// and a plain scan drops a row whose file is gone
	if err := idx.Put(ctx, ghost, "ghost"); err != nil {
		t.Fatal(err)
	}

	if st, err := m.Scan(ctx); err != nil || st.Removed != 1 {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestReindexOfAnEmptyOrMissingLibrary(t *testing.T) {
	t.Parallel()

	m, idx, _ := newManager(t)

	st, err := m.Reindex(context.Background())
	if err != nil || st.Memory+st.Skills+st.Prompts != 0 {
		t.Fatalf("%+v %v", st, err)
	}

	if rows, _ := idx.All(context.Background()); len(rows) != 0 {
		t.Fatal(rows)
	}
}

func TestStoreReviewedIsComputedFromTheContent(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	s := sampleSkill("s")
	s.SetOrigin(core.OriginImported, "somewhere")
	s.MarkReviewed()

	if err := m.WriteSkill(ctx, s); err != nil {
		t.Fatal(err)
	}

	if got := mustList(t, m, core.KindSkill); !got[0].Reviewed {
		t.Fatalf("%+v", got)
	}

	// an edit made by another tool takes the review away
	write(
		t,
		filepath.Join(dir, "skills", "s", "references", "patterns.md"),
		"Changed behind the back of the person who read it.\n",
	)

	if got := mustList(t, m, core.KindSkill); got[0].Reviewed {
		t.Fatalf("an edited skill must not stay reviewed: %+v", got)
	}
}
