// Disposable spike: is a pure-Go compositor fast enough? Decoder ffmpeg -> Go compose -> encoder ffmpeg.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const swsFlags = "bicubic+accurate_rnd+full_chroma_int"

var allSws bool

type Result struct {
	Case       string   `json:"case"`
	Encoder    string   `json:"encoder"`
	Sampler    string   `json:"sampler,omitempty"`
	Bands      int      `json:"bands"`
	Run        int      `json:"run"`
	WallSec    float64  `json:"wall_sec"`
	Realtime   float64  `json:"realtime_factor"`
	FPS        float64  `json:"fps"`
	PeakRSSMB  float64  `json:"peak_rss_mb"`
	ReadSec    float64  `json:"read_sec"`
	ComposeSec float64  `json:"compose_sec"`
	WriteSec   float64  `json:"write_sec"`
	SizeBytes  int64    `json:"size_bytes"`
	Valid      *bool    `json:"valid,omitempty"`
	Problems   []string `json:"problems,omitempty"`
	HashSum    string   `json:"hash_of_hashes,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func textFor(k int) string { return fmt.Sprintf("Scene %02d - Vukem", k+1) }

func main() {
	if len(os.Args) < 2 {
		fatal("usage: compositor gen-source|run|cmp")
	}
	sub := os.Args[1]
	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	ffmpeg := fs.String("ffmpeg", "ffmpeg", "ffmpeg binary")
	ffprobe := fs.String("ffprobe", "ffprobe", "ffprobe binary")
	srcP := fs.String("src", "portrait.mp4", "1080x1920 source (also audio)")
	srcL := fs.String("srcl", "landscape.mp4", "1920x1080 source")
	out := fs.String("out", "out.mp4", "output file")
	cs := fs.String("case", "c1", "c1..c5")
	enc := fs.String("encoder", "h264_nvenc", "encoder, or null / nullconv")
	bands := fs.Int("bands", 1, "horizontal bands (goroutines)")
	approx := fs.Bool("approx", false, "C4: ApproxBiLinear instead of BiLinear")
	hashes := fs.String("hashes", "", "write per-frame sha256 of composed frames to this file")
	run := fs.Int("run", 1, "run label")
	results := fs.String("results", "", "append JSON line here")
	kind := fs.String("kind", "portrait", "gen-source: portrait|landscape")
	allSwsF := fs.Bool("allsws", false, "apply -sws_flags to every decoder (default: only C2 bg and C3 pip)")
	fs.Parse(os.Args[2:])
	allSws = *allSwsF

	switch sub {
	case "gen-source":
		genSource(*ffmpeg, *out, *kind)
	case "run":
		r := doRun(*ffmpeg, *ffprobe, *srcP, *srcL, *out, *cs, *enc, *bands, *approx, *hashes)
		r.Run = *run
		b, _ := json.Marshal(r)
		fmt.Println(string(b))
		if *results != "" {
			f, err := os.OpenFile(*results, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			check(err)
			f.Write(append(b, '\n'))
			f.Close()
		}
	case "cmp":
		a, b := fs.Arg(0), fs.Arg(1)
		ha, err := os.ReadFile(a)
		check(err)
		hb, err := os.ReadFile(b)
		check(err)
		la, lb := strings.Split(strings.TrimSpace(string(ha)), "\n"), strings.Split(strings.TrimSpace(string(hb)), "\n")
		diff := 0
		for i := range la {
			if i >= len(lb) || la[i] != lb[i] {
				diff++
			}
		}
		fmt.Printf("%s vs %s: frames=%d/%d differing=%d\n", a, b, len(la), len(lb), diff)
	default:
		fatal("unknown subcommand " + sub)
	}
}

func genSource(ffmpeg, out, kind string) {
	size, freq := fmt.Sprintf("%dx%d", W, H), 440
	if kind == "landscape" {
		size, freq = fmt.Sprintf("%dx%d", LandW, LandH), 660
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=%s:r=%d:d=%d", size, FPS, Seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=f=%d:r=48000:d=%d", freq, Seconds),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-g", "15", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", out}
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stderr = os.Stderr
	check(cmd.Run())
}

type decoder struct {
	cmd    *exec.Cmd
	out    io.ReadCloser
	stderr *bytes.Buffer
	w, h   int
}

func startDecoder(ffmpeg, src, vf string, w, h int, sws bool) *decoder {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats"}
	if sws {
		args = append(args, "-sws_flags", swsFlags)
	}
	args = append(args, "-i", src, "-an")
	if vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args, "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	cmd := exec.Command(ffmpeg, args...)
	d := &decoder{cmd: cmd, stderr: &bytes.Buffer{}, w: w, h: h}
	cmd.Stderr = d.stderr
	var err error
	d.out, err = cmd.StdoutPipe()
	check(err)
	check(cmd.Start())
	return d
}

func doRun(ffmpeg, ffprobe, srcP, srcL, out, cs, enc string, bands int, approx bool, hashFile string) Result {
	r := Result{Case: cs, Encoder: enc, Bands: bands}
	if cs == "c4" {
		r.Sampler = "BiLinear"
		if approx {
			r.Sampler = "ApproxBiLinear"
		}
	}
	comp := newComposer(cs, bands, approx)

	// decoders
	var bgSrc, bgVF, pipVF string
	bgSrc = srcP
	pipW, pipH := 0, 0
	switch cs {
	case "c1", "c3", "c4", "c5":
	case "c2":
		bgSrc = srcL
		bgVF = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", W, H, W, H)
	default:
		fatal("bad case")
	}
	switch cs {
	case "c3":
		pipW, pipH, pipVF = PipW, PipH, fmt.Sprintf("scale=%d:%d", PipW, PipH)
	case "c4":
		pipW, pipH = LandW, LandH
	}

	// encoder
	eargs := []string{"-hide_banner", "-loglevel", "error", "-nostats", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", W, H), "-framerate", "30", "-i", "pipe:0"}
	switch enc {
	case "null":
		eargs = append(eargs, "-f", "null", "-")
	case "nullconv":
		eargs = append(eargs, "-pix_fmt", "yuv420p", "-f", "null", "-")
	default:
		eargs = append(eargs, "-i", srcP, "-map", "0:v", "-map", "1:a",
			"-c:v", enc, "-b:v", "8M", "-g", "60", "-pix_fmt", "yuv420p", "-r", "30",
			"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", out)
	}

	start := time.Now()
	bgDec := startDecoder(ffmpeg, bgSrc, bgVF, W, H, cs == "c2" || allSws)
	if cs == "c2" {
		bgDec.w, bgDec.h = W, H
	}
	var pipDec *decoder
	if pipW > 0 {
		src := srcL
		pipDec = startDecoder(ffmpeg, src, pipVF, pipW, pipH, cs == "c3" || allSws)
	}
	encCmd := exec.Command(ffmpeg, eargs...)
	var encErr bytes.Buffer
	encCmd.Stderr = &encErr
	stdin, err := encCmd.StdinPipe()
	check(err)
	check(encCmd.Start())

	var readNs, composeNs, writeNs int64 // each updated by one goroutine, read after join
	var errMu sync.Mutex
	var firstErr string
	fail := func(s string) {
		errMu.Lock()
		if firstErr == "" {
			firstErr = s
		}
		errMu.Unlock()
		// kill everything so goroutines unblock
		bgDec.cmd.Process.Kill()
		if pipDec != nil {
			pipDec.cmd.Process.Kill()
		}
		encCmd.Process.Kill()
	}

	type readerOut struct {
		ch   chan []byte
		free chan []byte
	}
	newReader := func(d *decoder, ns *int64) *readerOut {
		ro := &readerOut{ch: make(chan []byte, 3), free: make(chan []byte, 4)}
		for i := 0; i < 4; i++ {
			ro.free <- make([]byte, d.w*d.h*4)
		}
		go func() {
			defer close(ro.ch)
			br := bufio.NewReaderSize(d.out, 1<<20)
			for n := 0; n < Frames; n++ {
				buf := <-ro.free
				t := time.Now()
				_, err := io.ReadFull(br, buf)
				*ns += int64(time.Since(t))
				if err != nil {
					fail(fmt.Sprintf("decoder read frame %d: %v: %s", n, err, strings.TrimSpace(d.stderr.String())))
					return
				}
				ro.ch <- buf
			}
		}()
		return ro
	}
	var pipReadNs int64
	bgR := newReader(bgDec, &readNs)
	var pipR *readerOut
	if pipDec != nil {
		pipR = newReader(pipDec, &pipReadNs)
	}

	outFree := make(chan []byte, 4)
	outFull := make(chan []byte, 3)
	for i := 0; i < 4; i++ {
		outFree <- make([]byte, W*H*4)
	}
	go func() {
		defer close(outFull)
		for n := 0; n < Frames; n++ {
			bg, ok := <-bgR.ch
			if !ok {
				return
			}
			var pip []byte
			if pipR != nil {
				if pip, ok = <-pipR.ch; !ok {
					return
				}
			}
			o := <-outFree
			t := time.Now()
			comp.compose(n, bg, pip, o)
			composeNs += int64(time.Since(t))
			bgR.free <- bg
			if pipR != nil {
				pipR.free <- pip
			}
			outFull <- o
		}
	}()

	var hs [][32]byte
	if hashFile != "" {
		hs = make([][32]byte, 0, Frames)
	}
	written := 0
	for o := range outFull {
		if hashFile != "" {
			hs = append(hs, sha256.Sum256(o))
		}
		t := time.Now()
		_, err := stdin.Write(o)
		writeNs += int64(time.Since(t))
		if err != nil {
			fail("encoder pipe write: " + err.Error())
			break
		}
		written++
		outFree <- o
	}
	stdin.Close()
	werr := encCmd.Wait()
	wall := time.Since(start)
	bgDec.cmd.Process.Kill()
	bgDec.cmd.Wait()
	if pipDec != nil {
		pipDec.cmd.Process.Kill()
		pipDec.cmd.Wait()
	}

	r.WallSec = wall.Seconds()
	r.ReadSec = float64(readNs+pipReadNs) / 1e9
	r.ComposeSec = float64(composeNs) / 1e9
	r.WriteSec = float64(writeNs) / 1e9
	r.PeakRSSMB = float64(peakWorkingSet()) / (1 << 20)
	r.FPS = float64(written) / r.WallSec
	r.Realtime = Seconds / r.WallSec
	errMu.Lock()
	r.Error = firstErr
	errMu.Unlock()
	if werr != nil {
		r.Error = strings.TrimSpace(r.Error + " ffmpeg encoder: " + werr.Error() + " " + encErr.String())
	}
	if written != Frames && r.Error == "" {
		r.Error = fmt.Sprintf("wrote %d frames", written)
	}
	if s := strings.TrimSpace(encErr.String()); s != "" && r.Error == "" {
		r.Problems = append(r.Problems, "stderr: "+s)
	}
	if hashFile != "" {
		var sb strings.Builder
		h := sha256.New()
		for _, x := range hs {
			sb.WriteString(hex.EncodeToString(x[:]) + "\n")
			h.Write(x[:])
		}
		check(os.WriteFile(hashFile, []byte(sb.String()), 0o644))
		r.HashSum = hex.EncodeToString(h.Sum(nil))
	}
	if r.Error == "" && enc != "null" && enc != "nullconv" {
		if st, err := os.Stat(out); err == nil {
			r.SizeBytes = st.Size()
		}
		probs := validate(ffprobe, out)
		ok := true
		for _, p := range probs {
			if !strings.HasPrefix(p, "stderr:") {
				ok = false
			}
		}
		r.Problems = append(r.Problems, probs...)
		r.Valid = &ok
	}
	return r
}

// ---- validation (same checks as the export-pipeline spike) ----

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
