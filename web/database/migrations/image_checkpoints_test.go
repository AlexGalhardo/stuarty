package migrations

import (
	"context"
	"testing"
)

// The checkpoints of an image (SDD-005 section 5.14.1): a name is unique in its image, the rows hold the checks of
// the timeline's table, and a checkpoint leaves with its image.
func TestImageCheckpointsTable(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO image_documents (id, name, revision, schema_version, document, doc_sha256, width, height, created_ms, updated_ms, created_by)
 VALUES ('img_a', 'a', 3, 1, '{}', 'x', 10, 10, 1, 1, 'u')`,
		`INSERT INTO image_checkpoints (image_id, name, revision, actor, at_ms) VALUES ('img_a', 'before the agent', 2, 'u', 1)`,
	)

	for name, q := range map[string]string{
		"a second checkpoint of the same name": `INSERT INTO image_checkpoints (image_id, name, revision, actor, at_ms)
 VALUES ('img_a', 'before the agent', 3, 'u', 1)`,
		"a checkpoint of no image": `INSERT INTO image_checkpoints (image_id, name, revision, actor, at_ms)
 VALUES ('img_none', 'x', 1, 'u', 1)`,
		"an empty name": `INSERT INTO image_checkpoints (image_id, name, revision, actor, at_ms)
 VALUES ('img_a', '', 1, 'u', 1)`,
		"a revision of zero": `INSERT INTO image_checkpoints (image_id, name, revision, actor, at_ms)
 VALUES ('img_a', 'zero', 0, 'u', 1)`,
		"an auto that is not 0 or 1": `INSERT INTO image_checkpoints (image_id, name, revision, actor, auto, at_ms)
 VALUES ('img_a', 'auto', 1, 'u', 2, 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	execAll(t, db, `DELETE FROM image_documents WHERE id = 'img_a'`)

	if n := count(t, db, `SELECT count(*) FROM image_checkpoints`); n != 0 {
		t.Errorf("checkpoints left after the image went: %d", n)
	}
}
