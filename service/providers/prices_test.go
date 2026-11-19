package providers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
)

func entry(p, m string, in int64) provider.PriceEntry {
	return provider.PriceEntry{
		Provider:            p,
		Model:               m,
		Currency:            "USD",
		InputMicrosPerMTok:  in,
		OutputMicrosPerMTok: in * 2,
	}
}

func TestPriceBookRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "providers", PriceFile)
	b := NewPriceBook(path)

	got, err := b.List()
	if err != nil || len(got) != 0 {
		t.Fatalf("a missing file is an empty table, got %v, %v", got, err)
	}

	if _, err := os.Stat(path); err == nil {
		t.Fatal("reading must not create the file")
	}

	for _, e := range []provider.PriceEntry{entry("openai", "m2", 2), entry("anthropic", "m1", 1), entry("openai", "m1", 3)} {
		if _, err := b.Set(e); err != nil {
			t.Fatal(err)
		}
	}

	// A new book over the same file sees the same table: it is on disk, sorted.
	list, err := NewPriceBook(path).List()
	if err != nil || len(list) != 3 {
		t.Fatalf("got %v, %v", list, err)
	}

	if list[0].Provider != "anthropic" || list[1].Model != "m1" || list[2].Model != "m2" {
		t.Errorf("the table must be sorted by provider and model, got %+v", list)
	}

	p, err := b.Lookup("openai", "m2")
	if err != nil || p == nil || p.InputMicrosPerMTok != 2 {
		t.Fatalf("lookup: %+v, %v", p, err)
	}

	if p, err := b.Lookup("openai", "nope"); err != nil || p != nil {
		t.Fatalf("a missing entry is nil, got %+v, %v", p, err)
	}

	// Setting the same model again replaces the entry.
	list, err = b.Set(entry("openai", "m2", 7))
	if err != nil || len(list) != 3 {
		t.Fatalf("got %v, %v", list, err)
	}

	if p, _ := b.Lookup("openai", "m2"); p.InputMicrosPerMTok != 7 {
		t.Errorf("got %+v", p)
	}

	list, err = b.Remove("openai", "m2")
	if err != nil || len(list) != 2 {
		t.Fatalf("got %v, %v", list, err)
	}

	if list, err = b.Remove("openai", "m2"); err != nil || len(list) != 2 {
		t.Fatalf("removing a missing entry is not an error: %v, %v", list, err)
	}
}

func TestPriceBookFileModeAndNoTempFileLeft(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "providers")
	b := NewPriceBook(filepath.Join(dir, PriceFile))

	if _, err := b.Set(entry("openai", "m", 1)); err != nil {
		t.Fatal(err)
	}

	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != PriceFile {
		t.Fatalf("only the table is left in the folder, got %v, %v", files, err)
	}

	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(dir, PriceFile)); info.Mode().Perm() != 0o600 {
			t.Errorf("the table is 0600, got %v", info.Mode().Perm())
		}
	}
}

func TestPriceBookRefusesABadFile(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"not json":      "{",
		"wrong format":  `{"format":"other","version":1,"prices":[]}`,
		"wrong version": `{"format":"stuarty-prices","version":2,"prices":[]}`,
		"bad entry":     `{"format":"stuarty-prices","version":1,"prices":[{"provider":"openai","model":"a b","currency":"USD"}]}`,
		"negative":      `{"format":"stuarty-prices","version":1,"prices":[{"provider":"openai","model":"m","currency":"USD","input_micros_per_mtok":-1}]}`,
	} {
		path := filepath.Join(t.TempDir(), PriceFile)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		b := NewPriceBook(path)
		if _, err := b.List(); !errors.Is(err, ErrBadPriceFile) {
			t.Errorf("%s: got %v", name, err)
		}

		// A corrupt table is never silently replaced: a later Set fails and the file is as it was.
		if _, err := b.Set(entry("openai", "m", 1)); !errors.Is(err, ErrBadPriceFile) {
			t.Errorf("%s: Set got %v", name, err)
		}

		if raw, _ := os.ReadFile(path); string(raw) != content {
			t.Errorf("%s: the file was changed", name)
		}
	}
}

func TestPriceBookIsBounded(t *testing.T) {
	t.Parallel()

	b := NewPriceBook(filepath.Join(t.TempDir(), PriceFile))

	for i := range provider.MaxPrices {
		if _, err := b.Set(entry("openai", "m"+strconv.Itoa(i), 1)); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}

	if _, err := b.Set(entry("openai", "one-more", 1)); !errors.Is(err, ErrTooManyPrices) {
		t.Fatalf("got %v", err)
	}

	// Replacing one of the 500 still works.
	if _, err := b.Set(entry("openai", "m0", 5)); err != nil {
		t.Fatal(err)
	}
}
