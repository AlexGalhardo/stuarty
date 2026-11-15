package migrations

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem/schema"
)

const (
	artifactKindsBefore = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','transcript'`
	artifactKindsAfter  = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','transcript'`
)

// rebuildArtifacts recreates the artifacts table with the given kind list, keeping the rows whose kind is
// still allowed. SQLite cannot change a CHECK in place; nothing references artifacts, so the copy, drop
// and rename are safe with foreign keys on.
func rebuildArtifacts(s *schema.Schema, kinds string) error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE artifacts_rebuild (
  asset_hash TEXT    NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN (%s)),
  profile    INTEGER NOT NULL,
  variant    TEXT    NOT NULL DEFAULT '',
  ext        TEXT    NOT NULL,
  rel_path   TEXT    NOT NULL,
  size_bytes INTEGER NOT NULL,
  sha256     TEXT    NOT NULL,
  meta       TEXT,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (asset_hash, kind, profile, variant, ext)
)`, kinds),
		fmt.Sprintf(`INSERT INTO artifacts_rebuild (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, meta, created_ms)
SELECT asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, meta, created_ms
FROM artifacts WHERE kind IN (%s)`, strings.TrimSpace(kinds)),
		`DROP TABLE artifacts`,
		`ALTER TABLE artifacts_rebuild RENAME TO artifacts`,
	}

	for _, q := range stmts {
		if err := s.Raw(q); err != nil {
			return err
		}
	}

	return nil
}

// The speech map and the beat grid are artifacts of an asset (SDD-005 section 5.2, RF-043, RF-044).
func m20261115130000ArtifactsSpeechBeatsKinds() schema.Migration {
	return schema.Migration{
		ID:   "20261115130000_artifacts_speech_beats_kinds",
		Up:   func(s *schema.Schema) error { return rebuildArtifacts(s, artifactKindsAfter) },
		Down: func(s *schema.Schema) error { return rebuildArtifacts(s, artifactKindsBefore) },
	}
}
