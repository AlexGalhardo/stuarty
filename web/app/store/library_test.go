package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/service/library"
)

func libraryFixture(t *testing.T) (*library.Manager, library.Index) {
	t.Helper()

	d := newDir(t)
	s := openStore(t, d, Options{})

	return library.New(filepath.Join(string(d), "library"), s.Library(), nil), s.Library()
}

func seedLibrary(t *testing.T, m *library.Manager) {
	t.Helper()

	ctx := context.Background()

	skill := core.Skill{
		Name: "hook-writer", Description: "Write the opening of a short video.", License: "MIT",
		Body:  "# Hooks\n\nStart with the result of the video.\n",
		Files: []core.File{{Path: "references/patterns.md", Data: []byte("Pattern: open with a question.\n")}},
	}
	skill.SetOrigin(core.OriginUser, "")
	skill.MarkReviewed()

	for _, err := range []error{
		m.WriteMemory(ctx, core.Memory{
			Name: "viewers-skip-intros", Description: "Viewers skip long intros", Scope: "workspace", Tags: []string{"retention"},
			Actor: "local:ui", Kept: true, Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z",
			Body: "Cut the intro to two seconds. Produção rápida.\n",
		}),
		m.WritePrompt(ctx, core.Prompt{
			Name: "video-intro", Description: "An intro for a video", Actor: "local:ui",
			Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z", Body: "Write an intro about {{topic}}.\n",
		}),
		m.WriteSkill(ctx, skill),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func searchKeys(t *testing.T, idx library.Index, query string) []string {
	t.Helper()

	keys, err := idx.Search(context.Background(), core.Terms(query), 50)
	if err != nil {
		t.Fatal(err)
	}

	return keys
}

func TestLibraryIndexSearchesNamesDescriptionsTagsAndBodies(t *testing.T) {
	m, idx := libraryFixture(t)
	seedLibrary(t, m)

	for query, want := range map[string][]string{
		"intro":                {"memory/viewers-skip-intros", "prompt/video-intro"},
		"hook":                 {"skill/hook-writer"},
		"retention":            {"memory/viewers-skip-intros"},
		"open question":        {"skill/hook-writer"},
		"producao":             {"memory/viewers-skip-intros"},
		"video intro":          {"prompt/video-intro"},
		"video AND OR NEAR *":  nil,
		"\"intro\" -hook":      nil,
		"nothing-matches-this": nil,
	} {
		got := searchKeys(t, idx, query)
		if len(got) != len(want) {
			t.Errorf("%q: got %v, want %v", query, got, want)

			continue
		}

		for _, k := range want {
			found := false

			for _, g := range got {
				found = found || g == k
			}

			if !found {
				t.Errorf("%q: got %v, want %v", query, got, want)
			}
		}
	}

	if got := searchKeys(t, idx, "intro"); got[0] != "prompt/video-intro" && got[0] != "memory/viewers-skip-intros" {
		t.Fatalf("%v", got)
	}

	if got := searchKeys(t, idx, "   "); len(got) != 0 {
		t.Fatalf("an empty query finds nothing: %v", got)
	}
}

// A query is words and never an expression of the engine.
func TestLibraryIndexNeverReadsAQueryAsSyntax(t *testing.T) {
	m, idx := libraryFixture(t)
	seedLibrary(t, m)

	for _, q := range []string{`"`, `intro"`, `NEAR(a b)`, `intro OR hook`, `body:secret`, `*`, `-intro`, `intro*)`, "a\x00b", `'; DROP TABLE library_entries; --`} {
		if _, err := idx.Search(context.Background(), core.Terms(q), 10); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}

	if rows, _ := idx.All(context.Background()); len(rows) != 3 {
		t.Fatalf("%d rows", len(rows))
	}
}

// Deleting the index and running the reindex restores every result (acceptance criterion 2).
func TestLibraryIndexRebuildsFromTheFiles(t *testing.T) {
	m, idx := libraryFixture(t)
	seedLibrary(t, m)

	ctx := context.Background()
	before, _ := idx.All(ctx)

	queries := []string{"intro", "hook", "open question", "retention", "video"}

	results := map[string][]string{}
	for _, q := range queries {
		results[q] = searchKeys(t, idx, q)
	}

	if err := idx.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	if rows, _ := idx.All(ctx); len(rows) != 0 {
		t.Fatal("the index is not empty")
	}

	if got := searchKeys(t, idx, "intro"); len(got) != 0 {
		t.Fatalf("an emptied index finds %v", got)
	}

	st, err := m.Reindex(ctx)
	if err != nil || st.Memory != 1 || st.Skills != 1 || st.Prompts != 1 {
		t.Fatalf("%+v %v", st, err)
	}

	after, _ := idx.All(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rows differ:\n%+v\n%+v", before, after)
	}

	for _, q := range queries {
		if got := searchKeys(t, idx, q); !reflect.DeepEqual(got, results[q]) {
			t.Errorf("%q: %v, was %v", q, got, results[q])
		}
	}
}

func TestLibraryIndexPutReplacesTheTextOfAKey(t *testing.T) {
	m, idx := libraryFixture(t)
	ctx := context.Background()

	note := core.Memory{
		Name: "n", Description: "alpha", Scope: "workspace", Actor: "local:ui", Kept: true,
		Created: "2026-11-19T10:00:00Z", Updated: "2026-11-19T10:00:00Z", Body: "first body\n",
	}

	if err := m.WriteMemory(ctx, note); err != nil {
		t.Fatal(err)
	}

	note.Description, note.Body = "beta", "second body\n"

	if err := m.WriteMemory(ctx, note); err != nil {
		t.Fatal(err)
	}

	if got := searchKeys(t, idx, "alpha"); len(got) != 0 {
		t.Fatalf("the old text is still indexed: %v", got)
	}

	if got := searchKeys(t, idx, "second"); !reflect.DeepEqual(got, []string{"memory/n"}) {
		t.Fatalf("%v", got)
	}

	if err := m.RemoveMemory(ctx, "n"); err != nil {
		t.Fatal(err)
	}

	if got := searchKeys(t, idx, "second"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}
