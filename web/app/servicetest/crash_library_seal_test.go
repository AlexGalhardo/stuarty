//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/library"
)

const (
	libraryDirEnv     = "STUARTY_CRASH_LIBRARY_DIR"
	libraryCrashPoint = "library_seal.after_file"
)

func crashKey() *account.KeySealer {
	ks, err := account.NewKeySealer(bytes.Repeat([]byte{5}, account.KeySize))
	if err != nil {
		panic(err)
	}

	return ks
}

func crashLibraryFiles(dir string) *library.Manager {
	return library.New(dir, library.NewMemIndex(), nil, library.WithFiles(crashKey()))
}

// TestCrashLibrarySealChild is the body of the child of TestCrashLibrarySeal.
func TestCrashLibrarySealChild(t *testing.T) {
	if os.Getenv(roleEnv) != "library-seal-child" {
		t.Skip("only runs as the child of a crash test")
	}

	_, err := crashLibraryFiles(os.Getenv(libraryDirEnv)).SealAll(context.Background())

	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

// SDD-005 section 5.17.4: a process killed in the middle of the sealing of the library leaves every file whole,
// some sealed and some clear; the next run seals the rest, and every file reads back as it was.
func TestCrashLibrarySeal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "library")
	want := map[string]string{}

	for i := range 6 {
		rel := fmt.Sprintf("memory/n%d.md", i)
		text := fmt.Sprintf("---\nname: n%d\n---\nCANARY-CRASH-%d\n", i, i)
		want[rel] = text

		if err := os.MkdirAll(filepath.Join(dir, "memory"), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, rel), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashLibrarySealChild$", "-test.v")
	cmd.Env = append(os.Environ(), roleEnv+"=library-seal-child", libraryDirEnv+"="+dir,
		crashpoint.CrashEnv+"="+libraryCrashPoint)

	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out

	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 137 {
		t.Fatalf("child = %v, want exit 137\n%s", err, out.String())
	}

	// After the kill: every file is whole (sealed, or still the clear text), none is half written, and a temporary
	// name is at most a sealed leftover.
	sealed := 0

	for rel, text := range want {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s is gone after the kill: %v", rel, err)
		}

		switch {
		case account.IsSealedFile(raw):
			sealed++
		case string(raw) != text:
			t.Fatalf("%s is neither sealed nor what it was: %q", rel, raw)
		}
	}

	// The files are sealed side by side, so how many were done at the kill is not fixed; at least the one that
	// reached the point is.
	if sealed == 0 {
		t.Fatalf("the kill is at a point after a file was sealed, but none is: %d of %d", sealed, len(want))
	}

	// The next run finishes it.
	m := crashLibraryFiles(dir)

	left, err := m.SealAll(context.Background())
	if err != nil || left != len(want)-sealed {
		t.Fatalf("the next run sealed %d (%v), want the %d that were left", left, err, len(want)-sealed)
	}

	for rel, text := range want {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || !account.IsSealedFile(raw) || strings.Contains(string(raw), "CANARY-CRASH") {
			t.Fatalf("%s after the second run: sealed=%v %v", rel, account.IsSealedFile(raw), err)
		}

		back, err := crashKey().OpenFile(rel, raw)
		if err != nil || string(back) != text {
			t.Fatalf("%s reads back as %q (%v)", rel, back, err)
		}
	}
}
