package library

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// The weight of a term found in a field, as the full-text index of SDD-018 section 10 weighed them: a hit in the
// name counts most, then the description, the tags and the body.
const (
	weightName = 10
	weightDesc = 5
	weightTags = 3
	weightBody = 1
)

// memIndex is the Index of the product (ADR-0053): it lives in memory, is built by a scan at the unlock and is
// dropped at lock, so no index is on the disk and none outlives the key. Every term must start a word of the name,
// the description, the tags or the body.
type memIndex struct {
	mu    sync.Mutex
	rows  map[string]Entry
	words map[string]fieldWords
}

// fieldWords holds the words of each searchable field, split once when the row is put.
type fieldWords struct{ name, desc, tags, body []string }

func newMemIndex() *memIndex {
	return &memIndex{rows: map[string]Entry{}, words: map[string]fieldWords{}}
}

// NewMemIndex returns an empty in-memory index.
func NewMemIndex() Index { return newMemIndex() }

func (x *memIndex) All(context.Context) ([]Entry, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	out := make([]Entry, 0, len(x.rows))
	for k := range x.rows {
		out = append(out, x.rows[k])
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return out, nil
}

func (x *memIndex) Put(_ context.Context, e Entry, body string) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	x.rows[e.Key] = e
	x.words[e.Key] = fieldWords{
		name: foldedTokens(e.Name),
		desc: foldedTokens(e.Description),
		tags: foldedTokens(strings.Join(e.Tags, " ")),
		body: foldedTokens(body),
	}

	return nil
}

func (x *memIndex) Delete(_ context.Context, key string) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	delete(x.rows, key)
	delete(x.words, key)

	return nil
}

func (x *memIndex) Clear(context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	x.rows, x.words = map[string]Entry{}, map[string]fieldWords{}

	return nil
}

// fold removes the diacritics of a text, as the tokenizer of the former full-text index did (SDD-018 section 9): a
// search for "producao" finds "Produção".
func fold(s string) string {
	d := norm.NFD.String(s)

	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}

		return r
	}, d)
}

func foldedTokens(s string) []string { return core.Tokens(fold(s)) }

func hasPrefixWord(words []string, term string) bool {
	return slices.ContainsFunc(words, func(w string) bool { return strings.HasPrefix(w, term) })
}

// score is 0 when a term matches no field; otherwise the sum, over the terms, of the heaviest field each one starts a
// word of.
func (f fieldWords) score(terms []string) int {
	total := 0

	for _, t := range terms {
		switch {
		case hasPrefixWord(f.name, t):
			total += weightName
		case hasPrefixWord(f.desc, t):
			total += weightDesc
		case hasPrefixWord(f.tags, t):
			total += weightTags
		case hasPrefixWord(f.body, t):
			total += weightBody
		default:
			return 0
		}
	}

	return total
}

func (x *memIndex) Search(_ context.Context, terms []string, limit int) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	terms = slices.Clone(terms)
	for i := range terms {
		terms[i] = fold(terms[i])
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	type hit struct {
		key   string
		score int
	}

	var hits []hit

	for key, w := range x.words {
		if s := w.score(terms); s > 0 {
			hits = append(hits, hit{key, s})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}

		return hits[i].key < hits[j].key
	})

	keys := make([]string, 0, len(hits))
	for _, h := range hits {
		keys = append(keys, h.key)
	}

	if len(keys) > limit {
		keys = slices.Clone(keys[:limit])
	}

	return keys, nil
}
