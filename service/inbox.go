package service

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/core/inbox"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// InboxService is the one list of what waits for a person (SDD-015 section 4.1, epic 77, D-137). It is
// a read over the permission requests of the hosted agent chat and the approval queue; it decides
// nothing, and a row is answered by ChatAnswerPermission or ApprovalDecide.
type InboxService interface {
	InboxList(ctx context.Context, req InboxListRequest) (*InboxListResult, error)
}

// InboxListRequest has no member.
type InboxListRequest struct{}

// InboxListResult is every request that waits for the caller and the ones answered last, newest first
// (inbox.Order). Waiting counts the rows that wait. The text members of a row are untrusted.
type InboxListResult struct {
	Envelope
	Rows    []inbox.Row `json:"rows"`
	Waiting int         `json:"waiting"`
}

func inboxSessionError() *Error {
	return newError(CodePrivilegedSession, "the requests that wait for a person are not for an agent.").
		withHint("a person reads them in the notification center of the Stuarty app, or with inbox:list; " +
			"the session token of a hosted agent cannot")
}

// InboxList returns what waits for the caller. The caller class is checked first, so that an agent
// learns one thing: the session token of a hosted agent is refused before its scope is looked at. Each
// source then shows exactly what it shows on its own (the project check of the session, SDD-017 section
// 10, and the owner of an approval, SDD-015 section 4), never more.
func (s *svc) InboxList(ctx context.Context, _ InboxListRequest) (*InboxListResult, error) {
	if err := s.locked("InboxList"); err != nil {
		return nil, err
	}

	if c, ok := CallerFrom(ctx); ok && c.ChatSessionID != "" {
		return nil, inboxSessionError()
	}

	c, err := s.authorize(ctx, "InboxList", "")
	if err != nil {
		return nil, err
	}

	rows := s.inboxChat(ctx, c)

	approvals, err := s.inboxApprovals(ctx, c)
	if err != nil {
		return nil, err
	}

	rows = inbox.Order(append(rows, approvals...))

	res := &InboxListResult{Rows: rows}

	for i := range rows {
		if rows[i].State == inbox.StateWaiting {
			res.Waiting++
		}
	}

	if len(rows) > 0 {
		res.UntrustedNotice = UntrustedNotice
	}

	return res, nil
}

// inboxChat is the chat half: the permission requests of the live sessions of the projects the caller
// may see. A project the caller may not see, or that a key is not limited to, contributes nothing, as
// ChatSessionGet answers for it. A host-less process lists none.
func (s *svc) inboxChat(ctx context.Context, c Caller) []inbox.Row {
	if s.agents == nil {
		return nil
	}

	seen := map[string]bool{}

	var rows []inbox.Row

	permissions := s.agents.Permissions()

	for i := range permissions {
		p := &permissions[i]

		may, known := seen[p.ProjectID]
		if !known {
			may = s.mayReadChatProject(ctx, c, p.ProjectID)
			seen[p.ProjectID] = may
		}

		if !may {
			continue
		}

		rows = append(rows, inbox.FromPermission(permissionOf(p)))
	}

	return rows
}

func (s *svc) mayReadChatProject(ctx context.Context, c Caller, projectID string) bool {
	if c.ProjectID != "" && c.ProjectID != projectID {
		return false
	}

	head, err := s.store.Productions().Get(ctx, projectID)

	return err == nil && visible(c, head.OwnerUserID)
}

func permissionOf(p *agenthost.Permission) *inbox.Permission {
	return &inbox.Permission{
		SessionID: p.SessionID, ProjectID: p.ProjectID, Profile: p.Profile, RequestID: p.RequestID, Title: p.Title,
		ToolKind: p.ToolKind, Detail: p.Detail, Skill: p.Skill, Answer: p.Answer, AskedAt: p.AskedAt,
		AnsweredAt: p.AnsweredAt,
	}
}

// inboxApprovals is the approval half: the pending requests and the last of each settled state, read
// with the filter of ApprovalList, so a caller sees what ApprovalList shows it.
func (s *svc) inboxApprovals(ctx context.Context, c Caller) ([]inbox.Row, error) {
	s.expireDue(ctx)

	var rows []inbox.Row

	for _, state := range []string{
		approval.StatePending, approval.StateApproved, approval.StateRejected, approval.StateExpired,
	} {
		limit := inbox.MaxRecent
		if state == approval.StatePending {
			limit = maxApprovalLimit
		}

		found, _, err := s.store.Approvals().ListRequests(
			ctx, s.approvalFilter(c, approval.Filter{State: state, Limit: limit}),
		)
		if err != nil {
			return nil, asError(err)
		}

		for i := range found {
			rows = append(rows, inbox.FromApproval(approvalOf(&found[i])))
		}
	}

	return rows, nil
}

func approvalOf(r *approval.Row) *inbox.Approval {
	a := &inbox.Approval{
		ID:           r.ID,
		Actor:        r.Actor,
		Kind:         r.Kind,
		Summary:      r.Summary,
		ConnectionID: r.ConnectionID,
		ProjectID:    r.ProjectID,
		Estimate:     r.Estimate,
		State:        r.State,
		CreatedAt:    time.UnixMilli(r.CreatedMs).UTC(),
	}

	if r.DecidedMs != nil {
		t := time.UnixMilli(*r.DecidedMs).UTC()
		a.DecidedAt = &t
	}

	return a
}
