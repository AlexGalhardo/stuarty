package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type importFileIn struct {
	Path    string `json:"path,omitempty"     doc:"An absolute path on the machine that runs the server; or a hash."`
	Hash    string `json:"hash,omitempty"     doc:"sha256: and 64 hex digits of a file the library holds, in place of a path: the project uses it with no copy."`
	AssetID string `json:"asset_id,omitempty" doc:"An id of your own; ignored for a duplicate."`
}

type mediaImportIn struct {
	ID         string             `json:"id"                    validate:"required"`
	TimelineID string             `json:"timeline_id,omitempty"                                  doc:"The timeline of the project; required when the project has several."`
	Files      []importFileIn     `json:"files"                 validate:"required,min=1,max=50"`
	WaitMS     int                `json:"wait_ms,omitempty"                                      doc:"0 to 25000: hold the call until the assets are ready."`
	DryRun     any                `json:"dry_run,omitempty"                                      doc:"Checks the paths and limits only: the format is probed on the real import, so a file that is refused there (SVG, for one) still answers 202."`
	Origin     *asset.Origin      `json:"origin,omitempty"                                       doc:"Where the files came from: kind imported (default), generated (provider and model), fetched_by_agent (source and rights_note) or rendered_externally (source)."`
	Info       *service.AssetInfo `json:"info,omitempty"                                         doc:"Tags, the reuse mark and a description for the files of the call."`
}

type mediaListIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	pageIn
}

type mediaInspectIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	AssetID    string `json:"asset_id"              validate:"required"`
	Sections   string `json:"sections,omitempty"                        doc:"Comma separated: probe (default), transcript, silences, loudness, beats."`
	MinSilence string `json:"min_silence,omitempty"                     doc:"The shortest silence to list, as a time; default 0.4s."`
	pageIn
}

type mediaFileIn struct {
	ID      string `json:"id"       validate:"required"`
	AssetID string `json:"asset_id" validate:"required"`
}

type thumbnailIn struct {
	ID      string `json:"id"              validate:"required"`
	AssetID string `json:"asset_id"        validate:"required"`
	Width   string `json:"width,omitempty"                     doc:"160, 320 (default) or 640."`
}

type jobStartIn struct {
	ID         string         `json:"id"                    validate:"required"`
	TimelineID string         `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Kind       string         `json:"kind"                  validate:"required" doc:"export."`
	Args       map[string]any `json:"args,omitempty"                            doc:"The arguments of the kind: start, end, format (mp4, gif, wav, aac or png; default mp4), preset and codec (h264, hevc or av1, for mp4) for an export."`
	WaitMS     int            `json:"wait_ms,omitempty"                         doc:"0 to 25000: hold the call until the job is final."`
	DryRun     any            `json:"dry_run,omitempty"`
}

type jobListIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	State      string `json:"state,omitempty"                           doc:"queued, running, succeeded, failed, cancelled, active or all."`
	pageIn
}

type jobGetIn struct {
	JobID  string `json:"job_id"            validate:"required"`
	WaitMS int    `json:"wait_ms,omitempty"                     doc:"0 to 25000: hold the call until the job is final."`
}

func (h *apiHandler) mediaImport(c *web.Ctx, in mediaImportIn) (service.MediaImportResult, error) {
	files := make([]service.ImportFile, len(in.Files))
	for i, f := range in.Files {
		files[i] = service.ImportFile{Path: f.Path, Hash: f.Hash, AssetID: f.AssetID}
	}

	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.MediaImportResult{}, err
	}

	res, err := h.Service.MediaImport(c.Context(), service.MediaImportRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, Files: files, WaitMS: in.WaitMS, DryRun: dryRun,
		Origin: in.Origin, Info: in.Info,
	})
	if err != nil {
		return service.MediaImportResult{}, err
	}

	// 202 when every file was accepted, 200 when some were refused (SDD-003
	// section 2.6); the status depends on the answer, so the handler writes it.
	status := http.StatusAccepted
	if res.Failed > 0 {
		status = http.StatusOK
	}

	return service.MediaImportResult{}, c.JSON(status, res)
}

func (h *apiHandler) mediaList(c *web.Ctx, in mediaListIn) (service.MediaListResult, error) {
	res, err := h.Service.MediaList(
		c.Context(),
		service.MediaListRequest{ProjectID: in.ID, TimelineID: in.TimelineID, Page: page(in.pageIn)},
	)

	return deref(res), err
}

func (h *apiHandler) mediaInspect(c *web.Ctx, in mediaInspectIn) (service.MediaInspectResult, error) {
	var sections []string

	for s := range strings.SplitSeq(in.Sections, ",") {
		if s = strings.TrimSpace(s); s != "" {
			sections = append(sections, s)
		}
	}

	res, err := h.Service.MediaInspect(c.Context(), service.MediaInspectRequest{
		ProjectID:  in.ID,
		TimelineID: in.TimelineID,
		AssetID:    in.AssetID,
		Sections:   sections,
		MinSilence: in.MinSilence,
		Page:       page(in.pageIn),
	})

	return deref(res), err
}

func (h *apiHandler) jobStart(c *web.Ctx, in jobStartIn) (service.Job, error) {
	var args []byte

	if in.Args != nil {
		var err error
		if args, err = json.Marshal(in.Args); err != nil {
			return service.Job{}, &service.Error{
				Code:    command.CodeValidation,
				Path:    "args",
				Message: "the arguments are not valid JSON.",
			}
		}
	}

	isDry, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.Job{}, err
	}

	res, err := h.Service.JobStart(c.Context(), service.JobStartRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, Kind: in.Kind, Args: args, WaitMS: in.WaitMS, DryRun: isDry,
	})
	if err != nil {
		return service.Job{}, err
	}

	status := http.StatusAccepted
	if service.JobFinal(res.State) || isDry {
		status = http.StatusOK
	}

	res.Text = service.TextFor(service.SurfaceHTTP, res.Text)

	return service.Job{}, c.JSON(status, res)
}

func (h *apiHandler) jobGet(c *web.Ctx, in jobGetIn) (service.Job, error) {
	res, err := h.Service.JobGet(c.Context(), service.JobGetRequest{JobID: in.JobID, WaitMS: in.WaitMS})
	if res != nil {
		res.Text = service.TextFor(service.SurfaceHTTP, res.Text)
	}

	return deref(res), err
}

type jobCancelIn struct {
	JobID string `json:"job_id" validate:"required"`
}

func (h *apiHandler) jobCancel(c *web.Ctx, in jobCancelIn) (service.Job, error) {
	res, err := h.Service.JobCancel(c.Context(), service.JobCancelRequest{JobID: in.JobID})

	return deref(res), err
}

func (h *apiHandler) jobList(c *web.Ctx, in jobListIn) (service.JobListResult, error) {
	res, err := h.Service.JobList(c.Context(), service.JobListRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, State: in.State, Page: page(in.pageIn),
	})

	return deref(res), err
}

// serveAsset is the byte-serving route of an artifact of an asset (SDD-003
// section 6.5): the path comes from the database through OpenFile, never from the
// request, and the file system handed to c.File is the workspace, so a path cannot leave it.
func (h *apiHandler) serveAsset(c *web.Ctx, kind, projectID, assetID, variant string) (noIn, error) {
	f, err := h.Service.OpenFile(
		c.Context(),
		service.FileRef{Kind: kind, ProjectID: projectID, ID: assetID, Variant: variant},
	)
	if err != nil {
		return noIn{}, err
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", f.ContentType)
	hd.Set("ETag", f.ETag)

	maxAge := f.MaxAge
	if maxAge <= 0 {
		maxAge = 86400
	}

	cache := "private, max-age=" + strconv.Itoa(maxAge)
	if f.Immutable {
		cache += ", immutable"
	}

	hd.Set("Cache-Control", cache)

	if f.Body != nil {
		http.ServeContent(c.Writer(), c.Request(), "", f.ModTime, bytes.NewReader(f.Body))

		return noIn{}, nil
	}

	if err := c.File(os.DirFS(string(h.Workspace)), f.Path); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}

type templateListIn struct{ pageIn }

type templateSaveIn struct {
	ProjectID   string `json:"project_id"            validate:"required"`
	TimelineID  string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Name        string `json:"name"                  validate:"required"`
	Description string `json:"description,omitempty"`
	DryRun      any    `json:"dry_run,omitempty"`
}

type templateInstantiateIn struct {
	TemplateID string                       `json:"template_id"       validate:"required"`
	Name       string                       `json:"name"              validate:"required"`
	Fill       map[string]service.SlotValue `json:"fill,omitempty"                        doc:"Slot name to a value: one of asset_id, text or words."`
	ID         string                       `json:"id,omitempty"                          doc:"Optional project id; local mode only."`
	DryRun     any                          `json:"dry_run,omitempty"`

	BrandColors []string `json:"brand_colors,omitempty" doc:"Names of template colours that take the colour of the brand kit."`
	BrandFonts  []string `json:"brand_fonts,omitempty"  doc:"Families of the brand kit added to the fonts of the project."`
}

func (h *apiHandler) templateList(c *web.Ctx, in templateListIn) (service.TemplateListResult, error) {
	res, err := h.Service.TemplateList(c.Context(), service.TemplateListRequest{Page: page(in.pageIn)})

	return deref(res), err
}

func (h *apiHandler) templateSave(c *web.Ctx, in templateSaveIn) (service.TemplateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TemplateResult{}, err
	}

	res, err := h.Service.TemplateSave(c.Context(), service.TemplateSaveRequest{
		ProjectID: in.ProjectID, TimelineID: in.TimelineID, Name: in.Name, Description: in.Description, DryRun: dryRun,
	})
	if err != nil {
		return service.TemplateResult{}, err
	}

	if dryRun {
		return service.TemplateResult{}, c.JSON(http.StatusOK, res)
	}

	c.SetHeader("Location", BasePath+"/templates")

	return service.TemplateResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) templateInstantiate(
	c *web.Ctx,
	in templateInstantiateIn,
) (service.TemplateInstantiateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TemplateInstantiateResult{}, err
	}

	res, err := h.Service.TemplateInstantiate(c.Context(), service.TemplateInstantiateRequest{
		TemplateID: in.TemplateID, Name: in.Name, Fill: in.Fill, ID: in.ID, DryRun: dryRun,
		BrandColors: in.BrandColors, BrandFonts: in.BrandFonts,
	})
	if err != nil {
		return service.TemplateInstantiateResult{}, err
	}

	if dryRun {
		return service.TemplateInstantiateResult{}, c.JSON(http.StatusOK, res)
	}

	c.SetHeader("Location", BasePath+"/projects/"+url.PathEscape(res.ProjectID))

	return service.TemplateInstantiateResult{}, c.JSON(http.StatusCreated, res)
}

type templateImportIn struct {
	Path   string `json:"path"              validate:"required" doc:"An absolute path to a template file on the machine that runs the server."`
	DryRun any    `json:"dry_run,omitempty"`
}

type templateRefIn struct {
	TemplateID string `json:"template_id" validate:"required"`
}

func (h *apiHandler) templateImport(c *web.Ctx, in templateImportIn) (service.TemplateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TemplateResult{}, err
	}

	res, err := h.Service.TemplateImport(c.Context(), service.TemplateImportRequest{Path: in.Path, DryRun: dryRun})
	if err != nil {
		return service.TemplateResult{}, err
	}

	if dryRun {
		return service.TemplateResult{}, c.JSON(http.StatusOK, res)
	}

	return service.TemplateResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) templateExport(c *web.Ctx, in templateRefIn) (service.FileResult, error) {
	res, err := h.Service.TemplateExport(c.Context(), service.TemplateRef{TemplateID: in.TemplateID})

	return deref(res), err
}

// templateFile serves the template file of SDD-003 section 6.5: a zip, always
// an attachment named after the template id (the template name is untrusted and
// never reaches a header).
func (h *apiHandler) templateFile(c *web.Ctx, in templateRefIn) (noIn, error) {
	f, err := h.Service.OpenFile(c.Context(), service.FileRef{Kind: "template", ID: in.TemplateID})
	if err != nil {
		return noIn{}, err
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", f.ContentType)
	hd.Set("ETag", f.ETag)
	hd.Set("Cache-Control", "private, no-cache")
	hd.Set("Content-Disposition", `attachment; filename="`+in.TemplateID+`.stuarty-template.zip"`)

	if err := c.File(os.DirFS(string(h.Workspace)), f.Path); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}

type trashEmptyIn struct {
	ProjectID     string `json:"project_id,omitempty"      doc:"Empty only this trashed project."`
	OlderThanDays int    `json:"older_than_days,omitempty" doc:"Empty only what has been in the trash this many days."`
}

// trashEmpty is the only route that deletes data; the Service checks that the
// caller is a browser session or an admin key.
func (h *apiHandler) trashEmpty(c *web.Ctx, in trashEmptyIn) (service.TrashEmptyResult, error) {
	res, err := h.Service.TrashEmpty(c.Context(), service.TrashEmptyRequest{
		ProjectID: in.ProjectID, OlderThanDays: in.OlderThanDays,
	})

	return deref(res), err
}

type toolsInstallIn struct {
	Name  string `json:"name"            validate:"required" doc:"The tool to install, for example whisper."`
	Model string `json:"model,omitempty"                     doc:"The model, one the manifest of this build pins; default small."`
	Force bool   `json:"force,omitempty"                     doc:"Download again what is already installed and verified."`
}

type toolsStatusIn struct{}

// toolsInstall starts the download and answers 202 with what is coming. Only a
// browser session or an admin key passes the Service check.
func (h *apiHandler) toolsInstall(c *web.Ctx, in toolsInstallIn) (service.ToolsInstallResult, error) {
	res, err := h.Service.ToolsInstall(
		c.Context(), service.ToolsInstallRequest{Name: in.Name, Model: in.Model, Force: in.Force})
	if err != nil {
		return service.ToolsInstallResult{}, err
	}

	return service.ToolsInstallResult{}, c.JSON(http.StatusAccepted, res)
}

func (h *apiHandler) toolsStatus(c *web.Ctx, _ toolsStatusIn) (service.ToolsStatusResult, error) {
	res, err := h.Service.ToolsStatus(c.Context(), service.ToolsStatusRequest{})

	return deref(res), err
}
