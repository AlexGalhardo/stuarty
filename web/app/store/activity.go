package store

import (
	"context"
	"database/sql"

	"github.com/AlexGalhardo/stuarty/service"
)

// The document logs store sealed payloads. Select only their clear metadata.
// A video operation belongs to the production that contains its timeline.
const activityPageSQL = `
WITH activity(at_ms, source, subject, sequence, actor, action, project_id, details, owner) AS (
 SELECT a.at_ms, 'audit', coalesce(a.subject_id, ''), a.id, a.actor, a.action,
        coalesce(p.id, parent.id, a.project_id, ''), a.details,
        CASE WHEN p.id IS NOT NULL AND parent.id IS NOT NULL AND p.owner_user_id IS NOT parent.owner_user_id
             THEN NULL ELSE coalesce(p.owner_user_id, parent.owner_user_id, '') END
 FROM audit a
 LEFT JOIN project_items item ON item.kind = 'timeline' AND item.ref = a.project_id
 LEFT JOIN projects p ON p.id = a.project_id
 LEFT JOIN projects parent ON parent.id = item.project_id
 UNION ALL
 SELECT o.at_ms, 'video', o.project_id, o.revision, o.actor, 'timeline.' || o.kind,
        p.id, NULL, coalesce(p.owner_user_id, '')
 FROM operations o
 JOIN project_items item ON item.kind = 'timeline' AND item.ref = o.project_id
 JOIN projects p ON p.id = item.project_id
 UNION ALL
 SELECT o.at_ms, 'image', o.image_id, o.revision, o.actor, 'image.' || o.kind,
        coalesce(p.id, ''), NULL, coalesce(d.owner_user_id, '')
 FROM image_operations o
 JOIN image_documents d ON d.id = o.image_id
 LEFT JOIN (SELECT DISTINCT ref, project_id FROM project_items WHERE kind = 'image') item ON item.ref = d.id
 LEFT JOIN projects p ON p.id = item.project_id AND p.owner_user_id IS d.owner_user_id
 UNION ALL
 SELECT r.created_ms, 'text', r.item_id, r.revision, r.actor, 'text.' || r.source,
        p.id, NULL, coalesce(p.owner_user_id, '')
 FROM text_revisions r
 JOIN project_items item ON item.id = r.item_id
 JOIN projects p ON p.id = item.project_id
 WHERE r.source IN ('write', 'revert')
)
SELECT at_ms, source, subject, sequence, actor, action, project_id, details
FROM activity
WHERE (?1 = '' OR project_id = ?1)
  AND (?2 = '' OR owner = ?2
       OR (source = 'audit' AND project_id = '' AND actor = 'user:' || ?2))
  AND (?3 = '' OR (at_ms, source, subject, sequence, project_id) < (?4, ?3, ?5, ?6, ?7))
ORDER BY at_ms DESC, source DESC, subject DESC, sequence DESC, project_id DESC
LIMIT ?8`

func (r auditRepo) ActivityPage(ctx context.Context, q service.ActivityPageQuery) ([]service.ActivityRow, error) {
	rows, err := r.ex.QueryContext(ctx, activityPageSQL, q.ProjectID, q.Owner, q.Before.Source,
		q.Before.AtMs, q.Before.Subject, q.Before.Sequence, q.Before.ProjectID, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []service.ActivityRow
	for rows.Next() {
		var row service.ActivityRow
		var details sql.NullString
		key := &row.Key
		if err := rows.Scan(&key.AtMs, &key.Source, &key.Subject, &key.Sequence,
			&row.Actor, &row.Action, &key.ProjectID, &details); err != nil {
			return nil, err
		}

		row.Details = rawJSON(details)
		result = append(result, row)
	}

	return result, rows.Err()
}
