package service

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// On Windows a rename over a file another goroutine has open fails with a
// sharing violation; a status poll reading installed.json must not break the
// install that replaces it.
func TestWriteFileAtomicWhileTheTargetIsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	if err := os.WriteFile(path, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool

	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				_, _ = os.ReadFile(path)

				time.Sleep(time.Millisecond)
			}
		})
	}

	defer func() {
		stop.Store(true)
		wg.Wait()
	}()

	for i := range 100 {
		if err := writeFileAtomic(path, []byte("[]")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
}
