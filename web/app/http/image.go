package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The image documents of SDD-021: the HTTP face of the seven image operations of the Service. Each handler
// parses its input, calls one method and formats the result; the rules are in core/image.

type imageCreateIn struct {
	Name       string                         `json:"name,omitempty"        doc:"1 to 120 characters; default Untitled."`
	Preset     string                         `json:"preset,omitempty"      doc:"video_thumbnail, square_post or vertical_cover."`
	Width      int                            `json:"width,omitempty"       doc:"Pixels, 1 to 4096, with height."`
	Height     int                            `json:"height,omitempty"      doc:"Pixels, 1 to 4096, with width."`
	Background string                         `json:"background,omitempty"  doc:"#RRGGBB, #RRGGBBAA or transparent; default #FFFFFF."`
	Template   bool                           `json:"template,omitempty"    doc:"Make a template whose text and image layers may be slots."`
	TemplateID string                         `json:"template_id,omitempty" doc:"Make the image from this template."`
	Values     map[string]coreimage.SlotValue `json:"values,omitempty"      doc:"Slot name to {text} or {asset}, with template_id."`
	ID         string                         `json:"id,omitempty"          doc:"An id of your own; E_ID_EXISTS when taken."`
}

type imageListIn struct {
	Templates string `json:"templates,omitempty" doc:"only for the templates, none for the documents that are not; default both."`
	pageIn
}

type imageIDIn struct {
	ID string `json:"id" validate:"required"`
}

// ImageOperation is one operation of an image batch: the command name and its arguments. The OpenAPI document
// replaces args by the schema of each command of the image registry.
type ImageOperation struct {
	Op   string         `json:"op"             validate:"required" doc:"A command name, as listed by GET /commands?domain=image."`
	Args map[string]any `json:"args,omitempty"                     doc:"The arguments of the command."`
}

type imageEditIn struct {
	ID             string           `json:"id"                        validate:"required"`
	BaseRevision   int64            `json:"base_revision"             validate:"required"               doc:"The revision the batch was planned against."`
	Operations     []ImageOperation `json:"operations"                validate:"required,min=1,max=200"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"                                   doc:"8 to 64 characters; the Idempotency-Key header is the same."`
	DryRun         any              `json:"dry_run,omitempty"`
}

type imageExportIn struct {
	ID      string `json:"id"                validate:"required"`
	Format  string `json:"format,omitempty"                      doc:"png (default), jpeg or webp."`
	Quality int    `json:"quality,omitempty"                     doc:"1 to 100, for jpeg and lossy webp."`
	Preset  string `json:"preset,omitempty"                      doc:"Resample to a named size."`
	Width   int    `json:"width,omitempty"                       doc:"Resample to this width; 0 keeps the aspect."`
	Height  int    `json:"height,omitempty"                      doc:"Resample to this height; 0 keeps the aspect."`
}

type imageImportIn struct {
	Path           string `json:"path,omitempty"            doc:"An absolute path to an image file on the machine of the server, read with the rules of media:import."`
	Hash           string `json:"hash,omitempty"            doc:"sha256: and 64 hex characters: an image already in the workspace, instead of path."`
	ImageID        string `json:"image_id,omitempty"        doc:"Put the file in this document as one more layer; without it a new document is made."`
	Name           string `json:"name,omitempty"            doc:"The name of the new document, or of the layer when image_id is given; default the name of the file."`
	Width          int    `json:"width,omitempty"           doc:"The canvas of a new document, with height; default the size of the file when it fits 4096."`
	Height         int    `json:"height,omitempty"          doc:"The canvas of a new document, with width."`
	Preset         string `json:"preset,omitempty"          doc:"The canvas of a new document: video_thumbnail, square_post or vertical_cover."`
	WaitMS         int    `json:"wait_ms,omitempty"         doc:"How long to wait for the ingest of the file, 0 to 25000; default 10000."`
	BaseRevision   int64  `json:"base_revision,omitempty"   doc:"With image_id: the revision the layer was planned against."`
	IdempotencyKey string `json:"idempotency_key,omitempty" doc:"With image_id: 8 to 64 characters; the Idempotency-Key header is the same."`
	DryRun         any    `json:"dry_run,omitempty"`
}

func (h *apiHandler) imageImport(c *web.Ctx, in imageImportIn) (service.ImageImportResult, error) {
	key := in.IdempotencyKey

	if header := c.Header("Idempotency-Key"); header != "" {
		if key != "" && key != header {
			return service.ImageImportResult{}, &service.Error{
				Code: command.CodeValidation, Path: "idempotency_key",
				Message: "the Idempotency-Key header and the idempotency_key member differ.",
			}
		}

		key = header
	}

	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ImageImportResult{}, err
	}

	res, err := h.Service.ImageImport(c.Context(), service.ImageImportRequest{
		Path: in.Path, Hash: in.Hash, ImageID: in.ImageID, Name: in.Name, Width: in.Width, Height: in.Height,
		Preset: in.Preset, WaitMS: in.WaitMS, BaseRevision: in.BaseRevision, IdempotencyKey: key, DryRun: dryRun,
	})
	if err != nil {
		return service.ImageImportResult{}, err
	}

	if res.ImageID != "" {
		revisionHeader(c, res.Revision)
	}

	return *res, nil
}

// imageImportFile is POST /images/import/file (SDD-021 section 10.2): the body is the file, the other members are the
// query. The route binds no input, so the framework never parses the body (as JSON or anything else); the body
// limit of the router (1 MiB) is replaced by the one of an upload, and the Service reads at most that many bytes
// into the content store. The declared Content-Type is ignored: a cookie session needs a preflighted type on an
// unsafe method (SDD-007 section 3.6 step 6), which is why the screen sends application/json.
func (h *apiHandler) imageImportFile(c *web.Ctx, _ noIn) (service.ImageImportResult, error) {
	r := c.Request()
	web.LimitBody(c.Writer(), r, service.MaxImageFileBytes+1)

	q := func(name string) string { return c.Query(name) }

	count := func(name string) (int, error) {
		raw := q(name)
		if raw == "" {
			return 0, nil
		}

		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " must be a whole number of 0 or more.",
			}
		}

		return n, nil
	}

	var (
		width, height, wait int
		base                int64
		err                 error
	)

	for name, dst := range map[string]*int{"width": &width, "height": &height, "wait_ms": &wait} {
		if *dst, err = count(name); err != nil {
			return service.ImageImportResult{}, err
		}
	}

	if raw := q("base_revision"); raw != "" {
		if base, err = strconv.ParseInt(raw, 10, 64); err != nil || base < 0 {
			return service.ImageImportResult{}, &service.Error{
				Code: command.CodeValidation, Path: "base_revision",
				Message: "base_revision must be a whole number of 0 or more.",
			}
		}
	}

	key := q("idempotency_key")
	if header := c.Header("Idempotency-Key"); header != "" {
		if key != "" && key != header {
			return service.ImageImportResult{}, &service.Error{
				Code: command.CodeValidation, Path: "idempotency_key",
				Message: "the Idempotency-Key header and the idempotency_key member differ.",
			}
		}

		key = header
	}

	dryRun, err := readDryRun(c, nil)
	if err != nil {
		return service.ImageImportResult{}, err
	}

	res, err := h.Service.ImageImportFile(c.Context(), service.ImageImportFileRequest{
		ImageID: q("image_id"), Name: q("name"), Width: width, Height: height, Preset: q("preset"), WaitMS: wait,
		BaseRevision: base, IdempotencyKey: key, DryRun: dryRun,
		FileName: q("file_name"), Body: r.Body, Size: r.ContentLength,
	})
	if err != nil {
		return service.ImageImportResult{}, err
	}

	if res.ImageID != "" {
		revisionHeader(c, res.Revision)
	}

	return *res, nil
}

func (h *apiHandler) imageCreate(c *web.Ctx, in imageCreateIn) (service.ImageCreateResult, error) {
	res, err := h.Service.ImageCreate(c.Context(), service.ImageCreateRequest{
		Name: in.Name, Preset: in.Preset, Width: in.Width, Height: in.Height, Background: in.Background,
		Template: in.Template, TemplateID: in.TemplateID, Values: in.Values, ID: in.ID,
	})

	return deref(res), err
}

func (h *apiHandler) imageList(c *web.Ctx, in imageListIn) (service.ImageListResult, error) {
	res, err := h.Service.ImageList(
		c.Context(),
		service.ImageListRequest{Templates: in.Templates, Page: page(in.pageIn)},
	)

	return deref(res), err
}

func (h *apiHandler) imageGet(c *web.Ctx, in imageIDIn) (service.ImageDescribeResult, error) {
	res, err := h.Service.ImageDescribe(c.Context(), service.ImageDescribeRequest{ImageID: in.ID})
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}

func (h *apiHandler) imageEdit(c *web.Ctx, in imageEditIn) (service.ImageEditResult, error) {
	key := in.IdempotencyKey

	if header := c.Header("Idempotency-Key"); header != "" {
		if key != "" && key != header {
			return service.ImageEditResult{}, &service.Error{
				Code: command.CodeValidation, Path: "idempotency_key",
				Message: "the Idempotency-Key header and the idempotency_key member differ.",
			}
		}

		key = header
	}

	ops := make([]command.Operation, len(in.Operations))

	for i, o := range in.Operations {
		args := o.Args
		if args == nil {
			args = map[string]any{}
		}

		raw, err := json.Marshal(args)
		if err != nil {
			return service.ImageEditResult{}, &service.Error{
				Code: command.CodeValidation, Path: "operations", Message: "the arguments are not valid JSON.",
			}
		}

		ops[i] = command.Operation{Op: o.Op, Args: raw}
	}

	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ImageEditResult{}, err
	}

	res, err := h.Service.ImageEdit(c.Context(), service.ImageEditRequest{
		ImageID: in.ID, BaseRevision: in.BaseRevision, Operations: ops, IdempotencyKey: key, DryRun: dryRun,
	})
	if err != nil {
		return service.ImageEditResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

func (h *apiHandler) imageStep(
	c *web.Ctx,
	in stepIn,
	fn func(ctx context.Context, req service.ImageStepRequest) (*service.ImageStepResult, error),
) (service.ImageStepResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ImageStepResult{}, err
	}

	res, err := fn(c.Context(), service.ImageStepRequest{ImageID: in.ID, DryRun: dryRun})
	if err != nil {
		return service.ImageStepResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

func (h *apiHandler) imageExport(c *web.Ctx, in imageExportIn) (service.ImageExportResult, error) {
	res, err := h.Service.ImageExport(c.Context(), service.ImageExportRequest{
		ImageID: in.ID, Format: in.Format, Quality: in.Quality, Preset: in.Preset, Width: in.Width, Height: in.Height,
	})

	return deref(res), err
}

func (h *apiHandler) registerImages() {
	register(h, "images.create", h.imageCreate)
	register(h, "images.list", h.imageList)
	register(h, "images.get", h.imageGet)
	register(h, "images.edit", h.imageEdit)
	register(h, "images.undo", func(c *web.Ctx, in stepIn) (service.ImageStepResult, error) {
		return h.imageStep(c, in, h.Service.ImageUndo)
	})
	register(h, "images.redo", func(c *web.Ctx, in stepIn) (service.ImageStepResult, error) {
		return h.imageStep(c, in, h.Service.ImageRedo)
	})
	register(h, "images.export", h.imageExport)
	register(h, "images.import", h.imageImport)
	register(h, "images.import_file", h.imageImportFile)
	register(h, "images.frame", h.imagesFrame)
	register(h, "images.history", h.imagesHistory)
	register(h, "images.file", h.imagesFile)
	register(h, "images.events", h.imagesStream)
}

// The three reads of the image editor screen (SDD-012 section 24.3): the picture, the log and a file that an
// export wrote. They are HTTP only; the rules and the rendering are in the Service.

type imageFrameIn struct {
	ID    string `json:"id"              validate:"required"`
	Width int    `json:"width,omitempty"                     doc:"Pixels, 16 to 4096; default the canvas width."`
	Rev   int64  `json:"rev,omitempty"                       doc:"Only a cache key of the caller: the answer is always the current revision."`
}

type imageHistoryIn struct {
	ID    string `json:"id"              validate:"required"`
	Limit int    `json:"limit,omitempty"                     doc:"1 to 100; default 30."`
}

type imageFileIn struct {
	ID   string `json:"id"   validate:"required"`
	Name string `json:"name" validate:"required" doc:"The file name ImageExport returned: <revision>-<12 hex>.<png|jpg|webp>."`
}

// imagesFrame is GET /images/{id}/frame: the PNG of the current revision, never cached by a proxy.
func (h *apiHandler) imagesFrame(c *web.Ctx, in imageFrameIn) (noIn, error) {
	res, err := h.Service.ImageFrame(c.Context(), service.ImageFrameRequest{ImageID: in.ID, Width: in.Width})
	if err != nil {
		return noIn{}, err
	}

	revisionHeader(c, res.Revision)
	c.SetHeader("X-Vukem-Image-Width", strconv.Itoa(res.Width))
	c.SetHeader("X-Vukem-Image-Height", strconv.Itoa(res.Height))
	c.SetHeader("Cache-Control", "private, no-cache")
	c.SetHeader("Content-Type", res.MIME)

	_, werr := c.Writer().Write(res.Image)

	return noIn{}, werr
}

func (h *apiHandler) imagesHistory(c *web.Ctx, in imageHistoryIn) (service.ImageHistoryResult, error) {
	res, err := h.Service.ImageHistory(c.Context(), service.ImageHistoryRequest{ImageID: in.ID, Limit: in.Limit})
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}

// imagesFile is GET /images/{id}/exports/{name}: the file as a download, with the Range support of http.ServeContent.
func (h *apiHandler) imagesFile(c *web.Ctx, in imageFileIn) (noIn, error) {
	f, err := h.Service.ImageFile(c.Context(), service.ImageFileRequest{ImageID: in.ID, Name: in.Name})
	if err != nil {
		return noIn{}, err
	}

	hd := c.Writer().Header()
	hd.Set("Content-Type", f.ContentType)
	hd.Set("Content-Disposition", `attachment; filename="`+f.DownloadName+`"`)
	hd.Set("Cache-Control", "private, max-age="+strconv.Itoa(f.MaxAge)+", immutable")
	hd.Set("ETag", f.ETag)

	if err := c.File(os.DirFS(string(h.Workspace)), f.Path); err != nil {
		return noIn{}, err
	}

	return noIn{}, nil
}
