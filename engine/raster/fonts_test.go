package raster

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

func TestBundledFamilies(t *testing.T) {
	t.Parallel()

	want := []string{"Inter", "Montserrat", "Anton", "Bebas Neue", "Playfair Display", "JetBrains Mono"}
	if got := Families(); !slices.Equal(got, want) {
		t.Fatalf("Families() = %v, want %v", got, want)
	}

	for _, f := range families {
		for _, w := range f.weights() {
			if _, err := Bundled().Lookup(f.name, w); err != nil {
				t.Errorf("Lookup(%q, %d): %v", f.name, w, err)
			}
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		family  string
		weight  int
		weights []int
	}{
		{"Comic Sans", 400, nil},
		{"Anton", 700, []int{400}},
		{"Inter", 450, []int{100, 200, 300, 400, 500, 600, 700, 800, 900}},
		{"JetBrains Mono", 900, []int{100, 200, 300, 400, 500, 600, 700, 800}},
	}

	for _, tt := range tests {
		_, err := Bundled().Lookup(tt.family, tt.weight)

		var fe *FontError
		if !errors.As(err, &fe) || fe.Code != CodeFontUnknown {
			t.Errorf("Lookup(%q, %d) = %v, want E_FONT_UNKNOWN", tt.family, tt.weight, err)

			continue
		}

		if !slices.Equal(fe.Weights, tt.weights) {
			t.Errorf("Lookup(%q, %d) weights = %v, want %v", tt.family, tt.weight, fe.Weights, tt.weights)
		}
	}
}

// TestBundledWeightsMatchTheFiles reads the wght axis of every font file and
// compares it with the table, so the table cannot drift from the fonts.
func TestBundledWeightsMatchTheFiles(t *testing.T) {
	t.Parallel()

	for _, f := range families {
		data, err := fontFiles.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}

		ld, err := ot.NewLoader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		if !ld.HasTable(ot.MustNewTag("fvar")) {
			if f.variable {
				t.Errorf("%s: table says variable, the file has no fvar", f.name)
			}

			continue
		}

		raw, err := ld.RawTable(ot.MustNewTag("fvar"))
		if err != nil {
			t.Fatal(err)
		}

		fvar, _, err := tables.ParseFvar(raw)
		if err != nil {
			t.Fatal(err)
		}

		var found bool

		for _, a := range fvar.Axis {
			if a.Tag != wghtTag {
				continue
			}

			found = true

			if int(a.Minimum) != f.min || int(a.Maximum) != f.max {
				t.Errorf("%s: wght axis %v to %v, table says %d to %d", f.name, a.Minimum, a.Maximum, f.min, f.max)
			}
		}

		if f.variable && !found {
			t.Errorf("%s: no wght axis", f.name)
		}
	}
}

// TestFontsAreCommittedWithTheirLicences checks that every bundled file has an
// OFL text in licenses/fonts and a line in FONTS.md.
func TestFontsAreCommittedWithTheirLicences(t *testing.T) {
	t.Parallel()

	notes, err := os.ReadFile(filepath.Join("fonts", "FONTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range families {
		if !strings.Contains(string(notes), filepath.Base(f.file)) {
			t.Errorf("FONTS.md does not list %s", f.file)
		}

		lic := filepath.Join("..", "..", "licenses", "fonts", strings.ReplaceAll(f.name, " ", "")+"-OFL.txt")

		text, err := os.ReadFile(lic)
		if err != nil || !strings.Contains(string(text), "SIL OPEN FONT LICENSE Version 1.1") {
			t.Errorf("%s: licence %s missing or not the OFL 1.1: %v", f.name, lic, err)
		}
	}
}

// TestNoSystemFontAccess parses the package: it must not import the font
// scanner of go-text, nor any package that reads files, because every font
// comes from the embedded set (ADR-0012 item 3, D-016).
func TestNoSystemFontAccess(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"os",
		"io/ioutil",
		"io/fs",
		"path/filepath",
		"github.com/go-text/typesetting/fontscan",
		"net/http",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}

		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if slices.Contains(forbidden, path) {
				t.Errorf("%s imports %s: the rasterizer must not read files", file, path)
			}
		}
	}
}

// TestRenderWithoutHome renders with no home folder, no font path variables
// and an unreadable working directory name, so nothing but the embedded set
// can supply a font.
func TestRenderWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_DATA_DIRS", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("WINDIR", "")

	b := render(t, textSpec("No system fonts", plainStyle()))
	if b.inked() == 0 {
		t.Fatal("nothing was drawn")
	}
}
