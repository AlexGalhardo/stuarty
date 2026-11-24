package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/internal/testfs"
)

func storeRootOf(t *testing.T, ws string) (root, file string) {
	t.Helper()

	p, err := enginestore.New(ws).Path(storedHex)
	if err != nil {
		t.Fatal(err)
	}

	return enginestore.New(ws).Dir(), p
}

func wantRevealError(t *testing.T, err *Error, code, reason string) {
	t.Helper()

	if err == nil {
		t.Fatalf("no error, want %s %s", code, reason)
	}

	if err.Code != code || err.Details["reason"] != reason {
		t.Errorf("error = %s %v, want %s %s", err.Code, err.Details, code, reason)
	}
}

// noPath fails when the error says where anything is on the disk.
func noPath(t *testing.T, err *Error, forbidden ...string) {
	t.Helper()

	raw, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatal(jerr)
	}

	for _, f := range forbidden {
		if strings.Contains(string(raw), f) || strings.Contains(string(raw), strings.ReplaceAll(f, `\`, `\\`)) {
			t.Errorf("the error carries a path %q: %s", f, raw)
		}
	}
}

func TestRevealResolvesAStoredFileUnderTheMediaRoot(t *testing.T) {
	t.Parallel()

	ws := stored(t)
	root, file := storeRootOf(t, ws)

	got, err := resolveUnderRoot(root, file)
	if err != nil {
		t.Fatal(err)
	}

	want, serr := filepath.EvalSymlinks(file)
	if serr != nil {
		t.Fatal(serr)
	}

	if got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}
}

func TestRevealRefusesALinkOutOfTheMediaRoot(t *testing.T) {
	t.Parallel()

	ws := stored(t)
	root, file := storeRootOf(t, ws)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, storedHex), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	// store/aa becomes a link to a folder outside the store that holds a file of the same name.
	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}

	testfs.LinkDir(t, filepath.Dir(file), outside)

	// A symbolic link resolves to outside the root; a Windows junction cannot be resolved: both are refused.
	_, err := resolveUnderRoot(root, file)
	if err == nil || err.Code != "E_PATH_NOT_ALLOWED" ||
		err.Details["reason"] != "outside_media_root" && err.Details["reason"] != "link_unresolved" {
		t.Fatalf("error = %+v, want E_PATH_NOT_ALLOWED", err)
	}

	noPath(t, err, outside, ws)
}

// symlinkDir is a real symbolic link: a junction is not one for filepath.EvalSymlinks, so the cases that need the
// links to be followed are skipped where the account may not make symbolic links.
func symlinkDir(t *testing.T, link, target string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks: %v", err) // verify:env-skip a Windows account may not make links
	}
}

func TestRevealFollowsALinkThatStaysInsideTheMediaRoot(t *testing.T) {
	t.Parallel()

	ws := stored(t)
	root, file := storeRootOf(t, ws)

	inside := filepath.Join(root, "bb")
	if err := os.MkdirAll(inside, 0o700); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(inside, storedHex)
	if err := os.Rename(file, moved); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}

	symlinkDir(t, filepath.Dir(file), inside)

	if _, err := resolveUnderRoot(root, file); err != nil {
		t.Errorf("a link inside the media root was refused: %v", err)
	}
}

func TestRevealAMediaRootThatIsItselfALinkStillWorks(t *testing.T) {
	t.Parallel()

	home := stored(t)
	realRoot, _ := storeRootOf(t, home)

	// The workspace is reached through a link (a folder moved to another disk): the file is still under the root.
	linked := filepath.Join(t.TempDir(), "store")
	symlinkDir(t, linked, realRoot)

	file := filepath.Join(linked, storedHex[:2], storedHex)
	if _, err := resolveUnderRoot(linked, file); err != nil {
		t.Errorf("a media root reached through a link was refused: %v", err)
	}
}

func TestRevealAFolderOrAMissingFileIsMissingAndNamesNoPath(t *testing.T) {
	t.Parallel()

	ws := t.TempDir()
	root, file := storeRootOf(t, ws)

	_, err := resolveUnderRoot(root, file)
	wantRevealError(t, err, "E_FILE_NOT_FOUND", "file_missing")
	noPath(t, err, ws, root, file)

	if mkerr := os.MkdirAll(file, 0o700); mkerr != nil {
		t.Fatal(mkerr)
	}

	_, err = resolveUnderRoot(root, file)
	wantRevealError(t, err, "E_FILE_NOT_FOUND", "file_missing")
}

func TestRevealAPathAboveTheRootOrTheRootItselfIsOutside(t *testing.T) {
	t.Parallel()

	ws := stored(t)
	root, _ := storeRootOf(t, ws)

	sibling := filepath.Join(ws, "notes.txt")
	if err := os.WriteFile(sibling, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{sibling, filepath.Join(root, "..", "notes.txt")} {
		_, err := resolveUnderRoot(root, p)
		wantRevealError(t, err, "E_PATH_NOT_ALLOWED", "outside_media_root")
	}

	_, err := resolveUnderRoot(root, root)
	if err == nil {
		t.Error("the media root itself was accepted as a file")
	}
}

// The containment rule on resolved paths, which is what a symbolic link that leads out of the store ends up as:
// it is checked here as a table because an ordinary Windows account cannot make a symbolic link to test it with.
func TestRevealUnderRootIsByThePathNeverByATextPrefix(t *testing.T) {
	t.Parallel()

	sep := string(filepath.Separator)
	base := filepath.Join(t.TempDir(), "ws")
	root := filepath.Join(base, "store")

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"a file of the store", filepath.Join(root, "aa", "aabb"), true},
		{"a file straight in the root", filepath.Join(root, "aabb"), true},
		{"the root itself", root, false},
		{"a sibling folder with the root as a text prefix", root + "-evil" + sep + "aabb", false},
		{"a sibling of the root", filepath.Join(base, "other", "aabb"), false},
		{"the parent", base, false},
		{"a file the link led to, elsewhere", filepath.Join(t.TempDir(), "secret"), false},
		{"dot dot out of the root", filepath.Join(root, "..", "secret"), false},
	}

	for _, tc := range cases {
		if got := underRoot(root, tc.target); got != tc.want {
			t.Errorf("%s: underRoot = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A workspace folder named with a right-to-left override: the path the file manager takes is untouched and the
// path a page draws has the character written out.
func TestRevealAPathWithBidirectionalCharactersIsShownIsolated(t *testing.T) {
	t.Parallel()

	override := string(rune(0x202E))

	ws := filepath.Join(t.TempDir(), "ws"+override+"fdp.exe")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Skipf("bad name: %v", err) // verify:env-skip a file system may refuse a bidi control in a name
	}

	root, file := storeRootOf(t, ws)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, rerr := resolveUnderRoot(root, file)
	if rerr != nil {
		t.Fatal(rerr)
	}

	if !strings.Contains(got, override) {
		t.Fatalf("the path lost the character: %q", got)
	}

	shown := asset.DisplayPath(got)
	if strings.Contains(shown, override) || !strings.Contains(shown, `\u{202E}`) {
		t.Errorf("display = %q: the override must be written out", shown)
	}
}
