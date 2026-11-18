package export

import (
	"context"
	"errors"
	"image"
	"os"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

// RenderConfig is what a Renderer needs: the same inputs as an export, without the output.
type RenderConfig struct {
	Runner       *ffmpeg.Runner
	Doc          *timeline.Document
	JobID        string
	ArtifactsDir string
	Fonts        *rasterize.FontSet // the bundled fonts when nil
	Bands        int
}

// Renderer composes single frames of a project through the same compositor and decoders as the
// export (SDD-004 section 9), for the paused frame of the editor and for the views of an agent. It
// owns a work directory and, while a frame is being made, FFmpeg processes: Close ends both. It is
// not safe for concurrent use.
type Renderer struct {
	s  *state
	rd *renderer
}

// NewRenderer makes the renderer and its work directory. Each frame is independent:
// a decoder is not kept open for the next frame. ViewFrame and the paused frame use this.
func NewRenderer(ctx context.Context, cfg RenderConfig) (*Renderer, error) {
	return newRenderer(ctx, cfg, true)
}

// NewSequenceRenderer is NewRenderer for frames asked in order, which is how an
// export reads a clip. Decoders stay open and read ahead. A jump backwards, or
// of more than the restart gap, starts the process at the new frame. Close ends
// them. It is not safe for concurrent use, same as Renderer.
func NewSequenceRenderer(ctx context.Context, cfg RenderConfig) (*Renderer, error) {
	return newRenderer(ctx, cfg, false)
}

func newRenderer(ctx context.Context, cfg RenderConfig, single bool) (*Renderer, error) {
	if cfg.Runner == nil || cfg.Doc == nil || cfg.ArtifactsDir == "" {
		return nil, exportErr(
			ReasonRender,
			"The render job is incomplete.",
			errors.New("runner, document and artifacts are required"),
			"",
		)
	}
	dir, err := cfg.Runner.NewJobRunDir(cfg.JobID)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the frame could not be made.", "")
	}
	job := &Job{Runner: cfg.Runner, Doc: cfg.Doc, JobID: cfg.JobID, ArtifactsDir: cfg.ArtifactsDir, Fonts: cfg.Fonts}
	s := newState(ctx, job, dir, endFrame(cfg.Doc))
	s.single = single

	return &Renderer{s: s, rd: s.newRenderer(cfg.Bands)}, nil
}

// Render composes frame t at outScale times the project's size. The result is RGBA of
// round(width*outScale) by round(height*outScale) pixels. Uncovered pixels are opaque unless the
// plan is transparent, in which case their alpha is 0.
func (r *Renderer) Render(ctx context.Context, t int, outScale float64) (*image.RGBA, error) {
	st := r.s.job.Doc.Settings
	w, h := max(round(float64(st.Width)*outScale), 1), max(round(float64(st.Height)*outScale), 1)

	return r.render(ctx, t, outScale, w, h)
}

// RenderSize composes frame t into exactly w by h pixels, scaled by w over the project width, for a
// caller that owns the rounding of the height (the size table of SDD-003 section 3.11). The layers are
// placed at the width scale, so the picture differs from Render only by the rounding of the last row.
func (r *Renderer) RenderSize(ctx context.Context, t, w, h int) (*image.RGBA, error) {
	return r.render(ctx, t, float64(w)/float64(r.s.job.Doc.Settings.Width), max(w, 1), max(h, 1))
}

func (r *Renderer) render(ctx context.Context, t int, outScale float64, w, h int) (*image.RGBA, error) {
	r.s.ctx, r.s.scale = ctx, outScale
	// The plan rasterizes text and captions for the size they are drawn at (SDD-004 section 5.3); at
	// the project size the scale is 1, as in an export.
	r.s.opts.OutputScale = outScale
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := r.rd.render(dst, t); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, wrapRun(err, ReasonRender, "A frame of the project could not be composed.", "")
	}
	return dst, nil
}

// Close stops the decoders and removes the work directory.
func (r *Renderer) Close() error {
	r.rd.close()
	err := r.s.dir.Remove()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
