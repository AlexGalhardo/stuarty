package ffmpeg

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// TestGoldenCommandsRunOnThePinnedBuild builds the commands of SDD-011 section 4 and
// SDD-004 section 6 with the typed builders and runs each of them through the runner on the
// pinned LGPL build. It proves that the checker's grammar is not only closed but also
// complete: every argument list it lets through is one FFmpeg accepts, with the
// -format_whitelist, -sn -dn and file: forms of FF-02 and FF-03.
func TestGoldenCommandsRunOnThePinnedBuild(t *testing.T) {
	r, dir := pinnedRunner(t)
	in := r.In
	ctx := context.Background()

	src := putStore(t, r, gen(t, in, dir, "src.mp4",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=30:d=2", "-f", "lavfi", "-i", "sine=f=440:d=2",
		"-c:v", "mpeg4", "-c:a", "aac", "-f", "mp4"))
	img := putStore(
		t,
		r,
		gen(
			t,
			in,
			dir,
			"src.png",
			"-f",
			"lavfi",
			"-i",
			"testsrc2=s=200x100:r=1:d=1",
			"-frames:v",
			"1",
			"-c:v",
			"png",
			"-f",
			"image2",
		),
	)
	pol := r.Policy(dir)
	bx := ffargs.Bitexact()
	path := func(n string) string {
		p, err := dir.Path(n)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	exists := func(names ...string) {
		t.Helper()
		for _, n := range names {
			st, err := os.Stat(filepath.Join(dir.Dir(), filepath.FromSlash(n)))
			if err != nil || st.Size() == 0 {
				t.Errorf("%s was not written: %v", n, err)
			}
		}
	}
	runCmd := func(name string, c ffargs.Cmd, _ bool) {
		t.Helper()
		argv, err := c.Argv(pol)
		if err != nil {
			t.Fatalf("%s: builder: %v", name, err)
		}
		req := Request{Dir: dir, Argv: argv, Deadline: 2 * time.Minute}
		var last Progress
		if c.Progress {
			req.OnProgress = func(p Progress) { last = p }
		}
		res, err := r.RunFFmpeg(ctx, req)
		if err != nil {
			t.Fatalf("%s: %v\n%s\n%s", name, err, strings.Join(argv, " "), res.Stderr)
		}
		if c.Progress && !last.End {
			t.Errorf("%s: no progress=end block seen (%+v)", name, last)
		}
	}
	threads := in.ThreadLimit()

	// 4.3.2: one decode, three outputs.
	tiles, err := dir.Sub("thumbs")
	if err != nil {
		t.Fatal(err)
	}
	g := ffargs.Graph{
		{
			In: []string{"0:0"},
			Filters: []ffargs.Filter{
				ffargs.SetPTS(),
				ffargs.FPS(ffargs.Rate{Num: 30, Den: 1}),
				ffargs.Format("yuv420p"),
				ffargs.Split(2),
			},
			Out: []string{"n", "p0"},
		},
		{
			In:      []string{"p0"},
			Filters: []ffargs.Filter{ffargs.Scale(160, 120, "bicubic"), ffargs.Format("yuv420p"), ffargs.Split(2)},
			Out:     []string{"q", "r"},
		},
		{
			In: []string{"r"},
			Filters: []ffargs.Filter{
				ffargs.FPS(ffargs.Rate{Num: 1, Den: 1}),
				ffargs.Scale(72, 54, "bicubic"),
				ffargs.Format("yuvj420p"),
			},
			Out: []string{"s"},
		},
	}
	colour := []ffargs.Opt{
		ffargs.Enum("-colorspace", "bt709"),
		ffargs.Enum("-color_primaries", "bt709"),
		ffargs.Enum("-color_trc", "bt709"),
		ffargs.Enum("-color_range", "tv"),
	}
	opts := func(parts ...[]ffargs.Opt) []ffargs.Opt {
		var o []ffargs.Opt
		for _, p := range parts {
			o = append(o, p...)
		}
		return o
	}
	runCmd("video pass", ffargs.Cmd{
		Progress: true, LogErrorOnly: true, FilterThreads: threads, Threads: threads,
		Inputs: []ffargs.Input{ffargs.FileInput(src)}, Graph: g,
		Outputs: []ffargs.Output{
			{
				Format:  "mov",
				Path:    path("video.mov"),
				Maps:    []string{"[n]"},
				NoAudio: true,
				NoSubs:  true,
				NoData:  true,
				Opts: opts(
					[]ffargs.Opt{
						ffargs.Enum("-c:v", "mpeg4"),
						ffargs.Int("-q:v", 2),
						ffargs.Int("-g", 15),
						ffargs.Int("-bf", 0),
						ffargs.Enum("-pix_fmt", "yuv420p"),
					},
					colour,
					[]ffargs.Opt{ffargs.Enum("-fps_mode", "cfr"), ffargs.Int("-video_track_timescale", 30000)},
					bx,
				),
			},
			{
				Format:  "mp4",
				Path:    path("proxy-video.mp4"),
				Maps:    []string{"[q]"},
				NoAudio: true,
				NoSubs:  true,
				NoData:  true,
				Opts: opts([]ffargs.Opt{
					ffargs.Enum(
						"-c:v",
						"libvpx-vp9",
					),
					ffargs.Int("-crf", 33),
					ffargs.Bitrate("-b:v", "0"),
					ffargs.Enum("-deadline", "realtime"),
					ffargs.Int("-cpu-used", 8),
					ffargs.Int(
						"-row-mt",
						1,
					),
					ffargs.Int("-tile-columns", 1),
					ffargs.Int("-lag-in-frames", 0),
					ffargs.Int("-g", 15),
					ffargs.Int("-keyint_min", 15),
					ffargs.Enum("-pix_fmt", "yuv420p"),
				}, colour,
					[]ffargs.Opt{ffargs.Enum("-fps_mode", "cfr"), ffargs.Int("-video_track_timescale", 30000)}, bx),
			},
			{
				Format:  "image2",
				Path:    filepath.Join(tiles, "t%04d.jpg"),
				Maps:    []string{"[s]"},
				NoAudio: true,
				NoSubs:  true,
				NoData:  true,
				Opts: opts(
					[]ffargs.Opt{ffargs.Enum("-c:v", "mjpeg"), ffargs.Int("-q:v", 4), ffargs.Enum("-fps_mode", "cfr")},
					bx,
					[]ffargs.Opt{ffargs.Int("-start_number", 0)},
				),
			},
		},
	}, true)
	exists("video.mov", "proxy-video.mp4", "thumbs/t0000.jpg", "thumbs/t0001.jpg")

	// 4.3.3: the audio pass with a head offset, then the mux and the proxy mux.
	runCmd("audio pass", ffargs.Cmd{
		Progress: true, LogErrorOnly: true,
		Inputs: []ffargs.Input{ffargs.FileInput(src)},
		Outputs: []ffargs.Output{
			{
				Format:  "wav",
				Path:    path("audio.wav"),
				Maps:    []string{"0:1"},
				NoVideo: true,
				NoSubs:  true,
				NoData:  true,
				AF: ffargs.Chain1(
					ffargs.AResample(48000),
					ffargs.AFormatS16Stereo(),
					ffargs.ADelay(14400),
					ffargs.APad(96000),
					ffargs.ATrimEnd(96000),
				),
				Opts: opts([]ffargs.Opt{ffargs.Enum("-c:a", "pcm_s16le"), ffargs.Enum("-rf64", "auto")}, bx),
			},
		},
	}, true)
	exists("audio.wav")
	runCmd("audio pass, head trimmed", ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(src)},
		Outputs: []ffargs.Output{
			{
				Format:  "wav",
				Path:    path("audio2.wav"),
				Maps:    []string{"0:1"},
				NoVideo: true,
				AF: ffargs.Chain1(
					ffargs.AResample(48000),
					ffargs.AFormatS16Stereo(),
					ffargs.ATrimStart(4800),
					ffargs.ASetPTS(),
					ffargs.APad(96000),
					ffargs.ATrimEnd(96000),
				),
				Opts: []ffargs.Opt{ffargs.Enum("-c:a", "pcm_s16le")},
			},
		},
	}, false)
	runCmd("mux", ffargs.Cmd{
		LogErrorOnly: true,
		Inputs:       []ffargs.Input{ffargs.FileInput(path("video.mov")), ffargs.FileInput(path("audio.wav"))},
		Outputs: []ffargs.Output{{
			Format: "mov", Path: path("normalized.mov"), Maps: []string{"0:v:0", "1:a:0"},
			Opts: opts([]ffargs.Opt{ffargs.Enum("-c:v", "copy"), ffargs.Enum("-c:a", "copy")}, bx),
		}},
	}, false)
	runCmd("proxy audio", ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(path("audio.wav"))},
		Outputs: []ffargs.Output{
			{
				Format:  "mp4",
				Path:    path("proxy-audio.mp4"),
				Maps:    []string{"0:a:0"},
				NoVideo: true,
				Opts: opts(
					[]ffargs.Opt{
						ffargs.Enum("-c:a", "aac"),
						ffargs.Bitrate("-b:a", "128k"),
						ffargs.Int("-ar", 48000),
						ffargs.Int("-ac", 2),
					},
					bx,
				),
			},
		},
	}, false)
	runCmd("proxy mux", ffargs.Cmd{
		LogErrorOnly: true,
		Inputs: []ffargs.Input{
			ffargs.FileInput(path("proxy-video.mp4")),
			ffargs.FileInput(path("proxy-audio.mp4")),
		},
		Outputs: []ffargs.Output{
			{
				Format: "mp4",
				Path:   path("proxy.mp4"),
				Maps:   []string{"0:v:0", "1:a:0"},
				Opts: opts(
					[]ffargs.Opt{
						ffargs.Enum("-c:v", "copy"),
						ffargs.Enum("-c:a", "copy"),
						ffargs.Enum("-movflags", "+faststart"),
					},
					bx,
				),
			},
		},
	}, false)
	exists("normalized.mov", "proxy-audio.mp4", "proxy.mp4")

	// 4.5: posters from the proxy, with an accurate seek before the input.
	pg := ffargs.Graph{
		{In: []string{"0:v:0"}, Filters: []ffargs.Filter{ffargs.Split(2)}, Out: []string{"a", "b"}},
		{
			In:      []string{"a"},
			Filters: []ffargs.Filter{ffargs.ScaleMinWidth(160), ffargs.Format("yuvj420p")},
			Out:     []string{"x"},
		},
		{
			In:      []string{"b"},
			Filters: []ffargs.Filter{ffargs.ScaleMinWidth(640), ffargs.Format("yuvj420p")},
			Out:     []string{"y"},
		},
	}
	poster := func(l, n string) ffargs.Output {
		return ffargs.Output{
			Format: "image2",
			Path:   path(n),
			Maps:   []string{"[" + l + "]"},
			Opts: opts(
				[]ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-c:v", "mjpeg"), ffargs.Int("-q:v", 4)},
				bx,
				[]ffargs.Opt{ffargs.Int("-update", 1)},
			),
		}
	}
	runCmd("posters", ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(path("proxy.mp4")).At(1000)}, Graph: pg,
		Outputs: []ffargs.Output{poster("x", "w160.jpg"), poster("y", "w640.jpg")},
	}, false)
	exists("w160.jpg", "w640.jpg")

	// 4.7 and 4.8: an image, and an audio-only file.
	runCmd("image", ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(img)},
		Outputs: []ffargs.Output{
			{
				Format: "image2",
				Path:   path("normalized.png"),
				Maps:   []string{"0:0"},
				VF: ffargs.Chain1(
					ffargs.Scale(200, 100, "bicubic+accurate_rnd+full_chroma_int"),
					ffargs.Format("rgb24"),
				),
				Opts: opts(
					[]ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-c:v", "png"), ffargs.Int("-update", 1)},
					bx,
				),
			},
		},
	}, false)
	runCmd("audio only", ffargs.Cmd{
		Progress: true, LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(src)},
		Outputs: []ffargs.Output{
			{
				Format:  "wav",
				Path:    path("normalized.wav"),
				Maps:    []string{"0:1"},
				NoVideo: true,
				NoSubs:  true,
				NoData:  true,
				AF: ffargs.Chain1(
					ffargs.AResample(48000),
					ffargs.AFormatS16Stereo(),
				),
				Opts: opts([]ffargs.Opt{ffargs.Enum("-c:a", "pcm_s16le"), ffargs.Enum("-rf64", "auto")}, bx),
			},
		},
	}, true)
	exists("normalized.png", "normalized.wav")

	// 9.1 and 9.2: analysis runs that write nothing.
	var sg ffargs.Graph
	sg = append(
		sg,
		ffargs.Chain{In: []string{"0:a"}, Filters: []ffargs.Filter{ffargs.ASplit(2)}, Out: []string{"a", "b"}},
	)
	var outs []ffargs.Output
	for i, l := range []string{"a", "b"} {
		o := string(rune('w' + i))
		sg = append(
			sg,
			ffargs.Chain{
				In:      []string{l},
				Filters: []ffargs.Filter{ffargs.SilenceDetect(30+5*i, 150)},
				Out:     []string{o},
			},
		)
		outs = append(outs, ffargs.Output{Format: "null", Maps: []string{"[" + o + "]"}})
	}
	runCmd(
		"silence",
		ffargs.Cmd{Inputs: []ffargs.Input{ffargs.FileInput(path("audio.wav"))}, Graph: sg, Outputs: outs},
		false,
	)
	argv, err := ffargs.Cmd{
		Inputs:  []ffargs.Input{ffargs.FileInput(path("audio.wav"))},
		Outputs: []ffargs.Output{{Format: "null", Maps: []string{"0:a:0"}, AF: ffargs.Chain1(ffargs.Ebur128())}},
	}.Argv(pol)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: time.Minute})
	if err != nil || !strings.Contains(res.Stderr, "Integrated loudness") {
		t.Errorf("loudness: %v\n%s", err, res.Stderr)
	}

	// 10: audio for transcription, from a file of the run directory.
	runCmd("transcription audio", ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(path("normalized.wav"))},
		Outputs: []ffargs.Output{{
			Format: "wav", Path: path("in.wav"), Maps: []string{"0:a:0"}, NoVideo: true, NoSubs: true, NoData: true,
			Opts: []ffargs.Opt{ffargs.Int("-ac", 1), ffargs.Int("-ar", 16000), ffargs.Enum("-c:a", "pcm_s16le")},
		}},
	}, false)
	exists("in.wav")

	// SDD-004 section 6: one decoded frame as raw RGBA on pipe:1.
	var frame bytes.Buffer
	argv, err = ffargs.Cmd{
		Inputs: []ffargs.Input{ffargs.FileInput(path("normalized.mov")).At(500)},
		Outputs: []ffargs.Output{
			{
				Format: "rawvideo",
				Pipe:   true,
				Maps:   []string{"0:v:0"},
				Opts:   []ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-pix_fmt", "rgba")},
				VF: ffargs.Chain1(
					ffargs.Crop(320, 240, 0, 0),
					ffargs.ScaleFrameToRGBA(160, 120),
					ffargs.Format("rgba"),
				),
			},
		},
	}.Argv(pol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: time.Minute, Stdout: &frame}); err != nil {
		t.Fatal(err)
	}
	if frame.Len() != 160*120*4 {
		t.Errorf("a frame is %d bytes, want %d", frame.Len(), 160*120*4)
	}

	// pipe:0 cannot be fed by this contrib/engine; the runner says so rather than hanging.
	argv, err = ffargs.Cmd{
		LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.PipeRawVideo("rgba", 8, 8, ffargs.Rate{Num: 30, Den: 1})},
		Outputs: []ffargs.Output{{Format: "null", Maps: []string{"0:v:0"}}},
	}.Argv(pol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunFFmpeg(
		ctx,
		Request{Dir: dir, Argv: argv, Deadline: time.Minute},
	); err == nil ||
		asError(t, err).Reason != "stdin_unsupported" {
		t.Errorf("pipe:0: %v", err)
	}

	// The probes of SDD-011 3.1, 4.9 and 8.4 on a produced file.
	for name, f := range map[string]func() ([]string, error){
		"json":    func() ([]string, error) { return ffargs.ProbeJSON(pol, path("normalized.mov")) },
		"packets": func() ([]string, error) { return ffargs.ProbePacketTimes(pol, path("normalized.mov"), 0) },
		"count":   func() ([]string, error) { return ffargs.ProbeCountPackets(pol, path("normalized.mov")) },
		"timing":  func() ([]string, error) { return ffargs.ProbeStreamTiming(pol, path("normalized.mov")) },
		"side":    func() ([]string, error) { return ffargs.ProbeFirstFrameSideData(pol, path("normalized.mov"), 0) },
	} {
		a, err := f()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, res, err := r.RunFFprobe(ctx, dir, a, ProbeTimeout)
		if err != nil || len(out) == 0 {
			t.Errorf("ffprobe %s: %v %q", name, err, res.Stderr)
		}
	}
}
