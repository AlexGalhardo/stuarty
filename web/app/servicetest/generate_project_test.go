package servicetest

import (
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
