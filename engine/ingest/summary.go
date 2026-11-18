package ingest

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

// Kind is the kind of an asset, found by probing and never by the file name.
type Kind string

// The kinds of asset.
const (
	KindVideo Kind = "video"
	KindAudio Kind = "audio"
	KindImage Kind = "image"
)

// Doc is the part of ffprobe -show_format -show_streams that ingest reads.
type Doc struct {
	Streams []Stream `json:"streams"`
	Format  struct {
		Name      string            `json:"format_name"`
		Duration  string            `json:"duration"`
		Size      string            `json:"size"`
		NbStreams int               `json:"nb_streams"`
		Tags      map[string]string `json:"tags"`
	} `json:"format"`
}

// Stream is one ffprobe stream.
type Stream struct {
	Index          int    `json:"index"`
	CodecType      string `json:"codec_type"`
	CodecName      string `json:"codec_name"`
	Profile        string `json:"profile"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	CodedWidth     int    `json:"coded_width"`
	CodedHeight    int    `json:"coded_height"`
	SAR            string `json:"sample_aspect_ratio"`
	PixFmt         string `json:"pix_fmt"`
	BitsPerRaw     string `json:"bits_per_raw_sample"`
	FieldOrder     string `json:"field_order"`
	ColorRange     string `json:"color_range"`
	ColorSpace     string `json:"color_space"`
	ColorTransfer  string `json:"color_transfer"`
	ColorPrimaries string `json:"color_primaries"`
	RFrameRate     string `json:"r_frame_rate"`
	AvgFrameRate   string `json:"avg_frame_rate"`
	StartTime      string `json:"start_time"`
	TimeBase       string `json:"time_base"`
	DurationTS     int64  `json:"duration_ts"`
	NbFrames       string `json:"nb_frames"`
	Channels       int    `json:"channels"`
	SampleRate     string `json:"sample_rate"`
	Disposition    struct {
		AttachedPic int `json:"attached_pic"`
		Default     int `json:"default"`
	} `json:"disposition"`
	SideData []struct {
		Type          string `json:"side_data_type"`
		DisplayMatrix string `json:"displaymatrix"`
		DVProfile     int    `json:"dv_profile"`
	} `json:"side_data_list"`
	Tags map[string]string `json:"tags"`
}

// ParseDoc reads the JSON of the probe.
func ParseDoc(raw []byte) (*Doc, error) {
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, unsupportedErr("unrecognized_format", "The file is not a media file FFmpeg can read.", nil)
	}
	return &d, nil
}

func (s *Stream) attached() bool { return s.Disposition.AttachedPic == 1 }

// imageFormats are the container names of still images (SDD-011 3.4 row 2).
var imageFormats = []string{
	"image2",
	"png_pipe",
	"jpeg_pipe",
	"webp_pipe",
	"bmp_pipe",
	"tiff_pipe",
	"jpegxl_pipe",
	"gif",
	"apng",
	"webp_anim",
}

// IsImageContainer reports whether the container is one of the still-image formats.
func (d *Doc) IsImageContainer() bool {
	for f := range strings.SplitSeq(d.Format.Name, ",") {
		if slices.Contains(imageFormats, f) {
			return true
		}
	}
	return false
}

// Selection is the choice of streams (SDD-011 3.4).
type Selection struct {
	Kind  Kind
	Video *Stream // the stream used: the one with the most pixels, ties to the lowest index
	Audio *Stream // default disposition, else the lowest index, among streams with sound
}

// Select applies rows 2, 4 and 5 of the classification table and the audio choice (DS-15).
// It does not apply the limits; Summarize does.
func (d *Doc) Select() (Selection, error) {
	var sel Selection
	var videos []*Stream
	var audios []*Stream
	for i := range d.Streams {
		s := &d.Streams[i]
		switch {
		case s.CodecType == "video" && !s.attached():
			videos = append(videos, s)
		case s.CodecType == "audio" && s.Channels > 0 && atoi(s.SampleRate) > 0:
			audios = append(audios, s)
		}
	}
	for _, s := range audios {
		if sel.Audio == nil || (s.Disposition.Default == 1 && sel.Audio.Disposition.Default != 1) {
			sel.Audio = s
		}
	}
	for _, s := range videos {
		if sel.Video == nil || s.CodedWidth*s.CodedHeight > sel.Video.CodedWidth*sel.Video.CodedHeight {
			sel.Video = s
		}
	}
	switch {
	case d.IsImageContainer() && len(videos) == 1:
		sel.Kind = KindImage
	case sel.Video != nil:
		sel.Kind = KindVideo
	case sel.Audio != nil:
		sel.Kind = KindAudio
	default:
		return sel, unsupportedErr("no_usable_stream", "The file has no video or audio stream FFmpeg can use.",
			map[string]any{"streams": ignoredKinds(d)})
	}
	return sel, nil
}

func ignoredKinds(d *Doc) []string {
	var out []string
	for i := range d.Streams {
		s := &d.Streams[i]
		k := s.CodecType
		if s.attached() {
			k = "attached_picture"
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Matrix is the upper-left 2 by 2 of the display matrix, each entry in {-1, 0, 1}.
type Matrix [2][2]int

// ParseDisplayMatrix reads the text ffprobe prints for a display matrix: three rows of three
// fixed-point numbers. It returns the identity when the text has no matrix.
func ParseDisplayMatrix(text string) Matrix {
	id := Matrix{{1, 0}, {0, 1}}
	var rows [][]float64
	for line := range strings.SplitSeq(text, "\n") {
		_, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var row []float64
		for f := range strings.FieldsSeq(rest) {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return id
			}
			row = append(row, v)
		}
		if len(row) == 3 {
			rows = append(rows, row)
		}
	}
	if len(rows) != 3 {
		return id
	}
	var m Matrix
	for r := range 2 {
		for c := range 2 {
			m[r][c] = int(math.Round(rows[r][c] / 65536))
		}
	}
	return m
}

// SwapDimensions is a == 0 and d == 0: the matrix turns the picture a quarter turn.
func (m Matrix) SwapDimensions() bool { return m[0][0] == 0 && m[1][1] == 0 }

// Mirrored is a*d - b*c < 0.
func (m Matrix) Mirrored() bool { return m[0][0]*m[1][1]-m[0][1]*m[1][0] < 0 }

// VideoFacts is the video block of the probe summary (SDD-011 3.2).
type VideoFacts struct {
	StreamIndex   int      `json:"stream_index"`
	Codec         string   `json:"codec"`
	Profile       string   `json:"profile"`
	CodedWidth    int      `json:"coded_width"`
	CodedHeight   int      `json:"coded_height"`
	Matrix        Matrix   `json:"matrix"`
	SwapDims      bool     `json:"swap_dimensions"`
	Mirrored      bool     `json:"mirrored"`
	SAR           Rational `json:"sar"`
	DisplayWidth  int      `json:"display_width"`
	DisplayHeight int      `json:"display_height"`
	PixFmt        string   `json:"pix_fmt"`
	BitDepth      int      `json:"bit_depth"`
	FieldOrder    string   `json:"field_order"`
	Interlaced    bool     `json:"interlaced"`
	ColorRange    string   `json:"color_range"`
	ColorSpace    string   `json:"color_space"`
	ColorTransfer string   `json:"color_transfer"`
	ColorPrim     string   `json:"color_primaries"`
	HDR           string   `json:"hdr"` // "none", "hlg" or "pq"
	DolbyProfile  *int     `json:"dolby_vision_profile"`
	RFrameRate    string   `json:"r_frame_rate"`
	AvgFrameRate  string   `json:"avg_frame_rate"`
	MeasuredFPS   float64  `json:"measured_fps"`
	NominalFPS    Rational `json:"nominal_fps"`
	VFR           bool     `json:"vfr"`
	PacketCount   int      `json:"packet_count"`
	StartTimeMS   int64    `json:"start_time_ms"`
}

// AudioFacts is the audio block of the probe summary.
type AudioFacts struct {
	StreamIndex   int    `json:"stream_index"`
	Codec         string `json:"codec"`
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	StartTimeMS   int64  `json:"start_time_ms"`
	OffsetSamples int    `json:"offset_samples"`
}

// Ignored records a stream that is not used and why.
type Ignored struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	Reason    string `json:"reason"`
}

// Tag is an untrusted text value, cut and cleaned (SDD-011 3.2, 10.7).
type Tag struct {
	Value     string `json:"value"`
	Truncated bool   `json:"truncated"`
}

// Summary is the probe summary stored as probe-1.json (SDD-011 3.2).
type Summary struct {
	Version        int            `json:"version"`
	Kind           Kind           `json:"kind"`
	Container      string         `json:"container"`
	SizeBytes      int64          `json:"size_bytes"`
	DurationMS     int64          `json:"duration_ms"`
	Video          *VideoFacts    `json:"video,omitempty"`
	Audio          *AudioFacts    `json:"audio,omitempty"`
	IgnoredStreams []Ignored      `json:"ignored_streams"`
	Warnings       []string       `json:"warnings"`
	Tags           map[string]Tag `json:"tags,omitempty"`
	// FrameDelays is the display time of each frame of an animated image, in milliseconds (kind image only).
	FrameDelays []int `json:"frame_delays,omitempty"`
}

// Extras are the answers of the further ffprobe runs of SDD-011 3.1.
type Extras struct {
	// PacketTimes are the pts_time and dts_time values of the video packets of the selected stream, in file order.
	PacketTimes []float64
	// FrameSideData is the JSON of the first frame's side data, for images.
	FrameSideData []byte
	// NbReadPackets is the packet count of a still-image container (3.4 rows 2 and 3), at most
	// ffargs.MaxAnimProbePackets.
	NbReadPackets int
	// FrameDelays is the display time in milliseconds of each packet of a still-image container.
	FrameDelays []int
}

// tagNames is the closed set of kept tag keys (SDD-011 3.2): a hostile file cannot choose which keys reach the agent.
var tagNames = []string{
	"title", "artist", "comment", "description", "creation_time", "location", "encoder", "handler_name",
}

// cleanTag removes control characters and cuts to 200 characters.
func cleanTag(s string) Tag {
	var b strings.Builder
	n := 0
	truncated := false
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		if n == 200 {
			truncated = true
			break
		}
		b.WriteRune(r)
		n++
	}
	return Tag{Value: b.String(), Truncated: truncated}
}

func unsupportedErr(reason, msg string, details map[string]any) *ffmpeg.Error {
	return &ffmpeg.Error{Code: ffmpeg.CodeUnsupportedMedia, Reason: reason, Message: msg, Details: details}
}

// ceil of x to 3 decimals keeps millisecond values stable.
func ms(seconds float64) int64 { return int64(math.Round(seconds * 1000)) }

// Summarize builds the probe summary and applies the classification table of SDD-011 3.4,
// returning the typed error of the first row that matches. caps may be nil, which skips the
// capability rows.
func Summarize(d *Doc, sel Selection, ex Extras, caps *ffmpeg.Capabilities, size int64) (*Summary, error) {
	s := &Summary{
		Version: 1, Kind: sel.Kind, Container: d.Format.Name, SizeBytes: size,
		DurationMS: ms(atof(d.Format.Duration)), IgnoredStreams: []Ignored{}, Warnings: []string{},
	}
	if sel.Kind == KindImage {
		return summarizeImage(sel, ex, caps, s)
	}
	if len(d.Streams) > ffmpeg.MaxStreams {
		return nil, unsupportedErr(
			"too_many_streams",
			"The file has more streams than the product reads.",
			map[string]any{"streams": len(d.Streams)},
		)
	}
	if sel.Video != nil {
		v, err := videoFacts(sel.Video, ex, caps)
		if err != nil {
			return nil, err
		}
		s.Video = v
		s.Warnings = append(s.Warnings, videoWarnings(sel.Video, ex)...)
		if v.PacketCount == 0 {
			return nil, unsupportedErr("no_frames", "The video has no frames.", nil)
		}
	}
	if sel.Audio != nil {
		a := &AudioFacts{
			StreamIndex: sel.Audio.Index, Codec: sel.Audio.CodecName, SampleRate: atoi(sel.Audio.SampleRate),
			Channels: sel.Audio.Channels, StartTimeMS: ms(atof(sel.Audio.StartTime)),
		}
		if caps != nil && !caps.Decoders[sel.Audio.CodecName] {
			return nil, decoderMissing(sel.Audio.CodecName)
		}
		if s.Video != nil {
			a.OffsetSamples = int(math.Round((atof(sel.Audio.StartTime) - atof(sel.Video.StartTime)) * SampleRate))
		}
		s.Audio = a
	}
	if sel.Kind == KindAudio && s.DurationMS == 0 {
		return nil, unsupportedErr("empty_audio", "The audio has no samples.", nil)
	}
	for i := range d.Streams {
		st := &d.Streams[i]
		if st == sel.Video || st == sel.Audio {
			continue
		}
		s.IgnoredStreams = append(s.IgnoredStreams, ignoredStream(st, sel))
	}
	collectTags(d, s)
	return s, nil
}

func ignoredStream(st *Stream, sel Selection) Ignored {
	reason := st.CodecType
	switch {
	case st.attached():
		reason = "attached_picture"
	case st.CodecType == "video":
		reason = "extra_video"
	case st.CodecType == "audio" && sel.Audio != nil:
		reason = "extra_audio"
	}
	return Ignored{Index: st.Index, CodecType: st.CodecType, Reason: reason}
}

func decoderMissing(codec string) *ffmpeg.Error {
	return unsupportedErr(
		"decoder_missing",
		"The FFmpeg in use has no decoder for the "+codec+" codec.",
		map[string]any{"codec": codec},
	)
}

func collectTags(d *Doc, s *Summary) {
	// The container first, then the streams in index order; the first value of a key wins.
	sources := make([]map[string]string, 0, len(d.Streams)+1)
	sources = append(sources, d.Format.Tags)
	for i := range d.Streams {
		sources = append(sources, d.Streams[i].Tags)
	}
	for _, name := range tagNames {
		for _, tags := range sources {
			v := tagValue(tags, name)
			if v == "" {
				continue
			}
			if s.Tags == nil {
				s.Tags = map[string]Tag{}
			}
			s.Tags[name] = cleanTag(v)
			break
		}
	}
}

// tagValue finds a key ignoring case (Matroska writes upper case) and the smallest spelling wins, for a stable result.
func tagValue(tags map[string]string, name string) string {
	var best, val string
	for k, v := range tags {
		if v != "" && strings.EqualFold(k, name) && (best == "" || k < best) {
			best, val = k, v
		}
	}
	return val
}

func videoFacts(st *Stream, ex Extras, caps *ffmpeg.Capabilities) (*VideoFacts, error) {
	v := &VideoFacts{
		StreamIndex: st.Index, Codec: st.CodecName, Profile: st.Profile,
		CodedWidth: st.CodedWidth, CodedHeight: st.CodedHeight, PixFmt: st.PixFmt,
		FieldOrder: st.FieldOrder, ColorRange: st.ColorRange, ColorSpace: st.ColorSpace,
		ColorTransfer: st.ColorTransfer, ColorPrim: st.ColorPrimaries,
		RFrameRate: st.RFrameRate, AvgFrameRate: st.AvgFrameRate, StartTimeMS: ms(atof(st.StartTime)),
		Matrix: Matrix{{1, 0}, {0, 1}}, SAR: Rational{1, 1}, HDR: "none", PacketCount: len(ex.PacketTimes),
	}
	if v.CodedWidth == 0 {
		v.CodedWidth, v.CodedHeight = st.Width, st.Height
	}
	// A VP9 stream in WebM keeps its alpha plane apart and says so with a stream tag; ffprobe still
	// reports yuv420p. The summary says yuva420p so the plan keeps the alpha (VV-013).
	if st.CodecName == "vp9" && tagValue(st.Tags, "alpha_mode") == "1" && !pixFmtHasAlpha(v.PixFmt) {
		v.PixFmt = "yuva420p"
	}
	for _, sd := range st.SideData {
		switch sd.Type {
		case "Display Matrix":
			v.Matrix = ParseDisplayMatrix(sd.DisplayMatrix)
		case "DOVI configuration record":
			p := sd.DVProfile
			v.DolbyProfile = &p
		}
	}
	v.SwapDims, v.Mirrored = v.Matrix.SwapDimensions(), v.Matrix.Mirrored()
	if sar, ok := ParseRational(strings.Replace(st.SAR, ":", "/", 1)); ok {
		v.SAR = sar
	}
	v.DisplayWidth, v.DisplayHeight = displaySize(v.CodedWidth, v.CodedHeight, v.SAR, v.SwapDims)
	v.BitDepth = bitDepth(st)
	v.Interlaced = slices.Contains([]string{"tt", "bb", "tb", "bt"}, st.FieldOrder)
	switch st.ColorTransfer {
	case "arib-std-b67":
		v.HDR = "hlg"
	case "smpte2084":
		v.HDR = "pq"
	}

	if v.DolbyProfile != nil && *v.DolbyProfile == 5 {
		return nil, unsupportedErr(
			"dolby_vision_profile_5",
			"The video is Dolby Vision profile 5, which has no standard HDR or SDR base layer.",
			nil,
		)
	}
	if v.HDR != "none" && caps != nil && (!caps.Zscale || !caps.Tonemap) {
		return nil, unsupportedErr(
			"hdr_needs_zscale",
			"The video is HDR and the FFmpeg in use cannot tone-map it (no zscale filter).",
			nil,
		)
	}
	if caps != nil && !caps.Decoders[st.CodecName] {
		return nil, decoderMissing(st.CodecName)
	}

	r, _ := ParseRational(st.RFrameRate)
	a, _ := ParseRational(st.AvgFrameRate)
	measured, outliers, _ := MeasureRate(ex.PacketTimes, r, a)
	if measured > MaxFrameRateRefused {
		return nil, unsupportedErr(
			"frame_rate_too_high",
			"The video runs at more frames per second than the product reads.",
			map[string]any{"fps": math.Round(measured*100) / 100},
		)
	}
	v.MeasuredFPS = math.Round(measured*100000) / 100000
	v.NominalFPS, _ = NominalRate(ex.PacketTimes, r, a)
	v.VFR = IsVFR(r, a, outliers)
	return v, nil
}

func videoWarnings(st *Stream, ex Extras) []string {
	r, _ := ParseRational(st.RFrameRate)
	a, _ := ParseRational(st.AvgFrameRate)
	_, w := NominalRate(ex.PacketTimes, r, a)
	return w
}

func bitDepth(st *Stream) int {
	if n := atoi(st.BitsPerRaw); n > 0 {
		return n
	}
	switch {
	case strings.Contains(st.PixFmt, "10"):
		return 10
	case strings.Contains(st.PixFmt, "12"):
		return 12
	case strings.Contains(st.PixFmt, "16"):
		return 16
	}
	return 8
}

// displaySize makes pixels square (from the wider ratio side) and then swaps for a quarter turn.
func displaySize(w, h int, sar Rational, swap bool) (int, int) {
	switch {
	case sar.Num > sar.Den:
		w = int(math.Round(float64(w) * float64(sar.Num) / float64(sar.Den)))
	case sar.Num < sar.Den:
		h = int(math.Round(float64(h) * float64(sar.Den) / float64(sar.Num)))
	}
	if swap {
		return h, w
	}
	return w, h
}

func summarizeImage(sel Selection, ex Extras, caps *ffmpeg.Capabilities, s *Summary) (*Summary, error) {
	st := sel.Video
	animated := ex.NbReadPackets > 1
	if animated {
		if err := ffmpeg.CheckAnimation(ex.NbReadPackets, st.Width, st.Height); err != nil {
			return nil, err
		}
	}
	if caps != nil && !caps.Decoders[st.CodecName] {
		return nil, decoderMissing(st.CodecName)
	}
	v := &VideoFacts{
		StreamIndex:   st.Index,
		Codec:         st.CodecName,
		CodedWidth:    st.Width,
		CodedHeight:   st.Height,
		PixFmt:        st.PixFmt,
		Matrix:        Matrix{{1, 0}, {0, 1}},
		SAR:           Rational{1, 1},
		HDR:           "none",
		BitDepth:      bitDepth(st),
		ColorRange:    st.ColorRange,
		ColorSpace:    st.ColorSpace,
		ColorTransfer: st.ColorTransfer,
		ColorPrim:     st.ColorPrimaries,
	}
	// The display matrix of an image is frame side data (3.1).
	var fr struct {
		Frames []struct {
			SideData []struct {
				Type          string `json:"side_data_type"`
				DisplayMatrix string `json:"displaymatrix"`
			} `json:"side_data_list"`
		} `json:"frames"`
	}
	if len(ex.FrameSideData) > 0 && json.Unmarshal(ex.FrameSideData, &fr) == nil && len(fr.Frames) > 0 {
		for _, sd := range fr.Frames[0].SideData {
			if sd.DisplayMatrix != "" {
				v.Matrix = ParseDisplayMatrix(sd.DisplayMatrix)
			}
		}
	}
	v.SwapDims, v.Mirrored = v.Matrix.SwapDimensions(), v.Matrix.Mirrored()
	v.DisplayWidth, v.DisplayHeight = v.CodedWidth, v.CodedHeight
	if v.SwapDims {
		v.DisplayWidth, v.DisplayHeight = v.CodedHeight, v.CodedWidth
	}
	v.PacketCount = 1
	if animated {
		v.PacketCount = ex.NbReadPackets
		s.FrameDelays = ex.FrameDelays
	}
	s.Video = v
	return s, nil
}
