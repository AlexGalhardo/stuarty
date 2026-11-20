package ffargs

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func root() string {
	if runtime.GOOS == "windows" {
		return "C:/ws"
	}
	return "/ws"
}

const digest = "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"

func testPolicy() Policy {
	return Policy{
		StoreDir:   root() + "/store",
		RunDir:     root() + "/tmp/job_1",
		InputDirs:  []string{root() + "/artifacts/" + digest},
		MaxThreads: 4,
	}
}

func storeFile() string { return root() + "/store/ab/" + digest }
func run(name string) string {
	return root() + "/tmp/job_1/" + name
}

type scenario struct {
	name string
	argv func(pol Policy) []string
}

func must(a []string, err error) []string {
	if err != nil {
		panic("builder produced an argv the checker refuses: " + err.Error())
	}
	return a
}

// goldenScenarios builds the argument list of every command family of SDD-004 and SDD-011.
func goldenScenarios() []scenario {
	var (
		hexOut = []Opt{
			Enum("-colorspace", "bt709"),
			Enum("-color_primaries", "bt709"),
			Enum("-color_trc", "bt709"),
			Enum("-color_range", "tv"),
		}
		bx = Bitexact()
	)
	cat := func(parts ...[]Opt) []Opt {
		var o []Opt
		for _, p := range parts {
			o = append(o, p...)
		}
		return o
	}
	return []scenario{
		{"info", func(_ Policy) []string {
			var all []string
			for _, f := range infoFlags {
				a, err := Info(f)
				must(a, err)
				all = append(all, a...)
			}
			return all[:2]
		}},
		{"video pass", func(pol Policy) []string {
			g := Graph{
				{
					In:      []string{"0:0"},
					Filters: []Filter{SetPTS(), FPS(Rate{30, 1}), Format("yuv420p"), Split(2)},
					Out:     []string{"n", "p0"},
				},
				{
					In:      []string{"p0"},
					Filters: []Filter{Scale(540, 960, "bicubic"), Format("yuv420p"), Split(2)},
					Out:     []string{"q", "r"},
				},
				{
					In:      []string{"r"},
					Filters: []Filter{FPS(Rate{1, 1}), Scale(72, 128, "bicubic"), Format("yuvj420p")},
					Out:     []string{"s"},
				},
			}
			return must(Cmd{
				Progress: true, LogErrorOnly: true, FilterThreads: 4, Threads: 4,
				Inputs: []Input{FileInput(storeFile())}, Graph: g,
				Outputs: []Output{
					{
						Format:  "mov",
						Path:    run("video.mov"),
						Maps:    []string{"[n]"},
						NoAudio: true,
						NoSubs:  true,
						NoData:  true,
						Opts: cat(
							[]Opt{
								Enum("-c:v", "mpeg4"),
								Int("-q:v", 2),
								Int("-g", 15),
								Int("-bf", 0),
								Enum("-pix_fmt", "yuv420p"),
							},
							hexOut,
							[]Opt{Enum("-fps_mode", "cfr"), Int("-video_track_timescale", 30000)},
							bx,
						),
					},
					{
						Format:  "mp4",
						Path:    run("proxy-video.mp4"),
						Maps:    []string{"[q]"},
						NoAudio: true,
						NoSubs:  true,
						NoData:  true,
						Opts: cat([]Opt{
							Enum(
								"-c:v",
								"libvpx-vp9",
							),
							Int("-crf", 33),
							Bitrate("-b:v", "0"),
							Enum("-deadline", "realtime"),
							Int("-cpu-used", 8),
							Int(
								"-row-mt",
								1,
							),
							Int("-tile-columns", 1),
							Int("-lag-in-frames", 0),
							Int("-g", 15),
							Int("-keyint_min", 15),
							Enum("-pix_fmt", "yuv420p"),
						}, hexOut,
							[]Opt{Enum("-fps_mode", "cfr"), Int("-video_track_timescale", 30000)}, bx),
					},
					{
						Format:  "image2",
						Path:    run("thumbs/t%04d.jpg"),
						Maps:    []string{"[s]"},
						NoAudio: true,
						NoSubs:  true,
						NoData:  true,
						Opts: cat(
							[]Opt{Enum("-c:v", "mjpeg"), Int("-q:v", 4), Enum("-fps_mode", "cfr")},
							bx,
							[]Opt{Int("-start_number", 0)},
						),
					},
				},
			}.Argv(pol))
		}},
		{"audio pass", func(pol Policy) []string {
			return must(Cmd{
				Progress: true, LogErrorOnly: true,
				Inputs: []Input{FileInput(storeFile())},
				Outputs: []Output{
					{
						Format:  "wav",
						Path:    run("audio.wav"),
						Maps:    []string{"0:1"},
						NoVideo: true,
						NoSubs:  true,
						NoData:  true,
						AF: Chain1(
							AResample(48000),
							AFormatS16Stereo(),
							ADelay(14400),
							APad(2880000),
							ATrimEnd(2880000),
						),
						Opts: cat([]Opt{Enum("-c:a", "pcm_s16le"), Enum("-rf64", "auto")}, bx),
					},
				},
			}.Argv(pol))
		}},
		{"audio pass trim head", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{FileInput(storeFile())},
				Outputs: []Output{
					{
						Format:  "wav",
						Path:    run("audio.wav"),
						Maps:    []string{"0:1"},
						NoVideo: true,
						AF: Chain1(
							AResample(48000),
							AFormatS16Stereo(),
							ATrimStart(4800),
							ASetPTS(),
							APad(96000),
							ATrimEnd(96000),
						),
						Opts: []Opt{Enum("-c:a", "pcm_s16le")},
					},
				},
			}.Argv(pol))
		}},
		{"mux copy", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{FileInput(run("video.mov")), FileInput(run("audio.wav"))},
				Outputs: []Output{{
					Format: "mov", Path: run("normalized.mov"), Maps: []string{"0:v:0", "1:a:0"},
					Opts: cat([]Opt{Enum("-c:v", "copy"), Enum("-c:a", "copy")}, bx),
				}},
			}.Argv(pol))
		}},
		{"proxy audio", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{FileInput(run("audio.wav"))},
				Outputs: []Output{
					{
						Format:  "mp4",
						Path:    run("proxy-audio.mp4"),
						Maps:    []string{"0:a:0"},
						NoVideo: true,
						Opts: cat(
							[]Opt{Enum("-c:a", "aac"), Bitrate("-b:a", "128k"), Int("-ar", 48000), Int("-ac", 2)},
							bx,
						),
					},
				},
			}.Argv(pol))
		}},
		{"proxy mux", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{FileInput(run("proxy-video.mp4")), FileInput(run("proxy-audio.mp4"))},
				Outputs: []Output{{
					Format: "mp4", Path: run("proxy.mp4"), Maps: []string{"0:v:0", "1:a:0"},
					Opts: cat([]Opt{Enum("-c:v", "copy"), Enum("-c:a", "copy"), Enum("-movflags", "+faststart")}, bx),
				}},
			}.Argv(pol))
		}},
		{"posters", func(pol Policy) []string {
			g := Graph{
				{In: []string{"0:v:0"}, Filters: []Filter{Split(3)}, Out: []string{"a", "b", "c"}},
				{In: []string{"a"}, Filters: []Filter{ScaleMinWidth(160), Format("yuvj420p")}, Out: []string{"x"}},
				{In: []string{"b"}, Filters: []Filter{ScaleMinWidth(320), Format("yuvj420p")}, Out: []string{"y"}},
				{In: []string{"c"}, Filters: []Filter{ScaleMinWidth(640), Format("yuvj420p")}, Out: []string{"z"}},
			}
			o := func(l, n string) Output {
				return Output{
					Format: "image2",
					Path:   run(n),
					Maps:   []string{"[" + l + "]"},
					Opts: cat(
						[]Opt{Int("-frames:v", 1), Enum("-c:v", "mjpeg"), Int("-q:v", 4)},
						bx,
						[]Opt{Int("-update", 1)},
					),
				}
			}
			return must(Cmd{
				LogErrorOnly: true, Inputs: []Input{FileInput(run("proxy.mp4")).At(1000)}, Graph: g,
				Outputs: []Output{o("x", "w160.jpg"), o("y", "w320.jpg"), o("z", "w640.jpg")},
			}.Argv(pol))
		}},
		{"tile letterbox", func(pol Policy) []string {
			f := append(ScaleFit(256, 128), Format("yuvj420p"))
			return must(Cmd{
				LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
				Outputs: []Output{{
					Format: "image2", Path: run("tile.jpg"), Maps: []string{"0:v:0"}, VF: Chain1(f...),
					Opts: []Opt{Int("-frames:v", 1), Enum("-c:v", "mjpeg"), Int("-q:v", 4)},
				}},
			}.Argv(pol))
		}},
		{"image normalized", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
				Outputs: []Output{{
					Format: "image2", Path: run("normalized.png"), Maps: []string{"0:0"},
					VF:   Chain1(Scale(400, 300, "bicubic+accurate_rnd+full_chroma_int"), Format("rgb24")),
					Opts: cat([]Opt{Int("-frames:v", 1), Enum("-c:v", "png"), Int("-update", 1)}, bx),
				}},
			}.Argv(pol))
		}},
		{"audio only", func(pol Policy) []string {
			return must(Cmd{
				Progress: true, LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
				Outputs: []Output{
					{
						Format:  "wav",
						Path:    run("normalized.wav"),
						Maps:    []string{"0:0"},
						NoVideo: true,
						NoSubs:  true,
						NoData:  true,
						AF: Chain1(
							AResample(48000),
							AFormatS16Stereo(),
						),
						Opts: cat([]Opt{Enum("-c:a", "pcm_s16le"), Enum("-rf64", "auto")}, bx),
					},
				},
			}.Argv(pol))
		}},
		{"silence", func(pol Policy) []string {
			var g Graph
			g = append(g, Chain{In: []string{"0:a"}, Filters: []Filter{ASplit(4)}, Out: []string{"a", "b", "c", "d"}})
			var outs []Output
			for i, l := range []string{"a", "b", "c", "d"} {
				o := string(rune('w' + i))
				g = append(g, Chain{In: []string{l}, Filters: []Filter{SilenceDetect(30+5*i, 150)}, Out: []string{o}})
				outs = append(outs, Output{Format: "null", Maps: []string{"[" + o + "]"}})
			}
			return must(Cmd{Inputs: []Input{FileInput(run("audio.wav"))}, Graph: g, Outputs: outs}.Argv(pol))
		}},
		{"loudness", func(pol Policy) []string {
			return must(Cmd{
				Inputs:  []Input{FileInput(run("audio.wav"))},
				Outputs: []Output{{Format: "null", Maps: []string{"0:a:0"}, AF: Chain1(Ebur128())}},
			}.Argv(pol))
		}},
		{"transcription audio", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true, Inputs: []Input{FileInput(root() + "/artifacts/" + digest + "/normalized-1.wav")},
				Outputs: []Output{
					{
						Format:  "wav",
						Path:    run("in.wav"),
						Maps:    []string{"0:a:0"},
						NoVideo: true,
						NoSubs:  true,
						NoData:  true,
						Opts:    []Opt{Int("-ac", 1), Int("-ar", 16000), Enum("-c:a", "pcm_s16le")},
					},
				},
			}.Argv(pol))
		}},
		{"frame decode", func(pol Policy) []string {
			return must(Cmd{
				Threads: 2, Inputs: []Input{FileInput(root() + "/artifacts/" + digest + "/normalized-1.mov").At(2500)},
				Outputs: []Output{
					{
						Format: "rawvideo",
						Pipe:   true,
						Maps:   []string{"0:v:0"},
						VF: Chain1(
							Crop(1080, 1920, 0, 0),
							ScaleFrameToRGBA(540, 960),
							Format("rgba"),
						),
						Opts: []Opt{Enum("-pix_fmt", "rgba")},
					},
				},
			}.Argv(pol))
		}},
		{"view frame", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{FileInput(root() + "/artifacts/" + digest + "/normalized-1.mov").At(1000)},
				Outputs: []Output{
					{
						Format: "image2",
						Path:   run("f.jpg"),
						Maps:   []string{"0:v:0"},
						VF: Chain1(
							ScaleFrameToRGBA(582, 1036),
							Format("yuvj420p"),
						),
						Opts: []Opt{Int("-frames:v", 1), Enum("-c:v", "mjpeg"), Int("-q:v", 4), Int("-update", 1)},
					},
				},
			}.Argv(pol))
		}},
		{"export", func(pol Policy) []string {
			return must(Cmd{
				Progress: true, LogErrorOnly: true,
				Inputs: []Input{PipeRawVideo("rgba", 1080, 1920, Rate{30, 1}), FileInput(run("mix.wav"))},
				Outputs: []Output{{
					Format: "mp4", Path: run("job.partial.mp4"), Maps: []string{"0:v:0", "1:a:0"},
					VF: Chain1(Format("yuv420p")),
					Opts: cat(
						[]Opt{Enum("-c:v", "libopenh264"), Bitrate("-b:v", "8M"), Int("-g", 15)},
						hexOut,
						[]Opt{
							Enum("-c:a", "aac"),
							Bitrate("-b:a", "192k"),
							Int("-ar", 48000),
							Int("-ac", 2),
							Enum("-movflags", "+faststart"),
						},
					),
					Shortest: true,
				}},
			}.Argv(pol))
		}},
		{"mix input", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{PipeS16LE(48000, 2)},
				Outputs: []Output{
					{
						Format: "wav",
						Path:   run("mix.wav"),
						Maps:   []string{"0:a:0"},
						Opts:   []Opt{Enum("-c:a", "pcm_s16le")},
					},
				},
			}.Argv(pol))
		}},
		{"lavfi background", func(pol Policy) []string {
			return must(Cmd{
				LogErrorOnly: true,
				Inputs:       []Input{ColorSrc("#102030", 1080, 1920, Rate{30, 1}, 1000), AnullSrc(48000, 1000)},
				Outputs: []Output{{
					Format: "mp4", Path: run("bg.mp4"), Maps: []string{"0:v:0", "1:a:0"},
					Opts: []Opt{Enum("-c:v", "libopenh264"), Enum("-c:a", "aac")},
				}},
			}.Argv(pol))
		}},
		{"hdr capability", func(pol Policy) []string {
			f := append([]Filter{SetParamsHLG()}, ToneMapHDR("2020_ncl", "2020", "arib-std-b67", "tv")...)
			f = append(f, Format("yuv420p"))
			return must(Cmd{
				LogErrorOnly: true, Inputs: []Input{TestSrc2(320, 240, Rate{30, 1}, 500)},
				Outputs: []Output{{Format: "null", Maps: []string{"0:v:0"}, VF: Chain1(f...)}},
			}.Argv(pol))
		}},
		{"audio only aac", func(pol Policy) []string {
			return must(Cmd{
				Progress: true,
				Inputs:   []Input{FileInput(run("mix.wav"))},
				Outputs: []Output{{
					Format: "mp4", Path: run("job.partial.m4a"), Maps: []string{"0:a:0"}, NoVideo: true,
					Opts: cat(
						[]Opt{
							Enum("-c:a", "aac"), Bitrate("-b:a", "192k"), Int("-ar", 48000), Int("-ac", 2),
							Enum("-movflags", "+faststart"),
						},
						hexOut,
					),
				}},
			}.Argv(pol))
		}},
		{"4k 60 fps test encode", func(pol Policy) []string {
			return must(
				mustVal(
					EncoderTestAt(
						pol,
						"libopenh264",
						DetectInput{Filter: "format=yuv420p"},
						run("probe-4k.mp4"),
						3840,
						2160,
						Rate{60, 1},
					),
				),
			)
		}},
		{"vaapi test", func(pol Policy) []string {
			return must(
				mustVal(
					EncoderTest(
						pol,
						"h264_vaapi",
						DetectInput{VaapiDevice: "/dev/dri/renderD128", Filter: "format=nv12,hwupload"},
						run("probe-h264_vaapi.mp4"),
					),
				),
			)
		}},
	}
}

func mustVal(a []string, err error) ([]string, error) { return a, err }

func TestSecurity_T15_ArgvPolicy(t *testing.T) {
	pol := testPolicy()

	t.Run("accepts every golden scenario", func(t *testing.T) {
		for _, sc := range goldenScenarios() {
			a := sc.argv(pol)
			if len(a) == 0 {
				t.Errorf("%s: empty argv", sc.name)
			}
			if sc.name != "info" {
				if err := Check(FFmpeg, a, pol); err != nil {
					t.Errorf("%s: %v\n%s", sc.name, err, strings.Join(a, " "))
				}
			}
		}
		for _, enc := range []string{"libopenh264", "h264_nvenc", "h264_qsv", "h264_amf", "h264_mf", "h264_videotoolbox", "libx264"} {
			if _, err := EncoderTest(
				pol,
				enc,
				DetectInput{Filter: "format=yuv420p"},
				run("probe-"+enc+".mp4"),
			); err != nil {
				t.Errorf("encoder test %s: %v", enc, err)
			}
		}
		for _, f := range infoFlags {
			if _, err := Info(f); err != nil {
				t.Errorf("info %s: %v", f, err)
			}
		}
		probes := map[string]func() ([]string, error){
			"json":      func() ([]string, error) { return ProbeJSON(pol, storeFile()) },
			"packets":   func() ([]string, error) { return ProbePacketTimes(pol, storeFile(), 0) },
			"flags":     func() ([]string, error) { return ProbePacketFlags(pol, storeFile(), 0) },
			"side data": func() ([]string, error) { return ProbeFirstFrameSideData(pol, storeFile(), 0) },
			"count":     func() ([]string, error) { return ProbeCountPackets(pol, storeFile()) },
			"anim":      func() ([]string, error) { return ProbeAnimPackets(pol, storeFile(), 0) },
			"timing":    func() ([]string, error) { return ProbeStreamTiming(pol, run("normalized.mov")) },
			"encoded":   func() ([]string, error) { return ProbeEncodedPackets(pol, run("probe-x.mp4")) },
			// the WebP still of an image export (SDD-021 section 6), lossless and lossy
			"webp lossless": func() ([]string, error) { return WebPStill(pol, run("in.png"), run("out.webp"), 0) },
			"webp lossy":    func() ([]string, error) { return WebPStill(pol, run("in.png"), run("out.webp"), 100) },
		}
		for name, f := range probes {
			if _, err := f(); err != nil {
				t.Errorf("probe %s: %v", name, err)
			}
		}
	})

	good := goldenScenarios()[1].argv(pol) // the video pass
	mutate := func(old, repl string) []string {
		out := make([]string, 0, len(good))
		done := false
		for _, a := range good {
			if !done && a == old {
				done = true
				if repl == "" {
					continue
				}
				out = append(out, repl)
				continue
			}
			out = append(out, a)
		}
		if !done {
			t.Fatalf("fixture has no %q", old)
		}
		return out
	}
	withPrefix := func(old, newPrefix string) []string {
		out := append([]string(nil), good...)
		for i, a := range out {
			if a == old || strings.HasPrefix(a, old) {
				out[i] = newPrefix
				return out
			}
		}
		t.Fatalf("fixture has no %q", old)
		return nil
	}

	hostile := map[string][]string{
		"input without file:":           withPrefix("file:"+storeFile(), storeFile()),
		"http input":                    withPrefix("file:"+storeFile(), "http://example.com/a.mp4"),
		"https input":                   withPrefix("file:"+storeFile(), "https://example.com/a.mp4"),
		"concat input":                  withPrefix("file:"+storeFile(), "concat:a|b"),
		"pipe input without format":     withPrefix("file:"+storeFile(), "pipe:0"),
		"-f concat":                     mutate("mov", "concat"),
		"-f tee":                        mutate("mov", "tee"),
		"-f hls":                        mutate("mov", "hls"),
		"-f segment":                    mutate("mov", "segment"),
		"no protocol whitelist":         mutate("-protocol_whitelist", ""),
		"wide protocol whitelist":       mutate("file,pipe", "file,pipe,http"),
		"filter movie":                  withGraph(good, "movie=/etc/passwd[m]"),
		"filter subtitles":              withGraph(good, "[0:0]subtitles=/x/y.srt[n]"),
		"filter ass":                    withGraph(good, "[0:0]ass=a.ass[n]"),
		"filter drawtext":               withGraph(good, "[0:0]drawtext=text=hi[n]"),
		"filter fontfile":               withGraph(good, "[0:0]drawtext=fontfile=/x.ttf:text=a[n]"),
		"filter outside the allowlist":  withGraph(good, "[0:0]hue=s=0[n]"),
		"filter overlay":                withGraph(good, "[0:0]overlay[n]"),
		"filter eq":                     withGraph(good, "[0:0]eq=brightness=0.1[n]"),
		"filter option outside grammar": withGraph(good, "[0:0]scale=10:10:sws_flags=x[n]"),
		"filter with a quote escape":    withGraph(good, "[0:0]scale=w='iw':h='1;movie=x'[n]"),
		"filter script": append(
			append([]string(nil), good[:len(good)-1]...),
			"-filter_script:v",
			"x",
			good[len(good)-1],
		),
		"overwrite -y":       append([]string{"-nostdin", "-hide_banner", "-nostats", "-y"}, good[3:]...),
		"missing -n":         mutate("-n", ""),
		"missing -max_alloc": withoutPair(good, "-max_alloc"),
		"missing preamble":   good[1:],
		"unknown option": append(
			[]string{"-nostdin", "-hide_banner", "-nostats", "-headers", "x"},
			good[3:]...),
		"-metadata":               insertBeforeLast(good, "-metadata", "title=x"),
		"-attach":                 insertBeforeLast(good, "-attach", "/etc/passwd"),
		"-dump_attachment":        insertBeforeLast(good, "-dump_attachment:t", ""),
		"-fflags beyond bitexact": mutate("+bitexact", "+genpts"),
		"-enable_drefs": append(
			[]string{"-nostdin", "-hide_banner", "-nostats", "-enable_drefs", "1"},
			good[3:]...),
		"-allowed_extensions": append(
			[]string{"-nostdin", "-hide_banner", "-nostats", "-allowed_extensions", "ALL"},
			good[3:]...),
		"output outside the run dir": withPrefix("file:"+run("video.mov"), "file:"+root()+"/exports/x.mp4"),
		"output traversal":           withPrefix("file:"+run("video.mov"), "file:"+run("../../etc/x.mov")),
		"output without file:":       withPrefix("file:"+run("video.mov"), run("video.mov")),
		"output to a url":            withPrefix("file:"+run("video.mov"), "rtmp://example.com/live"),
		"output with a backslash": withPrefix(
			"file:"+run("video.mov"),
			"file:"+strings.ReplaceAll(run("video.mov"), "/", `\`),
		),
		"output with a pattern":          withPrefix("file:"+run("video.mov"), "file:"+run("a%d%s.mov")),
		"input outside the store":        withPrefix("file:"+storeFile(), "file:"+root()+"/other/ab/"+digest),
		"store path with a short digest": withPrefix("file:"+storeFile(), "file:"+root()+"/store/ab/abcdef"),
		"store path with upper digest": withPrefix(
			"file:"+storeFile(),
			"file:"+root()+"/store/AB/"+strings.ToUpper(digest),
		),
		"store path with a user name": withPrefix("file:"+storeFile(), "file:"+root()+"/store/ab/movie.mp4"),
		"input traversal":             withPrefix("file:"+storeFile(), "file:"+root()+"/store/ab/../../etc/passwd"),
		"input from a drive-relative": withPrefix("file:"+storeFile(), "file:C:relative.mp4"),
		"missing -map":                withoutPair(good, "-map"),
		"lavfi from a file":           withPrefix("file:"+storeFile(), "movie=x"),
		"@file value":                 mutate("-progress", "@x"),
		"unbalanced graph":            withGraph(good, "[0:0]scale=w='min(1,iw:h=2[n]"),
		"graph with a newline":        withGraph(good, "[0:0]format=yuv420p\n,movie=x[n]"),
		"graph with a backslash":      withGraph(good, `[0:0]scale=w=\iw[n]`),
	}
	for name, argv := range hostile {
		t.Run("rejects "+name, func(t *testing.T) {
			err := Check(FFmpeg, argv, pol)
			if err == nil {
				t.Fatalf("accepted:\n%s", strings.Join(argv, " "))
			}
			var v *ViolationError
			if !errors.As(err, &v) || !errors.Is(err, ErrPolicy) {
				t.Fatalf("error is not a policy violation: %v", err)
			}
			if !strings.HasPrefix(v.Rule, "FF-") {
				t.Errorf("violation carries no rule: %v", v)
			}
		})
	}

	t.Run("lavfi hostile sources", func(t *testing.T) {
		for _, src := range []string{
			"movie=/etc/passwd", "color=c=red", "color=c=#102030:s=10x10:r=30/1:d=1;movie=x", "nullsrc=s=10x10,movie=x",
			"amovie=x", "testsrc2=s=10x10:r=30/1:d=1:extra=1", "sine=f=1", "color=c=#102030:s=10x10:r=30/1:d=-1",
		} {
			a := []string{
				"-nostdin",
				"-hide_banner",
				"-nostats",
				"-n",
				"-max_alloc",
				"1073741824",
				"-protocol_whitelist",
				"file,pipe",
				"-f",
				"lavfi",
				"-i",
				src,
				"-map",
				"0:v:0",
				"-map_metadata",
				"-1",
				"-map_chapters",
				"-1",
				"-f",
				"null",
				"-",
			}
			if Check(FFmpeg, a, pol) == nil {
				t.Errorf("accepted lavfi source %q", src)
			}
		}
	})

	t.Run("thread budget", func(t *testing.T) {
		a := goldenScenarios()[1].argv(pol)
		low := pol
		low.MaxThreads = 2
		if Check(FFmpeg, a, low) == nil {
			t.Errorf("accepted -threads 4 with a budget of 2")
		}
		low.MaxThreads = 0
		if Check(FFmpeg, a, low) == nil {
			t.Errorf("accepted -threads with no budget")
		}
	})

	t.Run("ffprobe", func(t *testing.T) {
		okJSON, _ := ProbeJSON(pol, storeFile())
		bad := map[string][]string{
			"-show_packets":  replaceTok(okJSON, "-show_streams", "-show_packets"),
			"no probesize":   dropPair(okJSON, "-probesize"),
			"no whitelist":   dropPair(okJSON, "-protocol_whitelist"),
			"wide whitelist": replaceTok(okJSON, "file", "file,pipe"),
			"http":           replaceTok(okJSON, "file:"+storeFile(), "http://x/y"),
			"outside store":  replaceTok(okJSON, "file:"+storeFile(), "file:"+root()+"/etc/passwd"),
			"-nostdin":       append([]string{"-v", "error", "-hide_banner", "-nostdin"}, okJSON[3:]...),
			"-show_frames full": append(
				append([]string(nil), okJSON[:len(okJSON)-2]...),
				"-show_frames",
				"-i",
				"file:"+storeFile(),
			),
			"-i not last": append(
				append([]string(nil), okJSON[:len(okJSON)-2]...),
				"-i",
				"file:"+storeFile(),
				"-show_format",
			),
			"unknown option": append(
				append([]string(nil), okJSON[:len(okJSON)-2]...),
				"-headers",
				"x",
				"-i",
				"file:"+storeFile(),
			),
			"all packets": append(
				append([]string(nil), okJSON[:len(okJSON)-2]...),
				"-show_entries",
				"packet=pts,data",
				"-i",
				"file:"+storeFile(),
			),
		}
		for name, a := range bad {
			if Check(FFprobe, a, pol) == nil {
				t.Errorf("accepted ffprobe argv %s: %s", name, strings.Join(a, " "))
			}
		}
	})

	t.Run("allowed format names", func(t *testing.T) {
		for _, n := range []string{"mov,mp4,m4a,3gp,3g2,mj2", "matroska,webm", "wav", "png_pipe", "mpegts", "image2", "gif"} {
			if !AllowedFormatName(n) {
				t.Errorf("%q refused", n)
			}
		}
		for _, n := range []string{"", "hls", "concat", "ffconcat", "rtsp", "lavfi", "ass", "srt", "mov,hls", "tee", "sdp", "dash"} {
			if AllowedFormatName(n) {
				t.Errorf("%q accepted", n)
			}
		}
	})
}

func withGraph(good []string, g string) []string {
	out := append([]string(nil), good...)
	for i, a := range out {
		if a == "-filter_complex" {
			out[i+1] = g
		}
	}
	return out
}

func replaceTok(a []string, old, repl string) []string {
	out := append([]string(nil), a...)
	for i, t := range out {
		if t == old {
			out[i] = repl
			return out
		}
	}
	panic("no token " + old)
}

func dropPair(a []string, name string) []string {
	var out []string
	for i := 0; i < len(a); i++ {
		if a[i] == name {
			i++
			continue
		}
		out = append(out, a[i])
	}
	return out
}

func withoutPair(a []string, name string) []string {
	var out []string
	for i := 0; i < len(a); i++ {
		if a[i] == name {
			if name != "-n" {
				i++
			}
			continue
		}
		out = append(out, a[i])
	}
	return out
}

func insertBeforeLast(a []string, name, value string) []string {
	out := append([]string(nil), a[:len(a)-1]...)
	out = append(out, name)
	if value != "" {
		out = append(out, value)
	}
	return append(out, a[len(a)-1])
}

// An input may force one decoder, libvpx-vp9, which reads the alpha plane of VP9 in WebM (VV-013);
// no other decoder and no forced decoder on a synthetic input is accepted.
func TestSecurity_ForcedInputDecoder(t *testing.T) {
	pol := testPolicy()
	build := func(in Input) ([]string, error) {
		return Cmd{
			LogErrorOnly: true, Inputs: []Input{in},
			Outputs: []Output{{
				Format: "mov", Path: run("normalized.mov"), Maps: []string{"0:v:0"}, NoAudio: true,
				Opts: append([]Opt{Enum("-c:v", "prores_ks")}, Enum("-pix_fmt", "yuva444p10le")),
			}},
		}.Argv(pol)
	}

	argv, err := build(FileInput(storeFile()).Decoder("libvpx-vp9"))
	if err != nil {
		t.Fatalf("libvpx-vp9 on a file input: %v", err)
	}

	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "-c:v libvpx-vp9 -i file:") {
		t.Errorf("the decoder is not an input option before -i: %s", joined)
	}

	for _, name := range []string{"h264", "vp9", "prores_ks", "../x"} {
		if _, err := build(FileInput(storeFile()).Decoder(name)); err == nil {
			t.Errorf("decoder %q was accepted", name)
		}
	}

	if _, err := build(TestSrc2(64, 64, Rate{Num: 30, Den: 1}, 1000).Decoder("libvpx-vp9")); err == nil {
		t.Error("a forced decoder on a synthetic input was accepted")
	}
}

// The alpha proxy is VP9 with an alpha plane and Opus in WebM (VV-013): the output format, the
// pixel format, the audio codec and the one libvpx option it needs are on the grammar, and their
// neighbours (another container, a value other than 0, another audio codec) are not.
func TestSecurity_AlphaProxyGrammar(t *testing.T) {
	pol := testPolicy()
	video := func(altRef string) ([]string, error) {
		return Cmd{
			LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
			Outputs: []Output{{
				Format: "webm", Path: run("proxy-video.webm"), Maps: []string{"0:v:0"}, NoAudio: true,
				Opts: []Opt{
					Enum("-c:v", "libvpx-vp9"), Enum("-pix_fmt", "yuva420p"), Enum("-auto-alt-ref", altRef),
				},
			}},
		}.Argv(pol)
	}
	argv, err := video("0")
	if err != nil {
		t.Fatalf("the alpha proxy video: %v", err)
	}
	if joined := strings.Join(argv, " "); !strings.Contains(joined, "-auto-alt-ref 0") ||
		!strings.Contains(joined, "-f webm") {
		t.Errorf("argv lacks the alpha options: %s", joined)
	}

	audio := func(codec, format string) error {
		_, err := Cmd{
			LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
			Outputs: []Output{{
				Format: format, Path: run("proxy-audio.webm"), Maps: []string{"0:a:0"}, NoVideo: true,
				Opts: []Opt{Enum("-c:a", codec), Bitrate("-b:a", "128k")},
			}},
		}.Argv(pol)
		return err
	}
	if err := audio("libopus", "webm"); err != nil {
		t.Errorf("the alpha proxy audio: %v", err)
	}
	for _, c := range []struct{ codec, format string }{
		{"libvorbis", "webm"}, {"opus", "webm"}, {"libopus", "matroska"}, {"libopus", "ogg"},
	} {
		if audio(c.codec, c.format) == nil {
			t.Errorf("%s in %s was accepted", c.codec, c.format)
		}
	}
	if _, err := video("1"); err == nil {
		t.Error("-auto-alt-ref 1 was accepted")
	}

	// The tag that keeps the alpha plane visible is the one metadata option, with the one value.
	tag := func(opt, value string) error {
		_, err := Cmd{
			LogErrorOnly: true, Inputs: []Input{FileInput(storeFile())},
			Outputs: []Output{{
				Format: "webm", Path: run("proxy.webm"), Maps: []string{"0:v:0"}, NoAudio: true,
				Opts: []Opt{Enum("-c:v", "copy"), Enum(opt, value)},
			}},
		}.Argv(pol)
		return err
	}
	if err := tag("-metadata:s:v:0", "alpha_mode=1"); err != nil {
		t.Errorf("alpha_mode tag: %v", err)
	}
	for _, c := range [][2]string{
		{"-metadata:s:v:0", "alpha_mode=0"},
		{"-metadata:s:v:0", "title=x"},
		{"-metadata", "alpha_mode=1"},
		{"-metadata:s:a:0", "alpha_mode=1"},
	} {
		if tag(c[0], c[1]) == nil {
			t.Errorf("%s %s was accepted", c[0], c[1])
		}
	}
}

// On x86-64 every run masks the AVX-512 code of FFmpeg (SDD-007 FF-10); nothing else may be given to -cpuflags.
func TestCPUFlagsAreMaskedOnX86AndNothingElseIsAccepted(t *testing.T) {
	t.Parallel()

	pol := testPolicy()

	argv, err := Cmd{
		Inputs:  []Input{FileInput(storeFile())},
		Outputs: []Output{{Format: "null", Maps: []string{"0:v:0"}}},
	}.Argv(pol)
	if err != nil {
		t.Fatal(err)
	}

	has := slices.Contains(argv, "-cpuflags")
	if has != (runtime.GOARCH == "amd64") {
		t.Fatalf("-cpuflags present %v on %s: %v", has, runtime.GOARCH, argv)
	}

	if has {
		i := slices.Index(argv, "-cpuflags")
		if argv[i+1] != CPUMask {
			t.Fatalf("-cpuflags %s, want %s", argv[i+1], CPUMask)
		}

		bad := slices.Clone(argv)
		bad[i+1] = "0"

		if err := Check(FFmpeg, bad, pol); err == nil {
			t.Fatal("-cpuflags 0 was accepted")
		}
	}
}
