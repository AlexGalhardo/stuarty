package owner

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
)

// childRunners are the runners of the four kinds of FFmpeg run. The pinned contrib/engine applies
// limits per tool, so a kind with other limits needs a pair of its own (ffmpeg.Installation.WithLimits).
type childRunners struct {
	// Ingest is the runner of the decoding runs: ingest, the audio extraction of a transcription,
	// encoder detection and the doctor.
	Ingest, Frame, Export *ffmpeg.Runner
	// ExportHardware runs the export when detection chooses a hardware encoder: no RLIMIT_AS.
	// Export, the default, has it.
	ExportHardware *ffmpeg.Runner
	Policy         service.ChildPolicy
}

// newChildRunners applies SDD-007 sections 6.5 and 6.7 to an installation: prlimit(1) limits where
// they exist, and the bwrap sandbox when STUARTY_SANDBOX asks for it. A missing prlimit on Linux
// and an unusable bwrap are boot errors; there is never a child that is silently unlimited.
func newChildRunners(
	ctx context.Context,
	in *ffmpeg.Installation,
	workspace, sandboxMode string,
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
		return nil, fmt.Errorf(
			"owner: %s: the limits of SDD-007 section 6.5 need it, and children are never started unlimited",
			why,
		)
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
		{ffmpeg.KindExportSoftware, ffmpeg.ExportDeadline(24 * time.Hour), maxExportBytes},
		{ffmpeg.KindExportHardware, ffmpeg.ExportDeadline(24 * time.Hour), maxExportBytes},
	}

	var runners [4]*ffmpeg.Runner

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
		Ingest: runners[0], Frame: runners[1], Export: runners[2], ExportHardware: runners[3],
		Policy: service.ChildPolicy{Limits: limitsOK, LimitsWhy: why, Sandbox: sandbox},
	}, nil
}
