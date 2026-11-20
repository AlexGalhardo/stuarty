package ingest

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// checks collects the named checks of SDD-011 5.1 that failed.
type checks struct {
	mu   sync.Mutex
	list []map[string]any
}

func (c *checks) fail(name string, expected, actual any) {
	c.mu.Lock()
	c.list = append(c.list, checkFail(name, expected, actual))
	c.mu.Unlock()
}

func (c *checks) eq(name string, expected, actual any) {
	if fmt.Sprint(expected) != fmt.Sprint(actual) {
		c.fail(name, expected, actual)
	}
}

func (c *checks) err(step string) error {
	if len(c.list) == 0 {
		return nil
	}
	return verificationFailed(step, c.list)
}

// parallel runs the checks of one artifact at the same time and returns the first system error.
func parallel(fs ...func() error) error {
	errs := make([]error, len(fs))
	var wg sync.WaitGroup
	for i, f := range fs {
		wg.Go(func() { errs[i] = f() })
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// docOf probes a file of the work directory.
func (j *job) docOf(ctx context.Context, name string) (*Doc, error) {
	out, err := j.runProbe(ctx, pq(ffargs.ProbeJSON(j.pol, j.path(name))))
	if err != nil {
		return nil, err
	}
	return ParseDoc(out)
}

func sameRate(a string, want Rational) bool {
	r, ok := ParseRational(a)
	return ok && r.Num*want.Den == want.Num*r.Den
}

func (d *Doc) first(codecType string) *Stream {
	for i := range d.Streams {
		if d.Streams[i].CodecType == codecType {
			return &d.Streams[i]
		}
	}
	return nil
}

type packet struct {
	pts float64
	key bool
}

// packets reads pts_time and flags of one stream, in file order.
func (j *job) packets(ctx context.Context, name string, stream int) ([]packet, error) {
	out, err := j.runProbe(ctx, pq(ffargs.ProbePacketFlags(j.pol, j.path(name), stream)))
	if err != nil {
		return nil, err
	}
	var ps []packet
	for line := range strings.SplitSeq(string(out), "\n") {
		t, fl, ok := strings.Cut(strings.TrimSpace(line), ",")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			f = math.NaN()
		}
		ps = append(ps, packet{pts: f, key: strings.HasPrefix(fl, "K")})
	}
	return ps, nil
}

// gopOK checks the first packet is a keyframe and no keyframe gap exceeds g packets.
func gopOK(ps []packet, g int) (bool, string) {
	if len(ps) == 0 || !ps[0].key {
		return false, "the first packet is not a keyframe"
	}
	last := 0
	for i, p := range ps {
		if p.key {
			if i-last > g {
				return false, fmt.Sprintf("a keyframe gap of %d frames", i-last)
			}
			last = i
		}
	}
	if len(ps)-last > g {
		return false, fmt.Sprintf("a keyframe gap of %d frames", len(ps)-last)
	}
	return true, ""
}

// decodeFrames decodes the first stream of a kind to the null muxer and returns the last frame count
// the progress reported; a non-zero exit is an error.
func (j *job) decodeFrames(ctx context.Context, name, mapSpec string, deadline time.Duration) (int64, error) {
	var last atomic.Int64
	cmd := ffargs.Cmd{
		Progress: true, LogErrorOnly: true, Inputs: []ffargs.Input{ffargs.FileInput(j.path(name))},
		Outputs: []ffargs.Output{{Format: "null", Maps: []string{mapSpec}}},
	}
	argv, err := cmd.Argv(j.pol)
	if err != nil {
		return 0, err
	}
	res, err := j.r.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: j.wd, Argv: argv, Deadline: deadline, DeadlineCeiling: ffmpeg.IngestDeadlineCeiling,
		OnProgress: func(p ffmpeg.Progress) { last.Store(p.Frame) },
	})
	j.mu.Lock()
	j.decodeErrs += countDecodeErrors(res.Stderr)
	j.mu.Unlock()
	return last.Load(), mapRunErr(err, StepVerify, res.Stderr)
}

type countWriter struct{ n atomic.Int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n.Add(int64(len(p))); return len(p), nil }

// decodeSamples decodes the audio of a file to s16le stereo at 48 kHz on a pipe and counts the sample frames.
func (j *job) decodeSamples(ctx context.Context, name string, deadline time.Duration) (int64, error) {
	cmd := ffargs.Cmd{
		LogErrorOnly: true,
		Inputs:       []ffargs.Input{ffargs.FileInput(j.path(name))},
		Outputs: []ffargs.Output{
			{
				Format: "s16le",
				Pipe:   true,
				Maps:   []string{"0:a:0"},
				Opts:   []ffargs.Opt{ffargs.Int("-ar", SampleRate), ffargs.Int("-ac", 2)},
			},
		},
	}
	argv, err := cmd.Argv(j.pol)
	if err != nil {
		return 0, err
	}
	w := &countWriter{}
	res, err := j.r.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: j.wd, Argv: argv, Deadline: deadline, DeadlineCeiling: ffmpeg.IngestDeadlineCeiling, Stdout: w,
	})
	return w.n.Load() / 4, mapRunErr(err, StepVerify, res.Stderr)
}

// topLevelBoxes lists the box types at the top of an MP4 file, up to and including mdat.
func topLevelBoxes(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var types []string
	var pos int64
	for pos+8 <= st.Size() {
		var h [16]byte
		if _, err := f.ReadAt(h[:8], pos); err != nil && err != io.EOF {
			return nil, err
		}
		size := int64(binary.BigEndian.Uint32(h[0:4]))
		typ := string(h[4:8])
		types = append(types, typ)
		if typ == "mdat" {
			break
		}
		switch size {
		case 0:
			size = st.Size() - pos
		case 1:
			if _, err := f.ReadAt(h[8:16], pos+8); err != nil {
				return nil, err
			}
			size = int64(binary.BigEndian.Uint64(h[8:16]) & math.MaxInt64)
		}
		if size < 8 {
			return nil, fmt.Errorf("box %q of size %d", typ, size)
		}
		pos += size
	}
	return types, nil
}

func faststart(path string) (bool, error) {
	types, err := topLevelBoxes(path)
	if err != nil {
		return false, err
	}
	moov, mdat := -1, -1
	for i, t := range types {
		switch t {
		case "moov":
			moov = i
		case "mdat":
			mdat = i
		}
	}
	return moov >= 0 && mdat >= 0 && moov < mdat, nil
}

// verifyVideo is SDD-011 5.1 for a video asset: it decodes what was produced and compares it with the plan.
func (j *job) verifyVideo(ctx context.Context, plan VideoPlan, frames int, samples int64) error {
	c := &checks{}
	deadline := ffmpeg.IngestDeadlineFor(time.Duration(j.durationMS)*time.Millisecond, j.res.Summary.SizeBytes)
	hasAudio := j.res.Facts.HasAudio
	err := parallel(
		func() error { return j.checkNormalizedVideo(ctx, c, plan, frames, deadline) },
		func() error { return j.checkProxy(ctx, c, plan, frames, samples, hasAudio, deadline) },
		func() error {
			if !hasAudio {
				return nil
			}
			return j.checkNormalizedAudio(ctx, c, samples, deadline)
		},
		func() error { return j.checkThumbs(c, plan) },
	)
	if err != nil {
		return err
	}
	if hasAudio {
		j.checkAudioArtifacts(c, samples)
	}
	j.prog.set(StepVerify, 1)
	if e := c.err("normalized video"); e != nil {
		return e
	}
	if plan.Alpha {
		layout := j.layout
		j.markAlphaOpaque(func() bool { return pngAlphaOpaque(j.path("thumbnails."+plan.TileExt()), &layout) })
	}
	if j.decodeErrs > 0 {
		j.res.Warnings = append(j.res.Warnings, "decode_errors: "+strconv.Itoa(j.decodeErrs))
	}
	j.add(workFile{kind: "probe", profile: ProfileProbe, ext: "json", name: "probe.json"})
	return nil
}

// checkNormalizedCodec expects the encoder and pixel format the plan chose for video.mov.
func checkNormalizedCodec(c *checks, plan VideoPlan, codec, pixFmt string) {
	if !plan.Alpha {
		c.eq("video_codec", "mpeg4", codec)
		c.eq("pix_fmt", "yuv420p", pixFmt)

		return
	}

	c.eq("video_codec", "prores", codec)
	// The ProRes decoder reports a 4444 stream as 12 bit; the file holds 10.
	if pixFmt != "yuva444p10le" && pixFmt != "yuva444p12le" {
		c.fail("pix_fmt", "yuva444p10le", pixFmt)
	}
}

func (j *job) checkNormalizedVideo(
	ctx context.Context,
	c *checks,
	plan VideoPlan,
	frames int,
	deadline time.Duration,
) error {
	doc, err := j.docOf(ctx, "normalized.mov")
	if err != nil {
		return err
	}
	v := doc.first("video")
	if v == nil {
		c.fail("video_codec", "one video stream", "none")
		return nil
	}
	checkNormalizedCodec(c, plan, v.CodecName, v.PixFmt)
	c.eq("size", fmt.Sprintf("%dx%d", plan.W, plan.H), fmt.Sprintf("%dx%d", v.Width, v.Height))
	if plan.W%2 != 0 || plan.H%2 != 0 {
		c.fail("size", "even", fmt.Sprintf("%dx%d", plan.W, plan.H))
	}
	if v.SAR != "" && v.SAR != "1:1" && v.SAR != "N/A" {
		c.fail("sar", "1:1", v.SAR)
	}
	for name, got := range map[string]string{"color_space": v.ColorSpace, "color_primaries": v.ColorPrimaries, "color_transfer": v.ColorTransfer} {
		if got != "bt709" {
			c.fail("tags", name+" bt709", got)
		}
	}
	if v.ColorRange != "tv" && v.ColorRange != "" && v.ColorRange != "unknown" {
		c.fail("tags", "color_range tv or unknown", v.ColorRange)
	}
	for _, sd := range v.SideData {
		if sd.Type == "Display Matrix" {
			c.fail("no_display_matrix", "none", "Display Matrix")
		}
	}
	if !sameRate(v.RFrameRate, plan.FPS) || !sameRate(v.AvgFrameRate, plan.FPS) {
		c.fail("frame_rate", plan.FPS.String(), v.RFrameRate+" / "+v.AvgFrameRate)
	}

	ps, err := j.packets(ctx, "normalized.mov", v.Index)
	if err != nil {
		return err
	}
	c.eq("frame_count", frames, len(ps))
	if len(ps) > 0 && ps[0].pts != 0 {
		c.fail("first_pts", 0, ps[0].pts)
	}
	for k, p := range ps {
		if want := float64(
			k,
		) * float64(
			plan.FPS.Den,
		) / float64(
			plan.FPS.Num,
		); math.IsNaN(p.pts) ||
			math.Abs(p.pts-want) > 2e-6+want*1e-9 {
			c.fail("frame_rate", "pts of packet "+itoa(k)+" = "+strconv.FormatFloat(want, 'f', 6, 64), p.pts)
			break
		}
	}
	if ok, why := gopOK(ps, plan.GOP); !ok {
		c.fail("gop", "keyframe at most every "+itoa(plan.GOP)+" frames", why)
	}
	if n := atoi(v.NbFrames); n != 0 && n != frames {
		c.fail("frame_count", frames, "nb_frames "+itoa(n))
	}

	decoded, err := j.decodeFrames(ctx, "normalized.mov", "0:v:0", deadline)
	if err != nil {
		return err
	}
	c.eq("decode", frames, decoded)
	return nil
}

func (j *job) checkNormalizedAudio(ctx context.Context, c *checks, samples int64, deadline time.Duration) error {
	doc, err := j.docOf(ctx, "normalized.mov")
	if err != nil {
		return err
	}
	a := doc.first("audio")
	if a == nil {
		c.fail("audio_codec", "pcm_s16le", "no audio stream")
		return nil
	}
	c.eq("audio_codec", "pcm_s16le 48000 2", a.CodecName+" "+a.SampleRate+" "+itoa(a.Channels))
	if st := atof(a.StartTime); st != 0 {
		c.fail("audio_start", 0, a.StartTime)
	}
	n, err := j.decodeSamples(ctx, "normalized.mov", deadline)
	if err != nil {
		return audioErr(err, a.CodecName)
	}
	c.eq("sample_count", samples, n)
	if a.TimeBase == "1/48000" {
		c.eq("sample_count", samples, a.DurationTS)
	}
	return nil
}

func (j *job) checkProxy(
	ctx context.Context,
	c *checks,
	plan VideoPlan,
	frames int,
	samples int64,
	hasAudio bool,
	deadline time.Duration,
) error {
	proxy := "proxy." + plan.ProxyExt()
	doc, err := j.docOf(ctx, proxy)
	if err != nil {
		return err
	}
	v := doc.first("video")
	if v == nil {
		c.fail("proxy_codec", "vp9", "no video stream")
		return nil
	}
	c.eq("proxy_codec", "vp9", v.CodecName)
	if plan.Alpha && !hasAlphaMode(v) {
		c.fail("proxy_alpha", "alpha_mode=1", "no alpha plane")
	}
	c.eq("proxy_size", fmt.Sprintf("%dx%d", plan.PW, plan.PH), fmt.Sprintf("%dx%d", v.Width, v.Height))
	ps, err := j.packets(ctx, proxy, v.Index)
	if err != nil {
		return err
	}
	if ok, why := gopOK(ps, plan.GOP); !ok {
		c.fail("proxy_gop", "keyframe at most every "+itoa(plan.GOP)+" frames", why)
	}
	if !plan.Alpha {
		if ok, err := faststart(j.path(proxy)); err != nil || !ok {
			c.fail("faststart", "moov before mdat", "moov after mdat")
		}
	}
	decoded, err := j.decodeFrames(ctx, proxy, "0:v:0", deadline)
	if err != nil {
		return err
	}
	c.eq("proxy_frames", frames, decoded)
	if hasAudio {
		a := doc.first("audio")
		want := "aac"
		if plan.Alpha {
			want = "opus"
		}
		if a == nil || a.CodecName != want {
			c.fail("proxy_codec", want, "missing")
		} else if a.TimeBase == "1/48000" {
			c.eq("proxy_audio", samples, a.DurationTS)
		}
	}
	return nil
}

// hasAlphaMode is true for a WebM video stream that says it carries an alpha plane (ffprobe reports
// it as the tag alpha_mode, whatever the case of the name).
func hasAlphaMode(s *Stream) bool {
	for k, v := range s.Tags {
		if strings.EqualFold(k, "alpha_mode") && v == "1" {
			return true
		}
	}
	return false
}

func (j *job) checkThumbs(c *checks, plan VideoPlan) error {
	l := j.layout
	ext := plan.TileExt()
	decodeConfig := jpeg.DecodeConfig
	if plan.Alpha {
		decodeConfig = png.DecodeConfig
	}
	f, err := os.Open(j.path("thumbnails." + ext))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	cfg, err := decodeConfig(f)
	if err != nil {
		c.fail("thumbnails", "a decodable sheet", err.Error())
		return nil
	}
	c.eq(
		"thumbnails",
		fmt.Sprintf("%dx%d", l.Columns*l.TileW, l.Rows()*l.TileH),
		fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
	)
	d := float64(j.durationMS) / 1000
	i := float64(plan.Thumbs.IntervalS)
	lo, hi := max(1, int(math.Floor(d/i)))-1, int(math.Ceil(d/i))+1
	if l.Count < lo || l.Count > hi {
		c.fail("thumbnails", fmt.Sprintf("between %d and %d tiles", lo, hi), l.Count)
	}
	for _, w := range PosterWidths {
		pf, err := os.Open(j.path("w" + itoa(w) + "." + ext))
		if err != nil {
			return err
		}
		pc, derr := decodeConfig(pf)
		_ = pf.Close()
		if derr != nil {
			c.fail("thumbnails", "a decodable poster w"+itoa(w), derr.Error())
			continue
		}
		c.eq(
			"thumbnails",
			"poster w"+itoa(w)+" width "+itoa(min(w, plan.PW)),
			"poster w"+itoa(w)+" width "+itoa(pc.Width),
		)
	}
	return nil
}

// checkAudioArtifacts checks the peaks, silence and loudness files against the sample count.
func (j *job) checkAudioArtifacts(c *checks, samples int64) {
	b, err := os.ReadFile(j.path("peaks.bin"))
	if err != nil {
		c.fail("peaks", "a file", err.Error())
		return
	}
	pk, err := ParsePeaks(b)
	if err != nil {
		c.fail("peaks", "a valid file", err.Error())
		return
	}
	c.eq("peaks", samples, pk.SampleCount)
	if e := pk.Check(j.peaks.Min, j.peaks.Max); e != nil {
		c.fail("peaks", "consistent levels", e.Error())
	}
}

// verifyAudioOnly is SDD-011 5.1 for an audio asset.
func (j *job) verifyAudioOnly(ctx context.Context) error {
	c := &checks{}
	deadline := ffmpeg.IngestDeadlineFor(time.Duration(j.durationMS)*time.Millisecond, j.res.Summary.SizeBytes)
	samples := j.res.Facts.SampleCount
	f, err := os.Open(j.path("normalized.wav"))
	if err != nil {
		return err
	}
	w, rerr := ReadWAV(f)
	_ = f.Close()
	if rerr != nil {
		c.fail("audio_codec", "pcm_s16le 48000 2", rerr.Error())
	} else {
		c.eq("audio_codec", "16-bit 48000 2", fmt.Sprintf("%d-bit %d %d", w.Bits, w.SampleRate, w.Channels))
	}
	err = parallel(
		func() error {
			n, err := j.decodeSamples(ctx, "normalized.wav", deadline)
			if err != nil {
				return audioErr(err, "pcm_s16le")
			}
			c.eq("sample_count", samples, n)
			return nil
		},
		func() error {
			doc, err := j.docOf(ctx, "proxy.mp4")
			if err != nil {
				return err
			}
			a := doc.first("audio")
			if a == nil || a.CodecName != "aac" || doc.first("video") != nil {
				c.fail("proxy_codec", "aac only", "other")
				return nil
			}
			if a.TimeBase == "1/48000" && math.Abs(float64(a.DurationTS-samples)) > 1024 {
				c.fail("proxy_audio", fmt.Sprintf("within 1024 samples of %d", samples), a.DurationTS)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	j.checkAudioArtifacts(c, samples)
	j.prog.set(StepVerify, 1)
	if e := c.err("normalized audio"); e != nil {
		return e
	}
	j.add(workFile{kind: "probe", profile: ProfileProbe, ext: "json", name: "probe.json"})
	return nil
}

// verifyImage decodes the stills with Go's decoders (SDD-011 5.1).
func (j *job) verifyImage(plan ImagePlan, proxyName string) error {
	c := &checks{}
	size := func(name string, dec func(io.Reader) (image.Config, error)) (int, int) {
		f, err := os.Open(j.path(name))
		if err != nil {
			c.fail(name, "a file", err.Error())
			return 0, 0
		}
		defer func() { _ = f.Close() }()
		cfg, err := dec(f)
		if err != nil {
			c.fail(name, "a decodable image", err.Error())
			return 0, 0
		}
		return cfg.Width, cfg.Height
	}
	pngCfg, jpgCfg := png.DecodeConfig, jpeg.DecodeConfig

	w, h := size("normalized.png", pngCfg)
	c.eq("size", fmt.Sprintf("%dx%d", plan.W, plan.H), fmt.Sprintf("%dx%d", w, h))
	// A full decode proves the data, and only after the header has the planned size: a declared size
	// must not size an allocation.
	if w == plan.W && h == plan.H {
		if f, err := os.Open(j.path("normalized.png")); err == nil {
			if _, derr := png.Decode(f); derr != nil {
				c.fail("normalized.png", "a decodable image", derr.Error())
			}
			_ = f.Close()
		}
	}
	var pw, ph int
	if strings.HasSuffix(proxyName, ".png") {
		pw, ph = size(proxyName, pngCfg)
	} else {
		pw, ph = size(proxyName, jpgCfg)
	}
	c.eq("proxy_size", fmt.Sprintf("%dx%d", plan.PW, plan.PH), fmt.Sprintf("%dx%d", pw, ph))
	for _, pwid := range PosterWidths {
		// The poster of an image with alpha is a PNG, like its proxy (SDD-011 4.5).
		poster, dec := "w"+itoa(pwid)+".jpg", jpgCfg
		if plan.Alpha {
			poster, dec = "w"+itoa(pwid)+".png", pngCfg
		}
		w, _ := size(poster, dec)
		c.eq(
			"thumbnails",
			"poster w"+itoa(pwid)+" width "+itoa(min(pwid, plan.PW)),
			"poster w"+itoa(pwid)+" width "+itoa(w),
		)
	}
	if delays := j.res.Summary.FrameDelays; len(delays) > 1 {
		j.verifyAnim(c, plan, delays)
	}
	j.prog.set(StepVerify, 1)
	if e := c.err("image"); e != nil {
		return e
	}
	j.add(workFile{kind: "probe", profile: ProfileProbe, ext: "json", name: "probe.json"})
	return nil
}
