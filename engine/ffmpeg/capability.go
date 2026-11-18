package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

// SizeProbe is the size and the frame rate a test encode is asked for.
type SizeProbe struct {
	Width, Height  int
	FPSNum, FPSDen int
}

// ErrEncoderSize means the chosen encoder could not encode the size and rate that was asked for.
var ErrEncoderSize = errors.New("ffmpeg: the encoder cannot encode this size and frame rate")

type sizeKey struct {
	encoder string
	SizeProbe
}

// TestEncoderAt runs the test encode of SDD-011 8.4 at another size and rate (an export at 4K or at
// 60 fps, SDD-004 section 8) on the candidate that detection chose, and checks the 15 packets it
// wrote. The answer, good or bad, is cached for the life of the runner; a cancelled run is not. A
// failure wraps ErrEncoderSize and carries the first line of what the encoder said.
func (r *Runner) TestEncoderAt(ctx context.Context, c Candidate, p SizeProbe) error {
	key := sizeKey{c.Name, p}

	r.capMu.Lock()
	defer r.capMu.Unlock()

	if err, ok := r.capCache[key]; ok {
		return err
	}

	err := r.testEncoderAt(ctx, c, p)
	if ctx.Err() == nil {
		if r.capCache == nil {
			r.capCache = map[sizeKey]error{}
		}

		r.capCache[key] = err
	}

	return err
}

func (r *Runner) testEncoderAt(ctx context.Context, c Candidate, p SizeProbe) error {
	dir, err := r.newInternalRunDir("encoder-size")
	if err != nil {
		return err
	}

	defer func() { _ = dir.Remove() }()

	out, err := dir.Path("probe-size.mp4")
	if err != nil {
		return err
	}

	argv, err := ffargs.EncoderTestAt(
		r.Policy(dir), c.Name, c.Input, out, p.Width, p.Height, ffargs.Rate{Num: p.FPSNum, Den: p.FPSDen},
	)
	if err != nil {
		return fmt.Errorf("%w: the test encode was refused: %s", ErrEncoderSize, scrub(err.Error(), 200))
	}

	res, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: detectionTimeout})
	if err != nil {
		if ctx.Err() != nil {
			return err
		}

		return fmt.Errorf("%w: %s", ErrEncoderSize, firstFailureLine(res.Stderr, err))
	}

	pargv, err := ffargs.ProbeEncodedPackets(r.Policy(dir), out)
	if err != nil {
		return fmt.Errorf("%w: the check was refused", ErrEncoderSize)
	}

	b, pres, err := r.RunFFprobe(ctx, dir, pargv, detectionTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}

		return fmt.Errorf("%w: %s", ErrEncoderSize, firstFailureLine(pres.Stderr, err))
	}

	if got := strings.TrimSpace(string(b)); got != "h264,15" {
		return fmt.Errorf("%w: the test file has %s instead of 15 h264 packets", ErrEncoderSize, scrub(got, 40))
	}

	return nil
}

// EncoderForSize is the encoder an export at this size and rate uses (D-034, SDD-004 section 8): the one
// detection chose when its test encode at that size passes, else the next working candidate of the same
// order that passes it, down to the software encoder, so a GPU that cannot do 4K or 60 fps does not stop an
// export that libopenh264 can do. A forced encoder (VUKEM_VIDEO_ENCODER) is never replaced. When none
// passes, the error is the one of the chosen encoder and wraps ErrEncoderSize; a cancelled run or a failure
// that is not about the size ends the search at once.
func (r *Runner) EncoderForSize(ctx context.Context, det *EncoderDetection, p SizeProbe) (Candidate, error) {
	chosen, ok := det.ChosenCandidate()
	if !ok {
		return Candidate{}, errors.New("ffmpeg: no encoder was chosen")
	}

	err := r.TestEncoderAt(ctx, chosen, p)
	if err == nil {
		return chosen, nil
	}

	if det.Forced || ctx.Err() != nil || !errors.Is(err, ErrEncoderSize) {
		return chosen, err
	}

	for _, c := range det.Candidates {
		if c.Status != EncoderOK || c.Name == chosen.Name {
			continue
		}

		next := r.TestEncoderAt(ctx, c, p)
		if next == nil {
			return c, nil
		}

		if ctx.Err() != nil {
			return chosen, next
		}
	}

	return chosen, err
}
