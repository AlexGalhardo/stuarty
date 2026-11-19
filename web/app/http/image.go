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
	register(h, "images.frame", h.imagesFrame)
	register(h, "images.history", h.imagesHistory)
	register(h, "images.file", h.imagesFile)
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
