package migrations

import (
	"context"
	"slices"
	"testing"
)

const imageExportKindID = "20261120130000_jobs_image_export_kind"

// TestJobsImageExportKindKeepsEveryJobAndTakesAProjectlessImageExport: the rebuild keeps every row of a
// workspace that has jobs, accepts a job of the kind image.export with no project and no asset, and still
// refuses the other kinds without their owner; Down drops only the image.export rows.
func TestJobsImageExportKindKeepsEveryJobAndTakesAProjectlessImageExport(t *testing.T) {
	db := openWith(t, before(imageExportKindID))
	ctx := context.Background()

	insertTimeline(t, db, "prj_a")
	insertTimeline(t, db, "prj_b")
	insertEveryJob(t, db)

	want := jobRows(t, db)

	if ran := upTo(t, db, through(imageExportKindID)); !slices.Equal(ran, []string{imageExportKindID}) {
		t.Fatalf("ran %v", ran)
	}

	if got := jobRows(t, db); !slices.Equal(got, want) {
		t.Errorf("rows after the rebuild:\n%q\nwant\n%q", got, want)
	}

	execAll(t, db, `INSERT INTO jobs (id, kind, actor, state, input, created_ms, updated_ms)
 VALUES ('job_img', 'image.export', 'local:ui', 'succeeded', '{"image_id":"img_a"}', 70, 70)`)

	for name, q := range map[string]string{
		"an image export with a project": `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_x1', 'image.export', 'prj_b', 'a', '{}', 1, 1)`,
		"an export with no project":      `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms) VALUES ('job_x2', 'export', 'a', '{}', 1, 1)`,
		"an ingest with no asset":        `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms) VALUES ('job_x3', 'ingest', 'a', '{}', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if n := count(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN
 ('jobs_project_id_created_ms_index','jobs_kind_state_index','jobs_active_ingest_unique')`); n != 3 {
		t.Errorf("%d of the 3 indexes of jobs survived the rebuild", n)
	}

	if n := count(t, db, `SELECT count(*) FROM pragma_table_info('jobs') WHERE name = 'approval_id'`); n != 1 {
		t.Error("the rebuild lost approval_id")
	}
}
