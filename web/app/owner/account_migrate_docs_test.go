package owner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeline/migrate"
	"github.com/AlexGalhardo/stuarty/service"
)

func migrateFromZero(doc timeline.RawDoc) (timeline.RawDoc, error) {
	out, err := migrate.Run(doc, timeline.CurrentSchemaVersion, migrate.Steps{
		0: func(d migrate.Doc) (migrate.Doc, error) { return d, nil },
	})

	return timeline.RawDoc(out), err
}

// With an account the start-up migration of documents waits for the unlock (ADR-0053 item 6): the document is sealed
// and no key is in memory, so a document of an older schema version keeps its version until a person signs in, and
// is migrated, with its backup, at the unlock and not before.
func TestMigrateStoredDocumentsRunsAtTheUnlockAndNotBefore(t *testing.T) {
	getenv := accountEnv(t)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "timeline", "testdata", "versions", "v1",
		"all-item-types.json"))
	if err != nil {
		t.Fatal(err)
	}

	// The document looks as a binary of schema version 0 wrote it.
	old := bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":0`), 1)

	parsed, err := timeline.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}

	row := service.ProjectRow{
		ID: "prj_7k2m9x4q1b", Name: parsed.Name, State: service.ProjectActive, Revision: 4, SchemaVersion: 0,
		Summary: service.DocSummary{
			Width: parsed.Settings.Width, Height: parsed.Settings.Height, FPSNum: parsed.Settings.FPS.Num,
			FPSDen: parsed.Settings.FPS.Den,
		},
		CreatedMs: 1, UpdatedMs: 1, CreatedBy: "local:ui", Document: old, DocSHA256: "00",
	}

	if err := o.Store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Projects().Create(ctx, row)
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	// A new start: locked. Step 5 of the start-up did not touch the sealed document.
	o2, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = o2.Close() }()

	o2.migrateWith = migrateFromZero

	version := func() int {
		var v int
		if err := o2.Store.DB().QueryRowContext(ctx, `SELECT schema_version FROM timelines WHERE id = ?`,
			row.ID).Scan(&v); err != nil {
			t.Fatal(err)
		}

		return v
	}

	if v := version(); v != 0 {
		t.Fatalf("a document was migrated before the unlock: schema version %d", v)
	}

	if err := o2.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	if v := version(); v != timeline.CurrentSchemaVersion {
		t.Fatalf("the document was not migrated at the unlock: schema version %d", v)
	}

	var backups int
	if err := o2.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM document_backups WHERE owner_id = ?`,
		row.ID).Scan(&backups); err != nil || backups != 1 {
		t.Fatalf("the backup of the document before its migration: %d %v", backups, err)
	}

	got, err := o2.Store.Projects().Get(ctx, row.ID)
	if err != nil || got.SchemaVersion != timeline.CurrentSchemaVersion || len(got.Document) == 0 {
		t.Fatalf("the migrated document reads back: %+v %v", got.ProjectHead, err)
	}
}
