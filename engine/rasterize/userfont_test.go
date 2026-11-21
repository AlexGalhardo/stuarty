package rasterize

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func bundledBytes(tb testing.TB, name string) []byte {
	tb.Helper()

	data, err := os.ReadFile(filepath.Join(fontDir, name))
	if err != nil {
		tb.Fatal(err)
	}

	return data
}

type craftedTable struct {
	tag  string
	data []byte
}

// craftFont assembles an sfnt with the given tables, laid out one after the
// other and 4-byte aligned. The tables hold junk: only the structure is real.
func craftFont(magic string, tables []craftedTable) []byte {
	n := len(tables)
	out := append([]byte(nil), make([]byte, 12+16*n)...)
	copy(out, magic)
	binary.BigEndian.PutUint16(out[4:], uint16(n))

	for i, tb := range tables {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}

		rec := out[12+16*i:]
		copy(rec, tb.tag)
		binary.BigEndian.PutUint32(rec[8:], uint32(len(out)))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(tb.data)))
		out = append(out, tb.data...)
	}

	return out
}

func junkTables(extra ...craftedTable) []craftedTable {
	tbs := []craftedTable{}
	for _, tag := range []string{"cmap", "head", "hhea", "hmtx", "maxp", "glyf"} {
		tbs = append(tbs, craftedTable{tag, bytes.Repeat([]byte{0xAB}, 40)})
	}

	return append(tbs, extra...)
}

func wantReason(t *testing.T, err error, reason string) {
	t.Helper()

	var inv *InvalidFontError
	if !errors.As(err, &inv) {
		t.Fatalf("error = %v, want *InvalidFontError with reason %s", err, reason)
	}

	if inv.Reason != reason {
		t.Fatalf("reason = %s (%v), want %s", inv.Reason, err, reason)
	}

	if inv.Code() != "E_VALIDATION" {
		t.Fatalf("code = %s", inv.Code())
	}
}

func TestValidateFontAcceptsTheBundledFonts(t *testing.T) {
	t.Parallel()

	for _, f := range bundledFamilies {
		if err := ValidateFont(bundledBytes(t, f.file)); err != nil {
			t.Errorf("%s: %v", f.file, err)
		}
	}
}

func TestValidateFontRefusesCraftedFonts(t *testing.T) {
	t.Parallel()

	anton := bundledBytes(t, "Anton-Regular.ttf")

	overlap := craftFont("\x00\x01\x00\x00", junkTables())
	// Make the hmtx record start inside the cmap table.
	cmapOff := binary.BigEndian.Uint32(overlap[12+8:])
	binary.BigEndian.PutUint32(overlap[12+16*3+8:], cmapOff+4)

	outside := craftFont("\x00\x01\x00\x00", junkTables())
	binary.BigEndian.PutUint32(outside[12+16*5+12:], 1<<30)

	hugeNum := make([]byte, 64)
	copy(hugeNum, "\x00\x01\x00\x00")
	binary.BigEndian.PutUint16(hugeNum[4:], 65535)

	wrapped := craftFont("\x00\x01\x00\x00", junkTables())
	binary.BigEndian.PutUint32(wrapped[12+8:], 0xFFFFFFF0)
	binary.BigEndian.PutUint32(wrapped[12+12:], 0x20)

	collection := make([]byte, 16)
	copy(collection, "ttcf")
	binary.BigEndian.PutUint32(collection[8:], 100000)

	tests := []struct {
		name   string
		data   []byte
		reason string
	}{
		{"truncated", anton[:len(anton)/2], FontTableOutOfRange},
		{"truncated inside the directory", anton[:40], FontTruncated},
		{"too short", []byte("OTTO"), FontTruncated},
		{"overlapping tables", overlap, FontTablesOverlap},
		{"table past the end", outside, FontTableOutOfRange},
		{"offset plus length wraps 32 bits", wrapped, FontTableOutOfRange},
		{"numTables 65535", hugeNum, FontTooManyTables},
		{"collection of 100000 fonts", collection, FontTooManyTables},
		{"bad magic", append([]byte("wOFF"), anton[4:]...), FontBadMagic},
		{"no glyph table", craftFont("\x00\x01\x00\x00", junkTables()[:5]), FontMissingTable},
		{"no cmap", craftFont("OTTO", junkTables()[1:]), FontMissingTable},
		{
			"duplicate table",
			craftFont("true", junkTables(craftedTable{"cmap", []byte{1, 2, 3, 4}})),
			FontDuplicateTable,
		},
		{"too large", make([]byte, MaxFontBytes+1), FontTooLarge},
		{"right structure, junk tables", craftFont("\x00\x01\x00\x00", junkTables()), FontParseFailed},
	}

	// The service warms the module at start; the deadline is for refusing a font, not for the compile.
	Warm()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start := time.Now()
			err := ValidateFont(tc.data)

			if d := time.Since(start); d > FontParseDeadline {
				t.Errorf("refusing took %s, more than the %s deadline", d, FontParseDeadline)
			}

			wantReason(t, err, tc.reason)
		})
	}
}

// lookup draws one letter in the family and weight, which is how a font set resolves a face.
func lookup(set *FontSet, family string, weight int) error {
	st := plainStyle()
	st.FontFamily, st.FontWeight = family, weight

	_, err := Render(textSpec("x", st), set)

	return err
}

func hexOf(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func memorySource(files map[string][]byte) FontSource {
	return func(hash string) ([]byte, error) {
		if data, ok := files[hash]; ok {
			return data, nil
		}

		return nil, ErrFontNotStored
	}
}

func brandFonts(hash string) map[string]timeline.ProjectFont {
	return map[string]timeline.ProjectFont{
		"Brand Display": {Files: []timeline.FontFile{{Weight: 700, Hash: "sha256:" + hash}}},
	}
}

func TestFontSetLookupUserFonts(t *testing.T) {
	t.Parallel()

	data := bundledBytes(t, "BebasNeue-Regular.ttf")
	hash := hexOf(data)
	set := NewFontSet(brandFonts(hash), memorySource(map[string][]byte{hash: data}))

	if err := lookup(set, "Brand Display", 700); err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if err := lookup(set, "Inter", 400); err != nil {
		t.Fatalf("a bundled family still resolves: %v", err)
	}

	var fe *FontError

	err := lookup(set, "Brand Display", 400)
	if !errors.As(err, &fe) || fe.Code != CodeFontUnknown || len(fe.Weights) != 1 || fe.Weights[0] != 700 {
		t.Fatalf("unknown weight: %v", err)
	}

	err = lookup(set, "Nope", 400)
	if !errors.As(err, &fe) || fe.Code != CodeFontUnknown {
		t.Fatalf("unknown family: %v", err)
	}

	missing := NewFontSet(brandFonts(hash), memorySource(nil))

	err = lookup(missing, "Brand Display", 700)
	if !errors.As(err, &fe) || fe.Code != CodeFontUnknown || fe.Hash != hash {
		t.Fatalf("missing file: %v", err)
	}

	if err := lookup(Bundled(), "Brand Display", 700); err == nil {
		t.Fatal("the bundled set must not know a user family")
	}
}

func TestFontSetRefusesAFileThatIsNotWhatItsHashSays(t *testing.T) {
	t.Parallel()

	data := bundledBytes(t, "BebasNeue-Regular.ttf")
	hash := hexOf(data)
	other := bundledBytes(t, "Anton-Regular.ttf")
	set := NewFontSet(brandFonts(hash), memorySource(map[string][]byte{hash: other}))

	if err := lookup(set, "Brand Display", 700); err == nil {
		t.Fatal("a file with another hash was used")
	}

	junk := craftFont("\x00\x01\x00\x00", junkTables())
	set = NewFontSet(brandFonts(hexOf(junk)), memorySource(map[string][]byte{hexOf(junk): junk}))

	err := lookup(set, "Brand Display", 700)
	wantReason(t, err, FontParseFailed)
}

// TestUserFontGolden is the "valid OFL font used in a caption" of P3-RND-006:
// Bebas Neue (OFL) arrives through the user font path under a family name of
// the document, so the bytes of the bitmap prove the whole path.
func TestUserFontGolden(t *testing.T) {
	t.Parallel()

	data := bundledBytes(t, "BebasNeue-Regular.ttf")
	hash := hexOf(data)
	set := NewFontSet(brandFonts(hash), memorySource(map[string][]byte{hash: data}))

	st := goldenStyle(func(s *plan.ResolvedStyle) {
		s.FontFamily, s.FontWeight, s.Size, s.StrokeWidth = "Brand Display", 700, 64, 5
	})

	got, err := Render(captionSpec(neverWords, 2, st), set)
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(goldenDir, "user-font-caption.png")

	if *update {
		raw, err := got.PNG()
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(file, raw, 0o644); err != nil {
			t.Fatal(err)
		}

		return
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v (run: go test ./engine/rasterize -run UserFontGolden -update)", err)
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	want, ok := img.(*image.NRGBA)
	if !ok || want.Bounds().Dx() != got.Width || want.Bounds().Dy() != got.Height {
		t.Fatalf("golden is %T %v, bitmap is %dx%d", img, img.Bounds(), got.Width, got.Height)
	}

	if !bytes.Equal(got.Pix, want.Pix) {
		t.Errorf("pixels differ from %s (sha256 %s)", file, hashOf(got))
	}
}

func FuzzFontHeader(f *testing.F) {
	anton := bundledBytes(f, "Anton-Regular.ttf")
	f.Add(anton[:512])
	f.Add(anton[:4096])
	f.Add(craftFont("\x00\x01\x00\x00", junkTables()))
	f.Add(craftFont("OTTO", junkTables()[1:]))
	f.Add([]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The validation reads attacker numbers and must return, in time, for any input.
		start := time.Now()
		_ = ValidateFont(data)

		if d := time.Since(start); d > FontParseDeadline+time.Second {
			t.Fatalf("validation took %s", d)
		}
	})
}
