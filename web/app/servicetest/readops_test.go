package servicetest

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

func callerOf(actor, owner string, scopes ...string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: actor, OwnerUserID: owner, Scopes: scopes})
}

func TestSnapPoints(t *testing.T) {
	e := newEnv(t)
	project, track := e.withTrack()
	e.mustEdit(ui, project, 1, op(
		"text.add", fmt.Sprintf(`{"track_id":%q,"start":"1s","duration":"2s","text":"hello"}`, track),
	))

	res, err := e.svc.SnapPoints(e.as(ui), service.SnapPointsRequest{ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(e.document(project))
	if err != nil {
		t.Fatal(err)
	}

	want := plan.SnapPoints(doc, false)

	if res.Revision != 2 || len(res.Points) != len(want.Points) || len(res.Limits) != len(want.Limits) ||
		res.Points[0].Kind != plan.SnapTimelineStart {
		t.Fatalf("snap = %+v, want the structure of core/plan %+v at revision 2", res, want)
	}

	for i := range want.Points {
		if res.Points[i] != want.Points[i] {
			t.Errorf("point %d = %+v, want %+v", i, res.Points[i], want.Points[i])
		}
	}

	stale := service.SnapPointsRequest{ProjectID: project, Revision: 1}
	if _, err := e.svc.SnapPoints(e.as(ui), stale); code(err) != command.CodeRevisionConflict {
		t.Errorf("a stale revision = %v", err)
	}

	negative := service.SnapPointsRequest{ProjectID: project, Revision: -1}
	if _, err := e.svc.SnapPoints(e.as(ui), negative); code(err) != command.CodeValidation {
		t.Errorf("a negative revision = %v, want E_VALIDATION", err)
	}

	current := service.SnapPointsRequest{ProjectID: project, Revision: 2}
	if _, err := e.svc.SnapPoints(e.as(ui), current); err != nil {
		t.Errorf("the current revision = %v", err)
	}

	alice := callerOf("user:alice", "alice", service.AllScopes...)
	if _, err := e.svc.SnapPoints(alice, service.SnapPointsRequest{ProjectID: project}); code(err) !=
		command.CodeNotFound {
		t.Errorf("a project of another owner = %v", err)
	}

	noRead := callerOf("key:k", "", service.ScopeEdit)
	if _, err := e.svc.SnapPoints(noRead, service.SnapPointsRequest{ProjectID: project}); code(err) !=
		service.CodeForbidden {
		t.Errorf("a key without read = %v", err)
	}
}

func TestFontList(t *testing.T) {
	e := newEnv(t)

	res, err := e.svc.FontList(e.as(ui), service.FontListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	families := make([]string, 0, len(res.Fonts))
	for _, f := range res.Fonts {
		families = append(families, f.Family)

		if !slices.Equal(f.Weights, timeline.BundledWeights[f.Family]) {
			t.Errorf("%s weights = %v", f.Family, f.Weights)
		}
	}

	if len(families) != len(timeline.BundledWeights) || !slices.IsSorted(families) {
		t.Fatalf("families = %v", families)
	}

	noRead := callerOf("key:k", "", service.ScopeEdit)
	if _, err := e.svc.FontList(noRead, service.FontListRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("a key without read = %v", err)
	}
}

// exportJob stores a job row of a project in the given state.
func (e *env) exportJob(project, id, state string) {
	e.t.Helper()

	ctx := context.Background()
	row := service.JobRow{
		ID: id, Kind: service.JobExport, ProjectID: &project, Actor: ui, State: "queued", CreatedMs: 1, UpdatedMs: 1,
	}

	if err := e.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, row, nil)
	}); err != nil {
		e.t.Fatal(err)
	}

	if state == "queued" {
		return
	}

	if ok, err := e.store.Jobs().Claim(ctx, id, 2); err != nil || !ok {
		e.t.Fatalf("claim: %v %v", ok, err)
	}

	result := []byte(`{"export":{"sha256":"0123456789abcdef0123"}}`)
	if ok, err := e.store.Jobs().Finish(ctx, id, state, result, nil, 3); err != nil || !ok {
		e.t.Fatalf("finish: %v %v", ok, err)
	}
}

func TestExportFile(t *testing.T) {
	e := newEnv(t)
	mine := e.newProject("mine")
	theirs := e.newProject("theirs")

	e.exportJob(mine, "job_aaaaaaaaaa", "succeeded")
	e.exportJob(mine, "job_bbbbbbbbbb", "queued")

	f, err := e.svc.ExportFile(e.as(ui), service.ExportFileRequest{JobID: "job_aaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}

	if f.Path != "exports/"+mine+"/job_aaaaaaaaaa.mp4" || f.ContentType != "video/mp4" || !f.Immutable ||
		f.ETag != `"sha256-0123456789abcdef"` || filepath.IsAbs(f.Path) || strings.Contains(f.Path, "..") {
		t.Errorf("file = %+v", f)
	}

	if _, err := e.svc.ExportFile(e.as(ui), service.ExportFileRequest{JobID: "job_bbbbbbbbbb"}); code(err) !=
		command.CodeInvalidState {
		t.Errorf("a job that is not succeeded = %v", err)
	}

	if _, err := e.svc.ExportFile(e.as(ui), service.ExportFileRequest{JobID: "job_zzzzzzzzzz"}); code(err) !=
		command.CodeNotFound {
		t.Errorf("an unknown job = %v", err)
	}

	// T-06: a key limited to another project is forbidden; another user finds nothing.
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:l", Scopes: service.AllScopes, ProjectID: theirs,
	})
	if _, err := e.svc.ExportFile(limited, service.ExportFileRequest{JobID: "job_aaaaaaaaaa"}); code(err) !=
		service.CodeForbidden {
		t.Errorf("a key limited to another project = %v", err)
	}

	bob := callerOf("user:bob", "bob", service.AllScopes...)
	if _, err := e.svc.ExportFile(bob, service.ExportFileRequest{JobID: "job_aaaaaaaaaa"}); code(err) !=
		command.CodeNotFound {
		t.Errorf("a job of another user = %v", err)
	}
}

func TestAuditList(t *testing.T) {
	e := newEnv(t)
	a, b := e.newProject("a"), e.newProject("b")

	for _, p := range []string{a, b} {
		if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: p}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := e.svc.AuditList(e.as(ui), service.AuditListRequest{})
	if err != nil || len(res.Entries) < 2 {
		t.Fatalf("audit = %+v, %v", res, err)
	}

	for i := 1; i < len(res.Entries); i++ {
		if res.Entries[i].ID >= res.Entries[i-1].ID {
			t.Errorf("not newest first: %d then %d", res.Entries[i-1].ID, res.Entries[i].ID)
		}
	}

	var seen []int64

	cursor := ""

	for range 20 {
		page, err := e.svc.AuditList(e.as(ui), service.AuditListRequest{Cursor: cursor, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}

		for _, en := range page.Entries {
			seen = append(seen, en.ID)
		}

		if page.NextCursor == nil {
			break
		}

		cursor = *page.NextCursor
	}

	if len(seen) != len(res.Entries) {
		t.Errorf("paging saw %d entries, the list has %d", len(seen), len(res.Entries))
	}

	only, err := e.svc.AuditList(e.as(ui), service.AuditListRequest{ProjectID: a})
	if err != nil {
		t.Fatal(err)
	}

	for _, en := range only.Entries {
		if en.ProjectID != a {
			t.Errorf("entry of %q in the list of %q", en.ProjectID, a)
		}
	}

	noAdmin := callerOf("key:r", "", service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender)
	if _, err := e.svc.AuditList(noAdmin, service.AuditListRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("a key without admin = %v", err)
	}

	alice := callerOf("user:alice", "alice", service.AllScopes...)

	mine, err := e.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "alice project"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectTrash(alice, service.ProjectTrashRequest{ProjectID: mine.ProjectID}); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.AuditList(alice, service.AuditListRequest{})
	if err != nil || len(got.Entries) == 0 {
		t.Fatalf("alice sees %+v, %v", got, err)
	}

	for _, en := range got.Entries {
		if en.ProjectID != mine.ProjectID {
			t.Errorf("alice sees an entry of %q", en.ProjectID)
		}
	}
}
