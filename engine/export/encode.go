package export

import (
	"context"
	"fmt"
	"image"
	"os"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// progressEvery is the longest interval between two progress reports (at most 4 per second).
const progressEvery = 250 * time.Millisecond

// refPixels is the frame the bitrates of SDD-004 section 8 are given for: 1080 by 1920.
const refPixels = 1080 * 1920

// bitrateK is the video bit rate in kbit/s of the hardware, openh264 and Media Foundation encoders: the
// base rate for 1080x1920 at up to 30 fps (high above it) scaled by the pixel count, 2 Mbit/s at least.
func bitrateK(baseM, highM, w, h int, fps timeline.Rational) int {
	base := baseM
	if fps.Num > 30*fps.Den {
		base = highM
	}
	return max(2000, base*1000*w*h/refPixels)
}

// encoderOpts are the options of SDD-004 section 8 for the chosen encoder.
func encoderOpts(name string, w, h int, fps timeline.Rational) []ffargs.Opt {
	opts := []ffargs.Opt{ffargs.Enum("-c:v", name)}
	kbps := func(base, high int) ffargs.Opt {
		return ffargs.Bitrate("-b:v", fmt.Sprintf("%dk", bitrateK(base, high, w, h, fps)))
	}
	bframes := true
	switch name {
	case "libx264":
		opts = append(opts, ffargs.Int("-crf", 18), ffargs.Enum("-preset", "medium"), ffargs.Enum("-profile:v", "high"))
	case "h264_mf":
		opts = append(opts, kbps(12, 16))
	case "libopenh264":
		opts = append(opts, kbps(8, 12))
		bframes = false
	case "h264_vaapi":
		opts = append(opts, kbps(8, 12))
	default:
		opts = append(opts, kbps(8, 12), ffargs.Enum("-profile:v", "high"))
	}
	opts = append(opts, ffargs.Int("-g", max(1, (fps.Num+2*fps.Den-1)/(2*fps.Den))))
	if bframes {
		opts = append(opts, ffargs.Int("-bf", 2))
	}
	return opts
}

// encodeArgv is the encoder command of SDD-004 section 8 step 4.
func (s *state) encodeArgv(cand ffmpeg.Candidate, mix, partial string) ([]string, error) {
	st := s.job.Doc.Settings
	opts := encoderOpts(cand.Name, st.Width, st.Height, st.FPS)
	opts = append(opts,
		ffargs.Enum("-colorspace", "bt709"), ffargs.Enum("-color_primaries", "bt709"),
		ffargs.Enum("-color_trc", "bt709"), ffargs.Enum("-color_range", "tv"),
		ffargs.Enum("-c:a", "aac"), ffargs.Bitrate("-b:a", "192k"),
		ffargs.Int("-ar", 48000), ffargs.Int("-ac", 2),
		ffargs.Enum("-movflags", "+faststart"),
	)
	opts = append(opts, ffargs.Bitexact()...)

	r := s.job.Runner
	return ffargs.Cmd{
		Progress:      true,
		Threads:       r.In.ThreadLimit(),
		FilterThreads: r.In.ThreadLimit(),
		VaapiDevice:   cand.Input.VaapiDevice,
		Inputs: []ffargs.Input{
			ffargs.PipeRawVideo("rgba", st.Width, st.Height, ffargs.Rate{Num: st.FPS.Num, Den: st.FPS.Den}),
			ffargs.FileInput(mix),
		},
		Outputs: []ffargs.Output{{
			Format: "mp4", Path: partial, Maps: []string{"0:v:0", "1:a:0"}, Shortest: true,
			VF:   ffargs.Chain1(ffargs.ScaleBT709Out(), ffargs.Filter(cand.Input.Filter)),
			Opts: opts,
		}},
	}.Argv(r.Policy(s.dir))
}

type encoderResult struct {
	res ffmpeg.Result
	err error
}

// encode composes every frame and writes it to the standard input of the encoder, then waits for
// the encoder to finish (SDD-004 section 8 steps 4 to 6). Composing the next frame overlaps the
// encoder reading the last.
func (s *state) encode(
	ctx context.Context,
	cand ffmpeg.Candidate,
	mix, partial string,
	rd *renderer,
) error {
	st := s.job.Doc.Settings
	argv, err := s.encodeArgv(cand, mix, partial)
	if err != nil {
		return exportErr(ReasonEncoder, "The encoder command was refused.", err, "")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return exportErr(ReasonEncoder, "A pipe for the encoder could not be made.", err, "")
	}

	ectx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan encoderResult, 1)
	go func() {
		res, err := s.job.Runner.RunFFmpeg(ectx, ffmpeg.Request{
			Dir: s.dir, Argv: argv, Deadline: s.deadline, DeadlineCeiling: ffmpeg.ExportDeadlineCeiling,
			Stdin:      pr,
			OutputPath: partial, MaxOutputBytes: s.job.MaxBytes,
		})
		_ = pr.Close()
		done <- encoderResult{res, err}
	}()

	const buffers = 2
	free, full := make(chan *image.RGBA, buffers), make(chan *image.RGBA, buffers)
	for range buffers {
		free <- image.NewRGBA(image.Rect(0, 0, st.Width, st.Height))
	}
	writeErr := make(chan error, 1)
	broken := make(chan struct{}) // closed once, when the first write to the encoder fails
	go func() {
		var werr error
		for b := range full {
			if werr == nil {
				if _, werr = pw.Write(b.Pix); werr != nil {
					close(broken)
				}
			}
			free <- b
		}
		writeErr <- werr
	}()

	// finish ends the writer, tells the encoder the input is over and waits for it.
	finish := func(ok bool) (encoderResult, error) {
		if !ok {
			cancel() // a stuck encoder would never read what the writer is blocked on
		}
		close(full)
		werr := <-writeErr
		_ = pw.Close()
		r := <-done
		if r.err != nil {
			werr = nil // the encoder's own failure says more than the broken pipe it caused
		}
		return r, werr
	}

	fail := func(cause error) error {
		r, werr := finish(false)
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if cause != nil {
			return cause
		}
		return encoderOutcome(r, werr)
	}

	last := time.Now()
	for t := range s.frames {
		var buf *image.RGBA
		select {
		case buf = <-free:
		case <-broken:
			return fail(nil)
		case <-ctx.Done():
			return fail(nil)
		}

		if err := rd.render(buf, t); err != nil {
			free <- buf
			return fail(wrapRun(err, ReasonRender, "A frame of the project could not be composed.", ""))
		}
		full <- buf

		if s.job.OnProgress != nil && (time.Since(last) >= progressEvery || t == s.frames-1) {
			last = time.Now()
			s.job.OnProgress(Progress{Frame: t + 1, Total: s.frames})
		}
	}

	r, werr := finish(true)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return encoderOutcome(r, werr)
}

// encoderOutcome is the error of an encoder that ended: its own failure first, then the broken
// pipe, nil when both are clean.
func encoderOutcome(r encoderResult, werr error) error {
	switch {
	case r.err != nil:
		return wrapRun(r.err, ReasonEncoder, "The video encoder failed.", r.res.Stderr)
	case werr != nil:
		return exportErr(ReasonEncoder, "The frames could not be handed to the encoder.", werr, r.res.Stderr)
	}

	return nil
}
