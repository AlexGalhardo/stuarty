package migrations

import "github.com/AlexGalhardo/vukem/schema"

const (
	assetKindsBefore = `kind IS NULL OR kind IN ('video','audio','image')`
	assetKindsAfter  = `kind IS NULL OR kind IN ('video','audio','image','lut')`
)

// A colour lookup table (a .cube file) is an asset of kind lut (SDD-001 section 3.2, SDD-005 section
// 5.5, TASK-0254). The kind check is part of the table, so the table is rebuilt; assets is the parent
// of artifacts (which cascades) and of project_assets (which restricts), which is why the migration
// owns its transaction: Rebuild turns foreign keys off around it and checks them before it commits.
func m20261116130000AssetsLutKind() schema.Migration {
	return schema.Migration{
		ID:            "20261116130000_assets_lut_kind",
		NoTransaction: true,
		Up:            func(s *schema.Schema) error { return rebuildAssets(s, assetKindsAfter) },
		Down:          func(s *schema.Schema) error { return rebuildAssets(s, assetKindsBefore) },
	}
}

func rebuildAssets(s *schema.Schema, kindCheck string) error {
	return s.Rebuild("assets", func(t *schema.Table) {
		t.Text("id").Primary()
		t.Text("kind").Nullable()
		t.Integer("size_bytes")
		t.Text("original_name")
		t.Text("ingest_state").Default("pending")
		t.Float("ingest_progress").Default(0)
		t.Text("ingest_error").Nullable()
		t.Integer("normalized_version").Nullable()
		t.Integer("duration_ms").Nullable()
		t.Integer("width").Nullable()
		t.Integer("height").Nullable()
		t.Integer("has_audio").Default(0)
		t.Integer("frame_rate_num").Nullable()
		t.Integer("frame_rate_den").Nullable()
		t.Integer("frame_count").Nullable()
		t.Integer("sample_count").Nullable()
		t.Integer("created_ms")
		t.Integer("updated_ms")
		t.Text("frame_delays").Nullable()
		t.Integer("has_alpha").Default(0)
		t.Check("assets_kind_check", kindCheck)
		t.Check("assets_ingest_state_check", `ingest_state IN ('pending','ingesting','ready','failed')`)
		t.Check("assets_ingest_progress_check", `ingest_progress >= 0 AND ingest_progress <= 1`)
	})
}
