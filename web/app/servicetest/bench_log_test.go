package servicetest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// longLogRows is the size of the log of the reviews of P4-SRV-002: a project that was edited for a
// long time, far beyond the 1,000 revisions that a conflict analysis looks back.
const longLogRows = 100_000

// padLog appends rows empty edits to the log of the project and moves its revision, so a benchmark or
// a test runs against a log of that size without paying for rows of real commits. The rows are closed
// (no undo chain reaches them) and carry no operation: the cost they add is the index and the table.
func (e *env) padLog(tb testing.TB, project string, rows int) {
	tb.Helper()

	ctx := context.Background()
	head := e.head(project)

	var schemaVersion int

	var sha string

	err := e.store.DB().QueryRowContext(ctx,
		"SELECT schema_version, doc_sha256 FROM operations WHERE project_id = ? ORDER BY revision DESC LIMIT 1",
		project).Scan(&schemaVersion, &sha)
	if err != nil {
		tb.Fatal(err)
	}

	first, last := head.Revision+1, head.Revision+int64(rows)

	_, err = e.store.DB().ExecContext(ctx, `
INSERT INTO operations (project_id, revision, kind, actor, base_revision, at_ms, ops, inverse, diff,
  schema_version, doc_sha256, closed_by)
WITH RECURSIVE n(i) AS (SELECT ? UNION ALL SELECT i + 1 FROM n WHERE i < ?)
SELECT ?, i, 'edit', 'local:pad', i - 1, 0, '[]', '[]', '[]', ?, ?, i FROM n`,
		first, last, project, schemaVersion, sha)
	if err != nil {
		tb.Fatal(err)
	}

	if _, err = e.store.DB().
		ExecContext(ctx, "UPDATE projects SET revision = ? WHERE id = ?", last, project); err != nil {
		tb.Fatal(err)
	}
}

// SDD-005 section 9.2 step 2: a base_revision far behind a log of 100,000 rows is refused from the
// two revision numbers alone, and one at the edge of the lookback reads at most 1,000 rows.
func TestConflictAnalysisIsBoundedOnALongLog(t *testing.T) {
	e := newEnv(t)
	project, track := e.withTrack()
	e.padLog(t, project, longLogRows)

	head := e.head(project).Revision
	add := op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"500s","duration":"1s","text":"late"}`, track))

	t0 := time.Now()
	_, err := e.edit(ui, project, 0, add)
	zero := time.Since(t0)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeRevisionConflict {
		t.Fatalf("base_revision 0 on a log of %d rows = %v, want %s", head, err, command.CodeRevisionConflict)
	}

	t0 = time.Now()
	_, err = e.edit(ui, project, head-1000, add)
	edge := time.Since(t0)

	if err != nil {
		t.Fatalf("base_revision at the edge of the lookback = %v", err)
	}

	t.Logf("log of %d rows: base_revision 0 refused in %s, base_revision head-1000 applied in %s", head, zero, edge)

	if zero > time.Second || edge > time.Second {
		t.Errorf("the conflict analysis is not bounded: %s and %s", zero, edge)
	}
}
