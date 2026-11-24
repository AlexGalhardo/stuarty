package capturehost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// Live is the product's capture: the helper at HelperPath, the encoder chosen by the capability check of an
// export (engine/ffmpeg), and the FFmpeg the server discovered. It implements service.Capturer.
type Live struct {
	Runner *ffmpeg.Runner
	// Detect is the cached encoder detection of the export (engine/ffmpeg/encoders.go).
	Detect func(ctx context.Context) (*ffmpeg.EncoderDetection, error)
	// HelperPath is the absolute path of stuarty-capture.
	HelperPath string
	// FFmpegPath is the absolute path of the FFmpeg the helper starts.
	FFmpegPath string
	// Silence and the other timeouts are for tests; zero takes the defaults.
	Config Config
}

// ErrHelperMissing means the helper binary is not where the server looks for it.
var ErrHelperMissing = errors.New("capturehost: the capture helper is not installed")

func (l *Live) check() error {
	if l.HelperPath == "" || !filepath.IsAbs(l.HelperPath) {
		return fmt.Errorf("%w: no absolute path", ErrHelperMissing)
	}

	if st, err := os.Stat(l.HelperPath); err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrHelperMissing, filepath.Base(l.HelperPath))
	}

	return nil
}

func (l *Live) start(ctx context.Context, name string, deadline time.Duration) (*Helper, *ffmpeg.RunDir, error) {
	if err := l.check(); err != nil {
		return nil, nil, err
	}

	dir, err := l.Runner.NewRunDir(name)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	cfg := l.Config
	cfg.Path, cfg.Dir, cfg.Deadline = l.HelperPath, dir, deadline

	h, err := Start(ctx, l.Runner, cfg)
	if err != nil {
		_ = dir.Remove()

		return nil, nil, err
	}

	return h, dir, nil
}

// Sources lists the displays and windows the helper can capture.
func (l *Live) Sources(ctx context.Context) (Sources, error) {
	h, dir, err := l.start(ctx, "sources_"+strconv.FormatInt(time.Now().UnixNano(), 36), time.Minute)
	if err != nil {
		return Sources{}, err
	}

	defer func() { _ = dir.Remove() }()
	defer func() { _ = h.Close() }()

	return h.Sources(ctx)
}

// Session is one running recording of Live.
type Session struct {
	h       *Helper
	dir     *ffmpeg.RunDir
	started Started
	encoder string
	events  <-chan Event
}

// Started is what the helper reported.
func (s *Session) Started() Started { return s.started }

// Encoder is the encoder the helper was told to use.
func (s *Session) Encoder() string { return s.encoder }

// Events is the stream of the recording.
func (s *Session) Events() <-chan Event { return s.events }

// Stop asks for a graceful end.
func (s *Session) Stop() error { return s.h.Stop() }

// Kill ends the helper and its process tree.
func (s *Session) Kill() { s.h.Kill() }

// Close releases the helper and the run directory.
func (s *Session) Close() error {
	err := s.h.Close()
	_ = s.dir.Remove()

	return err
}

// StartParams is what Live.Start needs; it mirrors service.CaptureStart without importing the service.
type StartParams struct {
	RecordingID string
	Kind        string
	SourceID    string
	FPS         int
	Output      string
	MaxSeconds  int
}

// Begin starts the helper and the recording. It picks the encoder with the capability test of an export at
// 1920 by 1080 and the requested rate: the size of the source is not known before the helper has it.
func (l *Live) Begin(ctx context.Context, p StartParams) (*Session, error) {
	if l.Detect == nil || l.FFmpegPath == "" {
		return nil, fmt.Errorf("%w: no FFmpeg", ErrUnavailable)
	}

	det, err := l.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	cand, err := l.Runner.EncoderForSize(
		ctx,
		det,
		ffmpeg.SizeProbe{Width: 1920, Height: 1080, FPSNum: p.FPS, FPSDen: 1},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	h, dir, err := l.start(context.WithoutCancel(ctx), p.RecordingID, time.Duration(p.MaxSeconds+120)*time.Second)
	if err != nil {
		return nil, err
	}

	started, events, err := h.Begin(ctx, StartRequest{
		Recording: p.RecordingID, Kind: p.Kind, SourceID: p.SourceID, FPS: p.FPS, Encoder: cand.Name,
		FFmpeg: l.FFmpegPath, Output: p.Output,
	})
	if err != nil {
		h.Kill()
		_ = h.Close()
		_ = dir.Remove()

		return nil, err
	}

	return &Session{h: h, dir: dir, started: started, encoder: cand.Name, events: events}, nil
}

// Info is what a recorded file holds.
type Info struct {
	DurationMS    int64
	Width, Height int
}

type probeOut struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// Inspect reads a recorded file with ffprobe under the process policy and reports its duration and size. A file
// with no video stream or no duration is an error.
func (l *Live) Inspect(ctx context.Context, path string) (Info, error) {
	dir, err := l.Runner.NewRunDir("inspect_" + strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		return Info{}, err
	}

	defer func() { _ = dir.Remove() }()

	// The folder of the recording is the one input directory this run may read from (FF-03).
	folder := filepath.Dir(path)

	argv, err := ffargs.ProbeJSON(l.Runner.Policy(dir, folder), path)
	if err != nil {
		return Info{}, err
	}

	out, _, err := l.Runner.RunFFprobe(ctx, dir, argv, time.Minute, folder)
	if err != nil {
		return Info{}, err
	}

	var p probeOut
	if err := json.Unmarshal(out, &p); err != nil {
		return Info{}, err
	}

	secs, err := strconv.ParseFloat(p.Format.Duration, 64)
	if err != nil || secs <= 0 {
		return Info{}, errors.New("capturehost: the file has no duration")
	}

	for _, s := range p.Streams {
		if s.CodecType == "video" {
			return Info{DurationMS: int64(secs * 1000), Width: s.Width, Height: s.Height}, nil
		}
	}

	return Info{}, errors.New("capturehost: the file has no video stream")
}
