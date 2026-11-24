package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

const facetrackKindID = "20261124130000_artifacts_facetrack_kind"

// TestArtifactsFacetrackKindKeepsEveryArtifactAndTakesTheFaceTrack: the rebuild keeps the artifacts a workspace
// has, accepts the new kind and still refuses an unknown one; Down drops only the face tracks.
func TestArtifactsFacetrackKindKeepsEveryArtifactAndTakesTheFaceTrack(t *testing.T) {
	db := openWith(t, before(facetrackKindID))
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:a', 'video', 1, 'a.mp4', 1, 1)`,
		`INSERT INTO artifacts (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, created_ms)
 VALUES ('sha256:a', 'transcript', 1, 'abc', 'json', 'artifacts/a/t.json', 10, 'aa', 1)`,
	)

	if ran := upTo(t, db, through(facetrackKindID)); !slices.Equal(ran, []string{facetrackKindID}) {
		t.Fatalf("ran %v", ran)
	}

	if n := count(t, db, `SELECT count(*) FROM artifacts WHERE kind = 'transcript'`); n != 1 {
		t.Errorf("%d transcripts survived the rebuild, want 1", n)
	}

	execAll(
		t,
		db,
		`INSERT INTO artifacts (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, created_ms)
 VALUES ('sha256:a', 'facetrack', 1, 'yunet-5-0-2000', 'json', 'artifacts/a/f.json', 10, 'aa', 2)`,
	)

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO artifacts (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, created_ms)
 VALUES ('sha256:a', 'depth', 1, '', 'json', 'artifacts/a/d.json', 10, 'aa', 3)`,
	); err == nil {
		t.Error("an artifact of an unknown kind was accepted")
	}

	if err := m20261124130000ArtifactsFacetrackKind().Down(schema.New(ctx, db)); err != nil {
		t.Fatal(err)
	}

	if n := count(t, db, `SELECT count(*) FROM artifacts`); n != 1 {
		t.Errorf("%d artifacts after Down, want only the transcript", n)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO artifacts (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, created_ms)
 VALUES ('sha256:a', 'facetrack', 1, 'x', 'json', 'artifacts/a/g.json', 10, 'aa', 4)`,
	); err == nil {
		t.Error("the table Down left accepts a face track")
	}

	if err := m20261124130000ArtifactsFacetrackKind().Up(schema.New(ctx, db)); err != nil {
		t.Fatalf("Up after Down: %v", err)
	}
}
