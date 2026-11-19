package writing

import (
	"strings"
	"unicode"
)

// CountWords is the number of words of s: runs of characters between white space that hold at least one letter or
// digit (SDD-019 section 4).
func CountWords(s string) int {
	n := 0

	for f := range strings.FieldsSeq(s) {
		if strings.ContainsFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			n++
		}
	}

	return n
}
