package testfs

import (
	"os"
	"path/filepath"
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

func TestTempDirNameHasNoPathSeparator(t *testing.T) {
	t.Run("a/b", func(t *testing.T) {
		if base := filepath.Base(TempDir(t)); base == "" {
			t.Error("no folder")
		}
	})
}

func TestHeldEntryIsOnlyTheErrorsOfAHeldHandle(t *testing.T) {
	if heldEntry(os.ErrNotExist) {
		t.Error("a missing file is not a held entry")
	}

	if err := (&os.PathError{Op: "unlinkat", Path: "x", Err: syscall.ENOENT}); heldEntry(err) {
		t.Error("ENOENT is not a held entry")
	}
}
