package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

func seedSearch(t *testing.T, m *Manager) {
	t.Helper()

	ctx := context.Background()

	reviewed := core.Skill{
		Name: "hook-writer", Description: "Write the opening of a short video.", License: "MIT",
		Body:  "# Hooks\n\nStart with the result of the video.\n",
		Files: []core.File{{Path: "references/patterns.md", Data: []byte("Pattern: open with a question.\n")}},
	}
	reviewed.SetOrigin(core.OriginUser, "")
	reviewed.MarkReviewed()

	imported := core.Skill{
		Name: "thumbnail-pack", Description: "Thumbnails with a face and a short video title.", License: "MIT",
		Body: "Use a big face. Keep the title short.\n",
	}
	imported.SetOrigin(core.OriginImported, "a folder")

	for _, err := range []error{
		m.WriteSkill(ctx, reviewed), m.WriteSkill(ctx, imported),
		func() error {
			_, _, err := m.SaveMemory(ctx, person, MemoryInput{
				Name: "viewers-skip-intros", Description: "Viewers skip long intros", Tags: []string{"retention"},
				Body: "Cut the intro of a video to two seconds.\n",
			})

			return err
		}(),
		func() error {
			_, _, err := m.SaveMemory(ctx, agent, MemoryInput{
				Name: "agent-note", Description: "The agent saw that videos need a hook", Body: "Add a hook to every video.\n",
			})

			return err
		}(),
		func() error {
			_, _, err := m.SavePrompt(ctx, person, PromptInput{
				Name: "video-intro", Description: "An intro for a video", Body: "Write an intro about {{topic}}.\n",
			})

			return err
		}(),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func names(hits []Hit) []string {
	var out []string
	for i := range hits {
		out = append(out, hits[i].Entry.Key)
	}

	return out
}

func TestSearchFindsEveryKind(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	seedSearch(t, m)

	res, err := m.Search(context.Background(), SearchQuery{Query: "intro"})
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(
		names(res.Hits),
		",",
	); got != "memory/viewers-skip-intros,prompt/video-intro" &&
		got != "prompt/video-intro,memory/viewers-skip-intros" {
		t.Fatalf("a note and a prompt: %v", got)
	}

	res, _ = m.Search(context.Background(), SearchQuery{Query: "question", Filter: Filter{IncludeUnreviewed: true}})
	if len(res.Hits) != 1 || res.Hits[0].Entry.Key != "skill/hook-writer" {
		t.Fatalf("the text of a reference file of a skill is searched: %v", names(res.Hits))
	}

	if !strings.Contains(res.Hits[0].Snippet, "Hooks") {
		t.Fatalf("the snippet is the body of the skill: %q", res.Hits[0].Snippet)
	}
}

func TestSearchFiltersAndNeverLeavesUnreviewedToAnAgent(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	seedSearch(t, m)

	ctx := context.Background()

	res, _ := m.Search(ctx, SearchQuery{Query: "video"})
	for _, k := range names(res.Hits) {
		if k == "skill/thumbnail-pack" || k == "memory/agent-note" {
			t.Fatalf("by default an unreviewed skill and a pending note are left out: %v", names(res.Hits))
		}
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "video", Filter: Filter{IncludeUnreviewed: true}})
	if len(res.Hits) != 5 {
		t.Fatalf("a person asks for everything: %v", names(res.Hits))
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "video", Agent: true, Filter: Filter{IncludeUnreviewed: true}})
	for _, k := range names(res.Hits) {
		if k == "skill/thumbnail-pack" || k == "memory/agent-note" {
			t.Fatalf("an agent call never sees what is not reviewed, whatever it asks: %v", names(res.Hits))
		}
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "video", Filter: Filter{Kinds: []core.Kind{core.KindPrompt}}})
	if len(res.Hits) != 1 || res.Hits[0].Entry.Kind != core.KindPrompt {
		t.Fatalf("%v", names(res.Hits))
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "video", Filter: Filter{Tags: []string{"retention"}}})
	if len(res.Hits) != 1 || res.Hits[0].Entry.Name != "viewers-skip-intros" {
		t.Fatalf("%v", names(res.Hits))
	}

	res, _ = m.Search(
		ctx,
		SearchQuery{Query: "video", Filter: Filter{IncludeUnreviewed: true, Origin: core.OriginImported}},
	)
	if len(res.Hits) != 1 || res.Hits[0].Entry.Name != "thumbnail-pack" {
		t.Fatalf("%v", names(res.Hits))
	}
}

func TestSearchAllTermsAreRequiredAndPrefixes(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	seedSearch(t, m)

	ctx := context.Background()

	if res, _ := m.Search(
		ctx,
		SearchQuery{Query: "intr vid", Filter: Filter{IncludeUnreviewed: true}},
	); len(
		res.Hits,
	) != 2 {
		t.Fatalf("%v", names(res.Hits))
	}

	if res, _ := m.Search(ctx, SearchQuery{Query: "intro zebra"}); len(res.Hits) != 0 {
		t.Fatalf("%v", names(res.Hits))
	}

	var ve *ValidationError
	if _, err := m.Search(ctx, SearchQuery{Query: "  *  "}); !errors.As(err, &ve) {
		t.Fatalf("a query with no word is refused: %v", err)
	}
}

func TestSearchFitsTheBudgetAndSaysItWasCut(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	for i := range 60 {
		if _, _, err := m.SaveMemory(ctx, person, MemoryInput{
			Name: fmt.Sprintf("note-%02d", i), Description: "A fact about pacing " + strings.Repeat("x", 40),
			Body: strings.Repeat("pacing matters a lot in a short video. ", 40),
		}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := m.Search(ctx, SearchQuery{Query: "pacing", Budget: 1000})
	if err != nil {
		t.Fatal(err)
	}

	size := 0

	for i := range res.Hits {
		h := res.Hits[i]
		size += len(h.Entry.Name) + len(h.Entry.Description) + len(h.Snippet)

		if len([]rune(h.Snippet)) > core.MaxSnippet+2 {
			t.Fatalf("a snippet is at most %d characters: %d", core.MaxSnippet, len([]rune(h.Snippet)))
		}
	}

	if size > 1100 || !res.Truncated || res.Total != 60 || len(res.Hits) < 1 {
		t.Fatalf("size %d, truncated %v, total %d, hits %d", size, res.Truncated, res.Total, len(res.Hits))
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "pacing", Limit: 5, Budget: core.MaxBudget})
	if len(res.Hits) != 5 || !res.Truncated {
		t.Fatalf("%d %v", len(res.Hits), res.Truncated)
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "pacing", Limit: 500, Budget: 1 << 30})

	chars := 0
	for i := range res.Hits {
		chars += len(res.Hits[i].Entry.Name) + len(res.Hits[i].Entry.Description) + len(res.Hits[i].Snippet)
	}

	if len(res.Hits) > core.MaxHits || chars > core.MaxBudget || !res.Truncated {
		t.Fatalf("the limit and the budget have ceilings: %d hits, %d characters", len(res.Hits), chars)
	}
}

func TestSearchProjectKeysSeeTheirProjectAndTheWorkspace(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	for _, in := range []MemoryInput{
		{Name: "ws", Description: "shared fact", Body: "x"},
		{Name: "mine", Description: "shared fact of mine", Scope: "prj_0123456789", Body: "x"},
		{Name: "theirs", Description: "shared fact of theirs", Scope: "prj_abcdefghjk", Body: "x"},
	} {
		if _, _, err := m.SaveMemory(ctx, person, in); err != nil {
			t.Fatal(err)
		}
	}

	res, _ := m.Search(ctx, SearchQuery{Query: "shared", Filter: Filter{ProjectID: "prj_0123456789"}})
	if got := names(res.Hits); len(got) != 2 {
		t.Fatalf("%v", got)
	}

	res, _ = m.Search(ctx, SearchQuery{Query: "shared", Filter: Filter{Scope: "prj_abcdefghjk"}})
	if got := names(res.Hits); len(got) != 1 || got[0] != "memory/theirs" {
		t.Fatalf("%v", got)
	}
}

func TestSearchSkipsInvalidFilesAndFindsEditsMadeOutside(t *testing.T) {
	t.Parallel()

	m, _, dir := newManager(t)
	ctx := context.Background()

	write(t, dir+"/memory/broken.md", "no front matter but the word zebra\n")
	write(t, dir+"/memory/hand.md", "---\nname: hand\ndescription: By hand\nkept: true\n---\nA zebra crossing.\n")

	res, err := m.Search(ctx, SearchQuery{Query: "zebra", Filter: Filter{IncludeUnreviewed: true}})
	if err != nil || len(res.Hits) != 1 || res.Hits[0].Entry.Name != "hand" {
		t.Fatalf("%v %v", err, names(res.Hits))
	}
}
