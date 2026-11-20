package servicetest

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/schema"
)

// Acceptance criterion 1 of epic 06: a workspace that was written before the model of ADR-0039 opens with every
// former project as a project with one timeline, and its history and undo still work.
//
// The workspace is made with this build, taken back to the shape the releases up to 0.29.0 wrote by rolling the
// migration back, and then opened the way a start-up opens it: Open backs the database up and runs the migration.
func TestProjectWorkspaceOfAnEarlierReleaseOpens(t *testing.T) {
	e := newEnv(t)
	ctx := e.as(ui)

	alpha := e.newProject("Alpha")
	e.mustEdit(ui, alpha, 0, op("track.add", `{"kind":"visual"}`))
	e.mustEdit(ui, alpha, 1, op("track.add", `{"kind":"audio"}`))

	if _, err := e.undo(ui, alpha); err != nil { // revision 3 undoes revision 2
		t.Fatal(err)
	}

	beta := e.newProject("Beta")
	e.mustEdit(ui, beta, 0, op("track.add", `{"kind":"visual"}`))

	gone := e.newProject("Gone")
	if _, err := e.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: gone}); err != nil {
		t.Fatal(err)
	}

	docBefore := e.document(alpha)

	if err := e.store.Close(); err != nil {
		t.Fatal(err)
	}

	// Back to the shape of a release 0.29.0 workspace.
	path := filepath.ToSlash(e.dir.Path("db.sqlite"))

	db, err := database.Open(context.Background(), "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	m, err := schema.NewMigrator(db, store.Migrations())
	if err != nil {
		t.Fatal(err)
	}

	// Every later migration goes first (the export_batch kind of epic 35, the library index of epic 15, the
	// approval tables of epic 05, the asset library of epic 08), then project_items itself.
	for {
		rolled, err := m.Rollback(context.Background(), 1)
		if err != nil || len(rolled) != 1 {
			t.Fatalf("rollback = %v, %v", rolled, err)
		}

		if rolled[0] == "20261119120000_project_items" {
			break
		}
	}

	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM pragma_table_info('projects') WHERE name = 'document'").
		Scan(&n); err != nil ||
		n != 1 {
		t.Fatalf("the former projects table is not back: %d, %v", n, err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// The start-up of a build that has the model.
	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	svc, err := service.New(service.Options{
		Store: st, Registry: command.Default(), Now: e.clock.now, NewIDs: e.newID,
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := svc.ProjectList(ctx, service.ProjectListRequest{State: "all"})
	if err != nil || len(list.Projects) != 3 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	byID := map[string]service.ProjectSummary{}
	for _, p := range list.Projects {
		byID[p.ID] = p
	}

	if byID[alpha].Revision != 3 || byID[beta].Revision != 1 || byID[gone].State != "trashed" {
		t.Fatalf("summaries = %+v", byID)
	}

	for id, p := range byID {
		if p.Timelines != 1 {
			t.Errorf("%s has %d timelines", id, p.Timelines)
		}

		got, err := svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: id})
		if err != nil || got.TimelineID != id || len(got.Items) != 1 || got.Items[0].Kind != "timeline" ||
			got.Items[0].TimelineID != id || got.Production.Timelines != 1 {
			t.Fatalf("%s: %+v %v", id, got, err)
		}
	}

	// The document did not change by a byte.
	row, err := st.Projects().Get(context.Background(), alpha)
	if err != nil || !bytes.Equal(row.Document, docBefore) {
		t.Fatalf("the document of Alpha changed: %v", err)
	}

	// The history is whole, and the undo chain works.
	hist, err := svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: alpha})
	if err != nil || len(hist.Entries) != 3 || hist.Entries[0].Kind != "undo" {
		t.Fatalf("history = %+v, %v", hist, err)
	}

	if redo, err := svc.Redo(ctx, service.UndoRequest{ProjectID: alpha}); err != nil || redo.Revision != 4 {
		t.Fatalf("redo = %+v, %v", redo, err)
	}

	if undo, err := svc.Undo(ctx, service.UndoRequest{ProjectID: beta}); err != nil || undo.Revision != 2 {
		t.Fatalf("undo of Beta = %+v, %v", undo, err)
	}

	// And the project can grow a second timeline, which is edited by its id.
	added, err := svc.(service.ProductionService).ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: alpha, Kind: "timeline", Title: "Second",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Edit(ctx, service.EditRequest{
		TimelineID: added.Item.TimelineID, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	}); err != nil {
		t.Fatal(err)
	}
}
