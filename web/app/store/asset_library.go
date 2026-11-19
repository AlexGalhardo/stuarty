package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service/assets"

	"github.com/AlexGalhardo/vukem/database"
)

// assetLibraryRepo is assets.Store over the tables asset_info, asset_tags and asset_search (SDD-005 section 5.5.1).
type assetLibraryRepo struct{ ex database.Executor }

// maxSearchHits bounds the hashes a full-text query returns.
const maxSearchHits = 5000

// manyChunk is how many hashes one IN list holds, below the variable limit of SQLite.
const manyChunk = 400

const infoColumns = `asset_hash, origin_kind, source, provider, model, prompt, cost_micros, rights_note, description,
 reusable, created_by, created_ms, updated_ms, engine, engine_version, render_key`

func scanInfo(sc interface{ Scan(...any) error }) (assets.Row, error) {
	var (
		r    assets.Row
		kind string
		cost sql.NullInt64
		re   int
	)

	if err := sc.Scan(
		&r.Owner, &r.Hash, &kind, &r.Origin.Source, &r.Origin.Provider, &r.Origin.Model, &r.Origin.Prompt, &cost,
		&r.Origin.RightsNote, &r.Description, &re, &r.CreatedBy, &r.CreatedMs, &r.UpdatedMs,
		&r.Origin.Engine, &r.Origin.EngineVersion, &r.Origin.RenderKey,
	); err != nil {
		return r, err
	}

	r.Origin.Kind = asset.OriginKind(kind)
	r.Origin.CostMicros = ptrInt64(cost)
	r.Reusable = re == 1

	return r, nil
}

func (r assetLibraryRepo) Get(ctx context.Context, owner, hash string) (assets.Row, error) {
	row, err := scanInfo(
		r.ex.QueryRowContext(ctx, `SELECT `+infoColumns+` FROM asset_info WHERE owner_user_id = ? AND asset_hash = ?`,
			owner, hash),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return row, assets.ErrNotFound
	}

	if err != nil {
		return row, err
	}

	tags, err := r.tags(ctx, owner, []string{hash})
	row.Tags = tags[hash]

	return row, err
}

func (r assetLibraryRepo) tags(ctx context.Context, owner string, hashes []string) (map[string][]string, error) {
	out := map[string][]string{}

	for start := 0; start < len(hashes); start += manyChunk {
		part := hashes[start:min(start+manyChunk, len(hashes))]

		err := r.query(
			ctx,
			`SELECT asset_hash, tag FROM asset_tags WHERE owner_user_id = ? AND asset_hash IN (`+placeholders(
				len(part),
			)+`) ORDER BY asset_hash, tag`,
			append([]any{owner}, anys(part)...),
			func(sc *sql.Rows) error {
				var h, t string
				if err := sc.Scan(&h, &t); err != nil {
					return err
				}

				out[h] = append(out[h], t)

				return nil
			},
		)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// query runs a read and hands every row to each.
func (r assetLibraryRepo) query(ctx context.Context, q string, args []any, each func(*sql.Rows) error) error {
	rows, err := r.ex.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := each(rows); err != nil {
			return err
		}
	}

	return rows.Err()
}

func (r assetLibraryRepo) GetMany(ctx context.Context, owner string, hashes []string) (map[string]assets.Row, error) {
	out := map[string]assets.Row{}

	for start := 0; start < len(hashes); start += manyChunk {
		part := hashes[start:min(start+manyChunk, len(hashes))]

		err := r.query(
			ctx,
			`SELECT `+infoColumns+` FROM asset_info WHERE owner_user_id = ? AND asset_hash IN (`+placeholders(
				len(part),
			)+`)`,
			append([]any{owner}, anys(part)...),
			func(sc *sql.Rows) error {
				row, err := scanInfo(sc)
				if err != nil {
					return err
				}

				out[row.Hash] = row

				return nil
			},
		)
		if err != nil {
			return nil, err
		}
	}

	tags, err := r.tags(ctx, owner, hashes)
	if err != nil {
		return nil, err
	}

	for h := range out {
		row := out[h]
		row.Tags = tags[h]
		out[h] = row
	}

	return out, nil
}

func (r assetLibraryRepo) Put(ctx context.Context, row assets.Row) error {
	o := row.Origin
	if o.Kind == "" {
		o.Kind = asset.Imported
	}

	if _, err := r.ex.ExecContext(ctx,
		`INSERT INTO asset_info (`+infoColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (asset_hash) DO UPDATE SET origin_kind = excluded.origin_kind, source = excluded.source,
 provider = excluded.provider, model = excluded.model, prompt = excluded.prompt, cost_micros = excluded.cost_micros,
 rights_note = excluded.rights_note, description = excluded.description, reusable = excluded.reusable,
 updated_ms = excluded.updated_ms, engine = excluded.engine, engine_version = excluded.engine_version,
 render_key = excluded.render_key`,
		row.Hash, string(o.Kind), o.Source, o.Provider, o.Model, o.Prompt, o.CostMicros, o.RightsNote,
		row.Description, boolInt(row.Reusable), row.CreatedBy, row.CreatedMs, row.UpdatedMs,
		o.Engine, o.EngineVersion, o.RenderKey,
	); err != nil {
		return err
	}

	if _, err := r.ex.ExecContext(ctx, `DELETE FROM asset_tags WHERE owner_user_id = ? AND asset_hash = ?`,
		row.Owner, row.Hash); err != nil {
		return err
	}

	for _, t := range row.Tags {
		if _, err := r.ex.ExecContext(
			ctx,
			`INSERT INTO asset_tags (owner_user_id, asset_hash, tag) VALUES (?, ?, ?)`,
			row.Owner,
			row.Hash,
			t,
		); err != nil {
			return err
		}
	}

	return r.Reindex(ctx, row.Owner, row.Hash)
}

func (r assetLibraryRepo) Reindex(ctx context.Context, owner, hash string) error {
	if _, err := r.ex.ExecContext(
		ctx,
		`DELETE FROM asset_search WHERE owner_user_id = ? AND asset_hash = ?`,
		owner,
		hash,
	); err != nil {
		return err
	}

	_, err := r.ex.ExecContext(ctx,
		`INSERT INTO asset_search (owner_user_id, asset_hash, name, tags, description, origin)
 SELECT ?, a.id,
        COALESCE((SELECT group_concat(DISTINCT p.name) FROM project_assets p JOIN projects pr ON pr.id = p.project_id
                  WHERE p.asset_hash = a.id AND COALESCE(pr.owner_user_id, '') = ?), a.original_name),
        COALESCE((SELECT group_concat(t.tag, ' ') FROM asset_tags t
                  WHERE t.asset_hash = a.id AND t.owner_user_id = ?), ''),
        COALESCE(i.description, ''),
        TRIM(COALESCE(i.source, '') || ' ' || COALESCE(i.provider, '') || ' ' || COALESCE(i.model, '') || ' ' ||
             COALESCE(i.prompt, '') || ' ' || COALESCE(i.engine, '') || ' ' || COALESCE(i.engine_version, ''))
 FROM assets a LEFT JOIN asset_info i ON i.asset_hash = a.id
 WHERE a.id = ?`, hash)

	return err
}

func (r assetLibraryRepo) Search(ctx context.Context, owner, match string) (map[string]bool, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT asset_hash FROM asset_search WHERE owner_user_id = ? AND asset_search MATCH ? LIMIT ?`,
		owner, match, maxSearchHits)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}

	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}

		out[h] = true
	}

	return out, rows.Err()
}

func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}

	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anys(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}

	return out
}
