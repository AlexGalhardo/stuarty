package httpapi

import (
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type standaloneVideoCreateIn struct {
	Name      string `json:"name,omitempty"       doc:"Title for the standalone edit (1 to 120 characters)."`
	Preset    string `json:"preset,omitempty"     doc:"9:16, 1:1, 16:9 or 4:5."`
	FPS       string `json:"fps,omitempty"        doc:"Frame rate such as 30 or 30000/1001."`
	Path      string `json:"path,omitempty"       doc:"Absolute path to a video file on the server machine."`
	AssetHash string `json:"asset_hash,omitempty" doc:"sha256: content hash of an existing library asset."`
	WaitMS    int    `json:"wait_ms,omitempty"    doc:"Milliseconds to wait for ingest (0 to 25000)."`
	ID        string `json:"id,omitempty"         doc:"Optional caller-chosen prj_ identifier."`
	DryRun    any    `json:"dry_run,omitempty"`
}

type standalonePromoteIn struct {
	ID          string              `json:"id"             validate:"required"`
	Name        string              `json:"name"           validate:"required" doc:"4 to 32 letters, digits and single spaces."`
	Description string              `json:"description"    validate:"required" doc:"64 to 1024 characters."`
	Goal        service.ProjectGoal `json:"goal"           validate:"required"`
	Kind        string              `json:"kind,omitempty"                     doc:"video (default), text, image or undecided."`
	DryRun      any                 `json:"dry_run,omitempty"`
}

type imageFromAssetIn struct {
	AssetHash string `json:"asset_hash"        validate:"required" doc:"sha256: content hash of an image in the workspace library."`
	Name      string `json:"name,omitempty"                        doc:"Optional title for the new image document."`
	Width     int    `json:"width,omitempty"                       doc:"Optional canvas width (1 to 4096)."`
	Height    int    `json:"height,omitempty"                      doc:"Optional canvas height (1 to 4096)."`
	Preset    string `json:"preset,omitempty"                      doc:"Optional canvas preset."`
	WaitMS    int    `json:"wait_ms,omitempty"                     doc:"Milliseconds to wait for ingest (0 to 25000)."`
	DryRun    any    `json:"dry_run,omitempty"`
}

func (h *apiHandler) registerStandalone() {
	register(h, "standalone.videos.list", h.standaloneVideoList)
	register(h, "standalone.videos.create", h.standaloneVideoCreate)
	register(h, "standalone.videos.upload", h.standaloneVideoUpload)
	register(h, "standalone.videos.promote", h.standaloneVideoPromote)
	register(h, "images.from_asset", h.imageOpenFromAsset)
}

func (h *apiHandler) standaloneVideoList(c *web.Ctx, in listIn) (service.ProjectListResult, error) {
	if c.Query("limit") == "0" {
		return service.ProjectListResult{}, &service.Error{
			Code: command.CodeValidation, Path: "limit",
			Message: "limit must be at least 1.",
		}
	}

	res, err := h.Service.StandaloneVideoList(c.Context(), service.StandaloneVideoListRequest{
		State: in.State,
		Page:  page(in.pageIn),
	})

	return deref(res), err
}

func (h *apiHandler) standaloneVideoCreate(
	c *web.Ctx,
	in standaloneVideoCreateIn,
) (service.StandaloneVideoResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.StandaloneVideoResult{}, err
	}

	res, err := h.Service.StandaloneVideoCreate(c.Context(), service.StandaloneVideoCreateRequest{
		Name:      in.Name,
		Preset:    in.Preset,
		FPS:       in.FPS,
		Path:      in.Path,
		AssetHash: in.AssetHash,
		WaitMS:    in.WaitMS,
		ID:        in.ID,
		DryRun:    dryRun,
	})
	if err != nil {
		return service.StandaloneVideoResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

// standaloneVideoUpload is POST /standalone/videos/upload (SDD-005 section 5.12.5): accepts either
// raw bytes in the request body (with ?file_name=..., matching POST /images/import/file) or a
// multipart/form-data upload with a "file" part.
func (h *apiHandler) standaloneVideoUpload(c *web.Ctx, _ noIn) (service.StandaloneVideoResult, error) {
	const maxUploadBodyBytes int64 = 8 << 30 // matches service defaultMaxImportBytes; service enforces exact STUARTY_MAX_IMPORT_BYTES

	r := c.Request()
	web.LimitBody(c.Writer(), r, maxUploadBodyBytes+1)

	q := func(name string) string { return c.Query(name) }

	wait := 0
	if raw := q("wait_ms"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return service.StandaloneVideoResult{}, &service.Error{
				Code: command.CodeValidation, Path: "wait_ms",
				Message: "wait_ms must be a whole number of 0 or more.",
			}
		}

		wait = n
	}

	dryRun, err := readDryRun(c, nil)
	if err != nil {
		return service.StandaloneVideoResult{}, err
	}

	var (
		body     io.Reader = r.Body
		size               = r.ContentLength
		fileName           = q("file_name")
	)

	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType, params, perr := mime.ParseMediaType(ct); perr == nil &&
			strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
			mr := multipart.NewReader(r.Body, params["boundary"])

			var found bool

			for {
				part, nextErr := mr.NextPart()
				if nextErr == io.EOF {
					break
				}

				if nextErr != nil {
					return service.StandaloneVideoResult{}, &service.Error{
						Code: command.CodeValidation, Path: "file",
						Message: "could not read the uploaded multipart body.",
					}
				}

				if part.FormName() == "file" {
					body = part
					size = -1

					if fileName == "" {
						fileName = part.FileName()
					}

					found = true

					break
				}

				_ = part.Close()
			}

			if !found {
				return service.StandaloneVideoResult{}, &service.Error{
					Code: command.CodeValidation, Path: "file",
					Message: "multipart upload must include a file field.",
				}
			}
		}
	}

	res, err := h.Service.StandaloneVideoUpload(c.Context(), service.StandaloneVideoUploadRequest{
		Name:     q("name"),
		Preset:   q("preset"),
		FPS:      q("fps"),
		WaitMS:   wait,
		FileName: fileName,
		Body:     body,
		Size:     size,
		DryRun:   dryRun,
	})
	if err != nil {
		return service.StandaloneVideoResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

func (h *apiHandler) standaloneVideoPromote(c *web.Ctx, in standalonePromoteIn) (service.ProjectUpdateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectUpdateResult{}, err
	}

	res, err := h.Service.StandalonePromote(c.Context(), service.StandalonePromoteRequest{
		ProjectID:   in.ID,
		Name:        in.Name,
		Description: in.Description,
		Goal:        in.Goal,
		Kind:        in.Kind,
		DryRun:      dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) imageOpenFromAsset(c *web.Ctx, in imageFromAssetIn) (service.ImageImportResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ImageImportResult{}, err
	}

	res, err := h.Service.ImageOpenFromAsset(c.Context(), service.ImageOpenFromAssetRequest{
		AssetHash: in.AssetHash,
		Name:      in.Name,
		Width:     in.Width,
		Height:    in.Height,
		Preset:    in.Preset,
		WaitMS:    in.WaitMS,
		DryRun:    dryRun,
	})
	if err != nil {
		return service.ImageImportResult{}, err
	}

	if res.ImageID != "" {
		revisionHeader(c, res.Revision)
	}

	return *res, nil
}
