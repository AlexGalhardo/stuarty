package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// Epic 56 task 7 (SDD-005 section 5.12.3): a project made since the epic has no timeline until a video is added, and
// a generation belongs to the project, so each of the six project-wide kinds starts in it.
func noTimelineProject(t *testing.T, e *env) string {
	t.Helper()

	made, err := testproject.Create(e.as(ui), e.svc, "No timeline")
	if err != nil {
		t.Fatal(err)
	}

	return made.ProjectID
}

func TestImageAndVoiceGenerateInAProjectWithNoTimeline(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	g.setPrice(voiceModel, "mchar", 15_000_000)
	g.prj = noTimelineProject(t, g.env)

	job, err := g.image(1, "a fox")
	requireWaiting(t, "ImageGenerate", job, err)

	if job.ProjectID == nil || *job.ProjectID != g.prj {
		t.Errorf("the job names %v, want the project %q", job.ProjectID, g.prj)
	}

	job, err = g.voice("hello", "mp3")
	requireWaiting(t, "VoiceGenerate", job, err)
}

func TestTextGenerateAndDraftInAProjectWithNoTimeline(t *testing.T) {
	d := newDraftEnv(t)
	d.prj = noTimelineProject(t, d.env)
	d.item = d.newText(d.prj, "Script", "script", "# Hook\nVO: a person wrote this\n")

	job, err := d.generate(d.as(token), "Caption a video about sourdough.")
	requireWaiting(t, "TextGenerate", job, err)

	job, err = d.draft(d.as(token), "Make it shorter.")
	requireWaiting(t, "TextDraft", job, err)
}

func TestVideoGenerateInAProjectWithNoTimeline(t *testing.T) {
	v := newVideoEnv(t)
	v.prj = noTimelineProject(t, v.env)

	job, err := v.start(5, "p")
	requireWaiting(t, "VideoGenerate", job, err)
}

func TestImageModelEditInAProjectWithNoTimeline(t *testing.T) {
	e := newEditEnv(t)
	e.prj = noTimelineProject(t, e.env)

	job, err := e.removeBackground()
	requireWaiting(t, "ImageModelEdit", job, err)
}

// The job of a project with no timeline is kept under the project, and it is the same row once a video is added:
// the first timeline has the id of the project.
func TestAJobOfAProjectWithNoTimelineIsListedAndKeptWhenAVideoIsAdded(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	g.prj = noTimelineProject(t, g.env)

	if _, err := g.image(1, "a fox"); err != nil {
		t.Fatal(err)
	}

	list, err := g.svc.JobList(g.as(token), service.JobListRequest{ProjectID: g.prj})
	if err != nil || len(list.Jobs) != 1 {
		t.Fatalf("the jobs of the project = %+v, %v", list, err)
	}

	if _, err := g.prod().ProjectItemAdd(
		g.as(ui), service.ProjectItemAddRequest{ProjectID: g.prj, Kind: "timeline", Title: "Video"},
	); err != nil {
		t.Fatal(err)
	}

	if list, err = g.svc.JobList(
		g.as(token),
		service.JobListRequest{ProjectID: g.prj},
	); err != nil ||
		len(list.Jobs) != 1 {
		t.Fatalf("the jobs of the project after the first video = %+v, %v", list, err)
	}
}

// The generation runs to its end in a project with no timeline: the file is stored and attached to the project, and
// the first video added later finds it.
func TestImageAndVoiceGeneratedFilesAttachToAProjectWithNoTimeline(t *testing.T) {
	g := newGenEnv(t, mediaCfg{})
	g.setPrice(imageModel, "image", 40_000)
	g.setPrice(voiceModel, "mchar", 15_000_000)
	g.prj = noTimelineProject(t, g.env)

	done := g.approved(g.image(1, "A red fox in the snow"))
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated == nil {
		t.Fatalf("the image job after approval = %+v", done)
	}

	voice := g.approved(g.voice("Hello there.", "wav"))
	if voice.State != service.JobSucceeded {
		t.Fatalf("the voice job after approval = %+v", voice)
	}

	if n := g.generatedAssets(); n != 2 {
		t.Errorf("the project holds %d generated files, want 2: %+v", n, done.Result.Generated)
	}

	if _, err := g.prod().ProjectItemAdd(
		g.as(ui), service.ProjectItemAddRequest{ProjectID: g.prj, Kind: "timeline", Title: "Video"},
	); err != nil {
		t.Fatal(err)
	}

	got, err := g.svc.ProjectGet(g.as(ui), service.ProjectGetRequest{ProjectID: g.prj, Detail: "document"})
	if err != nil || got.Document == nil || len(got.Document.Assets) != 2 {
		t.Fatalf("the first timeline has %v assets, %v; want the 2 generated files", got.Document, err)
	}
}
