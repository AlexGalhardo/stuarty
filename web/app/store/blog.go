package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"

	"github.com/AlexGalhardo/vukem/database"
)

// blogRepo is destinations.Store over the tables of SDD-005 section 5.15.1.
type blogRepo struct{ ex database.Executor }

func (r blogRepo) Remote(ctx context.Context, connectionID, postItemID string) (destinations.Remote, error) {
	var (
		out destinations.Remote
		ms  int64
	)

	err := r.ex.QueryRowContext(ctx,
		`SELECT connection_id, post_item_id, remote_id, url, updated_ms FROM blog_remote_posts
 WHERE connection_id = ? AND post_item_id = ?`, connectionID, postItemID).
		Scan(&out.ConnectionID, &out.PostItemID, &out.RemoteID, &out.URL, &ms)
	if errors.Is(err, sql.ErrNoRows) {
		return out, destinations.ErrNotFound
	}

	out.UpdatedAt = time.UnixMilli(ms).UTC()

	return out, err
}

func (r blogRepo) PutRemote(ctx context.Context, rm destinations.Remote) error {
	_, err := r.ex.ExecContext(ctx,
		`INSERT INTO blog_remote_posts (connection_id, post_item_id, remote_id, url, updated_ms) VALUES (?, ?, ?, ?, ?)
 ON CONFLICT (connection_id, post_item_id) DO UPDATE SET remote_id = excluded.remote_id, url = excluded.url,
 updated_ms = excluded.updated_ms`,
		rm.ConnectionID, rm.PostItemID, rm.RemoteID, rm.URL, rm.UpdatedAt.UnixMilli())

	return err
}

func (r blogRepo) DeleteRemote(ctx context.Context, connectionID, postItemID string) error {
	_, err := r.ex.ExecContext(ctx, `DELETE FROM blog_remote_posts WHERE connection_id = ? AND post_item_id = ?`,
		connectionID, postItemID)

	return err
}

func (r blogRepo) PutPublished(ctx context.Context, p destinations.PublishedPost) error {
	var holder string

	err := r.ex.QueryRowContext(ctx, `SELECT post_item_id || '/' || connection_id FROM published_posts WHERE slug = ?`,
		p.Slug).Scan(&holder)

	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	case holder != p.PostItemID+"/"+p.ConnectionID:
		return destinations.ErrSlugTaken
	}

	tags, err := json.Marshal(append([]string{}, p.Meta.Tags...))
	if err != nil {
		return err
	}

	images, err := json.Marshal(append([]destinations.Image{}, p.Images...))
	if err != nil {
		return err
	}

	// A post whose slug changed since an earlier send keeps one row: the old slug is replaced, the time of the first
	// publication stays.
	first := p.PublishedAt.UnixMilli()

	var prevFirst sql.NullInt64
	if qerr := r.ex.QueryRowContext(ctx,
		`SELECT published_ms FROM published_posts WHERE post_item_id = ? AND connection_id = ?`,
		p.PostItemID, p.ConnectionID).Scan(&prevFirst); qerr != nil && !errors.Is(qerr, sql.ErrNoRows) {
		return qerr
	}

	if prevFirst.Valid {
		first = prevFirst.Int64
	}

	if _, err := r.ex.ExecContext(
		ctx,
		`DELETE FROM published_posts WHERE post_item_id = ? AND connection_id = ?`,
		p.PostItemID,
		p.ConnectionID,
	); err != nil {
		return err
	}

	_, err = r.ex.ExecContext(
		ctx,
		`INSERT INTO published_posts (slug, post_item_id, project_id, connection_id, owner_user_id, title, summary, tags,
 language, canonical, status, markdown, html, images, published_ms, updated_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Slug,
		p.PostItemID,
		p.ProjectID,
		p.ConnectionID,
		nullString(p.OwnerUserID),
		p.Meta.Title,
		p.Meta.Summary,
		string(tags),
		p.Meta.Language,
		p.Meta.Canonical,
		p.Status,
		p.Markdown,
		p.HTML,
		string(images),
		first,
		p.UpdatedAt.UnixMilli(),
	)

	return err
}

const publishedColumns = `slug, post_item_id, project_id, connection_id, owner_user_id, title, summary, tags, language,
 canonical, status, markdown, html, images, published_ms, updated_ms`

func scanPublished(sc scanner) (destinations.PublishedPost, error) {
	var (
		p                       destinations.PublishedPost
		owner                   sql.NullString
		tags, images            string
		publishedMs, updatedMs  int64
		title, summary, lang, c string
	)

	if err := sc.Scan(&p.Slug, &p.PostItemID, &p.ProjectID, &p.ConnectionID, &owner, &title, &summary, &tags, &lang,
		&c, &p.Status, &p.Markdown, &p.HTML, &images, &publishedMs, &updatedMs); err != nil {
		return p, err
	}

	p.OwnerUserID = owner.String
	p.Meta = writing.PostMeta{
		Title: title, Slug: p.Slug, Summary: summary, Language: lang, Canonical: c, Tags: []string{},
	}

	if err := json.Unmarshal([]byte(tags), &p.Meta.Tags); err != nil {
		return p, err
	}

	if err := json.Unmarshal([]byte(images), &p.Images); err != nil {
		return p, err
	}

	for i := range p.Images {
		if p.Images[i].Role == "cover" {
			p.Meta.Cover = p.Images[i].AssetID
		}
	}

	p.PublishedAt, p.UpdatedAt = time.UnixMilli(publishedMs).UTC(), time.UnixMilli(updatedMs).UTC()

	return p, nil
}

func (r blogRepo) Published(ctx context.Context, slug string) (destinations.PublishedPost, error) {
	p, err := scanPublished(
		r.ex.QueryRowContext(ctx, `SELECT `+publishedColumns+` FROM published_posts WHERE slug = ?`, slug),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return p, destinations.ErrNotFound
	}

	return p, err
}

func (r blogRepo) ListPublished(ctx context.Context, f destinations.ListFilter) ([]destinations.PublishedPost, error) {
	where := []string{`status = 'published'`}
	args := []any{}

	if f.OwnerUserID != "" {
		where, args = append(where, `owner_user_id = ?`), append(args, f.OwnerUserID)
	}

	if f.ProjectID != "" {
		where, args = append(where, `project_id = ?`), append(args, f.ProjectID)
	}

	if f.AfterMs > 0 {
		where = append(where, `(published_ms < ? OR (published_ms = ? AND slug > ?))`)
		args = append(args, f.AfterMs, f.AfterMs, f.AfterSlug)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.ex.QueryContext(ctx, `SELECT `+publishedColumns+` FROM published_posts WHERE `+
		strings.Join(where, ` AND `)+` ORDER BY published_ms DESC, slug ASC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []destinations.PublishedPost

	for rows.Next() {
		p, serr := scanPublished(rows)
		if serr != nil {
			return nil, serr
		}

		out = append(out, p)
	}

	return out, rows.Err()
}
