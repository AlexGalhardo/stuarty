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

// TextWords is the number of words a person reads in a text of the given kind: for a post, the words of the body
// without its front matter (title, slug, tags and the like are metadata, not words of the post), for every other
// kind, all of them. The list of texts and the panel of a post both say this number (VV-324).
func TextWords(kind, body string) int {
	if kind == "post" {
		post, _ := ParsePost(body)

		return CountWords(post.Markdown)
	}

	return CountWords(body)
}
