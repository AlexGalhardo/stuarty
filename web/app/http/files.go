package httpapi

import (
	"os"
	"path"
	"strconv"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
)

type jobFileIn struct {
	JobID string `json:"job_id" validate:"required"`
}

// jobsFile is GET /jobs/{job_id}/file: the file of a succeeded export, from
// exports/<project>/<job>.<ext> inside the workspace, with the Range support of
// http.ServeContent (SDD-003 section 6.5, SDD-007 section 5.3). The Service
// resolves the path from the job row and checks who may read it; the request
// never names a path.
func (h *apiHandler) jobsFile(c *web.Ctx, in jobFileIn) (noIn, error) {
	f, err := h.Service.ExportFile(c.Context(), service.ExportFileRequest{JobID: in.JobID})
	if err != nil {
		return noIn{}, err
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", f.ContentType)

	name := f.DownloadName
	if name == "" {
		name = in.JobID + path.Ext(f.Path)
	}

	hd.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	hd.Set("Cache-Control", "private, max-age="+strconv.Itoa(f.MaxAge)+", immutable")
	hd.Set("ETag", f.ETag)

	if err := c.File(os.DirFS(string(h.Workspace)), f.Path); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}
