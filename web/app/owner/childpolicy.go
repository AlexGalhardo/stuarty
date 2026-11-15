package owner

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
)

// softwareEncoders are the H.264 encoders that run on the CPU: RLIMIT_AS applies to an
// export with one of them and not to a hardware encoder (SDD-007 section 6.5).
var softwareEncoders = []string{"libopenh264", "libx264"}

// childRunners are the runners of the four kinds of FFmpeg run. The pinned contrib/engine applies
// limits per tool, so a kind with other limits needs a pair of its own (ffmpeg.Installation.WithLimits).
type childRunners struct {
	// Ingest is the runner of the decoding runs: ingest, the audio extraction of a transcription,
	// encoder detection and the doctor.
	Ingest, Frame, Export *ffmpeg.Runner
	Policy                service.ChildPolicy
}

// newChildRunners applies SDD-007 sections 6.5 and 6.7 to an installation: prlimit(1) limits where
// they exist, and the bwrap sandbox when VUKEM_VIDEO_SANDBOX asks for it. A missing prlimit on Linux
// and an unusable bwrap are boot errors; there is never a child that is silently unlimited.
func newChildRunners(
	ctx context.Context,
	in *ffmpeg.Installation,
	workspace, sandboxMode, forcedEncoder string,
	maxExportBytes int64,
) (*childRunners, error) {
	sandboxMode = strings.TrimSpace(sandboxMode)
	if strings.EqualFold(sandboxMode, "none") {
		sandboxMode = ""
	}

	sandbox, err := ffmpeg.ResolveSandbox(ctx, strings.ToLower(sandboxMode))
	if err != nil {
		return nil, fmt.Errorf("owner: sandbox requested but unavailable: %w", err)
	}

	limitsOK, why := ffmpeg.LimitsAvailable()
	if !limitsOK && runtime.GOOS == "linux" {
		return nil, fmt.Errorf("owner: %s: the limits of SDD-007 section 6.5 need it, and children are never started unlimited", why)
	}

	exportKind := ffmpeg.KindExportHardware
	if slices.Contains(softwareEncoders, forcedEncoder) {
		exportKind = ffmpeg.KindExportSoftware
	}

	cpus := in.CPUCount

	// The deadline of a run is only known when it starts, so the CPU time backstop is the
	// ceiling of its kind; the per-run deadline is the context's.
	kinds := []struct {
		kind      ffmpeg.RunKind
		deadline  time.Duration
		fileBytes int64
	}{
		{ffmpeg.KindIngest, ffmpeg.IngestDeadline(24 * time.Hour), 0},
		{ffmpeg.KindFrame, ffmpeg.FrameTimeout, 0},
		{exportKind, ffmpeg.ExportDeadline(24 * time.Hour), maxExportBytes},
	}

	var runners [3]*ffmpeg.Runner

	for i, k := range kinds {
		tooled, werr := in.WithLimits(ctx, ffmpeg.LimitsFor(k.kind, k.deadline, cpus, k.fileBytes))
		if werr != nil {
			return nil, fmt.Errorf("owner: applying the child limits: %w", werr)
		}

		r, rerr := ffmpeg.NewRunner(tooled, workspace)
		if rerr != nil {
			return nil, rerr
		}

		r.Sandbox = sandbox
		runners[i] = r
	}

	return &childRunners{
		Ingest: runners[0], Frame: runners[1], Export: runners[2],
		Policy: service.ChildPolicy{Limits: limitsOK, LimitsWhy: why, Sandbox: sandbox},
	}, nil
}
