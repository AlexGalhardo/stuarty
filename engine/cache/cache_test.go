package cache

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func open(t *testing.T, limit int64) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cache"), limit)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestKeys(t *testing.T) {
	t.Parallel()
	if got := FrameKey("prj_a", 12, 30, 540); got != "frames/prj_a/12/30-540.jpg" {
		t.Error(got)
	}
	if got := SheetKey("prj_a", 12, "0123456789abcdef", 804); got != "frames/prj_a/12/sheet-0123456789ab-804.jpg" {
		t.Error(got)
	}
	if got := BitmapKey("ab12"); got != "bitmaps/ab12.png" {
		t.Error(got)
	}
}

func TestPutGetAndLeastRecentlyUsedEviction(t *testing.T) {
	t.Parallel()
	s := open(t, 100)
	blob := bytes.Repeat([]byte{'x'}, 30)
	for _, k := range []string{"frames/p/1/a.jpg", "frames/p/1/b.jpg", "bitmaps/c.png"} {
		if err := s.Put(k, blob); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := s.Get("frames/p/1/a.jpg"); !ok { // a is now the most recent; b the oldest
		t.Fatal("a is missing")
	}
	if err := s.Put("frames/p/1/d.jpg", blob); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("frames/p/1/b.jpg"); ok {
		t.Error("the least recently used file survived")
	}
	for _, k := range []string{"frames/p/1/a.jpg", "bitmaps/c.png", "frames/p/1/d.jpg"} {
		if _, ok := s.Get(k); !ok {
			t.Errorf("%s was evicted", k)
		}
	}
	if s.Size() != 90 {
		t.Errorf("size %d", s.Size())
	}
}

func TestReplacingAKeyDoesNotCountTwice(t *testing.T) {
	t.Parallel()
	s := open(t, 100)
	for range 5 {
		if err := s.Put("frames/p/1/a.jpg", bytes.Repeat([]byte{'x'}, 60)); err != nil {
			t.Fatal(err)
		}
	}
	if s.Size() != 60 {
		t.Errorf("size %d, want 60", s.Size())
	}
}

func TestKeysCannotLeaveTheRoot(t *testing.T) {
	t.Parallel()
	s := open(t, 100)
	for _, k := range []string{"", "..", "../x", "a/../../x", "/abs", `a\b`, "c:/x", "a//b", "./a"} {
		if err := s.Put(k, []byte("x")); !errors.Is(err, ErrKey) {
			t.Errorf("Put(%q) = %v", k, err)
		}
		if _, ok := s.Get(k); ok {
			t.Errorf("Get(%q) found something", k)
		}
	}
}

func TestOpenIndexesWhatIsThereAndEvictsToTheCap(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(filepath.Join(root, "frames", "p", "1"), 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)
	for i := range 5 {
		p := filepath.Join(root, "frames", "p", "1", strconv.Itoa(i)+"-540.jpg")
		if err := os.WriteFile(p, bytes.Repeat([]byte{'x'}, 40), 0o600); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	if s.Size() != 80 {
		t.Errorf("size %d, want 80 (the three oldest were evicted down to two files)", s.Size())
	}
	for i, want := range []bool{false, false, false, true, true} {
		if _, ok := s.Get("frames/p/1/" + strconv.Itoa(i) + "-540.jpg"); ok != want {
			t.Errorf("file %d present = %v, want %v", i, ok, want)
		}
	}
}

// T-19: an agent that asks for thousands of distinct frames cannot grow the cache past its cap, and
// frames and bitmaps share it.
func TestSecurity_T19_CacheIsCapped(t *testing.T) {
	t.Parallel()
	const limit = 64 << 10
	s := open(t, limit)
	blob := bytes.Repeat([]byte{'j'}, 1500)
	for i := range 3000 {
		key := FrameKey("prj_a", int64(i/100), i, 384)
		if i%3 == 0 {
			key = BitmapKey(strconv.Itoa(i))
		}
		if err := s.Put(key, blob); err != nil {
			t.Fatal(err)
		}
		if s.Size() > limit {
			t.Fatalf("after %d writes the cache holds %d bytes, cap %d", i+1, s.Size(), limit)
		}
	}
	var onDisk int64
	_ = filepath.WalkDir(s.root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, ierr := d.Info(); ierr == nil {
				onDisk += info.Size()
			}
		}
		return nil
	})
	if onDisk > limit {
		t.Errorf("%d bytes on disk, cap %d", onDisk, limit)
	}
	if err := s.Put("frames/p/0/huge.jpg", bytes.Repeat([]byte{'j'}, limit+1)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("frames/p/0/huge.jpg"); ok || s.Size() > limit {
		t.Error("a file larger than the cap was stored")
	}
}

// The root may be the workspace: only frames/ and bitmaps/ are the cache's, so
// the database next to them is never indexed and never evicted.
func TestOpenIndexesOnlyTheCacheFolders(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"db.sqlite": "database", "frames/p/1/0-8.jpg": "jpeg", "bitmaps/k.png": "png"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "db.sqlite")); err != nil {
		t.Errorf("the database was evicted: %v", err)
	}
	if s.Size() != 0 {
		t.Errorf("size = %d, want 0 after evicting both files under a 1-byte cap", s.Size())
	}
}
