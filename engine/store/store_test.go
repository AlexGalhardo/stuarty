package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestPutStoresByHashAndDeduplicates(t *testing.T) {
	ws := t.TempDir()
	s := New(ws)
	data := bytes.Repeat([]byte("Stuarty "), 1000)

	r, err := s.Put(context.Background(), bytes.NewReader(data), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	want := sum(data)
	if r.Hash != want || r.Size != int64(len(data)) || r.Deduplicated {
		t.Fatalf("result = %+v", r)
	}
	if r.Path != filepath.Join(ws, "store", want[:2], want) {
		t.Errorf("path = %s", r.Path)
	}
	got, err := os.ReadFile(r.Path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("stored bytes differ: %v", err)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(r.Path); st.Mode().Perm() != 0o400 {
			t.Errorf("mode = %v, want 0400", st.Mode().Perm())
		}
	} else if f, err := os.OpenFile(r.Path, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Error("the stored file is writable")
	}

	// The same bytes again: one object, the copy is discarded (ls store | wc -l is unchanged).
	before := entries(t, s.Dir())
	r2, err := s.Put(context.Background(), bytes.NewReader(data), 1<<20)
	if err != nil || !r2.Deduplicated || r2.Hash != want {
		t.Fatalf("second put = %+v, %v", r2, err)
	}
	if after := entries(t, s.Dir()); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("store listing changed: %v -> %v", before, after)
	}
	if left := entries(t, filepath.Join(s.Dir(), ".incoming")); len(left) != 0 {
		t.Errorf("copies left in .incoming: %v", left)
	}
	if n := len(entries(t, filepath.Join(s.Dir(), want[:2]))); n != 1 {
		t.Errorf("%d objects for one hash", n)
	}
	if !s.Has(want) || s.Has(sum([]byte("other"))) {
		t.Error("Has is wrong")
	}
}

func TestPutRefusesWhatSDD007Refuses(t *testing.T) {
	s := New(t.TempDir())
	ctx := context.Background()

	_, err := s.Put(ctx, bytes.NewReader(make([]byte, 101)), 100)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeLimitExceeded || e.Details["limit"] != "import_bytes" ||
		e.Details["ceiling"] != int64(100) ||
		e.Details["variable"] != "STUARTY_MAX_IMPORT_BYTES" {
		t.Errorf("over the limit: %v", err)
	}
	// Exactly max is fine.
	if _, err := s.Put(ctx, bytes.NewReader(bytes.Repeat([]byte{1}, 100)), 100); err != nil {
		t.Errorf("a file of exactly max bytes: %v", err)
	}
	if _, err := s.Put(
		ctx,
		bytes.NewReader(nil),
		100,
	); !errors.As(err, &e) || e.Code != CodeUnsupportedMedia ||
		e.Reason != "empty_file" {
		t.Errorf("empty: %v", err)
	}
	if _, err := s.Put(ctx, bytes.NewReader([]byte("short")), 100); !errors.As(err, &e) || e.Reason != "too_small" {
		t.Errorf("under 12 bytes: %v", err)
	}
	if left := entries(t, filepath.Join(s.Dir(), ".incoming")); len(left) != 0 {
		t.Errorf("a refused copy was left behind: %v", left)
	}
}

func TestPutStopsOnCancelAndReadErrors(t *testing.T) {
	s := New(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, bytes.NewReader(make([]byte, 64)), 100); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	boom := errors.New("boom")
	if _, err := s.Put(
		context.Background(),
		io.MultiReader(bytes.NewReader(make([]byte, 64)), iotest.ErrReader(boom)),
		1000,
	); !errors.Is(
		err,
		boom,
	) {
		t.Errorf("read error: %v", err)
	}
	if left := entries(t, filepath.Join(s.Dir(), ".incoming")); len(left) != 0 {
		t.Errorf("copies left: %v", left)
	}
}

func TestPathAndRemove(t *testing.T) {
	s := New(t.TempDir())
	for _, bad := range []string{"", "../x", strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("a", 65)} {
		if _, err := s.Path(bad); err == nil {
			t.Errorf("hash %q accepted", bad)
		}
	}
	r, err := s.Put(context.Background(), bytes.NewReader([]byte("some media bytes")), 100)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := s.Open(r.Hash); err != nil {
		t.Fatal(err)
	} else {
		_ = f.Close()
	}
	if err := s.Remove(r.Hash); err != nil {
		t.Fatalf("remove of a read-only file: %v", err)
	}
	if s.Has(r.Hash) {
		t.Error("still stored")
	}
	if _, err := os.Stat(filepath.Dir(r.Path)); err == nil {
		t.Error("the empty folder was kept")
	}
	if err := s.Remove(r.Hash); err != nil {
		t.Errorf("removing twice: %v", err)
	}
	var e *Error
	if _, err := s.Open(r.Hash); !errors.As(err, &e) || e.Code != CodeNotFound {
		t.Errorf("open of a missing file: %v", err)
	}
}

func TestSweeps(t *testing.T) {
	s := New(t.TempDir())
	ctx := context.Background()
	keep, _ := s.Put(ctx, bytes.NewReader([]byte("kept original")), 100)
	drop, _ := s.Put(ctx, bytes.NewReader([]byte("orphan original")), 100)

	old := filepath.Join(s.Dir(), ".incoming", "put-old")
	young := filepath.Join(s.Dir(), ".incoming", "put-young")
	for _, p := range []string{old, young} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if n, err := s.SweepIncoming(time.Now(), time.Hour); err != nil || n != 1 {
		t.Errorf("SweepIncoming = %d, %v", n, err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the old copy survived")
	}
	if _, err := os.Stat(young); err != nil {
		t.Error("the young copy was removed")
	}

	// Orphans are removed only when old enough and unknown.
	if n, _ := s.SweepOrphans(time.Now(), time.Hour, func(string) bool { return false }); n != 0 {
		t.Errorf("a fresh file was swept: %d", n)
	}
	later := time.Now().Add(2 * time.Hour)
	n, err := s.SweepOrphans(later, time.Hour, func(h string) bool { return h == keep.Hash })
	if err != nil || n != 1 || !s.Has(keep.Hash) || s.Has(drop.Hash) {
		t.Errorf("SweepOrphans = %d, %v (keep %v drop %v)", n, err, s.Has(keep.Hash), s.Has(drop.Hash))
	}
}

func TestErrorTextAndUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("disk full")
	e := &Error{Code: CodeNotFound, Message: "the file is not in the store", Err: cause}

	if got, want := e.Error(), CodeNotFound+": the file is not in the store: disk full"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(e, cause) {
		t.Error("Unwrap does not expose the cause")
	}

	if got := (&Error{Code: CodeNotFound, Message: "m"}).Error(); got != CodeNotFound+": m" {
		t.Errorf("Error() without a cause = %q", got)
	}
}

func TestStoreRefusesALinkInsideTheStore(t *testing.T) {
	s := New(t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, bytes.Repeat([]byte{7}, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sum([]byte("linked"))
	p, err := s.Path(hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, p); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if s.Has(hash) {
		t.Error("Has followed a link")
	}
	_, err = s.Open(hash)
	var e *Error
	if !errors.As(err, &e) || e.Reason != "not_regular" {
		t.Errorf("Open of a link: %v", err)
	}
}

func TestStoreRefusesAnythingThatIsNotARegularFile(t *testing.T) {
	s := New(t.TempDir())
	hash := sum([]byte("a folder"))
	p, err := s.Path(hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if s.Has(hash) {
		t.Error("Has accepted a folder")
	}
	var e *Error
	if _, err := s.Open(hash); !errors.As(err, &e) || e.Reason != "not_regular" {
		t.Errorf("Open of a folder: %v", err)
	}
}
