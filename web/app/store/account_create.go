package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/account"

	"github.com/AlexGalhardo/vukem/database"
)

// cell is a sealed column and the columns that make the key of its row (SDD-005 section 5.17.2). The repositories
// bind a value to its cell the same way, through the row key they pass to the sealer.
type cell struct {
	table, column string
	keys          []string
	// filter limits the cell to some rows of its table: the jobs whose kind keeps words.
	filter string
}

var sealedCells = []cell{
	{table: "project_items", column: "body", keys: []string{"id"}},
	{table: "text_revisions", column: "body", keys: []string{"item_id", "revision"}},
	{table: "chat_events", column: "body_json", keys: []string{"session_id", "seq"}},
	{table: "chat_sessions", column: "title", keys: []string{"id"}},
	{table: "asset_info", column: "prompt", keys: []string{"owner_user_id", "asset_hash"}},
	{table: "publications", column: "text", keys: []string{"id"}},
	{table: "publication_targets", column: "fields", keys: []string{"id"}},
	{table: "publication_targets", column: "snapshot", keys: []string{"id"}},
	{table: "projects", column: "description", keys: []string{"id"}},
	{table: "projects", column: "goal_specific", keys: []string{"id"}},
	{table: "projects", column: "goal_measurable", keys: []string{"id"}},
	{table: "projects", column: "goal_achievable", keys: []string{"id"}},
	{table: "projects", column: "goal_relevant", keys: []string{"id"}},
	{table: "projects", column: "goal_time_bound", keys: []string{"id"}},
	{table: "webhooks", column: "url", keys: []string{"id"}},
	{table: "published_posts", column: "markdown", keys: []string{"slug"}},
	{table: "published_posts", column: "html", keys: []string{"slug"}},
	{table: "approval_requests", column: "summary", keys: []string{"id"}},
	{table: "jobs", column: "input", keys: []string{"id"}, filter: jobClearFilter()},
	{table: "jobs", column: "result", keys: []string{"id"}, filter: jobClearFilter()},
}

type cellRow struct {
	where []string
	key   string
	plain string
}

func (c cell) read(ctx context.Context, ex database.Executor) ([]cellRow, error) {
	where := c.column + " IS NOT NULL"
	if c.filter != "" {
		where += " AND " + c.filter
	}

	rows, err := ex.QueryContext(ctx, "SELECT "+strings.Join(c.keys, ", ")+", "+c.column+" FROM "+c.table+
		" WHERE "+where)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []cellRow

	for rows.Next() {
		vals := make([]sql.NullString, len(c.keys)+1)
		dest := make([]any, len(vals))

		for i := range vals {
			dest[i] = &vals[i]
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}

		row := cellRow{plain: vals[len(c.keys)].String}
		for _, v := range vals[:len(c.keys)] {
			row.where = append(row.where, v.String)
		}

		row.key = strings.Join(row.where, "/")
		out = append(out, row)
	}

	return out, rows.Err()
}

func (c cell) update(ctx context.Context, ex database.Executor, row cellRow, value string) error {
	where := make([]string, len(c.keys))
	args := []any{value}

	for i, k := range c.keys {
		where[i] = k + " = ?"
		args = append(args, row.where[i])
	}

	n, err := rowsAffected(ex.ExecContext(ctx,
		"UPDATE "+c.table+" SET "+c.column+" = ? WHERE "+strings.Join(where, " AND "), args...))
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("store: sealing %s.%s touched %d rows for one key", c.table, c.column, n)
	}

	return nil
}

// CreateAccount makes the account in a workspace that may hold data (SDD-005 section 5.17.3): in one
// transaction it seals every sealed column, empties the body of the library index, inserts the record and reads
// every sealed value back against what it was. Any failure rolls the transaction back and the workspace is as
// it was. hook, when set, is called after each stage and may fail it (a test uses it to stop in the middle).
func (s *Store) CreateAccount(
	ctx context.Context, rec account.Record, sl account.Sealer, hook func(stage string) error,
) error {
	if hook == nil {
		hook = func(string) error { return nil }
	}

	return s.db.Tx(ctx, func(dtx *database.Tx) error {
		sums, err := sealCells(ctx, dtx, sl)
		if err != nil {
			return err
		}

		if err := hook("sealed"); err != nil {
			return err
		}

		if err := clearIndexes(ctx, dtx); err != nil {
			return err
		}

		if err := (accountRepo{ex: dtx}).Create(ctx, rec); err != nil {
			return err
		}

		if err := hook("inserted"); err != nil {
			return err
		}

		if err := verifySealed(ctx, dtx, sl, sums); err != nil {
			return err
		}

		return installSealGuards(ctx, dtx)
	})
}

// guardMessage is what a guard raises; sealedExec turns it into account.ErrLocked.
const guardMessage = "stuarty: a sealed column was written in clear text"

// installSealGuards makes the database refuse a clear value in a sealed column from the commit of the account on.
// A write that sealed its value (in clear text, the workspace had no account) just before the creation began can
// reach the database after the creation has sealed everything; the sealing cannot see it, and only the database,
// which serializes the two, can tell which came first. The guards are in the transaction of the creation, so
// they exist exactly when the account does.
func installSealGuards(ctx context.Context, ex database.Executor) error {
	for _, c := range sealedCells {
		when := "NEW." + c.column + " IS NOT NULL AND NEW." + c.column + " <> '' AND substr(NEW." + c.column + ", 1, " +
			strconv.Itoa(len(account.Prefix)) + ") <> '" + account.Prefix + "'"
		if c.filter != "" {
			when += " AND NEW." + c.filter
		}

		for _, event := range []string{"INSERT", "UPDATE OF " + c.column} {
			name := "sealed_guard_" + c.table + "_" + c.column + "_" + strings.ToLower(strings.Fields(event)[0])

			q := "CREATE TRIGGER IF NOT EXISTS " + name + " BEFORE " + event + " ON " + c.table +
				" WHEN " + when + " BEGIN SELECT RAISE(ABORT, '" + guardMessage + "'); END"
			if _, err := ex.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("store: guard %s.%s: %w", c.table, c.column, err)
			}
		}
	}

	return nil
}

// guardSealedColumns installs the guards again at every open of a database that has an account, after the schema
// migrations: a migration that rebuilds a sealed table drops its triggers, and an account made before the guards
// existed has none. Each statement is CREATE TRIGGER IF NOT EXISTS, so an open that finds them all changes nothing.
func (s *Store) guardSealedColumns(ctx context.Context) error {
	var tables int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'accounts'`).Scan(&tables); err != nil {
		return fmt.Errorf("store: guards: %w", err)
	}

	if tables == 0 {
		return nil
	}

	var accounts int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&accounts); err != nil {
		return fmt.Errorf("store: guards: %w", err)
	}

	if accounts == 0 {
		return nil
	}

	return installSealGuards(ctx, s.db)
}

// sealCells seals every value of every sealed column and returns the digest of each, for the read back.
func sealCells(ctx context.Context, ex database.Executor, sl account.Sealer) (map[string][sha256.Size]byte, error) {
	sums := map[string][sha256.Size]byte{}

	for _, c := range sealedCells {
		rows, err := c.read(ctx, ex)
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			sealed, err := sl.Seal(c.table, c.column, row.key, row.plain)
			if err != nil {
				return nil, err
			}

			sums[c.table+"."+c.column+"/"+row.key] = sha256.Sum256([]byte(row.plain))

			if err := c.update(ctx, ex, row, sealed); err != nil {
				return nil, err
			}
		}
	}

	return sums, nil
}

// clearIndexes takes the words of the person out of the two full-text indexes. An update of an FTS5 table leaves
// the old terms in its segments until a merge, so each is rebuilt.
func clearIndexes(ctx context.Context, ex database.Executor) error {
	for _, q := range []string{
		`UPDATE library_fts SET body = ''`,
		`INSERT INTO library_fts (library_fts) VALUES ('rebuild')`,
		// The index of the files repeats the prompt in its origin column: it is written again without it.
		`UPDATE asset_search SET origin = COALESCE((
  SELECT TRIM(COALESCE(i.source, '') || ' ' || COALESCE(i.provider, '') || ' ' || COALESCE(i.model, '') || ' ' ||
              COALESCE(i.engine, '') || ' ' || COALESCE(i.engine_version, ''))
  FROM asset_info i
  WHERE i.asset_hash = asset_search.asset_hash AND i.owner_user_id = asset_search.owner_user_id), '')`,
		`INSERT INTO asset_search (asset_search) VALUES ('rebuild')`,
	} {
		if _, err := ex.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	return nil
}

// CompactSealed takes out of the file what the sealing replaced (SDD-005 section 5.17.3). The old pages of a
// sealed column stay in the free space of db.sqlite, and a VACUUM in WAL mode writes the new file to the log, so
// only the checkpoint after it leaves db.sqlite without them and empties the log. The checkpoint is also what
// makes the commit of the account durable; a checkpoint that could not finish is an error.
func (s *Store) CompactSealed(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("store: compact the database: %w", err)
	}

	var busy, logged, moved int

	// On the write connection: the read pool is read-only, and a checkpoint from it is an I/O error.
	err := s.db.QueryRowWriteContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logged, &moved)
	if err != nil {
		return fmt.Errorf("store: checkpoint the database: %w", err)
	}

	if busy != 0 {
		return errors.New("store: the write-ahead log is in use and could not be emptied")
	}

	return nil
}

// verifySealed reads every sealed column back and opens it: each value must open and be what it was before.
func verifySealed(
	ctx context.Context,
	ex database.Executor,
	sl account.Sealer,
	sums map[string][sha256.Size]byte,
) error {
	seen := 0

	for _, c := range sealedCells {
		rows, err := c.read(ctx, ex)
		if err != nil {
			return err
		}

		for _, row := range rows {
			plain, err := sl.Open(c.table, c.column, row.key, row.plain)
			if err != nil {
				return fmt.Errorf("store: %s.%s of %s did not read back: %w", c.table, c.column, row.key, err)
			}

			want, ok := sums[c.table+"."+c.column+"/"+row.key]
			if !ok || sha256.Sum256([]byte(plain)) != want {
				return errors.New(
					"store: a sealed value is not what it was before sealing: " + c.table + "." + c.column,
				)
			}

			seen++
		}
	}

	if seen != len(sums) {
		return fmt.Errorf("store: %d sealed values were read back, %d were sealed", seen, len(sums))
	}

	return nil
}
