package migrations

import "github.com/AlexGalhardo/vukem/schema"

// RF-134 asks for a record that cannot be edited, and SDD-005 section 5.9 says
// the audit table is never purged, TrashEmpty included. The database enforces
// it, as it does for operations (SDD-007 T-28).
func m20261114100900CreateAuditAppendOnlyTriggers() schema.Migration {
	return schema.Migration{
		ID: "20261114100900_create_audit_append_only_triggers",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit
BEGIN
  SELECT RAISE(ABORT, 'audit is append-only');
END`,
				`CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit
BEGIN
  SELECT RAISE(ABORT, 'audit is append-only');
END`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`DROP TRIGGER IF EXISTS audit_no_update`); err != nil {
				return err
			}
			return s.Raw(`DROP TRIGGER IF EXISTS audit_no_delete`)
		},
	}
}
