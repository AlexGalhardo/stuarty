package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The routes of the hand-off from an external motion engine (SDD-011 section 15). They carry no rule: the Service
// validates and core/motion holds what a brief is (ADR-0008).

type motionBriefIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	From       string `json:"from,omitempty"                            doc:"Start of the range, a time; default 0s."`
	Until      string `json:"until,omitempty"                           doc:"End of the range, exclusive; default the end of the timeline."`
	TrackID    string `json:"track_id,omitempty"                        doc:"The visual track the clip will go on; tells whether the render needs alpha."`
}

func (h *apiHandler) motionBrief(c *web.Ctx, in motionBriefIn) (service.MotionBriefResult, error) {
	res, err := h.Service.MotionBrief(c.Context(), service.MotionBriefRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, From: in.From, Until: in.Until, TrackID: in.TrackID,
	})

	return deref(res), err
}

type motionCheckIn struct {
	ID         string `json:"id"                    validate:"required"`
	AssetID    string `json:"asset_id"              validate:"required" doc:"The asset to check: an asset of the project, ready."`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	From       string `json:"from,omitempty"                            doc:"Start of the range, a time; default 0s."`
	Until      string `json:"until,omitempty"                           doc:"End of the range, exclusive; default the end of the timeline."`
	TrackID    string `json:"track_id,omitempty"                        doc:"The visual track the clip will go on; tells whether the render needs alpha."`
}

func (h *apiHandler) motionCheck(c *web.Ctx, in motionCheckIn) (service.MotionCheckResult, error) {
	res, err := h.Service.MotionCheck(c.Context(), service.MotionCheckRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, AssetID: in.AssetID, From: in.From, Until: in.Until,
		TrackID: in.TrackID,
	})

	return deref(res), err
}
