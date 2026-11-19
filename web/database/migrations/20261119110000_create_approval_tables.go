package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The approval queue, its rules, the cost ceilings and the cost ledger (SDD-015 section 3, ADR-0032).
// Amounts are integers in millionths of a currency unit. No table holds a secret.
func m20261119110000CreateApprovalTables() schema.Migration {
	return schema.Migration{
		ID: "20261119110000_create_approval_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE approval_requests (
  id              TEXT    NOT NULL,
  actor           TEXT    NOT NULL,
  owner_user_id   TEXT,
  kind            TEXT    NOT NULL,
  summary         TEXT    NOT NULL,
  connection_id   TEXT,
  project_id      TEXT,
  job_id          TEXT,
  payload_ref     TEXT    NOT NULL DEFAULT '',
  estimate_micros INTEGER NOT NULL DEFAULT 0 CHECK (estimate_micros >= 0),
  currency        TEXT    NOT NULL DEFAULT '',
  state           TEXT    NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','approved','rejected','expired')),
  decided_by      TEXT,
  rule_id         TEXT,
  reason          TEXT    NOT NULL DEFAULT '',
  created_ms      INTEGER NOT NULL,
  decided_ms      INTEGER,
  expires_ms      INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX approval_requests_state_created_ms_index ON approval_requests (state, created_ms)`,
				`CREATE INDEX approval_requests_job_id_index ON approval_requests (job_id)`,
				`CREATE TABLE approval_rules (
  id              TEXT    NOT NULL,
  kind            TEXT    NOT NULL,
  connection_id   TEXT,
  project_id      TEXT,
  currency        TEXT    NOT NULL DEFAULT '',
  per_item_micros INTEGER CHECK (per_item_micros IS NULL OR per_item_micros >= 0),
  per_day_micros  INTEGER CHECK (per_day_micros IS NULL OR per_day_micros >= 0),
  enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_by      TEXT    NOT NULL,
  created_ms      INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE TABLE cost_ceilings (
  project_id      TEXT    NOT NULL DEFAULT '',
  currency        TEXT    NOT NULL,
  per_item_micros INTEGER CHECK (per_item_micros IS NULL OR per_item_micros >= 0),
  per_day_micros  INTEGER CHECK (per_day_micros IS NULL OR per_day_micros >= 0),
  updated_by      TEXT    NOT NULL,
  updated_ms      INTEGER NOT NULL,
  PRIMARY KEY (project_id, currency)
)`,
				`CREATE TABLE cost_ledger (
  id              TEXT    NOT NULL,
  request_id      TEXT    NOT NULL,
  job_id          TEXT    NOT NULL,
  connection_id   TEXT    NOT NULL DEFAULT '',
  project_id      TEXT    NOT NULL DEFAULT '',
  kind            TEXT    NOT NULL,
  currency        TEXT    NOT NULL,
  estimate_micros INTEGER NOT NULL CHECK (estimate_micros >= 0),
  actual_micros   INTEGER CHECK (actual_micros IS NULL OR actual_micros >= 0),
  state           TEXT    NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','settled','released')),
  day             TEXT    NOT NULL,
  created_ms      INTEGER NOT NULL,
  settled_ms      INTEGER,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX cost_ledger_day_currency_index ON cost_ledger (day, currency)`,
				`CREATE INDEX cost_ledger_job_id_index ON cost_ledger (job_id)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"cost_ledger", "cost_ceilings", "approval_rules", "approval_requests"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
