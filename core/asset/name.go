package asset

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxName is the longest display name of a file, in characters (SDD-011 section 14.8).
const MaxName = 120

// forbiddenInName are the characters Windows does not allow in a file name. The display name is the name a file
// leaves Stuarty under, so it must be one on every system the product runs on.
const forbiddenInName = `<>:"|?*`

// deviceNames are the names Windows keeps for devices, with or without an extension.
var deviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true,
	"COM9": true, "COM¹": true, "COM²": true, "COM³": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true,
	"LPT9": true, "LPT¹": true, "LPT²": true, "LPT³": true,
}

// RenameAll is the display name each holding of one file takes when a person asks for requested, given the names
// the holdings have now (the same bytes can be held under different names). The name is trimmed. It cannot be
// empty, hold a path separator, a control or invisible character or a character Windows refuses in a file name, or
// be a device name of Windows. Each holding keeps its own extension: a request that ends with the extension of any
// holding (compared without regard to case) names the part before it, and any other request is that part whole, so
// a person edits what comes before the dot. Each result is at most MaxName characters. A request equal to the one
// name of every holding changes nothing, even if that name came from a system with other rules. A refusal is a
// *RuleError with the reason name_invalid.
func RenameAll(current []string, requested string) ([]string, *RuleError) {
	typed := strings.TrimSpace(requested)

	refuse := func(why string) ([]string, *RuleError) {
		return nil, &RuleError{Path: "name", Reason: "name_invalid", Detail: why}
	}

	if !slices.ContainsFunc(current, func(c string) bool { return c != typed }) {
		return slices.Clone(current), nil
	}

	if why := nameProblem(typed); why != "" {
		return refuse(why)
	}

	stem, typedExt := splitTyped(typed, current)

	if strings.Trim(stem, ". ") == "" {
		return refuse("the name has nothing before the extension")
	}

	out := make([]string, len(current))

	for i, c := range current {
		ext := extension(c)
		if typedExt != "" && strings.EqualFold(typedExt, ext) {
			ext = typedExt
		}

		name := stem + ext

		if strings.HasSuffix(name, ".") {
			return refuse("the name ends with a dot")
		}

		if base, _, _ := strings.Cut(name, "."); deviceNames[strings.ToUpper(strings.TrimRight(base, " "))] {
			return refuse("the name is a device name of Windows")
		}

		if n := utf8.RuneCountInString(name); n > MaxName {
			return refuse(fmt.Sprintf("the name has %d characters; at most %d", n, MaxName))
		}

		out[i] = name
	}

	return out, nil
}

// nameProblem says why a typed name cannot be a file name, or is empty when it can.
func nameProblem(typed string) string {
	if typed == "" {
		return "the name is empty"
	}

	if !utf8.ValidString(typed) {
		return "the name is not valid text"
	}

	for _, r := range typed {
		switch {
		case r == '/' || r == '\\':
			return "the name has a path separator"
		case hiddenInName(r):
			return "the name has a control or invisible character"
		case strings.ContainsRune(forbiddenInName, r):
			return fmt.Sprintf("the name has the character %q, which a file name cannot have", r)
		}
	}

	return ""
}

// hiddenInName is a character a person cannot see or that changes how the rest reads: the controls, the format
// characters (bidirectional controls, zero-width characters, the byte order mark), the line and paragraph
// separators, the whole tag block (some of it unassigned, so not Cf) and the variation selectors supplement. It holds
// every character service/untrusted removes from a file name at import. The two joiners stay: scripts and emoji
// sequences need them.
func hiddenInName(r rune) bool {
	if r == 0x200C || r == 0x200D {
		return false
	}

	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) ||
		unicode.Is(unicode.Zp, r) || r >= 0xE0000 && r <= 0xE007F || r >= 0xE0100 && r <= 0xE01EF
}

// extension is the extension of a name; a lone final dot is not one.
func extension(name string) string {
	if ext := path.Ext(name); ext != "." {
		return ext
	}

	return ""
}

// splitTyped cuts what a person typed into the part before the extension and the extension as they typed it, when
// it ends with the extension of one of the current names.
func splitTyped(typed string, current []string) (stem, ext string) {
	for _, c := range current {
		e := extension(c)
		if e == "" || len(e) > len(typed) {
			continue
		}

		if tail := typed[len(typed)-len(e):]; strings.EqualFold(tail, e) {
			return typed[:len(typed)-len(e)], tail
		}
	}

	return typed, ""
}
