package ingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// workFile is a file of the work directory that publish moves into artifacts/<hash>/.
type workFile struct {
	kind    string
	profile int
	variant string
	ext     string
	name    string // file name in the work directory
	meta    any
}

func (j *job) add(f workFile) { j.mu.Lock(); j.files = append(j.files, f); j.mu.Unlock() }

func bitexact() []ffargs.Opt { return ffargs.Bitexact() }

func cat(parts ...[]ffargs.Opt) []ffargs.Opt {
	var o []ffargs.Opt
	for _, p := range parts {
		o = append(o, p...)
	}
	return o
}

var colourTags = []ffargs.Opt{
	ffargs.Enum("-colorspace", "bt709"), ffargs.Enum("-color_primaries", "bt709"),
	ffargs.Enum("-color_trc", "bt709"), ffargs.Enum("-color_range", "tv"),
}

func linkOf(idx int) string { return "0:" + itoa(idx) }

// video is the path of a video asset: the video pass, the audio pass, the mux, the posters,
// the verification (SDD-011 7.1 steps 3 to 6).
func (j *job) video(ctx context.Context) error {
	sum := j.res.Summary
	plan := PlanVideo(sum, j.in.Caps)
	j.res.Plan = plan
	hasAudio := sum.Audio != nil
	deadline := ffmpeg.IngestDeadlineFor(time.Duration(sum.DurationMS)*time.Millisecond, sum.SizeBytes)

	var frames int
	if err := j.timed(StepVideo, func() error {
		n, err := j.videoPass(ctx, plan, deadline)
		frames = n
		return err
	}); err != nil {
		return err
	}
	j.res.Facts = Facts{
		FrameRate:  plan.FPS,
		FrameCount: frames,
		Width:      plan.W,
		Height:     plan.H,
		HasAudio:   hasAudio,
		HasAlpha:   plan.Alpha,
	}

	var samples int64
	if hasAudio {
		samples = (int64(frames)*SampleRate*plan.FPS.Den + plan.FPS.Num/2) / plan.FPS.Num
		if err := j.timed(StepAudio, func() error { return j.audioPass(ctx, samples, deadline) }); err != nil {
			return err
		}
		j.res.Facts.SampleCount = samples
	}
	if err := j.timed(StepMux, func() error { return j.mux(ctx, hasAudio, deadline) }); err != nil {
		return err
	}
	j.prog.set(StepMux, 1)
	return j.timed(StepVerify, func() error { return j.verifyVideo(ctx, plan, frames, samples) })
}

// videoPass is SDD-011 4.3.2: one decode, three outputs. It returns the frame count of video.mov.
func (j *job) videoPass(ctx context.Context, plan VideoPlan, deadline time.Duration) (int, error) {
	tiles, err := j.wd.Sub("thumbs")
	if err != nil {
		return 0, err
	}
	g := ffargs.Graph{
		{
			In:      []string{linkOf(plan.StreamIndex)},
			Filters: append(plan.NormalizedFilters(), ffargs.Split(2)),
			Out:     []string{"n", "p0"},
		},
		{In: []string{"p0"}, Filters: append(plan.ProxyFilters(), ffargs.Split(2)), Out: []string{"q", "r"}},
		{In: []string{"r"}, Filters: plan.ThumbFilters(), Out: []string{"s"}},
	}
	// One filter thread: swscale and zscale cut a frame in slices by thread count, and the
	// normalized bytes must not depend on the machine (NFR-010).
	cmd := ffargs.Cmd{
		Progress: true, LogErrorOnly: true, Threads: j.r.In.ThreadLimit(), FilterThreads: 1,
		Inputs: []ffargs.Input{videoInput(j.original, plan)}, Graph: g,
		Outputs: []ffargs.Output{
			{
				Format:  "mov",
				Path:    j.path("video.mov"),
				Maps:    []string{"[n]"},
				NoAudio: true,
				NoSubs:  true,
				NoData:  true,
				Opts: cat(
					normalizedCodecOpts(plan),
					colourTags,
					[]ffargs.Opt{ffargs.Enum("-fps_mode", "cfr"), ffargs.Int("-video_track_timescale", plan.Timescale)},
					bitexact(),
				),
			},
			proxyVideoOutput(j.path("proxy-video."+plan.ProxyExt()), plan),
			{
				Format:  "image2",
				Path:    filepath.Join(tiles, "t%04d."+plan.TileExt()),
				Maps:    []string{"[s]"},
				NoAudio: true,
				NoSubs:  true,
				NoData:  true,
				Opts: cat(
					tileCodecOpts(plan),
					[]ffargs.Opt{
						ffargs.Enum("-fps_mode", "cfr"),
						ffargs.Int("-frames:v", plan.Thumbs.Planned),
					},
					bitexact(),
					[]ffargs.Opt{ffargs.Int("-start_number", 0)},
				),
			},
		},
	}
	res, err := j.run(ctx, StepVideo, cmd, deadline, func(f float64) { j.prog.set(StepVideo, f) })
	if err != nil {
		return 0, err
	}
	j.decodeErrs += countDecodeErrors(res.Stderr)

	out, err := j.runProbe(ctx, pq(ffargs.ProbeEncodedPackets(j.pol, j.path("video.mov"))))
	if err != nil {
		return 0, err
	}
	frames := lastInt(string(out))
	// The coverage check runs before the audio pass, so a truncated file does not spend the rest of the pipeline.
	declared := float64(j.durationMS) / 1000
	produced := float64(frames) * float64(plan.FPS.Den) / float64(plan.FPS.Num)
	if produced < declared-max(0.5, 0.02*declared) {
		return 0, ingestErr(
			ReasonTruncated,
			"Only part of the declared media could be decoded; the rest is damaged or missing.",
			map[string]any{"declared_ms": j.durationMS, "decoded_ms": int64(produced * 1000)},
			nil,
		)
	}
	j.prog.set(StepVideo, 1)
	return frames, nil
}

// proxyVideoOutput is the video of the proxy: VP9 in MP4, or VP9 with an alpha plane in WebM when the
// source has alpha (SDD-011 4.3.5). The mov-only timescale is not an option of the WebM muxer.
func proxyVideoOutput(path string, plan VideoPlan) ffargs.Output {
	opts := []ffargs.Opt{
		ffargs.Enum("-c:v", "libvpx-vp9"), ffargs.Int("-crf", ProxyCRF), ffargs.Bitrate("-b:v", "0"),
		ffargs.Enum("-deadline", "realtime"), ffargs.Int("-cpu-used", 8), ffargs.Int("-row-mt", 1),
		ffargs.Int("-tile-columns", 1), ffargs.Int("-lag-in-frames", 0), ffargs.Int("-g", plan.GOP),
		ffargs.Int("-keyint_min", plan.GOP),
	}
	format, tail := "mp4", []ffargs.Opt{
		ffargs.Enum("-fps_mode", "cfr"),
		ffargs.Int("-video_track_timescale", plan.Timescale),
	}

	if plan.Alpha {
		// libvpx-vp9 only carries an alpha plane without the alternate reference frames.
		opts = append(opts, ffargs.Enum("-auto-alt-ref", "0"), ffargs.Enum("-pix_fmt", "yuva420p"))
		format, tail = "webm", []ffargs.Opt{ffargs.Enum("-fps_mode", "cfr")}
	} else {
		opts = append(opts, ffargs.Enum("-pix_fmt", "yuv420p"))
	}

	return ffargs.Output{
		Format: format, Path: path, Maps: []string{"[q]"}, NoAudio: true, NoSubs: true, NoData: true,
		Opts: cat(opts, colourTags, tail, bitexact()),
	}
}

// tileCodecOpts is the encoder of a thumbnail tile: JPEG, or PNG with alpha for a source that has it.
func tileCodecOpts(plan VideoPlan) []ffargs.Opt {
	if plan.Alpha {
		return []ffargs.Opt{ffargs.Enum("-c:v", "png"), ffargs.Enum("-pix_fmt", "rgba")}
	}
	return []ffargs.Opt{ffargs.Enum("-c:v", "mjpeg"), ffargs.Int("-q:v", 4)}
}

// videoInput is the source of the video pass, with the decoder the plan forces.
func videoInput(path string, plan VideoPlan) ffargs.Input {
	in := ffargs.FileInput(path)
	if plan.Decoder != "" {
		in = in.Decoder(plan.Decoder)
	}

	return in
}

// normalizedCodecOpts is the encoder of video.mov: mpeg4 in yuv420p, or ProRes 4444 in yuva444p10le
// when the source has an alpha channel (SDD-011 4.3.2 and 4.3.5).
func normalizedCodecOpts(plan VideoPlan) []ffargs.Opt {
	if plan.Alpha {
		return []ffargs.Opt{
			ffargs.Enum("-c:v", "prores_ks"),
			ffargs.Enum("-profile:v", "4444"),
			// One thread keeps the bytes the same on every machine (NFR-010).
			ffargs.Int("-threads", 1),
			ffargs.Enum("-pix_fmt", "yuva444p10le"),
		}
	}

	return []ffargs.Opt{
		ffargs.Enum("-c:v", "mpeg4"),
		ffargs.Int("-q:v", NormalizedQScale),
		ffargs.Int("-threads", NormalizedThreads),
		// The automatic DCT picks a SIMD version by CPU that is not bit-exact; int is.
		ffargs.Enum("-dct", "int"),
		ffargs.Int("-g", plan.GOP),
		ffargs.Int("-bf", 0),
		ffargs.Enum("-pix_fmt", "yuv420p"),
	}
}

func countDecodeErrors(stderr string) int {
	n := 0
	for l := range strings.SplitSeq(stderr, "\n") {
		if strings.Contains(l, "Decoding error") || strings.Contains(l, "error while decoding") ||
			strings.Contains(l, "Invalid data") {
			n++
		}
	}
	return n
}

// lastInt is the integer after the last comma of the last line of a csv answer.
func lastInt(out string) int {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if i := strings.LastIndexByte(last, ','); i >= 0 {
		last = last[i+1:]
	}
	return atoi(last)
}

// audioPass is SDD-011 4.3.3 for a video: the exact-length WAV, then the AAC proxy audio and the
// analyses in parallel.
func (j *job) audioPass(ctx context.Context, samples int64, deadline time.Duration) error {
	a := j.res.Summary.Audio
	chain := []ffargs.Filter{ffargs.AResample(SampleRate), ffargs.AFormatS16Stereo()}
	switch off := a.OffsetSamples; {
	case off > 0:
		chain = append(chain, ffargs.ADelay(off))
	case off < 0:
		chain = append(chain, ffargs.ATrimStart(-off), ffargs.ASetPTS())
	}
	chain = append(chain, ffargs.APad(int(samples)), ffargs.ATrimEnd(int(samples)))
	cmd := ffargs.Cmd{
		Progress: true, LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.original)},
		Outputs: []ffargs.Output{
			{
				Format:  "wav",
				Path:    j.path("audio.wav"),
				Maps:    []string{linkOf(a.StreamIndex)},
				NoVideo: true,
				NoSubs:  true,
				NoData:  true,
				AF: ffargs.Chain1(
					chain...),
				Opts: cat([]ffargs.Opt{ffargs.Enum("-c:a", "pcm_s16le"), ffargs.Enum("-rf64", "auto")}, bitexact()),
			},
		},
	}
	if _, err := j.run(ctx, StepAudio, cmd, deadline, func(f float64) { j.prog.set(StepAudio, 0.2*f) }); err != nil {
		return audioErr(err, a.Codec)
	}
	j.prog.set(StepAudio, 0.2)
	proxyAudio := "proxy-audio.mp4"
	if plan, ok := j.res.Plan.(VideoPlan); ok && plan.Alpha {
		proxyAudio = "proxy-audio.webm"
	}
	return j.audioDerived(ctx, "audio.wav", proxyAudio, samples, deadline)
}

// audioErr turns a decode failure of the audio into audio_decode_failed: the video decoded and the
// sound did not, and the file is never imported without its sound (SDD-011 5.3).
func audioErr(err error, codec string) error {
	var fe *ffmpeg.Error
	if errorsAs(err, &fe) && fe.Code == CodeIngestFailed && fe.Reason == ReasonDecodeFailed {
		return ingestErr(ReasonAudioDecodeFailed, "The video decoded, but its audio (codec \""+codec+"\") did not.",
			map[string]any{"codec": codec}, err)
	}
	return err
}

// audioDerived writes the AAC proxy of a WAV and runs the peaks, the silence and the loudness, in parallel.
func (j *job) audioDerived(ctx context.Context, wav, proxyName string, samples int64, deadline time.Duration) error {
	var wg sync.WaitGroup
	errs := make([]error, 5)
	var silence *Silence
	do := func(i int, f func() error) {
		wg.Go(func() { errs[i] = f() })
	}
	do(0, func() error {
		format, codec, tail := "mp4", "aac", []ffargs.Opt{ffargs.Enum("-movflags", "+faststart")}
		if strings.HasSuffix(proxyName, ".webm") {
			// The proxy of a source with alpha is WebM, which holds Opus and not AAC.
			format, codec, tail = "webm", "libopus", nil
		}
		cmd := ffargs.Cmd{
			LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.path(wav))},
			Outputs: []ffargs.Output{{
				Format: format, Path: j.path(proxyName), Maps: []string{"0:a:0"}, NoVideo: true,
				Opts: cat([]ffargs.Opt{
					ffargs.Enum("-c:a", codec),
					ffargs.Bitrate("-b:a", "128k"),
					ffargs.Int("-ar", SampleRate),
					ffargs.Int("-ac", 2),
				}, tail, bitexact()),
			}},
		}
		_, err := j.run(ctx, StepAudio, cmd, deadline, nil)
		return err
	})
	do(1, func() error {
		pk, err := PeaksOfFile(j.path(wav))
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The audio peaks could not be computed.",
				map[string]any{"step": StepAudio},
				err,
			)
		}
		j.peaks = pk
		return os.WriteFile(j.path("peaks.bin"), pk.Marshal(), 0o600)
	})
	do(2, func() error {
		var g ffargs.Graph
		g = append(
			g,
			ffargs.Chain{
				In:      []string{"0:a"},
				Filters: []ffargs.Filter{ffargs.ASplit(4)},
				Out:     []string{"a", "b", "c", "d"},
			},
		)
		var outs []ffargs.Output
		for i, l := range []string{"a", "b", "c", "d"} {
			o := string(rune('w' + i))
			g = append(
				g,
				ffargs.Chain{
					In:      []string{l},
					Filters: []ffargs.Filter{ffargs.SilenceDetect(SilenceThresholdsDB[i], SilenceMinMS)},
					Out:     []string{o},
				},
			)
			outs = append(outs, ffargs.Output{Format: "null", Maps: []string{"[" + o + "]"}})
		}
		res, err := j.run(
			ctx,
			StepAudio,
			ffargs.Cmd{Inputs: []ffargs.Input{ffargs.FileInput(j.path(wav))}, Graph: g, Outputs: outs},
			deadline,
			nil,
		)
		if err != nil {
			return err
		}
		s, err := ParseSilence(res.Stderr, samples)
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The silence analysis could not be read.",
				map[string]any{"step": StepAudio},
				err,
			)
		}
		silence = s

		return j.writeJSON("silence.json", s)
	})
	do(3, func() error {
		cmd := ffargs.Cmd{
			Inputs:  []ffargs.Input{ffargs.FileInput(j.path(wav))},
			Outputs: []ffargs.Output{{Format: "null", Maps: []string{"0:a:0"}, AF: ffargs.Chain1(ffargs.Ebur128())}},
		}
		res, err := j.run(ctx, StepAudio, cmd, deadline, nil)
		if err != nil {
			return err
		}
		l, err := ParseLoudness(res.Stderr, samples)
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The loudness analysis could not be read.",
				map[string]any{"step": StepAudio},
				err,
			)
		}
		return j.writeJSON("loudness.json", l)
	})
	do(4, func() error {
		if !j.in.SoundMaps {
			return nil
		}

		b, err := BeatsOfFile(ctx, j.path(wav))
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The beat grid could not be computed.",
				map[string]any{"step": StepAudio},
				err,
			)
		}

		return j.writeJSON("beats.json", b)
	})
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	if j.in.SoundMaps {
		if err := j.writeSpeech(silence); err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"The speech map could not be derived.",
				map[string]any{"step": StepAudio},
				err,
			)
		}
	}
	j.prog.set(StepAudio, 1)
	j.add(workFile{kind: "peaks", profile: ProfilePeaks, ext: "bin", name: "peaks.bin"})
	j.add(workFile{kind: "silence", profile: ProfileSilence, ext: "json", name: "silence.json"})
	j.add(workFile{kind: "loudness", profile: ProfileLoudness, ext: "json", name: "loudness.json"})
	if j.in.SoundMaps {
		j.add(workFile{kind: "speech", profile: ProfileSpeech, ext: "json", name: "speech.json"})
		j.add(workFile{kind: "beats", profile: ProfileBeats, ext: "json", name: "beats.json"})
	}
	return nil
}

// mux is SDD-011 4.3.4 and 4.4: copy only, with each stream keeping its timestamps; then the
// posters and the thumbnail sheet.
func (j *job) mux(ctx context.Context, hasAudio bool, deadline time.Duration) error {
	if hasAudio {
		cmd := ffargs.Cmd{
			LogErrorOnly: true,
			Inputs:       []ffargs.Input{ffargs.FileInput(j.path("video.mov")), ffargs.FileInput(j.path("audio.wav"))},
			Outputs: []ffargs.Output{{
				Format: "mov", Path: j.path("normalized.mov"), Maps: []string{"0:v:0", "1:a:0"},
				Opts: cat([]ffargs.Opt{ffargs.Enum("-c:v", "copy"), ffargs.Enum("-c:a", "copy")}, bitexact()),
			}},
		}
		if _, err := j.run(ctx, StepMux, cmd, deadline, nil); err != nil {
			return err
		}
	} else if err := os.Rename(j.path("video.mov"), j.path("normalized.mov")); err != nil {
		return err
	}
	plan, _ := j.res.Plan.(VideoPlan)
	ext := plan.ProxyExt()
	proxyName := "proxy." + ext
	ins := []ffargs.Input{ffargs.FileInput(j.path("proxy-video." + ext))}
	maps := []string{"0:v:0"}
	opts := []ffargs.Opt{ffargs.Enum("-c:v", "copy")}
	if hasAudio {
		ins = append(ins, ffargs.FileInput(j.path("proxy-audio."+ext)))
		maps = append(maps, "1:a:0")
		opts = append(opts, ffargs.Enum("-c:a", "copy"))
	}
	// Only the MP4 proxy moves its index to the front and has a chunk index; the WebM proxy of a
	// source with alpha is played by a <video> element (SDD-011 4.4).
	format, tail := "mp4", []ffargs.Opt{ffargs.Enum("-movflags", "+faststart")}
	if plan.Alpha {
		// -map_metadata -1 (FF-08) drops the alpha_mode tag that the WebM muxer turns into the AlphaMode
		// element of the track, without which a browser ignores the alpha plane; it is written back.
		format, tail = "webm", []ffargs.Opt{ffargs.Enum("-metadata:s:v:0", "alpha_mode=1")}
	}
	pcmd := ffargs.Cmd{
		LogErrorOnly: true,
		Inputs:       ins,
		Outputs: []ffargs.Output{
			{Format: format, Path: j.path(proxyName), Maps: maps, Opts: cat(opts, tail, bitexact())},
		},
	}
	if _, err := j.run(ctx, StepMux, pcmd, deadline, nil); err != nil {
		return err
	}
	if !plan.Alpha {
		if err := j.writeChunks(); err != nil {
			return err
		}
	}
	j.add(workFile{kind: "normalized", profile: ProfileNormalized, ext: "mov", name: "normalized.mov"})
	j.add(workFile{kind: "proxy", profile: ProfileProxy, ext: ext, name: proxyName})
	return j.posters(ctx, ffmpegSeek(j.durationMS), true, proxyName, plan.Alpha, deadline)
}

// ffmpegSeek is min(1 s, duration / 2) in milliseconds.
func ffmpegSeek(durationMS int64) int64 { return min(1000, durationMS/2) }

// posters writes the three posters from a stored proxy and builds the thumbnail sheet of a video.
// With alpha the posters and the sheet are PNG and keep it; the proxy is then read by libvpx-vp9,
// which carries the alpha plane of VP9 that the native decoder drops (SDD-011 4.3.5).
func (j *job) posters(
	ctx context.Context,
	seekMS int64,
	sheet bool,
	proxyName string,
	alpha bool,
	deadline time.Duration,
) error {
	in := ffargs.FileInput(j.path(proxyName))
	if alpha {
		in = in.Decoder("libvpx-vp9")
	}
	if seekMS > 0 {
		in = in.At(seekMS)
	}
	g := ffargs.Graph{{In: []string{"0:v:0"}, Filters: []ffargs.Filter{ffargs.Split(3)}, Out: []string{"a", "b", "c"}}}
	var outs []ffargs.Output
	for i, w := range PosterWidths {
		l, o := string(rune('a'+i)), string(rune('x'+i))
		g = append(
			g,
			ffargs.Chain{
				In:      []string{l},
				Filters: []ffargs.Filter{ffargs.ScaleMinWidth(w), ffargs.Format(posterPixFmt(alpha))},
				Out:     []string{o},
			},
		)
		ext, codec := "jpg", []ffargs.Opt{ffargs.Enum("-c:v", "mjpeg"), ffargs.Int("-q:v", 4)}
		if alpha {
			ext, codec = "png", []ffargs.Opt{ffargs.Enum("-c:v", "png")}
		}
		name := "w" + itoa(w) + "." + ext
		outs = append(outs, ffargs.Output{
			Format: "image2",
			Path:   j.path(name),
			Maps:   []string{"[" + o + "]"},
			Opts: cat(
				[]ffargs.Opt{ffargs.Int("-frames:v", 1)},
				codec,
				bitexact(),
				[]ffargs.Opt{ffargs.Int("-update", 1)},
			),
		})
		j.add(workFile{kind: "thumbnails", profile: ProfileThumbnails, variant: "w" + itoa(w), ext: ext, name: name})
	}
	if _, err := j.run(
		ctx,
		StepMux,
		ffargs.Cmd{LogErrorOnly: true, Inputs: []ffargs.Input{in}, Graph: g, Outputs: outs},
		deadline,
		nil,
	); err != nil {
		return err
	}
	if !sheet {
		return nil
	}
	return j.thumbSheet()
}

func posterPixFmt(alpha bool) string {
	if alpha {
		return "rgba"
	}
	return "yuvj420p"
}

// thumbSheet assembles the sheet from the tile files of the video pass.
func (j *job) thumbSheet() error {
	plan, ok := j.res.Plan.(VideoPlan)
	if !ok {
		return errors.New("ingest: the plan is not a video plan")
	}
	matches, err := filepath.Glob(filepath.Join(j.wd.Dir(), "thumbs", "t*."+plan.TileExt()))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	data, layout, err := BuildSheet(matches, plan.Thumbs)
	if err != nil {
		return ingestErr(
			ReasonVerificationFailed,
			"The thumbnails could not be assembled.",
			map[string]any{"checks": []map[string]any{checkFail("thumbnails", "decodable tiles", err.Error())}},
			err,
		)
	}
	sheet := "thumbnails." + plan.TileExt()
	if err := os.WriteFile(j.path(sheet), data, 0o600); err != nil {
		return err
	}
	j.layout = layout
	j.add(workFile{kind: "thumbnails", profile: ProfileThumbnails, ext: plan.TileExt(), name: sheet, meta: layout})
	return nil
}
