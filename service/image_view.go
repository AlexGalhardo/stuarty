package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/export"
)

// The reads that the image editor screen needs besides the operations of SDD-021 section 11 (SDD-012 section 24.3):
// the picture of the current revision, the log of the latest batches and a file that an export wrote. All three
// are HTTP only: an agent has the text view, the log is in `image:show` and the file is on its machine.

const (
	imageFrameMinWidth  = 16
	defaultImageHistory = 30
	maxImageHistory     = 100
)

// ImageFrameRequest asks for the picture of a document at a width.
type ImageFrameRequest struct {
	ImageID string `json:"image_id"`
	Width   int    `json:"width,omitempty"` // 16 to 4096; 0 is the canvas width
}

// ImageFrameResult is the picture: PNG bytes with the straight alpha of the document.
type ImageFrameResult struct {
	ImageID  string `json:"image_id"`
	Revision int64  `json:"revision"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MIME     string `json:"mime"`
	Image    []byte `json:"image"`
}

// ImageFrame renders the current revision at screen size. It runs the same renderer as ImageExport and writes no
// file; unlike an export it is not stopped by a finding, so an image with an empty slot can still be seen.
func (s *svc) ImageFrame(ctx context.Context, req ImageFrameRequest) (*ImageFrameResult, error) {
	c, err := imageAuthorize(ctx, "ImageFrame")
	if err != nil {
		return nil, err
	}

	if req.Width != 0 && (req.Width < imageFrameMinWidth || req.Width > coreimage.MaxCanvas) {
		return nil, newError(
			command.CodeValidation,
			"the width is %d; it must be %d to %d.",
			req.Width,
			imageFrameMinWidth,
			coreimage.MaxCanvas,
		).withPath("width")
	}

	row, doc, lerr := s.loadImageRead(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	pix, rerr := export.RenderImage(ctx, doc, s.imageRenderConfig(ctx, c))
	if rerr != nil {
		return nil, imageExportError(rerr)
	}

	width := req.Width
	if width == 0 || width == doc.Width {
		width = 0
	}

	body, eerr := export.EncodeImage(ctx, pix, export.ImageEncodeOptions{Format: export.ImagePNG, Width: width})
	if eerr != nil {
		return nil, imageExportError(eerr)
	}

	w, h := doc.Width, doc.Height
	if width != 0 {
		// The encoder's resample: the height rounded half up and never below one pixel.
		w, h = width, max((doc.Height*width+doc.Width/2)/doc.Width, 1)
	}

	return &ImageFrameResult{
		ImageID: row.ID, Revision: row.Revision, Width: w, Height: h, MIME: "image/png", Image: body,
	}, nil
}

// ImageHistoryRequest asks for the latest batches of a document.
type ImageHistoryRequest struct {
	ImageID string `json:"image_id"`
	Limit   int    `json:"limit,omitempty"` // 1 to 100; default 30
}

// ImageHistoryEntry is one batch of the log.
type ImageHistoryEntry struct {
	Revision int64     `json:"revision"`
	Kind     string    `json:"kind"` // edit, undo or redo
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Ops      []string  `json:"ops"`
	Changes  int       `json:"changes"`
	UndoOf   *int64    `json:"undo_of,omitempty"`   // the revision an undo or a redo acted on
	UndoneBy *int64    `json:"undone_by,omitempty"` // the revision that undid this batch
	Skipped  int       `json:"skipped,omitempty"`   // the primitives an undo or a redo left alone
	Mine     bool      `json:"mine"`                // made by the caller
}

// ImageHistoryResult lists the latest batches, newest first.
type ImageHistoryResult struct {
	ImageID  string              `json:"image_id"`
	Revision int64               `json:"revision"`
	Entries  []ImageHistoryEntry `json:"entries"`
}

// ImageHistory reads the log of a document: who did what, newest first.
func (s *svc) ImageHistory(ctx context.Context, req ImageHistoryRequest) (*ImageHistoryResult, error) {
	c, err := imageAuthorize(ctx, "ImageHistory")
	if err != nil {
		return nil, err
	}

	if req.Limit < 0 || req.Limit > maxImageHistory {
		return nil, newError(
			command.CodeValidation,
			"the limit is %d; it must be 1 to %d.",
			req.Limit,
			maxImageHistory,
		).withPath("limit")
	}

	limit := req.Limit
	if limit == 0 {
		limit = defaultImageHistory
	}

	row, _, lerr := s.loadImageRead(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	// Revisions are consecutive: the latest `limit` batches are those after revision-limit.
	rows, oerr := s.store.Images().OpsSince(ctx, row.ID, max(row.Revision-int64(limit), 0))
	if oerr != nil {
		return nil, asError(oerr)
	}

	entries := make([]ImageHistoryEntry, 0, len(rows))

	for i := len(rows) - 1; i >= 0; i-- {
		entries = append(entries, imageHistoryEntry(&rows[i], c.Actor))
	}

	return &ImageHistoryResult{
		ImageID: row.ID, Revision: row.Revision, Entries: entries,
	}, nil
}

func imageHistoryEntry(row *ImageOpRow, me string) ImageHistoryEntry {
	e := ImageHistoryEntry{
		Revision: row.Revision, Kind: string(row.Kind), Actor: row.Actor, At: msTime(row.AtMs),
		Ops: opNames(row.Ops), Changes: len(decodeDiff(row.Diff)), Mine: row.Actor == me,
		Skipped: skippedCount(row.Skipped),
	}

	if e.Ops == nil {
		e.Ops = []string{}
	}

	if row.Kind == OpUndo || row.Kind == OpRedo {
		e.UndoOf = row.TargetRevision
	}

	if (row.Kind == OpEdit || row.Kind == OpRedo) && row.ClosedBy != nil && *row.ClosedBy != row.Revision {
		e.UndoneBy = row.ClosedBy
	}

	return e
}

// imageFileName is the name ImageExport gives a file: the revision, twelve characters of the digest and the
// extension of the format.
var imageFileName = regexp.MustCompile(`^\d{1,18}-[0-9a-f]{12}\.(png|jpg|webp)$`)

var imageFileTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp"}

// ImageFileRequest names a file that ImageExport wrote.
type ImageFileRequest struct {
	ImageID string `json:"image_id"`
	Name    string `json:"name"`
}

// ImageFile resolves a file of exports/images/<image id>/ for download. The name is checked against the shape
// ImageExport gives, and the path is built from the id of a document the caller may see, never from the request
// alone. The path is relative to the workspace; the caller serves it.
func (s *svc) ImageFile(ctx context.Context, req ImageFileRequest) (*File, error) {
	c, err := imageAuthorize(ctx, "ImageFile")
	if err != nil {
		return nil, err
	}

	row, _, lerr := s.loadImageRead(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	if !imageFileName.MatchString(req.Name) {
		return nil, newError(command.CodeNotFound, "there is no such export of %s.", row.ID).
			withHint("export the image, then download the file it names")
	}

	rel := path.Join("exports", "images", row.ID, req.Name)

	info, serr := os.Stat(filepath.Join(s.workspace, filepath.FromSlash(rel)))
	if serr != nil {
		if errors.Is(serr, os.ErrNotExist) {
			return nil, newError(command.CodeNotFound, "there is no such export of %s.", row.ID).
				withHint("export the image, then download the file it names")
		}

		return nil, internalError()
	}

	ext := path.Ext(req.Name)

	return &File{
		Path: rel, DownloadName: fmt.Sprintf("%s-%s", row.ID, req.Name), ContentType: imageFileTypes[ext],
		Size: info.Size(), ETag: `"` + req.Name + `"`, ModTime: info.ModTime().UTC().Truncate(time.Second),
		Immutable: true, MaxAge: 31536000,
	}, nil
}

// skippedCount is how many primitives an undo or a redo left alone, from the stored list.
func skippedCount(raw json.RawMessage) int {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return 0
	}

	return len(list)
}
