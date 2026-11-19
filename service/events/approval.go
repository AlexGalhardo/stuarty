package events

import (
	"github.com/AlexGalhardo/stuarty/service"
)

// The events of the approval queue (SDD-015 section 7). They are on a stream of their own, not on a
// project stream, and carry no id: the stream has no replay, a client that connects is sent the
// requests that are pending.
const (
	ApprovalRequested = "approval.requested"
	ApprovalDecided   = "approval.decided"
)

// ApprovalFrame is approval.requested or approval.decided: the data is the request itself, as
// GET /approvals/{id} answers it. The summary is text a job kind or an agent wrote; it travels as a
// string member and never as markup.
func ApprovalFrame(ev *service.Event) Frame {
	return marshal(ev.Type, ev.Approval)
}
