package aihost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// FrameSide is the side of the square frame the YuNet model of the catalogue reads (SDD-024 section 5).
const FrameSide = 640

// ModelName is the name a track carries for the model of the catalogue.
const ModelName = "yunet-2023mar"

// Providers a caller may ask for.
const (
	ProviderAuto     = "auto"
	ProviderCPU      = "cpu"
	ProviderDirectML = "directml"
)

// Config says what to run. Every path is absolute and was verified against tools/installed.json by the
// caller just before (SDD-007 section 6.11 item 2); this package does not look inside them.
type Config struct {
	Runner  *ffmpeg.Runner
	Helper  string
	Runtime string
	Model   string
	// Provider is ProviderAuto when empty.
	Provider string
	// Threads is the number of threads the runtime uses on the CPU; 0 lets it choose.
	Threads int
	// StallTimeout kills a run that wrote no line for this long; 120 s when zero.
	StallTimeout time.Duration
}

// Host runs the helper.
type Host struct{ cfg Config }

// New makes a host. It does not start anything.
func New(cfg Config) (*Host, error) {
	if cfg.Runner == nil {
		return nil, errors.New("aihost: a runner is required")
	}

	for _, p := range []string{cfg.Helper, cfg.Runtime, cfg.Model} {
		if !filepath.IsAbs(p) {
			return nil, errors.New("aihost: the helper, the runtime and the model must be absolute paths")
		}
	}

	if cfg.Provider == "" {
		cfg.Provider = ProviderAuto
	}

	if cfg.Provider != ProviderAuto && cfg.Provider != ProviderCPU && cfg.Provider != ProviderDirectML {
		return nil, fmt.Errorf("aihost: unknown provider %q", cfg.Provider)
	}

	if cfg.StallTimeout <= 0 {
		cfg.StallTimeout = 120 * time.Second
	}

	return &Host{cfg: cfg}, nil
}

// Input is one range of one video to analyse.
type Input struct {
	// Source is the video file, an absolute path inside one of InputDirs.
	Source    string
	InputDirs []string
	// WorkDir is the run directory of the job.
	WorkDir *ffmpeg.RunDir
	// Width and Height are the pixels of the source after rotation.
	Width, Height int
	// StartMS and EndMS are the range on the asset's time axis; EndMS is after StartMS.
	StartMS, EndMS int64
	// FPS is the frames analysed per second, 1 to 10.
	FPS int
}

// Validate checks what Detect relies on.
func (in *Input) Validate() error {
	switch {
	case in.WorkDir == nil || in.Source == "":
		return errors.New("aihost: a source and a run directory are required")
	case in.Width < 1 || in.Height < 1 || in.Width > facetrack.MaxSide || in.Height > facetrack.MaxSide:
		return errors.New("aihost: the source size is outside the limits")
	case in.FPS < 1 || in.FPS > 10:
		return errors.New("aihost: fps is from 1 to 10")
	case in.StartMS < 0 || in.EndMS <= in.StartMS:
		return errors.New("aihost: the range is empty")
	}

	return nil
}

// Frames is the number of frames the range holds at its fps, which is the most the helper may report.
func (in *Input) Frames() int {
	return int((in.EndMS-in.StartMS)*int64(in.FPS)+999) / 1000
}

// Deadline bounds a run: the range divided by two and a minute of start-up, at most two hours (SDD-024
// section 10).
func Deadline(in *Input) time.Duration {
	d := time.Duration(in.EndMS-in.StartMS)*time.Millisecond/2 + time.Minute

	return min(d, 2*time.Hour)
}

// ffmpegArgv is the FFmpeg command that writes the analysis frames: the range, sampled at fps, scaled to
// fit the square the model reads and padded at the right and the bottom, as RGB24 on a pipe.
func ffmpegArgv(r *ffmpeg.Runner, in *Input) ([]string, error) {
	vf := ffargs.Chain1(
		ffargs.Filter("fps="+strconv.Itoa(in.FPS)+"/1"),
		ffargs.Filter(fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", FrameSide, FrameSide)),
		ffargs.Filter(fmt.Sprintf("pad=%d:%d:0:0", FrameSide, FrameSide)),
	)

	secs := strconv.FormatFloat(float64(in.EndMS-in.StartMS)/1000, 'f', 3, 64)

	return ffargs.Cmd{
		LogErrorOnly: true,
		Threads:      r.In.ThreadLimit(),
		Inputs:       []ffargs.Input{ffargs.FileInput(in.Source).At(in.StartMS)},
		Outputs: []ffargs.Output{{
			Format: "rawvideo", Pipe: true, Maps: []string{"0:v:0"}, NoAudio: true, NoSubs: true, NoData: true, VF: vf,
			Opts: []ffargs.Opt{ffargs.Enum("-pix_fmt", "rgb24"), ffargs.Enum("-t", secs)},
		}},
	}.Argv(r.Policy(in.WorkDir, in.InputDirs...))
}

// sink reads the helper's standard output line by line, with a cap on a line and on the number of frames.
type sink struct {
	mu       sync.Mutex
	buf      []byte
	maxFrame int
	onFrame  func()
	last     time.Time

	ready    bool
	provider string
	done     bool
	frames   map[int][]pixelFace
	failure  *Error
	count    int
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.last = time.Now()

	s.buf = append(s.buf, p...)

	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			break
		}

		line := bytes.TrimSpace(s.buf[:i])
		s.buf = s.buf[i+1:]

		if len(line) > 0 {
			s.accept(line)
		}
	}

	if len(s.buf) > MaxLine {
		s.fail(protocolError("a line of the helper is longer than %d bytes", MaxLine))
		s.buf = nil
	}

	if s.failure != nil {
		return 0, s.failure
	}

	return len(p), nil
}

func (s *sink) fail(e *Error) {
	if s.failure == nil {
		s.failure = e
	}
}

func (s *sink) accept(b []byte) {
	if len(b) > MaxLine {
		s.fail(protocolError("a line of the helper is longer than %d bytes", MaxLine))

		return
	}

	l, faces, err := parseLine(b)
	if err != nil {
		if pe, ok := errors.AsType[*Error](err); ok {
			s.fail(pe)
		}

		return
	}

	switch l.Type {
	case "ready":
		if l.Protocol != Protocol || l.Width != FrameSide || l.Height != FrameSide {
			s.fail(protocolError("the helper speaks protocol %d at %dx%d", l.Protocol, l.Width, l.Height))

			return
		}

		s.ready, s.provider = true, l.Provider
	case "frame":
		if !s.ready {
			s.fail(protocolError("a frame came before the helper was ready"))

			return
		}

		if s.count >= s.maxFrame {
			s.fail(protocolError("the helper reported more frames than were sent"))

			return
		}

		if _, dup := s.frames[*l.Index]; dup || *l.Index >= s.maxFrame {
			s.fail(protocolError("a frame index is repeated or out of range"))

			return
		}

		s.frames[*l.Index] = faces
		s.count++

		if s.onFrame != nil {
			s.onFrame()
		}
	case "done":
		s.done = true
	case "error":
		reason := l.Code
		if !helperCodes[reason] {
			reason = "protocol"
		}

		s.fail(&Error{Reason: reason, Message: printable(l.Message)})
	default:
		s.fail(protocolError("an unknown line type"))
	}
}

// printable keeps a short, plain cut of a message: the helper is trusted to carry no path, and the host does
// not rely on it.
func printable(s string) string {
	out := make([]rune, 0, 160)

	for _, r := range s {
		if len(out) >= 160 {
			break
		}

		if r >= 0x20 && r != 0x7f {
			out = append(out, r)
		}
	}

	return string(out)
}

// Detect runs the helper over the range and returns the track. progress, when not nil, is called with the
// frames done and the frames expected. A cancelled ctx kills both children.
func (h *Host) Detect(ctx context.Context, in *Input, progress func(done, total int)) (facetrack.Track, error) {
	if err := in.Validate(); err != nil {
		return facetrack.Track{}, err
	}

	deadline := Deadline(in)

	tool, err := h.cfg.Runner.HelperTool(ctx, h.cfg.Helper, deadline)
	if err != nil {
		return facetrack.Track{}, &Error{Reason: "start", Message: "the AI helper could not be started"}
	}

	argv, err := ffmpegArgv(h.cfg.Runner, in)
	if err != nil {
		return facetrack.Track{}, &Error{Reason: "decoder", Message: "the decoder command was refused"}
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return facetrack.Track{}, &Error{Reason: "start", Message: "a pipe could not be made"}
	}

	total := in.Frames()
	out := &sink{maxFrame: total, frames: map[int][]pixelFace{}, last: time.Now()}

	if progress != nil {
		out.onFrame = func() { progress(out.count, total) }
	}

	rctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		ffErr error
		ffRes ffmpeg.Result
		wg    sync.WaitGroup
	)

	wg.Go(func() {
		ffRes, ffErr = h.cfg.Runner.RunFFmpeg(rctx, ffmpeg.Request{
			Dir: in.WorkDir, Argv: argv, InputDirs: in.InputDirs, Deadline: deadline, Stdout: pw,
		})
		_ = pw.Close()
	})

	stop := watchStall(rctx, out, h.cfg.StallTimeout, cancel)

	res, hErr := h.cfg.Runner.RunHelper(rctx, ffmpeg.HelperRequest{
		Tool: tool, Dir: in.WorkDir, Deadline: deadline, Stdin: pr, Stdout: out,
		Argv: []string{
			"faces", "--runtime", h.cfg.Runtime, "--model", h.cfg.Model,
			"--width", strconv.Itoa(FrameSide), "--height", strconv.Itoa(FrameSide), "--provider", h.cfg.Provider,
			"--threads", strconv.Itoa(h.cfg.Threads),
		},
	})

	_ = pr.Close()

	cancel()
	wg.Wait()
	stop()

	return h.judge(ctx, in, out, res, hErr, ffRes, ffErr)
}

// watchStall cancels a run that writes nothing for stall.
func watchStall(ctx context.Context, s *sink, stall time.Duration, cancel context.CancelFunc) (stop func()) {
	t := time.NewTicker(max(stall/4, 10*time.Millisecond))
	quit := make(chan struct{})

	go func() {
		defer t.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				s.mu.Lock()
				idle := time.Since(s.last)
				s.mu.Unlock()

				if idle > stall {
					s.mu.Lock()
					s.fail(&Error{Reason: "timeout", Message: "the helper wrote nothing for too long"})
					s.mu.Unlock()
					cancel()

					return
				}
			}
		}
	}()

	return func() { close(quit) }
}

func (h *Host) judge(
	ctx context.Context, in *Input, out *sink, res ffmpeg.Result, hErr error, ffRes ffmpeg.Result, ffErr error,
) (facetrack.Track, error) {
	out.mu.Lock()
	defer out.mu.Unlock()

	if ctx.Err() != nil {
		return facetrack.Track{}, ctx.Err()
	}

	if out.failure != nil {
		return facetrack.Track{}, out.failure
	}

	if errors.Is(hErr, ffmpeg.ErrDeadline) || errors.Is(ffErr, ffmpeg.ErrDeadline) {
		return facetrack.Track{}, &Error{Reason: "timeout", Message: "the analysis took too long and was ended"}
	}

	if ffErr != nil && !errors.Is(ffErr, context.Canceled) {
		return facetrack.Track{}, &Error{Reason: "decoder", Message: printable(tail(ffRes.Stderr))}
	}

	if hErr != nil || res.ExitCode != 0 || !out.done {
		return facetrack.Track{}, &Error{Reason: "exit", Message: printable(tail(res.Stderr))}
	}

	return buildTrack(in, out.provider, out.frames), nil
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}

	return s
}

// scaled is the size of the source after the scale of the analysis frame, as FFmpeg computes it.
func scaled(w, h int) (sw, sh int) {
	if w >= h {
		return FrameSide, max((FrameSide*h+w/2)/w, 1)
	}

	return max((FrameSide*w+h/2)/h, 1), FrameSide
}

// buildTrack maps the boxes of the padded frames back to ten-thousandths of the source frame, in integers,
// and drops a box whose centre is in the padding.
func buildTrack(in *Input, provider string, frames map[int][]pixelFace) facetrack.Track {
	sw, sh := scaled(in.Width, in.Height)

	t := facetrack.Track{
		Version: facetrack.Version, Model: ModelName, Provider: provider,
		Source: facetrack.Size{Width: in.Width, Height: in.Height}, FPS: in.FPS,
		StartMS: int(in.StartMS), EndMS: int(in.EndMS), Samples: make([]facetrack.Sample, 0, len(frames)),
	}

	for i := range in.Frames() {
		faces, ok := frames[i]
		if !ok {
			continue
		}

		s := facetrack.Sample{T: int(in.StartMS + int64(i)*1000/int64(in.FPS)), Faces: []facetrack.Face{}}

		for _, f := range faces {
			if cx, cy := f.X+f.W/2, f.Y+f.H/2; cx < 0 || cy < 0 || cx > sw*10 || cy > sh*10 {
				continue
			}

			s.Faces = append(s.Faces, mapFace(f, sw, sh))
		}

		t.Samples = append(t.Samples, s)
	}

	return t
}

func mapFace(f pixelFace, sw, sh int) facetrack.Face {
	x1, y1 := clampUnit(unit(f.X, sw)), clampUnit(unit(f.Y, sh))
	x2, y2 := clampUnit(unit(f.X+f.W, sw)), clampUnit(unit(f.Y+f.H, sh))

	return facetrack.Face{X: x1, Y: y1, W: max(x2-x1, 1), H: max(y2-y1, 1), Score: f.Score}
}

// unit converts tenths of a pixel to ten-thousandths of a side of size px pixels, rounded.
func unit(tenths, px int) int {
	return (tenths*facetrack.Unit + px*5) / (px * 10)
}

func clampUnit(v int) int { return max(0, min(v, facetrack.Unit-1)) }

// Probe runs `probe` and returns whether the runtime loads and offers DirectML; used by doctor.
func (h *Host) Probe(ctx context.Context, dir *ffmpeg.RunDir) (directml bool, err error) {
	tool, terr := h.cfg.Runner.HelperTool(ctx, h.cfg.Helper, time.Minute)
	if terr != nil {
		return false, &Error{Reason: "start", Message: "the AI helper could not be started"}
	}

	var buf bytes.Buffer

	if _, rerr := h.cfg.Runner.RunHelper(ctx, ffmpeg.HelperRequest{
		Tool: tool, Dir: dir, Deadline: time.Minute, Argv: []string{"probe", "--runtime", h.cfg.Runtime},
		Stdout: &limitedBuffer{buf: &buf, max: MaxLine},
	}); rerr != nil {
		return false, &Error{Reason: "exit", Message: "the runtime could not be probed"}
	}

	var l struct {
		Type     string `json:"type"`
		Protocol int    `json:"protocol"`
		DirectML bool   `json:"directml"`
		Code     string `json:"code"`
	}

	if json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &l) != nil || l.Type != "probe" || l.Protocol != Protocol {
		return false, protocolError("the probe answered something else")
	}

	return l.DirectML, nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		return 0, io.ErrShortWrite
	}

	return l.buf.Write(p)
}
