package servicetest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeline/migrate"
	"github.com/AlexGalhardo/stuarty/service"
)

// stored is a schema version that no binary writes: the migration of the tests
// brings it to the current one with the chain of the real migrate package.
const stored = 0

func migrateFrom0(doc timeline.RawDoc) (timeline.RawDoc, error) {
	out, err := migrate.Run(doc, timeline.CurrentSchemaVersion, migrate.Steps{
		stored: func(d migrate.Doc) (migrate.Doc, error) { return d, nil },
	})

	return timeline.RawDoc(out), err
}

// downgraded makes the stored bytes of a document look like the ones a binary of
// the version `stored` wrote.
func downgraded(data []byte) []byte {
	return bytes.Replace(data, []byte(`"schema_version":1`), []byte(`"schema_version":0`), 1)
}

func (e *env) migrateOptions() service.MigrateOptions {
	return service.MigrateOptions{NowMs: e.clock.now().UnixMilli(), Migrate: migrateFrom0}
}

func (e *env) insertProject(id string, doc []byte) {
	e.t.Helper()

	parsed, err := timeline.Decode(bytes.Replace(doc, []byte(`"schema_version":0`), []byte(`"schema_version":1`), 1))
	if err != nil {
		e.t.Fatal(err)
	}

	row := service.ProjectRow{
		ID: id, Name: parsed.Name, State: service.ProjectActive, Revision: 4, SchemaVersion: stored,
		Summary: service.DocSummary{
			Width: parsed.Settings.Width, Height: parsed.Settings.Height, FPSNum: parsed.Settings.FPS.Num,
			FPSDen: parsed.Settings.FPS.Den,
		},
		CreatedMs: 1, UpdatedMs: 1, CreatedBy: "local:ui", Document: doc, DocSHA256: "00",
	}

	err = e.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		return tx.Projects().Create(ctx, row)
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) backups(kind, id string) []service.BackupRow {
	e.t.Helper()

	rows, err := e.store.Backups().List(context.Background(), kind, id)
	if err != nil {
		e.t.Fatal(err)
	}

	return rows
}

// SDD-005 section 18 check 26: for every fixture of every version a workspace
// migrates it; the backup is the original bytes, the log has a migrate row and
// the resync floor is its revision.
func TestStartupMigrationOfEveryFixture(t *testing.T) {
	e := newEnv(t)
	fixtures := filepath.Join("..", "..", "..", "core", "timeline", "testdata", "versions", "v*", "*.json")
	roots, err := filepath.Glob(fixtures)

	if err != nil || len(roots) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}

	originals := map[string][]byte{}

	for _, path := range roots {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		id := e.newID("prj")
		originals[id] = downgraded(raw)
		e.insertProject(id, originals[id])

	}

	res, err := service.MigrateStoredDocuments(context.Background(), e.store, e.migrateOptions())
	if err != nil || res.Projects != len(roots) {
		t.Fatalf("migrated %+v, %v; want %d projects", res, err, len(roots))
	}

	for id, original := range originals {
		backups := e.backups("project", id)
		if len(backups) != 1 || backups[0].SchemaVersion != stored || backups[0].Revision != 4 ||
			backups[0].Reason != "migrate v0 to v1" {
			t.Fatalf("%s: backups = %+v", id, backups)
		}

		full, err := e.store.Backups().Get(context.Background(), backups[0].ID)
		if err != nil || !bytes.Equal(full.Document, original) {
			t.Errorf("%s: the backup is not the original bytes (%v)", id, err)
		}

		row, err := e.store.Operations().Get(context.Background(), id, 5)
		if err != nil || row.Kind != service.OpMigrate || row.BaseRevision != 4 || row.Actor != "local:migrate" ||
			row.ClosedBy == nil || *row.ClosedBy != 5 || row.SchemaVersion != timeline.CurrentSchemaVersion {
			t.Errorf("%s: migrate row = %+v, %v", id, row, err)
		}

		head := e.head(id)
		if head.Revision != 5 || head.ResyncFloor != 5 || head.SchemaVersion != timeline.CurrentSchemaVersion {
			t.Errorf("%s: head = %+v", id, head)
		}

		doc, err := timeline.Decode(e.document(id))
		if err != nil || len(timeline.Validate(doc)) != 0 {
			t.Errorf("%s: the migrated document is not valid: %v", id, err)
		}
	}

	// A second start finds nothing to migrate.
	if res, err := service.MigrateStoredDocuments(context.Background(), e.store, e.migrateOptions()); err != nil ||
		res != (service.MigrateResult{}) {
		t.Errorf("second run = %+v, %v", res, err)
	}
}

// SDD-005 section 18 check 17: no undo or redo chain crosses a migration.
func TestStartupMigrationClosesEveryUndoChain(t *testing.T) {
	e := newEnv(t)
	project, track := e.withTrack()

	if _, err := e.addText(ui, project, track, e.head(project).Revision, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	if _, err := e.addText(agent, project, track, e.head(project).Revision, "2s", "world"); err != nil {
		t.Fatal(err)
	}

	for _, actor := range []string{ui, agent} {
		if _, err := e.svc.Undo(e.as(actor), service.UndoRequest{ProjectID: project}); err != nil {
			t.Fatalf("%s cannot undo before the migration: %v", actor, err)
		}
	}

	_, err := e.store.DB().ExecContext(context.Background(),
		`UPDATE projects SET schema_version = 0, document = replace(document, '"schema_version":1', '"schema_version":0')
 WHERE id = ?`, project)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.MigrateStoredDocuments(context.Background(), e.store, e.migrateOptions()); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	for _, actor := range []string{ui, agent, "local:migrate"} {
		if _, err := e.store.Operations().UndoCandidate(ctx, project, actor); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("UndoCandidate(%s) = %v, want ErrNotFound", actor, err)
		}

		if _, err := e.store.Operations().RedoCandidate(ctx, project, actor); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("RedoCandidate(%s) = %v, want ErrNotFound", actor, err)
		}
	}

	_, err = e.svc.Undo(e.as(ui), service.UndoRequest{ProjectID: project})
	if code(err) != service.CodeNothingToUndo {
		t.Errorf("Undo after the migration = %v, want E_NOTHING_TO_UNDO", err)
	}
}

// A migration that fails names the project and leaves it as it was: no backup
// row, no log row, the document and its version untouched.
func TestStartupMigrationFailureNamesTheProjectAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	project, _ := e.withTrack()

	_, err := e.store.DB().ExecContext(context.Background(),
		`UPDATE projects SET schema_version = 0, document = replace(document, '"schema_version":1', '"schema_version":0')
 WHERE id = ?`, project)
	if err != nil {
		t.Fatal(err)
	}

	before := e.document(project)
	opsBefore := e.operationCount(project)
	boom := errors.New("no way to migrate this")

	_, err = service.MigrateStoredDocuments(context.Background(), e.store, service.MigrateOptions{
		NowMs: 1, Migrate: func(timeline.RawDoc) (timeline.RawDoc, error) { return nil, boom },
	})

	var me *service.MigrateError
	if !errors.As(err, &me) || me.ID != project || me.Kind != "project" || !errors.Is(err, boom) ||
		!strings.Contains(err.Error(), project) {
		t.Fatalf("err = %v, want a MigrateError naming %s", err, project)
	}

	if got := e.backups("project", project); len(got) != 0 {
		t.Errorf("a failed migration left backups: %+v", got)
	}

	if !bytes.Equal(e.document(project), before) || e.head(project).SchemaVersion != stored ||
		e.operationCount(project) != opsBefore {
		t.Error("a failed migration changed the project")
	}
}

// A template migrates with the projects: its original is backed up, it gets no
// log row, and its listing summary stays right.
func TestStartupMigrationOfATemplateWritesNoLogRow(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	saved := m.saveTemplate(m.templateProject())
	id := saved.Template.TemplateID

	_, err := m.store.DB().ExecContext(context.Background(),
		`UPDATE templates SET schema_version = 0, document = replace(document, '"schema_version":1', '"schema_version":0')
 WHERE id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}

	row, err := m.store.Templates().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.MigrateStoredDocuments(context.Background(), m.store, m.migrateOptions())
	if err != nil || res.Templates != 1 || res.Projects != 0 {
		t.Fatalf("migrated %+v, %v", res, err)
	}

	backups := m.backups("template", id)
	if len(backups) != 1 || backups[0].Revision != 0 {
		t.Fatalf("backups = %+v", backups)
	}

	full, err := m.store.Backups().Get(context.Background(), backups[0].ID)
	if err != nil || !bytes.Equal(full.Document, row.Document) {
		t.Errorf("the backup is not the original bytes (%v)", err)
	}

	after, err := m.store.Templates().Get(context.Background(), id)
	if err != nil || after.SchemaVersion != timeline.CurrentSchemaVersion {
		t.Fatalf("template = %+v, %v", after, err)
	}

	if _, err := timeline.Decode(after.Document); err != nil {
		t.Errorf("the migrated template does not decode: %v", err)
	}

	if len(m.templateList(m.as(ui))) != 1 {
		t.Error("the migrated template is not listed")
	}
}
