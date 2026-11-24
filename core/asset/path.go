package asset

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// DisplayPath is a path as a page may draw it (SDD-011 section 14.10): every control, format (bidirectional
// controls, zero-width characters, the byte order mark), line and paragraph separator, tag or variation-selector
// character, the two joiners included, is written out as a visible \u{...} escape, and a byte that is not valid
// UTF-8 as \x.. So a folder named with a right-to-left override cannot reorder the text around it, and no control
// character reaches a screen reader or a terminal. The path itself, the one the file manager takes, is not
// changed.
func DisplayPath(path string) string {
	var b strings.Builder

	for i := 0; i < len(path); {
		r, size := utf8.DecodeRuneInString(path[i:])

		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02X`, path[i])
		case hiddenInName(r) || r == 0x200C || r == 0x200D || r == 0x7F:
			fmt.Fprintf(&b, `\u{%X}`, r)
		default:
			b.WriteRune(r)
		}

		i += size
	}

	return b.String()
}
