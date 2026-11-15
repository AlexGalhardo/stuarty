package ffmpeg

import (
	"context"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// RunKind says what a run is for, which decides its address space ceiling (SDD-007 6.5).
type RunKind int

// The kinds of run.
const (
	KindProbe RunKind = iota
	KindIngest
	KindFrame
	// KindExportSoftware is an export with a software encoder.
	KindExportSoftware
	// KindExportHardware is an export with h264_nvenc, h264_qsv, h264_vaapi or h264_amf: GPU
	// drivers map very large address ranges, so RLIMIT_AS is not set for it.
	KindExportHardware
)

const (
	probeAddressSpace = 2 << 30
	runAddressSpace   = 8 << 30
)

// limitsFor is the table of SDD-007 6.5: RLIMIT_CPU is the deadline times the CPU count, as a
// backstop; RLIMIT_FSIZE is the largest file the run may write (zero: none); RLIMIT_AS only
// for software paths.
func limitsFor(kind RunKind, deadline time.Duration, cpus int, fileBytes int64) engine.Limits {
	l := engine.Limits{CPUTime: deadline * time.Duration(max(cpus, 1)), FileSize: fileBytes}

	switch kind {
	case KindProbe:
		l.AddressSpace = probeAddressSpace
	case KindIngest, KindFrame, KindExportSoftware:
		l.AddressSpace = runAddressSpace
	case KindExportHardware:
	}

	return l
}

// WithLimits returns a copy of the installation whose tools run under the limits. The pinned
// contrib/engine applies limits per tool, so a run kind with other limits needs its own pair.
// With no limits (the zero value, always the case outside Linux) the installation is returned as it is.
func (in *Installation) WithLimits(ctx context.Context, l engine.Limits) (*Installation, error) {
	if l == (engine.Limits{}) {
		return in, nil
	}

	retool := func(t *engine.Tool, maxOutput int) (*engine.Tool, error) {
		return engine.Find(ctx, engine.Config{
			Path: t.Path(), Limits: l, CPUCount: t.CPUCount(), MaxOutput: maxOutput,
		})
	}

	ff, err := retool(in.FFmpeg, ffmpegMaxOutput)
	if err != nil {
		return nil, err
	}

	fp, err := retool(in.FFprobe, 0)
	if err != nil {
		return nil, err
	}

	out := *in
	out.FFmpeg, out.FFprobe, out.limits = ff, fp, l

	return &out, nil
}
