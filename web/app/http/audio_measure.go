package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type audioMeasureIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	ItemID     string `json:"item_id,omitempty"                         doc:"Measure this clip or audio item alone, with the chain of its track; its own range is the default range."`
	Start      string `json:"start,omitempty"                           doc:"The start of the range; empty is the start of the item, or of the timeline."`
	End        string `json:"end,omitempty"                             doc:"The end of the range; empty is the end of the item, or of the timeline."`
}

func (h *apiHandler) audioMeasure(c *web.Ctx, in audioMeasureIn) (service.AudioMeasureResult, error) {
	res, err := h.Service.AudioMeasure(c.Context(), service.AudioMeasureRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, ItemID: in.ItemID, Start: in.Start, End: in.End,
	})
	if res != nil {
		res.Text = service.TextFor(service.SurfaceHTTP, res.Text)
	}

	return deref(res), err
}
