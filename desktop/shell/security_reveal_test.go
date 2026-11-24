package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/testfs"
)

// T-124 (SDD-007): the page that asks the shell to open a file is not trusted. Whatever path it sends to
// revealAsset, the shell starts the file manager only for a regular file under the media folder of its own
// workspace, checked by the shell itself with the links resolved; every other path ends in an error and in no
// process.
func TestSecurity_T124_TheShellChecksTheRevealPathItself(t *testing.T) {
	root, file := mediaFile(t)
	ws := filepath.Dir(root)

	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Another folder that has the media folder as a text prefix.
	evil := filepath.Join(ws, "store-evil")
	if err := os.MkdirAll(evil, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(evil, storedHash), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A link inside the media folder that leads out of it.
	linked := filepath.Join(root, "cd")
	testLinked := false

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, otherHash), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a link out of the media folder", func(t *testing.T) {
		testfs.LinkDir(t, linked, outside)

		testLinked = true
	})

	fm := &fakeManager{}
	r := Revealer{
		GOOS: "windows", Root: root, Start: fm.start,
		LookPath: func(name string) (string, error) { return name, nil },
	}

	hostile := []string{
		secret,
		filepath.Join(evil, storedHash),
		filepath.Join(root, "..", "store-evil", storedHash),
		filepath.Join(root, "ab", "..", "..", filepath.Base(secret)),
		root,
		filepath.Dir(file),
		"ab/" + storedHash,
		`\attacker\share\` + storedHash,
		"//attacker/share/" + storedHash,
		file + "\x00.txt",
		"",
		"C:",
	}

	if testLinked {
		hostile = append(hostile, filepath.Join(linked, otherHash))
	}

	for _, p := range hostile {
		if err := r.Reveal(p); err == nil {
			t.Errorf("Reveal(%q) was accepted", p)
		}
	}

	if len(fm.started) != 0 {
		t.Errorf("the file manager was started for a hostile path: %q", fm.started)
	}

	if err := r.Reveal(file); err != nil || len(fm.started) != 1 {
		t.Errorf("the stored file itself: %v, started %q", err, fm.started)
	}
}

// T-124: a path on another host is refused from its text, before the disk is asked: on Windows a stat of a UNC
// path is an SMB connection to that host, which offers it the credentials of the person. The refusal says the file
// is outside the media folder, not that it does not exist, which is what a stat would have answered.
func TestSecurity_T124_ARemotePathIsRefusedBeforeTheDiskIsAsked(t *testing.T) {
	root, _ := mediaFile(t)

	for _, p := range []string{
		`\attacker.invalid\share\ab\` + storedHash,
		`\?\UNC\attacker.invalid\share\ab\` + storedHash,
		"//attacker.invalid/share/ab/" + storedHash,
	} {
		if !filepath.IsAbs(p) {
			continue // not a path of this system
		}

		err := CheckRevealUnder(root, p)
		if err == nil || !strings.Contains(err.Error(), "outside the media folder") {
			t.Errorf("CheckRevealUnder(%q) = %v, want the refusal of a path outside the media folder", p, err)
		}
	}
}
