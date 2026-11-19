package httpapi

import (
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
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
