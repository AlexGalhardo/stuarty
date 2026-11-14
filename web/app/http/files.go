package httpapi

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

type jobFileIn struct {
	JobID string `json:"job_id" validate:"required"`
}

// jobsFile is GET /jobs/{job_id}/file: the file of a succeeded export, from
// exports/<project>/<job>.mp4 inside the workspace, by database lookup, with
// the Range support of http.ServeContent (SDD-003 section 6.5, SDD-007
// section 5.3). The path is built from the row, never from the request.
func (h *apiHandler) jobsFile(c *web.Ctx, in jobFileIn) (noIn, error) {
	caller, ok := service.CallerFrom(c.Context())
	if !ok || h.Jobs == nil {
		return noIn{}, &service.Error{Code: command.CodeNotFound, Message: "the job does not exist."}
	}

	job, err := h.Jobs.Get(c.Context(), in.JobID)
	if errors.Is(err, service.ErrNotFound) {
		return noIn{}, &service.Error{Code: command.CodeNotFound, Message: "the job does not exist."}
	}

	if err != nil {
		return noIn{}, err
	}

	if job.ProjectID == nil || (caller.ProjectID != "" && caller.ProjectID != *job.ProjectID) {
		return noIn{}, &service.Error{Code: service.CodeForbidden, Message: "the caller cannot read this job."}
	}

	if job.Kind != "export" || job.State != "succeeded" {
		return noIn{}, &service.Error{
			Code: command.CodeInvalidState, Message: "the job is not a succeeded export.",
			Hint: "wait for the job to succeed, then ask for the file",
		}
	}

	name := path.Join("exports", *job.ProjectID, job.ID+".mp4")
	if strings.ContainsAny(job.ID+*job.ProjectID, `/\`) {
		return noIn{}, &service.Error{Code: command.CodeInternal, Message: "the job record is not valid."}
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", "video/mp4")
	hd.Set("Content-Disposition", `attachment; filename="`+job.ID+`.mp4"`)
	hd.Set("Cache-Control", "private, max-age=31536000, immutable")
	hd.Set("ETag", exportETag(job))

	if err := c.File(os.DirFS(string(h.Workspace)), name); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}

// exportETag is "sha256-<first 16 hex of the file hash>" when the result
// records the hash, and the job id otherwise: the id pins the content too.
func exportETag(job service.JobRow) string {
	var res struct {
		Export struct {
			SHA256 string `json:"sha256"`
		} `json:"export"`
	}

	if json.Unmarshal(job.Result, &res) == nil && len(res.Export.SHA256) >= 16 {
		return `"sha256-` + res.Export.SHA256[:16] + `"`
	}

	return `"job-` + job.ID + `"`
}
