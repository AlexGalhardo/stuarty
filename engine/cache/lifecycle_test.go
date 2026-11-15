package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The cap is 8 MiB and 20 MiB of distinct frames are written: the cache ends at the cap or below,
// with the most recently used entries kept.
func TestCacheCapped(t *testing.T) {
	t.Parallel()

	const (
		mib   = 1 << 20
		limit = 8 * mib
	)

	s := open(t, limit)
	blob := bytes.Repeat([]byte{'f'}, mib)

	for i := range 20 {
		if err := s.Put(FrameKey("prj_a", 1, i, 1080), blob); err != nil {
			t.Fatal(err)
		}

		if s.Size() > limit+mib {
			t.Fatalf("after %d writes the cache holds %d bytes", i+1, s.Size())
		}
	}

	if s.Size() > limit {
		t.Errorf("%d bytes held, cap %d", s.Size(), limit)
	}

	for i := range 12 {
		if _, ok := s.Get(FrameKey("prj_a", 1, i, 1080)); ok {
			t.Errorf("frame %d is old and should be gone", i)
		}
	}

	for i := 12; i < 20; i++ {
		if _, ok := s.Get(FrameKey("prj_a", 1, i, 1080)); !ok {
			t.Errorf("frame %d is recent and was evicted", i)
		}
	}
}

// Eviction removes only files under the cache root: the store, the artifacts and the exports of
// the workspace are all still there.
func TestEvictionNeverTouchesStore(t *testing.T) {
	t.Parallel()

	ws := t.TempDir()
	keep := map[string][]byte{}

	for _, dir := range []string{"store/ab", "artifacts/abcd", "exports/prj_a"} {
		if err := os.MkdirAll(filepath.Join(ws, dir), 0o700); err != nil {
			t.Fatal(err)
		}

		p := filepath.Join(ws, dir, "file")
		keep[p] = []byte("precious " + dir)

		if err := os.WriteFile(p, keep[p], 0o600); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(filepath.Join(ws, "cache"), 64<<10)
	if err != nil {
		t.Fatal(err)
	}

	blob := bytes.Repeat([]byte{'x'}, 4<<10)
	for i := range 200 {
		if err := s.Put(BitmapKey(strconv.Itoa(i)), blob); err != nil {
			t.Fatal(err)
		}
	}

	for p, want := range keep {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s is gone or changed: %v", p, err)
		}
	}
}

// After 8 revisions only the current and the previous 5 directories remain.
func TestRevisionDirsPruned(t *testing.T) {
	t.Parallel()

	s := open(t, 0)
	blob := []byte("jpeg")

	for rev := int64(1); rev <= 8; rev++ {
		if err := s.Put(FrameKey("prj_a", rev, 0, 540), blob); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Put(FrameKey("prj_b", 1, 0, 540), blob); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(s.root, "frames", "prj_a"))
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}

	if want := []string{"3", "4", "5", "6", "7", "8"}; len(got) != len(want) {
		t.Fatalf("revisions left: %v, want %v", got, want)
	}

	if _, ok := s.Get(FrameKey("prj_a", 2, 0, 540)); ok {
		t.Error("revision 2 is still readable")
	}

	if _, ok := s.Get(FrameKey("prj_a", 3, 0, 540)); !ok {
		t.Error("revision 3 was pruned")
	}

	if _, ok := s.Get(FrameKey("prj_b", 1, 0, 540)); !ok {
		t.Error("another project lost its frame")
	}

	var held int64

	s.mu.Lock()
	for _, el := range s.items {
		held += sizeOf(el)
	}
	used := s.used
	s.mu.Unlock()

	if held != used {
		t.Errorf("accounting drifted: items hold %d, used says %d", held, used)
	}
}

// A file under a lease survives the cap and the pruning until the lease is released.
func TestLeasedFileSurvivesEviction(t *testing.T) {
	t.Parallel()

	const limit = 4 << 10

	s := open(t, limit)
	blob := bytes.Repeat([]byte{'x'}, 1<<10)
	old := FrameKey("prj_a", 1, 0, 540)

	if err := s.Put(old, blob); err != nil {
		t.Fatal(err)
	}

	release := s.Lease(old)

	for i := 1; i < 20; i++ {
		if err := s.Put(FrameKey("prj_a", 9, i, 540), blob); err != nil {
			t.Fatal(err)
		}
	}

	// Revision 1 is also far older than the 5 kept; the lease holds it anyway.
	for rev := int64(2); rev <= 8; rev++ {
		if err := s.Put(FrameKey("prj_a", rev, 0, 540), []byte("j")); err != nil {
			t.Fatal(err)
		}
	}

	if _, ok := s.Get(old); !ok {
		t.Fatal("a leased file was evicted")
	}

	release()
	release()

	for i := 20; i < 30; i++ {
		if err := s.Put(FrameKey("prj_a", 9, i, 540), blob); err != nil {
			t.Fatal(err)
		}
	}

	if _, ok := s.Get(old); ok {
		t.Error("the file was kept after its lease ended")
	}
}
