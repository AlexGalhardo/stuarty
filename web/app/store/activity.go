package store

import (
	"context"
	"database/sql"

	"github.com/AlexGalhardo/stuarty/service"
)

// The document logs store sealed payloads. Select only their clear metadata.
// A video operation belongs to the production that contains its timeline.
const activityPageSQL = `
WITH audited_jobs AS (
 SELECT DISTINCT subject_id FROM audit WHERE action = 'job.start'
),
archived_chat AS (
 SELECT at_ms, actor, project_id,
        substr(subject_id, 1, instr(subject_id, '/') - 1) AS session_id,
        CAST(substr(subject_id, instr(subject_id, '/') + 1) AS INTEGER) AS seq
 FROM audit
 WHERE action = 'chat.tool_call'
),
job_events AS (
 SELECT j.id, j.kind, j.actor, j.created_ms, j.finished_ms, j.state,
        coalesce(parent.id, p.id, '') AS project_id, coalesce(parent.owner_user_id, p.owner_user_id, k.user_id, '') AS owner
 FROM jobs j
 LEFT JOIN project_items item ON item.kind = 'timeline' AND item.ref = j.project_id
 LEFT JOIN projects parent ON parent.id = item.project_id
 LEFT JOIN projects p ON p.id = j.project_id
 LEFT JOIN api_keys k ON j.project_id IS NULL AND j.actor = 'key:' || k.id
 WHERE j.kind <> 'ingest'
   AND (j.project_id IS NULL OR parent.id IS NOT NULL OR p.id IS NOT NULL)
   AND (parent.id IS NULL OR p.id IS NULL OR parent.owner_user_id IS p.owner_user_id)
),
activity(at_ms, source, subject, sequence, actor, action, project_id, details, owner) AS (
 SELECT a.at_ms, 'audit', coalesce(a.subject_id, ''), a.id, a.actor, a.action,
        coalesce(p.id, parent.id, a.project_id, ''), a.details,
        CASE WHEN p.id IS NOT NULL AND parent.id IS NOT NULL AND p.owner_user_id IS NOT parent.owner_user_id
             THEN NULL ELSE coalesce(p.owner_user_id, parent.owner_user_id, k.user_id, '') END
 FROM audit a
 LEFT JOIN project_items item ON item.kind = 'timeline' AND item.ref = a.project_id
 LEFT JOIN projects p ON p.id = a.project_id
 LEFT JOIN projects parent ON parent.id = item.project_id
 LEFT JOIN api_keys k ON a.project_id IS NULL AND a.actor = 'key:' || k.id
 WHERE a.action <> 'chat.tool_call' AND (?1 <> '' OR a.action <> 'asset.rename.project')
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
 WHERE r.source IN ('created', 'write', 'revert')
 UNION ALL
 SELECT a.imported_ms, 'asset', a.asset_id, 1, a.imported_by, 'media.import',
        coalesce(parent.id, p.id), NULL, coalesce(parent.owner_user_id, p.owner_user_id, '')
 FROM project_assets a
 LEFT JOIN project_items item ON item.kind = 'timeline' AND item.ref = a.project_id
 LEFT JOIN projects parent ON parent.id = item.project_id
 LEFT JOIN projects p ON p.id = a.project_id
 WHERE (parent.id IS NOT NULL OR p.id IS NOT NULL)
   AND (parent.id IS NULL OR p.id IS NULL OR parent.owner_user_id IS p.owner_user_id)
 UNION ALL
 SELECT j.created_ms, 'job', j.id, 1, j.actor, 'job.' || j.kind || '.requested', j.project_id, NULL, j.owner
 FROM job_events j
 LEFT JOIN audited_jobs recorded ON recorded.subject_id = j.id
 WHERE recorded.subject_id IS NULL
 UNION ALL
 SELECT j.finished_ms, 'job', j.id, 2, j.actor, 'job.' || j.kind || '.' || j.state, j.project_id, NULL, j.owner
 FROM job_events j
 WHERE j.finished_ms IS NOT NULL AND j.state IN ('succeeded','failed','cancelled')
 UNION ALL
 SELECT e.at_ms, 'chat', c.id, e.seq,
        CASE WHEN c.owner_user_id IS NULL THEN 'local:chat-' || substr(c.id, 5) ELSE 'key:' || c.id END,
        'chat.tool_call', p.id, NULL, coalesce(p.owner_user_id, '')
 FROM chat_events e
 JOIN chat_sessions c ON c.id = e.session_id
 JOIN projects p ON p.id = c.project_id
 LEFT JOIN archived_chat archived ON archived.session_id = e.session_id AND archived.seq = e.seq
 WHERE e.kind = 'tool_call' AND c.owner_user_id IS p.owner_user_id AND archived.session_id IS NULL
 UNION ALL
 SELECT a.at_ms, 'chat', a.session_id, a.seq, a.actor, 'chat.tool_call',
        a.project_id, NULL, coalesce(p.owner_user_id, '')
 FROM archived_chat a
 LEFT JOIN projects p ON p.id = a.project_id
 UNION ALL
 SELECT t.published_ms, 'publication', t.id, 1, 'scheduler', 'publication.published',
        p.id, NULL, coalesce(p.owner_user_id, '')
 FROM publication_targets t
 JOIN publications publication ON publication.id = t.publication_id
 JOIN projects p ON p.id = publication.project_id
 WHERE t.published_ms IS NOT NULL AND publication.owner_user_id IS p.owner_user_id
)
SELECT activity.at_ms, source, subject, sequence, activity.actor, activity.action,
       activity.project_id, details, coalesce(chat_sessions.profile, '')
FROM activity
LEFT JOIN chat_sessions ON source = 'chat' AND subject = chat_sessions.id
WHERE (?1 = '' OR activity.project_id = ?1)
  AND (?2 = '' OR owner = ?2
       OR (activity.project_id = '' AND activity.actor = 'user:' || ?2))
  AND (?3 = '' OR (activity.at_ms, source, subject, sequence, activity.project_id) < (?4, ?3, ?5, ?6, ?7))
ORDER BY activity.at_ms DESC, source DESC, subject DESC, sequence DESC, activity.project_id DESC
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
			&row.Actor, &row.Action, &key.ProjectID, &details, &row.ActorLabel); err != nil {
			return nil, err
		}

		row.Details = rawJSON(details)
		result = append(result, row)
	}

	return result, rows.Err()
}
