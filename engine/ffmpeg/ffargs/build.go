package ffargs

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Rate is a rational frame rate or sample rate.
type Rate struct{ Num, Den int }

func (r Rate) String() string { return fmt.Sprintf("%d/%d", r.Num, r.Den) }

// Filter is one rendered filter. Only the constructors below make one, from numbers and
// fixed names; the checker validates the result like any other graph.
type Filter string

// Scale scales to w x h with the given scaler flags.
func Scale(w, h int, flags string) Filter {
	return Filter(fmt.Sprintf("scale=%d:%d:flags=%s", w, h, flags))
}

// ScaleRange is the scale-only colour conversion of SDD-011 4.3.1 for a build with no zscale.
func ScaleRange(w, h int, flags, inRange, inMatrix string) Filter {
	return Filter(
		fmt.Sprintf(
			"scale=%d:%d:flags=%s:in_range=%s:out_range=tv:in_color_matrix=%s:out_color_matrix=bt709",
			w,
			h,
			flags,
			inRange,
			inMatrix,
		),
	)
}

// ScaleMinWidth scales to a width that never exceeds the source, keeping an even height (posters).
func ScaleMinWidth(w int) Filter {
	return Filter(fmt.Sprintf("scale=w='min(%d,iw)':h=-2:flags=bicubic", w))
}

// ScaleFit fits into w x h and letterboxes (tiles wider than 256 pixels).
func ScaleFit(w, h int) []Filter {
	return []Filter{
		Filter(fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", w, h)),
		Filter(fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2", w, h)),
	}
}

// ScaleFrameToRGBA is the decoder-side scale of SDD-004 section 6: BT.709 limited to RGB.
func ScaleFrameToRGBA(w, h int) Filter {
	return Filter(
		fmt.Sprintf("scale=%d:%d:flags=bicubic+accurate_rnd+full_chroma_int:in_range=tv:in_color_matrix=bt709", w, h),
	)
}

// ScaleBT709Out converts RGB to BT.709 limited-range YUV with the colour matrix named, for the
// encoder input of the export (SDD-004 section 8).
func ScaleBT709Out() Filter { return "scale=out_color_matrix=bt709:out_range=tv" }

// FPS makes the frame rate constant at r, taking the nearest frame for each slot.
func FPS(r Rate) Filter { return Filter(fmt.Sprintf("fps=%s:round=near", r)) }

// Format converts to one of the pixel formats, in order of preference.
func Format(pixFmts ...string) Filter { return Filter("format=" + strings.Join(pixFmts, "|")) }

// SetPTS makes the first frame time zero.
func SetPTS() Filter { return "setpts=PTS-STARTPTS" }

// ASetPTS makes the first audio sample time zero.
func ASetPTS() Filter { return "asetpts=PTS-STARTPTS" }

// SetSAR1 sets square pixels.
func SetSAR1() Filter { return "setsar=1" }

// Split copies a video link to n outputs.
func Split(n int) Filter { return Filter("split=" + strconv.Itoa(n)) }

// ASplit copies an audio link to n outputs.
func ASplit(n int) Filter { return Filter("asplit=" + strconv.Itoa(n)) }

// Crop cuts a w x h rectangle at x, y. exact=1 keeps x, y, w and h as given on a subsampled source:
// without it FFmpeg rounds x and y down to the chroma grid (an odd x moves the picture one pixel).
func Crop(w, h, x, y int) Filter { return Filter(fmt.Sprintf("crop=%d:%d:%d:%d:exact=1", w, h, x, y)) }

// CropRect cuts a w x h rectangle at x, y so that every plane of a subsampled source is cut at that
// corner. Crop with exact=1 keeps the luma at x, y but places the chroma of a 4:2:0 source on its own
// grid (x/2, y/2 rounded down), which moves the colour one source pixel off the picture when x or y is
// odd; such a corner converts to full-resolution chroma first.
func CropRect(w, h, x, y int) []Filter {
	if x%2 == 0 && y%2 == 0 {
		return []Filter{Crop(w, h, x, y)}
	}

	return []Filter{Format("yuv444p"), Crop(w, h, x, y)}
}

// Bwdif deinterlaces into one frame per source frame.
func Bwdif() Filter { return "bwdif=mode=send_frame:parity=auto:deint=interlaced" }

// Yadif is the fallback of Bwdif with the same options.
func Yadif() Filter { return "yadif=mode=send_frame:parity=auto:deint=interlaced" }

// AResample resamples audio to rate hertz.
func AResample(rate int) Filter { return Filter("aresample=" + strconv.Itoa(rate)) }

// AFormatS16Stereo converts audio to 16-bit stereo.
func AFormatS16Stereo() Filter { return "aformat=sample_fmts=s16:channel_layouts=stereo" }

// ADelay inserts samples of silence at the head of the audio.
func ADelay(samples int) Filter { return Filter(fmt.Sprintf("adelay=%dS:all=1", samples)) }

// APad pads the audio with silence up to wholeLen samples.
func APad(wholeLen int) Filter { return Filter("apad=whole_len=" + strconv.Itoa(wholeLen)) }

// ATrimEnd cuts the audio after endSample.
func ATrimEnd(endSample int) Filter { return Filter("atrim=end_sample=" + strconv.Itoa(endSample)) }

// ATrimStart drops the audio before startSample.
func ATrimStart(startSample int) Filter {
	return Filter("atrim=start_sample=" + strconv.Itoa(startSample))
}

// ATempo changes the tempo of audio without changing its pitch. One filter takes 0.5 to 100;
// the export chains them for a speed outside 0.5 to 2 (SDD-004 section 7).
func ATempo(tempo float64) Filter { return Filter(fmt.Sprintf("atempo=%.6f", tempo)) }

// AFFTDN reduces the noise of audio by nr decibels (0.01 to 97), taking the noise to lie at nf dBFS
// (-80 to -20) and tracking it as it goes (SDD-004 section 7).
func AFFTDN(nr, nf float64) Filter { return Filter(fmt.Sprintf("afftdn=nr=%.2f:nf=%.2f:tn=1", nr, nf)) }

// Ebur128 measures loudness and prints the summary on stderr.
func Ebur128() Filter { return "ebur128=peak=true:framelog=quiet" }

// SilenceDetect reports silences below -db decibels that last at least minMS milliseconds.
func SilenceDetect(db, minMS int) Filter {
	return Filter(fmt.Sprintf("silencedetect=noise=-%ddB:d=%d.%03d", db, minMS/1000, minMS%1000))
}

// ZScale converts colour with explicit input properties (SDD-011 4.3.1). Each argument is an
// enumerated name that the checker validates.
func ZScale(matrixIn, primariesIn, transferIn, rangeIn string) Filter {
	return Filter(
		fmt.Sprintf(
			"zscale=min=%s:pin=%s:tin=%s:rin=%s:m=709:p=709:t=709:r=limited:d=ordered",
			matrixIn,
			primariesIn,
			transferIn,
			rangeIn,
		),
	)
}

// ToneMapHDR is the HLG and PQ chain of SDD-011 4.3.1 (hable, npl=100, desat=0).
func ToneMapHDR(matrixIn, primariesIn, transferIn, rangeIn string) []Filter {
	return []Filter{
		Filter(
			fmt.Sprintf(
				"zscale=min=%s:pin=%s:tin=%s:rin=%s:t=linear:npl=100",
				matrixIn,
				primariesIn,
				transferIn,
				rangeIn,
			),
		),
		Format("gbrpf32le"),
		"zscale=p=709",
		"tonemap=tonemap=hable:desat=0",
		"zscale=m=709:p=709:t=709:r=limited:d=ordered",
	}
}

// SetParamsBT709 tags the frames BT.709 limited range. FFmpeg writes the colour tags of an encoder
// from the frames, so a source with no tags needs this before the encoder (SDD-011 4.3.1).
func SetParamsBT709() Filter {
	return "setparams=colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv"
}

// SetParamsHLG tags a synthetic source as BT.2020 HLG, for the capability test of SDD-011 8.3.
func SetParamsHLG() Filter {
	return "setparams=colorspace=bt2020nc:color_primaries=bt2020:color_trc=arib-std-b67:range=tv"
}

// Chain is one filter chain with its input and output link labels.
type Chain struct {
	In      []string
	Filters []Filter
	Out     []string
}

// Graph is a -filter_complex value.
type Graph []Chain

func label(l string) string { return "[" + l + "]" }

func (g Graph) String() string {
	var parts []string
	for i := range g {
		c := &g[i]
		var b strings.Builder
		for _, l := range c.In {
			b.WriteString(label(l))
		}
		for i, f := range c.Filters {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(string(f))
		}
		for _, l := range c.Out {
			b.WriteString(label(l))
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, ";")
}

// Chain1 renders filters as a plain -vf or -af value.
func Chain1(f ...Filter) string { return Graph{{Filters: f}}.String() }

// Input is one -i.
type Input struct {
	path    string // absolute path, for a file input
	pipe    string // "rawvideo" or "s16le" for pipe:0
	lavfi   string // source for -f lavfi
	pixFmt  string
	size    string
	rate    string
	ar, ac  int
	seekMS  int64
	decoder string // forces the decoder of a file input (-c:v before -i)
}

// FileInput reads the file at an absolute path (a store file or a file of the run).
func FileInput(path string) Input { return Input{path: toSlash(path)} }

// PipeRawVideo reads raw frames of the given pixel format and size on pipe:0.
func PipeRawVideo(pixFmt string, w, h int, rate Rate) Input {
	return Input{pipe: "rawvideo", pixFmt: pixFmt, size: fmt.Sprintf("%dx%d", w, h), rate: rate.String()}
}

// PipeS16LE reads interleaved 16-bit samples on pipe:0.
func PipeS16LE(sampleRate, channels int) Input {
	return Input{pipe: "s16le", ar: sampleRate, ac: channels}
}

// TestSrc2 is the synthetic picture of the encoder and capability tests.
func TestSrc2(w, h int, rate Rate, durMS int) Input {
	return Input{lavfi: fmt.Sprintf("testsrc2=s=%dx%d:r=%s:d=%s", w, h, rate, seconds(int64(durMS)))}
}

// ColorSrc is a solid background (FF-12).
func ColorSrc(hexColour string, w, h int, rate Rate, durMS int) Input {
	return Input{lavfi: fmt.Sprintf("color=c=%s:s=%dx%d:r=%s:d=%s", hexColour, w, h, rate, seconds(int64(durMS)))}
}

// AnullSrc is silent audio (FF-12).
func AnullSrc(sampleRate, durMS int) Input {
	return Input{lavfi: fmt.Sprintf("anullsrc=r=%d:cl=stereo:d=%s", sampleRate, seconds(int64(durMS)))}
}

// At starts the input at ms (an input option, so the seek is accurate and fast).
func (in Input) At(ms int64) Input { in.seekMS = ms; return in }

// Decoder forces the video decoder of a file input. The only one the grammar accepts is libvpx-vp9,
// which is what reads the alpha plane of a VP9 stream in WebM; the native decoder drops it.
func (in Input) Decoder(name string) Input { in.decoder = name; return in }

func seconds(ms int64) string {
	return fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
}

// Opt is one output option made by a typed constructor.
type Opt struct{ name, value string }

// Pair is the option name and its value, for a test that compares what a command carries.
func (o Opt) Pair() (name, value string) { return o.name, o.value }

// Int is an option with an integer value.
func Int(name string, v int) Opt { return Opt{name, strconv.Itoa(v)} }

// Enum is an option with a value of a fixed set.
func Enum(name, v string) Opt { return Opt{name, v} }

// Bitrate is an option with a bit rate value such as 8M or 128k.
func Bitrate(name string, v string) Opt { return Opt{name, v} }

// Bitexact makes the output reproducible: no encoder tag, no creation time.
func Bitexact() []Opt { return []Opt{{"-fflags", "+bitexact"}, {"-flags", "+bitexact"}} }

// Output is one output file or pipe.
type Output struct {
	Format                           string // mp4, mov, wav, image2, rawvideo, s16le, null
	Path                             string // absolute path in the run directory; "" for null; ignored for pipes
	Pipe                             bool   // pipe:1 (rawvideo or s16le)
	Maps                             []string
	NoAudio, NoVideo, NoSubs, NoData bool
	Shortest                         bool
	VF, AF                           string // a plain filter chain, from Chain1
	Opts                             []Opt
}

// Cmd is one ffmpeg run.
type Cmd struct {
	Progress      bool // -progress pipe:1
	LogErrorOnly  bool // -v error
	Threads       int  // input decoder threads; zero leaves FFmpeg's default
	FilterThreads int  // threads of the filter graph, simple or complex; zero leaves FFmpeg's default
	VaapiDevice   string
	Inputs        []Input
	Graph         Graph
	Outputs       []Output
}

// CPUMask is the one value of -cpuflags: FFmpeg runs on an x86-64 machine without its AVX-512 code (SDD-007
// FF-10, SDD-011 NFR-010). The pixels and samples it writes are then those of the SSE and AVX2 tiers, which
// TestNormalizeIsTheSameOnEveryCPUTier shows equal to the plain C code, whatever the CPU of the runner.
const CPUMask = "-avx512-avx512icl"

// Argv renders the command and runs it through [Check], so an argv that leaves the
// builder is one the grammar accepts.
func (c Cmd) Argv(pol Policy) ([]string, error) {
	a := []string{"-nostdin", "-hide_banner", "-nostats"}
	if c.LogErrorOnly {
		a = append(a, "-v", "error")
	}
	if c.Progress {
		a = append(a, "-progress", "pipe:1")
	}
	a = append(a, "-n", "-max_alloc", strconv.Itoa(MaxAlloc))
	if runtime.GOARCH == "amd64" {
		a = append(a, "-cpuflags", CPUMask)
	}
	if c.FilterThreads > 0 {
		// -filter_threads sizes the simple graphs (-vf); a -filter_complex graph takes its own option.
		opt := "-filter_threads"
		if len(c.Graph) > 0 {
			opt = "-filter_complex_threads"
		}
		a = append(a, opt, strconv.Itoa(c.FilterThreads))
	}
	if c.VaapiDevice != "" {
		a = append(a, "-vaapi_device", c.VaapiDevice)
	}
	for i := range c.Inputs {
		a = c.Inputs[i].appendTo(a, c.Threads)
	}
	if len(c.Graph) > 0 {
		a = append(a, "-filter_complex", c.Graph.String())
	}
	for i := range c.Outputs {
		a = c.Outputs[i].appendTo(a)
	}
	if err := Check(FFmpeg, a, pol); err != nil {
		return nil, err
	}
	return a, nil
}

// appendTo adds the whitelist options, the input options and the -i of one input.
func (in *Input) appendTo(a []string, threads int) []string {
	a = append(a, "-protocol_whitelist", "file,pipe")
	if in.path != "" {
		a = append(a, "-format_whitelist", InputFormats, "-sn", "-dn")
	}
	if threads > 0 {
		a = append(a, "-threads", strconv.Itoa(threads))
	}
	if in.seekMS > 0 {
		a = append(a, "-ss", seconds(in.seekMS))
	}
	if in.decoder != "" {
		a = append(a, "-c:v", in.decoder)
	}
	switch {
	case in.lavfi != "":
		return append(a, "-f", "lavfi", "-i", in.lavfi)
	case in.pipe == "rawvideo":
		return append(a, "-f", "rawvideo", "-pix_fmt", in.pixFmt, "-s", in.size, "-framerate", in.rate, "-i", "pipe:0")
	case in.pipe == "s16le":
		return append(a, "-f", "s16le", "-ar", strconv.Itoa(in.ar), "-ac", strconv.Itoa(in.ac), "-i", "pipe:0")
	}
	return append(a, "-i", "file:"+in.path)
}

// appendTo adds the options and the destination of one output.
func (o *Output) appendTo(a []string) []string {
	for _, m := range o.Maps {
		a = append(a, "-map", m)
	}
	for _, d := range []struct {
		on   bool
		flag string
	}{{o.NoVideo, "-vn"}, {o.NoAudio, "-an"}, {o.NoSubs, "-sn"}, {o.NoData, "-dn"}} {
		if d.on {
			a = append(a, d.flag)
		}
	}
	a = append(a, "-map_metadata", "-1", "-map_chapters", "-1")
	if o.VF != "" {
		a = append(a, "-vf", o.VF)
	}
	if o.AF != "" {
		a = append(a, "-af", o.AF)
	}
	for _, op := range o.Opts {
		a = append(a, op.name, op.value)
	}
	if o.Shortest {
		a = append(a, "-shortest")
	}
	a = append(a, "-f", o.Format)
	switch {
	case o.Format == "null":
		return append(a, "-")
	case o.Pipe:
		return append(a, "pipe:1")
	}
	return append(a, "file:"+filepath.ToSlash(o.Path))
}

// Info is a capability listing: -hide_banner plus -version, -encoders, -decoders, -filters, -muxers or -demuxers.
func Info(flag string) ([]string, error) {
	a := []string{"-hide_banner", flag}
	if err := Check(FFmpeg, a, Policy{}); err != nil {
		return nil, err
	}
	return a, nil
}

func probeBase(extra ...string) []string {
	a := []string{
		"-v",
		"error",
		"-hide_banner",
		"-probesize",
		strconv.Itoa(ProbeSize),
		"-analyzeduration",
		strconv.Itoa(ProbeAnalyze),
		"-protocol_whitelist",
		"file",
		"-format_whitelist",
		InputFormats,
	}
	return append(a, extra...)
}

func probeArgv(pol Policy, path string, extra ...string) ([]string, error) {
	a := append(probeBase(extra...), "-i", "file:"+toSlash(path))
	if err := Check(FFprobe, a, pol); err != nil {
		return nil, err
	}
	return a, nil
}

// ProbeJSON is the first run of SDD-011 3.1: container and streams as JSON.
func ProbeJSON(pol Policy, path string) ([]string, error) {
	return probeArgv(pol, path, "-print_format", "json", "-show_format", "-show_streams")
}

// ProbePacketFlags lists the pts_time and flags of the packets of one stream, for the keyframe
// checks of SDD-011 5.1 (a flags value starting with K is a keyframe).
func ProbePacketFlags(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-select_streams",
		strconv.Itoa(stream),
		"-show_entries",
		"packet=pts_time,flags",
		"-of",
		"csv=p=0",
	)
}

// ProbePacketTimes lists the timestamps of the packets of one stream, for the nominal frame rate.
func ProbePacketTimes(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-select_streams",
		strconv.Itoa(stream),
		"-show_entries",
		"packet=pts_time,dts_time",
		"-of",
		"csv=p=0",
	)
}

// ProbeFirstFrameSideData reads the display matrix of the first frame of an image.
func ProbeFirstFrameSideData(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(pol, path, "-select_streams", strconv.Itoa(stream), "-show_frames", "-read_intervals", "%+#1",
		"-show_entries", "frame=side_data_list", "-print_format", "json")
}

// ProbeCountPackets tells an animated still image from a single one.
func ProbeCountPackets(pol Policy, path string) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-count_packets",
		"-show_entries",
		"stream=nb_read_packets:format=format_name",
		"-of",
		"csv=p=0",
	)
}

// ProbeStreamTiming reads the rates and time base of the first video stream of a produced file.
func ProbeStreamTiming(pol Policy, path string) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-select_streams",
		"v:0",
		"-show_entries",
		"stream=r_frame_rate,avg_frame_rate,time_base",
		"-of",
		"json",
	)
}

// ProbeEncodedPackets counts the packets of an encoder test file (SDD-011 8.4).
func ProbeEncodedPackets(pol Policy, path string) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-count_packets",
		"-select_streams",
		"v:0",
		"-show_entries",
		"stream=codec_name,nb_read_packets",
		"-of",
		"csv=p=0",
	)
}

// DetectInput is what encoder detection records for the encoder it chose: the input
// arguments and filter its test encode needed (SDD-011 8.4, DS-13).
type DetectInput struct {
	VaapiDevice string
	Filter      string // the -vf value
}

// EncoderTest is the test encode of SDD-011 8.4 for one candidate: 15 frames of
// 1080x1920 at 30 fps to a file of the run directory.
func EncoderTest(pol Policy, encoder string, di DetectInput, out string) ([]string, error) {
	return EncoderTestAt(pol, encoder, di, out, 1080, 1920, Rate{30, 1})
}

// EncoderTestAt is EncoderTest at another size and rate, the capability test of an export at 4K or at
// 60 fps (SDD-004 section 8): 15 frames of the synthetic picture, so the bitrate is the one of the export.
func EncoderTestAt(pol Policy, encoder string, di DetectInput, out string, w, h int, rate Rate) ([]string, error) {
	in := TestSrc2(w, h, rate, 500)
	opts := []Opt{Enum("-c:v", encoder), Int("-frames:v", 15)}
	switch encoder {
	case "libx264":
		opts = append(opts, Int("-crf", 18), Enum("-preset", "medium"), Enum("-profile:v", "high"))
	case "h264_vaapi", "h264_mf", "libopenh264":
		opts = append(opts, Bitrate("-b:v", "8M"))
	default:
		opts = append(opts, Bitrate("-b:v", "8M"), Enum("-profile:v", "high"))
	}
	return Cmd{
		LogErrorOnly: true,
		VaapiDevice:  di.VaapiDevice,
		Inputs:       []Input{in},
		Outputs: []Output{
			{Format: "mp4", Path: out, Maps: []string{"0:v:0"}, NoAudio: true, VF: di.Filter, Opts: opts},
		},
	}.Argv(pol)
}

// SmokeEncoders are the encoders of the capability test of SDD-011 8.3, with the pixel
// format each accepts.
var SmokeEncoders = map[string]string{
	"mpeg4": "yuv420p", "libvpx-vp9": "yuv420p", "mjpeg": "yuvj420p", "png": "rgb24",
	"aac": "", "pcm_s16le": "",
}

// EncoderSmoke encodes 5 frames (or 0.5 s of silence for an audio encoder) to the null
// muxer with one required encoder.
func EncoderSmoke(pol Policy, encoder string) ([]string, error) {
	pix, ok := SmokeEncoders[encoder]
	if !ok {
		return nil, &ViolationError{Rule: "FF-15", Index: -1, Msg: "not an encoder of the capability test"}
	}
	if pix == "" {
		return Cmd{
			LogErrorOnly: true, Inputs: []Input{AnullSrc(48000, 500)},
			Outputs: []Output{{Format: "null", Maps: []string{"0:a:0"}, Opts: []Opt{Enum("-c:a", encoder)}}},
		}.Argv(pol)
	}
	return Cmd{
		LogErrorOnly: true,
		Inputs:       []Input{TestSrc2(320, 240, Rate{30, 1}, 500)},
		Outputs: []Output{
			{
				Format: "null",
				Maps:   []string{"0:v:0"},
				VF:     Chain1(Format(pix)),
				Opts:   []Opt{Enum("-c:v", encoder), Int("-frames:v", 5)},
			},
		},
	}.Argv(pol)
}

// HDRSmoke runs the tone-mapping chain of SDD-011 4.3.1 on a synthetic HLG source; it sets hdr_ok.
func HDRSmoke(pol Policy) ([]string, error) {
	f := append([]Filter{SetParamsHLG()}, ToneMapHDR("2020_ncl", "2020", "arib-std-b67", "tv")...)
	f = append(f, Format("yuv420p"))
	return Cmd{
		LogErrorOnly: true, Inputs: []Input{TestSrc2(320, 240, Rate{30, 1}, 500)},
		Outputs: []Output{{Format: "null", Maps: []string{"0:v:0"}, VF: Chain1(f...)}},
	}.Argv(pol)
}

// MaxAnimProbePackets is how many packets ProbeAnimPackets reads: one more than the frames an
// animated image may have, so a file with more is seen to be over the ceiling without reading all of it.
const MaxAnimProbePackets = 1001

// ProbeAnimPackets lists the start and the duration of the first MaxAnimProbePackets packets of an
// image stream: the frame delays of an animated image (SDD-011 3.4 row 3), read without decoding.
func ProbeAnimPackets(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(
		pol,
		path,
		"-select_streams",
		strconv.Itoa(stream),
		"-read_intervals",
		"%+#1001",
		"-show_entries",
		"packet=pts_time,duration_time",
		"-of",
		"csv=p=0",
	)
}
