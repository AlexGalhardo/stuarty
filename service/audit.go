package service

import "context"

// appendAudit writes the audit row of an action that is not an edit of a
// document, in the transaction of the action (SDD-005 section 5.9). An empty
// projectID or subjectID is stored as NULL.
func appendAudit(ctx context.Context, tx Tx, now int64, actor, action, projectID, subjectID string) error {
	row := AuditRow{AtMs: now, Actor: actor, Action: action}

	if projectID != "" {
		row.ProjectID = &projectID
	}

	if subjectID != "" {
		row.SubjectID = &subjectID
	}

	return tx.Audit().Append(ctx, row)
}
