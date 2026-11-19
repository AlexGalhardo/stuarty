package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

// migrateActor is the actor of the log row a document migration writes: a valid
// actor of SDD-000 section 3.
const migrateActor = "local:migrate"

// MigrateOptions of MigrateStoredDocuments.
type MigrateOptions struct {
	// NowMs is the clock for the backup rows and the log rows.
	NowMs int64
	// Migrate chains the migration functions of a document; the default is
	// timeline.Migrate. Tests replace it to exercise a version that does not exist yet.
	Migrate func(timeline.RawDoc) (timeline.RawDoc, error)
}

// MigrateResult counts what a start-up migrated.
type MigrateResult struct {
	Projects  int
	Templates int
}

// MigrateError is a document that could not be migrated: it names the project
// or template, and the start-up stops (SDD-005 section 8, NFR-012). Nothing of
// that document was written.
type MigrateError struct {
	Kind string // "project" or "template"
	ID   string
	Err  error
}

func (e *MigrateError) Error() string {
	return fmt.Sprintf(
		"migrating the %s %s to schema version %d failed: %v", e.Kind, e.ID, timeline.CurrentSchemaVersion, e.Err,
	)
}

func (e *MigrateError) Unwrap() error { return e.Err }

// MigrateStoredDocuments is step 5 of the start-up order (SDD-005 section 7,
// section 8): every project and template whose schema_version is below the
// current one is migrated in its own write transaction, holding the project
// lock. The original document is kept in document_backups; a project also gets a
// `migrate` log row that closes every undo and redo chain, and a resync floor
// at its new revision, so no client resumes a stream across the migration. A
// failure rolls back that document, names it and stops: a half-migrated
// workspace would break NFR-012. No client is connected at start-up, so no event
// is published.
func MigrateStoredDocuments(ctx context.Context, st Store, o MigrateOptions) (MigrateResult, error) {
	if o.Migrate == nil {
		o.Migrate = timeline.Migrate
	}

	var res MigrateResult

	projects, err := outdatedProjects(ctx, st)
	if err != nil {
		return res, err
	}

	for _, id := range projects {
		migrated, err := migrateProject(ctx, st, id, o)
		if err != nil {
			return res, &MigrateError{Kind: "project", ID: id, Err: err}
		}

		if migrated {
			res.Projects++
		}
	}

	templates, err := outdatedTemplates(ctx, st)
	if err != nil {
		return res, err
	}

	for _, id := range templates {
		migrated, err := migrateTemplate(ctx, st, id, o)
		if err != nil {
			return res, &MigrateError{Kind: "template", ID: id, Err: err}
		}

		if migrated {
			res.Templates++
		}
	}

	return res, nil
}

func outdatedProjects(ctx context.Context, st Store) ([]string, error) {
	var ids []string

	after := ""

	for {
		heads, next, err := st.Projects().List(ctx, ProjectQuery{IncludeTrashed: true, After: after, Limit: 200})
		if err != nil {
			return nil, fmt.Errorf("service: list the projects to migrate: %w", err)
		}

		for i := range heads {
			if heads[i].SchemaVersion < timeline.CurrentSchemaVersion {
				ids = append(ids, heads[i].ID)
			}
		}

		if next == "" {
			return ids, nil
		}

		after = next
	}
}

func outdatedTemplates(ctx context.Context, st Store) ([]string, error) {
	var ids []string

	after := ""

	for {
		rows, next, err := st.Templates().List(ctx, 100, after, "")
		if err != nil {
			return nil, fmt.Errorf("service: list the templates to migrate: %w", err)
		}

		for i := range rows {
			if rows[i].SchemaVersion < timeline.CurrentSchemaVersion {
				ids = append(ids, rows[i].ID)
			}
		}

		if next == "" {
			return ids, nil
		}

		after = next
	}
}

// migrated decodes, migrates and validates the stored bytes of one document.
func migrated(data []byte, migrate func(timeline.RawDoc) (timeline.RawDoc, error)) (*timeline.Document, error) {
	raw, err := timeline.DecodeRaw(data)
	if err != nil {
		return nil, err
	}

	raw, err = migrate(raw)
	if err != nil {
		return nil, err
	}

	doc, err := timeline.FromRaw(raw)
	if err != nil {
		return nil, err
	}

	if vs := timeline.Validate(doc); len(vs) > 0 {
		return nil, fmt.Errorf("the migrated document is not valid: %s", vs[0].String())
	}

	return doc, nil
}

func migrateProject(ctx context.Context, st Store, id string, o MigrateOptions) (bool, error) {
	done := false

	err := st.WriteProject(ctx, id, func(ctx context.Context, tx ProjectTx) error {
		row, err := tx.Projects().Get(ctx, id)
		if err != nil {
			return err
		}

		old := row.SchemaVersion
		if old >= timeline.CurrentSchemaVersion {
			return nil
		}

		doc, err := migrated(row.Document, o.Migrate)
		if err != nil {
			return err
		}

		canon := timeline.Canonical(doc)
		sha := sha256Hex(canon)
		next := row.Revision + 1

		err = tx.Backups().Put(ctx, BackupRow{
			OwnerKind: "project", OwnerID: id, Revision: row.Revision, SchemaVersion: old, Document: row.Document,
			Reason: fmt.Sprintf("migrate v%d to v%d", old, timeline.CurrentSchemaVersion), CreatedMs: o.NowMs,
		})
		if err != nil {
			return err
		}

		diff, err := json.Marshal([]map[string]any{{
			"kind": "project.updated", "id": id,
			"changes": map[string]any{"schema_version": []int{old, timeline.CurrentSchemaVersion}},
		}})
		if err != nil {
			return err
		}

		err = tx.Operations().Append(ctx, OperationRow{
			ProjectID: id, Revision: next, Kind: OpMigrate, Actor: migrateActor, BaseRevision: row.Revision,
			AtMs: o.NowMs, Ops: json.RawMessage("[]"), Inverse: json.RawMessage("[]"), Diff: diff,
			SchemaVersion: timeline.CurrentSchemaVersion, DocSHA256: sha, ClosedBy: &next,
		})
		if err != nil {
			return err
		}

		if err := tx.Operations().CloseAllOpen(ctx, id, next); err != nil {
			return err
		}

		err = tx.Projects().Commit(ctx, id, row.Revision, ProjectUpdate{
			Name: row.Name, SchemaVersion: timeline.CurrentSchemaVersion, Document: canon, DocSHA256: sha,
			Summary: summarize(doc), UpdatedMs: o.NowMs, ResyncFloor: &next,
		})
		if err != nil {
			return err
		}

		done = true

		crashpoint.Hit("migrate.before_commit")

		return nil
	})
	if err == nil {
		crashpoint.Hit("migrate.after_commit")
	}

	if errors.Is(err, ErrNotFound) {
		return false, nil // removed since the listing
	}

	return done, err
}

func migrateTemplate(ctx context.Context, st Store, id string, o MigrateOptions) (bool, error) {
	done := false

	err := st.Write(ctx, func(ctx context.Context, tx Tx) error {
		row, err := tx.Templates().Get(ctx, id)
		if err != nil {
			return err
		}

		old := row.SchemaVersion
		if old >= timeline.CurrentSchemaVersion {
			return nil
		}

		doc, err := migrated(row.Document, o.Migrate)
		if err != nil {
			return err
		}

		err = tx.Backups().Put(ctx, BackupRow{
			OwnerKind: "template", OwnerID: id, SchemaVersion: old, Document: row.Document,
			Reason: fmt.Sprintf("migrate v%d to v%d", old, timeline.CurrentSchemaVersion), CreatedMs: o.NowMs,
		})
		if err != nil {
			return err
		}

		canon := timeline.Canonical(doc)

		slots, err := json.Marshal(slotInfos(doc))
		if err != nil {
			return err
		}

		done = true

		return tx.Templates().UpdateDocument(
			ctx, id, timeline.CurrentSchemaVersion, canon, sha256Hex(canon), slots, summarize(doc),
		)
	})
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}

	return done, err
}
