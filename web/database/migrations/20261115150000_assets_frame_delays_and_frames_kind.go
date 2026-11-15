package migrations

import "github.com/AlexGalhardo/vukem/schema"

const (
	artifactKindsWithoutFrames = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','transcript'`
	artifactKindsWithFrames    = `'probe','normalized','proxy','thumbnails','peaks','silence','loudness','speech','beats','frames','transcript'`
)

// An animated image keeps the display time of its frames on the asset row (a JSON array of
// milliseconds, NULL for a still) and its frame sequence is an artifact of kind frames (SDD-011
// section 4.7, SDD-005 section 5.5, RF-028).
func m20261115150000AssetsFrameDelaysAndFramesKind() schema.Migration {
	return schema.Migration{
		ID: "20261115150000_assets_frame_delays_and_frames_kind",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`ALTER TABLE assets ADD COLUMN frame_delays TEXT`); err != nil {
				return err
			}

			return rebuildArtifacts(s, artifactKindsWithFrames)
		},
		Down: func(s *schema.Schema) error {
			if err := rebuildArtifacts(s, artifactKindsWithoutFrames); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE assets DROP COLUMN frame_delays`)
		},
	}
}
