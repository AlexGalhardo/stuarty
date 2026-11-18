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
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
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
	// ExportsDir and ProjectID make the destination: <ExportsDir>/<ProjectID>/<JobID>.mp4, from
	// identifiers that are checked, written through os.Root (SDD-004 section 8 step 8). There is no
	// way to name another destination. Nothing is written there unless the file passed its verification.
	ExportsDir string
	ProjectID  string
	// OutPath is the old way to name the destination, used only when ExportsDir is empty; the service
	// moves to ExportsDir and ProjectID and OutPath is then removed.
	OutPath string
	// Format is what the export writes; the zero value is FormatMP4.
	Format Format
	// MaxBytes is VUKEM_VIDEO_MAX_EXPORT_BYTES: a larger file ends the job with reason "size_limit".
	// Zero is no limit.
	MaxBytes int64
	// Encoder is VUKEM_VIDEO_ENCODER and VUKEM_VIDEO_ALLOW_USER_X264 (ADR-0011 item 5).
	Encoder ffmpeg.DetectOptions
	// Start and End are the frames [Start, End) of the timeline to export. End 0 is the end of the
	// last item; Start 0 and End 0 export the whole timeline (VV-018).
	Start, End int
	// Lint is the environment of the lint that gates the export (unready media, text sizes).
	Lint lint.Env
	// Fonts is the font set of the rasterizer; the bundled fonts when nil.
	Fonts *rasterize.FontSet
	// Bands is the number of bands of the compositor; zero is one per CPU, at most 8.
	Bands int
	// PixelRenderer is VUKEM_VIDEO_EXPORT_RENDERER: the Go compositor (the zero value) or the Rust renderer
	// running as WebAssembly under wazero (SDD-013 section 4.6). Both draw the same bytes.
	PixelRenderer PixelRenderer
	// GPUHost is the absolute path of the native GPU host vukem-render. With PixelRendererGPU and a path the
	// frames are drawn by the child when the machine has a graphics adapter, and by the WebAssembly module
	// otherwise (SDD-013 section 4.9); empty means no GPU host.
	GPUHost string
	// OnProgress is called at most 4 times per second and once at the last frame.
	OnProgress func(Progress)

	// afterEncode lets a test damage the partial file before it is verified.
	afterEncode func(partial string) error
}

// Result is what a successful export produced.
type Result struct {
	Path       string
	Format     Format
	Size       int64
	DurationMS int64
	Frames     int
	Encoder    string
	Loudness   Loudness
	// Width and Height are the pixels of the file when they differ from the project's (a GIF is
	// capped at 480 pixels wide); zero means the project's size.
	Width, Height int
}

// state is what the parts of one export share.
type state struct {
	job    *Job
	ctx    context.Context
	dir    *ffmpeg.RunDir
	frames int
	// offset is the first timeline frame of the export; export frame i draws timeline frame offset+i.
	offset int
	fonts  *rasterize.FontSet
	opts   plan.Options
	// scale is the output scale of the frames; 1 is the project's size. single says one frame is
	// asked for, so every unrotated layer is as good as constant and the decoder scales it.
	scale     float64
	single    bool
	deadline  time.Duration
	inputDirs []string
	children  childDirs
}

func newState(ctx context.Context, job *Job, dir *ffmpeg.RunDir, frames int) *state {
	fonts := job.Fonts
	if fonts == nil {
		fonts = rasterize.Bundled()
	}
	fps := job.Doc.Settings.FPS
	secs := time.Duration(float64(frames) * float64(fps.Den) / float64(fps.Num) * float64(time.Second))
	return &state{
		job: job, ctx: ctx, dir: dir, frames: frames, fonts: fonts, scale: 1,
		opts:      plan.Options{Measure: rasterize.Measure(fonts)},
		deadline:  exportDeadline(secs),
		inputDirs: assetDirs(job),
	}
}

// renderer composes the frames of the project: the compositor with its decoders and bitmaps. The
// export and the single frames of the editor (SDD-004 section 9) draw through the same one.
type renderer struct {
	s    *state
	comp *compositor.Compositor
	fs   *frameSource
	// rust draws instead of comp when the job asks for the Rust renderer.
	rust *rustRenderer
}

func (s *state) newRenderer(bands int) *renderer {
	var infos map[string]*layerInfo
	if !s.single {
		infos = s.scanLayers()
	}
	fs := newFrameSource(s, infos)
	cache := rasterize.NewCache(s.fonts, 0)
	rd := &renderer{
		s:    s,
		fs:   fs,
		comp: &compositor.Compositor{Media: fs.media, Bitmaps: cache.Get, LUTs: fs.lut, Bands: bands},
	}
	if s.job.PixelRenderer == PixelRendererRust || s.job.PixelRenderer == PixelRendererGPU {
		rd.rust = newRustRenderer(s, fs, cache.Get, bands)
	}

	return rd
}

// render composes frame t into dst, then stops the decoders of the clips that are no longer drawn.
func (r *renderer) render(dst *image.RGBA, t int) error {
	fp := plan.FrameWith(r.s.job.Doc, r.s.offset+t, r.s.opts)
	if r.s.single {
		r.fs.prestart(&fp)
	}
	var err error
	if r.rust != nil {
		err = r.rust.compose(dst, fp, r.s.scale)
	} else {
		err = r.comp.Compose(dst, fp, r.s.scale)
	}
	r.fs.endFrame()
	return err
}

func (r *renderer) close() {
	if r.rust != nil {
		r.rust.close()
	}
	r.fs.close()
}

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
	if job.Runner == nil || job.Doc == nil || job.ArtifactsDir == "" || (job.OutPath == "") == (job.ExportsDir == "") {
		return nil, exportErr(
			ReasonRender,
			"The export job is incomplete.",
			errors.New("runner, document, artifacts and exactly one of ExportsDir and OutPath are required"),
			"",
		)
	}

	if job.ExportsDir != "" {
		if _, err := exportPath(job.ExportsDir, job.ProjectID, job.JobID); err != nil {
			return nil, exportErr(ReasonBadOutput, "The export destination is not valid.", err, "")
		}
	}
	if !job.Format.Valid() {
		return nil, exportErr(
			ReasonBadFormat,
			"The export format is not one of mp4, gif, wav and aac.",
			errors.New("unknown format"),
			"",
		)
	}
	format := job.Format.normal()
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
	start, end, err := exportRange(job, endFrame(doc))
	if err != nil {
		return nil, err
	}
	frames := end - start
	if err := checkLimits(doc.Settings); err != nil {
		return nil, err
	}

	dir, err := job.Runner.NewJobRunDir(job.JobID)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the export could not be made.", "")
	}
	defer func() { _ = dir.Remove() }()

	s := newState(ctx, job, dir, frames)
	s.offset = start

	b, err := s.build(ctx, format)
	if err != nil {
		return nil, err
	}
	partial, ver := b.partial, b.ver

	if job.MaxBytes > 0 && ver.Size > job.MaxBytes {
		_ = os.Remove(partial)

		return nil, sizeLimitError(ver.Size, job.MaxBytes)
	}

	// 8. Publish.
	out := job.OutPath
	if job.ExportsDir != "" {
		out, err = publishUnder(job.ExportsDir, job.ProjectID, job.JobID, format.Ext(), partial)
	} else {
		err = publish(partial, out)
	}
	if err != nil {
		return nil, exportErr(ReasonPublish, "The exported file could not be moved into place.", err, "")
	}
	return &Result{
		Path:       out,
		Size:       ver.Size,
		DurationMS: ver.DurationMS,
		Frames:     frames,
		Format:     format,
		Encoder:    b.encoder,
		Loudness:   b.loud,
		Width:      ver.Width,
		Height:     ver.Height,
	}, nil
}

// exportRange is the frames [start, end) the job exports out of a timeline of total frames.
func exportRange(job *Job, total int) (start, end int, err error) {
	start, end = job.Start, job.End
	if end == 0 {
		end = total
	}
	if start < 0 || start >= end || end > total {
		return 0, 0, exportErr(
			ReasonRender,
			"The range to export is outside the timeline.",
			fmt.Errorf("range [%d, %d) of a timeline of %d frames", start, end, total),
			"",
		)
	}
	return start, end, nil
}

// endFrame is the end of the last item: the number of frames of the export.
// exportDeadline is ffmpeg.ExportDeadline; only the tests under the race detector stretch it.
var exportDeadline = ffmpeg.ExportDeadline

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

// built is a verified partial file and what the build learned on the way.
type built struct {
	partial string
	encoder string
	loud    Loudness
	ver     Verified
}

// build makes and verifies the partial file of the format (steps 2 to 7 of SDD-004 section 8).
func (s *state) build(ctx context.Context, f Format) (built, error) {
	switch f {
	case FormatGIF:
		rd := s.newRenderer(s.job.Bands)
		defer rd.close()

		return s.buildGIF(ctx, rd)
	case FormatWAV, FormatAAC:
		return s.buildAudio(ctx, f)
	case FormatMP4:
	}

	return s.buildMP4(ctx)
}

func (s *state) buildMP4(ctx context.Context) (built, error) {
	job := s.job

	// 2. Encoder.
	det, err := job.Runner.DetectEncoder(ctx, job.Caps, job.Encoder)
	if err != nil {
		return built{}, err
	}

	if _, ok := det.ChosenCandidate(); !ok {
		return built{}, exportErr(ReasonEncoder, "No encoder was chosen.", nil, "")
	}

	cand, err := s.checkCapability(ctx, det)
	if err != nil {
		return built{}, err
	}

	// 3. Audio.
	mix, loud, err := s.mixAudio(ctx)
	if err != nil {
		return built{}, err
	}

	// 4. Video.
	rd := s.newRenderer(job.Bands)
	defer rd.close()

	partial, err := s.dir.Path(job.JobID + ".partial.mp4")
	if err != nil {
		return built{}, err
	}

	if err := s.encode(ctx, cand, mix, partial, rd); err != nil {
		return built{}, err
	}

	rd.close()

	if job.afterEncode != nil {
		if err := job.afterEncode(partial); err != nil {
			return built{}, err
		}
	}

	// 7. Verification.
	ver, err := s.verify(ctx, partial, s.frames)
	if err != nil {
		_ = os.Remove(partial)

		return built{}, err
	}

	return built{partial: partial, encoder: cand.Name, loud: loud, ver: ver}, nil
}
