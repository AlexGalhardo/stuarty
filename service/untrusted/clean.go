// Package untrusted is the one place where text that the product did not
// generate (a transcript, a file name, tags of a container, the text of a
// project or a template) is cleaned, escaped, cut and delimited before it
// reaches an agent (SDD-007 section 8, ADR-0020 item 5).
package untrusted

import (
	"strings"
	"unicode/utf8"
)

// removed is true for the characters that never survive: control characters
// except line feed (U+0000 to U+001F, U+007F to U+009F), the bidirectional
// controls (U+202A to U+202E, U+2066 to U+2069, U+200E, U+200F) and the line
// and paragraph separators U+2028 and U+2029. U+061C is removed too: it is the
// Arabic letter mark, a bidirectional control of the same kind. So are the
// characters a person cannot see and a model still reads: U+200B, U+2060 to
// U+2064, U+FEFF, U+180E, U+FFF9 to U+FFFB, the tag characters U+E0000 to
// U+E007F and the variation selectors supplement U+E0100 to U+E01EF.
func removed(r rune) bool {
	switch {
	case r == '\n':
		return false
	case r < 0x20, r >= 0x7F && r <= 0x9F:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r >= 0x2060 && r <= 0x2064, r >= 0xFFF9 && r <= 0xFFFB:
		return true
	case r >= 0xE0000 && r <= 0xE007F, r >= 0xE0100 && r <= 0xE01EF:
		return true
	}

	return r == 0x200B || r == 0xFEFF || r == 0x180E || r == 0x200E || r == 0x200F || r == 0x061C ||
		r == 0x2028 || r == 0x2029
}

// Removed reports whether Clean drops r.
func Removed(r rune) bool { return removed(r) }

// joiner is true for the characters that are legitimate once (scripts, emoji
// sequences) and only useful to hide data in a run: ZWNJ, ZWJ and U+FE00 to
// U+FE0F.
func joiner(r rune) bool {
	return r == 0x200C || r == 0x200D || r >= 0xFE00 && r <= 0xFE0F
}

// Clean removes the characters of the removed classes, replaces invalid UTF-8
// with U+FFFD, reduces a run of joiners to its first, and collapses runs of
// more than 3 blank lines to 3.
func Clean(s string) string {
	s = strings.ToValidUTF8(s, "�")

	var b strings.Builder

	b.Grow(len(s))

	newlines := 0
	prevJoiner := false

	for _, r := range s {
		if removed(r) {
			continue
		}

		if joiner(r) {
			if prevJoiner {
				continue
			}

			prevJoiner = true
		} else {
			prevJoiner = false
		}

		if r == '\n' {
			newlines++
			if newlines > 4 { // a line, then at most 3 blank lines
				continue
			}
		} else {
			newlines = 0
		}

		b.WriteRune(r)
	}

	return b.String()
}

// Escape writes the ampersand as &amp; and the less-than sign as &lt;, so that
// no content can close a block or open another one. It is applied after Clean.
func Escape(s string) string {
	if !strings.ContainsAny(s, "&<") {
		return s
	}

	return strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(s)
}

// Cut returns the first n characters of s and how many were left out.
func Cut(s string, n int) (kept string, more int) {
	if n < 0 {
		n = 0
	}

	if utf8.RuneCountInString(s) <= n {
		return s, 0
	}

	i := 0

	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}

	return s[:i], utf8.RuneCountInString(s[i:])
}
