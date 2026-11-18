package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the decoder that reads the size of a rendered frame
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
	"github.com/AlexGalhardo/vukem-video/engine/cache"
	"github.com/AlexGalhardo/vukem-video/engine/frame"
	"github.com/AlexGalhardo/vukem-video/service/textview"
)

const (
	minViewTokens   = 100
	minSheetCells   = 2
	defaultSheetCnt = 9
)

// ViewFrame renders one frame of the current revision through the export
// executor (SDD-003 section 2.7).
func (s *svc) ViewFrame(ctx context.Context, req ViewFrameRequest) (*ViewResult, error) {
	v, err := s.beginView(ctx, "ViewFrame", req.ProjectID, req.Pin)
	if err != nil {
		return nil, err
	}

	if req.Width != 0 && req.MaxTokens != 0 {
		return nil, newError(command.CodeValidation, "width and max_tokens cannot be used together.").
			withPath("width").withHint("pass one of them")
	}

	if req.At == "" {
		return nil, newError(command.CodeValidation, "at is required.").withPath("at")
	}

	at, err := v.parseTime("at", req.At)
	if err != nil {
		return nil, err
	}

	st := v.doc.Settings

	var width, height int

	if req.Width != 0 {
		if req.Width < 16 || req.Width > frame.MaxWidth || req.Width%2 != 0 {
			return nil, newError(
				"E_LIMIT_EXCEEDED",
				"width is %d; it must be an even number from 16 to %d.",
				req.Width,
				frame.MaxWidth,
			).
				withHint("ask for an even width of at most %d", frame.MaxWidth).
				withLimit("width", req.Width, frame.MaxWidth).
				withPath("width")
		}

		width = req.Width
	} else {
		budget, berr := viewBudget(req.MaxTokens, frame.DefaultFrameBudg)
		if berr != nil {
			return nil, berr
		}

		width, height, _ = frame.ImageSize(st.Width, st.Height, budget)

		// A budget of more than about 1000 tokens sizes a wide frame past the widest one served; the
		// frame is then the widest one, not a refusal (VV-037).
		if width > frame.MaxWidth {
			width = frame.MaxWidth
			height = max(2, 2*int(math.Round(float64(width)*float64(st.Height)/float64(st.Width)/2)))
		}
	}

	jpeg, rerr := s.framesFor(v.doc).FrameSized(ctx, v.doc, v.ref(), int(at), width, height)
	if rerr != nil {
		return nil, viewError(rerr)
	}

	w, h := jpegSize(jpeg, width, st.Width, st.Height)
	key := cache.FrameKey(v.row.ID, v.row.Revision, int(at), width)
	if height != 0 {
		key = cache.FrameKeyHeight(v.row.ID, v.row.Revision, int(at), width, height)
	}
	path := s.viewPath(v.caller, key)

	res := &ViewResult{
		ProjectID:   v.row.ID,
		Revision:    v.row.Revision,
		Mode:        "frame",
		Width:       w,
		Height:      h,
		ImageTokens: frame.Tokens(w, h),
		MIME:        "image/jpeg",
		Cells:       []ViewCell{{Index: 1, At: timeValue(st.FPS, at)}},
		Path:        path,
		Image:       jpeg,
	}

	fps := v.fps()
	res.Text = fmt.Sprintf("frame %s of %s r%d: %dx%d jpeg, about %d image tokens%s",
		textview.Cell(fps, at), res.ProjectID, res.Revision, w, h, res.ImageTokens, saved(path))

	return res, nil
}

// ViewSheet renders a contact sheet of the current revision (SDD-003 section 3.11).
func (s *svc) ViewSheet(ctx context.Context, req ViewSheetRequest) (*ViewResult, error) {
	v, err := s.beginView(ctx, "ViewSheet", req.ProjectID, req.Pin)
	if err != nil {
		return nil, err
	}

	budget, err := viewBudget(req.MaxTokens, frame.DefaultSheetBudg)
	if err != nil {
		return nil, err
	}

	times, err := v.sheetTimes(&req)
	if err != nil {
		return nil, err
	}

	if req.Cols < 0 || req.Cols > frame.MaxSheetCells {
		return nil, newError(command.CodeValidation, "cols is %d; it must be 1 to %d.", req.Cols, frame.MaxSheetCells).
			withPath("cols")
	}

	sheet, rerr := s.framesFor(v.doc).Sheet(
		ctx,
		v.doc,
		v.ref(),
		frame.SheetRequest{Times: times, Cols: req.Cols, Budget: budget},
	)
	if rerr != nil {
		return nil, viewError(rerr)
	}

	st := v.doc.Settings
	cells := make([]ViewCell, len(sheet.Cells))

	var labels strings.Builder

	for i, c := range sheet.Cells {
		cells[i] = ViewCell{Index: c.Index, At: timeValue(st.FPS, int64(c.Frame))}

		labels.WriteString(" " + strconv.Itoa(c.Index) + "=" + textview.Cell(v.fps(), int64(c.Frame)))
	}

	cols := req.Cols
	if cols <= 0 {
		cols = int(math.Ceil(math.Sqrt(float64(len(times)))))
	}

	cols = min(cols, len(times))
	rows := (len(times) + cols - 1) / cols
	path := s.viewPath(v.caller, sheet.Key)

	res := &ViewResult{
		ProjectID: v.row.ID, Revision: v.row.Revision, Mode: "sheet", Width: sheet.Width, Height: sheet.Height,
		ImageTokens: sheet.Tokens, MIME: "image/jpeg", Cells: cells, Path: path, Image: sheet.JPEG,
	}
	res.Text = fmt.Sprintf("sheet %dx%d of %s r%d: %dx%d jpeg, about %d image tokens%s\ncells:%s",
		cols, rows, res.ProjectID, res.Revision, res.Width, res.Height, res.ImageTokens, saved(path), labels.String())

	return res, nil
}

func saved(path string) string {
	if path == "" {
		return ""
	}

	return ", saved " + path
}

// view is what a view operation reads once: the project at one revision.
type view struct {
	caller Caller
	row    ProjectRow
	doc    *timeline.Document
}

func (v *view) ref() frame.Ref { return frame.Ref{Project: v.row.ID, Revision: v.row.Revision} }

func (v *view) fps() timeval.FPS {
	return timeval.FPS{Num: int64(v.doc.Settings.FPS.Num), Den: int64(v.doc.Settings.FPS.Den)}
}

// last is the last frame that has a picture: the end of the timeline is not one.
func (v *view) last() int64 { return max(endFrame(v.doc)-1, 0) }

// pinMismatch is the error of a view or plan pinned to a revision that is not
// the current one (SDD-003 section 6.3): the service renders the current one only.
func pinMismatch(current int64) *Error {
	return newError(command.CodeInvalidState, "the project is at revision %d.", current).
		withHint("ask again without rev, or with the current revision").
		withDetail("current_revision", current)
}

func (s *svc) beginView(ctx context.Context, method, projectID string, pin int64) (*view, error) {
	c, err := authorize(ctx, method, projectID)
	if err != nil {
		return nil, err
	}

	// The project is looked up first: a caller who may not see it learns nothing
	// about what this build can render.
	row, lerr := s.loadProject(ctx, c, projectID)
	if lerr != nil {
		return nil, lerr
	}

	if pin != 0 && row.Revision != pin {
		return nil, pinMismatch(row.Revision)
	}

	if s.media.Frames == nil {
		return nil, newError(command.CodeInternal, "viewing is not available in this build.")
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	return &view{caller: c, row: row, doc: doc}, nil
}

// parseTime reads a time and refuses one past the last frame, naming the last
// valid time in the hint.
func (v *view) parseTime(field, raw string) (int64, error) {
	t, perr := timeval.Parse(raw, v.fps())
	if perr != nil {
		return 0, newError(command.CodeInvalidTime, "%s is not a time.", field).withPath(field).
			withHint("write a time like 1.5s or 45f")
	}

	if t < 0 || t > v.last() {
		return 0, newError(command.CodeInvalidTime, "%s is frame %d; the last frame is %d.", field, t, v.last()).
			withPath(field).withHint("the last valid time is %s", v.fps().FormatSeconds(v.last()))
	}

	return t, nil
}

func (v *view) sheetTimes(req *ViewSheetRequest) ([]int, error) {
	if len(req.Times) > 0 {
		if req.From != "" || req.Until != "" || req.Count != 0 {
			return nil, newError(
				command.CodeValidation,
				"times cannot be used with from, until or count.",
			).withPath("times")
		}

		if len(req.Times) < minSheetCells || len(req.Times) > frame.MaxSheetCells {
			return nil, newError(command.CodeValidation, "times has %d entries; it needs %d to %d.",
				len(req.Times), minSheetCells, frame.MaxSheetCells).withPath("times")
		}

		out := make([]int, len(req.Times))

		for i, raw := range req.Times {
			t, err := v.parseTime("times["+strconv.Itoa(i)+"]", raw)
			if err != nil {
				return nil, err
			}

			out[i] = int(t)
		}

		return out, nil
	}

	count := req.Count
	if count == 0 {
		count = defaultSheetCnt
	}

	if count < minSheetCells || count > frame.MaxSheetCells {
		return nil, newError(
			command.CodeValidation,
			"count is %d; it must be %d to %d.",
			count,
			minSheetCells,
			frame.MaxSheetCells,
		).
			withPath("count")
	}

	// With no time given the sheet spans the timeline; an empty one has nothing
	// to show, and six cells of "0s" would pass for an answer.
	if req.From == "" && req.Until == "" && endFrame(v.doc) == 0 {
		return nil, newError(command.CodeValidation, "the timeline is empty; there is nothing to show.").
			withHint("add clips to the timeline, or pass times that exist")
	}

	from, to := int64(0), v.last()

	var err error
	if req.From != "" {
		if from, err = v.parseTime("from", req.From); err != nil {
			return nil, err
		}
	}

	if req.Until != "" {
		if to, err = v.parseTime("until", req.Until); err != nil {
			return nil, err
		}
	}

	if to < from {
		return nil, newError(command.CodeValidation, "until is before from.").withPath("until")
	}

	return frame.SheetTimes(int(from), int(to), count), nil
}

func viewBudget(requested, def int) (int, error) {
	if requested == 0 {
		return def, nil
	}

	if requested < minViewTokens || requested > frame.MaxTokens {
		return 0, newError(
			command.CodeValidation,
			"max_tokens is %d; it must be %d to %d.",
			requested,
			minViewTokens,
			frame.MaxTokens,
		).
			withPath("max_tokens")
	}

	return requested, nil
}

// viewPath is the file of a cached image for a local caller; a network caller
// never learns where the server keeps it.
func (s *svc) viewPath(c Caller, key string) string {
	if c.OwnerUserID != "" || s.workspace == "" || key == "" {
		return ""
	}

	p := filepath.Join(s.workspace, filepath.FromSlash(key))

	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}

	return abs
}

// jpegSize reads the true size of a rendered image; the planned size is the
// fallback when the bytes cannot be read.
func jpegSize(b []byte, width, pw, ph int) (w, h int) {
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(b)); err == nil {
		return cfg.Width, cfg.Height
	}

	return width, int(math.Round(float64(width) * float64(ph) / float64(pw)))
}

// viewError maps what the frame engine returns: a range error is the caller's,
// anything else is the server's and says nothing about its insides.
func viewError(err error) error {
	if errors.Is(err, frame.ErrRange) {
		return newError(command.CodeValidation, "the frame is outside what can be shown.")
	}

	return asError(err)
}
