package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"slices"
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
	{table: "recordings", column: "source_label", keys: []string{"id"}},
	{table: "published_posts", column: "markdown", keys: []string{"slug"}},
	{table: "published_posts", column: "html", keys: []string{"slug"}},
	{table: "approval_requests", column: "summary", keys: []string{"id"}},
	{table: "jobs", column: "input", keys: []string{"id"}, filter: jobClearFilter()},
	{table: "jobs", column: "result", keys: []string{"id"}, filter: jobClearFilter()},
}

// sealedCellsV2 is the second stage (ADR-0053, SDD-005 section 5.17.2): the documents, the edit log that repeats them,
// the backups, the idempotency results and the asset notes. An account made by this version seals them from its
// first row; one made before seals them at its first unlock (sealStage2), and until then they are read as clear text
// and have no guard.
var sealedCellsV2 = []cell{
	{table: "timelines", column: "document", keys: []string{"id"}},
	{table: "image_documents", column: "document", keys: []string{"id"}},
	{table: "templates", column: "document", keys: []string{"id"}},
	{table: "brand_kits", column: "document", keys: []string{"owner_user_id"}},
	{table: "operations", column: "ops", keys: []string{"project_id", "revision"}},
	{table: "operations", column: "inverse", keys: []string{"project_id", "revision"}},
	{table: "operations", column: "diff", keys: []string{"project_id", "revision"}},
	{table: "operations", column: "skipped", keys: []string{"project_id", "revision"}},
	{table: "image_operations", column: "ops", keys: []string{"image_id", "revision"}},
	{table: "image_operations", column: "inverse", keys: []string{"image_id", "revision"}},
	{table: "image_operations", column: "diff", keys: []string{"image_id", "revision"}},
	{table: "image_operations", column: "skipped", keys: []string{"image_id", "revision"}},
	{table: "image_operations", column: "result", keys: []string{"image_id", "revision"}},
	{table: "document_backups", column: "document", keys: []string{"id"}},
	{table: "idempotency_keys", column: "result", keys: []string{"project_id", "actor", "key"}},
	{table: "asset_info", column: "description", keys: []string{"owner_user_id", "asset_hash"}},
	{table: "asset_info", column: "rights_note", keys: []string{"owner_user_id", "asset_hash"}},
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
// transaction it seals every sealed column, rebuilds the index of the asset library, inserts the record and reads
// every sealed value back against what it was. Any failure rolls the transaction back and the workspace is as
// it was. hook, when set, is called after each stage and may fail it (a test uses it to stop in the middle).
func (s *Store) CreateAccount(
	ctx context.Context, rec account.Record, sl account.Sealer, hook func(stage string) error,
) error {
	if hook == nil {
		hook = func(string) error { return nil }
	}

	err := s.db.Tx(ctx, func(dtx *database.Tx) error {
		var sums map[string][sha256.Size]byte

		err := withLogOpen(ctx, dtx, func() error {
			var serr error

			sums, serr = sealCells(ctx, dtx, sl, allSealedCells())

			return serr
		})
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

		if err := verifySealed(ctx, dtx, sl, sums, allSealedCells()); err != nil {
			return err
		}

		return installSealGuards(ctx, dtx, allSealedCells())
	})
	if err != nil {
		return err
	}

	// An account made by this version has sealed both stages: nothing is read as clear text any more.
	s.ref.lenient.Store(false)

	return nil
}

// allSealedCells is both stages of sealed columns.
func allSealedCells() []cell { return slices.Concat(sealedCells, sealedCellsV2) }

// logTriggers are the append-only triggers of the two edit logs. Sealing an existing row updates columns they forbid
// to change, so the sealing drops them and creates them again from the SQL the database holds, inside the one
// transaction: the log is append-only for every other statement, before and after.
var logTriggers = []string{"operations_append_only", "image_operations_append_only"}

// withLogOpen runs fn with the append-only triggers of the edit logs dropped, and creates them again.
func withLogOpen(ctx context.Context, ex database.Executor, fn func() error) error {
	saved := map[string]string{}

	for _, name := range logTriggers {
		var sqlText sql.NullString

		err := ex.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?`, name).
			Scan(&sqlText)
		if errors.Is(err, sql.ErrNoRows) || !sqlText.Valid {
			continue
		}

		if err != nil {
			return err
		}

		saved[name] = sqlText.String
	}

	for name := range saved {
		if _, err := ex.ExecContext(ctx, `DROP TRIGGER `+name); err != nil {
			return err
		}
	}

	if err := fn(); err != nil {
		return err
	}

	for _, name := range logTriggers {
		if text, ok := saved[name]; ok {
			if _, err := ex.ExecContext(ctx, text); err != nil {
				return fmt.Errorf("store: put the trigger %s back: %w", name, err)
			}
		}
	}

	return nil
}

// guardMessage is what a guard raises; sealedExec turns it into account.ErrLocked.
const guardMessage = "stuarty: a sealed column was written in clear text"

// installSealGuards makes the database refuse a clear value in a sealed column from the commit of the account on.
// A write that sealed its value (in clear text, the workspace had no account) just before the creation began can
// reach the database after the creation has sealed everything; the sealing cannot see it, and only the database,
// which serializes the two, can tell which came first. The guards are in the transaction of the creation, so
// they exist exactly when the account does.
func installSealGuards(ctx context.Context, ex database.Executor, cells []cell) error {
	for _, c := range cells {
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

	var version int

	err := s.db.QueryRowContext(ctx, `SELECT seal_version FROM accounts WHERE id = 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		s.ref.lenient.Store(false)

		return nil
	}

	if err != nil {
		return fmt.Errorf("store: guards: %w", err)
	}

	// An account of seal version 0 still holds clear text in the second stage: it is read as it is until the first
	// unlock has sealed it, and the guards of that stage come with the version 1 (SDD-005 section 5.17.4).
	s.ref.lenient.Store(version < account.SealedAll)

	if version < account.SealedAll {
		return installSealGuards(ctx, s.db, sealedCells)
	}

	return installSealGuards(ctx, s.db, allSealedCells())
}

// sealCells seals every value of the given sealed columns and returns the digest of each, for the read back.
func sealCells(
	ctx context.Context, ex database.Executor, sl account.Sealer, cells []cell,
) (map[string][sha256.Size]byte, error) {
	sums := map[string][sha256.Size]byte{}

	for _, c := range cells {
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

// clearIndexes takes the words of the person out of the full-text index of the asset library. An update of an FTS5
// table leaves the old terms in its segments until a merge, so it is rebuilt. (The index of the library of notes is
// in memory and holds nothing on the disk, ADR-0053.)
func clearIndexes(ctx context.Context, ex database.Executor) error {
	for _, q := range []string{
		// The index of the files repeats the prompt in its origin column: it is written again without it.
		// The description is sealed too (ADR-0053): the index keeps none of it, as it keeps none of the prompt.
		`UPDATE asset_search SET description = '', origin = COALESCE((
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
	cells []cell,
) error {
	seen := 0

	for _, c := range cells {
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
