package store

import (
	"database/sql"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/library"

	"github.com/AlexGalhardo/vukem/database"
)

// repos is the set of repositories over one executor: the database for reads
// and single statements, or a transaction.
type repos struct{ ex database.Executor }

func (r repos) Projects() service.ProjectRepo        { return projectRepo(r) }
func (r repos) Productions() service.ProductionRepo  { return productionRepo(r) }
func (r repos) Operations() service.OperationRepo    { return operationRepo(r) }
func (r repos) Idempotency() service.IdempotencyRepo { return idempotencyRepo(r) }
func (r repos) Assets() service.AssetRepo            { return assetRepo(r) }
func (r repos) Jobs() service.JobRepo                { return jobRepo(r) }
func (r repos) Templates() service.TemplateRepo      { return templateRepo(r) }
func (r repos) Backups() service.BackupRepo          { return backupRepo(r) }
func (r repos) Audit() service.AuditRepo             { return auditRepo(r) }
func (r repos) Fonts() service.FontRepo              { return fontRepo(r) }
func (r repos) BrandKit() service.BrandKitRepo       { return brandKitRepo(r) }
func (r repos) Connections() connections.Store       { return connectionRepo(r) }
func (r repos) Approvals() approval.Store            { return approvalRepo(r) }

// nullJSON stores an empty raw message as NULL.
func nullJSON(m json.RawMessage) any {
	if len(m) == 0 {
		return nil
	}
	return string(m)
}

func rawJSON(s sql.NullString) json.RawMessage {
	if !s.Valid {
		return nil
	}
	return json.RawMessage(s.String)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ptrInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func ptrInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func ptrString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// rowsAffected reports how many rows a statement changed.
func rowsAffected(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
