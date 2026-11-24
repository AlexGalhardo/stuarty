package httpapi

import (
	"context"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// inboxList is GET /inbox (SDD-015 section 4.1): what waits for the caller, from the chat and from the approval
// queue. It is a read; a request is decided on the endpoint of its source.
func (h *apiHandler) inboxList(c *web.Ctx, _ noIn) (service.InboxListResult, error) {
	res, err := h.Service.InboxList(c.Context(), service.InboxListRequest{})

	return deref(res), err
}

// inboxStream is GET /inbox/events: the list as the caller reads it, again each time it changes. The service decides
// what the caller may see: every frame is its answer, so the stream tells a member only what the member's own read
// tells.
func (h *apiHandler) inboxStream(c *web.Ctx, _ noIn) (noIn, error) {
	if h.Events == nil {
		return noIn{}, &service.Error{
			Code:    command.CodeInternal,
			Message: "the event stream is not available in this build.",
		}
	}

	read := func(ctx context.Context) (*service.InboxListResult, error) {
		return h.Service.InboxList(ctx, service.InboxListRequest{})
	}

	return noIn{}, h.Events.ServeInbox(c, read, func() bool {
		_, err := read(c.Context())

		return err == nil
	})
}
