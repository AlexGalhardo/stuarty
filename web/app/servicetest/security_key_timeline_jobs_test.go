package servicetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// exportableSecond adds a second timeline to project with one visual track and a text, so that an export of it
// passes the lint, and returns its id.
func (m *mediaEnv) exportableSecond(project string) string {
	m.t.Helper()

	added, err := m.prod().ProjectItemAdd(
		m.as(ui), service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Wide", Preset: "16:9"},
	)
	if err != nil {
		m.t.Fatal(err)
	}

	second := added.Item.TimelineID

	tr, err := m.svc.Edit(m.as(ui), service.EditRequest{
		TimelineID:   second,
		BaseRevision: m.head(second).Revision,
		Operations:   []command.Operation{op("track.add", `{"kind":"visual","name":"V"}`)},
	})
	if err != nil {
		m.t.Fatal(err)
	}

	if _, err := m.svc.Edit(m.as(ui), service.EditRequest{
		TimelineID: second, BaseRevision: m.head(second).Revision,
		Operations: []command.Operation{op("text.add",
			fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"hi"}`, tr.Created[0]))},
	}); err != nil {
		m.t.Fatal(err)
	}

	return second
}

// VV-250 (SDD-003 section 1.7 rule 4): a key limited to project A reaches the jobs of every timeline of A,
// and for a timeline of another project it gets the answer it gets for an id that does not exist.
func TestSecurity_T06_LimitedKeyReachesJobsOfItsSecondTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	a := m.exportable()
	a2 := m.exportableSecond(a)
	b := m.exportable()
	b2 := m.exportableSecond(b)

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	// The second timeline of A, by timeline id alone, in the place of the project id, and beside the project.
	for name, req := range map[string]service.JobStartRequest{
		"timeline_id alone":    {TimelineID: a2, Kind: "export"},
		"id in place":          {ProjectID: a2, Kind: "export"},
		"project and timeline": {ProjectID: a, TimelineID: a2, Kind: "export"},
	} {
		job, err := m.svc.JobStart(key, req)
		if err != nil {
			t.Fatalf("JobStart %s: %v", name, err)
		}

		if job.ProjectID == nil || *job.ProjectID != a2 {
			t.Errorf("JobStart %s: job %v, want a job of %q", name, job.ProjectID, a2)
		}

		if _, err := m.svc.JobGet(key, service.JobGetRequest{JobID: job.JobID}); err != nil {
			t.Errorf("JobGet %s: %v", name, err)
		}
	}

	for name, req := range map[string]service.JobListRequest{
		"timeline_id alone":    {TimelineID: a2},
		"id in place":          {ProjectID: a2},
		"project and timeline": {ProjectID: a, TimelineID: a2},
	} {
		list, err := m.svc.JobList(key, req)
		if err != nil {
			t.Fatalf("JobList %s: %v", name, err)
		}

		if len(list.Jobs) != 3 || list.ProjectID != a2 {
			t.Errorf("JobList %s: %d jobs of %q, want 3 of %q", name, len(list.Jobs), list.ProjectID, a2)
		}
	}

	// A timeline of B, B itself and an id of nothing: one answer, and nothing of B is read first.
	missing := "prj_0000000000zz"
	answer := func(err error) string {
		se := serviceError(t, err)

		return fmt.Sprintf("%s|%s|%s", se.Code, se.Message, se.Hint)
	}

	foreign, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: b2, Kind: "export"})
	if err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]string{"timeline of B": b2, "project B": b, "missing": missing} {
		_, startErr := m.svc.JobStart(key, service.JobStartRequest{ProjectID: id, Kind: "export"})
		_, listErr := m.svc.JobList(key, service.JobListRequest{ProjectID: id})

		if code(startErr) != service.CodeForbidden || code(listErr) != service.CodeForbidden {
			t.Errorf("%s: start %v, list %v; want E_FORBIDDEN", name, startErr, listErr)
		}

		if answer(startErr) != answer(listErr) {
			t.Errorf("%s: start and list answer differently", name)
		}

		_, byTimeline := m.svc.JobList(key, service.JobListRequest{ProjectID: a, TimelineID: id})
		if code(byTimeline) != service.CodeForbidden {
			t.Errorf("%s as timeline_id: %v, want E_FORBIDDEN", name, byTimeline)
		}
	}

	_, wantStart := m.svc.JobStart(key, service.JobStartRequest{ProjectID: missing, Kind: "export"})
	for name, id := range map[string]string{"timeline of B": b2, "project B": b} {
		_, got := m.svc.JobStart(key, service.JobStartRequest{ProjectID: id, Kind: "export"})
		if answer(got) != answer(wantStart) {
			t.Errorf("%s: %q differs from the missing id: %q", name, answer(got), answer(wantStart))
		}
	}

	// A job of B is not found or refused the way a job of another project is, never served.
	if _, err := m.svc.JobGet(key, service.JobGetRequest{JobID: foreign.JobID}); err == nil {
		t.Error("JobGet of a job of B by the key of A succeeded")
	}

	if _, err := m.svc.JobCancel(key, service.JobCancelRequest{JobID: foreign.JobID}); err == nil {
		t.Error("JobCancel of a job of B by the key of A succeeded")
	}
}
