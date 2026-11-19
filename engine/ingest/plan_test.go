package ingest

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

func videoSummary(v VideoFacts, durationMS int64) *Summary {
	if v.NominalFPS == (Rational{}) {
		v.NominalFPS = Rational{30, 1}
	}
	if v.DisplayWidth == 0 {
		v.DisplayWidth, v.DisplayHeight = v.CodedWidth, v.CodedHeight
	}
	if v.Matrix == (Matrix{}) {
		v.Matrix = Matrix{{1, 0}, {0, 1}}
	}
	if v.HDR == "" {
		v.HDR = "none"
	}
	return &Summary{Kind: KindVideo, DurationMS: durationMS, Video: &v}
}

func chain(p VideoPlan) string {
	var parts []string
	for _, f := range p.NormalizedFilters() {
		parts = append(parts, string(f))
	}
	return strings.Join(parts, ",")
}

func TestGeometry(t *testing.T) {
	for _, c := range []struct{ in, want int }{{1, 2}, {3, 4}, {5, 6}, {1079, 1080}, {1081, 1082}, {7, 8}} {
		// Even is 2*round(x/2) with halves rounded away from zero, so 5 becomes 6.
		if got := Even(float64(c.in)); got != c.want {
			t.Errorf("Even(%d) = %d, want %d", c.in, got, c.want)
		}
	}
	if w, h := NormalizedSize(7680, 4320); w != 3840 || h != 2160 {
		t.Errorf("8K: %dx%d", w, h)
	}
	if w, h := NormalizedSize(1080, 1920); w != 1080 || h != 1920 {
		t.Errorf("never upscaled or changed: %dx%d", w, h)
	}
	if w, h := NormalizedSize(321, 641); w%2 != 0 || h%2 != 0 {
		t.Errorf("odd source: %dx%d", w, h)
	}
	if pw, ph := ProxySize(1080, 1920); pw != 540 || ph != 960 {
		t.Errorf("proxy of 1080x1920: %dx%d", pw, ph)
	}
	if pw, ph := ProxySize(3840, 2160); pw != 960 || ph != 540 {
		t.Errorf("proxy of 4K: %dx%d", pw, ph)
	}
	if pw, ph := ProxySize(360, 640); pw != 360 || ph != 640 {
		t.Errorf("a small clip keeps its size: %dx%d", pw, ph)
	}
	if GOPFor(Rational{30, 1}) != 15 || GOPFor(Rational{30000, 1001}) != 15 || GOPFor(Rational{24, 1}) != 12 ||
		GOPFor(Rational{25, 1}) != 13 ||
		GOPFor(Rational{60, 1}) != 30 {
		t.Error("GOP")
	}
	if TimescaleFor(Rational{30, 1}) != 30000 || TimescaleFor(Rational{30000, 1001}) != 30000 ||
		TimescaleFor(Rational{24000, 1001}) != 24000 ||
		TimescaleFor(Rational{60, 1}) != 60000 {
		t.Error("timescale")
	}
}

func TestThumbnailPlan(t *testing.T) {
	// 72x128 tiles for a 1080x1920 clip, a tile per second up to 400 s.
	if p := PlanThumbnails(60_000, 1080, 1920); p.TileW != 72 || p.TileH != 128 || p.IntervalS != 1 || p.Letterbox {
		t.Errorf("%+v", p)
	}
	if p := PlanThumbnails(400_000, 1080, 1920); p.IntervalS != 1 {
		t.Errorf("400 s: %+v", p)
	}
	if p := PlanThumbnails(401_000, 1080, 1920); p.IntervalS != 2 {
		t.Errorf("401 s: %+v", p)
	}
	// 16:9 would be 228 wide: under the 256 limit; a 3:1 frame is letterboxed at 256.
	if p := PlanThumbnails(10_000, 1920, 1080); p.TileW != 228 || p.Letterbox {
		t.Errorf("16:9: %+v", p)
	}
	if p := PlanThumbnails(10_000, 3000, 1000); p.TileW != 256 || !p.Letterbox {
		t.Errorf("3:1: %+v", p)
	}
}

func TestPlanVideoColourPaths(t *testing.T) {
	caps := &ffmpeg.Capabilities{Zscale: true, Tonemap: true, Bwdif: true, Yadif: true}

	// 1080p, untagged, 8-bit 4:2:0: assumed BT.709 limited, already the target; the tags are set.
	p := PlanVideo(videoSummary(VideoFacts{CodedWidth: 1080, CodedHeight: 1920, PixFmt: "yuv420p"}, 60000), caps)
	if p.Colour != ColourNone || p.Scale || !strings.Contains(chain(p), "setparams=colorspace=bt709") {
		t.Errorf("1080p untagged: %v %s", p.Colour, chain(p))
	}
	if got := chain(
		p,
	); got != "setpts=PTS-STARTPTS,fps=30/1:round=near,format=yuv420p,setparams=colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv" {
		t.Errorf("chain %s", got)
	}

	// Below 720 lines the assumption is SMPTE 170M: a conversion with explicit input properties.
	p = PlanVideo(videoSummary(VideoFacts{CodedWidth: 360, CodedHeight: 640, PixFmt: "yuv420p"}, 3000), caps)
	if p.Colour != ColourZScale ||
		!strings.Contains(chain(p), "zscale=min=709") && !strings.Contains(chain(p), "zscale=min=170m") {
		t.Errorf("small untagged: %v %s", p.Colour, chain(p))
	}

	// A full-range JPEG-style source (yuvj420p, smpte170m, no other tags): pin and tin are given.
	p = PlanVideo(
		videoSummary(
			VideoFacts{CodedWidth: 1080, CodedHeight: 1920, PixFmt: "yuvj420p", ColorSpace: "smpte170m"},
			3000,
		),
		caps,
	)
	if p.Colour != ColourZScale || !strings.Contains(chain(p), "min=170m") || !strings.Contains(chain(p), "rin=pc") ||
		!strings.Contains(chain(p), "tin=709") {
		t.Errorf("yuvj: %v %s", p.Colour, chain(p))
	}

	// HLG: tone mapping in linear light, hable at 100 nits with no desaturation.
	p = PlanVideo(
		videoSummary(
			VideoFacts{
				CodedWidth:    1080,
				CodedHeight:   1920,
				PixFmt:        "yuv420p10le",
				ColorSpace:    "bt2020nc",
				ColorPrim:     "bt2020",
				ColorTransfer: "arib-std-b67",
				HDR:           "hlg",
			},
			3000,
		),
		caps,
	)
	c := chain(p)
	if p.Colour != ColourToneMap || !strings.Contains(c, "tonemap=tonemap=hable:desat=0") ||
		!strings.Contains(c, "t=linear:npl=100") ||
		!strings.Contains(c, "tin=arib-std-b67") ||
		!strings.Contains(c, "min=2020_ncl") {
		t.Errorf("hlg: %v %s", p.Colour, c)
	}

	// A build with no zscale: range and matrix by scale alone, always with the size.
	p = PlanVideo(
		videoSummary(
			VideoFacts{CodedWidth: 1080, CodedHeight: 1920, PixFmt: "yuvj420p", ColorSpace: "smpte170m"},
			3000,
		),
		&ffmpeg.Capabilities{Bwdif: true},
	)
	if p.Colour != ColourScale ||
		!strings.Contains(
			chain(p),
			"scale=1080:1920:flags=bicubic+accurate_rnd:in_range=pc:out_range=tv:in_color_matrix=bt601:out_color_matrix=bt709",
		) {
		t.Errorf("no zscale: %v %s", p.Colour, chain(p))
	}
}

func TestPlanVideoShapeAndDeinterlace(t *testing.T) {
	caps := &ffmpeg.Capabilities{Zscale: true, Tonemap: true, Yadif: true}
	// A rotated clip: coded 1920x1080 with a quarter turn shows 1080x1920, no scale needed.
	p := PlanVideo(videoSummary(VideoFacts{
		CodedWidth:    1920,
		CodedHeight:   1080,
		PixFmt:        "yuv420p",
		ColorSpace:    "bt709",
		ColorPrim:     "bt709",
		ColorTransfer: "bt709",
		SwapDims:      true,
		DisplayWidth:  1080,
		DisplayHeight: 1920,
	}, 3000), caps)
	if p.W != 1080 || p.H != 1920 || p.Scale {
		t.Errorf("rotated: %dx%d scale=%v", p.W, p.H, p.Scale)
	}
	// 4K stays 4K; an 8K source is capped and scaled, and a non-square pixel clip is scaled.
	p = PlanVideo(
		videoSummary(
			VideoFacts{
				CodedWidth:    7680,
				CodedHeight:   4320,
				PixFmt:        "yuv420p",
				ColorSpace:    "bt709",
				ColorPrim:     "bt709",
				ColorTransfer: "bt709",
			},
			3000,
		),
		caps,
	)
	if p.W != 3840 || p.H != 2160 || !p.Scale ||
		!strings.Contains(chain(p), "scale=3840:2160:flags=bicubic+accurate_rnd,setsar=1") {
		t.Errorf("8K: %dx%d %s", p.W, p.H, chain(p))
	}
	// bwdif is absent from the capability list: yadif with the same options.
	p = PlanVideo(
		videoSummary(VideoFacts{CodedWidth: 720, CodedHeight: 576, PixFmt: "yuv420p", Interlaced: true}, 3000),
		caps,
	)
	if p.Deinterlacer != "yadif" || !strings.Contains(chain(p), "yadif=mode=send_frame:parity=auto:deint=interlaced") {
		t.Errorf("deinterlace: %s %s", p.Deinterlacer, chain(p))
	}
}

func jsonDoc(streams string) *Doc {
	d, err := ParseDoc(
		[]byte(
			`{"streams":[` + streams + `],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"3.0","size":"1000"}}`,
		),
	)
	if err != nil {
		panic(err)
	}
	return d
}

func TestSummarizeFollowsTheClassificationTable(t *testing.T) {
	caps := &ffmpeg.Capabilities{
		Zscale:   true,
		Tonemap:  true,
		Decoders: map[string]bool{"h264": true, "hevc": true, "aac": true, "mjpeg": true},
	}
	pts := regular(90, 1.0/30)
	vid := func(extra string) string {
		return `{"index":0,"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"coded_width":1080,"coded_height":1920,"pix_fmt":"yuv420p","r_frame_rate":"30/1","avg_frame_rate":"30/1"` + extra + `}`
	}
	aud := `{"index":1,"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"44100","start_time":"0.000000"}`

	check := func(name string, d *Doc, ex Extras, reason string) {
		t.Helper()
		sel, err := d.Select()
		if err == nil {
			_, err = Summarize(d, sel, ex, caps, 1000)
		}
		var fe *ffmpeg.Error
		if reason == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			return
		}
		if !errors.As(err, &fe) || fe.Reason != reason {
			t.Errorf("%s: %v, want %s", name, err, reason)
		}
	}
	check("a phone clip", jsonDoc(vid("")+","+aud), Extras{PacketTimes: pts}, "")
	check("no frames", jsonDoc(vid("")), Extras{}, "no_frames")
	check(
		"hevc without a decoder",
		jsonDoc(strings.Replace(vid(""), "h264", "av1", 1)),
		Extras{PacketTimes: pts},
		"decoder_missing",
	)
	check(
		"dolby vision 5",
		jsonDoc(vid(`,"side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":5}]`)),
		Extras{PacketTimes: pts},
		"dolby_vision_profile_5",
	)
	check("hlg", jsonDoc(vid(`,"color_transfer":"arib-std-b67"`)), Extras{PacketTimes: pts}, "")
	check(
		"only a picture",
		jsonDoc(
			`{"index":0,"codec_type":"video","codec_name":"mjpeg","width":10,"height":10,"disposition":{"attached_pic":1}}`,
		),
		Extras{},
		"no_usable_stream",
	)
	check("only data", jsonDoc(`{"index":0,"codec_type":"data","codec_name":"bin_data"}`), Extras{}, "no_usable_stream")
	check("too fast", jsonDoc(vid("")), Extras{PacketTimes: regular(600, 1.0/300)}, "frame_rate_too_high")

	// A cover picture beside audio is an audio asset with the picture ignored.
	d := jsonDoc(
		aud + `,{"index":2,"codec_type":"video","codec_name":"mjpeg","width":300,"height":300,"disposition":{"attached_pic":1}}`,
	)
	sel, err := d.Select()
	if err != nil || sel.Kind != KindAudio {
		t.Fatalf("cover art: %v %v", sel.Kind, err)
	}
	s, _ := Summarize(d, sel, Extras{}, caps, 10)
	if len(s.IgnoredStreams) != 1 || s.IgnoredStreams[0].Reason != "attached_picture" {
		t.Errorf("ignored %+v", s.IgnoredStreams)
	}

	// HDR without zscale.
	hdr := jsonDoc(vid(`,"color_transfer":"smpte2084"`))
	sel, _ = hdr.Select()
	if _, err := Summarize(
		hdr,
		sel,
		Extras{PacketTimes: pts},
		&ffmpeg.Capabilities{Decoders: caps.Decoders},
		1,
	); err == nil ||
		!strings.Contains(err.Error(), "tone-map") {
		t.Errorf("hdr with no zscale: %v", err)
	}

	// The audio offset is the start difference in samples.
	d = jsonDoc(
		vid(
			`,"start_time":"0.000000"`,
		) + `,{"index":1,"codec_type":"audio","codec_name":"aac","channels":1,"sample_rate":"44100","start_time":"0.276800"}`,
	)
	sel, _ = d.Select()
	s, err = Summarize(d, sel, Extras{PacketTimes: pts}, caps, 1)
	if err != nil || s.Audio.OffsetSamples != 13286 {
		t.Errorf("offset %v %v", s.Audio, err)
	}
}

func TestDisplayMatrix(t *testing.T) {
	rot90 := "\n00000000:            0       65536           0\n00000001:       -65536           0           0\n00000002:           0           0  1073741824\n"
	if m := ParseDisplayMatrix(rot90); !m.SwapDimensions() || m.Mirrored() {
		t.Errorf("rotation 90: %v", m)
	}
	// Rotation 90 plus a flip is [[0,-1],[-1,0]]: swapped and mirrored.
	flip := "\n00000000:            0      -65536           0\n00000001:       -65536           0           0\n00000002:           0           0  1073741824\n"
	if m := ParseDisplayMatrix(flip); !m.SwapDimensions() || !m.Mirrored() {
		t.Errorf("rotation and flip: %v", m)
	}
	hflip := "\n00000000:       -65536           0           0\n00000001:            0       65536           0\n00000002:           0           0  1073741824\n"
	if m := ParseDisplayMatrix(hflip); m.SwapDimensions() || !m.Mirrored() {
		t.Errorf("hflip: %v", m)
	}
	if m := ParseDisplayMatrix("garbage"); m != (Matrix{{1, 0}, {0, 1}}) {
		t.Errorf("identity expected: %v", m)
	}
	if w, h := displaySize(720, 480, Rational{32, 27}, false); w != 853 || h != 480 {
		t.Errorf("anamorphic: %dx%d", w, h)
	}
}

func TestSummarizeKeepsContainerAndStreamTags(t *testing.T) {
	d, err := ParseDoc([]byte(`{
		"streams":[
			{"index":0,"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"coded_width":1080,"coded_height":1920,"pix_fmt":"yuv420p","r_frame_rate":"30/1","avg_frame_rate":"30/1",
			 "tags":{"title":"stream title","handler_name":"VideoHandler","encoder":"stream encoder","unlisted":"x"}},
			{"index":1,"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"44100","start_time":"0.000000",
			 "tags":{"HANDLER_NAME":"SoundHandler","artist":"stream artist"}}
		],
		"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"3.0","size":"1000",
			"tags":{"title":"container title","encoder":"Lavf61.7.100","comment":"a\u0000b\u001b[31mred\nline","description":"` + strings.Repeat("é", 250) + `","major_brand":"isom"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	sel, err := d.Select()
	if err != nil {
		t.Fatal(err)
	}
	caps := &ffmpeg.Capabilities{Decoders: map[string]bool{"h264": true, "aac": true}}
	s, err := Summarize(d, sel, Extras{PacketTimes: regular(90, 1.0/30)}, caps, 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Tag{
		"title":        {Value: "container title"},
		"encoder":      {Value: "Lavf61.7.100"},
		"artist":       {Value: "stream artist"},
		"handler_name": {Value: "VideoHandler"},
		"comment":      {Value: "ab[31mredline"},
		"description":  {Value: strings.Repeat("é", 200), Truncated: true},
	}
	if len(s.Tags) != len(want) {
		t.Errorf("tags %v, want %v", s.Tags, want)
	}
	for k, w := range want {
		if s.Tags[k] != w {
			t.Errorf("tag %s = %+v, want %+v", k, s.Tags[k], w)
		}
	}
}

// A source with an alpha channel keeps it: ProRes 4444 in yuva444p10le, through the swscale path.
func TestPlanVideoKeepsAlpha(t *testing.T) {
	caps := &ffmpeg.Capabilities{Zscale: true, Tonemap: true, Bwdif: true, Yadif: true}

	p := PlanVideo(videoSummary(VideoFacts{CodedWidth: 1920, CodedHeight: 1080, PixFmt: "yuva444p10le"}, 3000), caps)
	if !p.Alpha || p.Colour != ColourScale || !strings.Contains(chain(p), "format=yuva444p10le") ||
		strings.Contains(chain(p), "yuv420p") {
		t.Errorf("alpha source: alpha=%v %v %s", p.Alpha, p.Colour, chain(p))
	}

	p = PlanVideo(videoSummary(VideoFacts{CodedWidth: 1920, CodedHeight: 1080, PixFmt: "yuv420p"}, 3000), caps)
	if p.Alpha || !strings.Contains(chain(p), "format=yuv420p") {
		t.Errorf("opaque source: alpha=%v %s", p.Alpha, chain(p))
	}

	// The proxy and the thumbnails keep the alpha too, in WebM and PNG.
	proxy := func(p VideoPlan) string { return fmt.Sprint(p.ProxyFilters(), p.ThumbFilters()) }
	alpha := PlanVideo(
		videoSummary(VideoFacts{CodedWidth: 1920, CodedHeight: 1080, PixFmt: "yuva444p10le"}, 3000),
		caps,
	)
	if got := proxy(alpha); !strings.Contains(got, "format=yuva420p") || !strings.Contains(got, "format=rgba") ||
		strings.Contains(got, "yuv420p") || strings.Contains(got, "yuvj420p") ||
		alpha.ProxyExt() != "webm" || alpha.TileExt() != "png" {
		t.Errorf("alpha proxy: %s %s %s", got, alpha.ProxyExt(), alpha.TileExt())
	}
	if got := proxy(p); !strings.Contains(got, "format=yuv420p") || !strings.Contains(got, "format=yuvj420p") ||
		p.ProxyExt() != "mp4" || p.TileExt() != "jpg" {
		t.Errorf("opaque proxy: %s %s %s", got, p.ProxyExt(), p.TileExt())
	}
}

// A VP9 stream in WebM with an alpha plane is probed as yuv420p plus an alpha_mode tag: the summary
// says yuva420p and the plan forces libvpx-vp9, the decoder that keeps the plane (VV-013).
func TestPlanVideoKeepsVP9Alpha(t *testing.T) {
	caps := &ffmpeg.Capabilities{Zscale: true, Tonemap: true, Bwdif: true, Yadif: true}
	st := func(codec, tag string) *VideoFacts {
		v, err := videoFacts(&Stream{
			CodecName: codec, PixFmt: "yuv420p", Width: 64, Height: 64, CodedWidth: 64, CodedHeight: 64,
			Tags: map[string]string{"alpha_mode": tag},
		}, Extras{}, nil)
		if err != nil {
			t.Fatal(err)
		}

		return v
	}

	v := st("vp9", "1")
	if v.PixFmt != "yuva420p" {
		t.Fatalf("pix_fmt = %q, want yuva420p", v.PixFmt)
	}

	if p := PlanVideo(videoSummary(*v, 3000), caps); !p.Alpha || p.Decoder != "libvpx-vp9" {
		t.Errorf("vp9 alpha: alpha=%v decoder=%q", p.Alpha, p.Decoder)
	}

	if v := st("vp9", "0"); v.PixFmt != "yuv420p" {
		t.Errorf("alpha_mode 0 changed pix_fmt to %q", v.PixFmt)
	}

	if v := st("h264", "1"); v.PixFmt != "yuv420p" {
		t.Errorf("a non-vp9 stream changed pix_fmt to %q", v.PixFmt)
	}

	prores := VideoFacts{Codec: "prores", CodedWidth: 64, CodedHeight: 64, PixFmt: "yuva444p10le"}

	p := PlanVideo(videoSummary(prores, 3000), caps)
	if p.Decoder != "" {
		t.Errorf("prores alpha forced the decoder %q", p.Decoder)
	}
}
