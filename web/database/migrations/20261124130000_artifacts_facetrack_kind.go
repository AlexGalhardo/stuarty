package migrations

import "github.com/AlexGalhardo/vukem/schema"

const artifactKindsWithFacetrack = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','frames','chunks','transcript','facetrack'`

// The face track of an asset is an artifact: the faces the local helper found in a range of the video, in
// ten-thousandths of the frame (SDD-024 section 6, SDD-005 section 5).
func m20261124130000ArtifactsFacetrackKind() schema.Migration {
	return schema.Migration{
		ID: "20261124130000_artifacts_facetrack_kind",
		Up: func(s *schema.Schema) error { return rebuildArtifacts(s, artifactKindsWithFacetrack) },
		Down: func(s *schema.Schema) error {
			// A face track cannot be kept under the narrower check; it is derived data and a job writes it again.
			if err := s.Raw(`DELETE FROM artifacts WHERE kind = 'facetrack'`); err != nil {
				return err
			}

			return rebuildArtifacts(s, artifactKindsWithChunks)
		},
	}
}
