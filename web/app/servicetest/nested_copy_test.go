package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// compositionTargets are the timelines the composition items of a stored timeline name.
func (e *env) compositionTargets(tl string) []string {
	e.t.Helper()

	doc, err := timeline.Decode(e.document(tl))
	if err != nil {
		e.t.Fatal(err)
	}

	return timeline.CompositionTimelines(doc)
}

// timelinesOf are the active timelines of a production, by title.
func (e *env) timelinesOf(project string) map[string]string {
	e.t.Helper()

	got, err := e.svc.ProjectDescribe(e.as(ui), service.ProjectDescribeRequest{ProjectID: project})
	if err != nil {
		e.t.Fatal(err)
	}

	out := map[string]string{}

	for i := range got.Items {
		if got.Items[i].Kind == "timeline" {
			out[got.Items[i].Title] = got.Items[i].TimelineID
		}
	}

	return out
}

// TestNestedCopiesUseTheirOwnTimelines: a duplicate of a production, and a bundle of it imported as new, give the
// timelines new ids, and a composition of the copy names the copy of its timeline, never the original.
func TestNestedCopiesUseTheirOwnTimelines(t *testing.T) {
	t.Parallel()

	m := newMediaEnv(t, mediaCfg{})
	parent, child, _ := nestedProject(t, m.env)

	copied, err := m.prod().ProjectDuplicate(m.as(ui), service.ProjectDuplicateRequest{ProjectID: parent})
	if err != nil {
		t.Fatal(err)
	}

	intro := m.timelinesOf(copied.ProjectID)["Intro"]
	if intro == "" || intro == child {
		t.Fatalf("the copy has the timelines %v, want a new Intro", m.timelinesOf(copied.ProjectID))
	}

	if got := m.compositionTargets(copied.ProjectID); len(got) != 1 || got[0] != intro {
		t.Errorf("the composition of the duplicate names %v, want its own Intro %s", got, intro)
	}

	bundle := m.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: parent})

	imported, err := m.importBundle(bundle, true)
	if err != nil {
		t.Fatal(err)
	}

	intro = m.timelinesOf(imported.ProjectID)["Intro"]
	if intro == "" || intro == child {
		t.Fatalf("the import has the timelines %v, want a new Intro", m.timelinesOf(imported.ProjectID))
	}

	if got := m.compositionTargets(imported.ProjectID); len(got) != 1 || got[0] != intro {
		t.Errorf("the composition of the import names %v, want its own Intro %s", got, intro)
	}
}
