//go:build windows

package testfs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A handle that is never closed is a leak, not a scanner: the removal gives up after its limit and returns the
// error of the held entry, so TempDir fails the test instead of hiding it.
func TestRemoveAllGivesUpOnAHandleThatIsNeverClosed(t *testing.T) {
	dir, err := os.MkdirTemp("", "testfs-leak-") //nolint:usetesting // the folder outlives the removal under test
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = f.Close()
		_ = os.RemoveAll(dir)
	})

	waited, err := removeAll(dir, 300*time.Millisecond)
	if err == nil || !heldEntry(err) {
		t.Fatalf("removeAll with an open file = %v, want the error of a held entry", err)
	}

	// It stops when the next pause, at most 100 ms, would pass the limit.
	if waited < 200*time.Millisecond {
		t.Errorf("removeAll gave up after %v, before its limit", waited)
	}
}

// A handle closed while the removal retries is the short hold the package exists for: the folder goes.
func TestRemoveAllWaitsForAHandleClosedLater(t *testing.T) {
	dir, err := os.MkdirTemp("", "testfs-hold-") //nolint:usetesting // the folder is what removeAll removes
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = f.Close()
		_ = os.RemoveAll(dir)
	})

	old := pause
	t.Cleanup(func() { pause = old })

	tries := 0
	pause = func(time.Duration) {
		tries++
		if tries == 3 {
			_ = f.Close()
		}
	}

	if _, err := removeAll(dir, 10*time.Second); err != nil {
		t.Fatalf("removeAll after the handle was closed: %v", err)
	}

	if tries < 3 {
		t.Errorf("the folder went after %d tries, while its file was still open", tries)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
}
