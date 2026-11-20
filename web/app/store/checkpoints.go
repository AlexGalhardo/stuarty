package store

import (
	"context"
	"database/sql"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

// checkpointRepo stores the checkpoints of a document in one table: `checkpoints` for a timeline (SDD-005
// section 5.14) and `image_checkpoints` for an image (section 5.14.1). The two have the same columns and the same
// rules; only the table and the name of the column that holds the document id differ, so the code is one.
type checkpointRepo struct {
	ex    database.Executor
	table string
	owner string
}

func timelineCheckpoints(ex database.Executor) checkpointRepo {
	return checkpointRepo{ex: ex, table: "checkpoints", owner: "project_id"}
}

func imageCheckpoints(ex database.Executor) checkpointRepo {
	return checkpointRepo{ex: ex, table: "image_checkpoints", owner: "image_id"}
}

// columns is the select list: the owner column is always read as the first one.
func (r checkpointRepo) columns() string {
	return r.owner + `, name, note, revision, actor, auto, at_ms`
}

func scanCheckpoint(sc scanner) (service.CheckpointRow, error) {
	var (
		c    service.CheckpointRow
		auto int
	)

	if err := sc.Scan(&c.ProjectID, &c.Name, &c.Note, &c.Revision, &c.Actor, &auto, &c.AtMs); err != nil {
		return service.CheckpointRow{}, err
	}

	c.Auto = auto == 1

	return c, nil
}

func (r checkpointRepo) Create(ctx context.Context, c service.CheckpointRow) error {
	auto := 0
	if c.Auto {
		auto = 1
	}

	_, err := r.ex.ExecContext(ctx, `INSERT INTO `+r.table+` (`+r.columns()+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ProjectID, c.Name, c.Note, c.Revision, c.Actor, auto, c.AtMs)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	return err
}

func (r checkpointRepo) Get(ctx context.Context, projectID, name string) (service.CheckpointRow, error) {
	c, err := scanCheckpoint(r.ex.QueryRowContext(ctx,
		`SELECT `+r.columns()+` FROM `+r.table+` WHERE `+r.owner+` = ? AND name = ?`, projectID, name))

	return c, mapNoRows(err)
}

func (r checkpointRepo) List(ctx context.Context, projectID string) ([]service.CheckpointRow, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+r.columns()+` FROM `+r.table+` WHERE `+r.owner+` = ?
 ORDER BY revision DESC, name`, projectID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []service.CheckpointRow

	for rows.Next() {
		c, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, c)
	}

	return out, rows.Err()
}

func (r checkpointRepo) Rename(ctx context.Context, projectID, name, newName string, note *string) error {
	var (
		res sql.Result
		err error
	)

	// A renamed checkpoint is a named one: auto goes to 0, so the ceiling of the automatic ones keeps it.
	if note != nil {
		res, err = r.ex.ExecContext(ctx,
			`UPDATE `+r.table+` SET name = ?, note = ?, auto = 0 WHERE `+r.owner+` = ? AND name = ?`,
			newName, *note, projectID, name)
	} else {
		res, err = r.ex.ExecContext(ctx, `UPDATE `+r.table+` SET name = ?, auto = 0 WHERE `+r.owner+` = ? AND name = ?`,
			newName, projectID, name)
	}

	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	if err != nil {
		return err
	}

	return needOne(res)
}

func (r checkpointRepo) Remove(ctx context.Context, projectID, name string) error {
	res, err := r.ex.ExecContext(ctx, `DELETE FROM `+r.table+` WHERE `+r.owner+` = ? AND name = ?`, projectID, name)
	if err != nil {
		return err
	}

	return needOne(res)
}

// needOne is ErrNotFound for a statement that changed no row.
func needOne(res sql.Result) error {
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return service.ErrNotFound
	}

	return nil
}

func (r checkpointRepo) TrimAuto(ctx context.Context, projectID string, keep int) error {
	_, err := r.ex.ExecContext(ctx, `DELETE FROM `+r.table+` WHERE `+r.owner+` = ? AND auto = 1 AND name NOT IN (
 SELECT name FROM `+r.table+` WHERE `+r.owner+` = ? AND auto = 1 ORDER BY at_ms DESC, revision DESC, name DESC LIMIT ?)`,
		projectID, projectID, keep)

	return err
}
