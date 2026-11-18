package servicetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

func (m *mediaEnv) assetRow(project, id string) service.ProjectAssetRow {
	m.t.Helper()

	row, err := m.store.Assets().GetProjectAsset(context.Background(), project, id)
	if err != nil {
		m.t.Fatal(err)
	}

	return row
}

func (m *mediaEnv) inDocument(project, id string) bool {
	m.t.Helper()

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		m.t.Fatal(err)
	}

	_, ok := doc.Assets[id]

	return ok
}

// SDD-005 section 18 check 18.
func TestTrashProjectRefusesWritesAndKeepsReads(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.withTrack()
	asset := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]
	rev, ops := m.head(project).Revision, m.operationCount(project)

	if _, err := m.svc.ProjectTrash(m.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	head := m.head(project)
	if head.State != service.ProjectTrashed || head.Revision != rev || m.operationCount(project) != ops {
		t.Errorf("trash changed the log or the revision: %+v, %d operations", head, m.operationCount(project))
	}

	row, err := m.store.Projects().Get(context.Background(), project)
	if err != nil || row.TrashedBy == nil || *row.TrashedBy != ui || row.TrashedMs == nil {
		t.Errorf("trashed_by / trashed_ms = %v / %v, %v", row.TrashedBy, row.TrashedMs, err)
	}

	checks := map[string]func() error{
		"Edit": func() error {
			_, err := m.edit(ui, project, rev, op("track.add", `{"kind":"visual"}`))
			return err
		},
		"Undo": func() error { _, err := m.undo(ui, project); return err },
		"Redo": func() error { _, err := m.redo(project); return err },
		"MediaImport": func() error {
			_, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
				ProjectID: project, Files: []service.ImportFile{{Path: m.write("b.mp4", clipBytes)}},
			})
			return err
		},
		"JobStart": func() error {
			_, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: service.JobExport})
			return err
		},
	}
	for name, call := range checks {
		err := call()
		if code(err) != command.CodeInvalidState {
			t.Errorf("%s of a trashed project = %v, want E_INVALID_STATE", name, err)
		}

		if se, ok := errors.AsType[*service.Error](err); ok && !strings.Contains(se.Hint, "restore the project first") {
			t.Errorf("%s: hint = %q", name, se.Hint)
		}
	}

	reads := map[string]func() error{
		"ProjectGet": func() error {
			_, err := m.svc.ProjectGet(m.as(ui), service.ProjectGetRequest{ProjectID: project})
			return err
		},
		"HistoryList": func() error {
			_, err := m.svc.HistoryList(m.as(ui), service.HistoryListRequest{ProjectID: project})
			return err
		},
		"MediaInspect": func() error {
			_, err := m.svc.MediaInspect(
				m.as(ui),
				service.MediaInspectRequest{ProjectID: project, AssetID: asset.AssetID},
			)
			return err
		},
		"Lint": func() error { _, err := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: project}); return err },
	}
	for name, call := range reads {
		if err := call(); err != nil {
			t.Errorf("%s of a trashed project: %v", name, err)
		}
	}

	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: asset.AssetID},
	); code(err) != command.CodeInvalidState {
		t.Errorf("MediaTrash of a trashed project = %v", err)
	}

	var updated []string

	for _, ev := range m.eventsOf("project.updated") {
		updated = append(updated, ev.State)
	}

	if len(updated) == 0 || updated[len(updated)-1] != "trashed" {
		t.Errorf("project.updated states = %v", updated)
	}
}

// SDD-005 section 18 check 19.
func TestTrashMediaOfAnActiveAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, track := m.withTrack()
	clip := m.write("a.mp4", clipBytes)
	id := m.importOne(project, clip).Assets[0].AssetID

	if !m.inDocument(project, id) {
		t.Fatal("the imported asset is not active")
	}

	// A used asset is refused and says who uses it.
	use := m.mustEdit(ui, project, m.head(project).Revision, op(
		"clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, id),
	))

	_, err := m.svc.MediaTrash(m.as(ui), service.MediaTrashRequest{ProjectID: project, AssetID: id})
	if code(err) != command.CodeInUse {
		t.Fatalf("MediaTrash of a used asset = %v, want E_IN_USE", err)
	}

	if se, _ := errors.AsType[*service.Error](err); se == nil || se.Details["used_by"] == nil {
		t.Errorf("E_IN_USE lacks details.used_by: %+v", err)
	}

	if m.assetRow(project, id).State != service.ProjectAssetActive {
		t.Error("a refused trash changed the row")
	}

	m.mustEdit(
		ui,
		project,
		m.head(project).Revision,
		op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, use.Created[0])),
	)

	ops := m.operationCount(project)

	res, err := m.svc.MediaTrash(m.as(ui), service.MediaTrashRequest{ProjectID: project, AssetID: id})
	if err != nil || res.State != "trashed" {
		t.Fatalf("MediaTrash = %+v, %v", res, err)
	}

	if row := m.assetRow(project, id); row.State != service.ProjectAssetTrashed || len(row.Snapshot) == 0 {
		t.Errorf("row after trash = %+v", row)
	}

	if m.inDocument(project, id) || m.operationCount(project) != ops+1 {
		t.Error("the trash did not leave the document through one batch")
	}

	// Restore re-adds the same identifier.
	if res, err = m.svc.MediaRestore(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: id},
	); err != nil || res.State != "active" {
		t.Fatalf("MediaRestore = %+v, %v", res, err)
	}

	if !m.inDocument(project, id) || m.assetRow(project, id).State != service.ProjectAssetActive {
		t.Error("restore did not bring back the same asset")
	}

	rows, err := m.store.Audit().List(context.Background(), project, 0, 50)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, r := range rows {
		if r.SubjectID != nil && *r.SubjectID == id {
			seen[r.Action+"/"+r.Actor] = true
		}
	}

	for _, want := range []string{"media.trash/" + ui, "media.restore/" + ui} {
		if !seen[want] {
			t.Errorf("audit of the batch path lacks %s: %v", want, seen)
		}
	}

	// Undo of the removal reaches the same state as restore.
	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: id},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := m.undo(ui, project); err != nil {
		t.Fatal(err)
	}

	if !m.inDocument(project, id) || m.assetRow(project, id).State != service.ProjectAssetActive {
		t.Error("undo of asset.remove did not restore the asset")
	}

	// Importing the same file again restores a trashed asset.
	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: id},
	); err != nil {
		t.Fatal(err)
	}

	if again := m.importOne(project, clip).Assets[0]; again.AssetID != id || !m.inDocument(project, id) {
		t.Errorf("re-import = %+v, in document %v", again, m.inDocument(project, id))
	}

	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: "ast_0000000000"},
	); code(err) != command.CodeNotFound {
		t.Errorf("unknown asset = %v", err)
	}
}

func TestTrashMediaOfAPendingAssetIsAuditedWithoutABatch(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	project := m.newProject("p")
	id := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0].AssetID
	ops := m.operationCount(project)

	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: id},
	); err != nil {
		t.Fatal(err)
	}

	if m.assetRow(project, id).State != service.ProjectAssetTrashed || m.operationCount(project) != ops {
		t.Error("a pending asset must change state without a batch")
	}

	if res, err := m.svc.MediaRestore(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: id},
	); err != nil || res.State != "pending" {
		t.Fatalf("MediaRestore = %+v, %v", res, err)
	}

	rows, err := m.store.Audit().List(context.Background(), project, 0, 20)
	if err != nil {
		t.Fatal(err)
	}

	var actions []string

	for _, r := range rows {
		actions = append(actions, r.Action+"/"+r.Actor)
	}

	for _, want := range []string{"media.trash/" + ui, "media.restore/" + ui} {
		found := false

		for _, a := range actions {
			found = found || a == want
		}

		if !found {
			t.Errorf("audit %v lacks %s", actions, want)
		}
	}
}

func TestMediaTrashNeedsTheMediaScope(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	readOnly := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit}},
	)

	for name, call := range map[string]func() error{
		"MediaTrash": func() error {
			_, err := m.svc.MediaTrash(readOnly, service.MediaTrashRequest{ProjectID: project, AssetID: "ast_0000000001"})
			return err
		},
		"MediaRestore": func() error {
			_, err := m.svc.MediaRestore(readOnly, service.MediaTrashRequest{ProjectID: project, AssetID: "ast_0000000001"})
			return err
		},
	} {
		if err := call(); code(err) != service.CodeForbidden {
			t.Errorf("%s without the media scope = %v", name, err)
		}
	}
}

// RF-134: every trash and restore leaves an audit row with its actor. That the
// table refuses to be rewritten is TestAuditAppendOnly of the migrations.
func TestTrashAuditRecordsTheActor(t *testing.T) {
	e := newEnv(t)
	project := e.newProject("p")

	if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	rows, err := e.store.Audit().List(context.Background(), project, 0, 10)
	if err != nil || len(rows) < 2 {
		t.Fatalf("audit = %+v, %v", rows, err)
	}

	for _, r := range rows {
		if r.Actor != ui {
			t.Errorf("audit row %s has actor %q", r.Action, r.Actor)
		}
	}
}
