package raster

import (
	"bytes"
	"embed"
	"fmt"
	"slices"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// The bundled fonts are embedded in the binary. System fonts are never read
// (ADR-0012 item 3, D-016). fonts/FONTS.md lists the files and their sources.
//
//go:embed fonts/*.ttf
var fontFiles embed.FS

// CodeFontUnknown is the error code of a family or weight that is not in the
// font set (SDD-000 section 8).
const CodeFontUnknown = "E_FONT_UNKNOWN"

// FontError is returned for a family or weight the set does not have.
type FontError struct {
	Code   string
	Family string
	Weight int
	// Weights lists the weights that exist for the family; nil when the family
	// is unknown.
	Weights []int
}

func (e *FontError) Error() string {
	if e.Weights == nil {
		return fmt.Sprintf("%s: no font family %q", e.Code, e.Family)
	}

	return fmt.Sprintf("%s: family %q has no weight %d (available: %v)", e.Code, e.Family, e.Weight, e.Weights)
}

// family is one bundled family. A variable font covers every weight from min
// to max in steps of 100; a static font covers the one weight in min.
type family struct {
	name     string
	file     string
	min, max int
	variable bool
}

// families is the bundled set (D-022). The weight ranges are the ranges of the
// wght axis of each file; TestBundledWeightsMatchTheFiles reads them from the
// fonts and fails when this table is wrong.
var families = []family{
	{name: "Inter", file: "fonts/Inter-Variable.ttf", min: 100, max: 900, variable: true},
	{name: "Montserrat", file: "fonts/Montserrat-Variable.ttf", min: 100, max: 900, variable: true},
	{name: "Anton", file: "fonts/Anton-Regular.ttf", min: 400, max: 400},
	{name: "Bebas Neue", file: "fonts/BebasNeue-Regular.ttf", min: 400, max: 400},
	{name: "Playfair Display", file: "fonts/PlayfairDisplay-Variable.ttf", min: 400, max: 900, variable: true},
	{name: "JetBrains Mono", file: "fonts/JetBrainsMono-Variable.ttf", min: 100, max: 800, variable: true},
}

var wghtTag = ot.MustNewTag("wght")

// Families returns the names of the bundled families, Inter first.
func Families() []string {
	names := make([]string, len(families))
	for i, f := range families {
		names[i] = f.name
	}

	return names
}

func (f family) weights() []int {
	var out []int
	for w := f.min; w <= f.max; w += 100 {
		out = append(out, w)
	}

	return out
}

// FontSet gives the rasterizer its fonts. The zero value is not usable; use
// Bundled.
type FontSet struct {
	mu     sync.Mutex
	parsed map[string]*font.Font
}

var bundled = &FontSet{parsed: map[string]*font.Font{}}

// Bundled returns the font set shipped inside the binary. It is safe for
// concurrent use.
func Bundled() *FontSet { return bundled }

// Lookup returns a face of the family at the weight, or a *FontError with the
// code E_FONT_UNKNOWN. Every call returns a new face, so callers may set
// variations on it.
func (s *FontSet) Lookup(familyName string, weight int) (*font.Face, error) {
	for _, f := range families {
		if f.name != familyName {
			continue
		}

		if weight < f.min || weight > f.max || weight%100 != 0 {
			return nil, &FontError{Code: CodeFontUnknown, Family: familyName, Weight: weight, Weights: f.weights()}
		}

		parsed, err := s.load(f)
		if err != nil {
			return nil, err
		}

		face := font.NewFace(parsed)
		if f.variable {
			face.SetVariations([]font.Variation{{Tag: wghtTag, Value: float32(weight)}})
		}

		return face, nil
	}

	return nil, &FontError{Code: CodeFontUnknown, Family: familyName, Weight: weight}
}

func (s *FontSet) load(f family) (*font.Font, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parsed, ok := s.parsed[f.file]; ok {
		return parsed, nil
	}

	data, err := fontFiles.ReadFile(f.file)
	if err != nil {
		return nil, fmt.Errorf("raster: bundled font %s: %w", f.file, err)
	}

	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("raster: parse %s: %w", f.file, err)
	}

	s.parsed[f.file] = face.Font

	return face.Font, nil
}

// Missing returns the runes of text that the family has no glyph for, in the
// order they first appear, each once. Those runes are drawn as .notdef: there
// is no fallback to another font (SDD-004 section 5.1 item 9). Control and
// format characters (line breaks, joiners) are not glyphs and are skipped.
func (s *FontSet) Missing(familyName string, weight int, text string) ([]rune, error) {
	face, err := s.Lookup(familyName, weight)
	if err != nil {
		return nil, err
	}

	var out []rune

	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || slices.Contains(out, r) {
			continue
		}

		if _, ok := face.NominalGlyph(r); !ok {
			out = append(out, r)
		}
	}

	return out, nil
}
