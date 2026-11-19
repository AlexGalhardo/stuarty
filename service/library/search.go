package library

import (
	"context"
	"os"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// SearchQuery is one full-text search over the three kinds (SDD-018 section 10).
type SearchQuery struct {
	Query  string
	Filter Filter
	Limit  int // hits, default 20, at most 50
	Budget int // characters of the answer, default 4000, at most 16000
	// Agent is true for a call that comes from an agent tool: it never sees what a person has
	// not reviewed or kept, whatever the filter says.
	Agent bool
}

// Hit is a result: the row and the text around the match.
type Hit struct {
	Entry   Entry
	Snippet string
}

// SearchResult is the answer of a search.
type SearchResult struct {
	Hits      []Hit
	Total     int  // entries that matched, before the limit and the budget
	Truncated bool // more matched than the answer holds
}

const searchCandidates = 500

// Search runs a query. Terms are the words of the query, each a prefix, all of
// them required; what is left of the query is never read as syntax.
func (m *Manager) Search(ctx context.Context, q SearchQuery) (SearchResult, error) {
	terms := core.Terms(q.Query)
	if len(terms) == 0 {
		return SearchResult{}, invalid("query", "it needs at least one word")
	}

	limit := q.Limit
	if limit <= 0 {
		limit = core.DefaultHits
	}

	limit = min(limit, core.MaxHits)

	budget := q.Budget
	if budget <= 0 {
		budget = core.DefaultBudget
	}

	budget = min(budget, core.MaxBudget)

	rows, err := m.refresh(ctx)
	if err != nil {
		return SearchResult{}, err
	}

	byKey := make(map[string]int, len(rows))
	for i := range rows {
		byKey[rows[i].Key] = i
	}

	keys, err := m.idx.Search(ctx, terms, searchCandidates)
	if err != nil {
		return SearchResult{}, err
	}

	filter := q.Filter
	if q.Agent {
		filter.IncludeUnreviewed = false
	}

	var (
		res  SearchResult
		used int
		root *os.Root
	)

	if root, err = m.files.open(); err != nil {
		return SearchResult{}, err
	}

	if root != nil {
		defer func() { _ = root.Close() }()
	}

	for _, key := range keys {
		i, ok := byKey[key]
		if !ok {
			continue
		}

		e := rows[i]
		if e.State != StateOK || !filter.match(e) {
			continue
		}

		res.Total++

		if len(res.Hits) >= limit || res.Truncated {
			res.Truncated = true

			continue
		}

		snippet := core.Snippet(m.bodyOf(root, &e), terms, core.MaxSnippet)
		cost := len([]rune(e.Name)) + len([]rune(e.Description)) + len([]rune(snippet))

		if used+cost > budget && len(res.Hits) > 0 {
			res.Truncated = true

			continue
		}

		used += cost

		res.Hits = append(res.Hits, Hit{Entry: e, Snippet: snippet})
	}

	return res, nil
}

// visibleTo tells whether a key limited to a project may see the entry: it sees
// the notes of the workspace and of its project, and every skill and prompt.
func visibleTo(e *Entry, projectID string) bool {
	return projectID == "" || e.Kind != core.KindMemory || e.Scope == "workspace" || e.Scope == projectID
}

// bodyOf reads the text of an entry for its snippet: the file, not the index.
func (m *Manager) bodyOf(root *os.Root, e *Entry) string {
	if root == nil {
		return ""
	}

	switch e.Kind {
	case core.KindMemory:
		if data, _, err := readSmall(root, e.Path, maxEntryFile); err == nil {
			if mem, err := core.ParseMemory(string(data)); err == nil {
				return mem.Body
			}
		}
	case core.KindPrompt:
		if data, _, err := readSmall(root, e.Path, maxEntryFile); err == nil {
			if p, err := core.ParsePrompt(string(data)); err == nil {
				return p.Body
			}
		}
	case core.KindSkill:
		if data, _, err := readSmall(root, e.Path+"/"+core.SkillFile, core.MaxSkillMD); err == nil {
			if _, body, err := core.ParseDocument(string(data)); err == nil {
				return body
			}
		}
	}

	return ""
}
