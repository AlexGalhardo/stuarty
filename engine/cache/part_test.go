package cache

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"testing"
)

func TestCommitStoresAWrittenFileAndCountsIt(t *testing.T) {
	t.Parallel()

	s := open(t, 1000)
	key := AudioPreviewKey("a" + string(bytes.Repeat([]byte{'0'}, 63)))

	if s.Has(key) {
		t.Fatal("a key that was never written is there")
	}

	part, err := s.PartPath(key)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(part, bytes.Repeat([]byte{'w'}, 300), 0o600); err != nil {
		t.Fatal(err)
	}

	if s.Has(key) {
		t.Error("a file that was not committed is there")
	}

	if err := s.Commit(key); err != nil {
		t.Fatal(err)
	}

	if !s.Has(key) || s.Size() != 300 {
		t.Errorf("has = %v, size = %d", s.Has(key), s.Size())
	}

	if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the .part file is still there: %v", err)
	}
}

func TestCommitEvictsAndRefusesWhatIsTooBig(t *testing.T) {
	t.Parallel()

	s := open(t, 500)

	write := func(i, n int) error {
		key := AudioPreviewKey(strconv.Itoa(i))

		part, err := s.PartPath(key)
		if err != nil {
			return err
		}

		if err := os.WriteFile(part, bytes.Repeat([]byte{'w'}, n), 0o600); err != nil {
			return err
		}

		return s.Commit(key)
	}

	for i := range 10 {
		if err := write(i, 200); err != nil {
			t.Fatal(err)
		}

		if s.Size() > 500 {
			t.Fatalf("the cache holds %d bytes, cap 500", s.Size())
		}
	}

	if s.Has(AudioPreviewKey("0")) || !s.Has(AudioPreviewKey("9")) {
		t.Error("the least recently used file did not go first")
	}

	if err := write(99, 501); err == nil {
		t.Error("a file over the cap was stored")
	}

	for _, k := range []string{"../x", "audio-previews/../../x", `a\b`} {
		if _, err := s.PartPath(k); !errors.Is(err, ErrKey) {
			t.Errorf("PartPath(%q) = %v", k, err)
		}
	}
}
