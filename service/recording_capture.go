package service

import (
	"context"

	"github.com/AlexGalhardo/stuarty/engine/capturehost"
)

// Capturer is the native capture helper as the service sees it (SDD-025 section 7). The product's one
// implementation is capturehost.Live; a test supplies a fake. The service never starts a helper without having
// checked that the caller is a person first.
type Capturer interface {
	// Sources lists the displays and windows that can be captured.
	Sources(ctx context.Context) (capturehost.Sources, error)
	// Start starts the helper and the recording and returns once the first frame is in. ctx bounds the start
	// only: the recording lives until it ends, is stopped or is killed.
	Start(ctx context.Context, req CaptureStart) (CaptureSession, error)
	// Inspect reads a recorded file and reports what is decodable in it.
	Inspect(ctx context.Context, path string) (CaptureInfo, error)
}

// CaptureStart is what the service asks the helper to record.
type CaptureStart struct {
	RecordingID string
	Kind        string
	SourceID    string
	FPS         int
	// Output is the absolute path of the .mp4 file.
	Output string
	// MaxSeconds is the longest the recording may run; it sets the deadline of the helper process.
	MaxSeconds int

	// The tracks beyond the screen and the file of each, in the folder of Output (SDD-025 section 16).
	Microphone       string
	MicrophoneOutput string
	SystemAudio      bool
	SystemOutput     string
	Camera           string
	CameraOutput     string
	Events           bool
	EventsOutput     string
}

// CaptureSession is one running recording.
type CaptureSession interface {
	Started() capturehost.Started
	// Encoder is the FFmpeg name of the encoder the helper was told to use.
	Encoder() string
	// Events ends with one terminal event: Finished, Failed or Lost.
	Events() <-chan capturehost.Event
	// Stop asks for a graceful end.
	Stop() error
	// Pause asks every source to stop writing; Resume asks them to write again.
	Pause() error
	Resume() error
	// Kill ends the helper and its process tree at once.
	Kill()
	// Close releases the session after its events ended.
	Close() error
}

// CaptureInfo is what a recorded file holds.
type CaptureInfo struct {
	DurationMS    int64
	Width, Height int
}

// NewLiveCapturer makes the Capturer of the product over capturehost.Live.
func NewLiveCapturer(l *capturehost.Live) Capturer { return liveCapturer{l} }

type liveCapturer struct{ l *capturehost.Live }

func (a liveCapturer) Sources(ctx context.Context) (capturehost.Sources, error) {
	return a.l.Sources(ctx)
}

func (a liveCapturer) Start(ctx context.Context, r CaptureStart) (CaptureSession, error) {
	s, err := a.l.Begin(ctx, capturehost.StartParams{
		RecordingID: r.RecordingID, Kind: r.Kind, SourceID: r.SourceID, FPS: r.FPS, Output: r.Output,
		MaxSeconds: r.MaxSeconds, Microphone: r.Microphone, MicrophoneOutput: r.MicrophoneOutput,
		SystemAudio: r.SystemAudio, SystemOutput: r.SystemOutput, Camera: r.Camera, CameraOutput: r.CameraOutput,
		Events: r.Events, EventsOutput: r.EventsOutput,
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (a liveCapturer) Inspect(ctx context.Context, path string) (CaptureInfo, error) {
	i, err := a.l.Inspect(ctx, path)

	return CaptureInfo{DurationMS: i.DurationMS, Width: i.Width, Height: i.Height}, err
}

// CaptureIndicator is what a Capturer that keeps the indicator of SDD-025 section 19.4 offers: whether a helper that
// was asked to capture is alive. It is driven by the host, never by a recording row, a window or a page.
type CaptureIndicator interface {
	CaptureActivity() capturehost.Activity
}

// CaptureActivity is the indicator of the live capturer.
func (a liveCapturer) CaptureActivity() capturehost.Activity { return a.l.Indicator.State() }

// CaptureActivityOf is the indicator of a Service made by New, for the desktop shell to draw (SDD-025 section
// 19.4). A service with no capture, or a test one with no indicator, says no helper is alive. The second result is
// false for a Service that is not a server.
func CaptureActivityOf(s Service) (capturehost.Activity, bool) {
	v, ok := s.(*svc)
	if !ok {
		return capturehost.Activity{}, false
	}

	if ind, has := v.capture.(CaptureIndicator); has {
		return ind.CaptureActivity(), true
	}

	return capturehost.Activity{}, true
}
