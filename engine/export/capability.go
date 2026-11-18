package export

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

// The ceilings of SDD-007 section 9.1: the document accepts no more, and the export checks again.
const (
	maxSide = 4096
	maxFPS  = 120
)

// ReasonUnsupportedSize is details.reason of an export the chosen encoder cannot do at the size or the
// frame rate of the project.
const ReasonUnsupportedSize = "unsupported_size"

// needsSizeTest says whether the detection test encode (1080x1920 at 30 fps, SDD-011 section 8.4) says
// enough: a project that is neither larger nor faster than that is covered by it.
func needsSizeTest(st timeline.Settings) bool {
	return st.Width*st.Height > refPixels || st.FPS.Num > 30*st.FPS.Den
}

// checkLimits refuses a project beyond the ceilings of SDD-007 section 9.1.
func checkLimits(st timeline.Settings) error {
	if st.Width > maxSide || st.Height > maxSide || st.FPS.Num > maxFPS*st.FPS.Den {
		return exportErr(
			ReasonUnsupportedSize,
			fmt.Sprintf(
				"The project is larger or faster than the export allows (%d pixels per side, %d fps).",
				maxSide,
				maxFPS,
			),
			errors.New("settings over the export ceilings"),
			"",
		)
	}

	return nil
}

// checkCapability runs the cached test encode at the size and the rate of the project when the project is
// larger or faster than the detection test (RF-064). An encoder that cannot do it is replaced by the next
// working one (D-034, Runner.EncoderForSize); when none can, the error names the encoder that was chosen
// and says what to change. The result is the encoder to use.
func (s *state) checkCapability(ctx context.Context, det *ffmpeg.EncoderDetection) (ffmpeg.Candidate, error) {
	chosen, _ := det.ChosenCandidate()

	st := s.job.Doc.Settings
	if !needsSizeTest(st) {
		return chosen, nil
	}

	cand, err := s.job.Runner.EncoderForSize(ctx, det, ffmpeg.SizeProbe{
		Width: st.Width, Height: st.Height, FPSNum: st.FPS.Num, FPSDen: st.FPS.Den,
	})
	if err == nil {
		return cand, nil
	}

	if ctx.Err() != nil || !errors.Is(err, ffmpeg.ErrEncoderSize) {
		return chosen, wrapRun(err, ReasonEncoder, "The encoder could not be tested at the size of the project.", "")
	}

	msg := fmt.Sprintf(
		"The encoder %s on this machine cannot encode %dx%d at %.4g fps.",
		chosen.Name, st.Width, st.Height, float64(st.FPS.Num)/float64(st.FPS.Den),
	)
	e := exportErr(ReasonUnsupportedSize, msg, err, "")
	e.Hint = "Lower the project size or the frame rate, or install a build with an encoder that supports it " +
		"(\"vukem-video doctor\" lists the encoders and why each one was or was not chosen)."

	return chosen, e
}
