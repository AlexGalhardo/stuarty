package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

type frameIn struct {
	ID        string `json:"id"                   validate:"required"`
	At        string `json:"at,omitempty"                             doc:"The time to render; required."`
	Rev       int64  `json:"rev,omitempty"                            doc:"Must be the current revision when given."`
	AssetID   string `json:"asset_id,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Width     int    `json:"width,omitempty"`
}

type sheetIn struct {
	ID        string `json:"id"                   validate:"required"`
	Times     string `json:"times,omitempty"                          doc:"Comma-separated times, 2 to 16."`
	From      string `json:"from,omitempty"`
	Until     string `json:"until,omitempty"`
	Count     int    `json:"count,omitempty"`
	Cols      int    `json:"cols,omitempty"`
	Rev       int64  `json:"rev,omitempty"                            doc:"Must be the current revision when given."`
	AssetID   string `json:"asset_id,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// projectsFrame is GET /projects/{id}/frame (SDD-003 section 6.5): the JPEG, or
// the ViewResult as JSON when the client asks for application/json.
func (h *apiHandler) projectsFrame(c *web.Ctx, in frameIn) (noIn, error) {
	if err := h.pinRevision(c, in.ID, in.Rev); err != nil {
		return noIn{}, err
	}

	res, err := h.Service.ViewFrame(c.Context(), service.ViewFrameRequest{
		ProjectID: in.ID, At: in.At, AssetID: in.AssetID, MaxTokens: in.MaxTokens, Width: in.Width,
	})
	if err != nil {
		return noIn{}, err
	}

	return noIn{}, writeView(c, res, in.Rev != 0, "f"+strconv.FormatInt(res.Cells[0].At.Frame, 10), 8)
}

// projectsSheet is GET /projects/{id}/sheet.
func (h *apiHandler) projectsSheet(c *web.Ctx, in sheetIn) (noIn, error) {
	if err := h.pinRevision(c, in.ID, in.Rev); err != nil {
		return noIn{}, err
	}

	var times []string

	if in.Times != "" {
		times = strings.Split(in.Times, ",")
	}

	res, err := h.Service.ViewSheet(c.Context(), service.ViewSheetRequest{
		ProjectID: in.ID, Times: times, From: in.From, Until: in.Until, Count: in.Count, Cols: in.Cols,
		AssetID: in.AssetID, MaxTokens: in.MaxTokens,
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

// pinRevision refuses a rev that is not the current revision (SDD-003 section
// 6.3): the service renders the current one only.
func (h *apiHandler) pinRevision(c *web.Ctx, project string, rev int64) error {
	if rev == 0 {
		return nil
	}

	head, err := h.Service.ProjectGet(c.Context(), service.ProjectGetRequest{ProjectID: project})
	if err != nil {
		return err
	}

	if head.Revision != rev {
		return &service.Error{
			Code:    command.CodeInvalidState,
			Message: "the project is at revision " + strconv.FormatInt(head.Revision, 10) + ".",
			Hint:    "ask again without rev, or with the current revision",
			Details: map[string]any{"current_revision": head.Revision},
		}
	}

	return nil
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

	if pinned {
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

type planIn struct {
	ID    string `json:"id"             validate:"required"`
	From  int    `json:"from,omitempty"                     doc:"The first frame of the window; default 0."`
	Until int    `json:"until"          validate:"required" doc:"The end of the window, not included; at most 300 frames after from."`
	Rev   int64  `json:"rev,omitempty"                      doc:"Must be the current revision when given."`
}

// projectsPlan is GET /projects/{id}/plan (SDD-004 section 10).
func (h *apiHandler) projectsPlan(c *web.Ctx, in planIn) (service.PlanWindowResult, error) {
	if err := h.pinRevision(c, in.ID, in.Rev); err != nil {
		return service.PlanWindowResult{}, err
	}

	res, err := h.Service.PlanWindow(
		c.Context(),
		service.PlanWindowRequest{ProjectID: in.ID, From: in.From, Until: in.Until},
	)
	if res != nil {
		revisionHeader(c, res.Revision)
	}

	return deref(res), err
}
