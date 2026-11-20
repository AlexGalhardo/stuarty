package events

import (
	"github.com/AlexGalhardo/stuarty/service"
)

// PublicationUpdated is the event of the calendar (SDD-020 section 29). It is on a stream of its own, not on a
// project stream, and carries no id: the stream has no replay, a client lists the publications first.
const PublicationUpdated = "publication.updated"

// PublicationFrame is publication.updated: the data is the publication with its targets, as
// GET /publications/{id} answers it. The text of a post travels as a string member and never as markup.
func PublicationFrame(ev *service.Event) Frame {
	return marshal(ev.Type, ev.Publication)
}
