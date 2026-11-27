package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/testproject"
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

	mine, err := testproject.CreateWithTimeline(alice, e.svc, "alice project")
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

func TestActivityList(t *testing.T) {
	e := newEnv(t)
	a, b := e.newProject("Project Alpha"), e.newProject("Project Beta")

	agentCaller := callerOf("local:chat-claude", "", service.ScopeRead, service.ScopeEdit)
	if _, err := e.svc.ProjectTrash(agentCaller, service.ProjectTrashRequest{ProjectID: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ProjectRestore(e.as(ui), service.ProjectTrashRequest{ProjectID: a}); err != nil {
		t.Fatal(err)
	}

	// Read-only caller (no admin scope) can read activity history.
	readOnly := callerOf("key:r", "", service.ScopeRead)
	all, err := e.svc.ActivityList(readOnly, service.ActivityListRequest{})
	if err != nil || len(all.Entries) < 3 {
		t.Fatalf("activity = %+v, %v", all, err)
	}

	// Filter by agent actor_kind returns only agent-attributed entries.
	agents, err := e.svc.ActivityList(readOnly, service.ActivityListRequest{ActorKind: "agent"})
	if err != nil || len(agents.Entries) == 0 {
		t.Fatalf("agent entries = %+v, %v", agents, err)
	}
	for _, en := range agents.Entries {
		if en.ActorKind != "agent" {
			t.Errorf("expected actor_kind=agent, got %+v", en)
		}
	}

	// Filter by project returns only that project's entries with resolved project_name.
	onlyA, err := e.svc.ActivityList(readOnly, service.ActivityListRequest{ProjectID: a})
	if err != nil || len(onlyA.Entries) == 0 {
		t.Fatalf("project A entries = %+v, %v", onlyA, err)
	}
	for _, en := range onlyA.Entries {
		if en.ProjectID != a || en.ProjectName != "Project Alpha" {
			t.Errorf("unexpected entry in project A history: %+v", en)
		}
	}

	// Project-scoped key cannot read another project's history.
	keyForA := service.WithCaller(t.Context(), service.Caller{
		Actor: "key:a", ProjectID: a, Scopes: []string{service.ScopeRead},
	})
	if _, err := e.svc.ActivityList(
		keyForA,
		service.ActivityListRequest{ProjectID: b},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("project-scoped key on another project = %v, want E_FORBIDDEN", err)
	}

	// Validation errors reject unknown actor_kind and domain.
	if _, err := e.svc.ActivityList(
		readOnly,
		service.ActivityListRequest{ActorKind: "bot"},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("invalid actor_kind = %v, want E_VALIDATION", err)
	}
	if _, err := e.svc.ActivityList(
		readOnly,
		service.ActivityListRequest{Domain: "unknown"},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("invalid domain = %v, want E_VALIDATION", err)
	}

	// Network-mode user isolation: Alice sees only her own projects' activity.
	alice := callerOf("user:alice", "alice", service.AllScopes...)
	mine, err := testproject.CreateWithTimeline(alice, e.svc, "Alice Production")
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.ActivityList(alice, service.ActivityListRequest{})
	if err != nil || len(got.Entries) == 0 {
		t.Fatalf("alice activity = %+v, %v", got, err)
	}
	for _, en := range got.Entries {
		if en.ProjectID != mine.ProjectID {
			t.Errorf("alice sees another user's entry: %+v", en)
		}
	}
}

func TestActivityListPagesAcrossAuditAndTimelineRevisions(t *testing.T) {
	e := newEnv(t)
	project := e.newProject("Activity Before")
	e.mustEdit(ui, project, 0, op("project.rename", `{"name":"Activity After"}`))

	req := service.ActivityListRequest{ProjectID: project, Limit: 1}
	seen := map[string]bool{}
	sawRevision, sawCreate := false, false
	for range 10 {
		page, err := e.svc.ActivityList(e.as(ui), req)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if seen[entry.ID] || entry.ProjectID != project {
				t.Fatalf("unexpected entry: %+v", entry)
			}
			seen[entry.ID] = true
			sawCreate = sawCreate || entry.Action == "project.create"
			sawRevision = sawRevision || (entry.Action == "timeline.edit" &&
				entry.Revision == 1 && entry.SubjectID == project)
		}
		if page.NextCursor == nil {
			if !sawRevision || !sawCreate {
				t.Fatalf("revision = %v, create = %v", sawRevision, sawCreate)
			}
			return
		}
		req.Cursor = *page.NextCursor
	}
	t.Fatal("activity pagination did not end")
}

func TestActivityListReadsJobAndAgentMetadata(t *testing.T) {
	e := newEnv(t)
	project := e.newProject("Activity metadata")
	ctx := e.as(ui)
	start := time.Unix(1_789_000_000, 0).UTC()
	at := start.UnixMilli()
	const private = "PRIVATE_TEST_PROMPT"
	const jobID, sessionID = "job_activity", "cht_abcdefghjk"

	err := e.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		job := service.JobRow{
			ID: jobID, Kind: service.JobImageGenerate, ProjectID: &project, Actor: "local:chat-abcdefghjk",
			State: service.JobQueued, CreatedMs: at, UpdatedMs: at,
			Input: json.RawMessage(`{"prompt":"` + private + `"}`),
		}
		if err := tx.Jobs().CreateLimited(ctx, job, nil); err != nil {
			return err
		}
		if ok, err := tx.Jobs().Claim(ctx, jobID, at); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("job %s was not claimed", jobID)
		}
		if ok, err := tx.Jobs().Finish(ctx, jobID, service.JobSucceeded, job.Input, nil, at+1); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("job %s was not finished", jobID)
		}
		if err := tx.Chat().SessionCreate(ctx, agenthost.Session{
			ID: sessionID, ProjectID: project, Profile: "stub", StartedBy: ui,
			State: chat.StateIdle, CreatedAt: start, UpdatedAt: start,
		}); err != nil {
			return err
		}
		return tx.Chat().EventUpsert(ctx, sessionID, chat.Event{
			Seq: 1, At: start, Kind: chat.KindToolCall, Tool: &chat.Tool{Title: private},
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.ActivityList(ctx, service.ActivityListRequest{ProjectID: project, ActorKind: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	var toolID string
	for _, entry := range res.Entries {
		got[entry.Action] = entry.Domain
		if entry.Action == "chat.tool_call" {
			toolID = entry.ID
			if entry.ActorLabel != "stub" {
				t.Errorf("unexpected agent display name: %q", entry.ActorLabel)
			}
		}
	}
	if got["job.image.generate.requested"] != "image" || got["job.image.generate.succeeded"] != "image" ||
		got["chat.tool_call"] != "agent" || len(got) != 3 {
		t.Fatalf("unexpected activity: %+v", res.Entries)
	}
	body, err := json.Marshal(res)
	if err != nil || strings.Contains(string(body), private) {
		t.Fatalf("private content in activity response: %v", err)
	}

	if err := e.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Chat().SessionDelete(ctx, sessionID)
	}); err != nil {
		t.Fatal(err)
	}
	archived, err := e.svc.ActivityList(ctx, service.ActivityListRequest{ProjectID: project, Domain: "agent"})
	if err != nil || len(archived.Entries) != 1 || archived.Entries[0].ID != toolID {
		t.Fatalf("expected one tool action with its original ID after deleting the session: %+v, %v", archived, err)
	}
}
