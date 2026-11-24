package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The tracks, the pauses and the pointer events of a recording (SDD-025 section 16, SDD-005 section 5.18). SQLite
// cannot change a CHECK, and the state list gains `paused`, so the table is rebuilt (Schema.Rebuild keeps the rows
// and, with KeepIndexes, the two indexes); the migration owns its transaction because Rebuild turns foreign keys
// off around it. Three columns are added: the JSON lists of tracks and pauses, and the number of pointer events.
// None holds a device name or a pointer position.
func m20261124130000RecordingsTracks() schema.Migration {
	const (
		statesBefore = `state IN ('prepared','recording','finishing','ready','failed','cancelled')`
		statesAfter  = `state IN ('prepared','recording','paused','finishing','ready','failed','cancelled')`
	)

	recordings := func(states string, withTracks bool) func(t *schema.Table) {
		return func(t *schema.Table) {
			t.Text("id").Primary()
			t.Text("project_id")
			t.Text("state")
			t.Text("review").Default("")
			t.Text("source_kind")
			t.Text("source_label").Default("")
			t.Integer("width").Default(0)
			t.Integer("height").Default(0)
			t.Integer("fps")
			t.Text("encoder").Default("")
			t.Integer("started_ms").Default(0)
			t.Integer("ended_ms").Default(0)
			t.Integer("duration_ms").Default(0)
			t.Integer("frames").Default(0)
			t.Integer("dropped").Default(0)
			t.Integer("bytes").Default(0)
			t.Text("stop_reason").Default("")
			t.Text("error_code").Default("")
			t.Text("error_message").Default("")
			t.Text("asset_hash").Default("")
			t.Text("asset_id").Default("")
			t.Text("actor")
			t.Integer("created_ms")

			if withTracks {
				t.Text("tracks").Default("[]")
				t.Text("pauses").Default("[]")
				t.Integer("events_count").Default(0)
			}

			t.Check("recordings_state_check", states)
			t.Check("recordings_review_check", `review IN ('','pending','added','dismissed')`)
		}
	}

	return schema.Migration{
		ID:            "20261124130000_recordings_tracks",
		NoTransaction: true,
		Up: func(s *schema.Schema) error {
			return s.Rebuild("recordings", recordings(statesAfter, true), schema.KeepIndexes())
		},
		Down: func(s *schema.Schema) error {
			// A paused recording goes back to being a recording; the three columns are dropped with the rebuild.
			if err := s.Raw(`UPDATE recordings SET state = 'recording' WHERE state = 'paused'`); err != nil {
				return err
			}

			return s.Rebuild("recordings", recordings(statesBefore, false), schema.KeepIndexes(),
				schema.DropColumns("tracks", "pauses", "events_count"))
		},
	}
}
