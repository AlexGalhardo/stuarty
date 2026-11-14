package export

import (
	"context"
	"errors"
	"image"
	"os"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
)

// RenderConfig is what a Renderer needs: the same inputs as an export, without the output.
type RenderConfig struct {
	Runner       *ffmpeg.Runner
	Doc          *timeline.Document
	JobID        string
	ArtifactsDir string
	Fonts        *raster.FontSet // the bundled fonts when nil
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

// NewRenderer makes the renderer and its work directory.
func NewRenderer(ctx context.Context, cfg RenderConfig) (*Renderer, error) {
	if cfg.Runner == nil || cfg.Doc == nil || cfg.ArtifactsDir == "" {
		return nil, exportErr(
			ReasonRender,
			"The render job is incomplete.",
			errors.New("runner, document and artifacts are required"),
			"",
		)
	}
	dir, err := cfg.Runner.NewRunDir(cfg.JobID)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the frame could not be made.", "")
	}
	job := &Job{Runner: cfg.Runner, Doc: cfg.Doc, JobID: cfg.JobID, ArtifactsDir: cfg.ArtifactsDir, Fonts: cfg.Fonts}
	s := newState(ctx, job, dir, endFrame(cfg.Doc))
	s.single = true
	return &Renderer{s: s, rd: s.newRenderer(cfg.Bands)}, nil
}

// Render composes frame t at outScale times the project's size. The result is opaque RGBA of
// round(width*outScale) by round(height*outScale) pixels.
func (r *Renderer) Render(ctx context.Context, t int, outScale float64) (*image.RGBA, error) {
	st := r.s.job.Doc.Settings
	r.s.ctx, r.s.scale = ctx, outScale
	dst := image.NewRGBA(
		image.Rect(0, 0, max(round(float64(st.Width)*outScale), 1), max(round(float64(st.Height)*outScale), 1)),
	)
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
