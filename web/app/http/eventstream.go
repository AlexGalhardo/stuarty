package httpapi

import (
	"context"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/web/app/events"

	"github.com/AlexGalhardo/vukem/web"
)

type eventsIn struct {
	ID    string `json:"id"              validate:"required"`
	Since string `json:"since,omitempty"                     doc:"The revision the client holds; Last-Event-ID wins."`
}

// eventsStream is GET /projects/{id}/events (SDD-006 section 2). Everything that
// can fail is checked before the response becomes a stream, so a failure is a
// JSON error; once it is one, the events package owns it.
func (h *apiHandler) eventsStream(c *web.Ctx, in eventsIn) (noIn, error) {
	if h.Events == nil {
		return noIn{}, &service.Error{
			Code:    command.CodeInternal,
			Message: "the event stream is not available in this build.",
		}
	}

	since, err := events.ParseSince(c.Request().Header.Get("Last-Event-ID"), in.Since)
	if err != nil {
		return noIn{}, err
	}

	// The Service decides who may see the project, a trashed one included.
	check := func() error {
		_, err := h.Service.ProjectGet(c.Context(), service.ProjectGetRequest{ProjectID: in.ID})

		return err
	}

	if err := check(); err != nil {
		return noIn{}, err
	}

	return noIn{}, h.Events.Serve(c, in.ID, since, func() bool { return check() == nil })
}

// approvalsStream is GET /approvals/events (SDD-015 section 7): the pending requests, then the events
// of the queue. It needs the read scope; a key limited to one project has no use for a stream of the
// whole queue and is refused. The Service decides what the caller may see: the snapshot is its answer.
func (h *apiHandler) approvalsStream(c *web.Ctx, _ noIn) (noIn, error) {
	if h.Events == nil {
		return noIn{}, &service.Error{
			Code:    command.CodeInternal,
			Message: "the event stream is not available in this build.",
		}
	}

	caller, ok := service.CallerFrom(c.Context())
	if !ok {
		return noIn{}, &service.Error{Code: service.CodeUnauthorized, Message: "the request carries no credentials."}
	}

	if caller.ProjectID != "" {
		return noIn{}, &service.Error{
			Code:    service.CodeForbidden,
			Message: "the key is limited to one project and cannot stream the approval queue.",
			Hint:    "use a key without a project limit, or list the requests of the project with approval:list",
		}
	}

	owner := ""
	if caller.OwnerUserID != "" && !slices.Contains(caller.Scopes, service.ScopeAdmin) {
		owner = caller.OwnerUserID
	}

	pending := func(ctx context.Context) ([]approval.Request, error) {
		res, err := h.Service.ApprovalList(ctx, service.ApprovalListRequest{State: approval.StatePending, Limit: 200})
		if err != nil {
			return nil, err
		}

		return res.Requests, nil
	}

	return noIn{}, h.Events.ServeApprovals(c, owner, pending, func() bool {
		_, err := pending(c.Context())

		return err == nil
	})
}
