package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/service/account"
)

const canary = "CANARY-LIBRARY-7f3a"

// lockable is a FileSealer over one key that can be locked, like the manager of the account.
type lockable struct {
	*account.KeySealer
	locked bool
}

func (l *lockable) Ready() error {
	if l.locked {
		return account.ErrLocked
	}

	return nil
}

func (l *lockable) SealFile(rel string, b []byte) ([]byte, error) {
	if l.locked {
		return nil, account.ErrLocked
	}

	return l.KeySealer.SealFile(rel, b)
}

func (l *lockable) OpenFile(rel string, b []byte) ([]byte, error) {
	if l.locked {
		return nil, account.ErrLocked
	}

	return l.KeySealer.OpenFile(rel, b)
}

func newSealed(t *testing.T) (*Manager, *lockable, string) {
	t.Helper()

	key := bytes.Repeat([]byte{9}, account.KeySize)

	ks, err := account.NewKeySealer(key)
	if err != nil {
		t.Fatal(err)
	}

	sl := &lockable{KeySealer: ks}
	dir := filepath.Join(t.TempDir(), "library")

	return New(dir, newMemIndex(), func() time.Time { return testNow }, WithFiles(sl)), sl, dir
}

// everyFile lists the regular files under dir, with their bytes.
func everyFile(t *testing.T, dir string) map[string][]byte {
	t.Helper()

	out := map[string][]byte{}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}

		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = b

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func writeAll(t *testing.T, m *Manager) {
	t.Helper()

	ctx := context.Background()

	mem := sampleMemory("note")
	mem.Body = canary + " in a note\n"

	if err := m.WriteMemory(ctx, mem); err != nil {
		t.Fatal(err)
	}

	p := samplePrompt("intro")
	p.Body = canary + " in a prompt {{topic}}\n"

	if err := m.WritePrompt(ctx, p); err != nil {
		t.Fatal(err)
	}

	s := sampleSkill("hook")
	s.Body = "# Hook\n\n" + canary + " in a skill\n"
	s.Files = []core.File{{Path: "references/patterns.md", Data: []byte(canary + " in a reference\n")}}

	if err := m.WriteSkill(ctx, s); err != nil {
		t.Fatal(err)
	}

	// Writing the review key and the record of the built-in skills goes through the same door.
	if _, err := m.reviewKey(true); err != nil {
		t.Fatal(err)
	}
}

func TestSealedLibraryReadsBackWhatItWrote(t *testing.T) {
	m, _, _ := newSealed(t)
	writeAll(t, m)

	ctx := context.Background()

	mem, _, err := m.ReadMemory(ctx, "note")
	if err != nil || !strings.Contains(mem.Body, canary) {
		t.Fatalf("note: %v", err)
	}

	p, _, err := m.ReadPrompt(ctx, "intro")
	if err != nil || !strings.Contains(p.Body, canary) {
		t.Fatalf("prompt: %v", err)
	}

	s, _, err := m.ReadSkill(ctx, "hook")
	if err != nil || !strings.Contains(string(s.Files[0].Data), canary) {
		t.Fatalf("skill: %v", err)
	}
}

func TestSecurity_T130_ACopyOfTheWorkspaceHoldsNoLibraryFileInClearText(t *testing.T) {
	m, _, dir := newSealed(t)
	writeAll(t, m)

	files := everyFile(t, dir)
	if len(files) < 5 {
		t.Fatalf("expected a note, a prompt, two skill files and the review key, got %d files", len(files))
	}

	for name, b := range files {
		if bytes.Contains(b, []byte(canary)) {
			t.Errorf("%s holds the canary in clear text", name)
		}

		if !account.IsSealedFile(b) {
			t.Errorf("%s is not sealed", name)
		}
	}

	// A clear file the person dropped in is sealed by the scan, and the clear one is gone.
	write(
		t,
		filepath.Join(dir, "memory", "dropped.md"),
		"---\nname: dropped\ndescription: d\nscope: workspace\nactor: local:ui\n"+
			"kept: true\ncreated: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\n"+canary+" dropped\n",
	)
	write(t, filepath.Join(dir, "skills", "imported", "SKILL.md"),
		"---\nname: imported\ndescription: A skill that was dropped in.\n---\n"+canary+"\n")
	write(t, filepath.Join(dir, "skills", "imported", "references", "a.md"), canary+" ref\n")

	if _, err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	for name, b := range everyFile(t, dir) {
		if bytes.Contains(b, []byte(canary)) || !account.IsSealedFile(b) {
			t.Errorf("%s is still readable after the scan", name)
		}
	}

	mem, _, err := m.ReadMemory(context.Background(), "dropped")
	if err != nil || !strings.Contains(mem.Body, "dropped") {
		t.Fatalf("a dropped note is read after it was sealed: %v", err)
	}
}

func TestScanSealsAClearFileByRenameKeepsItsTimeAndReadsItBack(t *testing.T) {
	m, _, dir := newSealed(t)
	path := filepath.Join(dir, "prompts", "p.md")

	write(
		t,
		path,
		"---\nname: p\ndescription: d\nactor: local:ui\ncreated: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\nBody {{x}}\n",
	)

	old := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if !info.ModTime().Equal(old) {
		t.Errorf("the time of the file changed: %v", info.ModTime())
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "prompts"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("a temporary file was left: %s", e.Name())
		}
	}

	if b, _ := os.ReadFile(path); !account.IsSealedFile(b) {
		t.Fatal("the file is not sealed")
	}
}

func TestSecurity_T131_ASealedFileRenamedSwappedOrCutDoesNotOpenAndIsListedInvalid(t *testing.T) {
	m, _, dir := newSealed(t)
	ctx := context.Background()

	for _, n := range []string{"a", "b", "c"} {
		mem := sampleMemory(n)
		mem.Body = "body of " + n + "\n"

		if err := m.WriteMemory(ctx, mem); err != nil {
			t.Fatal(err)
		}
	}

	read := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, "memory", n+".md"))
		if err != nil {
			t.Fatal(err)
		}

		return b
	}

	put := func(n string, b []byte) {
		p := filepath.Join(dir, "memory", n+".md")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		// A distinct time, so the scan sees a change.
		when := time.Now().Add(time.Hour)
		_ = os.Chtimes(p, when, when)
	}

	// b holds the sealed bytes of a (renamed); c is cut; a is swapped with b's content then flipped.
	put("b", read("a"))
	put("c", read("c")[:20])

	flipped := bytes.Clone(read("a"))
	flipped[len(flipped)-1] ^= 1
	put("a", flipped)

	rows, err := m.List(ctx, core.KindMemory, Filter{IncludeUnreviewed: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 {
		t.Fatalf("every file must be listed, got %d", len(rows))
	}

	for _, e := range rows {
		if e.State != StateInvalid || e.Error == "" {
			t.Errorf("%s: state %q error %q", e.Name, e.State, e.Error)
		}
	}

	if _, _, err := m.ReadMemory(ctx, "b"); err == nil {
		t.Fatal("a sealed file under another name opened")
	}
}

func TestLockedLibraryScansNothingAndLeavesAClearFileAlone(t *testing.T) {
	m, sl, dir := newSealed(t)
	writeAll(t, m)

	clearPath := filepath.Join(dir, "memory", "clear.md")
	write(t, clearPath, "plain")

	sl.locked = true

	if _, err := m.Scan(context.Background()); !errors.Is(err, account.ErrLocked) {
		t.Fatalf("a locked scan: %v", err)
	}

	if _, _, err := m.ReadMemory(context.Background(), "note"); !errors.Is(err, account.ErrLocked) {
		t.Fatalf("a locked read: %v", err)
	}

	if b, _ := os.ReadFile(clearPath); string(b) != "plain" {
		t.Fatal("a locked scan changed a clear file")
	}

	if err := m.WriteMemory(context.Background(), sampleMemory("n2")); !errors.Is(err, account.ErrLocked) {
		t.Fatalf("a locked write: %v", err)
	}

	sl.locked = false

	if _, err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	if b, _ := os.ReadFile(clearPath); string(b) == "plain" {
		t.Fatal("the clear file was not sealed at the next unlocked scan")
	}
}

func TestLibraryWithNoAccountStaysClearText(t *testing.T) {
	m, _, dir := newManager(t)

	mem := sampleMemory("note")
	mem.Body = canary + "\n"

	if err := m.WriteMemory(context.Background(), mem); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "memory", "note.md"))
	if err != nil || !bytes.Contains(b, []byte(canary)) || account.IsSealedFile(b) {
		t.Fatalf("a workspace with no account must keep clear files: %v", err)
	}
}

func TestLibrarySealAllSealsEveryFileOfTheFolderIncludingStrayOnes(t *testing.T) {
	m, _, dir := newSealed(t)

	write(t, filepath.Join(dir, "memory", "x.md"), canary)
	write(t, filepath.Join(dir, "skills", "s", "scripts", "run.sh"), canary)
	write(t, filepath.Join(dir, ".review-key"), canary)
	write(t, filepath.Join(dir, "memory", "notes.txt"), canary)

	n, err := m.SealAll(context.Background())
	if err != nil || n != 4 {
		t.Fatalf("sealed %d: %v", n, err)
	}

	for name, b := range everyFile(t, dir) {
		if bytes.Contains(b, []byte(canary)) || !account.IsSealedFile(b) {
			t.Errorf("%s is readable", name)
		}
	}

	if n, err := m.SealAll(context.Background()); err != nil || n != 0 {
		t.Fatalf("a second run must find nothing: %d %v", n, err)
	}
}

func TestDropForgetsTheIndexAndTheReviewKey(t *testing.T) {
	m, _, _ := newSealed(t)
	idx := m.idx
	writeAll(t, m)

	if rows, _ := idx.All(context.Background()); len(rows) == 0 {
		t.Fatal("expected rows")
	}

	if err := m.Drop(context.Background()); err != nil {
		t.Fatal(err)
	}

	if rows, _ := idx.All(context.Background()); len(rows) != 0 {
		t.Fatal("the index survived Drop")
	}

	m.keyMu.Lock()
	defer m.keyMu.Unlock()

	if m.key != nil {
		t.Fatal("the review key survived Drop")
	}
}

// A library of the limits of SDD-018 section 12 (2,000 notes, 500 skills, 500 prompts) is ready within two seconds
// of the unlock: the index is built by one scan over sealed files (ADR-0053).
func TestLibraryOfTheMaximumSizeIsReadyWithinTwoSecondsOfTheUnlock(t *testing.T) {
	if testing.Short() {
		t.Skip("builds 3,000 sealed files") // verify:env-skip short mode: a minute of file writes on a slow disk
	}

	if raceEnabled {
		t.Skip("the race detector slows 3,000 files about tenfold") // verify:env-skip a race build
	}

	m, _, dir := newSealed(t)
	ctx := context.Background()

	body := strings.Repeat("A line of the text of a note, a prompt or a skill that a search can match.\n", 40)

	for i := range 2000 {
		n := fmt.Sprintf("note-%04d", i)
		write(
			t,
			filepath.Join(dir, "memory", n+".md"),
			"---\nname: "+n+"\ndescription: A note about hooks\nscope: workspace\n"+
				"actor: local:ui\nkept: true\ncreated: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\n"+body,
		)
	}

	for i := range 500 {
		n := fmt.Sprintf("prompt-%04d", i)
		write(t, filepath.Join(dir, "prompts", n+".md"), "---\nname: "+n+"\ndescription: A prompt\nactor: local:ui\n"+
			"created: 2026-11-19T10:00:00Z\nupdated: 2026-11-19T10:00:00Z\n---\n"+body+"{{topic}}\n")
	}

	for i := range 500 {
		n := fmt.Sprintf("skill-%04d", i)
		write(
			t,
			filepath.Join(dir, "skills", n, "SKILL.md"),
			"---\nname: "+n+"\ndescription: A skill for hooks.\n---\n"+body,
		)
		write(t, filepath.Join(dir, "skills", n, "references", "a.md"), body)
	}

	// The first unlock after an upgrade seals what is clear; the ones after it only read.
	start := time.Now()

	if _, err := m.SealAll(ctx); err != nil {
		t.Fatal(err)
	}

	t.Logf("sealing 3,000 clear entries took %v", time.Since(start))

	if err := m.Drop(ctx); err != nil {
		t.Fatal(err)
	}

	start = time.Now()

	st, err := m.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}

	took := time.Since(start)
	t.Logf("the index of %d notes, %d prompts and %d skills was ready in %v", st.Memory, st.Prompts, st.Skills, took)

	if st.Memory != 2000 || st.Prompts != 500 || st.Skills != 500 {
		t.Fatalf("%+v", st)
	}

	// The budget is 2 s. On Windows every open of a file goes through the antivirus filter and costs milliseconds
	// (the same scan with no account, measured before sealing existed, took about 10 s serial and a third of that
	// side by side), so there the test only guards against a regression of the order of magnitude; the 2 s are
	// asserted where a file open is cheap.
	limit := 2 * time.Second
	if runtime.GOOS == "windows" {
		limit = 12 * time.Second
	}

	if took > limit {
		t.Errorf("the library of the maximum size took %v to be ready; the limit is %v", took, limit)
	}
}

// The scan reads scanWorkers skills side by side and seals the files of each one after the other, so no more than
// scanWorkers files are ever in memory at once; a file larger than any entry is sealed alone (epic 92 review: a pool
// inside each job held up to scanWorkers squared files).
func TestScanSealsAtMostScanWorkersFilesAtOnce(t *testing.T) {
	m, _, dir := newSealed(t)

	for i := range 2 * scanWorkers {
		n := fmt.Sprintf("skill-%03d", i)
		write(t, filepath.Join(dir, "skills", n, "SKILL.md"), "---\nname: "+n+"\ndescription: A skill.\n---\nBody\n")

		for j := range scanWorkers {
			write(t, filepath.Join(dir, "skills", n, "references", fmt.Sprintf("r%02d.md", j)), "a reference\n")
		}
	}

	big := strings.Repeat("x", largeSealFile+1)
	for i := range 4 {
		write(t, filepath.Join(dir, "memory", fmt.Sprintf("big-%d.md", i)), big)
	}

	sealsInFlight.peak.Store(0)

	if _, err := m.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}

	if peak := sealsInFlight.peak.Load(); peak > scanWorkers {
		t.Errorf("%d files were sealed at once; the bound is %d", peak, scanWorkers)
	}

	for rel, raw := range everyFile(t, dir) {
		if !account.IsSealedFile(raw) {
			t.Errorf("%s is still clear after the scan", rel)
		}
	}
}
