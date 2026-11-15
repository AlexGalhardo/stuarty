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
	"strconv"
	"sync"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/engine/export"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
)

// Config is what an export needs besides the project.
type Config struct {
	Runner *ffmpeg.Runner
	// Caps is ffmpeg's capability list; read from the runner on first use when nil.
	Caps *ffmpeg.Capabilities
	// Workspace is the workspace folder; the normalized media is under its artifacts folder.
	Workspace string
	// Encoder is VUKEM_VIDEO_ENCODER and VUKEM_VIDEO_ALLOW_USER_X264.
	Encoder ffmpeg.DetectOptions
	// Lint is the environment of the lint that gates the export.
	Lint lint.Env
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

	// The engine renders the whole timeline; a range is not a feature of the
	// export yet, and rendering more than was asked would be a wrong answer.
	if in.Doc == nil {
		return nil, internal("no document to export")
	}

	if last := lastFrame(in); in.Start != 0 || in.End != 0 && in.End != last {
		return nil, &ffmpeg.Error{
			Code: export.CodeExportFailed, Reason: "range_unsupported",
			Message: "Exporting a range of the timeline is not available in this build.",
			Hint:    "Export the whole timeline.",
			Details: map[string]any{"reason": "range_unsupported"},
		}
	}

	caps, err := a.capabilities(ctx)
	if err != nil {
		return nil, err
	}

	res, err := export.Run(ctx, export.Job{
		Runner:       a.cfg.Runner,
		Caps:         caps,
		Doc:          in.Doc,
		JobID:        in.JobID,
		ArtifactsDir: filepath.Join(a.cfg.Workspace, "artifacts"),
		OutPath:      in.OutPath,
		MaxBytes:     in.MaxBytes,
		Encoder:      a.cfg.Encoder,
		Lint:         a.cfg.Lint,
		OnProgress: func(p export.Progress) {
			if in.OnProgress != nil {
				in.OnProgress(p.Frame, p.Total)
			}
		},
	})
	if err != nil {
		return nil, err
	}

	sum, err := sha256File(res.Path)
	if err != nil {
		_ = os.Remove(res.Path)

		return nil, internal("the exported file could not be read back")
	}

	st := in.Doc.Settings

	out := &service.ExportOutput{
		Bytes: res.Size, SHA256: sum, DurationMS: res.DurationMS, Frames: res.Frames,
		VideoCodec: "h264", AudioCodec: "aac",
		Checks: []service.VerifyCheck{
			{Name: "streams", OK: true, Detail: "video and audio present"},
			{
				Name:   "duration",
				OK:     true,
				Detail: fmt.Sprintf("%.3f s, %d frames", float64(res.DurationMS)/1000, res.Frames),
			},
			{Name: "dimensions", OK: true, Detail: strconv.Itoa(st.Width) + "x" + strconv.Itoa(st.Height)},
			{Name: "pixel_format", OK: true, Detail: "yuv420p"},
			{Name: "faststart", OK: true, Detail: "moov before mdat"},
		},
	}

	if l := res.Loudness.IntegratedLUFS; l != nil {
		v := *l + res.Loudness.GainDB
		out.LoudnessLUFS = &v
	}

	return out, nil
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

func lastFrame(in service.ExportInput) int64 {
	var end int64

	for id := range in.Doc.Items {
		it := in.Doc.Items[id]
		end = max(end, int64(it.Start+it.Duration))
	}

	return end
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
