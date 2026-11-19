package library

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tokens splits a text into lower case words: runs of letters and digits.
func Tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// Terms turns the query of a search into its terms: the words of the query, at
// most 8, each cut at 64 characters, repeated ones dropped. A term is a prefix:
// "prod" finds "production". Nothing of the query is kept as syntax, so it can
// never be an expression of the full-text engine.
func Terms(query string) []string {
	var out []string

	seen := map[string]bool{}

	for _, t := range Tokens(query) {
		if utf8.RuneCountInString(t) > MaxTermLength {
			t = string([]rune(t)[:MaxTermLength])
		}

		if seen[t] {
			continue
		}

		seen[t] = true

		out = append(out, t)
		if len(out) == MaxSearchTerms {
			break
		}
	}

	return out
}

// MatchesAll reports whether every term is a prefix of a word of the text.
func MatchesAll(terms []string, text string) bool {
	words := Tokens(text)

	for _, t := range terms {
		found := false

		for _, w := range words {
			if strings.HasPrefix(w, t) {
				found = true

				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

// Snippet is a window of at most limit characters of the body around the first
// word that a term starts, with runs of white space made one space and an
// ellipsis where the text was cut. With no match it is the start of the body.
func Snippet(body string, terms []string, limit int) string {
	flat := strings.Join(strings.Fields(body), " ")
	rs := []rune(flat)

	if len(rs) <= limit {
		return flat
	}

	at := 0
	lower := []rune(strings.ToLower(flat))

search:
	for i := range lower {
		if i > 0 && (unicode.IsLetter(lower[i-1]) || unicode.IsDigit(lower[i-1])) {
			continue
		}

		for _, t := range terms {
			if hasRunePrefix(lower[i:], []rune(t)) {
				at = i

				break search
			}
		}
	}

	start := max2(0, at-limit/4)
	end := min2(len(rs), start+limit)
	start = max2(0, end-limit)

	out := string(rs[start:end])
	if start > 0 {
		out = "…" + out
	}

	if end < len(rs) {
		out += "…"
	}

	return out
}

func hasRunePrefix(s, prefix []rune) bool {
	if len(prefix) == 0 || len(s) < len(prefix) {
		return false
	}

	for i, r := range prefix {
		if s[i] != r {
			return false
		}
	}

	return true
}

func max2(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func min2(a, b int) int {
	if a < b {
		return a
	}

	return b
}
