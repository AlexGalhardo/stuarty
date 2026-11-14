// Disposable spike: compares three ways of compositing an animated text layer
// on a 1080x1920 30 fps video with FFmpeg driven from Go.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	W, H      = 1080, 1920
	FPS       = 30
	Seconds   = 30
	Frames    = FPS * Seconds
	BmpW      = 900
	BmpH      = 180
	TextY     = 1310 // canvas y of the text bitmap
	RegionY   = 1200 // S2 overlay region origin
	RegionH   = 400
	FadeSec   = 0.25
	TextX0    = 40.0
	TextSpeed = 60.0 // px per second inside each 1 s slot
)

type Result struct {
	Strategy    string   `json:"strategy"`
	Encoder     string   `json:"encoder"`
	Run         int      `json:"run"`
	Mode        string   `json:"mode"`
	WallSec     float64  `json:"wall_sec"`
	PrerenderMs float64  `json:"prerender_ms"`
	GoRenderSec float64  `json:"go_render_sec"`
	EncodeFPS   float64  `json:"encode_fps"`
	Realtime    float64  `json:"realtime_factor"`
	SizeBytes   int64    `json:"size_bytes"`
	Valid       bool     `json:"valid"`
	Problems    []string `json:"problems,omitempty"`
	Error       string   `json:"error,omitempty"`
	GoRawSHA256 string   `json:"go_raw_sha256,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: exporter gen-source|export [flags]")
	}
	sub := os.Args[1]
	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	ffmpeg := fs.String("ffmpeg", "ffmpeg", "ffmpeg binary")
	ffprobe := fs.String("ffprobe", "ffprobe", "ffprobe binary")
	src := fs.String("src", "source.mp4", "source media")
	out := fs.String("out", "out.mp4", "output file")
	strategy := fs.String("strategy", "s1", "s1|s2|s3")
	encoder := fs.String("encoder", "h264_mf", "h264_mf|libopenh264|h264_nvenc|libx264|ffv1")
	mode := fs.String("mode", "encode", "encode|framemd5")
	run := fs.Int("run", 1, "run number (label only)")
	bitrate := fs.String("bitrate", "8M", "video bitrate")
	results := fs.String("results", "", "append JSON result line to this file")
	workdir := fs.String("work", ".", "dir for S3 PNGs")
	fs.Parse(os.Args[2:])

	switch sub {
	case "gen-source":
		genSource(*ffmpeg, *out)
	case "export":
		r := export(*ffmpeg, *ffprobe, *src, *out, *strategy, *encoder, *mode, *bitrate, *workdir)
		r.Run = *run
		b, _ := json.Marshal(r)
		fmt.Println(string(b))
		if *results != "" {
			f, err := os.OpenFile(*results, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			check(err)
			f.Write(append(b, '\n'))
			f.Close()
		}
	default:
		fatal("unknown subcommand " + sub)
	}
}

func genSource(ffmpeg, out string) {
	// Stand-in for "normalized media": short GOP (15 frames). libx264 from PATH is
	// used only to build this fixture; it is not part of the shipped pipeline.
	args := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=%dx%d:r=%d:d=%d", W, H, FPS, Seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=f=440:r=48000:d=%d", Seconds),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-g", "15", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", out}
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stderr = os.Stderr
	check(cmd.Run())
}

// ---- text rasterization (our own Go code, straight-alpha RGBA) ----

func textFor(k int) string { return fmt.Sprintf("Scene %02d - Vukem", k+1) }

func renderTexts() []*image.NRGBA {
	f, err := opentype.Parse(goregular.TTF)
	check(err)
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 100, DPI: 72, Hinting: font.HintingFull})
	check(err)
	bmps := make([]*image.NRGBA, Seconds)
	for k := range bmps {
		img := image.NewNRGBA(image.Rect(0, 0, BmpW, BmpH))
		for _, layer := range []struct {
			c  color.NRGBA
			dx int
		}{{color.NRGBA{0, 0, 0, 160}, 5}, {color.NRGBA{255, 255, 255, 255}, 0}} {
			d := &font.Drawer{Dst: img, Src: image.NewUniform(layer.c), Face: face,
				Dot: fixed.P(20+layer.dx, 130+layer.dx)}
			d.DrawString(textFor(k))
		}
		bmps[k] = img
	}
	return bmps
}

func fade(u float64) float64 {
	return math.Max(0, math.Min(1, math.Min(u/FadeSec, (1-u)/FadeSec)))
}

// blit draws bitmap b (straight alpha) into dst (RGBA bytes, straight alpha) with extra alpha factor.
func blit(dst []byte, dw int, b *image.NRGBA, ox, oy int, factor float64) {
	f := int(factor*256 + 0.5)
	for y := 0; y < BmpH; y++ {
		s := b.Pix[y*b.Stride : y*b.Stride+BmpW*4]
		d := dst[((oy+y)*dw+ox)*4:]
		for x := 0; x < BmpW*4; x += 4 {
			a := int(s[x+3])
			if a == 0 {
				continue
			}
			d[x], d[x+1], d[x+2] = s[x], s[x+1], s[x+2]
			d[x+3] = byte(a * f >> 8)
		}
	}
}

// ---- export ----

func export(ffmpeg, ffprobe, src, out, strategy, encoder, mode, bitrate, workdir string) Result {
	r := Result{Strategy: strategy, Encoder: encoder, Mode: mode}
	t0 := time.Now()
	bmps := renderTexts()

	args := []string{"-hide_banner", "-loglevel", "error", "-nostats", "-y", "-i", src}
	bg := "[0:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,fps=30,format=yuv420p[bg];"
	var filter string
	pipeW, pipeH, pipeY := 0, 0, 0
	switch strategy {
	case "s1":
		pipeW, pipeH, pipeY = W, H, 0
	case "s2":
		pipeW, pipeH, pipeY = W, RegionH, RegionY
	case "s3":
		var sb strings.Builder
		sb.WriteString(bg)
		prev := "bg"
		for k := 0; k < Seconds; k++ {
			p := filepath.Join(workdir, fmt.Sprintf("text_%02d.png", k))
			fp, err := os.Create(p)
			check(err)
			check(png.Encode(fp, bmps[k]))
			fp.Close()
			args = append(args, "-loop", "1", "-framerate", "30", "-t", "1", "-i", p)
			fmt.Fprintf(&sb, "[%d:v]format=rgba,fade=t=in:st=0:d=%g:alpha=1,fade=t=out:st=%g:d=%g:alpha=1,setpts=PTS+%d/TB[t%d];",
				k+1, FadeSec, 1-FadeSec, FadeSec, k, k)
			fmt.Fprintf(&sb, "[%s][t%d]overlay=x='%g+%g*(t-%d)':y=%d:eval=frame:eof_action=pass:format=yuv420:enable='between(t,%d,%g)'[v%d];",
				prev, k, TextX0, TextSpeed, k, TextY, k, float64(k)+1-0.001, k)
			prev = fmt.Sprintf("v%d", k)
		}
		fmt.Fprintf(&sb, "[%s]format=yuv420p[v]", prev)
		filter = sb.String()
	default:
		fatal("bad strategy")
	}
	r.PrerenderMs = float64(time.Since(t0).Microseconds()) / 1000
	piped := strategy != "s3"
	if piped {
		args = append(args, "-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", pipeW, pipeH),
			"-framerate", "30", "-i", "pipe:0")
		filter = bg + fmt.Sprintf("[bg][1:v]overlay=0:%d:format=yuv420:eof_action=pass,format=yuv420p[v]", pipeY)
	}
	args = append(args, "-filter_complex", filter, "-map", "[v]")
	switch {
	case mode == "framemd5":
		args = append(args, "-f", "framemd5", out)
	case encoder == "ffv1":
		args = append(args, "-c:v", "ffv1", "-level", "3", "-g", "1", "-r", "30", out)
	default:
		vargs := []string{"-c:v", encoder, "-b:v", bitrate, "-g", "60", "-pix_fmt", "yuv420p", "-r", "30"}
		if encoder == "libx264" {
			vargs = append(vargs, "-preset", "veryfast")
		}
		args = append(args, vargs...)
		args = append(args, "-map", "0:a", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", out)
	}

	cmd := exec.Command(ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdin io.WriteCloser
	if piped {
		var err error
		stdin, err = cmd.StdinPipe()
		check(err)
	}
	start := time.Now()
	check(cmd.Start())
	var renderTime time.Duration
	var hasher hash.Hash
	if mode == "framemd5" && piped {
		hasher = sha256.New()
	}
	if piped {
		free := make(chan []byte, 6)
		full := make(chan []byte, 6)
		for i := 0; i < 6; i++ {
			free <- make([]byte, pipeW*pipeH*4)
		}
		done := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(full)
			for n := 0; n < Frames; n++ {
				var buf []byte
				select {
				case buf = <-free:
				case <-done:
					return
				}
				ts := time.Now()
				clear(buf)
				k, u := n/FPS, float64(n%FPS)/FPS
				x := int(TextX0 + TextSpeed*u)
				blit(buf, pipeW, bmps[k], x, TextY-pipeY, fade(u))
				renderTime += time.Since(ts)
				select {
				case full <- buf:
				case <-done:
					return
				}
			}
		}()
		for buf := range full {
			if hasher != nil {
				hasher.Write(buf)
			}
			if _, err := stdin.Write(buf); err != nil {
				r.Error = "pipe write: " + err.Error()
				close(done)
				break
			}
			free <- buf
		}
		stdin.Close()
		wg.Wait()
	}
	werr := cmd.Wait()
	wall := time.Since(start)
	r.WallSec = wall.Seconds()
	r.GoRenderSec = renderTime.Seconds()
	if hasher != nil {
		r.GoRawSHA256 = hex.EncodeToString(hasher.Sum(nil))
	}
	if werr != nil {
		r.Error = strings.TrimSpace(r.Error + " ffmpeg: " + werr.Error() + " " + stderr.String())
		return r
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		r.Problems = append(r.Problems, "stderr: "+s)
	}
	// Wall for S3 includes PNG pre-render; S1/S2 include the in-process rasterizer (overlapped).
	if strategy == "s3" {
		r.WallSec += r.PrerenderMs / 1000
	}
	r.EncodeFPS = Frames / r.WallSec
	r.Realtime = Seconds / r.WallSec
	if st, err := os.Stat(out); err == nil {
		r.SizeBytes = st.Size()
	}
	if mode == "encode" && encoder != "ffv1" {
		r.Problems = append(r.Problems, validate(ffprobe, out)...)
		r.Valid = true
		for _, p := range r.Problems {
			if !strings.HasPrefix(p, "stderr:") {
				r.Valid = false
			}
		}
	}
	return r
}

// ---- validation ----

func validate(ffprobe, file string) []string {
	var probs []string
	o, err := exec.Command(ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", file).Output()
	if err != nil {
		return []string{"ffprobe failed: " + err.Error()}
	}
	var j struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			PixFmt    string `json:"pix_fmt"`
			Duration  string `json:"duration"`
			RFrame    string `json:"r_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(o, &j); err != nil {
		return []string{"ffprobe json: " + err.Error()}
	}
	var hasV, hasA bool
	for _, s := range j.Streams {
		switch s.CodecType {
		case "video":
			hasV = true
			if s.Width != W || s.Height != H {
				probs = append(probs, fmt.Sprintf("size %dx%d", s.Width, s.Height))
			}
			if s.PixFmt != "yuv420p" {
				probs = append(probs, "pix_fmt "+s.PixFmt)
			}
			if s.RFrame != "30/1" {
				probs = append(probs, "r_frame_rate "+s.RFrame)
			}
			if s.CodecName != "h264" {
				probs = append(probs, "codec "+s.CodecName)
			}
			var d float64
			fmt.Sscanf(s.Duration, "%g", &d)
			if math.Abs(d-Seconds) > 1.0/FPS+1e-6 {
				probs = append(probs, fmt.Sprintf("video duration %g", d))
			}
		case "audio":
			hasA = true
			if s.CodecName != "aac" {
				probs = append(probs, "audio codec "+s.CodecName)
			}
		}
	}
	if !hasV || !hasA {
		probs = append(probs, fmt.Sprintf("streams video=%v audio=%v", hasV, hasA))
	}
	var fd float64
	fmt.Sscanf(j.Format.Duration, "%g", &fd)
	if math.Abs(fd-Seconds) > 1.0/FPS+1e-6 {
		probs = append(probs, fmt.Sprintf("format duration %g", fd))
	}
	if ok, err := faststart(file); err != nil || !ok {
		probs = append(probs, fmt.Sprintf("faststart=%v err=%v", ok, err))
	}
	return probs
}

// faststart reports whether the moov box precedes mdat at the top level.
func faststart(file string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, _ := f.Stat()
	var off int64
	moov, mdat := int64(-1), int64(-1)
	for off+8 <= st.Size() {
		var hdr [16]byte
		if _, err := f.ReadAt(hdr[:8], off); err != nil {
			return false, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		if size == 1 {
			f.ReadAt(hdr[8:16], off+8)
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		} else if size == 0 {
			size = st.Size() - off
		}
		if typ == "moov" && moov < 0 {
			moov = off
		}
		if typ == "mdat" && mdat < 0 {
			mdat = off
		}
		if size < 8 {
			break
		}
		off += size
	}
	return moov >= 0 && mdat >= 0 && moov < mdat, nil
}

func check(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(1)
}
