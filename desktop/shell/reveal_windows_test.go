//go:build windows

package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The forms a Windows path can take, as a hostile page would send them (SDD-007 T-124): a junction out of the media
// folder, a device path, a short 8.3 name and another case. Only the stored file itself, written in a way that
// still names it under the media folder, is accepted.

func TestCheckRevealUnderRefusesAJunctionOutOfTheMediaFolder(t *testing.T) {
	root, _ := mediaFile(t)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, otherHash), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A junction needs no privilege; mklink is part of cmd. Both paths are in the test's own folders.
	junction := filepath.Join(root, "cd")

	out, err := exec.CommandContext(context.Background(), "cmd", "/c", "mklink", "/J", junction, outside).
		CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}

	if err := CheckRevealUnder(root, filepath.Join(junction, otherHash)); err == nil {
		t.Error("a file reached through a junction that leaves the media folder was accepted")
	}
}

func TestCheckRevealUnderRefusesADevicePath(t *testing.T) {
	root, file := mediaFile(t)

	for _, p := range []string{`\\?\` + file, `\\.\` + file} {
		if err := CheckRevealUnder(root, p); err == nil {
			t.Errorf("the device path %q was accepted", p)
		}
	}
}

func TestCheckRevealUnderRefusesTheShortNameOfAFileOutside(t *testing.T) {
	root, _ := mediaFile(t)

	dir := filepath.Join(t.TempDir(), "a folder with a long name")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	secret := filepath.Join(dir, otherHash)
	if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	short := shortName(t, secret)

	if err := CheckRevealUnder(root, short); err == nil {
		t.Errorf("the short name %q of a file outside was accepted", short)
	}
}

func TestCheckRevealUnderTakesTheStoredFileInAnotherCase(t *testing.T) {
	root, file := mediaFile(t)

	// The volume and the folders in upper case; the name of the file stays the stored one.
	upper := strings.ToUpper(filepath.Dir(file)) + `\` + filepath.Base(file)

	if err := CheckRevealUnder(root, upper); err != nil {
		t.Errorf("the stored file written in another case was refused: %v", err)
	}
}

func shortName(t *testing.T, long string) string {
	t.Helper()

	p, err := syscall.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]uint16, syscall.MAX_PATH)

	n, err := syscall.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("no 8.3 names: %v", err) // verify:env-skip a volume may keep no short names
	}

	short := syscall.UTF16ToString(buf[:n])
	if short == long {
		t.Skip("no 8.3 names") // verify:env-skip a volume may keep no short names
	}

	return short
}
