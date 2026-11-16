package migrations

import "github.com/AlexGalhardo/vukem/schema"

const artifactKindsWithChunks = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','frames','chunks','transcript'`

// The chunk index of a video proxy is an artifact of an asset: the codec string and the sample table
// the browser needs to decode the proxy with WebCodecs (SDD-011 section 4.3.6, ADR-0029 item 5).
func m20261116120000ArtifactsChunksKind() schema.Migration {
	return schema.Migration{
		ID:   "20261116120000_artifacts_chunks_kind",
		Up:   func(s *schema.Schema) error { return rebuildArtifacts(s, artifactKindsWithChunks) },
		Down: func(s *schema.Schema) error { return rebuildArtifacts(s, artifactKindsWithFrames) },
	}
}
