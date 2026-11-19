package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type otioExportIn struct {
	ID string `json:"id" validate:"required"`
}

type otioImportIn struct {
	ID           string `json:"id"                      validate:"required"`
	BaseRevision int64  `json:"base_revision"                               doc:"The revision the import is applied to."`
	Path         string `json:"path"                    validate:"required" doc:"An absolute path to an .otio file on the machine that runs the server."`
	DryRun       any    `json:"dry_run,omitempty"`
	AllowPartial bool   `json:"allow_partial,omitempty"                     doc:"Import what the project can take when media is missing; default: refuse."`
}

func (h *apiHandler) projectOtioExport(c *web.Ctx, in otioExportIn) (service.ProjectOtioExportResult, error) {
	res, err := h.Service.ProjectOtioExport(c.Context(), service.ProjectOtioExportRequest{ProjectID: in.ID})
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}

func (h *apiHandler) projectOtioImport(c *web.Ctx, in otioImportIn) (service.ProjectOtioImportResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectOtioImportResult{}, err
	}

	res, err := h.Service.ProjectOtioImport(c.Context(), service.ProjectOtioImportRequest{
		ProjectID: in.ID, BaseRevision: in.BaseRevision, Path: in.Path, DryRun: dryRun,
		AllowPartial: in.AllowPartial,
	})
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}
