package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/internal/testfs"
)

func TestRevealCommand(t *testing.T) {
	tests := []struct {
		goos, path string
		name       string
		args       []string
	}{
		{"windows", `C:\out\a b.mp4`, "explorer.exe", []string{`/select,C:\out\a b.mp4`}},
		{"darwin", "/out/a.mp4", "open", []string{"-R", "/out/a.mp4"}},
		{"linux", "/out/a.mp4", "xdg-open", []string{filepath.Dir("/out/a.mp4")}},
	}
	for _, tc := range tests {
		name, args := RevealCommand(tc.goos, tc.path)
		if name != tc.name || !reflect.DeepEqual(args, tc.args) {
			t.Errorf("RevealCommand(%s) = %s %q; want %s %q", tc.goos, name, args, tc.name, tc.args)
		}
	}
}

// fakeManager is a file manager that records what it was asked to open: the test never opens the real one.
type fakeManager struct {
	started [][]string
	err     error
}

func (f *fakeManager) start(name string, args []string) error {
	f.started = append(f.started, append([]string{name}, args...))

	return f.err
}

// storedHash and otherHash are names a stored original has: 64 lowercase hexadecimal characters.
var (
	storedHash = strings.Repeat("ab", 32)
	otherHash  = strings.Repeat("cd", 32)
)

// mediaFile makes <workspace>/store/ab/<storedHash> and returns the media folder and the file.
func mediaFile(t *testing.T) (root, file string) {
	t.Helper()

	root = filepath.Join(t.TempDir(), "store")
	file = filepath.Join(root, "ab", storedHash)

	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	return root, file
}

func TestRevealerOpensAStoredFileThroughTheFileManager(t *testing.T) {
	root, file := mediaFile(t)
	fm := &fakeManager{}
	r := Revealer{
		GOOS: "windows", Root: root, Start: fm.start,
		LookPath: func(name string) (string, error) { return `C:\Windows\` + name, nil },
	}

	if err := r.Reveal(file); err != nil {
		t.Fatalf("Reveal = %v", err)
	}

	want := []string{`C:\Windows\explorer.exe`, `/select,` + file}
	if len(fm.started) != 1 || !reflect.DeepEqual(fm.started[0], want) {
		t.Errorf("started = %q, want %q", fm.started, want)
	}
}

// A desktop without a file manager (a minimal Linux, a stripped container) says so, and nothing is started.
func TestRevealerWithNoFileManagerSaysSo(t *testing.T) {
	root, file := mediaFile(t)
	fm := &fakeManager{}
	r := Revealer{
		GOOS: "linux", Root: root, Start: fm.start,
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	}

	err := r.Reveal(file)
	if err == nil || !strings.Contains(err.Error(), "no file manager") {
		t.Fatalf("Reveal = %v, want the sentence that no file manager was found", err)
	}

	if len(fm.started) != 0 {
		t.Errorf("something was started: %q", fm.started)
	}

	if strings.Contains(err.Error(), root) {
		t.Errorf("the error names a place: %v", err)
	}
}

func TestRevealerSaysWhenTheFileManagerCannotStart(t *testing.T) {
	root, file := mediaFile(t)
	fm := &fakeManager{err: errors.New("fork/exec " + file + ": permission denied")}
	r := Revealer{
		GOOS: "darwin", Root: root, Start: fm.start,
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}

	err := r.Reveal(file)
	if err == nil || strings.Contains(err.Error(), file) {
		t.Errorf("Reveal = %v, want an error that names no path", err)
	}
}

func TestCheckRevealUnderRefusesWhatIsNotAFileOfTheMediaFolder(t *testing.T) {
	root, file := mediaFile(t)
	ws := filepath.Dir(root)

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	sibling := filepath.Join(ws, "store-evil")
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}

	evil := filepath.Join(sibling, storedHash)
	if err := os.WriteFile(evil, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Files under the media folder whose name is not a stored original's: a partial import, a program.
	partial := filepath.Join(root, ".incoming", "import-1")
	program := filepath.Join(root, "ab", "run.exe")

	for _, p := range []string{partial, program} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte("s"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := CheckRevealUnder(root, file); err != nil {
		t.Errorf("a stored file was refused: %v", err)
	}

	dotDot := filepath.Join(root, "ab", "..", "..", "store-evil", storedHash)

	bad := map[string]string{
		"a file elsewhere": outside,
		"a sibling folder with the root as a text prefix": evil,
		"a dot dot path out of the root":                  dotDot,
		"the media folder itself":                         root,
		"a folder inside it":                              filepath.Dir(file),
		"a relative path":                                 filepath.Join("ab", storedHash),
		"an empty path":                                   "",
		"a path with a NUL":                               file + "\x00",
		"a file that is not there":                        filepath.Join(root, "ab", otherHash),
		"a partial import under the folder":               partial,
		"a program under the folder":                      program,
	}

	for name, p := range bad {
		err := CheckRevealUnder(root, p)
		if err == nil {
			t.Errorf("%s was accepted", name)

			continue
		}

		if p != "" && strings.Contains(err.Error(), p) || strings.Contains(err.Error(), ws) {
			t.Errorf("%s: the error names a place: %v", name, err)
		}
	}
}

func TestCheckRevealUnderRefusesALinkOutOfTheMediaFolder(t *testing.T) {
	root, file := mediaFile(t)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, storedHash), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}

	testfs.LinkDir(t, filepath.Dir(file), outside)

	if err := CheckRevealUnder(root, file); err == nil {
		t.Error("a file reached through a link that leaves the media folder was accepted")
	}
}

// The shell and the content store must agree on the media folder, or every reveal is refused (or, worse, allowed from
// the wrong folder).
func TestMediaRootIsTheFolderOfTheContentStore(t *testing.T) {
	ws := t.TempDir()

	if got, want := MediaRoot(ws), enginestore.New(ws).Dir(); got != want {
		t.Errorf("MediaRoot = %q, the content store is at %q", got, want)
	}
}

func TestCheckRevealPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckRevealPath(file); err != nil {
		t.Errorf("a real file was refused: %v", err)
	}
	for _, bad := range []string{"", "a.mp4", dir, filepath.Join(dir, "missing.mp4"), file + "\x00"} {
		if CheckRevealPath(bad) == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
