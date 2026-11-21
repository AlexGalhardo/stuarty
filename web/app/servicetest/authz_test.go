package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// projectCalls is one call of every operation that names a project (or a job
// of one), for the authorization matrix. job is the id of a job of the project.
func projectCalls(m *mediaEnv, project, job string) map[string]func(ctx context.Context) error {
	asset := "ast_0000000001"
	title := "t"

	return map[string]func(ctx context.Context) error{
		"ProjectGet": func(ctx context.Context) error {
			_, err := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project})
			return err
		},
		"ProjectTrash": func(ctx context.Context) error {
			_, err := m.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: project})
			return err
		},
		"ProjectRestore": func(ctx context.Context) error {
			_, err := m.svc.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: project})
			return err
		},
		"Edit": func(ctx context.Context) error {
			_, err := m.svc.Edit(ctx, service.EditRequest{
				ProjectID: project, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
			})
			return err
		},
		"HistoryList": func(ctx context.Context) error {
			_, err := m.svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: project})
			return err
		},
		"HistoryRevert": func(ctx context.Context) error {
			_, err := m.svc.HistoryRevert(ctx, service.HistoryRevertRequest{ProjectID: project, Revision: 1})
			return err
		},
		"HistoryDiff": func(ctx context.Context) error {
			_, err := m.svc.HistoryDiff(ctx, service.HistoryDiffRequest{ProjectID: project, From: 1})
			return err
		},
		"CheckpointList": func(ctx context.Context) error {
			_, err := m.svc.CheckpointList(ctx, service.CheckpointListRequest{ProjectID: project})
			return err
		},
		"CheckpointCreate": func(ctx context.Context) error {
			_, err := m.svc.CheckpointCreate(ctx, service.CheckpointCreateRequest{ProjectID: project, Name: "n"})
			return err
		},
		"Undo": func(ctx context.Context) error {
			_, err := m.svc.Undo(ctx, service.UndoRequest{ProjectID: project})
			return err
		},
		"Redo": func(ctx context.Context) error {
			_, err := m.svc.Redo(ctx, service.UndoRequest{ProjectID: project})
			return err
		},
		"Lint": func(ctx context.Context) error {
			_, err := m.svc.Lint(ctx, service.LintRequest{ProjectID: project})
			return err
		},
		"MediaImport": func(ctx context.Context) error {
			_, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
				ProjectID: project, Files: []service.ImportFile{{Path: m.write("x.mp4", clipBytes)}},
			})
			return err
		},
		"MediaList": func(ctx context.Context) error {
			_, err := m.svc.MediaList(ctx, service.MediaListRequest{ProjectID: project})
			return err
		},
		"MediaInspect": func(ctx context.Context) error {
			_, err := m.svc.MediaInspect(ctx, service.MediaInspectRequest{ProjectID: project, AssetID: asset})
			return err
		},
		"MediaTrash": func(ctx context.Context) error {
			_, err := m.svc.MediaTrash(ctx, service.MediaTrashRequest{ProjectID: project, AssetID: asset})
			return err
		},
		"MediaRestore": func(ctx context.Context) error {
			_, err := m.svc.MediaRestore(ctx, service.MediaTrashRequest{ProjectID: project, AssetID: asset})
			return err
		},
		"JobStart": func(ctx context.Context) error {
			_, err := m.svc.JobStart(ctx, service.JobStartRequest{ProjectID: project, Kind: service.JobExport})
			return err
		},
		"JobList": func(ctx context.Context) error {
			_, err := m.svc.JobList(ctx, service.JobListRequest{ProjectID: project})
			return err
		},
		"JobGet": func(ctx context.Context) error {
			_, err := m.svc.JobGet(ctx, service.JobGetRequest{JobID: job})
			return err
		},
		"JobCancel": func(ctx context.Context) error {
			_, err := m.svc.JobCancel(ctx, service.JobCancelRequest{JobID: job})
			return err
		},
		"PlanWindow": func(ctx context.Context) error {
			_, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, Until: 30})
			return err
		},
		"ViewFrame": func(ctx context.Context) error {
			_, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "0s"})
			return err
		},
		"ViewSheet": func(ctx context.Context) error {
			_, err := m.svc.ViewSheet(ctx, service.ViewSheetRequest{ProjectID: project})
			return err
		},
		"OpenFile": func(ctx context.Context) error {
			_, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "proxy", ProjectID: project, ID: asset})
			return err
		},
		"TemplateSave": func(ctx context.Context) error {
			_, err := m.svc.TemplateSave(ctx, service.TemplateSaveRequest{ProjectID: project, Name: title})
			return err
		},
		"TrashEmpty": func(ctx context.Context) error {
			_, err := m.svc.TrashEmpty(ctx, service.TrashEmptyRequest{ProjectID: project})
			return err
		},
	}
}

// SDD-007 section 15 check 9 (T-06): a key limited to project A gets
// E_FORBIDDEN for every operation on project B, jobs included.
func TestAuthzProjectLimitedKeyIsForbiddenEverywhereElse(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	a, b := m.exportable(), m.exportable()

	job, err := m.startExport(b)
	if err != nil {
		t.Fatal(err)
	}

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	for name, call := range projectCalls(m, b, job.JobID) {
		if err := call(limited); code(err) != service.CodeForbidden {
			t.Errorf("%s on another project = %v, want E_FORBIDDEN", name, err)
		}
	}
}

// SDD-007 section 15 check 9: in network mode user A gets E_NOT_FOUND for user
// B's project, whatever the operation, with the answer of a project that does
// not exist.
func TestSecurity_T06_CrossUserProjectIsNotFound(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	alice, bob := network("user:alice", "alice"), network("user:bob", "bob")

	res, err := testproject.CreateWithTimeline(bob, m.svc, "theirs")
	if err != nil {
		t.Fatal(err)
	}

	// A job of Bob's project, created by Bob.
	first, err := m.svc.Edit(bob, service.EditRequest{
		ProjectID: res.ProjectID, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.Edit(bob, service.EditRequest{
		ProjectID:    res.ProjectID,
		BaseRevision: 1,
		Operations: []command.Operation{
			op("text.add", `{"track_id":"`+first.Created[0]+`","start":"0s","duration":"1s","text":"x"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	job, err := m.svc.JobStart(bob, service.JobStartRequest{ProjectID: res.ProjectID, Kind: service.JobExport})
	if err != nil {
		t.Fatal(err)
	}

	for name, call := range projectCalls(m, res.ProjectID, job.JobID) {
		if name == "TrashEmpty" {
			continue // refused earlier, for a caller that is not a browser or a key
		}

		if err := call(alice); code(err) != command.CodeNotFound {
			t.Errorf("%s on another user's project = %v, want E_NOT_FOUND", name, err)
		}
	}
}

// A batch needs the union of the scopes of its commands: the first command the
// key may not run fails the whole batch with its op_index and changes nothing.
func TestSecurity_T05_BatchNeedsUnionOfCommandScopes(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.withTrack()
	rev := m.head(project).Revision
	editOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:editor", Scopes: []string{service.ScopeRead, service.ScopeEdit},
	})

	_, err := m.svc.Edit(editOnly, service.EditRequest{
		ProjectID: project, BaseRevision: rev,
		Operations: []command.Operation{
			op("track.add", `{"kind":"visual"}`),
			op("asset.remove", `{"asset_id":"ast_0000000001"}`),
		},
	})

	se := serviceErr(err)
	if se == nil || se.Code != service.CodeForbidden || se.OpIndex == nil || *se.OpIndex != 1 {
		t.Fatalf("edit + media batch with an edit key = %+v, want E_FORBIDDEN at op_index 1", err)
	}

	if m.head(project).Revision != rev {
		t.Error("a refused batch changed the project")
	}
}

// asset.add is internal: no caller can send it, and the answer is the one of a
// name that does not exist; CommandHelp does not list it either.
func TestAuthzInternalCommandIsUnknownToCallers(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.withTrack()

	_, err := m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project, BaseRevision: m.head(project).Revision,
		Operations: []command.Operation{op("asset.add", `{}`)},
	})
	if code(err) != command.CodeUnknownCommand {
		t.Errorf("asset.add = %v, want E_UNKNOWN_COMMAND", err)
	}

	help, err := m.svc.CommandHelp(m.as(ui), service.CommandHelpRequest{})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range help.Commands {
		if c.Name == "asset.add" {
			t.Error("CommandHelp lists the internal command asset.add")
		}
	}

	if _, err := m.svc.CommandHelp(m.as(ui), service.CommandHelpRequest{Name: "asset.add"}); code(err) == "" {
		t.Error("CommandHelp explains asset.add")
	}
}

// A project-limited key is told E_FORBIDDEN only about jobs of its own owner's
// projects; a job of another user answers like a job that does not exist.
func TestAuthzLimitedKeyDoesNotLearnAboutAnotherUsersJob(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	alice, bob := network("user:alice", "alice"), network("user:bob", "bob")

	mk := func(c context.Context) (project string) {
		res, err := testproject.CreateWithTimeline(c, m.svc, "p")
		if err != nil {
			t.Fatal(err)
		}

		first, err := m.svc.Edit(c, service.EditRequest{
			ProjectID: res.ProjectID, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := m.svc.Edit(c, service.EditRequest{
			ProjectID: res.ProjectID, BaseRevision: 1,
			Operations: []command.Operation{
				op("text.add", `{"track_id":"`+first.Created[0]+`","start":"0s","duration":"1s","text":"x"}`),
			},
		}); err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	mine, other, theirs := mk(alice), mk(alice), mk(bob)

	jobOf := func(c context.Context, p string) string {
		j, err := m.svc.JobStart(c, service.JobStartRequest{ProjectID: p, Kind: service.JobExport})
		if err != nil {
			t.Fatal(err)
		}

		return j.JobID
	}

	otherJob, theirJob := jobOf(alice, other), jobOf(bob, theirs)

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, OwnerUserID: "alice", ProjectID: mine,
	})

	get := func(id string) error {
		_, err := m.svc.JobGet(limited, service.JobGetRequest{JobID: id})
		return err
	}
	cancel := func(id string) error {
		_, err := m.svc.JobCancel(limited, service.JobCancelRequest{JobID: id})
		return err
	}

	for name, call := range map[string]func(string) error{"JobGet": get, "JobCancel": cancel} {
		if err := call(otherJob); code(err) != service.CodeForbidden {
			t.Errorf("%s on a job of the owner's other project = %v, want E_FORBIDDEN", name, err)
		}

		hidden, missing := serviceErr(call(theirJob)), serviceErr(call("job_0000000000"))
		if hidden == nil || missing == nil || hidden.Code != command.CodeNotFound || hidden.Code != missing.Code ||
			hidden.Hint != missing.Hint {
			t.Errorf("%s on another user's job = %+v, want the answer of a missing job (%+v)", name, hidden, missing)
		}
	}
}
