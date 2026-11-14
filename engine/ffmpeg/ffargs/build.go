package ffargs

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Rate is a rational frame rate or sample rate.
type Rate struct{ Num, Den int }

func (r Rate) String() string { return fmt.Sprintf("%d/%d", r.Num, r.Den) }

// Filter is one rendered filter. Only the constructors below make one, from numbers and
// fixed names; the checker validates the result like any other graph.
type Filter string

func Scale(w, h int, flags string) Filter {
	return Filter(fmt.Sprintf("scale=%d:%d:flags=%s", w, h, flags))
}

// ScaleRange is the scale-only colour conversion of SDD-011 4.3.1 for a build with no zscale.
func ScaleRange(w, h int, flags, inRange, inMatrix string) Filter {
	return Filter(fmt.Sprintf("scale=%d:%d:flags=%s:in_range=%s:out_range=tv:in_color_matrix=%s:out_color_matrix=bt709", w, h, flags, inRange, inMatrix))
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
	return Filter(fmt.Sprintf("scale=%d:%d:flags=bicubic+accurate_rnd+full_chroma_int:in_range=tv:in_color_matrix=bt709", w, h))
}

func FPS(r Rate) Filter               { return Filter(fmt.Sprintf("fps=%s:round=near", r)) }
func Format(pixFmts ...string) Filter { return Filter("format=" + strings.Join(pixFmts, "|")) }
func SetPTS() Filter                  { return "setpts=PTS-STARTPTS" }
func ASetPTS() Filter                 { return "asetpts=PTS-STARTPTS" }
func SetSAR1() Filter                 { return "setsar=1" }
func Split(n int) Filter              { return Filter("split=" + strconv.Itoa(n)) }
func ASplit(n int) Filter             { return Filter("asplit=" + strconv.Itoa(n)) }
func Crop(w, h, x, y int) Filter      { return Filter(fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y)) }
func Bwdif() Filter                   { return "bwdif=mode=send_frame:parity=auto:deint=interlaced" }
func Yadif() Filter                   { return "yadif=mode=send_frame:parity=auto:deint=interlaced" }
func AResample(rate int) Filter       { return Filter("aresample=" + strconv.Itoa(rate)) }
func AFormatS16Stereo() Filter        { return "aformat=sample_fmts=s16:channel_layouts=stereo" }
func ADelay(samples int) Filter       { return Filter(fmt.Sprintf("adelay=%dS:all=1", samples)) }
func APad(wholeLen int) Filter        { return Filter("apad=whole_len=" + strconv.Itoa(wholeLen)) }
func ATrimEnd(endSample int) Filter   { return Filter("atrim=end_sample=" + strconv.Itoa(endSample)) }
func ATrimStart(startSample int) Filter {
	return Filter("atrim=start_sample=" + strconv.Itoa(startSample))
}
func Ebur128() Filter { return "ebur128=peak=true:framelog=quiet" }
func SilenceDetect(db, minMS int) Filter {
	return Filter(fmt.Sprintf("silencedetect=noise=-%ddB:d=%d.%03d", db, minMS/1000, minMS%1000))
}

// ZScale converts colour with explicit input properties (SDD-011 4.3.1). Each argument is an
// enumerated name that the checker validates.
func ZScale(matrixIn, primariesIn, transferIn, rangeIn string) Filter {
	return Filter(fmt.Sprintf("zscale=min=%s:pin=%s:tin=%s:rin=%s:m=709:p=709:t=709:r=limited:d=ordered", matrixIn, primariesIn, transferIn, rangeIn))
}

// ToneMapHDR is the HLG and PQ chain of SDD-011 4.3.1 (hable, npl=100, desat=0).
func ToneMapHDR(matrixIn, primariesIn, transferIn, rangeIn string) []Filter {
	return []Filter{
		Filter(fmt.Sprintf("zscale=min=%s:pin=%s:tin=%s:rin=%s:t=linear:npl=100", matrixIn, primariesIn, transferIn, rangeIn)),
		Format("gbrpf32le"),
		"zscale=p=709",
		"tonemap=tonemap=hable:desat=0",
		"zscale=m=709:p=709:t=709:r=limited:d=ordered",
	}
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
	for _, c := range g {
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
	path   string // absolute path, for a file input
	pipe   string // "rawvideo" or "s16le" for pipe:0
	lavfi  string // source for -f lavfi
	pixFmt string
	size   string
	rate   string
	ar, ac int
	seekMS int64
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

func seconds(ms int64) string {
	return fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
}

// Opt is one output option made by a typed constructor.
type Opt struct{ name, value string }

func Int(name string, v int) Opt        { return Opt{name, strconv.Itoa(v)} }
func Enum(name, v string) Opt           { return Opt{name, v} }
func Bitrate(name string, v string) Opt { return Opt{name, v} }

// Common option sets.
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
	FilterThreads int
	VaapiDevice   string
	Inputs        []Input
	Graph         Graph
	Outputs       []Output
}

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
	if c.FilterThreads > 0 {
		a = append(a, "-filter_threads", strconv.Itoa(c.FilterThreads))
	}
	if c.VaapiDevice != "" {
		a = append(a, "-vaapi_device", c.VaapiDevice)
	}
	for _, in := range c.Inputs {
		a = append(a, "-protocol_whitelist", "file,pipe")
		if in.path != "" {
			a = append(a, "-format_whitelist", InputFormats, "-sn", "-dn")
		}
		if c.Threads > 0 {
			a = append(a, "-threads", strconv.Itoa(c.Threads))
		}
		if in.seekMS > 0 {
			a = append(a, "-ss", seconds(in.seekMS))
		}
		switch {
		case in.lavfi != "":
			a = append(a, "-f", "lavfi", "-i", in.lavfi)
		case in.pipe == "rawvideo":
			a = append(a, "-f", "rawvideo", "-pix_fmt", in.pixFmt, "-s", in.size, "-framerate", in.rate, "-i", "pipe:0")
		case in.pipe == "s16le":
			a = append(a, "-f", "s16le", "-ar", strconv.Itoa(in.ar), "-ac", strconv.Itoa(in.ac), "-i", "pipe:0")
		default:
			a = append(a, "-i", "file:"+in.path)
		}
	}
	if len(c.Graph) > 0 {
		a = append(a, "-filter_complex", c.Graph.String())
	}
	for _, o := range c.Outputs {
		for _, m := range o.Maps {
			a = append(a, "-map", m)
		}
		if o.NoVideo {
			a = append(a, "-vn")
		}
		if o.NoAudio {
			a = append(a, "-an")
		}
		if o.NoSubs {
			a = append(a, "-sn")
		}
		if o.NoData {
			a = append(a, "-dn")
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
			a = append(a, "-")
		case o.Pipe:
			a = append(a, "pipe:1")
		default:
			a = append(a, "file:"+filepath.ToSlash(o.Path))
		}
	}
	if err := Check(FFmpeg, a, pol); err != nil {
		return nil, err
	}
	return a, nil
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
	a := []string{"-v", "error", "-hide_banner", "-probesize", strconv.Itoa(ProbeSize), "-analyzeduration", strconv.Itoa(ProbeAnalyze),
		"-protocol_whitelist", "file", "-format_whitelist", InputFormats}
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

// ProbePacketTimes lists the timestamps of the packets of one stream, for the nominal frame rate.
func ProbePacketTimes(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(pol, path, "-select_streams", strconv.Itoa(stream), "-show_entries", "packet=pts_time,dts_time", "-of", "csv=p=0")
}

// ProbeFirstFrameSideData reads the display matrix of the first frame of an image.
func ProbeFirstFrameSideData(pol Policy, path string, stream int) ([]string, error) {
	return probeArgv(pol, path, "-select_streams", strconv.Itoa(stream), "-show_frames", "-read_intervals", "%+#1",
		"-show_entries", "frame=side_data_list", "-print_format", "json")
}

// ProbeCountPackets tells an animated still image from a single one.
func ProbeCountPackets(pol Policy, path string) ([]string, error) {
	return probeArgv(pol, path, "-count_packets", "-show_entries", "stream=nb_read_packets:format=format_name", "-of", "csv=p=0")
}

// ProbeStreamTiming reads the rates and time base of the first video stream of a produced file.
func ProbeStreamTiming(pol Policy, path string) ([]string, error) {
	return probeArgv(pol, path, "-select_streams", "v:0", "-show_entries", "stream=r_frame_rate,avg_frame_rate,time_base", "-of", "json")
}

// ProbeEncodedPackets counts the packets of an encoder test file (SDD-011 8.4).
func ProbeEncodedPackets(pol Policy, path string) ([]string, error) {
	return probeArgv(pol, path, "-count_packets", "-select_streams", "v:0", "-show_entries", "stream=codec_name,nb_read_packets", "-of", "csv=p=0")
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
	in := TestSrc2(1080, 1920, Rate{30, 1}, 500)
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
		LogErrorOnly: true, VaapiDevice: di.VaapiDevice,
		Inputs:  []Input{in},
		Outputs: []Output{{Format: "mp4", Path: out, Maps: []string{"0:v:0"}, NoAudio: true, VF: di.Filter, Opts: opts}},
	}.Argv(pol)
}
