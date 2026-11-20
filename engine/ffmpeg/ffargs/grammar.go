package ffargs

import (
	"regexp"
	"slices"
	"strconv"
)

// kind validates the value of an option. A kind never looks at the filesystem.
type kind func(string) bool

func intRange(lo, hi int) kind {
	return func(s string) bool {
		if s == "" || (len(s) > 1 && s[0] == '0') || len(s) > 9 {
			return false
		}
		n, err := strconv.Atoi(s)
		return err == nil && n >= lo && n <= hi
	}
}

func enum(vals ...string) kind {
	return func(s string) bool {
		return slices.Contains(vals, s)
	}
}

func match(re string) kind {
	r := regexp.MustCompile(re)
	return func(s string) bool { return r.MatchString(s) }
}

var (
	kNumber   = match(`^[0-9]{1,6}(\.[0-9]{1,6})?$`)
	kRational = match(`^[1-9][0-9]{0,6}/[1-9][0-9]{0,6}$`)
	kSize     = match(`^[1-9][0-9]{0,4}x[1-9][0-9]{0,4}$`)
	kBitrate  = match(`^(0|[1-9][0-9]{0,5}[kKM]?)$`)
	kPixFmt   = enum("rgba", "rgb24", "yuv420p", "yuvj420p", "yuva444p10le", "yuva420p", "nv12", "gbrpf32le", "gray")
	kColour   = enum("bt709", "smpte170m", "tv", "pc", "limited", "full")
	kMap      = match(`^([0-9]{1,2}:[0-9]{1,3}|[0-9]{1,2}:(v|a)(:[0-9]{1,2})?|\[[A-Za-z0-9_]{1,16}\])$`)
	kProgress = enum("pipe:1")
	kLogLevel = enum("error", "warning", "info")
	kVaapiDev = match(`^/dev/dri/renderD[0-9]{1,3}$`)
	kFFlags   = enum("+bitexact")
	kMovflags = enum("+faststart")
	kReadInt  = enum("%+#1", "%+#1001")
)

// Codecs of the fixed table of ADR-0011 item 5 and SDD-011 section 4.2.
var videoCodecs = []string{
	"mpeg4", "prores_ks", "libvpx-vp9", "mjpeg", "png", "libwebp", "copy",
	"libopenh264", "libx264", "h264_nvenc", "h264_qsv", "h264_amf", "h264_videotoolbox", "h264_mf", "h264_vaapi",
	// HEVC and AV1 (RF-089, D-067): the vendor encoders and the permissively licensed software ones.
	// libx265 and every other GPL encoder are absent on purpose; TestEncoderAllowList keeps them out.
	"hevc_videotoolbox", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_mf", "hevc_vaapi",
	"av1_nvenc", "av1_qsv", "av1_amf", "av1_vaapi", "libsvtav1", "libaom-av1",
}

var audioCodecs = []string{"pcm_s16le", "aac", "libopus", "copy"}

// OutputFormats is FF-13, plus mov (normalized video, SDD-011 4.3) and null (analysis
// and detection runs write nothing).
var outputFormats = []string{"mp4", "mov", "webm", "wav", "image2", "rawvideo", "s16le", "null"}

type optClass int

const (
	classGlobal optClass = iota
	classInput
	classOutput
	classEither // meaning depends on position (before -i: input; after: output)
)

type optSpec struct {
	class optClass
	flag  bool // takes no value
	value kind
}

// ffmpegOptions is the whole option grammar of ffmpeg. An option that is not here is refused.
var ffmpegOptions = map[string]optSpec{
	// global
	"-n":                      {class: classGlobal, flag: true},
	"-nostdin":                {class: classGlobal, flag: true},
	"-hide_banner":            {class: classGlobal, flag: true},
	"-nostats":                {class: classGlobal, flag: true},
	"-max_alloc":              {class: classGlobal, value: enum(strconv.Itoa(MaxAlloc))},
	"-v":                      {class: classGlobal, value: kLogLevel},
	"-progress":               {class: classGlobal, value: kProgress},
	"-cpuflags":               {class: classGlobal, value: enum(CPUMask)},
	"-filter_threads":         {class: classGlobal, value: intRange(1, 1024)},
	"-filter_complex_threads": {class: classGlobal, value: intRange(1, 1024)},
	"-vaapi_device":           {class: classGlobal, value: kVaapiDev},
	"-filter_complex":         {class: classGlobal, value: nil}, // graph, checked apart

	// input only
	"-protocol_whitelist": {class: classInput, value: enum("file,pipe")},
	"-format_whitelist":   {class: classInput, value: enum(InputFormats)},
	"-pattern_type":       {class: classInput, value: enum("none")},
	"-framerate":          {class: classInput, value: kRational},
	"-s":                  {class: classInput, value: kSize},

	// both
	"-ss":      {class: classEither, value: kNumber},
	"-t":       {class: classEither, value: kNumber},
	"-f":       {class: classEither, value: enum(append([]string{"lavfi", "rawvideo", "s16le"}, outputFormats...)...)},
	"-pix_fmt": {class: classEither, value: kPixFmt},
	"-ar":      {class: classEither, value: intRange(8000, 192000)},
	"-ac":      {class: classEither, value: intRange(1, 8)},
	"-threads": {class: classEither, value: intRange(1, 1024)},
	"-sn":      {class: classEither, flag: true},
	"-dn":      {class: classEither, flag: true},

	// output only
	"-map":                   {class: classOutput, value: kMap},
	"-map_metadata":          {class: classOutput, value: enum("-1")},
	"-map_chapters":          {class: classOutput, value: enum("-1")},
	"-metadata:s:v:0":        {class: classOutput, value: enum("alpha_mode=1")},
	"-an":                    {class: classOutput, flag: true},
	"-vn":                    {class: classOutput, flag: true},
	"-c:v":                   {class: classOutput, value: enum(videoCodecs...)},
	"-c:a":                   {class: classOutput, value: enum(audioCodecs...)},
	"-q:v":                   {class: classOutput, value: intRange(1, 31)},
	"-b:v":                   {class: classOutput, value: kBitrate},
	"-b:a":                   {class: classOutput, value: kBitrate},
	"-crf":                   {class: classOutput, value: intRange(0, 63)},
	"-quality":               {class: classOutput, value: intRange(1, 100)}, // libwebp, lossy
	"-lossless":              {class: classOutput, value: enum("1")},        // libwebp
	"-g":                     {class: classOutput, value: intRange(1, 1200)},
	"-keyint_min":            {class: classOutput, value: intRange(1, 1200)},
	"-bf":                    {class: classOutput, value: intRange(0, 16)},
	"-dct":                   {class: classOutput, value: enum("int")},
	"-profile:v":             {class: classOutput, value: enum("high", "main", "baseline", "4444")},
	"-preset":                {class: classOutput, value: enum("medium", "8")}, // 8: the speed of libsvtav1
	"-tag:v":                 {class: classOutput, value: enum("hvc1")},
	"-deadline":              {class: classOutput, value: enum("realtime", "good")},
	"-cpu-used":              {class: classOutput, value: intRange(0, 16)},
	"-row-mt":                {class: classOutput, value: enum("0", "1")},
	"-tile-columns":          {class: classOutput, value: intRange(0, 6)},
	"-lag-in-frames":         {class: classOutput, value: intRange(0, 25)},
	"-auto-alt-ref":          {class: classOutput, value: enum("0")},
	"-colorspace":            {class: classOutput, value: kColour},
	"-color_primaries":       {class: classOutput, value: kColour},
	"-color_trc":             {class: classOutput, value: kColour},
	"-color_range":           {class: classOutput, value: kColour},
	"-fps_mode":              {class: classOutput, value: enum("cfr", "vfr")},
	"-video_track_timescale": {class: classOutput, value: intRange(1, 10_000_000)},
	"-frames:v":              {class: classOutput, value: intRange(1, 10_000_000)},
	"-frames:a":              {class: classOutput, value: intRange(1, 10_000_000)},
	"-start_number":          {class: classOutput, value: intRange(0, 99999)},
	"-update":                {class: classOutput, value: enum("1")},
	"-fflags":                {class: classOutput, value: kFFlags},
	"-flags":                 {class: classOutput, value: kFFlags},
	"-movflags":              {class: classOutput, value: kMovflags},
	"-rf64":                  {class: classOutput, value: enum("auto")},
	"-shortest":              {class: classOutput, flag: true},
	"-vf":                    {class: classOutput, value: nil}, // graph, checked apart
	"-af":                    {class: classOutput, value: nil},
}

// infoFlags are the capability listings: ffmpeg -hide_banner -<flag>.
var infoFlags = []string{"-version", "-encoders", "-decoders", "-filters", "-muxers", "-demuxers"}
