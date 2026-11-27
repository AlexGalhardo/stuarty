package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/audit"
	"github.com/AlexGalhardo/stuarty/core/command"
)

// ActivityListRequest queries the human- and agent-attributed activity timeline,
// newest first, globally or scoped to one project (epic 111).
type ActivityListRequest struct {
	ProjectID string `json:"project_id,omitempty"`
	ActorKind string `json:"actor_kind,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// ActivityEntry is one normalized entry in the Activity History timeline.
type ActivityEntry struct {
	ID          string          `json:"id"`
	At          string          `json:"at"`
	Actor       string          `json:"actor"                  untrusted:"true"`
	ActorKind   string          `json:"actor_kind"`
	ActorLabel  string          `json:"actor_label"            untrusted:"true"`
	Domain      string          `json:"domain"`
	Action      string          `json:"action"`
	ProjectID   string          `json:"project_id,omitempty"`
	ProjectName string          `json:"project_name,omitempty" untrusted:"true"`
	SubjectID   string          `json:"subject_id,omitempty"`
	SubjectKind string          `json:"subject_kind,omitempty"`
	Revision    int64           `json:"revision,omitempty"`
	Details     json.RawMessage `json:"details,omitempty"`
}

// ActivityListResult is one cursor-paginated page of ActivityEntry rows.
type ActivityListResult struct {
	Entries    []ActivityEntry `json:"entries"`
	NextCursor *string         `json:"next_cursor"`
}

// ActivityKey orders events from independent logs and breaks ties within a millisecond.
type ActivityKey struct {
	AtMs      int64  `json:"at"`
	Source    string `json:"source"`
	Subject   string `json:"subject"`
	Sequence  int64  `json:"sequence"`
	ProjectID string `json:"project"`
}

type ActivityRow struct {
	Key     ActivityKey
	Actor   string
	Action  string
	Details json.RawMessage
}

type ActivityPageQuery struct {
	ProjectID string
	Owner     string
	Before    ActivityKey
	Limit     int
}

type activityCursor struct {
	Project string      `json:"project"`
	Before  ActivityKey `json:"before"`
}

func parseActivityCursor(cursor, project string) (ActivityKey, error) {
	if cursor == "" {
		return ActivityKey{}, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	var page activityCursor
	if err != nil || json.Unmarshal(raw, &page) != nil || page.Project != project ||
		page.Before.Source == "" || page.Before.Sequence < 1 {
		return ActivityKey{}, newError(command.CodeValidation, "invalid activity cursor.").withPath("cursor")
	}

	return page.Before, nil
}

func (res *ActivityListResult) setCursor(project string, key ActivityKey) {
	next := base64.RawURLEncoding.EncodeToString(marshalOr(activityCursor{Project: project, Before: key}))
	res.NextCursor = &next
}

// ActivityList lists recent user, agent, and system actions globally or for a single project.
// It requires ScopeRead (every signed-in person can inspect their own history), enforces
// project-key isolation, local-lock E_LOCKED, and network-mode owner isolation. Only
// allowlisted audit details enter the result.
func (s *svc) ActivityList(ctx context.Context, req ActivityListRequest) (*ActivityListResult, error) {
	c, err := s.authorizeProject(ctx, "ActivityList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if !audit.ValidateActorKind(req.ActorKind) {
		return nil, &Error{
			Code:    command.CodeValidation,
			Path:    "actor_kind",
			Message: "actor_kind must be all, human, agent, or system.",
			Details: map[string]any{"allowed": audit.AllowedActorKinds()},
		}
	}

	if !audit.ValidateDomain(req.Domain) {
		return nil, &Error{
			Code:    command.CodeValidation,
			Path:    "domain",
			Message: "domain is not a recognized activity domain.",
			Details: map[string]any{"allowed": audit.AllowedDomains()},
		}
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

	before, cerr := parseActivityCursor(req.Cursor, project)
	if cerr != nil {
		return nil, cerr
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	limit := clampLimit(req.Limit, defAuditLimit, maxAuditLimit)
	res := &ActivityListResult{Entries: make([]ActivityEntry, 0, limit)}
	names := map[string]string{}

	wantActor := req.ActorKind
	if wantActor == "all" {
		wantActor = ""
	}
	wantDomain := req.Domain
	if wantDomain == "all" {
		wantDomain = ""
	}

	batchSize := max(limit, defAuditLimit)
	for range maxAuditPasses {
		rows, lerr := s.store.Audit().ActivityPage(ctx, ActivityPageQuery{
			ProjectID: project, Owner: c.OwnerUserID, Before: before, Limit: batchSize + 1,
		})
		if lerr != nil {
			return nil, asError(lerr)
		}

		for i := range min(len(rows), batchSize) {
			row := &rows[i]
			kind, label := audit.ClassifyActor(row.Actor)
			domain := audit.ClassifyDomain(row.Action)
			if (wantActor != "" && kind != wantActor) || (wantDomain != "" && domain != wantDomain) {
				before = row.Key
				continue
			}

			if len(res.Entries) == limit {
				res.setCursor(project, before)
				return res, nil
			}

			before = row.Key
			res.Entries = append(res.Entries, s.activityEntry(ctx, row, kind, label, domain, names))
		}

		if len(rows) <= batchSize {
			return res, nil
		}
	}

	res.setCursor(project, before)
	return res, nil
}

func (s *svc) activityEntry(
	ctx context.Context, r *ActivityRow, actorKind, actorLabel, domain string, names map[string]string,
) ActivityEntry {
	key := r.Key
	e := ActivityEntry{
		ID: strings.Join(
			[]string{key.Source, key.Subject, strconv.FormatInt(key.Sequence, 10), key.ProjectID},
			":",
		),
		At:         time.UnixMilli(key.AtMs).UTC().Format(time.RFC3339Nano),
		Actor:      r.Actor,
		ActorKind:  actorKind,
		ActorLabel: actorLabel,
		Domain:     domain,
		Action:     r.Action,
		ProjectID:  key.ProjectID,
		SubjectID:  key.Subject,
		Details:    audit.SanitizeDetails(r.Details),
	}

	if pid := key.ProjectID; pid != "" {
		if name, ok := names[pid]; ok {
			e.ProjectName = name
		} else if row, err := s.store.Productions().Get(ctx, pid); err == nil {
			names[pid] = row.Name
			e.ProjectName = row.Name
		} else {
			names[pid] = ""
		}
	}

	if key.Source != "audit" {
		e.SubjectKind = key.Source
		e.Revision = key.Sequence
	}

	return e
}
