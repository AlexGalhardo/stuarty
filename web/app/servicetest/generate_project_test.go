package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// VV-330: a generation belongs to the project, not to a timeline. A project that holds two timelines is not asked
// which one, because the operations have no timeline_id; the job is kept under the first active timeline.
func addSecondTimeline(t *testing.T, m *env, project string) string {
	t.Helper()

	added, err := m.prod().ProjectItemAdd(
		m.as(ui), service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second", Preset: "16:9"},
	)
	if err != nil {
		t.Fatal(err)
	}

	return added.Item.TimelineID
}

func requireWaiting(t *testing.T, name string, job *service.Job, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s in a project with two timelines: %v", name, err)
	}

	if job.State != service.JobWaitingApproval {
		t.Errorf("%s: the job is %s, want waiting_approval", name, job.State)
	}
}

func TestGenerationsAcceptAProjectWithTwoTimelines(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	g.setPrice(voiceModel, "mchar", 15_000_000)
	addSecondTimeline(t, g.env, g.prj)

	job, err := g.image(1, "a fox")
	requireWaiting(t, "ImageGenerate", job, err)

	job, err = g.voice("hello", "mp3")
	requireWaiting(t, "VoiceGenerate", job, err)
}

func TestTextGenerateAcceptsAProjectWithTwoTimelines(t *testing.T) {
	te := newTextEnv(t)
	te.setPrice(price)
	addSecondTimeline(t, te.env, te.prj)

	job, err := te.generate(te.as(token), "Caption a video about sourdough.")
	requireWaiting(t, "TextGenerate", job, err)
}

func TestVideoGenerateAcceptsAProjectWithTwoTimelines(t *testing.T) {
	v := newVideoEnv(t)
	addSecondTimeline(t, v.env, v.prj)

	job, err := v.start(5, "p")
	requireWaiting(t, "VideoGenerate", job, err)
}

func TestImageModelEditAcceptsAProjectWithTwoTimelines(t *testing.T) {
	e := newEditEnv(t)
	addSecondTimeline(t, e.env, e.prj)

	job, err := e.removeBackground()
	requireWaiting(t, "ImageModelEdit", job, err)
}

func TestGenerationJobIsKeptUnderTheFirstActiveTimeline(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	second := addSecondTimeline(t, g.env, g.prj)

	if _, err := g.prod().ProjectItemTrash(
		g.as(ui), service.ProjectItemTrashRequest{ProjectID: g.prj, ItemID: firstTimelineItem(t, g.env, g.prj)},
	); err != nil {
		t.Fatal(err)
	}

	job, err := g.image(1, "a fox")
	requireWaiting(t, "ImageGenerate", job, err)

	list, err := g.svc.JobList(g.as(token), service.JobListRequest{ProjectID: second})
	if err != nil || len(list.Jobs) != 1 {
		t.Fatalf("the jobs of the second timeline = %+v, %v", list, err)
	}
}

// SDD-003 section 1.7 rule 2: a timeline id in the place of project_id names that timeline, for a generation as for
// any other operation, so its job, and the files it keeps, belong to that timeline and not to the first one.
func TestGenerationInTheNamedTimelineIsKeptUnderIt(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	second := addSecondTimeline(t, g.env, g.prj)

	job, err := g.svc.ImageGenerate(g.as(token), service.ImageGenerateRequest{
		ProjectID: second, ConnectionID: g.conn, ImageRequest: imageRequest(1, "a fox"),
	})
	requireWaiting(t, "ImageGenerate in the second timeline", job, err)

	if job.ProjectID == nil || *job.ProjectID != second {
		t.Errorf("the job of a generation in %s is kept under %+v", second, job)
	}
}

func TestGenerationInATrashedTimelineIsRefused(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	second := addSecondTimeline(t, g.env, g.prj)

	got, err := g.svc.ProjectGet(g.as(ui), service.ProjectGetRequest{ProjectID: g.prj})
	if err != nil {
		t.Fatal(err)
	}

	for i := range got.Items {
		if got.Items[i].TimelineID == second {
			if _, err := g.prod().ProjectItemTrash(
				g.as(ui), service.ProjectItemTrashRequest{ProjectID: g.prj, ItemID: got.Items[i].ID},
			); err != nil {
				t.Fatal(err)
			}
		}
	}

	_, err = g.svc.ImageGenerate(g.as(token), service.ImageGenerateRequest{
		ProjectID: second, ConnectionID: g.conn, ImageRequest: imageRequest(1, "a fox"),
	})
	if code(err) != "E_INVALID_STATE" {
		t.Errorf("a generation in a timeline in the trash = %v, want E_INVALID_STATE", err)
	}
}

// The project-wide path checks who may reach the project as the timeline path does: a key limited to the project
// reaches it whatever its timelines, a key limited to another and a user who does not own it do not, and a dry
// run of a project with two timelines shows the estimate.
func TestGenerationOfAProjectWithTwoTimelinesKeepsItsAccessRules(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	addSecondTimeline(t, g.env, g.prj)

	start := func(ctx context.Context, dry bool) (*service.Job, error) {
		return g.svc.ImageGenerate(ctx, service.ImageGenerateRequest{
			ProjectID: g.prj, ConnectionID: g.conn, ImageRequest: imageRequest(1, "a fox"), DryRun: dry,
		})
	}

	limited := func(project string) context.Context {
		return service.WithCaller(
			context.Background(), service.Caller{Actor: "key:k", Scopes: service.AllScopes, ProjectID: project},
		)
	}

	job, err := start(limited(g.prj), false)
	requireWaiting(t, "a key limited to the project", job, err)

	if _, err := start(limited("prj_other00000"), false); code(err) != "E_FORBIDDEN" {
		t.Errorf("a key limited to another project = %v, want E_FORBIDDEN", err)
	}

	if _, err := start(asUserKey("u", "usr_someone"), false); code(err) != "E_NOT_FOUND" {
		t.Errorf("a user who does not own the project = %v, want E_NOT_FOUND", err)
	}

	// The first timeline has the id of the project (SDD-003 section 1.7 rule 5).
	jobs := service.JobListRequest{ProjectID: g.prj, TimelineID: g.prj, State: "all"}

	before, err := g.svc.JobList(g.as(token), jobs)
	if err != nil {
		t.Fatal(err)
	}

	dry, err := start(g.as(token), true)
	if err != nil || dry.Estimate == nil {
		t.Fatalf("a dry run in a project with two timelines = %+v, %v", dry, err)
	}

	after, err := g.svc.JobList(g.as(token), jobs)
	if err != nil || len(after.Jobs) != len(before.Jobs) {
		t.Errorf("a dry run wrote a job: %d jobs before, %+v after (%v)", len(before.Jobs), after, err)
	}
}

func firstTimelineItem(t *testing.T, e *env, project string) string {
	t.Helper()

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}

	for i := range got.Items {
		if got.Items[i].Kind == "timeline" {
			return got.Items[i].ID
		}
	}

	t.Fatal("the project has no timeline item")

	return ""
}
