package writing

import (
	"strings"
	"unicode/utf8"
)

// The numbers of the text summary of a project (SDD-005 section 5.12.4).
const (
	// WordsPerMinute is the reading speed the reading time assumes.
	WordsPerMinute = 200
	// ExcerptRunes is the length of the excerpt of a text, in characters.
	ExcerptRunes = 160
)

// ReadingMinutes is the minutes to read a text of that many words: the words divided by 200, rounded up. A text
// with no words takes no time.
func ReadingMinutes(words int) int {
	if words <= 0 {
		return 0
	}

	return (words + WordsPerMinute - 1) / WordsPerMinute
}

// Excerpt is the first 160 characters of a text of the given kind, with the front matter of a post removed and the
// white space at both ends trimmed. It is the creator's words: a surface marks it as untrusted.
func Excerpt(kind, body string) string {
	text := strings.ReplaceAll(body, "\r\n", "\n")

	if kind == "post" {
		post, _ := ParsePost(text)
		text = post.Markdown
	}

	text = strings.TrimSpace(text)

	if utf8.RuneCountInString(text) <= ExcerptRunes {
		return text
	}

	runes := []rune(text)

	return strings.TrimRightFunc(
		string(runes[:ExcerptRunes]),
		func(r rune) bool { return r == ' ' || r == '\n' || r == '\t' },
	)
}

// Summary is what a list says about the text of a project.
type Summary struct {
	Words          int
	ReadingMinutes int
	Excerpt        string
}

// Summarize counts the words of a text, its reading time and its excerpt.
func Summarize(kind, body string) Summary {
	words := TextWords(kind, body)

	return Summary{Words: words, ReadingMinutes: ReadingMinutes(words), Excerpt: Excerpt(kind, body)}
}

// Candidate is a text of a project that may be the one its list summarises.
type Candidate struct {
	Kind string // script, copy (also an empty one), caption or post
	Body string
}

// SummaryText picks the text a list of projects summarises: the first, by the order given, that has words and is not
// a caption, preferring a post, then a copy, then a script. ok is false when there is none.
func SummaryText(candidates []Candidate) (index int, ok bool) {
	best, bestRank := -1, 4

	for i := range candidates {
		c := &candidates[i]

		rank := summaryRank(c.Kind)
		if rank == 0 || rank >= bestRank || TextWords(c.Kind, c.Body) == 0 {
			continue
		}

		best, bestRank = i, rank
	}

	return best, best >= 0
}

// summaryRank orders the kinds a summary prefers: 1 is the best, 0 is a kind that is never summarised.
func summaryRank(kind string) int {
	switch kind {
	case "post":
		return 1
	case "copy", "":
		return 2
	case "script":
		return 3
	}

	return 0
}
