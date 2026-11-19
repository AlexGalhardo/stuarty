package migrations

import "github.com/AlexGalhardo/vukem/schema"

const (
	jobKindsBefore = `kind IN ('ingest','transcribe','export','frame')`
	jobKindsAfter  = `kind IN ('ingest','transcribe','export','export_batch','frame')`
)

// The parent of an export batch is a job of kind export_batch (SDD-000 section 6, SDD-004 section 8.4,
// RF-088). The kind is a check of the table, so the table is rebuilt: the indexes are recreated by Rebuild,
// and the migration owns its transaction because Rebuild turns foreign keys off around it.
func m20261119140000JobsExportBatchKind() schema.Migration {
	return schema.Migration{
		ID:            "20261119140000_jobs_export_batch_kind",
		NoTransaction: true,
		Up:            func(s *schema.Schema) error { return rebuildJobs(s, jobKindsAfter) },
		Down:          func(s *schema.Schema) error { return rebuildJobs(s, jobKindsBefore) },
	}
}

func rebuildJobs(s *schema.Schema, kindCheck string) error {
	if err := rebuildJobsTable(s, kindCheck); err != nil {
		return err
	}

	// The indexes are written out so that the stored SQL reads as it did before (the table builder quotes names),
	// and the partial unique one has a WHERE clause the builder does not express.
	for _, q := range []string{
		`CREATE INDEX jobs_project_id_created_ms_index ON jobs (project_id, created_ms)`,
		`CREATE INDEX jobs_kind_state_index ON jobs (kind, state)`,
		`CREATE UNIQUE INDEX jobs_active_ingest_unique ON jobs (asset_hash) ` +
			`WHERE kind = 'ingest' AND state IN ('queued','running')`,
	} {
		if err := s.Raw(q); err != nil {
			return err
		}
	}

	return nil
}

func rebuildJobsTable(s *schema.Schema, kindCheck string) error {
	return s.Rebuild("jobs", func(t *schema.Table) {
		t.Text("id").Primary()
		t.Text("kind")
		t.Text("project_id").Nullable().Constrained("projects").CascadeOnDelete()
		t.Text("asset_hash").Nullable().Constrained("assets").CascadeOnDelete()
		t.Text("actor")
		t.Text("state").Default("queued")
		t.Float("progress").Default(0)
		t.Text("message").Default("")
		t.Text("input")
		t.Text("result").Nullable()
		t.Text("error").Nullable()
		t.Integer("cancel_requested").Default(0)
		t.Integer("attempts").Default(0)
		t.Integer("created_ms")
		t.Integer("updated_ms")
		t.Integer("started_ms").Nullable()
		t.Integer("finished_ms").Nullable()
		t.Check("jobs_kind_check", kindCheck)
		t.Check("jobs_state_check", `state IN ('queued','running','succeeded','failed','cancelled')`)
		t.Check("jobs_progress_check", `progress >= 0 AND progress <= 1`)
		t.Check(
			"jobs_owner_check",
			`(kind = 'ingest' AND asset_hash IS NOT NULL) OR (kind <> 'ingest' AND project_id IS NOT NULL)`,
		)
	})
}
