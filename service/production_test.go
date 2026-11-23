package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// fakeProductions answers the three questions the resolution of a timeline asks, from a map of projects to
// the timelines they hold; any other call is a bug of the test and panics on the nil interface.
type fakeProductions struct {
	ProductionRepo
	timelines map[string][]string
	owners    map[string]string
}

func (f fakeProductions) ProjectOfTimeline(_ context.Context, id string) (string, error) {
	for p, tls := range f.timelines {
		if slices.Contains(tls, id) {
			return p, nil
		}
	}

	return "", ErrNotFound
}

func (f fakeProductions) ActiveTimelines(_ context.Context, id string) ([]TimelineRef, error) {
	var out []TimelineRef
	for _, tl := range f.timelines[id] {
		out = append(out, TimelineRef{TimelineID: tl, Title: "t"})
	}

	return out, nil
}

func (f fakeProductions) Get(_ context.Context, id string) (ProductionHead, error) {
	if _, ok := f.timelines[id]; !ok {
		return ProductionHead{}, ErrNotFound
	}

	h := ProductionHead{ID: id}
	if o, ok := f.owners[id]; ok {
		h.OwnerUserID = &o
	}

	return h, nil
}

type fakeStore struct {
	Store
	prod ProductionRepo
}

func (f fakeStore) Productions() ProductionRepo { return f.prod }

func TestProjectResolveTimeline(t *testing.T) {
	t.Parallel()

	s := &svc{store: fakeStore{prod: fakeProductions{
		timelines: map[string][]string{
			"prj_one":  {"prj_one"},
			"prj_two":  {"prj_two", "prj_2b"},
			"prj_bob":  {"prj_bob", "prj_bob2"},
			"prj_all":  {"prj_a", "prj_b"},
			"prj_bob1": {"prj_bob1b"},
		},
		owners: map[string]string{"prj_bob": "bob", "prj_bob1": "bob"},
	}}}

	ctx := context.Background()
	local := Caller{Actor: "local:cli", Scopes: AllScopes}
	alice := Caller{Actor: "user:alice", OwnerUserID: "alice", Scopes: AllScopes}
	limited := Caller{Actor: "key:k", Scopes: AllScopes, ProjectID: "prj_two"}

	cases := []struct {
		name      string
		c         Caller
		project   string
		timeline  string
		want      string
		wantCode  string
		wantInMsg string
	}{
		{"neither", local, "", "", "", "", ""},
		{"one timeline", local, "prj_one", "", "prj_one", "", ""},
		{"a project that is not one is left to the operation", local, "prj_nobody", "", "prj_nobody", "", ""},
		{"a timeline id in the place of the project id", local, "prj_2b", "", "prj_2b", "", ""},
		{"two timelines and none named", local, "prj_two", "", "", "E_VALIDATION", "prj_2b"},
		{"the first of two named", local, "prj_two", "prj_two", "prj_two", "", ""},
		{"the second of two named", local, "prj_two", "prj_2b", "prj_2b", "", ""},
		{"the second alone", local, "", "prj_2b", "prj_2b", "", ""},
		{"the second in both places", local, "prj_2b", "prj_2b", "prj_2b", "", ""},
		{"the first in both places", local, "prj_two", "prj_two", "prj_two", "", ""},
		{"a timeline of another project", local, "prj_one", "prj_2b", "", "E_NOT_FOUND", ""},
		{"a timeline that does not exist", local, "", "prj_zzz", "", "E_NOT_FOUND", ""},
		{"a project of another user, ambiguous, is left to the operation", alice, "prj_bob", "", "prj_bob", "", ""},
		{"a limited key, its own second timeline", limited, "", "prj_2b", "prj_2b", "", ""},
		{"a limited key, another project", limited, "", "prj_a", "", "E_FORBIDDEN", ""},
		{"three choices are all listed", local, "prj_all", "", "", "E_VALIDATION", "prj_b"},
		{"a timeline of a project of another user", alice, "", "prj_bob2", "", "E_NOT_FOUND", ""},
		{"a project of another user whose one timeline has another id", alice, "prj_bob1", "", "prj_bob1", "", ""},
		{"a limited key, a timeline that does not exist", limited, "", "prj_zzz", "", "E_FORBIDDEN", ""},
		{"a limited key, its own project and a missing timeline", limited, "prj_two", "prj_zzz", "", "E_FORBIDDEN", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := s.resolveTimeline(ctx, tc.c, tc.project, tc.timeline)

			if tc.wantCode == "" {
				if err != nil || got != tc.want {
					t.Fatalf("got %q, %v; want %q", got, err, tc.want)
				}

				return
			}

			var se *Error
			ok := errors.As(err, &se)
			if !ok || se.Code != tc.wantCode {
				t.Fatalf("err = %v, want code %s", err, tc.wantCode)
			}

			if tc.wantInMsg != "" && !strings.Contains(se.Hint, tc.wantInMsg) {
				t.Errorf("the hint %q does not name %q", se.Hint, tc.wantInMsg)
			}

			if tc.wantCode == "E_VALIDATION" && se.Path != "timeline_id" {
				t.Errorf("path = %q", se.Path)
			}
		})
	}
}

// A caller who may not reach an id cannot tell from the answer whether it exists: the answer for a timeline of a
// project of another user, or out of the reach of a limited key, is the answer for one that does not exist, and
// no answer names the timeline of a project the caller may not see.
func TestProjectResolveTimelineTellsNoExistence(t *testing.T) {
	t.Parallel()

	s := &svc{store: fakeStore{prod: fakeProductions{
		timelines: map[string][]string{"prj_bob": {"prj_bob", "prj_bob2"}, "prj_bob1": {"prj_bob1b"}},
		owners:    map[string]string{"prj_bob": "bob", "prj_bob1": "bob"},
	}}}

	ctx := context.Background()
	alice := Caller{Actor: "user:alice", OwnerUserID: "alice", Scopes: AllScopes}
	limited := Caller{Actor: "key:k", Scopes: AllScopes, ProjectID: "prj_own"}

	answer := func(c Caller, projectID, timelineID string) string {
		_, err := s.resolveTimeline(ctx, c, projectID, timelineID)

		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%s %s: err = %v", projectID, timelineID, err)
		}

		return se.Code + " " + strings.ReplaceAll(se.Message+" "+se.Hint, timelineID, "<id>")
	}

	if got, want := answer(alice, "", "prj_bob2"), answer(alice, "", "prj_nobodyy"); got != want {
		t.Errorf("another user's timeline: %q; a missing one: %q", got, want)
	}

	if got, want := answer(limited, "", "prj_bob2"), answer(limited, "", "prj_nobodyy"); got != want {
		t.Errorf("limited key, another project's timeline: %q; a missing one: %q", got, want)
	}

	// A project alice may not see is answered as an id of nothing is: the id comes back as it went, naming no timeline
	// of it, and the operation refuses it after checking its request.
	for _, id := range []string{"prj_bob", "prj_bob1", "prj_nobodyy"} {
		if got, err := s.resolveTimeline(ctx, alice, id, ""); err != nil || got != id {
			t.Errorf("%s: got %q, %v; want the id unchanged", id, got, err)
		}
	}
}

// The rules of core/project are checked before the store is touched.
func TestProjectItemAddRefusesBeforeTheStore(t *testing.T) {
	t.Parallel()

	s := &svc{}
	ctx := WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes})
	neg := -1

	bad := map[string]ProjectItemAddRequest{
		"a brief":             {ProjectID: "prj_a", Kind: "brief", Title: "Brief"},
		"a reserved kind":     {ProjectID: "prj_a", Kind: "publication", Title: "Post"},
		"an unknown kind":     {ProjectID: "prj_a", Kind: "video", Title: "x"},
		"no title":            {ProjectID: "prj_a", Kind: "text", Title: ""},
		"a title with a NUL":  {ProjectID: "prj_a", Kind: "text", Title: "a\x00"},
		"a negative position": {ProjectID: "prj_a", Kind: "text", Title: "x", Position: &neg},
		"a huge text": {
			ProjectID: "prj_a",
			Kind:      "text",
			Title:     "x",
			Text:      strings.Repeat("a", project.MaxText+1),
		},
		"a bad preset":            {ProjectID: "prj_a", Kind: "timeline", Title: "x", Preset: "3:2"},
		"an output without a job": {ProjectID: "prj_a", Kind: "output", Title: "x"},
	}

	for name, req := range bad {
		if _, err := s.ProjectItemAdd(ctx, req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if _, err := s.BriefSet(
		ctx,
		BriefSetRequest{ProjectID: "prj_a", Text: strings.Repeat("a", project.MaxText+1)},
	); err == nil ||
		codeOf(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("an oversized brief: %v", err)
	}

	if _, err := s.ProjectDescribe(ctx, ProjectDescribeRequest{ProjectID: "prj_a", MaxTokens: 10}); err == nil ||
		codeOf(err) != "E_VALIDATION" {
		t.Errorf("a budget below the minimum: %v", err)
	}

	noEdit := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}})
	if _, err := s.BriefSet(
		noEdit,
		BriefSetRequest{ProjectID: "prj_a", Text: "x"},
	); err == nil ||
		codeOf(err) != CodeForbidden {
		t.Errorf("a read key wrote the brief: %v", err)
	}
}

func TestProjectDescribeTextStaysInTheBudget(t *testing.T) {
	t.Parallel()

	v := textview.ProductionView{
		ID: "prj_0000000001", Name: "Launch", State: "active", HasBrief: true, Timelines: 2,
		Brief: strings.Repeat("A line of the brief.\n", 2000),
	}

	for i := range 300 {
		v.Lines = append(
			v.Lines,
			textview.ProductionLine{ID: "pit_0000000001", Kind: "text", Title: strings.Repeat("t", i%50+1)},
		)
	}

	for _, tokens := range []int{200, 1500, 6000} {
		text := textview.Production(&v, tokens)
		if limit := tokens*3 + 400; len([]rune(text)) > limit {
			t.Errorf("%d tokens: %d characters, at most %d", tokens, len([]rune(text)), limit)
		}

		for _, want := range []string{"project prj_0000000001", "production: brief yes", "brief:", "items:", `truncated="true"`, "read all of it with brief"} {
			if !strings.Contains(text, want) {
				t.Errorf("%d tokens: the text lacks %q", tokens, want)
			}
		}

		if !strings.Contains(text, "more items; read them with project_get") {
			t.Errorf("%d tokens: the cut list is not said", tokens)
		}
	}
}

func codeOf(err error) string {
	if se, ok := errors.AsType[*Error](err); ok {
		return se.Code
	}

	return ""
}
