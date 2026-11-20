package httpapi

import (
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"

	"github.com/AlexGalhardo/vukem/web"
)

type schedulerIn struct {
	Action string `json:"action" validate:"required,oneof=status pause resume" doc:"status, pause or resume."`
}

type schedulerOut struct {
	Paused    bool `json:"paused"    doc:"Whether publishing is paused."`
	Scheduled int  `json:"scheduled" doc:"The targets that still have to go out: approved, scheduled and publishing."`
}

// systemScheduler is POST /api/v1/system/scheduler (SDD-020 section 27): the desktop shell reads the
// state of the scheduler, with the number of targets that wait, and pauses or resumes it from its tray.
// Reading takes the admin token; pausing and resuming also take the launcher secret (SDD-007 section 5.3),
// so an agent that holds the token can see the state and never stops the posts of the person. The desktop
// shell runs its server in local mode only, so in network mode the route answers 404 to every caller.
func (h *apiHandler) systemScheduler(c *web.Ctx, in schedulerIn) (schedulerOut, error) {
	if h.Network != nil {
		return schedulerOut{}, &service.Error{
			Code:    command.CodeNotFound,
			Message: "this route belongs to the desktop shell and exists in local mode only.",
		}
	}

	sched, ok := service.SchedulerOf(h.Service)
	if !ok {
		return schedulerOut{}, &service.Error{
			Code:    command.CodeNotFound,
			Message: "this server has no publication scheduler.",
		}
	}

	if in.Action != "status" &&
		!auth.IsLauncher(h.LauncherSecret, c.Request().Header.Get(auth.LauncherHeader)) {
		return schedulerOut{}, &service.Error{
			Code:    service.CodeForbidden,
			Message: "only the desktop shell pauses or resumes publishing.",
		}
	}

	switch in.Action {
	case "pause":
		sched.SetPaused(true)
	case "resume":
		sched.SetPaused(false)
	}

	n, err := sched.Waiting(c.Context())
	if err != nil {
		return schedulerOut{}, err
	}

	return schedulerOut{Paused: sched.Paused(), Scheduled: n}, nil
}
