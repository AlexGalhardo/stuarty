package timeline

import (
	"bytes"
	"reflect"
	"slices"
)

// BundledWeights lists the weights shipped for each bundled family, as in
// renderer/fonts/FONTS.md (a test keeps the two in step). The variable fonts
// cover every multiple of 100 in their range.
var BundledWeights = map[string][]int{
	"Inter":            {100, 200, 300, 400, 500, 600, 700, 800, 900},
	"Montserrat":       {100, 200, 300, 400, 500, 600, 700, 800, 900},
	"Anton":            {400},
	"Bebas Neue":       {400},
	"Playfair Display": {400, 500, 600, 700, 800, 900},
	"JetBrains Mono":   {100, 200, 300, 400, 500, 600, 700, 800},
}

// FontWeights returns the weights available for a family of the bundled set or of
// the document's fonts, and whether the family exists.
func FontWeights(doc *Document, family string) ([]int, bool) {
	if f, ok := doc.Fonts[family]; ok {
		weights := make([]int, 0, len(f.Files))
		for _, ff := range f.Files {
			weights = append(weights, ff.Weight)
		}
		slices.Sort(weights)
		return weights, true
	}
	w, ok := BundledWeights[family]
	return w, ok
}

// DefaultFontWeight is the weight a style without font_weight resolves to for a family: 700,
// or the nearest weight the family has (the heavier one at a tie), so a brand font with
// only a regular face works without naming its weight.
func DefaultFontWeight(doc *Document, family string) int {
	weights, ok := FontWeights(doc, family)
	if !ok || len(weights) == 0 || slices.Contains(weights, 700) {
		return 700
	}
	best := weights[0]
	for _, w := range weights {
		if d, bd := abs(w-700), abs(best-700); d < bd || d == bd && w > best {
			best = w
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// StyleKeys returns the keys of a style object: those of TextStyle, then the ones
// only a CaptionStyle has.
func StyleKeys() (text, caption []string) {
	for _, f := range infoOf(reflect.TypeFor[CaptionStyle]()).fields {
		if _, isText := reflect.TypeFor[TextStyle]().FieldByName(fieldName(f)); isText {
			text = append(text, f.name)
		} else {
			caption = append(caption, f.name)
		}
	}
	return text, caption
}

func fieldName(f field) string {
	return reflect.TypeFor[CaptionStyle]().FieldByIndex(f.index).Name
}

// StyleToMap returns a style as a JSON object tree holding exactly the keys that are
// set (SDD-002 R15), or nil for a nil style.
func StyleToMap(s *CaptionStyle) map[string]any {
	if s == nil {
		return nil
	}
	var buf bytes.Buffer
	writeValue(&buf, reflect.ValueOf(s))
	raw, err := DecodeRaw(buf.Bytes())
	if err != nil {
		panic("timeline: style encoding is not valid JSON: " + err.Error())
	}
	return raw
}

// StyleFromMap types a style object tree (numbers as json.Number). A value of the
// wrong type is a *ViolationsError.
func StyleFromMap(m map[string]any) (*CaptionStyle, error) {
	d := &decoder{}
	s := &CaptionStyle{}
	d.decode(reflect.ValueOf(s).Elem(), m, "")
	if len(d.errs) > 0 {
		return nil, &ViolationsError{d.errs}
	}
	return s, nil
}

// StyleViolations checks the numbers, the colours and the keys of a text style the way Validate does,
// without the font (which needs the project) and without template colour references.
func StyleViolations(s *CaptionStyle) []Violation { return styleViolations(s, false) }

// CaptionStyleViolations is StyleViolations for the style of a caption or of a caption track, where
// the position and the highlight keys are allowed.
func CaptionStyleViolations(s *CaptionStyle) []Violation { return styleViolations(s, true) }

func styleViolations(s *CaptionStyle, caption bool) []Violation {
	v := &validator{doc: &Document{}}
	v.styleNumbers("style", "", s)
	v.styleColors("style", "", s)
	v.styleCaption("style", "", s, caption)

	return v.vs
}
