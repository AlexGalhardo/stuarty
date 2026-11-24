// Package capturehost drives the native capture helper stuarty-capture (ADR-0049, SDD-025). The helper
// captures the screen and writes a fragmented MP4 through an FFmpeg child of its own, and the sound, the camera
// and the pointer to files of their own on one clock; this package starts it under the process policy of FFmpeg
// (SDD-007 6.10 and 6.11: absolute path, minimal environment, a run directory, a deadline, the kill of the
// whole process tree) and speaks version 2 of its JSON protocol (SDD-025 sections 6 and 16.7). It decides
// nothing about a recording: the rules are in core/recording.
package capturehost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// ProtocolVersion is the version of the protocol this host speaks.
const ProtocolVersion = 2

const (
	maxLine        = 65536
	defaultReady   = 10 * time.Second
	defaultStarted = 15 * time.Second
	defaultSilence = 10 * time.Second
	endGrace       = 3 * time.Second
)

// Encoders is the closed list of encoders the helper builds an FFmpeg command for.
var Encoders = []string{"h264_nvenc", "h264_qsv", "h264_amf", "h264_mf", "libopenh264"}

// FrameRates is the closed set of frame rates.
var FrameRates = []int{24, 30, 60}

// Errors of the host. Each wraps the cause where there is one.
var (
	// ErrUnavailable means the helper could not be started or did not speak the protocol.
	ErrUnavailable = errors.New("capturehost: the capture helper is not available")
	// ErrProtocol means the helper sent a malformed, oversized or unknown message.
	ErrProtocol = errors.New("capturehost: the helper broke the protocol")
	// ErrLost means the helper ended without finishing, or went silent.
	ErrLost = errors.New("capturehost: the helper ended or went silent")
	// ErrBadRequest means the request was refused before anything was sent.
	ErrBadRequest = errors.New("capturehost: bad request")
	// ErrProtocolVersion means the helper answered with another version of the protocol; it is also
	// ErrUnavailable.
	ErrProtocolVersion = fmt.Errorf("%w: the helper speaks another protocol version", ErrUnavailable)
)

// HelperError is an error message of the helper (code and a printable message).
type HelperError struct {
	Code    string
	Message string
}

func (e *HelperError) Error() string {
	return "capturehost: the helper reported " + e.Code + ": " + e.Message
}

// Config is what Start needs.
type Config struct {
	// Path is the absolute path of stuarty-capture.
	Path string
	// Dir is the run directory: the working directory, HOME and temp directory of the child.
	Dir *ffmpeg.RunDir
	// Deadline bounds the life of the child.
	Deadline time.Duration
	// Zero values take the defaults of the constants above.
	ReadyTimeout, StartedTimeout, Silence time.Duration
}

// Source is a display or a window the helper can capture.
type Source struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Device is a microphone or a camera the helper lists. The name is untrusted text.
type Device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// Sources is the answer to a sources command.
type Sources struct {
	Displays    []Source `json:"displays"`
	Windows     []Source `json:"windows"`
	Microphones []Device `json:"microphones"`
	Cameras     []Device `json:"cameras"`
	SystemAudio bool     `json:"system_audio"`
}

// StartRequest is what the host asks the helper to record. A track beyond the screen is asked for with its
// device (or SystemAudio, or Events) and needs its Output path, in the folder of Output.
type StartRequest struct {
	Recording string
	Kind      string
	SourceID  string
	FPS       int
	Encoder   string
	FFmpeg    string
	Output    string

	Microphone       string
	MicrophoneOutput string
	SystemAudio      bool
	SystemOutput     string
	Camera           string
	CameraOutput     string
	Events           bool
	EventsOutput     string
}

// TrackStart is a track that is recording.
type TrackStart struct {
	Kind    string `json:"kind"`
	StartMS int64  `json:"start_ms"`
}

// TrackResult is a track at the end of the recording.
type TrackResult struct {
	Kind       string `json:"kind"`
	StartMS    int64  `json:"start_ms"`
	DurationMS int64  `json:"duration_ms"`
	Bytes      int64  `json:"bytes"`
	Lost       bool   `json:"lost"`
}

// PauseSpan is a pause on the helper's wall clock, in ms since the origin.
type PauseSpan struct {
	FromMS int64 `json:"from_ms"`
	ToMS   int64 `json:"to_ms"`
}

// Started is what the helper says when the first frame is in.
type Started struct {
	Width, Height, FPS int
	Tracks             []TrackStart
}

// Event is one thing that happens during a recording: a Progress, a Level, a Paused, a Resumed, a Rejected, or
// the terminal Finished, Failed or Lost.
type Event interface{ isEvent() }

// Progress is sent about once a second, paused or not. ElapsedMS is active time.
type Progress struct{ ElapsedMS, Frames, Dropped, Bytes int64 }

// Level is the peak of each sound track over the last interval, 0 to 1000; a track the recording has not has 0.
type Level struct{ Microphone, System int }

// Paused confirms a pause; ElapsedMS is active time.
type Paused struct{ ElapsedMS int64 }

// Resumed confirms a resume; ElapsedMS is active time.
type Resumed struct{ ElapsedMS int64 }

// Rejected is a command the helper refused without ending the recording (a pause that was not possible).
type Rejected struct{ Message string }

// Finished ends a recording whose files are complete. Reason is stopped or source_closed.
type Finished struct {
	Reason                             string
	Frames, Dropped, DurationMS, Bytes int64
	Tracks                             []TrackResult
	Pauses                             []PauseSpan
	PointerEvents                      int64
}

// Failed is an error message of the helper; the stream ends after it.
type Failed struct{ Code, Message string }

// Lost is the host's own end of the stream: the helper exited with no finished, went silent or broke the
// protocol. Err wraps ErrLost or ErrProtocol.
type Lost struct{ Err error }

func (Progress) isEvent() {}
func (Level) isEvent()    {}
func (Paused) isEvent()   {}
func (Resumed) isEvent()  {}
func (Rejected) isEvent() {}
func (Finished) isEvent() {}
func (Failed) isEvent()   {}
func (Lost) isEvent()     {}

// message is every field of every message of the helper, the ones a message lacks left empty.
type message struct {
	Type          string      `json:"type"`
	V             int         `json:"v"`
	Helper        string      `json:"helper"`
	Display       bool        `json:"display"`
	Window        bool        `json:"window"`
	Audio         bool        `json:"audio"`
	Camera        bool        `json:"camera"`
	Events        bool        `json:"events"`
	Displays      []Source    `json:"displays"`
	Windows       []Source    `json:"windows"`
	Microphones   []Device    `json:"microphones"`
	Cameras       []Device    `json:"cameras"`
	SystemAudio   bool        `json:"system_audio"`
	Width         int         `json:"width"`
	Height        int         `json:"height"`
	FPS           int         `json:"fps"`
	ElapsedMS     int64       `json:"elapsed_ms"`
	Frames        int64       `json:"frames"`
	Dropped       int64       `json:"dropped"`
	Bytes         int64       `json:"bytes"`
	DurationMS    int64       `json:"duration_ms"`
	Reason        string      `json:"reason"`
	Code          string      `json:"code"`
	Message       string      `json:"message"`
	Tracks        []trackMsg  `json:"tracks"`
	Pauses        []PauseSpan `json:"pauses"`
	PointerEvents int64       `json:"pointer_events"`
	Microphone    int         `json:"microphone"`
	System        int         `json:"system"`
}

// trackMsg is a member of the tracks list of a started or a finished message.
type trackMsg struct {
	Kind       string `json:"kind"`
	StartMS    int64  `json:"start_ms"`
	DurationMS int64  `json:"duration_ms"`
	Bytes      int64  `json:"bytes"`
	Lost       bool   `json:"lost"`
}

// Helper is a running stuarty-capture.
type Helper struct {
	cfg    Config
	in     *io.PipeWriter
	cancel context.CancelFunc
	done   chan struct{}
	msgs   chan msgOrErr

	wmu sync.Mutex

	mu     sync.Mutex
	runErr error
	stderr string

	// Platform of the helper: whether it can capture a display and a window, sound, a camera, the pointer.
	Display, Window, Audio, Camera, Events bool
}

type msgOrErr struct {
	m   message
	err error
}

// Start runs the helper and waits for its ready message. Any failure before that is ErrUnavailable.
func Start(ctx context.Context, r *ffmpeg.Runner, cfg Config) (*Helper, error) {
	if cfg.Dir == nil || cfg.Deadline <= 0 {
		return nil, fmt.Errorf("%w: a run directory and a deadline are required", ErrUnavailable)
	}

	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultReady
	}

	if cfg.StartedTimeout <= 0 {
		cfg.StartedTimeout = defaultStarted
	}

	if cfg.Silence <= 0 {
		cfg.Silence = defaultSilence
	}

	tool, err := r.HelperTool(ctx, cfg.Path, cfg.Deadline)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &Helper{cfg: cfg, in: inW, cancel: cancel, done: make(chan struct{}), msgs: make(chan msgOrErr, 64)}

	go func() {
		defer close(h.done)

		res, err := r.RunHelper(runCtx, ffmpeg.HelperRequest{
			Tool: tool, Dir: cfg.Dir, Deadline: cfg.Deadline, Stdin: inR,
			Stdout: &endWriter{w: outW, onEnd: func() { _ = inW.Close() }},
		})

		h.mu.Lock()
		h.runErr, h.stderr = err, res.Stderr
		h.mu.Unlock()

		_ = outW.CloseWithError(io.EOF)
		_ = inR.CloseWithError(io.ErrClosedPipe)
	}()

	go h.read(outR)

	ready, err := h.next(ctx, cfg.ReadyTimeout)
	if err != nil || ready.Type != "ready" || ready.V != ProtocolVersion {
		h.kill()

		switch {
		case err != nil:
			return nil, fmt.Errorf("%w: no ready message: %w", ErrUnavailable, err)
		case ready.Type == "ready":
			return nil, fmt.Errorf("%w: protocol version %d, want %d", ErrProtocolVersion, ready.V, ProtocolVersion)
		default:
			return nil, fmt.Errorf("%w: the first message was %q", ErrUnavailable, clean(ready.Type))
		}
	}

	h.Display, h.Window, h.Audio, h.Camera, h.Events = ready.Display, ready.Window, ready.Audio, ready.Camera,
		ready.Events

	return h, nil
}

// endWriter passes the child's standard output to the pipe and calls onEnd when that output ends, which is when
// the child has exited (or closed it).
type endWriter struct {
	w     *io.PipeWriter
	onEnd func()
}

func (e *endWriter) Write(p []byte) (int, error) { return e.w.Write(p) }

func (e *endWriter) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(struct{ io.Writer }{e.w}, r)
	e.onEnd()

	return n, err
}

// read turns the output of the child into messages. A line over the limit or a line that is not JSON is a
// protocol error and ends the stream.
func (h *Helper) read(r io.Reader) {
	defer close(h.msgs)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxLine)

	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m.Type == "" {
			h.msgs <- msgOrErr{err: fmt.Errorf("%w: a line that is not a message", ErrProtocol)}

			// Drain so the child is never blocked on its output while it is being killed.
			_, _ = io.Copy(io.Discard, r)

			return
		}

		h.msgs <- msgOrErr{m: m}
	}

	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		h.msgs <- msgOrErr{err: fmt.Errorf("%w: %w", ErrProtocol, err)}

		_, _ = io.Copy(io.Discard, r)
	}
}

// next waits for one message.
func (h *Helper) next(ctx context.Context, timeout time.Duration) (message, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case m, ok := <-h.msgs:
		if !ok {
			return message{}, h.ended()
		}

		return m.m, m.err
	case <-t.C:
		return message{}, fmt.Errorf("%w: no message in %s", ErrLost, timeout)
	case <-ctx.Done():
		return message{}, ctx.Err()
	}
}

// ended is the error for a stream that closed: the helper exited.
func (h *Helper) ended() error {
	select {
	case <-h.done:
	case <-time.After(endGrace):
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stderr != "" {
		return fmt.Errorf("%w: it exited (%s)", ErrLost, clean(lastLine(h.stderr)))
	}

	return fmt.Errorf("%w: it exited", ErrLost)
}

func (h *Helper) send(v map[string]any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	h.wmu.Lock()
	defer h.wmu.Unlock()

	_, err = h.in.Write(append(b, '\n'))

	return err
}

// Sources asks which displays and windows can be captured. The helper keeps running; call Close after.
func (h *Helper) Sources(ctx context.Context) (Sources, error) {
	if err := h.send(map[string]any{"type": "sources"}); err != nil {
		return Sources{}, fmt.Errorf("%w: %w", ErrLost, err)
	}

	m, err := h.next(ctx, h.cfg.ReadyTimeout)
	if err != nil {
		return Sources{}, err
	}

	if m.Type != "sources" {
		return Sources{}, h.unexpected(m)
	}

	return Sources{
		Displays: cleanSources(m.Displays), Windows: cleanSources(m.Windows),
		Microphones: cleanDevices(m.Microphones), Cameras: cleanDevices(m.Cameras), SystemAudio: m.SystemAudio,
	}, nil
}

// Begin asks the helper to record and waits for its started message. A refusal of the helper is a
// *HelperError. On success read Events until a Finished, a Failed or a Lost.
func (h *Helper) Begin(ctx context.Context, req StartRequest) (Started, <-chan Event, error) {
	if err := checkRequest(req); err != nil {
		return Started{}, nil, err
	}

	msg := map[string]any{
		"type": "start", "recording": req.Recording,
		"source": map[string]any{"kind": req.Kind, "id": req.SourceID},
		"fps":    req.FPS, "encoder": req.Encoder, "ffmpeg": req.FFmpeg, "output": req.Output,
	}

	outputs := map[string]string{}

	if req.Microphone != "" {
		msg["microphone"], outputs["microphone"] = req.Microphone, req.MicrophoneOutput
	}

	if req.SystemAudio {
		msg["system_audio"], outputs["system"] = true, req.SystemOutput
	}

	if req.Camera != "" {
		msg["camera"], outputs["camera"] = req.Camera, req.CameraOutput
	}

	if req.Events {
		msg["events"], outputs["events"] = true, req.EventsOutput
	}

	if len(outputs) > 0 {
		msg["outputs"] = outputs
	}

	err := h.send(msg)
	if err != nil {
		return Started{}, nil, fmt.Errorf("%w: %w", ErrLost, err)
	}

	m, err := h.next(ctx, h.cfg.StartedTimeout)
	if err != nil {
		h.kill()

		return Started{}, nil, err
	}

	switch m.Type {
	case "started":
	case "error":
		h.kill()

		return Started{}, nil, &HelperError{Code: clean(m.Code), Message: clean(m.Message)}
	default:
		h.kill()

		return Started{}, nil, h.unexpected(m)
	}

	events := make(chan Event, 16)

	go h.pump(events)

	tracks := make([]TrackStart, 0, len(m.Tracks))
	for _, t := range m.Tracks {
		tracks = append(tracks, TrackStart{Kind: clean(t.Kind), StartMS: t.StartMS})
	}

	return Started{Width: m.Width, Height: m.Height, FPS: m.FPS, Tracks: tracks}, events, nil
}

func (h *Helper) unexpected(m message) error {
	h.kill()

	return fmt.Errorf("%w: unexpected %q message", ErrProtocol, clean(m.Type))
}

// pump turns messages into events and enforces the silence limit. It always ends with one terminal event.
func (h *Helper) pump(events chan<- Event) {
	defer close(events)

	silence := time.NewTimer(h.cfg.Silence)
	defer silence.Stop()

	for {
		select {
		case m, ok := <-h.msgs:
			if !ok {
				events <- Lost{Err: h.ended()}

				return
			}

			if m.err != nil {
				h.kill()

				events <- Lost{Err: m.err}

				return
			}

			if !silence.Stop() {
				select {
				case <-silence.C:
				default:
				}
			}

			silence.Reset(h.cfg.Silence)

			switch m.m.Type {
			case "progress":
				events <- Progress{ElapsedMS: m.m.ElapsedMS, Frames: m.m.Frames, Dropped: m.m.Dropped, Bytes: m.m.Bytes}
			case "level":
				events <- Level{Microphone: m.m.Microphone, System: m.m.System}
			case "paused":
				events <- Paused{ElapsedMS: m.m.ElapsedMS}
			case "resumed":
				events <- Resumed{ElapsedMS: m.m.ElapsedMS}
			case "finished":
				events <- finishedEvent(m.m)

				return
			case "error":
				// A refused command (a pause that cannot be done) leaves the recording running.
				if clean(m.m.Code) == "bad_request" {
					events <- Rejected{Message: clean(m.m.Message)}

					continue
				}

				events <- Failed{Code: clean(m.m.Code), Message: clean(m.m.Message)}

				return
			default:
				h.kill()

				events <- Lost{Err: fmt.Errorf("%w: unexpected %q message", ErrProtocol, clean(m.m.Type))}

				return
			}
		case <-silence.C:
			h.kill()

			events <- Lost{Err: fmt.Errorf("%w: silent for %s", ErrLost, h.cfg.Silence)}

			return
		}
	}
}

// Stop asks the helper to end the recording gracefully. It is safe to call twice.
func (h *Helper) Stop() error {
	if err := h.send(map[string]any{"type": "stop"}); err != nil {
		return fmt.Errorf("%w: %w", ErrLost, err)
	}

	return nil
}

// Pause asks the helper to stop writing every track and freeze the clock; the Paused event confirms it. A pause
// that cannot be done comes back as a Rejected event.
func (h *Helper) Pause() error { return h.command("pause") }

// Resume asks the helper to write again; the Resumed event confirms it.
func (h *Helper) Resume() error { return h.command("resume") }

func (h *Helper) command(name string) error {
	if err := h.send(map[string]any{"type": name}); err != nil {
		return fmt.Errorf("%w: %w", ErrLost, err)
	}

	return nil
}

func finishedEvent(m message) Finished {
	f := Finished{
		Reason: clean(m.Reason), Frames: m.Frames, Dropped: m.Dropped, DurationMS: m.DurationMS, Bytes: m.Bytes,
		Pauses: m.Pauses, PointerEvents: m.PointerEvents,
	}

	for _, t := range m.Tracks {
		f.Tracks = append(f.Tracks, TrackResult{
			Kind: clean(t.Kind), StartMS: t.StartMS, DurationMS: t.DurationMS, Bytes: t.Bytes, Lost: t.Lost,
		})
	}

	return f
}

// Kill ends the helper and its whole process tree at once.
func (h *Helper) Kill() { h.kill() }

func (h *Helper) kill() {
	h.cancel()
	_ = h.in.CloseWithError(io.ErrClosedPipe)
}

// Close ends the child: end of input first, so it exits by itself, then the kill of its tree when it does
// not within a few seconds. It returns what the run reported, nil for a clean end.
func (h *Helper) Close() error {
	_ = h.in.Close()

	select {
	case <-h.done:
	case <-time.After(endGrace):
	}

	h.cancel()
	<-h.done

	// Nobody may be left blocked on the message channel.
	go func() {
		for range h.msgs {
		}
	}()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.runErr != nil && !errors.Is(h.runErr, context.Canceled) {
		return h.runErr
	}

	return nil
}

// Stderr is the tail of what the helper wrote on its error output, once it has ended.
func (h *Helper) Stderr() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return clean(h.stderr)
}

func checkRequest(r StartRequest) error {
	bad := func(what string) error { return fmt.Errorf("%w: %s", ErrBadRequest, what) }

	switch {
	case r.Recording == "" || len(r.Recording) > 32:
		return bad("recording")
	case r.Kind != "display" && r.Kind != "window":
		return bad("source kind")
	case r.SourceID == "" || len(r.SourceID) > 64:
		return bad("source id")
	case !slices.Contains(FrameRates, r.FPS):
		return bad("fps")
	case !slices.Contains(Encoders, r.Encoder):
		return bad("encoder")
	case !filepath.IsAbs(r.FFmpeg):
		return bad("ffmpeg path")
	case !filepath.IsAbs(r.Output) || !strings.EqualFold(filepath.Ext(r.Output), ".mp4"):
		return bad("output path")
	}

	return checkTracks(r, bad)
}

// validDeviceID is what the helper makes of a device: ASCII letters, digits, dash and underscore.
func validDeviceID(id string) bool {
	if id == "" || len(id) > 64 || strings.HasPrefix(id, "fake") {
		return false
	}

	for _, c := range id {
		ok := c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !ok {
			return false
		}
	}

	return true
}

// checkTracks refuses a track whose device is not an id, and a track and its file that do not go together: a
// file is given for exactly each track asked for, in the folder of the screen file, with the extension of its
// kind.
func checkTracks(r StartRequest, bad func(string) error) error {
	folder := filepath.Dir(r.Output)

	file := func(asked bool, path, ext, name string) error {
		switch {
		case !asked && path != "":
			return bad(name + " file without the track")
		case !asked:
			return nil
		case !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ext) || filepath.Dir(path) != folder:
			return bad(name + " file")
		}

		return nil
	}

	if r.Microphone != "" && !validDeviceID(r.Microphone) {
		return bad("microphone id")
	}

	if r.Camera != "" && !validDeviceID(r.Camera) {
		return bad("camera id")
	}

	for _, c := range []struct {
		asked bool
		path  string
		ext   string
		name  string
	}{
		{r.Microphone != "", r.MicrophoneOutput, ".wav", "microphone"},
		{r.SystemAudio, r.SystemOutput, ".wav", "system"},
		{r.Camera != "", r.CameraOutput, ".mp4", "camera"},
		{r.Events, r.EventsOutput, ".jsonl", "events"},
	} {
		if err := file(c.asked, c.path, c.ext, c.name); err != nil {
			return err
		}
	}

	return nil
}

func cleanDevices(in []Device) []Device {
	out := make([]Device, 0, len(in))

	for _, d := range in {
		d.ID, d.Name = clean(d.ID), clean(d.Name)
		out = append(out, d)
	}

	return out
}

func cleanSources(in []Source) []Source {
	out := make([]Source, 0, len(in))

	for _, s := range in {
		s.ID, s.Name = clean(s.ID), clean(s.Name)
		out = append(out, s)
	}

	return out
}

// clean makes text from the helper printable and short before it goes anywhere else.
func clean(s string) string {
	var b strings.Builder

	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}

		if b.Len() >= 300 {
			break
		}
	}

	return b.String()
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}

	return s
}
