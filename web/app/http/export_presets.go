package httpapi

import (
	"net/http"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type exportPresetsIn struct {
	Codecs bool `json:"codecs,omitempty" doc:"true also tells which codecs this machine can write (runs the test encodes the first time)."`
}

type exportCheckIn struct {
	ID     string `json:"id"              validate:"required"`
	Preset string `json:"preset"          validate:"required" doc:"A preset id, as listed by GET /export-presets."`
	Start  string `json:"start,omitempty"                     doc:"The start of the range to export; empty is the start of the timeline."`
	End    string `json:"end,omitempty"                       doc:"The end of the range to export; empty is the end of the timeline."`
	Codec  string `json:"codec,omitempty"                     doc:"h264, hevc or av1: the codec of the estimate of the size; empty is the first of the preset."`
}

type exportBatchItemIn struct {
	Preset string `json:"preset,omitempty" doc:"A preset id; it needs the format mp4."`
	Format string `json:"format,omitempty" doc:"mp4 (default), gif, wav or aac."`
	Codec  string `json:"codec,omitempty"  doc:"h264, hevc or av1, for mp4; empty is the choice of the preset."`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

type exportBatchIn struct {
	ID     string              `json:"id"                validate:"required"`
	Items  []exportBatchItemIn `json:"items"             validate:"required,min=1,max=12" doc:"The exports, one after the other."`
	WaitMS int                 `json:"wait_ms,omitempty"                                  doc:"0 to 25000: hold the call until the job is final."`
}

func (h *apiHandler) exportPresetList(c *web.Ctx, in exportPresetsIn) (service.ExportPresetListResult, error) {
	res, err := h.Service.ExportPresetList(c.Context(), service.ExportPresetListRequest{Codecs: in.Codecs})
	if res != nil {
		res.Text = service.TextFor(service.SurfaceHTTP, res.Text)
	}

	return deref(res), err
}

func (h *apiHandler) exportCheck(c *web.Ctx, in exportCheckIn) (service.ExportCheckResult, error) {
	res, err := h.Service.ExportCheck(c.Context(), service.ExportCheckRequest{
		ProjectID: in.ID, Preset: in.Preset, Start: in.Start, End: in.End, Codec: in.Codec,
	})
	if res != nil {
		res.Text = service.TextFor(service.SurfaceHTTP, res.Text)
	}

	return deref(res), err
}

func (h *apiHandler) exportBatch(c *web.Ctx, in exportBatchIn) (service.Job, error) {
	items := make([]service.ExportBatchItem, len(in.Items))
	for i, it := range in.Items {
		items[i] = service.ExportBatchItem{
			Preset: it.Preset,
			Format: it.Format,
			Codec:  it.Codec,
			Start:  it.Start,
			End:    it.End,
		}
	}

	res, err := h.Service.ExportBatch(c.Context(), service.ExportBatchRequest{
		ProjectID: in.ID, Items: items, WaitMS: in.WaitMS,
	})
	if err != nil {
		return service.Job{}, err
	}

	status := http.StatusAccepted
	if service.JobFinal(res.State) {
		status = http.StatusOK
	}

	res.Text = service.TextFor(service.SurfaceHTTP, res.Text)

	return service.Job{}, c.JSON(status, res)
}
