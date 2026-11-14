package export

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
)

// Progress is the share of the frames written to the encoder.
type Progress struct {
	Frame, Total int
}

// Job is one export (SDD-004 section 8).
type Job struct {
	Runner *ffmpeg.Runner
	// Caps is ffmpeg's capability list, for the encoder detection.
	Caps *ffmpeg.Capabilities
	Doc  *timeline.Document
	// JobID names the run directory under <workspace>/tmp.
	JobID string
	// ArtifactsDir is <workspace>/artifacts, where the normalized media of the assets is.
	ArtifactsDir string
	// OutPath is the file the export publishes, normally exports/<project>/<job>.mp4. Nothing is
	// written there unless the exported file passed its verification.
	OutPath string
	// Encoder is VUKEM_VIDEO_ENCODER and VUKEM_VIDEO_ALLOW_USER_X264 (ADR-0011 item 5).
	Encoder ffmpeg.DetectOptions
	// Lint is the environment of the lint that gates the export (unready media, text sizes).
	Lint lint.Env
	// Fonts is the font set of the rasterizer; the bundled fonts when nil.
	Fonts *raster.FontSet
	// Bands is the number of bands of the compositor; zero is one per CPU, at most 8.
	Bands int
	// OnProgress is called at most 4 times per second and once at the last frame.
	OnProgress func(Progress)

	// afterEncode lets a test damage the partial file before it is verified.
	afterEncode func(partial string) error
}

// Result is what a successful export produced.
type Result struct {
	Path       string
	Size       int64
	DurationMS int64
	Frames     int
	Encoder    string
	Loudness   Loudness
}

// state is what the parts of one export share.
type state struct {
	job       *Job
	ctx       context.Context
	dir       *ffmpeg.RunDir
	frames    int
	fonts     *raster.FontSet
	opts      plan.Options
	deadline  time.Duration
	inputDirs []string
	children  childDirs
}

func newState(ctx context.Context, job *Job, dir *ffmpeg.RunDir, frames int) *state {
	fonts := job.Fonts
	if fonts == nil {
		fonts = raster.Bundled()
	}
	fps := job.Doc.Settings.FPS
	secs := time.Duration(float64(frames) * float64(fps.Den) / float64(fps.Num) * float64(time.Second))
	return &state{
		job: job, ctx: ctx, dir: dir, frames: frames, fonts: fonts,
		opts:      plan.Options{Measure: raster.Measure(fonts)},
		deadline:  ffmpeg.ExportDeadline(secs),
		inputDirs: assetDirs(job),
	}
}

// renderer composes the frames of the project: the compositor with its decoders and bitmaps. The
// export and the single frames of the editor (SDD-004 section 9) draw through the same one.
type renderer struct {
	s    *state
	comp *compositor.Compositor
	fs   *frameSource
}

func (s *state) newRenderer(bands int) *renderer {
	fs := newFrameSource(s, s.scanLayers())
	cache := raster.NewCache(s.fonts, 0)
	return &renderer{s: s, fs: fs, comp: &compositor.Compositor{Media: fs.media, Bitmaps: cache.Get, Bands: bands}}
}

// render composes frame t into dst, then stops the decoders of the clips that are no longer drawn.
func (r *renderer) render(dst *image.RGBA, t int) error {
	err := r.comp.Compose(dst, plan.FrameWith(r.s.job.Doc, t, r.s.opts), 1)
	r.fs.endFrame()
	return err
}

func (r *renderer) close() { r.fs.close() }

// Run exports the project: it checks the lint, mixes the audio, composes every frame into the
// encoder, verifies the file and publishes it (SDD-004 section 8). On any failure, and on
// cancellation, nothing is left under OutPath and the work directory is removed.
func Run(ctx context.Context, job Job) (*Result, error) {
	res, err := run(ctx, &job)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, fmt.Errorf("export: %w", cerr)
		}
		return nil, err
	}
	return res, nil
}

func run(ctx context.Context, job *Job) (*Result, error) {
	if job.Runner == nil || job.Doc == nil || job.OutPath == "" || job.ArtifactsDir == "" {
		return nil, exportErr(
			ReasonRender,
			"The export job is incomplete.",
			errors.New("runner, document, artifacts and output path are required"),
			"",
		)
	}
	doc := job.Doc

	// 1. Check.
	var blocking []lint.Finding
	for _, f := range lint.Lint(doc, job.Lint, lint.Full) {
		if lint.Blocking(f.Code) {
			blocking = append(blocking, f)
		}
	}
	if len(blocking) > 0 {
		return nil, lintError(blocking)
	}
	frames := endFrame(doc)

	// 2. Encoder.
	det, err := job.Runner.DetectEncoder(ctx, job.Caps, job.Encoder)
	if err != nil {
		return nil, err
	}
	cand, ok := det.ChosenCandidate()
	if !ok {
		return nil, exportErr(ReasonEncoder, "No encoder was chosen.", nil, "")
	}

	dir, err := job.Runner.NewRunDir(job.JobID)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the export could not be made.", "")
	}
	defer func() { _ = dir.Remove() }()

	s := newState(ctx, job, dir, frames)

	// 3. Audio.
	mix, loud, err := s.mixAudio(ctx)
	if err != nil {
		return nil, err
	}

	// 4. Video.
	rd := s.newRenderer(job.Bands)
	defer rd.close()
	partial, err := dir.Path(job.JobID + ".partial.mp4")
	if err != nil {
		return nil, err
	}
	if err := s.encode(ctx, cand, mix, partial, rd); err != nil {
		return nil, err
	}
	rd.close()
	if job.afterEncode != nil {
		if err := job.afterEncode(partial); err != nil {
			return nil, err
		}
	}

	// 7. Verification.
	ver, err := s.verify(ctx, partial, frames)
	if err != nil {
		_ = os.Remove(partial)
		return nil, err
	}

	// 8. Publish.
	if err := publish(partial, job.OutPath); err != nil {
		return nil, exportErr(ReasonPublish, "The exported file could not be moved into place.", err, "")
	}
	return &Result{
		Path:       job.OutPath,
		Size:       ver.Size,
		DurationMS: ver.DurationMS,
		Frames:     frames,
		Encoder:    cand.Name,
		Loudness:   loud,
	}, nil
}

// endFrame is the end of the last item: the number of frames of the export.
func endFrame(doc *timeline.Document) int {
	end := 0
	for id := range doc.Items {
		it := doc.Items[id]
		end = max(end, it.Start+it.Duration)
	}
	return end
}

// assetDirs are the artifact folders of the assets, where the children may read the normalized media.
func assetDirs(job *Job) []string {
	seen := map[string]bool{}
	var dirs []string
	for id := range job.Doc.Assets {
		hash, ok := strings.CutPrefix(job.Doc.Assets[id].Hash, "sha256:")
		if !ok || seen[hash] {
			continue
		}
		seen[hash] = true
		dirs = append(dirs, filepath.Join(job.ArtifactsDir, hash))
	}
	return dirs
}

// publish renames the verified file into place. When the rename fails (the output is on another volume)
// the file is copied next to its destination and renamed from there, so a reader never sees half of it.
func publish(partial, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	if err := os.Rename(partial, out); err == nil {
		return nil
	}
	tmp := filepath.Join(filepath.Dir(out), "."+filepath.Base(out)+".part")
	if err := copyFile(partial, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
