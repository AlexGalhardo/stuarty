package service

import (
	"context"
	"encoding/json"
	"time"
)

const (
	defAuditLimit = 50
	maxAuditLimit = 200
	// maxAuditPasses bounds the pages read to fill one answer when rows of other users are skipped.
	maxAuditPasses = 20
)

// AuditListRequest pages the audit table, newest first.
type AuditListRequest struct {
	// ProjectID keeps the entries of one project; empty means every project the caller may see.
	ProjectID string `json:"project_id,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// AuditEntry is one row of the audit table.
type AuditEntry struct {
	ID        int64           `json:"id"`
	At        string          `json:"at"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	ProjectID string          `json:"project_id,omitempty"`
	SubjectID string          `json:"subject_id,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// AuditListResult is a page of the audit table.
type AuditListResult struct {
	Entries    []AuditEntry `json:"entries"`
	NextCursor *string      `json:"next_cursor"`
}

// AuditList needs the admin scope. A key limited to one project sees that
// project only, and in network mode a caller sees the entries of the projects
// of their own user; an entry that belongs to no project is the system's and is
// not shown there.
func (s *svc) AuditList(ctx context.Context, req AuditListRequest) (*AuditListResult, error) {
	c, err := s.authorizeProject(ctx, "AuditList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	project := req.ProjectID
	if c.ProjectID != "" {
		project = c.ProjectID
	}

	if project != "" {
		if _, lerr := s.loadProjectScope(ctx, c, project); lerr != nil {
			return nil, lerr
		}
	}

	before, cerr := decodeRevisionCursor(req.Cursor, req.ProjectID)
	if cerr != nil {
		return nil, cerr
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	limit := clampLimit(req.Limit, defAuditLimit, maxAuditLimit)
	scoped := project != "" || c.OwnerUserID != ""
	res := &AuditListResult{Entries: make([]AuditEntry, 0, limit)}
	owners := map[string]bool{}

	for pass := 0; pass < maxAuditPasses && len(res.Entries) < limit; pass++ {
		rows, lerr := s.store.Audit().List(ctx, project, before, limit+1)
		if lerr != nil {
			return nil, asError(lerr)
		}

		for i := range rows {
			if len(res.Entries) == limit {
				next := encodeRevisionCursor(req.ProjectID, res.Entries[len(res.Entries)-1].ID)
				res.NextCursor = &next

				return res, nil
			}

			before = rows[i].ID

			if scoped && !s.auditVisible(ctx, c, &rows[i], owners) {
				continue
			}

			res.Entries = append(res.Entries, auditEntry(&rows[i]))
		}

		if len(rows) <= limit {
			return res, nil
		}
	}

	if before > 0 {
		next := encodeRevisionCursor(req.ProjectID, before)
		res.NextCursor = &next
	}

	return res, nil
}

// auditVisible tells whether a row belongs to a project the caller may see.
func (s *svc) auditVisible(ctx context.Context, c Caller, r *AuditRow, memo map[string]bool) bool {
	if c.OwnerUserID == "" {
		return true
	}

	if r.ProjectID == nil {
		return false
	}

	if v, ok := memo[*r.ProjectID]; ok {
		return v
	}

	h, err := s.scopeRow(ctx, *r.ProjectID)
	v := err == nil && visible(c, h.OwnerUserID)
	memo[*r.ProjectID] = v

	return v
}

func auditEntry(r *AuditRow) AuditEntry {
	e := AuditEntry{
		ID: r.ID, At: time.UnixMilli(r.AtMs).UTC().Format(time.RFC3339), Actor: r.Actor, Action: r.Action,
		Details: r.Details,
	}

	if r.ProjectID != nil {
		e.ProjectID = *r.ProjectID
	}

	if r.SubjectID != nil {
		e.SubjectID = *r.SubjectID
	}

	return e
}
