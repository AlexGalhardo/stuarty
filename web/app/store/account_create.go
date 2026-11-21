package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/account"

	"github.com/AlexGalhardo/vukem/database"
)

// cell is a sealed column and the columns that make the key of its row (SDD-005 section 5.16.2). The repositories
// bind a value to its cell the same way, through the row key they pass to the sealer.
type cell struct {
	table, column string
	keys          []string
}

var sealedCells = []cell{
	{"project_items", "body", []string{"id"}},
	{"text_revisions", "body", []string{"item_id", "revision"}},
	{"chat_events", "body_json", []string{"session_id", "seq"}},
	{"publications", "text", []string{"id"}},
	{"publication_targets", "fields", []string{"id"}},
	{"publication_targets", "snapshot", []string{"id"}},
}

type cellRow struct {
	where []string
	key   string
	plain string
}

func (c cell) read(ctx context.Context, ex database.Executor) ([]cellRow, error) {
	rows, err := ex.QueryContext(ctx, "SELECT "+strings.Join(c.keys, ", ")+", "+c.column+" FROM "+c.table+
		" WHERE "+c.column+" IS NOT NULL")
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

// CreateAccount makes the account in a workspace that may hold data (SDD-005 section 5.16.3): in one
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
		sums := map[string][sha256.Size]byte{}

		for _, c := range sealedCells {
			rows, err := c.read(ctx, dtx)
			if err != nil {
				return err
			}

			for _, row := range rows {
				sealed, err := sl.Seal(c.table, c.column, row.key, row.plain)
				if err != nil {
					return err
				}

				sums[c.table+"."+c.column+"/"+row.key] = sha256.Sum256([]byte(row.plain))

				if err := c.update(ctx, dtx, row, sealed); err != nil {
					return err
				}
			}
		}

		if err := hook("sealed"); err != nil {
			return err
		}

		if _, err := dtx.ExecContext(ctx, `UPDATE library_fts SET body = ''`); err != nil {
			return err
		}

		if err := (accountRepo{ex: dtx}).Create(ctx, rec); err != nil {
			return err
		}

		if err := hook("inserted"); err != nil {
			return err
		}

		return verifySealed(ctx, dtx, sl, sums)
	})
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
