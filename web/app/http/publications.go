package httpapi

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"

	"github.com/AlexGalhardo/vukem/web"
)

type publicationCreateIn struct {
	ProjectID string            `json:"project_id"          validate:"required" doc:"The project the publication belongs to."`
	Title     string            `json:"title"               validate:"required" doc:"The title the targets use unless they have their own, 1 to 200 characters."`
	Text      string            `json:"text,omitempty"                          doc:"The caption the targets use unless they have their own description."`
	Export    string            `json:"export,omitempty"                        doc:"The id of a succeeded export job of the project: the video. Give this or post."`
	Thumbnail string            `json:"thumbnail,omitempty"                     doc:"The id of a succeeded export job of a still of the project: the cover."`
	Post      string            `json:"post,omitempty"                          doc:"The id of a text of the kind post of the project: a blog post, which goes to the blog destinations. Give this or export."`
	Targets   []pub.TargetInput `json:"targets,omitempty"                       doc:"One entry per account the post goes to: connection_id, fields (title, description, tags, visibility, made_for_kids, ai_disclosure), at (a time such as 2026-11-20T09:00 read in zone, or an instant with an offset), zone (an IANA name such as America/Sao_Paulo), late_window_minutes."`
}

type publicationListIn struct {
	ProjectID string `json:"project_id,omitempty" doc:"Only the publications of one project."`
	State     string `json:"state,omitempty"      doc:"Only publications with a target in this state: draft, waiting_approval, approved, scheduled, publishing, published, sent_as_draft, failed, missed or cancelled."`
	Network   string `json:"network,omitempty"    doc:"Only publications with a target on this network."`
	From      string `json:"from,omitempty"       doc:"Only publications with a target at or after this time (RFC 3339) or date."`
	To        string `json:"to,omitempty"         doc:"Only publications with a target before this time (RFC 3339) or date."`
	Zone      string `json:"zone,omitempty"       doc:"The zone a date or local time of from and to is read in; empty is UTC."`
	Limit     int    `json:"limit,omitempty"      doc:"The page size, 1 to 500; the default is 100."`
	Offset    int    `json:"offset,omitempty"     doc:"How many publications to skip."`
}

type publicationRefIn struct {
	ID string `json:"id" validate:"required" doc:"A publication id, pub_ and 10 characters."`
}

type publicationUpdateIn struct {
	ID        string            `json:"id"                  validate:"required" doc:"A publication id, pub_ and 10 characters."`
	Title     *string           `json:"title,omitempty"                         doc:"The new title; leave out to keep it."`
	Text      *string           `json:"text,omitempty"                          doc:"The new caption; leave out to keep it."`
	Export    *string           `json:"export,omitempty"                        doc:"The new video; leave out to keep it."`
	Thumbnail *string           `json:"thumbnail,omitempty"                     doc:"The new cover; an empty text removes it."`
	Post      *string           `json:"post,omitempty"                          doc:"The new post; leave out to keep it."`
	Targets   []pub.TargetInput `json:"targets,omitempty"                       doc:"Targets to change (with their id), add (without one) or remove (remove: true). A target that was waiting for approval, approved or scheduled and whose post changed goes back to waiting for approval."`
}

type publicationScheduleIn struct {
	ID                string `json:"id"                            validate:"required" doc:"A publication id, pub_ and 10 characters."`
	TargetID          string `json:"target_id,omitempty"                               doc:"The target; leave out to schedule every draft target that has a time."`
	At                string `json:"at,omitempty"                                      doc:"The time: a date and time such as 2026-11-20T09:00 read in zone, or an instant with an offset or Z. Leave out to keep the time the target has."`
	Zone              string `json:"zone,omitempty"                                    doc:"The IANA zone the time is read in, for example America/Sao_Paulo; the default is UTC."`
	LateWindowMinutes *int   `json:"late_window_minutes,omitempty"                     doc:"0 to 1440: how late after its time the post may still be sent without asking again. The default is 0: a post whose time passed is never sent on its own."`
}

type publicationTargetIn struct {
	ID       string `json:"id"                  validate:"required" doc:"A publication id, pub_ and 10 characters."`
	TargetID string `json:"target_id,omitempty"                     doc:"The target, tgt_ and 10 characters."`
}

func (h *apiHandler) publicationsCreate(c *web.Ctx, in publicationCreateIn) (service.Publication, error) {
	res, err := h.Service.PublicationCreate(c.Context(), service.PublicationCreateRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsList(c *web.Ctx, in publicationListIn) (service.PublicationListResult, error) {
	res, err := h.Service.PublicationList(c.Context(), service.PublicationListRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsGet(c *web.Ctx, in publicationRefIn) (service.Publication, error) {
	res, err := h.Service.PublicationGet(c.Context(), service.PublicationGetRequest{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) publicationsUpdate(c *web.Ctx, in publicationUpdateIn) (service.Publication, error) {
	res, err := h.Service.PublicationUpdate(c.Context(), service.PublicationUpdateRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsSchedule(c *web.Ctx, in publicationScheduleIn) (service.Publication, error) {
	res, err := h.Service.PublicationSchedule(c.Context(), service.PublicationScheduleRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsSendNow(c *web.Ctx, in publicationTargetIn) (service.Publication, error) {
	res, err := h.Service.PublicationSendNow(c.Context(), service.PublicationSendNowRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsRetry(c *web.Ctx, in publicationTargetIn) (service.Publication, error) {
	res, err := h.Service.PublicationRetry(c.Context(), service.PublicationRetryRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsCancel(c *web.Ctx, in publicationTargetIn) (service.Publication, error) {
	res, err := h.Service.PublicationCancel(c.Context(), service.PublicationCancelRequest(in))

	return deref(res), err
}

// publicationsStream is GET /publications/events (SDD-020 section 29): publication.updated for every change
// of a publication the caller may see. It has no replay: a client lists first and listens after. A key limited
// to one project has no use for a stream of the whole calendar and is refused.
func (h *apiHandler) publicationsStream(c *web.Ctx, _ noIn) (noIn, error) {
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
			Message: "the key is limited to one project and cannot stream the whole calendar.",
			Hint:    "use a key without a project limit, or list the publications of the project with publication:list",
		}
	}

	owner := ""
	if caller.OwnerUserID != "" && !slices.Contains(caller.Scopes, service.ScopeAdmin) {
		owner = caller.OwnerUserID
	}

	return noIn{}, h.Events.ServePublications(c, owner, func() bool {
		_, err := h.Service.PublicationList(c.Context(), service.PublicationListRequest{Limit: 1})

		return err == nil
	})
}
