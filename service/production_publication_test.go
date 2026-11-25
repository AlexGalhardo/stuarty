package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/project"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/approval"
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

// noApprovals is a queue with no request, which is what the cost of a workflow asks of it.
type noApprovals struct{ approval.Store }

func (noApprovals) ListRequests(context.Context, approval.Filter) ([]approval.Row, int, error) {
	return nil, 0, nil
}

func (noApprovals) SettledByKind(context.Context, string) ([]approval.KindSpend, error) {
	return nil, nil
}

type publicationsStore struct {
	fakeStore
	pubs pub.Store
}

func (p publicationsStore) Publications() pub.Store { return p.pubs }

func (p publicationsStore) Approvals() approval.Store { return noApprovals{} }

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

	// What waits on the calendar is not known, so no step that depends on it is done (SDD-005 section 5.12.4).
	for _, id := range []project.StepID{project.StepApproval, project.StepPublication} {
		st, ok := res.Workflow.Step(id)
		if ok && st.State != project.StepUnknown {
			t.Errorf("the step %s reads %q with no calendar, want unknown", id, st.State)
		}
	}
}

// A calendar that is read puts its publications into the workflow: one that waits for a person is the approval.
func TestWorkflowReadsThePublicationsOfTheProject(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	repo := fakeItems{items: []ItemRow{
		{
			ID:        "pit_0000000002",
			ProjectID: "prj_0000000001",
			Kind:      "brief",
			Title:     "Brief",
			State:     ProjectActive,
			Body:      "A brief.",
		},
		{
			ID: "pit_0000000001", ProjectID: "prj_0000000001", Kind: "text", Title: "Post", State: ProjectActive,
			TextKind: "post", Body: "Words of the post.",
		},
	}}

	list := []pub.Publication{{
		ID: "pub_0000000001", ProjectID: "prj_0000000001", Title: "Week one", CreatedAt: at, UpdatedAt: at,
		Targets: []pub.Target{targetIn(corepub.TargetWaitingApproval, "youtube")},
	}}

	s := &svc{store: publicationsStore{fakeStore: fakeStore{prod: repo}, pubs: fakePublications{list: list}}}

	res, err := s.ProjectDescribe(
		WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes}),
		ProjectDescribeRequest{ProjectID: "prj_0000000001"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.Kind != "text" || res.Workflow.Current != project.StepApproval {
		t.Fatalf("kind %q, current %q", res.Kind, res.Workflow.Current)
	}

	if st, _ := res.Workflow.Step(
		project.StepApproval,
	); st.State != project.StepWaiting ||
		st.Actor != project.ActorPerson {
		t.Errorf("approval = %+v", st)
	}

	if !strings.Contains(res.Text, "workflow (text):") ||
		!strings.Contains(res.Text, "now approval (waits for a person)") {
		t.Errorf("the text does not say where the project stands:\n%s", res.Text)
	}
}

type brokenPublications struct{ pub.Store }

func (brokenPublications) OfProject(context.Context, string) ([]pub.Publication, error) {
	return nil, ErrNotFound
}

// The next post of a project is the earliest instant of a target that still goes out (core/publish NextPost);
// a project with no such target has none.
func TestProjectDescribeNamesTheNextPost(t *testing.T) {
	t.Parallel()

	soon := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	later := soon.Add(72 * time.Hour)
	past := time.Now().Add(-24 * time.Hour).UTC()

	scheduled := func(state corepub.TargetState, at *time.Time) pub.Publication {
		tg := targetIn(state, "blog")
		tg.ScheduledAt = at

		return pub.Publication{
			ID:        "pub_000000000" + string(state[:1]),
			ProjectID: "prj_0000000001",
			Targets:   []pub.Target{tg},
		}
	}

	describe := func(list ...pub.Publication) *ProjectDescribeResult {
		s := &svc{store: publicationsStore{
			fakeStore: fakeStore{prod: fakeItems{}}, pubs: fakePublications{list: list},
		}}

		res, err := s.ProjectDescribe(
			WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes}),
			ProjectDescribeRequest{ProjectID: "prj_0000000001"},
		)
		if err != nil {
			t.Fatal(err)
		}

		return res
	}

	res := describe(
		scheduled(corepub.TargetScheduled, &later), scheduled(corepub.TargetApproved, &soon),
		scheduled(corepub.TargetPublished, &past), scheduled(corepub.TargetScheduled, &past),
	)
	if res.NextPostAt == nil || !res.NextPostAt.Equal(soon) {
		t.Errorf("next post = %v, want %v", res.NextPostAt, soon)
	}

	if res := describe(
		scheduled(corepub.TargetPublished, &soon),
		scheduled(corepub.TargetDraft, nil),
	); res.NextPostAt != nil {
		t.Errorf("next post = %v, want none", res.NextPostAt)
	}

	if res := describe(); res.NextPostAt != nil {
		t.Errorf("a project with no publication has next post %v", res.NextPostAt)
	}
}
