package project

import (
	"path"
	"strings"
	"unicode"
)

// DefaultStandaloneName is used when a standalone video edit is created without a name or file stem.
const DefaultStandaloneName = "Untitled edit"

const (
	// MinFrameSide and MaxFrameSide match the timeline frame bounds (SDD-001 / core/command settings.set).
	MinFrameSide = 16
	MaxFrameSide = 4096
)

// CheckStandaloneTitle validates the title of a standalone video session (1 to 120 visible characters,
// no control or bidi characters), reporting errors under path "name".
func CheckStandaloneTitle(name string) *RuleError {
	if err := CheckTitle(name); err != nil {
		cp := *err
		cp.Path = "name"
		cp.Message = strings.ReplaceAll(cp.Message, "title", "name")
		cp.Hint = strings.ReplaceAll(cp.Hint, "title", "name")

		return &cp
	}

	return nil
}

// StandaloneNameFromFile derives a clean standalone edit title from an uploaded or imported file name.
func StandaloneNameFromFile(fileName string) string {
	base := fileName
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}

	if ext := path.Ext(base); ext != "" && ext != base {
		base = strings.TrimSuffix(base, ext)
	}

	var b strings.Builder

	n := 0

	for _, r := range strings.TrimSpace(base) {
		if n >= MaxTitle {
			break
		}

		if unicode.IsControl(r) || isBidiControl(r) {
			continue
		}

		b.WriteRune(r)

		n++
	}

	clean := strings.TrimSpace(b.String())
	if clean == "" || CheckStandaloneTitle(clean) != nil {
		return DefaultStandaloneName
	}

	return clean
}

// StandaloneDimensions normalizes probed video pixel dimensions to valid even timeline dimensions
// within [MinFrameSide, MaxFrameSide]. ok is false when either input side is non-positive.
func StandaloneDimensions(width, height int) (w, h int, ok bool) {
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}

	clampEven := func(v int) int {
		if v < MinFrameSide {
			return MinFrameSide
		}

		if v > MaxFrameSide {
			v = MaxFrameSide
		}

		v &^= 1
		if v < MinFrameSide {
			return MinFrameSide
		}

		return v
	}

	return clampEven(width), clampEven(height), true
}

// CheckPromote checks whether a standalone session may be promoted to a full production project
// with the supplied context (SDD-005 section 5.12.5).
func CheckPromote(standalone bool, c Context, opts CheckOptions) []*RuleError {
	if !standalone {
		return []*RuleError{{
			Code:    "E_INVALID_STATE",
			Path:    "project_id",
			Message: "this project is already a full production",
			Hint:    "use project:update to edit an existing project",
		}}
	}

	return CheckContext(c, opts)
}
