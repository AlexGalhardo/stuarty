package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

func antonBytes(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "renderer", "fonts", "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestFontsPutReadRoundTrip(t *testing.T) {
	t.Parallel()

	data := antonBytes(t)
	fonts := NewFonts(t.TempDir())

	hash, err := fonts.Put(data)
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(data)
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %s", hash)
	}

	if !fonts.Has(hash) {
		t.Fatal("Has is false after Put")
	}

	got, err := fonts.Read(hash)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Read: %v", err)
	}

	if again, err := fonts.Put(data); err != nil || again != hash {
		t.Fatalf("second Put: %v %s", err, again)
	}

	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(fonts.Dir(), hash))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v", st.Mode(), err)
		}
	}

	entries, _ := os.ReadDir(fonts.Dir())
	if len(entries) != 1 {
		t.Fatalf("%d entries in fonts/, want only the font", len(entries))
	}
}

func TestFontsReadMissingAndCorrupt(t *testing.T) {
	t.Parallel()

	fonts := NewFonts(t.TempDir())

	if _, err := fonts.Read(
		hex.EncodeToString(make([]byte, 32)),
	); !errors.Is(err, rasterize.ErrFontNotStored) ||
		!errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}

	if _, err := fonts.Read("../x"); err == nil {
		t.Fatal("a path was accepted as a key")
	}

	hash, err := fonts.Put(antonBytes(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(fonts.Dir(), hash), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := fonts.Read(hash); err == nil {
		t.Fatal("a file that does not match its hash was returned")
	}
}

// TestSecurity_T21_FontValidation: a crafted font is refused within 5 seconds
// and nothing is stored (SDD-007 section 15 check 21).
func TestSecurity_T21_FontValidation(t *testing.T) {
	t.Parallel()

	anton := antonBytes(t)

	huge := make([]byte, 64)
	copy(huge, "\x00\x01\x00\x00")
	binary.BigEndian.PutUint16(huge[4:], 65535)

	// Two tables that share bytes: the first table is made to cover the second.
	overlap := append([]byte(nil), anton...)
	binary.BigEndian.PutUint32(overlap[12+12:], uint32(len(anton)))

	tests := map[string][]byte{
		"truncated":         anton[:len(anton)/3],
		"numTables 65535":   huge,
		"overlapping":       overlap,
		"not a font":        []byte("this is not a font file at all"),
		"too large":         make([]byte, rasterize.MaxFontBytes+1),
		"empty":             nil,
		"magic only":        []byte("OTTO\x00\x00\x00\x00\x00\x00\x00\x00"),
		"wrong magic, body": append([]byte("wOFF"), anton[4:]...),
	}

	// The service compiles the renderer module at start (go rasterize.Warm()); the deadline is
	// the time to refuse a font, not the one-off compile, which takes 7 to 11 s under -race on CI.
	rasterize.Warm()

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fonts := NewFonts(t.TempDir())
			start := time.Now()

			hash, err := fonts.Put(data)

			if _, ok := errors.AsType[*rasterize.InvalidFontError](err); !ok {
				t.Fatalf("Put = %q, %v; want *rasterize.InvalidFontError", hash, err)
			}

			if d := time.Since(start); d > rasterize.FontParseDeadline {
				t.Errorf("refused after %s", d)
			}

			if entries, _ := os.ReadDir(fonts.Dir()); len(entries) != 0 {
				t.Errorf("%d files stored for a refused font", len(entries))
			}
		})
	}
}
