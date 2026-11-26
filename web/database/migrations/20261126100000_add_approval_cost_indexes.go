package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261126100000AddApprovalCostIndexes adds composite indexes on cost_ledger and approval_requests
// so per-project spend summaries and expiry sweeps do not scan the full tables.
func m20261126100000AddApprovalCostIndexes() schema.Migration {
	return schema.Migration{
		ID: "20261126100000_add_approval_cost_indexes",
		Up: func(s *schema.Schema) error {
			for _, stmt := range []string{
				`CREATE INDEX IF NOT EXISTS cost_ledger_project_state_index ON cost_ledger (project_id, state)`,
				`CREATE INDEX IF NOT EXISTS approval_requests_project_state_created_index ON approval_requests (project_id, state, created_ms)`,
				`CREATE INDEX IF NOT EXISTS approval_requests_state_expires_index ON approval_requests (state, expires_ms)`,
			} {
				if err := s.Raw(stmt); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, stmt := range []string{
				`DROP INDEX IF EXISTS approval_requests_state_expires_index`,
				`DROP INDEX IF EXISTS approval_requests_project_state_created_index`,
				`DROP INDEX IF EXISTS cost_ledger_project_state_index`,
			} {
				if err := s.Raw(stmt); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
