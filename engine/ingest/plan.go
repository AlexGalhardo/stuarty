package ingest

import (
	"math"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

// Even is max(2, 2 * round(x / 2)): the even size of SDD-011 4.1.
func Even(x float64) int { return max(2, 2*int(math.Round(x/2))) }

// Colour says how the colour block of SDD-011 4.3.1 converts the picture.
type Colour int

// The conversions of the colour block: ColourNone when the stream already is BT.709 limited
// 8-bit 4:2:0, ColourZScale for SDR converted with zscale and explicit input properties,
// ColourToneMap for HLG or PQ tone mapped in linear light, and ColourScale for SDR converted by
// scale alone on a build without zscale.
const (
	ColourNone Colour = iota
	ColourZScale
	ColourToneMap
	ColourScale
)

// ColourProps are the input properties given to zscale, as zscale names them.
type ColourProps struct {
	Matrix, Primaries, Transfer, Range string
}

// ThumbPlan is the layout of SDD-011 4.5.
type ThumbPlan struct {
	TileW, TileH int
	Letterbox    bool
	IntervalS    int
	// Planned is the most tiles the video pass may write: ceil(duration / interval) + 1, at most ThumbMaxCount.
	Planned int
}

// VideoPlan is the plan of the video pass (SDD-011 4.1 and 4.3). It is stored in the job
// result so a failed ingest can be reproduced.
type VideoPlan struct {
	StreamIndex  int
	W, H         int // normalized size
	PW, PH       int // proxy size
	FPS          Rational
	GOP          int
	Timescale    int
	Interlaced   bool
	Deinterlacer string // "bwdif" or "yadif"
	Scale        bool   // piece 4: the size differs from the rotated coded size
	Colour       Colour
	Props        ColourProps
	Matrix601    bool // scale-only conversion from BT.601
	InRangePC    bool
	// Alpha keeps the alpha channel of the source: the normalized video is ProRes 4444 with
	// yuva444p10le instead of mpeg4 with yuv420p (SDD-011 4.3.5).
	Alpha  bool
	Thumbs ThumbPlan
	// Decoder forces the decoder of the source: libvpx-vp9 for a VP9 stream with alpha, because the
	// native decoder drops the alpha plane. Empty means the default.
	Decoder string
}

// GOPFor is ceil(num / (2 * den)): a keyframe every half second.
func GOPFor(r Rational) int {
	return int((r.Num + 2*r.Den - 1) / (2 * r.Den))
}

// TimescaleFor makes every frame duration an exact integer number of ticks.
func TimescaleFor(r Rational) int {
	if r.Den == 1001 {
		return int(r.Num)
	}
	return int(r.Num * 1000)
}

// NormalizedSize is W, H: display size capped to MaxEdge, never upscaled, even.
func NormalizedSize(dw, dh int) (int, int) {
	f := math.Min(1, float64(MaxEdge)/float64(max(dw, dh)))
	return Even(float64(dw) * f), Even(float64(dh) * f)
}

// ProxySize is PW, PH: the short edge capped to ProxyShortEdge, else the normalized size.
func ProxySize(w, h int) (int, int) {
	short := min(w, h)
	if short <= ProxyShortEdge {
		return w, h
	}
	g := float64(ProxyShortEdge) / float64(short)
	return Even(float64(w) * g), Even(float64(h) * g)
}

// PlanThumbnails is the layout of SDD-011 4.5 for a clip of durationMS and a normalized size.
func PlanThumbnails(durationMS int64, w, h int) ThumbPlan {
	tw := Even(math.Round(float64(w) * ThumbHeight / float64(h)))
	letterbox := false
	if tw > ThumbMaxTileWidth {
		tw, letterbox = ThumbMaxTileWidth, true
	}
	secs := math.Ceil(float64(durationMS) / 1000 / ThumbMaxCount)
	interval := max(1, int(secs))
	planned := min(ThumbMaxCount, int(math.Ceil(float64(durationMS)/1000/float64(interval)))+1)
	return ThumbPlan{TileW: tw, TileH: ThumbHeight, Letterbox: letterbox, IntervalS: interval, Planned: max(1, planned)}
}

// zscale names of the colour properties.
func zMatrix(v, fallback string) string {
	switch v {
	case "bt709":
		return "709"
	case "smpte170m", "bt470bg":
		return "170m"
	case "bt2020nc":
		return "2020_ncl"
	}
	return fallback
}

func zPrimaries(v, fallback string) string {
	switch v {
	case "bt709":
		return "709"
	case "smpte170m", "bt470bg":
		return "170m"
	case "bt2020":
		return "2020"
	}
	return fallback
}

func zTransfer(v string) string {
	switch v {
	case "arib-std-b67", "smpte2084":
		return v
	case "smpte170m":
		return "170m"
	}
	// The BT.709 and SMPTE 170M curves are the same function; unknown values are taken as BT.709.
	return "709"
}

// PlanVideo applies SDD-011 4.1 and 4.3.1 to a video summary. caps may be nil, which assumes zscale.
func PlanVideo(s *Summary, caps *ffmpeg.Capabilities) VideoPlan {
	v := s.Video
	p := VideoPlan{StreamIndex: v.StreamIndex, FPS: v.NominalFPS, Interlaced: v.Interlaced}
	p.W, p.H = NormalizedSize(v.DisplayWidth, v.DisplayHeight)
	p.PW, p.PH = ProxySize(p.W, p.H)
	p.GOP, p.Timescale = GOPFor(p.FPS), TimescaleFor(p.FPS)
	p.Thumbs = PlanThumbnails(s.DurationMS, p.W, p.H)

	p.Deinterlacer = "bwdif"
	if caps != nil && !caps.Bwdif && caps.Yadif {
		p.Deinterlacer = "yadif"
	}

	rw, rh := v.CodedWidth, v.CodedHeight
	if v.SwapDims {
		rw, rh = rh, rw
	}
	p.Scale = p.W != rw || p.H != rh
	p.Alpha = pixFmtHasAlpha(v.PixFmt) && v.HDR == "none"
	if p.Alpha && v.Codec == "vp9" {
		p.Decoder = "libvpx-vp9"
	}

	// Assumed values for what the stream does not say (4.3.1).
	assumed := "smpte170m"
	if v.CodedHeight >= 720 {
		assumed = "bt709"
	}
	m, pr := v.ColorSpace, v.ColorPrim
	if m == "" || m == "unknown" || m == "unspecified" {
		m = assumed
	}
	if pr == "" || pr == "unknown" || pr == "unspecified" {
		pr = assumed
	}
	p.InRangePC = strings.HasPrefix(v.PixFmt, "yuvj") || v.ColorRange == "pc"
	rng := "tv"
	if p.InRangePC {
		rng = "pc"
	}
	t := v.ColorTransfer
	if t == "" || t == "unknown" || t == "unspecified" {
		t = "bt709"
	}
	p.Props = ColourProps{
		Matrix:    zMatrix(m, "709"),
		Primaries: zPrimaries(pr, "709"),
		Transfer:  zTransfer(t),
		Range:     rng,
	}
	p.Matrix601 = m == "smpte170m" || m == "bt470bg"

	hasZ := caps == nil || caps.Zscale
	switch {
	case v.HDR != "none":
		p.Colour = ColourToneMap
	case v.PixFmt == "yuv420p" && m == "bt709" && pr == "bt709" && t == "bt709" && rng == "tv":
		p.Colour = ColourNone
	case p.Alpha:
		// zscale does not carry the alpha plane of every format; the swscale path does.
		p.Colour = ColourScale
		p.Scale = true
	case hasZ:
		p.Colour = ColourZScale
	default:
		p.Colour = ColourScale
		p.Scale = true
	}
	return p
}

// NormalizedFilters is FilterChain of SDD-011 4.3.1: the pieces up to format=yuv420p, in
// order, without the split that follows them. Pieces that do not apply are omitted.
func (p VideoPlan) NormalizedFilters() []ffargs.Filter {
	f := []ffargs.Filter{ffargs.SetPTS()}
	if p.Interlaced {
		if p.Deinterlacer == "yadif" {
			f = append(f, ffargs.Yadif())
		} else {
			f = append(f, ffargs.Bwdif())
		}
	}
	f = append(f, ffargs.FPS(ffargs.Rate{Num: int(p.FPS.Num), Den: int(p.FPS.Den)}))

	pr := p.Props
	switch p.Colour {
	case ColourScale:
		matrix := "bt709"
		if p.Matrix601 {
			matrix = "bt601"
		}
		f = append(f, ffargs.ScaleRange(p.W, p.H, "bicubic+accurate_rnd", pr.Range, matrix), ffargs.SetSAR1())
	default:
		if p.Scale {
			f = append(f, ffargs.Scale(p.W, p.H, "bicubic+accurate_rnd"), ffargs.SetSAR1())
		}
		switch p.Colour {
		case ColourZScale:
			f = append(f, ffargs.ZScale(pr.Matrix, pr.Primaries, pr.Transfer, pr.Range))
		case ColourToneMap:
			f = append(f, ffargs.ToneMapHDR(pr.Matrix, pr.Primaries, pr.Transfer, pr.Range)...)
		case ColourNone, ColourScale:
		}
	}
	if p.Alpha {
		f = append(f, ffargs.Format("yuva444p10le"))
	} else {
		f = append(f, ffargs.Format("yuv420p"))
	}
	// FFmpeg writes the colour tags from the frames: a stream with none needs them set, and the
	// zscale paths already set them.
	if p.Colour == ColourNone || p.Colour == ColourScale {
		f = append(f, ffargs.SetParamsBT709())
	}
	return f
}

// ProxyFilters scales the normalized picture to the proxy size (flags=bicubic: the cheaper set).
// A source with alpha keeps it in the proxy (yuva420p, SDD-011 4.3.5).
func (p VideoPlan) ProxyFilters() []ffargs.Filter {
	pix := "yuv420p"
	if p.Alpha {
		pix = "yuva420p"
	}
	return []ffargs.Filter{ffargs.Scale(p.PW, p.PH, "bicubic"), ffargs.Format(pix)}
}

// ProxyExt is the container of the video proxy: WebM for a source with alpha, which MP4 cannot
// carry for the browser, MP4 for every other (SDD-011 4.4).
func (p VideoPlan) ProxyExt() string {
	if p.Alpha {
		return "webm"
	}
	return "mp4"
}

// TileExt is the format of the thumbnail tiles, the sheet and the posters: PNG for a source with
// alpha, JPEG for every other (SDD-011 4.5).
func (p VideoPlan) TileExt() string {
	if p.Alpha {
		return "png"
	}
	return "jpg"
}

// ThumbFilters cuts one tile every IntervalS seconds from the proxy-sized picture.
func (p VideoPlan) ThumbFilters() []ffargs.Filter {
	f := []ffargs.Filter{ffargs.FPS(ffargs.Rate{Num: 1, Den: p.Thumbs.IntervalS})}
	if p.Thumbs.Letterbox {
		f = append(f, ffargs.ScaleFit(p.Thumbs.TileW, p.Thumbs.TileH)...)
	} else {
		f = append(f, ffargs.Scale(p.Thumbs.TileW, p.Thumbs.TileH, "bicubic"))
	}
	if p.Alpha {
		return append(f, ffargs.Format("rgba"))
	}
	return append(f, ffargs.Format("yuvj420p"))
}

// ImagePlan is the plan of an image (SDD-011 4.7).
type ImagePlan struct {
	StreamIndex int
	W, H        int // normalized size, rounded, not even
	PW, PH      int // proxy size, longest edge at most ImageProxyEdge
	Alpha       bool
}

// PlanImage applies 4.7 to an image summary.
func PlanImage(s *Summary) ImagePlan {
	v := s.Video
	f := math.Min(1, float64(MaxEdge)/float64(max(v.DisplayWidth, v.DisplayHeight)))
	p := ImagePlan{
		StreamIndex: v.StreamIndex,
		W:           max(1, int(math.Round(float64(v.DisplayWidth)*f))),
		H:           max(1, int(math.Round(float64(v.DisplayHeight)*f))),
	}
	g := math.Min(1, float64(ImageProxyEdge)/float64(max(p.W, p.H)))
	p.PW, p.PH = max(1, int(math.Round(float64(p.W)*g))), max(1, int(math.Round(float64(p.H)*g)))
	p.Alpha = pixFmtHasAlpha(v.PixFmt)
	return p
}

// pixFmtHasAlpha is true for the pixel formats that carry an alpha channel (or a palette with one).
func pixFmtHasAlpha(pf string) bool {
	return strings.Contains(pf, "a") &&
		(strings.HasPrefix(pf, "rgba") || strings.HasPrefix(pf, "bgra") ||
			strings.HasPrefix(pf, "argb") || strings.HasPrefix(pf, "ya") || strings.HasPrefix(pf, "yuva") ||
			strings.HasPrefix(pf, "gbrap") || pf == "pal8")
}
