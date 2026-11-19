package library

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// memIndex is an Index in memory with the semantics of the SQLite one: every
// term must start a word of the name, the description, the tags or the body.
type memIndex struct {
	mu   sync.Mutex
	rows map[string]Entry
	text map[string]string
}

func newMemIndex() *memIndex {
	return &memIndex{rows: map[string]Entry{}, text: map[string]string{}}
}

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
	x.text[e.Key] = e.Name + " " + e.Description + " " + strings.Join(e.Tags, " ") + " " + body

	return nil
}

func (x *memIndex) Delete(_ context.Context, key string) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	delete(x.rows, key)
	delete(x.text, key)

	return nil
}

func (x *memIndex) Clear(context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	x.rows, x.text = map[string]Entry{}, map[string]string{}

	return nil
}

func (x *memIndex) Search(_ context.Context, terms []string, limit int) ([]string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	type hit struct {
		key   string
		score int
	}

	var hits []hit

	for key, text := range x.text {
		if !core.MatchesAll(terms, text) {
			continue
		}

		score := 0
		if core.MatchesAll(terms, x.rows[key].Name) {
			score = 2
		}

		hits = append(hits, hit{key, score})
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
