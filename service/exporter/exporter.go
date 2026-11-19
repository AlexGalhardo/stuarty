// Package exporter adapts engine/export.Run to service.Exporter: it supplies
// the pieces the engine needs (runner, capabilities, artifacts folder),
// reports its progress, and turns the verified result into the output the
// service checks again before a job may succeed (ADR-0018 item 6).
package exporter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
)

// Config is what an export needs besides the project.
type Config struct {
	Runner *ffmpeg.Runner
	// HardwareRunner is the runner for an export with a hardware encoder, whose child has no
	// address space limit (SDD-007 section 6.5). Nil means Runner serves every encoder. The
	// encoder is chosen after boot, so the choice is made here, per export.
	HardwareRunner *ffmpeg.Runner
	// Caps is ffmpeg's capability list; read from the runner on first use when nil.
	Caps *ffmpeg.Capabilities
	// Workspace is the workspace folder; the normalized media is under its artifacts folder.
	Workspace string
	// Encoder is STUARTY_ENCODER and STUARTY_ALLOW_USER_X264.
	Encoder ffmpeg.DetectOptions
	// Lint is the environment of the lint that gates the export.
	Lint lint.Env
	// PixelRenderer is STUARTY_EXPORT_RENDERER; the zero value is the Go compositor.
	PixelRenderer export.PixelRenderer
	// GPUHost is the absolute path of stuarty-render, found by the owner; empty keeps the CPU path.
	GPUHost string
}

type adapter struct {
	cfg  Config
	mu   sync.Mutex
	caps *ffmpeg.Capabilities
}

// New returns the Exporter over the engine.
func New(cfg Config) service.Exporter { return &adapter{cfg: cfg, caps: cfg.Caps} }

var _ service.Exporter = (*adapter)(nil)

// Export implements service.Exporter. A failure is returned as the engine's
// *ffmpeg.Error, which the service maps to the code of the job.
func (a *adapter) Export(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error) {
	if a.cfg.Runner == nil {
		return nil, internal("no FFmpeg runner is configured")
	}

	if in.Doc == nil {
		return nil, internal("no document to export")
	}

	caps, err := a.capabilities(ctx)
	if err != nil {
		return nil, err
	}

	res, err := export.Run(ctx, a.job(in, a.runnerFor(ctx, caps), caps))
	if err != nil {
		return nil, err
	}

	sum, err := sha256File(res.Path)
	if err != nil {
		_ = os.Remove(res.Path)

		return nil, internal("the exported file could not be read back")
	}

	out := &service.ExportOutput{
		Bytes: res.Size, SHA256: sum, DurationMS: res.DurationMS, Frames: res.Frames,
	}
	out.VideoCodec, out.AudioCodec, out.Checks = describe(res, in.Doc.Settings)
	out.Width, out.Height = res.Width, res.Height

	if l := res.Loudness.IntegratedLUFS; l != nil {
		v := *l + res.Loudness.GainDB
		if res.Loudness.OutputLUFS != nil {
			v = *res.Loudness.OutputLUFS
		}

		out.LoudnessLUFS = &v
	}

	return out, nil
}

// gifDimensions is the detail of the dimensions check of a GIF. A GIF is capped in width and in
// frame rate (SDD-004 section 8.1), and the check says so when the file is smaller than the project.
func gifDimensions(res *export.Result, size string, st timeline.Settings) string {
	if res.Width == 0 || res.Width == st.Width && res.Height == st.Height {
		return size
	}

	return fmt.Sprintf("%dx%d (reduced from the project's %s; a GIF is capped at 480 pixels wide and 15 fps)",
		res.Width, res.Height, size)
}

// describe names the codecs of the verified file and the checks that the engine ran for its
// format (SDD-004 section 8.1): the engine refuses a file that fails them, so a result means
// they passed.
func describe(res *export.Result, st timeline.Settings) (video, audio string, checks []service.VerifyCheck) {
	dur := service.VerifyCheck{
		Name: "duration", OK: true, Detail: fmt.Sprintf("%.3f s, %d frames", float64(res.DurationMS)/1000, res.Frames),
	}
	size := strconv.Itoa(st.Width) + "x" + strconv.Itoa(st.Height)

	switch res.Format {
	case export.FormatGIF:
		return "gif", "", []service.VerifyCheck{
			{Name: "streams", OK: true, Detail: "one video stream, no audio"},
			dur,
			{Name: "dimensions", OK: true, Detail: gifDimensions(res, size, st)},
		}
	case export.FormatWAV:
		return "", "pcm_s16le", []service.VerifyCheck{
			{Name: "streams", OK: true, Detail: "one audio stream, 48000 Hz stereo"}, dur,
		}
	case export.FormatAAC:
		return "", "aac", []service.VerifyCheck{
			{Name: "streams", OK: true, Detail: "one audio stream, 48000 Hz stereo"},
			dur,
			{Name: "faststart", OK: true, Detail: "moov before mdat"},
		}
	case export.FormatMP4:
	}

	return "h264", "aac", []service.VerifyCheck{
		{Name: "streams", OK: true, Detail: "video and audio present"},
		dur,
		{Name: "dimensions", OK: true, Detail: size},
		{Name: "pixel_format", OK: true, Detail: "yuv420p"},
		{Name: "faststart", OK: true, Detail: "moov before mdat"},
	}
}

// job is the engine job of one export.
func (a *adapter) job(in service.ExportInput, runner *ffmpeg.Runner, caps *ffmpeg.Capabilities) export.Job {
	return export.Job{
		Runner:        runner,
		Caps:          caps,
		Doc:           in.Doc,
		Start:         int(in.Start),
		End:           int(in.End),
		JobID:         in.JobID,
		ArtifactsDir:  filepath.Join(a.cfg.Workspace, "artifacts"),
		OutPath:       in.OutPath,
		Format:        export.Format(in.Format),
		MaxBytes:      in.MaxBytes,
		Encoder:       a.cfg.Encoder,
		Lint:          a.cfg.Lint,
		PixelRenderer: a.cfg.PixelRenderer,
		GPUHost:       a.cfg.GPUHost,
		Fonts:         service.FontSetFor(a.cfg.Workspace, in.Doc),
		OnProgress: func(p export.Progress) {
			if in.OnProgress != nil {
				in.OnProgress(p.Frame, p.Total)
			}
		},
	}
}

// runnerFor picks the runner by the encoder detection finds: a software encoder (or none) runs
// under the address space limit, a hardware encoder does not. Detection runs on the hardware
// runner, whose child has no such limit, so a driver that maps a large address range is not
// mistaken for a missing encoder; a detection error leaves the stricter runner to report it.
func (a *adapter) runnerFor(ctx context.Context, caps *ffmpeg.Capabilities) *ffmpeg.Runner {
	hw := a.cfg.HardwareRunner
	if hw == nil {
		return a.cfg.Runner
	}

	det, err := hw.DetectEncoder(ctx, caps, a.cfg.Encoder)

	return pickRunner(det, err, a.cfg.Runner, hw)
}

// pickRunner is the hardware runner only for a detected encoder that is known to be hardware:
// anything else, an unknown name and a failed detection included, keeps the address space limit.
func pickRunner(det *ffmpeg.EncoderDetection, err error, software, hardware *ffmpeg.Runner) *ffmpeg.Runner {
	if err != nil || det == nil || !slices.Contains(hardwareEncoders, det.Chosen) {
		return software
	}

	return hardware
}

// hardwareEncoders are the H.264 encoders whose drivers map very large address ranges.
var hardwareEncoders = []string{
	"h264_videotoolbox", "h264_nvenc", "h264_qsv", "h264_amf", "h264_mf", "h264_vaapi",
}

// capabilities reads what ffmpeg can do once, on the first export.
func (a *adapter) capabilities(ctx context.Context) (*ffmpeg.Capabilities, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.caps != nil {
		return a.caps, nil
	}

	caps, err := a.cfg.Runner.ReadCapabilities(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, internal("the FFmpeg capabilities could not be read")
	}

	a.caps = caps

	return caps, nil
}

func internal(msg string) *ffmpeg.Error {
	return &ffmpeg.Error{
		Code:    "E_INTERNAL",
		Reason:  "internal",
		Message: "Exporting failed inside the server.",
		Err:     fmt.Errorf("%s", msg),
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
