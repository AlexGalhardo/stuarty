package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// formerFiles is a small workspace of the former name: the database with its write-ahead file, content
// addressed originals, an export, the token and a stale runtime.json.
var formerFiles = map[string]string{
	"db.sqlite":               "SQLite format 3 pretend database",
	"db.sqlite-wal":           "write-ahead bytes",
	"lock":                    "",
	"token":                   "vvl_secret",
	"runtime.json":            `{"schema":1,"pid":1}`,
	"store/ab/abcdef":         "an original",
	"exports/prj_1/job_1.mp4": "an export",
	"logs/vukem-video.log":    "old log",
}

func makeFormer(t *testing.T, home string) string {
	t.Helper()

	former := filepath.Join(home, formerDir)

	for name, body := range formerFiles {
		path := filepath.Join(former, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return former
}

// treeOf lists every file under dir with its content, relative and slash separated.
func treeOf(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(b)

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func fixedNow() time.Time { return time.Date(2026, 11, 18, 12, 30, 0, 0, time.UTC) }

func assertSameFiles(t *testing.T, got map[string]string, skip ...string) {
	t.Helper()

	for name, body := range formerFiles {
		if slices.Contains(skip, name) {
			continue
		}

		if got[name] != body {
			t.Errorf("%s = %q, want %q", name, got[name], body)
		}
	}
}

func readRecord(t *testing.T, dir string) AdoptionRecordFile {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(dir, AdoptionRecord))
	if err != nil {
		t.Fatal(err)
	}

	var rec AdoptionRecordFile
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}

	return rec
}

// TestAdoptFormerWorkspace is the adoption of ~/.vukem-video as ~/.stuarty (ADR-0044) in every case:
// the same volume, another volume, both folders present, and a former workspace that is refused.
func TestAdoptFormerWorkspace(t *testing.T) {
	t.Run("the same volume renames the folder and keeps a backup", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: current, Now: fixedNow})
		if err != nil || a == nil {
			t.Fatalf("AdoptFormer = %v, %v", a, err)
		}

		if a.Method != AdoptedByRename {
			t.Errorf("method = %s, want rename", a.Method)
		}

		if _, err := os.Stat(former); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the former folder is still there after a rename: %v", err)
		}

		got := treeOf(t, current)
		assertSameFiles(t, got)

		backup := "backups/adopt-20261118T123000Z/"
		if got[backup+"db.sqlite"] != formerFiles["db.sqlite"] ||
			got[backup+"db.sqlite-wal"] != formerFiles["db.sqlite-wal"] {
			t.Errorf("the backup of the database is missing or differs: %v", got)
		}

		if want := filepath.Join(current, "backups", "adopt-20261118T123000Z"); a.Backup != want {
			t.Errorf("backup = %s, want %s", a.Backup, want)
		}

		rec := readRecord(t, current)
		if rec.From != former || rec.To != current || rec.Method != AdoptedByRename || rec.Backup != a.Backup ||
			rec.AdoptedAt != "2026-11-18T12:30:00Z" {
			t.Errorf("record = %+v", rec)
		}

		if n := a.Notice(); !strings.Contains(n, former) || !strings.Contains(n, current) ||
			strings.Count(n, "\n") != 1 {
			t.Errorf("notice = %q", n)
		}

		// A second start finds the new folder and does nothing.
		again, err := AdoptFormer(AdoptOptions{Former: former, Current: current})
		if again != nil || err != nil {
			t.Errorf("second AdoptFormer = %v, %v, want nothing", again, err)
		}
	})

	t.Run("another volume copies, verifies and leaves a marker in the former folder", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)
		before := treeOf(t, former)

		crossing := func(string, string) error {
			return &os.LinkError{Op: "rename", Err: errCrossDevice}
		}

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: current, Rename: crossing, Now: fixedNow})
		if err != nil || a == nil {
			t.Fatalf("AdoptFormer = %v, %v", a, err)
		}

		if a.Method != AdoptedByCopy {
			t.Errorf("method = %s, want copy", a.Method)
		}

		got := treeOf(t, current)
		assertSameFiles(t, got, "runtime.json")

		if _, stale := got["runtime.json"]; stale {
			t.Error("the stale runtime.json was copied")
		}

		if got["backups/adopt-20261118T123000Z/db.sqlite"] != formerFiles["db.sqlite"] {
			t.Error("the backup is missing in the adopted workspace")
		}

		if _, err := os.Stat(current + adoptingSuffix); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the staging folder was left behind: %v", err)
		}

		// The former folder keeps every file it had, and says where the workspace went.
		left := treeOf(t, former)
		for name, body := range before {
			if left[name] != body {
				t.Errorf("former %s = %q, want it untouched (%q)", name, left[name], body)
			}
		}

		marker := left[MovedMarker]
		if !strings.Contains(marker, current) {
			t.Errorf("marker = %q, want it to name %s", marker, current)
		}

		if rec := readRecord(t, current); rec.Method != AdoptedByCopy {
			t.Errorf("record = %+v", rec)
		}

		if !strings.Contains(a.Notice(), "copied") {
			t.Errorf("notice = %q", a.Notice())
		}

		// A copied workspace is not adopted again, even if the new folder disappears.
		if err := os.RemoveAll(current); err != nil {
			t.Fatal(err)
		}

		if again, err := AdoptFormer(AdoptOptions{Former: former, Current: current}); again != nil || err != nil {
			t.Errorf("AdoptFormer after a copy = %v, %v, want nothing", again, err)
		}
	})

	t.Run("both folders present leaves both alone", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)

		if err := os.MkdirAll(current, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(current, "db.sqlite"), []byte("the new one"), 0o600); err != nil {
			t.Fatal(err)
		}

		if !HasFormerAlongside(former, current) {
			t.Error("HasFormerAlongside = false, want true")
		}

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: current})
		if a != nil || err != nil {
			t.Fatalf("AdoptFormer = %v, %v, want nothing", a, err)
		}

		assertSameFiles(t, treeOf(t, former))

		if got := treeOf(t, current); len(got) != 1 || got["db.sqlite"] != "the new one" {
			t.Errorf("the new workspace changed: %v", got)
		}
	})

	t.Run("a former workspace newer than supported is refused and untouched", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)
		newer := errors.New("written by version 9.9.9, this build reads up to 0.30.0")

		a, err := AdoptFormer(
			AdoptOptions{Former: former, Current: current, Check: func(string) error { return newer }},
		)
		if a != nil || !errors.Is(err, ErrAdoptionRefused) || !errors.Is(err, newer) {
			t.Fatalf("AdoptFormer = %v, %v, want ErrAdoptionRefused wrapping the reason", a, err)
		}

		got := treeOf(t, former)
		assertSameFiles(t, got)

		if len(got) != len(formerFiles) {
			t.Errorf("the refused folder gained files (a backup?): %v", got)
		}

		if _, err := os.Stat(current); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the new folder exists after a refusal: %v", err)
		}
	})

	t.Run("a former workspace that a process owns is not touched", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)

		lock, err := Dir(former).TryLock()
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: current})
		if a != nil || !errors.Is(err, ErrFormerInUse) {
			t.Fatalf("AdoptFormer = %v, %v, want ErrFormerInUse", a, err)
		}

		// The surfaces answer it as E_WORKSPACE_LOCKED, never as an internal error.
		if we, ok := errors.AsType[*Error](err); !ok || we.Code != CodeWorkspaceLocked || !we.Retryable ||
			!strings.Contains(we.Message, "close Vukem Video") || strings.Contains(we.Message, home) {
			t.Errorf("err = %#v, want a retryable %s that asks to close Vukem Video and names no folder",
				err, CodeWorkspaceLocked)
		}

		if _, err := os.Stat(current); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the new folder exists: %v", err)
		}
	})

	t.Run("a failed copy leaves no partial folder and the former one whole", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)

		// A symbolic link is not copied: the adoption stops instead of guessing.
		if err := os.Symlink(filepath.Join(former, "token"), filepath.Join(former, "link")); err != nil {
			t.Skipf( // verify:env-skip creating a symbolic link needs a privilege on Windows
				"no symbolic links here: %v",
				err,
			)
		}

		crossing := func(string, string) error { return &os.LinkError{Op: "rename", Err: errCrossDevice} }

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: current, Rename: crossing})
		if a != nil || err == nil {
			t.Fatalf("AdoptFormer = %v, %v, want an error", a, err)
		}

		for _, path := range []string{current, current + adoptingSuffix} {
			if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s exists after a failed copy: %v", path, err)
			}
		}

		assertSameFiles(t, treeOf(t, former))
	})

	t.Run("a rename that fails for another reason is reported and changes nothing", func(t *testing.T) {
		home := t.TempDir()
		former := makeFormer(t, home)
		current := filepath.Join(home, defaultDir)
		denied := errors.New("access is denied")

		a, err := AdoptFormer(AdoptOptions{
			Former: former, Current: current,
			Rename: func(string, string) error { return denied },
		})
		if a != nil || !errors.Is(err, denied) {
			t.Fatalf("AdoptFormer = %v, %v, want the failure of the rename", a, err)
		}

		if _, err := os.Stat(current); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the new folder exists: %v", err)
		}
	})

	t.Run("no former folder is nothing to do", func(t *testing.T) {
		home := t.TempDir()

		a, err := AdoptFormer(
			AdoptOptions{Former: filepath.Join(home, formerDir), Current: filepath.Join(home, defaultDir)},
		)
		if a != nil || err != nil {
			t.Errorf("AdoptFormer = %v, %v", a, err)
		}
	})

	t.Run("a workspace with no database is adopted with a notice that says so", func(t *testing.T) {
		home := t.TempDir()
		former := filepath.Join(home, formerDir)

		if err := os.MkdirAll(filepath.Join(former, "store"), 0o700); err != nil {
			t.Fatal(err)
		}

		a, err := AdoptFormer(AdoptOptions{Former: former, Current: filepath.Join(home, defaultDir)})
		if err != nil || a == nil || a.Backup != "" || !strings.Contains(a.Notice(), "no database") {
			t.Errorf("AdoptFormer = %+v, %v", a, err)
		}
	})
}

// TestResolveAdoptsTheFormerWorkspace is the first start: Resolve finds only ~/.vukem-video in a
// temporary home, adopts it, says so once, and keeps every file.
func TestResolveAdoptsTheFormerWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	former := makeFormer(t, home)
	noEnv := func(string) string { return "" }

	var said bytes.Buffer

	old := Notice
	Notice = &said

	t.Cleanup(func() { Notice = old })

	dir, err := Resolve(noEnv)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, defaultDir)
	if string(dir) != want {
		t.Fatalf("Resolve = %s, want %s", dir, want)
	}

	assertSameFiles(t, treeOf(t, want))

	if _, err := os.Stat(former); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the former folder is still there: %v", err)
	}

	first := said.String()
	if strings.Count(first, "\n") != 1 || !strings.Contains(first, "Vukem Video") {
		t.Errorf("notice = %q, want one line", first)
	}

	said.Reset()

	if _, err := Resolve(noEnv); err != nil {
		t.Fatal(err)
	}

	if said.Len() != 0 {
		t.Errorf("the second start printed %q, want nothing", said.String())
	}
}

// TestResolveKeepsAnExplicitHome: STUARTY_HOME is the person's choice; the former folder is not looked at.
func TestResolveKeepsAnExplicitHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	former := makeFormer(t, home)
	explicit := filepath.Join(t.TempDir(), "mine")

	var said bytes.Buffer

	old := Notice
	Notice = &said

	t.Cleanup(func() { Notice = old })

	dir, err := Resolve(func(name string) string {
		if name == EnvHome {
			return explicit
		}

		return ""
	})
	if err != nil || string(dir) != explicit {
		t.Fatalf("Resolve = %s, %v, want %s", dir, err, explicit)
	}

	assertSameFiles(t, treeOf(t, former))

	if said.Len() != 0 {
		t.Errorf("printed %q, want nothing", said.String())
	}
}

// TestResolveSaysWhenBothFoldersExist: the new workspace is used and the former one is named, so that a
// person who started Stuarty first can still find the old data.
func TestResolveSaysWhenBothFoldersExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	former := makeFormer(t, home)
	current := filepath.Join(home, defaultDir)

	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}

	var said bytes.Buffer

	old := Notice
	Notice = &said

	t.Cleanup(func() { Notice = old })

	dir, err := Resolve(func(string) string { return "" })
	if err != nil || string(dir) != current {
		t.Fatalf("Resolve = %s, %v", dir, err)
	}

	if !strings.Contains(said.String(), former) || !strings.Contains(said.String(), EnvHome) {
		t.Errorf("notice = %q, want it to name %s and %s", said.String(), former, EnvHome)
	}

	assertSameFiles(t, treeOf(t, former))
}

// TestAdoptFormerWorkspaceRefusesADamagedCopy: a copy that does not match the original is never put in
// place, whatever the system lets a test create (this case needs no symbolic link).
func TestAdoptFormerWorkspaceRefusesADamagedCopy(t *testing.T) {
	home := t.TempDir()
	former := makeFormer(t, home)
	current := filepath.Join(home, defaultDir)

	crossing := func(string, string) error { return &os.LinkError{Op: "rename", Err: errCrossDevice} }
	damage := func(staging string) error {
		return os.WriteFile(filepath.Join(staging, "store", "ab", "abcdef"), nil, 0o600)
	}

	a, err := AdoptFormer(AdoptOptions{Former: former, Current: current, Rename: crossing, afterCopy: damage})
	if a != nil || err == nil || !strings.Contains(err.Error(), "abcdef") {
		t.Fatalf("AdoptFormer = %v, %v, want the copy refused", a, err)
	}

	for _, path := range []string{current, current + adoptingSuffix} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s exists after a damaged copy: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(former, MovedMarker)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a marker was written for a copy that failed: %v", err)
	}

	assertSameFiles(t, treeOf(t, former), "backups")
}
