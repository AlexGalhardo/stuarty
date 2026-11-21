package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type frameIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	At         string `json:"at,omitempty"                              doc:"The time to render; required."`
	Rev        int64  `json:"rev,omitempty"                             doc:"Must be the current revision when given."`
	MaxTokens  int    `json:"max_tokens,omitempty"`
	Width      int    `json:"width,omitempty"`
}

type sheetIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Times      string `json:"times,omitempty"                           doc:"Comma-separated times, 2 to 16."`
	From       string `json:"from,omitempty"`
	Until      string `json:"until,omitempty"`
	To         string `json:"to,omitempty"                              doc:"Deprecated alias of until (whole frames), removed at 1.0."`
	Count      int    `json:"count,omitempty"`
	Cols       int    `json:"cols,omitempty"`
	Rev        int64  `json:"rev,omitempty"                             doc:"Must be the current revision when given."`
	MaxTokens  int    `json:"max_tokens,omitempty"`
}

// projectsFrame is GET /projects/{id}/frame (SDD-003 section 6.5): the JPEG, or
// the ViewResult as JSON when the client asks for application/json.
func (h *apiHandler) projectsFrame(c *web.Ctx, in frameIn) (noIn, error) {
	res, err := h.Service.ViewFrame(c.Context(), service.ViewFrameRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID, At: in.At, MaxTokens: in.MaxTokens, Width: in.Width, Pin: in.Rev,
	})
	if err != nil {
		return noIn{}, err
	}

	return noIn{}, writeView(c, res, in.Rev != 0, "f"+strconv.FormatInt(res.Cells[0].At.Frame, 10), 8)
}

// projectsSheet is GET /projects/{id}/sheet.
func (h *apiHandler) projectsSheet(c *web.Ctx, in sheetIn) (noIn, error) {
	until, err := untilAlias(in.Until, in.To)
	if err != nil {
		return noIn{}, err
	}

	var times []string

	if in.Times != "" {
		times = strings.Split(in.Times, ",")
	}

	res, err := h.Service.ViewSheet(c.Context(), service.ViewSheetRequest{
		ProjectID:  in.ID,
		TimelineID: in.TimelineID,
		Times:      times,
		From:       in.From,
		Until:      until,
		Count:      in.Count,
		Cols:       in.Cols,
		MaxTokens:  in.MaxTokens,
		Pin:        in.Rev,
	})
	if err != nil {
		return noIn{}, err
	}

	cells := make([]string, len(res.Cells))
	for i, cell := range res.Cells {
		cells[i] = strconv.Itoa(cell.Index) + "=" + strconv.FormatFloat(cell.At.Seconds, 'f', -1, 64)
	}

	c.SetHeader("X-Vukem-Cells", strings.Join(cells, ","))

	return noIn{}, writeView(c, res, in.Rev != 0, "s", 12)
}

// writeView answers the image with the headers of SDD-003 section 6.5, or the
// result as JSON for a client that accepts it.
func writeView(c *web.Ctx, res *service.ViewResult, pinned bool, kind string, hashHex int) error {
	sum := sha256.Sum256(res.Image)
	tag := `"r` + strconv.FormatInt(res.Revision, 10) + "-" + kind + "-w" + strconv.Itoa(res.Width) + "-" +
		hex.EncodeToString(sum[:])[:hashHex] + `"`

	revisionHeader(c, res.Revision)
	c.SetHeader("ETag", tag)
	c.SetHeader("X-Vukem-Frame", strconv.FormatInt(res.Cells[0].At.Frame, 10))
	c.SetHeader("X-Vukem-Time", strconv.FormatFloat(res.Cells[0].At.Seconds, 'f', -1, 64))
	c.SetHeader("X-Vukem-Image-Width", strconv.Itoa(res.Width))
	c.SetHeader("X-Vukem-Image-Height", strconv.Itoa(res.Height))
	c.SetHeader("X-Vukem-Image-Tokens", strconv.Itoa(res.ImageTokens))

	if pinned && !res.Composite {
		c.SetHeader("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		c.SetHeader("Cache-Control", "private, no-cache")
	}

	w := c.Writer()

	if strings.Contains(requestedAccept(c.Request()), "application/json") {
		w.Header().Set("Content-Type", "application/json")

		return json.NewEncoder(w).Encode(res)
	}

	if c.Request().Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)

		return nil
	}

	w.Header().Set("Content-Type", res.MIME)
	_, err := w.Write(res.Image)

	return err
}

type bitmapIn struct {
	ID  string `json:"id"  validate:"required"`
	Key string `json:"key" validate:"required"`
}

// untilAlias resolves the deprecated query parameter to onto until. Both given
// and different is a validation error; the alias goes at 1.0 (questions item 3).
func untilAlias[T comparable](until, to T) (T, error) {
	var zero T

	switch {
	case to == zero:
		return until, nil
	case until == zero || until == to:
		return to, nil
	default:
		return zero, &service.Error{
			Code:    "E_VALIDATION",
			Message: "until and its deprecated alias to differ.",
			Hint:    "send only until",
			Path:    "to",
		}
	}
}

type planIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	From       int    `json:"from,omitempty"                            doc:"The first frame of the window, as a whole number of frames (not a time such as 1s); default 0."`
	Until      int    `json:"until,omitempty"                           doc:"The end of the window in whole frames (not a time such as 1s), not included; at most 300 frames after from. Required."`
	To         int    `json:"to,omitempty"                              doc:"Deprecated alias of until (whole frames), removed at 1.0."`
	Rev        int64  `json:"rev,omitempty"                             doc:"Must be the current revision when given."`
	Width      int    `json:"width,omitempty"                           doc:"The width in pixels the preview draws at, 16 to 4096; bitmaps are rasterized at that scale. Default: the project width."`
}

// projectsPlan is GET /projects/{id}/plan (SDD-004 section 10).
func (h *apiHandler) projectsPlan(c *web.Ctx, in planIn) (service.PlanWindowResult, error) {
	until, err := untilAlias(in.Until, in.To)
	if err != nil {
		return service.PlanWindowResult{}, err
	}

	res, err := h.Service.PlanWindow(
		c.Context(),
		service.PlanWindowRequest{
			ProjectID:  in.ID,
			TimelineID: in.TimelineID,
			From:       in.From,
			Until:      until,
			Width:      in.Width,
			Pin:        in.Rev,
		},
	)
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}

type snapPointsIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Rev        int64  `json:"rev,omitempty"                             doc:"The revision the caller holds; 409 when the project has moved on."`
	Words      int    `json:"words,omitempty"                           doc:"1 adds the start and end of every caption word."`
}

// snapPoints is GET /projects/{id}/snap-points (SDD-012 section 7.8).
func (h *apiHandler) snapPoints(c *web.Ctx, in snapPointsIn) (service.SnapPointsResult, error) {
	res, err := h.Service.SnapPoints(
		c.Context(),
		service.SnapPointsRequest{ProjectID: in.ID, TimelineID: in.TimelineID, Revision: in.Rev, Words: in.Words == 1},
	)
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}

// fontList is GET /fonts.
func (h *apiHandler) fontList(c *web.Ctx, _ noIn) (service.FontListResult, error) {
	res, err := h.Service.FontList(c.Context(), service.FontListRequest{})

	return deref(res), err
}

type assetListIn struct {
	pageIn
	Kind     string `json:"kind,omitempty"     doc:"image, video, audio or lut; omit it for every kind."`
	Section  string `json:"section,omitempty"  doc:"visual (image, lut), video (video) or audio (audio): the three sections of the library."`
	Tag      string `json:"tag,omitempty"      doc:"Keep the files that carry this tag."`
	Project  string `json:"project,omitempty"  doc:"Keep the files a project, or one of its timelines, uses."`
	Origin   string `json:"origin,omitempty"   doc:"imported, generated, fetched_by_agent or rendered_externally."`
	Reusable string `json:"reusable,omitempty" doc:"true keeps the files marked reusable, false the others."`
	Text     string `json:"text,omitempty"     doc:"A full-text query over name, tags, description and origin; at most 8 words."`
}

// assetList is GET /assets: the files of the library by content hash (VV-117, SDD-011 section 14).
func (h *apiHandler) assetList(c *web.Ctx, in assetListIn) (service.AssetListResult, error) {
	reusable, err := parseReusable(in.Reusable)
	if err != nil {
		return service.AssetListResult{}, err
	}

	res, err := h.Service.AssetList(c.Context(), service.AssetListRequest{
		Kind: in.Kind, Section: in.Section, Tag: in.Tag, Project: in.Project, Origin: in.Origin,
		Reusable: reusable, Text: in.Text, Limit: in.Limit, Cursor: in.Cursor,
	})

	return deref(res), err
}

type auditIn struct {
	pageIn
	ProjectID string `json:"project_id,omitempty" doc:"Keep the entries of one project."`
}

// auditList is GET /audit; the Service requires the admin scope.
func (h *apiHandler) auditList(c *web.Ctx, in auditIn) (service.AuditListResult, error) {
	res, err := h.Service.AuditList(c.Context(), service.AuditListRequest{
		ProjectID: in.ProjectID, Cursor: in.Cursor, Limit: in.Limit,
	})

	return deref(res), err
}

type systemIn struct {
	Full   bool `json:"full,omitempty"   doc:"Run the checks."`
	Verify bool `json:"verify,omitempty" doc:"Recompute the digests of FFmpeg, ffprobe and the installed helpers."`
}

// systemGet is GET /system. The sweep and the bundle are not reachable here: they act on
// the machine of the server and run from its terminal.
func (h *apiHandler) systemGet(c *web.Ctx, in systemIn) (service.DoctorResult, error) {
	res, err := h.Service.Doctor(c.Context(), service.DoctorRequest{Full: in.Full, Verify: in.Verify})

	return deref(res), err
}
