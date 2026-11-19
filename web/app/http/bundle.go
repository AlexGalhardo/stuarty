package httpapi

import (
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type bundleExportIn struct {
	ID         string `json:"id"                    validate:"required"`
	IncludeLog *bool  `json:"include_log,omitempty"                     doc:"false leaves the history out, to share a project without it. Default true."`
	WaitMS     int    `json:"wait_ms,omitempty"                         doc:"0 to 25000: hold the call until the job is final."`
}

type bundleImportIn struct {
	Path   string `json:"path"              validate:"required" doc:"An absolute path to a .vvbundle on the machine that runs the server."`
	AsNew  bool   `json:"as_new,omitempty"                      doc:"true makes a new project id and leaves the history out; false keeps the id."`
	DryRun any    `json:"dry_run,omitempty"`
}

func (h *apiHandler) projectBundleExport(c *web.Ctx, in bundleExportIn) (service.Job, error) {
	res, err := h.Service.ProjectBundleExport(c.Context(), service.ProjectBundleExportRequest{
		ProjectID: in.ID, IncludeLog: in.IncludeLog, WaitMS: in.WaitMS,
	})
	if err != nil {
		return service.Job{}, err
	}

	status := http.StatusAccepted
	if service.JobFinal(res.State) {
		status = http.StatusOK
	}

	return service.Job{}, c.JSON(status, res)
}

func (h *apiHandler) projectBundleImport(c *web.Ctx, in bundleImportIn) (service.ProjectCreateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	res, err := h.Service.ProjectBundleImport(c.Context(), service.ProjectBundleImportRequest{
		Path: in.Path, AsNew: in.AsNew, DryRun: dryRun,
	})
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	if dryRun {
		return service.ProjectCreateResult{}, c.JSON(http.StatusOK, res)
	}

	c.SetHeader("Location", BasePath+"/projects/"+url.PathEscape(res.ProjectID))

	return service.ProjectCreateResult{}, c.JSON(http.StatusCreated, res)
}
