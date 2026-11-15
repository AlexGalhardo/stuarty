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
// Arabic letter mark, a bidirectional control of the same kind.
func removed(r rune) bool {
	switch {
	case r == '\n':
		return false
	case r < 0x20, r >= 0x7F && r <= 0x9F:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	}

	return r == 0x200E || r == 0x200F || r == 0x061C || r == 0x2028 || r == 0x2029
}

// Clean removes the characters of the removed classes, replaces invalid UTF-8
// with U+FFFD, and collapses runs of more than 3 blank lines to 3.
func Clean(s string) string {
	s = strings.ToValidUTF8(s, "�")

	var b strings.Builder

	b.Grow(len(s))

	newlines := 0

	for _, r := range s {
		if removed(r) {
			continue
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
