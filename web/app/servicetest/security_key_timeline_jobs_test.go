package servicetest

import (
	"context"
	"fmt"
	"slices"
	"strings"
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

// VV-250 beyond the jobs (SDD-003 section 1.7 rules 2 and 4): every operation that takes a project id accepts the
// id of a timeline in its place, and a key limited to project A reaches A's second timeline that way. A timeline
// of B, B itself and an id of nothing get one refusal, the one a project id outside the key always got.
func TestSecurity_T06_LimitedKeyReachesItsSecondTimelineByID(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	a := m.exportable()
	a2 := m.exportableSecond(a)
	b := m.exportable()
	b2 := m.exportableSecond(b)

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	calls := map[string]func(id string) error{
		"ProjectGet": func(id string) error {
			_, err := m.svc.ProjectGet(key, service.ProjectGetRequest{ProjectID: id})
			return err
		},
		"HistoryList": func(id string) error {
			_, err := m.svc.HistoryList(key, service.HistoryListRequest{ProjectID: id})
			return err
		},
		"MediaList": func(id string) error {
			_, err := m.svc.MediaList(key, service.MediaListRequest{ProjectID: id})
			return err
		},
		"Lint": func(id string) error {
			_, err := m.svc.Lint(key, service.LintRequest{ProjectID: id})
			return err
		},
		"MotionBrief": func(id string) error {
			_, err := m.svc.MotionBrief(key, service.MotionBriefRequest{ProjectID: id})
			return err
		},
	}

	for name, call := range calls {
		if err := call(a2); err != nil {
			t.Errorf("%s of the second timeline of A by its id: %v", name, err)
		}
	}

	// The replacement of epic 33 reaches the second timeline of A too: an asset A does not hold is not found there,
	// where B's timeline is refused.
	replace := func(id string) error {
		_, err := m.svc.MotionReplace(key, service.MotionReplaceRequest{ProjectID: id, AssetID: "ast_0000000000"})
		return err
	}

	if got := code(replace(a2)); got != "E_NOT_FOUND" {
		t.Errorf("MotionReplace on the second timeline of A = %s, want E_NOT_FOUND", got)
	}

	if got := code(replace(b2)); got != service.CodeForbidden {
		t.Errorf("MotionReplace on the timeline of B = %s, want %s", got, service.CodeForbidden)
	}

	if _, err := m.svc.Edit(key, service.EditRequest{
		ProjectID: a2, BaseRevision: m.head(a2).Revision,
		Operations: []command.Operation{op("track.add", `{"kind":"audio","name":"A"}`)},
	}); err != nil {
		t.Errorf("Edit of the second timeline of A by its id: %v", err)
	}

	answer := func(err error) string {
		se := serviceError(t, err)

		return fmt.Sprintf("%s|%s|%s", se.Code, se.Message, se.Hint)
	}

	for name, call := range calls {
		want := answer(call("prj_0000000000zz"))

		for what, id := range map[string]string{"timeline of B": b2, "project B": b} {
			if got := answer(call(id)); got != want || code(call(id)) != service.CodeForbidden {
				t.Errorf("%s of the %s = %q, want the refusal of a missing id %q", name, what, got, want)
			}
		}
	}
}

// VV-250 in the asset library: a key limited to project A sees the files of every timeline of A, and a file held
// only by a timeline of B answers as a hash that does not exist.
func TestSecurity_T06_LimitedKeyReachesTheFilesOfItsSecondTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	a, _ := m.withTrack()
	a2 := m.exportableSecond(a)
	b, _ := m.withTrack()
	b2 := m.exportableSecond(b)

	imp := func(timeline, name, body string) string {
		res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
			ProjectID: timeline, Files: []service.ImportFile{{Path: m.write(name, body)}},
		})
		if err != nil || res.Failed != 0 {
			t.Fatalf("import = %+v, %v", res, err)
		}

		return res.Assets[0].Hash
	}

	mine, theirs := imp(a2, "wide.mp4", clipBytes+" a2"), imp(b2, "other.mp4", clipBytes+" b2")

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	list, err := m.svc.AssetList(key, service.AssetListRequest{})
	if err != nil || !slices.Equal(assetIDs(list), []string{mine}) {
		t.Errorf("the key's library = %v, %v; want the file of its second timeline only", assetIDs(list), err)
	}

	if got, err := m.svc.AssetGet(key, service.AssetRef{Hash: mine}); err != nil || got.Projects != 1 {
		t.Errorf("the file of the second timeline of A = %+v, %v", got, err)
	}

	_, hidden := m.svc.AssetGet(key, service.AssetRef{Hash: theirs})
	_, missing := m.svc.AssetGet(key, service.AssetRef{Hash: "sha256:" + strings.Repeat("0", 64)})

	h, mi := serviceError(t, hidden), serviceError(t, missing)
	if h.Code != mi.Code || h.Hint != mi.Hint ||
		strings.ReplaceAll(
			h.Message,
			theirs,
			"",
		) != strings.ReplaceAll(
			mi.Message,
			"sha256:"+strings.Repeat("0", 64),
			"",
		) {
		t.Errorf("a file of B must answer as a missing one: %+v / %+v", h, mi)
	}
}

// VV-255: a key limited to project A lists and reads the approval requests of the jobs of A's second timeline
// (a request carries the id of its timeline), and a request of a timeline of B answers as one that does not exist.
func TestSecurity_T06_LimitedKeyReachesTheApprovalsOfItsSecondTimeline(t *testing.T) {
	e := newJobEnv(t, &probe{estimate: usd(500_000)})
	second := func(project string) string {
		added, err := e.prod().ProjectItemAdd(
			e.as(ui),
			service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Wide", Preset: "16:9"},
		)
		if err != nil {
			t.Fatal(err)
		}

		return added.Item.TimelineID
	}

	a, b := e.newProject("a"), e.newProject("b")
	a2, b2 := second(a), second(b)

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	mine, err := e.svc.JobStart(key, service.JobStartRequest{ProjectID: a2, Kind: testKind})
	if err != nil || mine.ApprovalID == "" {
		t.Fatalf("the key's job on A2 = %+v, %v", mine, err)
	}

	theirs, err := e.svc.JobStart(e.as(ui), service.JobStartRequest{ProjectID: b2, Kind: testKind})
	if err != nil || theirs.ApprovalID == "" {
		t.Fatalf("a job on B2 = %+v, %v", theirs, err)
	}

	ids := func(res *service.ApprovalListResult) []string {
		var out []string
		for i := range res.Requests {
			out = append(out, res.Requests[i].ID)
		}

		return out
	}

	for name, req := range map[string]service.ApprovalListRequest{
		"no project": {}, "the project": {ProjectID: a}, "the timeline": {ProjectID: a2},
	} {
		list, lerr := e.svc.ApprovalList(key, req)
		if lerr != nil || !slices.Equal(ids(list), []string{mine.ApprovalID}) {
			t.Errorf("ApprovalList %s = %v, %v; want the request of A2 only", name, ids(list), lerr)
		}
	}

	if got, gerr := e.svc.ApprovalGet(key, service.ApprovalRef{ID: mine.ApprovalID}); gerr != nil ||
		got.ProjectID == nil || *got.ProjectID != a2 {
		t.Errorf("ApprovalGet of the request of A2 = %+v, %v", got, gerr)
	}

	_, hidden := e.svc.ApprovalGet(key, service.ApprovalRef{ID: theirs.ApprovalID})
	_, missing := e.svc.ApprovalGet(key, service.ApprovalRef{ID: "apr_0000000000"})

	h, mi := serviceError(t, hidden), serviceError(t, missing)
	if h.Code != mi.Code || h.Hint != mi.Hint ||
		strings.ReplaceAll(h.Message, theirs.ApprovalID, "") != strings.ReplaceAll(mi.Message, "apr_0000000000", "") {
		t.Errorf("a request of B must answer as a missing one: %+v / %+v", h, mi)
	}

	if _, lerr := e.svc.ApprovalList(
		key,
		service.ApprovalListRequest{ProjectID: b2},
	); code(
		lerr,
	) != service.CodeForbidden {
		t.Errorf("ApprovalList of a timeline of B = %v, want E_FORBIDDEN", lerr)
	}
}
