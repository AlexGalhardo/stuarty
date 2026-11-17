package rasterize

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fontDir is where the font files the renderer module embeds live.
const fontDir = "../raster/fonts"

// bundledFamily is one bundled family as SDD-004 section 5.1 lists it: a variable font covers every
// weight from min to max in steps of 100, a static one the weight in min.
type bundledFamily struct {
	name     string
	file     string
	min, max int
}

var bundledFamilies = []bundledFamily{
	{"Inter", "Inter-Variable.ttf", 100, 900},
	{"Montserrat", "Montserrat-Variable.ttf", 100, 900},
	{"Anton", "Anton-Regular.ttf", 400, 400},
	{"Bebas Neue", "BebasNeue-Regular.ttf", 400, 400},
	{"Playfair Display", "PlayfairDisplay-Variable.ttf", 400, 900},
	{"JetBrains Mono", "JetBrainsMono-Variable.ttf", 100, 800},
}

func (f bundledFamily) weights() []int {
	var out []int

	for w := f.min; w <= f.max; w += 100 {
		out = append(out, w)
	}

	return out
}

func TestBundledFamilies(t *testing.T) {
	t.Parallel()

	for _, f := range bundledFamilies {
		for _, w := range f.weights() {
			st := plainStyle()
			st.FontFamily, st.FontWeight = f.name, w

			if _, err := Render(textSpec("Hamburgefonstiv", st), Bundled()); err != nil {
				t.Errorf("Render(%q, %d): %v", f.name, w, err)
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
		st := plainStyle()
		st.FontFamily, st.FontWeight = tt.family, tt.weight

		_, err := Render(textSpec("x", st), Bundled())

		var fe *FontError
		if !errors.As(err, &fe) || fe.Code != CodeFontUnknown {
			t.Errorf("Render(%q, %d) = %v, want E_FONT_UNKNOWN", tt.family, tt.weight, err)

			continue
		}

		if fe.Family != tt.family || fe.Weight != tt.weight || !slices.Equal(fe.Weights, tt.weights) {
			t.Errorf("Render(%q, %d) error = %+v, want weights %v", tt.family, tt.weight, fe, tt.weights)
		}
	}
}

// TestFontsAreCommittedWithTheirLicences checks that every bundled file exists, has an OFL text in
// licenses/fonts and a line in FONTS.md.
func TestFontsAreCommittedWithTheirLicences(t *testing.T) {
	t.Parallel()

	notes, err := os.ReadFile(filepath.Join(fontDir, "FONTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range bundledFamilies {
		if _, err := os.Stat(filepath.Join(fontDir, f.file)); err != nil {
			t.Errorf("%s: %v", f.name, err)
		}

		if !strings.Contains(string(notes), f.file) {
			t.Errorf("FONTS.md does not list %s", f.file)
		}

		lic := filepath.Join("..", "..", "licenses", "fonts", strings.ReplaceAll(f.name, " ", "")+"-OFL.txt")

		text, err := os.ReadFile(lic)
		if err != nil || !strings.Contains(string(text), "SIL OPEN FONT LICENSE Version 1.1") {
			t.Errorf("%s: licence %s missing or not the OFL 1.1: %v", f.name, lic, err)
		}
	}
}

// TestNoSystemFontAccess parses the package: it must not import anything that reads files or the
// network, because every font comes from the module's embedded set or from the document's font source
// (ADR-0012 item 3, D-016).
func TestNoSystemFontAccess(t *testing.T) {
	t.Parallel()

	forbidden := []string{"os", "io/ioutil", "io/fs", "path/filepath", "net/http"}

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
	if inked(b) == 0 {
		t.Fatal("nothing was drawn")
	}
}
