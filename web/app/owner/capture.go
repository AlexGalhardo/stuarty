package owner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
)

const captureHelperName = "stuarty-capture"

// locateCaptureHelper is the path of stuarty-capture: the one STUARTY_CAPTURE_HELPER names (it must be
// absolute), else the one next to the server binary or in ../libexec/stuarty, as the GPU host is found. "" when
// there is none.
func locateCaptureHelper(configured, execDir, goos string) string {
	if configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured)
		}

		return ""
	}

	if execDir == "" || !filepath.IsAbs(execDir) {
		return ""
	}

	name := captureHelperName
	if goos == "windows" {
		name += ".exe"
	}

	for _, p := range []string{
		filepath.Join(execDir, name),
		filepath.Join(execDir, "..", "libexec", "stuarty", name),
	} {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return filepath.Clean(p)
		}
	}

	return ""
}

// recordingSeconds reads STUARTY_MAX_RECORDING_SECONDS: 1 to 14,400, unset is the default (SDD-023 section 5).
func recordingSeconds(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 || v > recording.MaxSecondsCeiling {
		return 0, fmt.Errorf(
			"owner: invalid limit: STUARTY_MAX_RECORDING_SECONDS=%q must be a whole number from 1 to %d",
			raw,
			recording.MaxSecondsCeiling,
		)
	}

	return v, nil
}

// captureLimits reads the longest recording and settles the reason there is no capture when media() returned
// before it chose one (no FFmpeg was found).
func (o *Owner) captureLimits(cfg Config) (int, error) {
	seconds, err := recordingSeconds(cfg.Getenv("STUARTY_MAX_RECORDING_SECONDS"))
	if err != nil {
		return 0, err
	}

	if o.capture.capturer == nil && o.capture.reason == "" {
		o.capture.reason = "ffmpeg_missing"

		switch {
		case o.mode == config.ModeNetwork:
			o.capture.reason = "network_mode"
		case runtime.GOOS != "windows":
			o.capture.reason = "platform"
		}
	}

	return seconds, nil
}

// captureSetup is what the service needs to capture: the Capturer, or the reason there is none.
type captureSetup struct {
	capturer service.Capturer
	reason   string
}

// buildCapture decides whether this process can capture, and builds the capture of the product when it can
// (SDD-023 section 1): only the desktop on Windows, never in network mode, and only with an FFmpeg to encode with.
func (o *Owner) buildCapture(
	cfg Config, in *ffmpeg.Installation, run *ffmpeg.Runner, opt ffmpeg.DetectOptions, execDir string,
) captureSetup {
	switch {
	case o.mode == config.ModeNetwork:
		return captureSetup{reason: "network_mode"}
	case runtime.GOOS != "windows":
		return captureSetup{reason: "platform"}
	case in == nil || run == nil:
		return captureSetup{reason: "ffmpeg_missing"}
	}

	helper := locateCaptureHelper(cfg.Getenv("STUARTY_CAPTURE_HELPER"), execDir, runtime.GOOS)

	live := &capturehost.Live{
		Runner: run, HelperPath: helper, FFmpegPath: in.FFmpeg.Path(),
		Detect: func(ctx context.Context) (*ffmpeg.EncoderDetection, error) {
			caps, err := run.ReadCapabilities(ctx)
			if err != nil {
				return nil, err
			}

			return run.DetectEncoder(ctx, caps, opt)
		},
	}

	return captureSetup{capturer: service.NewLiveCapturer(live)}
}
