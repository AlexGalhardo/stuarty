package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

func itoa(n int) string { return strconv.Itoa(n) }

func errorsAs(err error, target **ffmpeg.Error) bool { return errors.As(err, target) }

func (j *job) writeJSON(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(j.path(name), b, 0o600)
}

// audioOnly is SDD-011 4.8: one pass writes normalized.wav, no padding, no alignment; the proxy
// is AAC from the WAV.
func (j *job) audioOnly(ctx context.Context) error {
	sum := j.res.Summary
	j.res.Plan = struct {
		StreamIndex int `json:"stream_index"`
	}{sum.Audio.StreamIndex}
	deadline := ffmpeg.IngestDeadlineFor(time.Duration(sum.DurationMS)*time.Millisecond, sum.SizeBytes)
	err := j.timed(StepAudio, func() error {
		cmd := ffargs.Cmd{
			Progress: true, LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.original)},
			Outputs: []ffargs.Output{
				{
					Format:  "wav",
					Path:    j.path("normalized.wav"),
					Maps:    []string{linkOf(sum.Audio.StreamIndex)},
					NoVideo: true,
					NoSubs:  true,
					NoData:  true,
					AF:      ffargs.Chain1(ffargs.AResample(SampleRate), ffargs.AFormatS16Stereo()),
					Opts: cat(
						[]ffargs.Opt{ffargs.Enum("-c:a", "pcm_s16le"), ffargs.Enum("-rf64", "auto")},
						bitexact(),
					),
				},
			},
		}
		if _, err := j.run(
			ctx,
			StepAudio,
			cmd,
			deadline,
			func(f float64) { j.prog.set(StepAudio, 0.2*f) },
		); err != nil {
			return audioErr(err, sum.Audio.Codec)
		}
		j.prog.set(StepAudio, 0.2)
		samples, err := wavSamples(j.path("normalized.wav"))
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The normalized audio could not be read back.",
				map[string]any{"step": StepAudio},
				err,
			)
		}
		j.res.Facts = Facts{SampleCount: samples, HasAudio: true}
		if samples == 0 {
			return unsupportedErr("empty_audio", "The audio has no samples.", nil)
		}
		return j.audioDerived(ctx, "normalized.wav", "proxy.mp4", samples, deadline)
	})
	if err != nil {
		return err
	}
	j.add(workFile{kind: "normalized", profile: ProfileNormalized, ext: "wav", name: "normalized.wav"})
	j.add(workFile{kind: "proxy", profile: ProfileProxy, ext: "mp4", name: "proxy.mp4"})
	return j.timed(StepVerify, func() error { return j.verifyAudioOnly(ctx) })
}

func wavSamples(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	w, err := ReadWAV(f)
	if err != nil {
		return 0, err
	}
	return w.DataSize / int64(w.Channels*w.Bits/8), nil
}

// image is SDD-011 4.7: normalized.png and the proxy from one process, then the posters.
func (j *job) image(ctx context.Context) error {
	sum := j.res.Summary
	plan := PlanImage(sum)
	j.res.Plan = plan
	deadline := ffmpeg.IngestDeadline(0)

	pix, proxyExt, proxyFilter, proxyOpts := "rgb24", "jpg", "yuvj420p", []ffargs.Opt{
		ffargs.Enum("-c:v", "mjpeg"),
		ffargs.Int("-q:v", 3),
	}
	if plan.Alpha {
		pix, proxyExt, proxyFilter, proxyOpts = "rgba", "png", "rgba", []ffargs.Opt{ffargs.Enum("-c:v", "png")}
	}
	g := ffargs.Graph{
		{In: []string{linkOf(plan.StreamIndex)}, Filters: []ffargs.Filter{ffargs.Split(2)}, Out: []string{"a", "b"}},
		{
			In: []string{"a"},
			Filters: []ffargs.Filter{
				ffargs.Scale(plan.W, plan.H, "bicubic+accurate_rnd+full_chroma_int"),
				ffargs.Format(pix),
			},
			Out: []string{"n"},
		},
		{
			In:      []string{"b"},
			Filters: []ffargs.Filter{ffargs.Scale(plan.PW, plan.PH, "bicubic"), ffargs.Format(proxyFilter)},
			Out:     []string{"p"},
		},
	}
	proxyName := "proxy." + proxyExt
	cmd := ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.original)}, Graph: g,
		Outputs: []ffargs.Output{
			{
				Format: "image2",
				Path:   j.path("normalized.png"),
				Maps:   []string{"[n]"},
				Opts: cat(
					[]ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-c:v", "png"), ffargs.Int("-update", 1)},
					bitexact(),
				),
			},
			{
				Format: "image2",
				Path:   j.path(proxyName),
				Maps:   []string{"[p]"},
				Opts: cat(
					[]ffargs.Opt{ffargs.Int("-frames:v", 1)},
					proxyOpts,
					bitexact(),
					[]ffargs.Opt{ffargs.Int("-update", 1)},
				),
			},
		},
	}
	if err := j.timed(StepVideo, func() error {
		_, err := j.run(ctx, StepVideo, cmd, deadline, nil)
		return err
	}); err != nil {
		return err
	}
	j.res.Facts = Facts{Width: plan.W, Height: plan.H, HasAlpha: plan.Alpha}
	if delays := sum.FrameDelays; len(delays) > 1 {
		if err := j.timed(StepVideo, func() error { return j.animated(ctx, plan) }); err != nil {
			return err
		}
		j.res.Facts.FrameCount, j.res.Facts.FrameDelays = len(delays), delays
	}
	j.prog.set(StepVideo, 1)
	j.markAlphaOpaque(j.path("normalized.png"), nil)
	j.add(workFile{kind: "normalized", profile: ProfileNormalized, ext: "png", name: "normalized.png"})
	j.add(workFile{kind: "proxy", profile: ProfileProxy, ext: proxyExt, name: proxyName})
	if err := j.timed(
		StepMux,
		func() error { return j.posters(ctx, 0, false, proxyName, false, deadline) },
	); err != nil {
		return err
	}
	return j.timed(StepVerify, func() error { return j.verifyImage(plan, proxyName) })
}
