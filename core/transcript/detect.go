package transcript

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// SpokenWord is a transcript word with its time in milliseconds, as the transcript artifact has it
// (SDD-011 section 10.5). Detection reads milliseconds so that it does not depend on a frame rate.
type SpokenWord struct {
	Text    string `json:"text"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
}

// Kind says what a suggestion found.
type Kind string

// The two kinds of suggestion.
const (
	KindFiller     Kind = "filler"
	KindRepetition Kind = "repetition"
)

// Strength says how sure the rule is. A strong suggestion is shown checked in the review, a weak one
// unchecked.
type Strength string

// The two strengths.
const (
	Strong Strength = "strong"
	Weak   Strength = "weak"
)

// Suggestion is a range of words a person may remove. It changes nothing by itself.
type Suggestion struct {
	Kind     Kind     `json:"kind"`
	Strength Strength `json:"strength"`
	FromWord int      `json:"from_word"`
	ToWord   int      `json:"to_word"`
	Text     string   `json:"text"`
	StartMS  int64    `json:"start_ms"`
	EndMS    int64    `json:"end_ms"`
}

// Options of Detect. The zero value is the default.
type Options struct {
	// Language is the ISO 639-1 code of the transcript. An unlisted language has no filler rule.
	Language string
	// RepeatWindowMS is the longest gap between the two copies of a repetition. Default 700.
	RepeatWindowMS int64
}

const (
	defaultRepeatWindowMS = 700
	maxRepeatLen          = 4
	// MaxSuggestions bounds the answer (SDD-023 section 8).
	MaxSuggestions = 2000
)

type fillerRules struct {
	strong *regexp.Regexp
	// weak are exact normalised words or phrases of several words (separated by a space).
	weak       map[string]bool
	weakPhrase [][]string
	// answers are the words that are legitimately said twice ("very very", "no no").
	answers map[string]bool
}

func rules(language string) (fillerRules, bool) {
	switch strings.ToLower(language) {
	case "en":
		return fillerRules{
			strong:     regexp.MustCompile(`^(?:u+m+|u+h+m*|e+r+m*|h+m+|m{2,}|m+h+m+)$`),
			weak:       map[string]bool{"ah": true},
			weakPhrase: [][]string{{"you", "know"}, {"i", "mean"}},
			answers:    set("very", "really", "so", "no", "yes", "bye"),
		}, true
	case "pt":
		return fillerRules{
			strong:  regexp.MustCompile(`^(?:h+u+m+|h+m+|a+h+n+|ã+h+|h+ã+|é+h+|é{2,}|u+h+m*)$`),
			weak:    map[string]bool{"né": true, "tipo": true},
			answers: set("muito", "não", "sim", "tchau"),
		}, true
	}
	return fillerRules{answers: set()}, false
}

// RuleDescription says in one sentence what Detect checks for a language, so that an empty list is not read
// as "there are no fillers".
func RuleDescription(language string) string {
	if _, ok := rules(language); !ok {
		return "repeated words and phrases only; there is no list of filler words for the language " +
			"\"" + language + "\""
	}
	return "hesitations and discourse markers of the language \"" + strings.ToLower(language) +
		"\" as written by the transcription, and repeated words and phrases"
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Normalize lowercases a word and strips punctuation and symbols; letters, digits and inner
// apostrophes stay.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '\'' || r == '’' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "'’")
}

// Detect finds the filler words and the repetitions of a transcript (SDD-023 section 4). It is a pure
// function: the same words and options give the same suggestions, ordered by first word, not
// overlapping, at most MaxSuggestions.
func Detect(words []SpokenWord, opts Options) []Suggestion {
	window := opts.RepeatWindowMS
	if window <= 0 {
		window = defaultRepeatWindowMS
	}
	rl, _ := rules(opts.Language)
	norm := make([]string, len(words))
	for i, w := range words {
		norm[i] = Normalize(w.Text)
	}

	claimed := make([]bool, len(words))
	var out []Suggestion
	add := func(kind Kind, strength Strength, from, to int) {
		var texts []string
		for i := from; i <= to; i++ {
			claimed[i] = true
			texts = append(texts, words[i].Text)
		}
		out = append(out, Suggestion{
			Kind: kind, Strength: strength, FromWord: from, ToWord: to,
			Text: strings.Join(texts, " "), StartMS: words[from].StartMS, EndMS: words[to].EndMS,
		})
	}

	for i := 0; i < len(words); i++ {
		if norm[i] == "" {
			continue
		}
		if rl.strong != nil && rl.strong.MatchString(norm[i]) {
			add(KindFiller, Strong, i, i)
			continue
		}
		if n := matchPhrase(rl.weakPhrase, norm, i); n > 0 {
			add(KindFiller, Weak, i, i+n-1)
			i += n - 1
			continue
		}
		if rl.weak[norm[i]] {
			add(KindFiller, Weak, i, i)
		}
	}

	for i := 0; i < len(words); i++ {
		for n := maxRepeatLen; n >= 1; n-- {
			if !repeatsAt(words, norm, claimed, i, n, window) {
				continue
			}
			strength := Strong
			if n == 1 && rl.answers[norm[i]] {
				strength = Weak
			}
			add(KindRepetition, strength, i, i+n-1)
			i += n - 1
			break
		}
	}

	slices.SortFunc(out, func(a, b Suggestion) int { return cmp.Compare(a.FromWord, b.FromWord) })
	if len(out) > MaxSuggestions {
		out = out[:MaxSuggestions]
	}
	return out
}

// repeatsAt reports whether the n words at i are repeated right after, within the window, with no word
// of either copy claimed by a filler.
func repeatsAt(words []SpokenWord, norm []string, claimed []bool, i, n int, window int64) bool {
	if i+2*n > len(words) {
		return false
	}
	for k := range n {
		if norm[i+k] == "" || norm[i+k] != norm[i+n+k] || claimed[i+k] || claimed[i+n+k] {
			return false
		}
	}
	return words[i+n].StartMS-words[i+n-1].EndMS <= window
}

func matchPhrase(phrases [][]string, norm []string, i int) int {
	for _, p := range phrases {
		if i+len(p) > len(norm) {
			continue
		}
		ok := true
		for k, w := range p {
			if norm[i+k] != w {
				ok = false
				break
			}
		}
		if ok {
			return len(p)
		}
	}
	return 0
}
