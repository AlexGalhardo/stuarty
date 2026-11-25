package httpapi

import (
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type captureStatusOut struct {
	Active  bool  `json:"active"   doc:"Whether a capture helper that was asked to record is alive."`
	SinceMS int64 `json:"since_ms" doc:"When the indicator went on, in Unix milliseconds; 0 while it is off."`
	Helpers int   `json:"helpers"  doc:"How many such helper processes are alive."`
}

// systemCapture is GET /api/v1/system/capture (SDD-025 section 19.4): the desktop shell asks it once a second and
// draws a system tray icon while it says a capture is on, with the window open or not. The answer is the count of
// live helper processes the host keeps, not the state of a recording, so nothing but the end of the process turns
// it off. It reads only, as the status of the scheduler does, and the shell runs its server in local mode only, so
// in network mode the route answers 404 to every caller.
func (h *apiHandler) systemCapture(_ *web.Ctx, _ noIn) (captureStatusOut, error) {
	if h.Network != nil {
		return captureStatusOut{}, &service.Error{
			Code:    command.CodeNotFound,
			Message: "this route belongs to the desktop shell and exists in local mode only.",
		}
	}

	act, ok := service.CaptureActivityOf(h.Service)
	if !ok {
		return captureStatusOut{}, &service.Error{
			Code:    command.CodeNotFound,
			Message: "this server has no capture.",
		}
	}

	return captureStatusOut{Active: act.Active, SinceMS: act.SinceMS, Helpers: act.Helpers}, nil
}
