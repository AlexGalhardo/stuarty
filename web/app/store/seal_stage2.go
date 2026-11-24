package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"

	"github.com/AlexGalhardo/vukem/database"
)

// stageTwoBatch is how many values one transaction seals: small enough not to hold the writer for long, large enough
// that a workspace of tens of thousands of log rows is a few hundred transactions.
const stageTwoBatch = 200

// SealVersion is the seal version of the account of this workspace (SDD-005 section 5.17.4): 0 while the second
// stage of sealed columns may still hold clear text, 1 once it is sealed, and 1 when there is no account to be
// behind (nothing to seal).
func (s *Store) SealVersion(ctx context.Context) (int, error) {
	rec, err := accountRepo{ex: s.db}.Get(ctx)
	if errors.Is(err, account.ErrNoAccount) {
		return account.SealedAll, nil
	}

	if err != nil {
		return 0, err
	}

	return rec.SealVersion, nil
}

// sealedOpens says whether a stored value is a sealed value that opens for its cell and row.
func sealedOpens(sl account.Sealer, c cell, row cellRow) bool {
	if !account.IsSealed(row.plain) {
		return false
	}

	_, err := sl.Open(c.table, c.column, row.key, row.plain)

	return err == nil
}

// readFrom reads at most limit rows of the cell with a rowid above after, in rowid order, with their rowids. It is
// the cursor of the migration: ordered by the row, never by a sealed column.
func (c cell) readFrom(ctx context.Context, ex database.Executor, after int64, limit int) ([]cellRow, []int64, error) {
	rows, err := ex.QueryContext(ctx, "SELECT rowid, "+strings.Join(c.keys, ", ")+", "+c.column+" FROM "+c.table+
		" WHERE rowid > ? AND "+c.column+" IS NOT NULL ORDER BY rowid LIMIT ?", after, limit)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = rows.Close() }()

	var (
		out []cellRow
		ids []int64
	)

	for rows.Next() {
		vals := make([]sql.NullString, len(c.keys)+1)
		dest := make([]any, len(vals)+1)

		var id int64

		dest[0] = &id

		for i := range vals {
			dest[i+1] = &vals[i]
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}

		row := cellRow{plain: vals[len(c.keys)].String}
		for _, v := range vals[:len(c.keys)] {
			row.where = append(row.where, v.String)
		}

		row.key = strings.Join(row.where, "/")
		out = append(out, row)
		ids = append(ids, id)
	}

	return out, ids, rows.Err()
}

// SealStageTwo seals the second stage of the sealed columns of a workspace whose account was made before ADR-0053
// (SDD-005 section 5.17.4). It works in batches, each its own transaction in which every value is read back against
// the digest of what it was, so a kill after any batch leaves a workspace that opens (the second stage is read as
// clear text where it is not sealed yet) and the next run continues: a value that already opens is skipped. Only when
// every column was read back does one last transaction create the guards and set the version to 1. hook, when set,
// is called after each batch and at "verified", and may fail the run (a test kills the process there).
func (s *Store) SealStageTwo(ctx context.Context, sl account.Sealer, hook func(stage string) error) error {
	if hook == nil {
		hook = func(string) error { return nil }
	}

	version, err := s.SealVersion(ctx)
	if err != nil || version >= account.SealedAll {
		return err
	}

	for _, c := range sealedCellsV2 {
		var after int64

		for {
			n, last, err := s.sealBatch(ctx, sl, c, after)
			if err != nil {
				return fmt.Errorf("store: sealing %s.%s: %w", c.table, c.column, err)
			}

			if n == 0 {
				break
			}

			after = last

			crashpoint.Hit("seal_documents.after_batch")

			if err := hook("batch"); err != nil {
				return err
			}
		}
	}

	if err := s.verifyStageTwo(ctx, sl); err != nil {
		return err
	}

	if err := hook("verified"); err != nil {
		return err
	}

	crashpoint.Hit("seal_documents.before_version")

	err = s.db.Tx(ctx, func(dtx *database.Tx) error {
		if _, err := dtx.ExecContext(ctx, `UPDATE accounts SET seal_version = ? WHERE id = 1`, account.SealedAll); err != nil {
			return err
		}

		// What the creation of the account does for the first stage, done for the second: the description also
		// leaves the index of the asset library.
		if err := clearIndexes(ctx, dtx); err != nil {
			return err
		}

		return installSealGuards(ctx, dtx, sealedCellsV2)
	})
	if err != nil {
		return err
	}

	s.ref.lenient.Store(false)

	crashpoint.Hit("seal_documents.after_version")

	return nil
}

// sealBatch seals the next batch of one cell above the rowid after. It returns how many rows it read and the last
// rowid, so the caller can go on; a row that is sealed already costs a read and nothing else.
func (s *Store) sealBatch(ctx context.Context, sl account.Sealer, c cell, after int64) (int, int64, error) {
	var (
		n    int
		last = after
	)

	err := s.db.Tx(ctx, func(dtx *database.Tx) error {
		return withLogOpen(ctx, dtx, func() error {
			rows, ids, err := c.readFrom(ctx, dtx, after, stageTwoBatch)
			if err != nil {
				return err
			}

			n = len(rows)

			if n > 0 {
				last = ids[n-1]
			}

			sums := map[string][sha256.Size]byte{}

			for i, row := range rows {
				// A clear value is the exception during this migration, and a sealed value that opens is the rule of
				// a workspace that was resumed.
				if sealedOpens(sl, c, row) {
					continue
				}

				sealed, err := sl.Seal(c.table, c.column, row.key, row.plain)
				if err != nil {
					return err
				}

				sums[fmt.Sprint(ids[i])] = sha256.Sum256([]byte(row.plain))

				if err := c.update(ctx, dtx, row, sealed); err != nil {
					return err
				}
			}

			// Read back what this batch wrote, in the same transaction: each value opens and is what it was.
			back, _, err := c.readFrom(ctx, dtx, after, stageTwoBatch)
			if err != nil {
				return err
			}

			if len(back) != len(rows) {
				return fmt.Errorf("%d values were read back, %d were sealed", len(back), len(rows))
			}

			for i, row := range back {
				plain, err := sl.Open(c.table, c.column, row.key, row.plain)
				if err != nil {
					return fmt.Errorf("%s.%s of %s did not read back: %w", c.table, c.column, row.key, err)
				}

				if want, ok := sums[fmt.Sprint(ids[i])]; ok && sha256.Sum256([]byte(plain)) != want {
					return errors.New("a value is not what it was before it was sealed: " + c.table + "." + c.column)
				}
			}

			return nil
		})
	})

	return n, last, err
}

// verifyStageTwo is the last read of the migration: every value of every second-stage column opens.
func (s *Store) verifyStageTwo(ctx context.Context, sl account.Sealer) error {
	for _, c := range sealedCellsV2 {
		var after int64

		for {
			rows, ids, err := c.readFrom(ctx, s.db, after, stageTwoBatch)
			if err != nil {
				return err
			}

			if len(rows) == 0 {
				break
			}

			for _, row := range rows {
				if !sealedOpens(sl, c, row) {
					return fmt.Errorf("store: %s.%s of %s is not sealed after the migration", c.table, c.column, row.key)
				}
			}

			after = ids[len(ids)-1]
		}
	}

	return nil
}
