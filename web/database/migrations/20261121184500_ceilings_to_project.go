package migrations

import "github.com/AlexGalhardo/vukem/schema"

// Since VV-315 an action is matched with the project that holds its timeline, so a ceiling stored with the id of a
// later timeline (written before a ceiling on a timeline id was refused) no longer limits anything. Author decision
// D-132: a ceiling written for a project covers every timeline of it, so each such row moves to the project that
// owns the timeline (SDD-015 section 3, SDD-005 section 5.12).
//
// The timeline is found in project_items (kind timeline, ref = the timeline id), trashed ones included. When the
// project already has a ceiling in the same currency, or several timelines of it do, the rows are folded field by
// field to the lowest limit of each (a null limit is no limit, so it never wins), which never raises a limit. A row
// whose id matches no timeline item (an emptied project, an id that never existed) is left as it is: it matches no
// action, so it limits nothing, exactly as before. The first timeline has the id of its project, so those rows and
// those of the workspace (empty id) are not candidates.
//
// Approval rules are not migrated: a rule stored with a later timeline id keeps not matching, so what it was meant
// to approve waits for a person.
func m20261121184500CeilingsToProject() schema.Migration {
	return schema.Migration{
		ID: "20261121184500_ceilings_to_project",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TEMP TABLE ceilings_to_move AS
SELECT c.project_id AS timeline_id, i.project_id AS owner, c.currency
  FROM cost_ceilings c
  JOIN project_items i ON i.kind = 'timeline' AND i.ref = c.project_id
 WHERE c.project_id <> '' AND i.project_id <> c.project_id
   AND c.project_id NOT IN (SELECT id FROM projects)`,
				`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms)
SELECT owner, currency, min(per_item_micros), min(per_day_micros), 'migration', CAST(strftime('%s', 'now') AS INTEGER) * 1000
  FROM (
    SELECT m.owner AS owner, c.currency AS currency, c.per_item_micros, c.per_day_micros
      FROM cost_ceilings c JOIN ceilings_to_move m ON m.timeline_id = c.project_id AND m.currency = c.currency
    UNION ALL
    SELECT c.project_id, c.currency, c.per_item_micros, c.per_day_micros
      FROM cost_ceilings c WHERE EXISTS (SELECT 1 FROM ceilings_to_move m WHERE m.owner = c.project_id AND m.currency = c.currency)
  )
 WHERE true
 GROUP BY owner, currency
ON CONFLICT (project_id, currency) DO UPDATE SET
  per_item_micros = excluded.per_item_micros,
  per_day_micros  = excluded.per_day_micros,
  updated_by      = excluded.updated_by,
  updated_ms      = excluded.updated_ms`,
				`DELETE FROM cost_ceilings
 WHERE EXISTS (SELECT 1 FROM ceilings_to_move m WHERE m.timeline_id = cost_ceilings.project_id AND m.currency = cost_ceilings.currency)`,
				`DROP TABLE ceilings_to_move`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		// The merged rows cannot be split back into the timelines they came from, so Down leaves them on the
		// project, where they limit more, never less.
		Down: func(*schema.Schema) error { return nil },
	}
}
