package store

import (
	"context"
	"encoding/json"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/service/library"

	"github.com/AlexGalhardo/vukem/database"
)

// libraryRepo is library.Index over the tables library_entries and library_fts (SDD-018 section 9).
// It holds an index and nothing else: every row can be rebuilt from the files of the workspace.
type libraryRepo struct{ ex database.Executor }

const libraryColumns = `key, kind, name, description, scope, origin, reviewed, kept, actor, tags, state, error, path, sig,
  lint_errors, lint_warnings, has_scripts, created_ms, updated_ms`

func (r libraryRepo) All(ctx context.Context) ([]library.Entry, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+libraryColumns+` FROM library_entries ORDER BY key`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []library.Entry

	for rows.Next() {
		var (
			e                       library.Entry
			kind, tags              string
			reviewed, kept, scripts int
		)

		if err := rows.Scan(
			&e.Key, &kind, &e.Name, &e.Description, &e.Scope, &e.Origin, &reviewed, &kept, &e.Actor, &tags, &e.State,
			&e.Error, &e.Path, &e.Sig, &e.LintErrors, &e.LintWarnings, &scripts, &e.CreatedMs, &e.UpdatedMs,
		); err != nil {
			return nil, err
		}

		e.Kind = core.Kind(kind)
		e.Reviewed, e.Kept, e.HasScripts = reviewed != 0, kept != 0, scripts != 0

		if err := json.Unmarshal([]byte(tags), &e.Tags); err != nil {
			return nil, err
		}

		out = append(out, e)
	}

	return out, rows.Err()
}

func (r libraryRepo) Put(ctx context.Context, e library.Entry, body string) error {
	tags, err := json.Marshal(append([]string{}, e.Tags...))
	if err != nil {
		return err
	}

	if _, err := r.ex.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO library_entries (`+libraryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Key,
		string(e.Kind),
		e.Name,
		e.Description,
		e.Scope,
		e.Origin,
		boolInt(e.Reviewed),
		boolInt(e.Kept),
		e.Actor,
		string(tags),
		e.State,
		e.Error,
		e.Path,
		e.Sig,
		e.LintErrors,
		e.LintWarnings,
		boolInt(e.HasScripts),
		e.CreatedMs,
		e.UpdatedMs,
	); err != nil {
		return err
	}

	if _, err := r.ex.ExecContext(ctx, `DELETE FROM library_fts WHERE key = ?`, e.Key); err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO library_fts (key, name, description, tags, body) VALUES (?, ?, ?, ?, ?)`,
		e.Key, e.Name, e.Description, strings.Join(e.Tags, " "), body,
	)

	return err
}

func (r libraryRepo) Delete(ctx context.Context, key string) error {
	if _, err := r.ex.ExecContext(ctx, `DELETE FROM library_entries WHERE key = ?`, key); err != nil {
		return err
	}

	_, err := r.ex.ExecContext(ctx, `DELETE FROM library_fts WHERE key = ?`, key)

	return err
}

func (r libraryRepo) Clear(ctx context.Context) error {
	if _, err := r.ex.ExecContext(ctx, `DELETE FROM library_entries`); err != nil {
		return err
	}

	_, err := r.ex.ExecContext(ctx, `DELETE FROM library_fts`)

	return err
}

// Search matches every term as a prefix, best first: a hit in the name weighs most, then the
// description, the tags and the body. The terms come from core/library.Terms, which keeps letters
// and digits only, so a term quoted here can never be an expression of the full-text engine.
func (r libraryRepo) Search(ctx context.Context, terms []string, limit int) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, ``) + `"*`
	}

	rows, err := r.ex.QueryContext(ctx,
		`SELECT key FROM library_fts WHERE library_fts MATCH ? ORDER BY bm25(library_fts, 0, 10, 5, 3, 1), key LIMIT ?`,
		strings.Join(quoted, " AND "), limit,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var keys []string

	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}

		keys = append(keys, k)
	}

	return keys, rows.Err()
}
