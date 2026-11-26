package testfs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestTempDirIsRemovedAfterTheTestClosesItsFiles(t *testing.T) {
	var dir string

	t.Run("inner", func(t *testing.T) {
		dir = TempDir(t)

		f, err := os.Create(filepath.Join(dir, "db"))
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = f.Close() })
	})

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
}

func TestTempDirNameKeepsOnlyWhatADatabaseURLAccepts(t *testing.T) {
	t.Run("seed#0/a b?c%d", func(t *testing.T) {
		base := filepath.Base(TempDir(t))
		if strings.ContainsAny(base, "#?% /\\") {
			t.Errorf("folder %q holds a character a database URL refuses", base)
		}
	})

	if got := folderName(strings.Repeat("x", 100)); len(got) != 40 {
		t.Errorf("a long name gave %d characters, want 40", len(got))
	}
}

func TestHeldEntryIsOnlyTheErrorsOfAHeldHandle(t *testing.T) {
	if heldEntry(os.ErrNotExist) {
		t.Error("a missing file is not a held entry")
	}

	if err := (&os.PathError{Op: "unlinkat", Path: "x", Err: syscall.ENOENT}); heldEntry(err) {
		t.Error("ENOENT is not a held entry")
	}
}

func TestLinkDirSkipsWhenTheLinkPathIsOccupied(t *testing.T) {
	root := t.TempDir()
	occupied := filepath.Join(root, "occupied")
	if err := os.WriteFile(occupied, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("occupied path", func(t *testing.T) {
		LinkDir(t, occupied, root)
		t.Fatal("LinkDir returned instead of skipping when no link could be made")
	})

	if data, err := os.ReadFile(occupied); err != nil || string(data) != "keep" {
		t.Fatalf("LinkDir changed the occupied path: %q, %v", data, err)
	}
}
