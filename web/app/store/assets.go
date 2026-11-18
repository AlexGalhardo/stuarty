package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/AlexGalhardo/vukem/database"

	"github.com/AlexGalhardo/vukem-video/service"
)

type assetRepo struct{ ex database.Executor }

const assetColumns = `id, kind, size_bytes, original_name, ingest_state, ingest_progress, ingest_error,
 normalized_version, duration_ms, width, height, has_audio, frame_rate_num, frame_rate_den,
 frame_count, sample_count, created_ms, updated_ms, frame_delays, has_alpha`

func scanAsset(sc scanner) (service.AssetRow, error) {
	var (
		a                                             service.AssetRow
		kind, ingestErr, delays                       sql.NullString
		version, dur, w, h, frn, frd, frames, samples sql.NullInt64
		hasAudio, hasAlpha                            int
	)
	err := sc.Scan(
		&a.Hash,
		&kind,
		&a.SizeBytes,
		&a.OriginalName,
		&a.IngestState,
		&a.Progress,
		&ingestErr,
		&version,
		&dur,
		&w,
		&h,
		&hasAudio,
		&frn,
		&frd,
		&frames,
		&samples,
		&a.CreatedMs,
		&a.UpdatedMs,
		&delays,
		&hasAlpha,
	)
	if err != nil {
		return service.AssetRow{}, err
	}
	a.Kind = kind.String
	a.IngestError = rawJSON(ingestErr)
	a.Facts = service.AssetFacts{
		NormalizedVersion: ptrInt(version), DurationMs: ptrInt64(dur), Width: ptrInt(w), Height: ptrInt(h),
		HasAudio: hasAudio != 0, HasAlpha: hasAlpha != 0, FrameRateNum: ptrInt(frn), FrameRateDen: ptrInt(frd),
		FrameCount: ptrInt64(frames), SampleCount: ptrInt64(samples),
	}
	if delays.Valid {
		if err := json.Unmarshal([]byte(delays.String), &a.Facts.FrameDelays); err != nil {
			return service.AssetRow{}, err
		}
	}
	return a, nil
}

func (r assetRepo) Upsert(ctx context.Context, a service.AssetRow) (service.AssetRow, bool, error) {
	state := a.IngestState
	if state == "" {
		state = service.IngestPending
	}
	f := a.Facts
	n, err := rowsAffected(r.ex.ExecContext(ctx, `INSERT INTO assets
 (id, kind, size_bytes, original_name, ingest_state, ingest_progress, ingest_error, normalized_version,
  duration_ms, width, height, has_audio, frame_rate_num, frame_rate_den, frame_count, sample_count,
  created_ms, updated_ms, frame_delays, has_alpha)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (id) DO NOTHING`,
		a.Hash, nullString(a.Kind), a.SizeBytes, a.OriginalName, string(state), a.Progress, nullJSON(a.IngestError),
		f.NormalizedVersion, f.DurationMs, f.Width, f.Height, boolInt(f.HasAudio), f.FrameRateNum, f.FrameRateDen,
		f.FrameCount, f.SampleCount, a.CreatedMs, a.UpdatedMs, delaysJSON(f.FrameDelays), boolInt(f.HasAlpha)))
	if err != nil {
		return service.AssetRow{}, false, err
	}
	existing, err := r.Get(ctx, a.Hash)
	return existing, n == 1, err
}

func (r assetRepo) Get(ctx context.Context, hash string) (service.AssetRow, error) {
	a, err := scanAsset(r.ex.QueryRowContext(ctx, "SELECT "+assetColumns+" FROM assets WHERE id = ?", hash))
	return a, mapNoRows(err)
}

func notFoundIfNone(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrNotFound
	}
	return nil
}

func (r assetRepo) SetIngest(
	ctx context.Context,
	hash string,
	st service.IngestState,
	progress float64,
	errJSON json.RawMessage,
	nowMs int64,
) error {
	return notFoundIfNone(rowsAffected(r.ex.ExecContext(ctx,
		"UPDATE assets SET ingest_state = ?, ingest_progress = ?, ingest_error = ?, updated_ms = ? WHERE id = ?",
		string(st), progress, nullJSON(errJSON), nowMs, hash)))
}

func (r assetRepo) SetFacts(ctx context.Context, hash string, f service.AssetFacts, nowMs int64) error {
	return notFoundIfNone(rowsAffected(r.ex.ExecContext(
		ctx,
		`UPDATE assets SET
 kind = COALESCE(NULLIF(?, ''), kind), normalized_version = ?, duration_ms = ?, width = ?, height = ?, has_audio = ?,
 frame_rate_num = ?, frame_rate_den = ?, frame_count = ?, sample_count = ?, frame_delays = ?, has_alpha = ?, updated_ms = ?
 WHERE id = ?`,
		f.Kind,
		f.NormalizedVersion,
		f.DurationMs,
		f.Width,
		f.Height,
		boolInt(f.HasAudio),
		f.FrameRateNum,
		f.FrameRateDen,
		f.FrameCount,
		f.SampleCount,
		delaysJSON(f.FrameDelays),
		boolInt(f.HasAlpha),
		nowMs,
		hash,
	)))
}

// delaysJSON stores the frame delays of an animated image as a JSON array, NULL for a still.
func delaysJSON(d []int) any {
	if len(d) == 0 {
		return nil
	}

	b, err := json.Marshal(d)
	if err != nil {
		return nil
	}

	return string(b)
}

func (r assetRepo) PutArtifact(ctx context.Context, a service.ArtifactRow) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO artifacts
 (asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, meta, created_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (asset_hash, kind, profile, variant, ext) DO UPDATE SET
   rel_path = excluded.rel_path, size_bytes = excluded.size_bytes, sha256 = excluded.sha256,
   meta = excluded.meta, created_ms = excluded.created_ms`,
		a.AssetHash,
		a.Kind,
		a.Profile,
		a.Variant,
		a.Ext,
		a.RelPath,
		a.SizeBytes,
		a.SHA256,
		nullJSON(a.Meta),
		a.CreatedMs,
	)
	return err
}

func (r assetRepo) Artifacts(ctx context.Context, hash string) ([]service.ArtifactRow, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT asset_hash, kind, profile, variant, ext, rel_path, size_bytes, sha256, meta, created_ms
 FROM artifacts WHERE asset_hash = ? ORDER BY kind, profile, variant, ext`,
		hash,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.ArtifactRow
	for rows.Next() {
		var (
			a    service.ArtifactRow
			meta sql.NullString
		)
		if err := rows.Scan(&a.AssetHash, &a.Kind, &a.Profile, &a.Variant, &a.Ext, &a.RelPath,
			&a.SizeBytes, &a.SHA256, &meta, &a.CreatedMs); err != nil {
			return nil, err
		}
		a.Meta = rawJSON(meta)
		out = append(out, a)
	}
	return out, rows.Err()
}

const projectAssetColumns = `project_id, asset_id, asset_hash, state, name, snapshot, imported_by, imported_ms, trashed_by, trashed_ms`

func scanProjectAsset(sc scanner) (service.ProjectAssetRow, error) {
	var (
		p         service.ProjectAssetRow
		snapshot  sql.NullString
		trashedBy sql.NullString
		trashedMs sql.NullInt64
	)
	err := sc.Scan(&p.ProjectID, &p.AssetID, &p.AssetHash, &p.State, &p.Name, &snapshot,
		&p.ImportedBy, &p.ImportedMs, &trashedBy, &trashedMs)
	if err != nil {
		return service.ProjectAssetRow{}, err
	}
	p.Snapshot, p.TrashedBy, p.TrashedMs = rawJSON(snapshot), ptrString(trashedBy), ptrInt64(trashedMs)
	return p, nil
}

func (r assetRepo) PutProjectAsset(ctx context.Context, p service.ProjectAssetRow) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO project_assets (`+projectAssetColumns+`)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (project_id, asset_id) DO UPDATE SET
   asset_hash = excluded.asset_hash, state = excluded.state, name = excluded.name,
   snapshot = excluded.snapshot, imported_by = excluded.imported_by, imported_ms = excluded.imported_ms,
   trashed_by = excluded.trashed_by, trashed_ms = excluded.trashed_ms`,
		p.ProjectID, p.AssetID, p.AssetHash, string(p.State), p.Name, nullJSON(p.Snapshot),
		p.ImportedBy, p.ImportedMs, p.TrashedBy, p.TrashedMs)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}
	return err
}

func (r assetRepo) GetProjectAsset(ctx context.Context, projectID, assetID string) (service.ProjectAssetRow, error) {
	p, err := scanProjectAsset(r.ex.QueryRowContext(ctx,
		"SELECT "+projectAssetColumns+" FROM project_assets WHERE project_id = ? AND asset_id = ?", projectID, assetID))
	return p, mapNoRows(err)
}

func (r assetRepo) FindProjectAssetByHash(
	ctx context.Context,
	projectID, hash string,
) (service.ProjectAssetRow, error) {
	p, err := scanProjectAsset(r.ex.QueryRowContext(ctx,
		"SELECT "+projectAssetColumns+" FROM project_assets WHERE project_id = ? AND asset_hash = ?", projectID, hash))
	return p, mapNoRows(err)
}

func (r assetRepo) queryProjectAssets(
	ctx context.Context,
	query string,
	args ...any,
) ([]service.ProjectAssetRow, error) {
	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.ProjectAssetRow
	for rows.Next() {
		p, err := scanProjectAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r assetRepo) ListProjectAssets(
	ctx context.Context,
	projectID string,
	states ...service.ProjectAssetState,
) ([]service.ProjectAssetRow, error) {
	query := "SELECT " + projectAssetColumns + " FROM project_assets WHERE project_id = ?"
	args := []any{projectID}
	if len(states) > 0 {
		query += " AND state IN (?" + strings.Repeat(", ?", len(states)-1) + ")"
		for _, s := range states {
			args = append(args, string(s))
		}
	}
	return r.queryProjectAssets(ctx, query+" ORDER BY imported_ms, asset_id", args...)
}

// SetProjectAssetState changes the state. A snapshot is kept when nil is
// passed; the trash columns are set for trashed and cleared otherwise.
func (r assetRepo) SetProjectAssetState(
	ctx context.Context,
	projectID, assetID string,
	st service.ProjectAssetState,
	by string,
	atMs int64,
	snapshot json.RawMessage,
) error {
	var trashedBy, trashedMs any
	if st == service.ProjectAssetTrashed {
		trashedBy, trashedMs = by, atMs
	}
	return notFoundIfNone(rowsAffected(r.ex.ExecContext(ctx, `UPDATE project_assets SET
 state = ?, trashed_by = ?, trashed_ms = ?, snapshot = COALESCE(?, snapshot)
 WHERE project_id = ? AND asset_id = ?`,
		string(st), trashedBy, trashedMs, nullJSON(snapshot), projectID, assetID)))
}

func (r assetRepo) ProjectsReferencing(ctx context.Context, hash string) ([]string, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		"SELECT DISTINCT project_id FROM project_assets WHERE asset_hash = ? ORDER BY project_id",
		hash,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r assetRepo) PendingForHash(ctx context.Context, hash string) ([]service.ProjectAssetRow, error) {
	return r.queryProjectAssets(
		ctx,
		"SELECT "+projectAssetColumns+" FROM project_assets WHERE asset_hash = ? AND state = 'pending' ORDER BY project_id, asset_id",
		hash,
	)
}

func (r assetRepo) DeleteTrashedProjectAssets(ctx context.Context, projectID string) (int64, error) {
	return rowsAffected(
		r.ex.ExecContext(ctx, "DELETE FROM project_assets WHERE project_id = ? AND state = 'trashed'", projectID),
	)
}

func (r assetRepo) Unreferenced(ctx context.Context) ([]service.AssetRow, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+assetColumns+` FROM assets
 WHERE NOT EXISTS (SELECT 1 FROM project_assets WHERE project_assets.asset_hash = assets.id) ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.AssetRow
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r assetRepo) Delete(ctx context.Context, hash string) error {
	return notFoundIfNone(rowsAffected(r.ex.ExecContext(ctx, "DELETE FROM assets WHERE id = ?", hash)))
}
