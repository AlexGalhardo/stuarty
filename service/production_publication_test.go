package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// fakeItems answers what ProjectDescribe asks of the production: the head and the items of one project.
type fakeItems struct {
	ProductionRepo
	items []ItemRow
}

func (f fakeItems) Get(_ context.Context, id string) (ProductionHead, error) {
	return ProductionHead{ID: id, Name: "Launch", State: ProjectActive}, nil
}

func (f fakeItems) Items(context.Context, string) ([]ItemRow, error) {
	return append([]ItemRow(nil), f.items...), nil
}

// TextRevisions is what the description reads of a text (epic 17): this production keeps none.
func (f fakeItems) TextRevisions(context.Context, string, int, int) ([]TextRevisionRow, error) {
	return nil, nil
}

// fakePublications answers the one question the overview asks of the calendar.
type fakePublications struct {
	pub.Store
	list []pub.Publication
}

func (f fakePublications) OfProject(context.Context, string) ([]pub.Publication, error) {
	return f.list, nil
}

type publicationsStore struct {
	fakeStore
	pubs pub.Store
}

func (p publicationsStore) Publications() pub.Store { return p.pubs }

func targetIn(state corepub.TargetState, network string) pub.Target {
	return pub.Target{ID: "tgt_" + string(state), Network: network, State: state}
}

func TestProjectDescribeShowsThePublicationsOfTheProject(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	repo := fakeItems{items: []ItemRow{{
		ID: "pit_0000000001", ProjectID: "prj_0000000001", Kind: "text", Title: "Script", State: ProjectActive,
		Body: "A script.",
	}}}

	list := []pub.Publication{
		{
			ID:        "pub_0000000001",
			ProjectID: "prj_0000000001",
			Title:     "Week one",
			CreatedAt: at,
			UpdatedAt: at,
			Targets: []pub.Target{
				targetIn(corepub.TargetPublished, "youtube"),
				targetIn(corepub.TargetWaitingApproval, "youtube"),
			},
		},
		{
			ID:        "pub_0000000002",
			ProjectID: "prj_0000000001",
			Title:     "Week two",
			CreatedAt: at,
			UpdatedAt: at,
			Targets: []pub.Target{
				targetIn(corepub.TargetPublished, "youtube"),
				targetIn(corepub.TargetCancelled, "fake"),
			},
		},
		{
			ID: "pub_0000000003", ProjectID: "prj_0000000001", Title: "Week three", CreatedAt: at, UpdatedAt: at,
			Targets: []pub.Target{targetIn(corepub.TargetDraft, "youtube")},
		},
	}

	s := &svc{store: publicationsStore{
		fakeStore: fakeStore{prod: repo}, pubs: fakePublications{list: list},
	}}

	res, err := s.ProjectDescribe(
		WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes}),
		ProjectDescribeRequest{ProjectID: "prj_0000000001"},
	)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]ProjectItem{}
	for _, it := range res.Items {
		byID[it.ID] = it
	}

	for id, want := range map[string]string{
		"pub_0000000001": corepub.ItemPendingApproval, "pub_0000000002": corepub.ItemPublished, "pub_0000000003": "",
	} {
		got, ok := byID[id]
		if !ok || got.Kind != "publication" || got.Status != want || got.State != "active" {
			t.Errorf("%s = %+v, want a publication with the status %q", id, got, want)
		}
	}

	if got := byID["pub_0000000001"].Format; got != "youtube" {
		t.Errorf("networks = %q", got)
	}

	if got := byID["pub_0000000002"].Format; got != "youtube,fake" {
		t.Errorf("networks = %q", got)
	}

	// The production counts what waits for a person and what went out (RF-303).
	if res.Production.PendingApprovals != 1 || res.Production.Published != 1 {
		t.Errorf("production = %+v", res.Production)
	}

	for _, want := range []string{"publication", "Week one", "status=pending_approval", "networks=youtube"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the text lacks %q:\n%s", want, res.Text)
		}
	}
}

// A calendar that cannot be read leaves the production as it was.
func TestProjectDescribeSurvivesACalendarThatCannotBeRead(t *testing.T) {
	t.Parallel()

	s := &svc{store: publicationsStore{fakeStore: fakeStore{prod: fakeItems{}}, pubs: brokenPublications{}}}
	s.log = slog.New(slog.DiscardHandler)

	res, err := s.ProjectDescribe(
		WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes}),
		ProjectDescribeRequest{ProjectID: "prj_0000000001"},
	)
	if err != nil || len(res.Items) != 0 {
		t.Fatalf("ProjectDescribe = %+v, %v", res, err)
	}
}

type brokenPublications struct{ pub.Store }

func (brokenPublications) OfProject(context.Context, string) ([]pub.Publication, error) {
	return nil, ErrNotFound
}
