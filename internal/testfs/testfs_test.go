package testfs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestReleasedWaitsForTheLastHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	released := make(chan time.Time, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = f.Close()

		released <- time.Now()
	}()

	Released(t, path)

	done := time.Now()

	if runtime.GOOS == "windows" && done.Before(<-released) {
		t.Error("Released returned while the handle was open")
	}
}

func TestReleasedOfAMissingFileReturns(t *testing.T) {
	Released(t, filepath.Join(t.TempDir(), "none"))
}
