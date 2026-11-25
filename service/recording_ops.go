package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// The error codes of SDD-025 (SDD-000 section 8).
const (
	CodeCaptureUnavailable = "E_CAPTURE_UNAVAILABLE"
	CodeCaptureFailed      = "E_CAPTURE_FAILED"
	CodeRecordingNotFound  = "E_RECORDING_NOT_FOUND"
)

const (
	recordingsDir  = "recordings"
	captureFile    = "capture.mp4"
	maxLabelRunes  = 120
	progressSaveMS = 5000
)

// RecordingSourceInfo is a display or a window that can be captured. A window title is untrusted text.
type RecordingSourceInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"   untrusted:"true"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// RecordingSourcesRequest has no member.
type RecordingSourcesRequest struct{}

// RecordingDevice is a microphone or a camera. Its id is the opaque one the helper made; the name is untrusted text.
type RecordingDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"    untrusted:"true"`
	Default bool   `json:"default"`
}

// RecordingSourcesResult is what can be captured now.
type RecordingSourcesResult struct {
	Displays    []RecordingSourceInfo `json:"displays"`
	Windows     []RecordingSourceInfo `json:"windows"`
	Microphones []RecordingDevice     `json:"microphones"`
	Cameras     []RecordingDevice     `json:"cameras"`
	// SystemAudio says whether the mix the computer plays can be recorded.
	SystemAudio bool `json:"system_audio"`
}

// RecordingSource names the source of a recording.
type RecordingSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Name is the title the person saw in the picker; optional, untrusted and sealed in the database.
	Name string `json:"name,omitempty" untrusted:"true"`
}

// RecordingStartRequest is SDD-025 section 9.1. FPS 0 is 30.
type RecordingStartRequest struct {
	ProjectID string          `json:"project_id"`
	Source    RecordingSource `json:"source"`
	FPS       int             `json:"fps,omitempty"`
	// Microphone is a device id from RecordingSources, or "default"; empty records none (SDD-025 section 16).
	Microphone string `json:"microphone,omitempty"`
	// SystemAudio records the mix the computer plays on a track of its own.
	SystemAudio bool `json:"system_audio,omitempty"`
	// Camera is a device id from RecordingSources; empty records none.
	Camera string `json:"camera,omitempty"`
	// Events saves the pointer position and the clicks (never a key) beside the recording.
	Events bool `json:"events,omitempty"`
}

// RecordingTrack is one source of a recording: its kind, where it starts on the shared clock, and how long it is.
type RecordingTrack struct {
	Kind       string `json:"kind"`
	StartMS    int64  `json:"start_ms"`
	DurationMS int64  `json:"duration_ms"`
	Bytes      int64  `json:"bytes"`
	// Lost is a track whose device went away during the recording.
	Lost    bool   `json:"lost,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
}

// RecordingPause is a pause, in ms on the wall clock since the start; no file holds that time.
type RecordingPause struct {
	FromMS int64 `json:"from_ms"`
	ToMS   int64 `json:"to_ms"`
}

// RecordingLevels are the peaks of the sound tracks over the last moment, 0 to 1000, while a recording runs.
type RecordingLevels struct {
	Microphone int `json:"microphone"`
	System     int `json:"system"`
}

// RecordingEventsRequest reads the pointer events of a recording. Kind is clicks (the default) or all.
type RecordingEventsRequest struct {
	ID     string `json:"recording_id"`
	FromMS int64  `json:"from_ms,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// RecordingEvent is a pointer position or a button edge, in active ms and in pixels of the captured source.
type RecordingEvent struct {
	TMS    int64  `json:"t_ms"`
	Kind   string `json:"kind"`
	Button string `json:"button,omitempty"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

// RecordingEventsResult is the part of the event file a request selected.
type RecordingEventsResult struct {
	ID     string `json:"recording_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Total counts the events of the kind asked for, from from_ms on.
	Total     int              `json:"total"`
	Events    []RecordingEvent `json:"events"`
	Truncated bool             `json:"truncated"`
	Text      string           `json:"text"`
}

// RecordingRef names a recording.
type RecordingRef struct {
	ID string `json:"recording_id"`
}

// RecordingListRequest lists the recordings of a project, newest first.
type RecordingListRequest struct {
	ProjectID string `json:"project_id"`
}

// RecordingListResult is the visible recordings of a project.
type RecordingListResult struct {
	Recordings []Recording `json:"recordings"`
}

// RecordingPlaceRequest places a recording a person added on a timeline of its project.
type RecordingPlaceRequest struct {
	ID         string `json:"recording_id"`
	TimelineID string `json:"timeline_id,omitempty"`
}

// Recording is a recording as a surface sees it (SDD-025 section 9.2).
type Recording struct {
	ID         string          `json:"recording_id"`
	ProjectID  string          `json:"project_id"`
	State      string          `json:"state"`
	Review     string          `json:"review,omitempty"`
	Source     RecordingSource `json:"source"`
	Width      int             `json:"width,omitempty"`
	Height     int             `json:"height,omitempty"`
	FPS        int             `json:"fps"`
	Encoder    string          `json:"encoder,omitempty"`
	DurationMS int64           `json:"duration_ms"`
	Frames     int64           `json:"frames"`
	Dropped    int64           `json:"dropped"`
	Bytes      int64           `json:"bytes"`
	StopReason string          `json:"stop_reason,omitempty"`
	Error      *Error          `json:"error,omitempty"`
	AssetID    string          `json:"asset_id,omitempty"`
	// Tracks and Pauses are what the recording holds beyond the screen (SDD-025 section 16); EventsCount is the
	// pointer events saved; Levels is set only while the recording runs.
	Tracks      []RecordingTrack `json:"tracks"`
	Pauses      []RecordingPause `json:"pauses"`
	EventsCount int64            `json:"events_count"`
	Levels      *RecordingLevels `json:"levels,omitempty"`
	// Upload is where a browser recording is, while it receives chunks (SDD-025 section 17.3).
	Upload    *RecordingUpload `json:"upload,omitempty"`
	Actor     string           `json:"actor"`
	CreatedAt time.Time        `json:"created_at"`
	StartedAt *time.Time       `json:"started_at,omitempty"`
	EndedAt   *time.Time       `json:"ended_at,omitempty"`
	Text      string           `json:"text"`
}

// recSession is a recording that is running in this process.
type recSession struct {
	cs   CaptureSession
	done chan struct{}
	// discard is set by a cancel on the goroutine of the request and read by the supervisor when the helper ends.
	discard atomic.Bool
	// move serialises pauses and resumes: the command to the helper and the write of the row happen together,
	// so two requests at once cannot leave the row saying paused while the helper records, or the reverse.
	move sync.Mutex

	// progress is the last line the helper sent, kept in memory so a read shows the clock of a recording that
	// is running without a write to the database every second.
	pmu      sync.Mutex
	progress capturehost.Progress
	levels   capturehost.Level
}

func (r *recSession) setLevels(l capturehost.Level) {
	r.pmu.Lock()
	r.levels = l
	r.pmu.Unlock()
}

func (r *recSession) lastLevels() capturehost.Level {
	r.pmu.Lock()
	defer r.pmu.Unlock()

	return r.levels
}

func (r *recSession) setProgress(p capturehost.Progress) {
	r.pmu.Lock()
	r.progress = p
	r.pmu.Unlock()
}

func (r *recSession) lastProgress() capturehost.Progress {
	r.pmu.Lock()
	defer r.pmu.Unlock()

	return r.progress
}

func recordingNotFound() *Error {
	return newError(CodeRecordingNotFound, "the recording does not exist.").
		withHint("list the recordings of the project with recording:list")
}

func captureUnavailable(reason, why string) *Error {
	return newError(CodeCaptureUnavailable, "screen capture is not available here: %s.", why).
		withDetail("reason", reason).
		withHint("recording works in the desktop app on Windows; the web, Docker, macOS and Linux come later")
}

// unavailable is the error of a service that has no capture.
func (s *svc) unavailable() *Error {
	reason := s.captureReason
	if reason == "" {
		reason = "platform"
	}

	why := map[string]string{
		"platform":       "this system has no capture helper yet",
		"web":            "the web has no native capture",
		"network_mode":   "a server in network mode does not capture its own screen",
		"helper_missing": "the capture helper is not installed",
	}[reason]
	if why == "" {
		why = reason
	}

	return captureUnavailable(reason, why)
}

// captureError maps what the host reports to an error of the service.
func captureError(err error) *Error {
	var he *capturehost.HelperError

	switch {
	case errors.Is(err, capturehost.ErrHelperMissing):
		return captureUnavailable("helper_missing", "the capture helper is not installed")
	case errors.Is(err, capturehost.ErrBadRequest):
		return newError(command.CodeValidation, "the recording request is not valid: %s.", err.Error()).
			withHint("use the device ids RecordingSources lists")
	case errors.As(err, &he):
		return newError(CodeCaptureFailed, "the capture helper could not start the recording: %s.", he.Message).
			withDetail("reason", he.Code)
	case errors.Is(err, capturehost.ErrProtocolVersion):
		return captureUnavailable("protocol_version", "the capture helper is from another version of Stuarty")
	case errors.Is(err, capturehost.ErrUnavailable):
		return captureUnavailable("helper_missing", "the capture helper could not be started")
	case errors.Is(err, capturehost.ErrProtocol):
		return newError(CodeCaptureFailed, "the capture helper broke its protocol.").withDetail("reason", "protocol")
	default:
		return newError(CodeCaptureFailed, "the capture helper ended before it started recording.").
			withDetail("reason", "helper_lost")
	}
}

func (s *svc) recordingLimits() recording.Limits {
	seconds := s.recMaxSeconds
	if seconds <= 0 {
		seconds = recording.DefaultMaxSeconds
	}

	return recording.Limits{MaxSeconds: seconds, MinFreeBytes: s.limits.MinFreeBytes, MaxBytes: s.limits.MaxImportBytes}
}

func (s *svc) freeBytes() int64 {
	if s.media.FreeBytes == nil || s.workspace == "" {
		return 1 << 62
	}

	free, err := s.media.FreeBytes(s.workspace)
	if err != nil {
		return 1 << 62
	}

	return int64(min(free, 1<<62))
}

func (s *svc) recordingPath(id string) string {
	return filepath.Join(s.workspace, recordingsDir, id, captureFile)
}

func isPerson(c Caller) bool { return c.Browser }

// RecordingSources lists the displays and windows that can be captured.
func (s *svc) RecordingSources(ctx context.Context, _ RecordingSourcesRequest) (*RecordingSourcesResult, error) {
	if _, err := s.authorize(ctx, "RecordingSources", ""); err != nil {
		return nil, err
	}

	if s.capture == nil {
		return nil, s.unavailable()
	}

	src, err := s.capture.Sources(ctx)
	if err != nil {
		return nil, captureError(err)
	}

	conv := func(in []capturehost.Source) []RecordingSourceInfo {
		out := make([]RecordingSourceInfo, 0, len(in))

		for _, x := range in {
			out = append(out, RecordingSourceInfo{
				ID: x.ID, Name: recording.CleanLabel(x.Name, maxLabelRunes), Width: x.Width, Height: x.Height,
			})
		}

		return out
	}

	devices := func(in []capturehost.Device) []RecordingDevice {
		out := make([]RecordingDevice, 0, len(in))

		for _, d := range in {
			out = append(out, RecordingDevice{
				ID: d.ID, Name: recording.CleanLabel(d.Name, maxLabelRunes), Default: d.Default,
			})
		}

		return out
	}

	return &RecordingSourcesResult{
		Displays: conv(src.Displays), Windows: conv(src.Windows), Microphones: devices(src.Microphones),
		Cameras: devices(src.Cameras), SystemAudio: src.SystemAudio,
	}, nil
}

// RecordingStart records a display or a window. Only a person starts a capture (ADR-0051): the check is the first
// thing that is done, before the project, the disk or the helper is looked at.
func (s *svc) RecordingStart(ctx context.Context, req RecordingStartRequest) (*Recording, error) {
	c, err := s.person(ctx, "RecordingStart")
	if err != nil {
		return nil, err
	}

	if s.capture == nil {
		return nil, s.unavailable()
	}

	spec := recording.StartSpec{
		ProjectID: req.ProjectID, SourceKind: req.Source.Kind, SourceID: req.Source.ID, FPS: req.FPS,
		Microphone: req.Microphone, SystemAudio: req.SystemAudio, Camera: req.Camera, Events: req.Events,
	}
	if fields := recording.ValidateStart(spec); len(fields) > 0 {
		return nil, newError(command.CodeValidation, "%s is not valid (%s).", fields[0].Field, fields[0].Reason).
			withPath(fields[0].Field)
	}

	if _, err := s.authorizeProject(ctx, "RecordingStart", req.ProjectID); err != nil {
		return nil, err
	}

	if head, gerr := s.store.Productions().Get(ctx, req.ProjectID); gerr != nil || head.State == ProjectTrashed {
		return nil, projectNotFound(req.ProjectID, gerr)
	}

	limits := s.recordingLimits()
	if lerr := limits.CheckStart(s.freeBytes()); lerr != nil {
		return nil, diskLowError()
	}

	fps := recording.FPSOrDefault(req.FPS)
	id, now := s.newIDs("rec"), s.nowMs()
	var asked []recording.Track

	for _, k := range recording.TracksAsked(spec) {
		asked = append(asked, recording.Track{Kind: k, File: k.File()})
	}

	askedJSON, jerr := json.Marshal(asked)
	if jerr != nil {
		return nil, internalError()
	}

	row := recordings.Row{
		ID: id, ProjectID: req.ProjectID, State: string(recording.Prepared), SourceKind: req.Source.Kind,
		SourceLabel: recording.CleanLabel(req.Source.Name, maxLabelRunes), FPS: fps, Actor: c.Actor, CreatedMs: now,
		Tracks: string(askedJSON),
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		open, err := tx.Recordings().Open(ctx)
		if err != nil {
			return err
		}

		// A recording a browser uploads uses no helper and has its own limits (SDD-025 section 17.4).
		open = slices.DeleteFunc(open, func(o recordings.Row) bool { return o.SourceKind == recording.SourceBrowser })

		if len(open) > 0 {
			return newError(CodeQueueFull, "a recording is already in progress.").
				withDetail("limit", "recordings").withDetail("value", len(open)).withDetail("ceiling", 1).
				withHint("stop the recording in progress first")
		}

		if err := tx.Recordings().Insert(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "recording.start", req.ProjectID, id)
	})
	if werr != nil {
		return nil, asError(werr)
	}

	out := s.recordingPath(id)
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		s.failRecording(ctx, id, "io", "the folder of the recording could not be made")

		return nil, newError(
			CodeCaptureFailed,
			"the folder of the recording could not be made.",
		).withDetail("reason", "io")
	}

	folder := filepath.Dir(out)
	cstart := CaptureStart{
		RecordingID: id, Kind: req.Source.Kind, SourceID: req.Source.ID, FPS: fps, Output: out,
		MaxSeconds: limits.MaxSeconds, Microphone: req.Microphone, SystemAudio: req.SystemAudio,
		Camera: req.Camera, Events: req.Events,
	}

	if req.Microphone != "" {
		cstart.MicrophoneOutput = filepath.Join(folder, recording.TrackMicrophone.File())
	}

	if req.SystemAudio {
		cstart.SystemOutput = filepath.Join(folder, recording.TrackSystem.File())
	}

	if req.Camera != "" {
		cstart.CameraOutput = filepath.Join(folder, recording.TrackCamera.File())
	}

	if req.Events {
		cstart.EventsOutput = filepath.Join(folder, recording.EventsFile)
	}

	cs, err := s.capture.Start(ctx, cstart)
	if err != nil {
		cerr := captureError(err)
		s.failRecording(ctx, id, cerr.Code, cerr.Message)
		_ = os.RemoveAll(filepath.Dir(out))

		return nil, cerr
	}

	st := cs.Started()

	got, uerr := s.updateRecording(ctx, id, func(r *recordings.Row) error {
		next, terr := recording.Next(recording.State(r.State), recording.EventStarted)
		if terr != nil {
			return terr
		}

		r.State, r.Width, r.Height, r.Encoder, r.StartedMs = string(next), st.Width, st.Height, cs.Encoder(), s.nowMs()

		return nil
	})
	if uerr != nil {
		cs.Kill()
		_ = cs.Close()

		return nil, asError(uerr)
	}

	sess := &recSession{cs: cs, done: make(chan struct{})}

	s.recMu.Lock()
	if s.recSessions == nil {
		s.recSessions = map[string]*recSession{}
	}

	s.recSessions[id] = sess
	s.recMu.Unlock()

	go s.superviseRecording(context.WithoutCancel(ctx), id, sess, limits)

	return s.liveView(got), nil
}

// updateRecording reads the row inside a transaction, applies fn and writes it back. The read leaves the sealed
// source label out, so a recording is settled while the studio is locked too (the recovery of start-up, or a
// recording that ends after the person locked the studio); the label is added to the answer when it can be opened.
func (s *svc) updateRecording(
	ctx context.Context, id string, fn func(*recordings.Row) error,
) (recordings.Row, error) {
	var out recordings.Row

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		r, err := tx.Recordings().Find(ctx, id)
		if err != nil {
			return err
		}

		if err := fn(&r); err != nil {
			return err
		}

		if err := tx.Recordings().Update(ctx, r); err != nil {
			return err
		}

		if full, gerr := tx.Recordings().Get(ctx, id); gerr == nil {
			r.SourceLabel = full.SourceLabel
		}

		out = r

		return nil
	})

	return out, err
}

func (s *svc) failRecording(ctx context.Context, id, code, message string) {
	_, _ = s.updateRecording(context.WithoutCancel(ctx), id, func(r *recordings.Row) error {
		if next, err := recording.Next(recording.State(r.State), recording.EventFailed); err == nil {
			r.State, r.EndedMs, r.ErrorCode, r.ErrorMessage = string(next), s.nowMs(), code, message
		}

		return nil
	})
}

// superviseRecording follows the events of a running recording to its end: it keeps the counters, stops the
// recording at a limit and settles the row when the helper ends, in every way it can end.
func (s *svc) superviseRecording(ctx context.Context, id string, sess *recSession, limits recording.Limits) {
	defer func() {
		_ = sess.cs.Close()

		s.recMu.Lock()
		delete(s.recSessions, id)
		s.recMu.Unlock()

		close(sess.done)
	}()

	var (
		last     capturehost.Progress
		savedAt  int64
		limitHit bool
	)

	for ev := range sess.cs.Events() {
		switch e := ev.(type) {
		case capturehost.Progress:
			last = e
			sess.setProgress(e)

			if !limitHit {
				if why := limits.Check(e.ElapsedMS, e.Bytes, s.freeBytes()); why != recording.StopNone {
					limitHit = true

					_ = s.stopSession(ctx, id, sess, string(why))
				}
			}

			if e.ElapsedMS-savedAt >= progressSaveMS {
				savedAt = e.ElapsedMS

				_, _ = s.updateRecording(ctx, id, func(r *recordings.Row) error {
					r.DurationMs, r.Frames, r.Dropped, r.Bytes = e.ElapsedMS, e.Frames, e.Dropped, e.Bytes

					return nil
				})
			}
		case capturehost.Level:
			sess.setLevels(e)
		case capturehost.Paused, capturehost.Resumed, capturehost.Rejected:
			// The row moved when the call was made; a refusal leaves the recording as it is.
		case capturehost.Finished:
			s.settleRecording(ctx, id, sess, &e, nil, last)
		case capturehost.Failed:
			s.settleRecording(ctx, id, sess, nil, &e, last)
		case capturehost.Lost:
			s.settleRecording(
				ctx,
				id,
				sess,
				nil,
				&capturehost.Failed{Code: "helper_lost", Message: e.Err.Error()},
				last,
			)
		}
	}
}

// settleRecording writes the end of a recording. A discarded one is removed. Otherwise the file is read: one with
// a decodable frame is ready (a failure of the helper that left one is ready too, with the reason noted), one
// without is failed.
func (s *svc) settleRecording(
	ctx context.Context, id string, sess *recSession, fin *capturehost.Finished, failed *capturehost.Failed,
	last capturehost.Progress,
) {
	if sess.discard.Load() {
		_, _ = s.updateRecording(ctx, id, func(r *recordings.Row) error {
			r.State, r.Review, r.EndedMs = string(recording.Cancelled), "", s.nowMs()

			return nil
		})
		_ = os.RemoveAll(filepath.Dir(s.recordingPath(id)))

		return
	}

	info, ierr := s.capture.Inspect(ctx, s.recordingPath(id))
	readable := ierr == nil && info.DurationMS > 0

	_, _ = s.updateRecording(ctx, id, func(r *recordings.Row) error {
		r.EndedMs = s.nowMs()

		if fin != nil {
			r.Frames, r.Dropped, r.Bytes, r.DurationMs = fin.Frames, fin.Dropped, fin.Bytes, fin.DurationMS
		} else {
			r.Frames, r.Dropped, r.Bytes, r.DurationMs = last.Frames, last.Dropped, last.Bytes, last.ElapsedMS
		}

		if readable {
			r.DurationMs = info.DurationMS
		}

		r.Tracks, r.Pauses, r.EventsCount = settledTracks(*r, filepath.Dir(s.recordingPath(id)), fin)
		if n := tracksBytes(r.Tracks); n > 0 {
			r.Bytes = n
		}

		if fin != nil && (r.State == string(recording.Recording) || r.State == string(recording.Paused)) {
			// The helper ended on its own (the window closed): it counts as a stop.
			if next, err := recording.Next(recording.State(r.State), recording.EventStop); err == nil {
				r.State = string(next)
			}
		}

		switch {
		case fin != nil && readable:
			if next, err := recording.Next(recording.State(r.State), recording.EventFinished); err == nil {
				r.State, r.Review = string(next), string(recording.ReviewPending)
				if r.StopReason == "" {
					r.StopReason = fin.Reason
				}
			}
		case readable:
			// The helper failed but left a file that plays: keep what was written (NFR-068).
			r.State, r.Review = string(recording.Ready), string(recording.ReviewPending)
			r.StopReason, r.ErrorCode, r.ErrorMessage = "recovered", CodeCaptureFailed, failedMessage(failed, ierr)
		default:
			r.State, r.Review = string(recording.Failed), ""
			r.ErrorCode, r.ErrorMessage = CodeCaptureFailed, failedMessage(failed, ierr)
		}

		return nil
	})
}

func failedMessage(f *capturehost.Failed, readErr error) string {
	if f == nil {
		if readErr != nil {
			return "the file could not be read: " + recording.CleanLabel(readErr.Error(), 500)
		}

		return "the recording has no frame"
	}

	return f.Code + ": " + f.Message
}

// stopSession asks the helper to end gracefully and moves the row to finishing.
func (s *svc) stopSession(ctx context.Context, id string, sess *recSession, reason string) error {
	if err := sess.cs.Stop(); err != nil {
		sess.cs.Kill()
	}

	_, err := s.updateRecording(ctx, id, func(r *recordings.Row) error {
		if r.State == string(recording.Finishing) {
			return nil
		}

		next, terr := recording.Next(recording.State(r.State), recording.EventStop)
		if terr != nil {
			return terr
		}

		r.State = string(next)
		if reason != "" {
			r.StopReason = reason
		}

		return nil
	})

	return err
}

// recordingFor loads a recording a caller may see: one of a project the caller covers, and not a dismissed one
// unless the caller is a person (SDD-025 section 4).
func (s *svc) recordingFor(ctx context.Context, c Caller, id string) (recordings.Row, error) {
	if !timeline.ValidID("rec", id) {
		return recordings.Row{}, recordingNotFound()
	}

	r, err := s.store.Recordings().Get(ctx, id)
	if err != nil {
		if errors.Is(err, recordings.ErrNotFound) {
			return r, recordingNotFound()
		}

		return r, asError(err)
	}

	if !s.covers(ctx, c, r.ProjectID) || !recording.VisibleTo(recording.Review(r.Review), isPerson(c)) {
		return r, recordingNotFound()
	}

	return r, nil
}

// RecordingStop ends a recording in progress and keeps what was written. Anyone with the media scope may: it
// only reduces exposure (ADR-0051 item 3).
func (s *svc) RecordingStop(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingStop", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if recording.State(r.State).Final() || r.State == string(recording.Finishing) {
		return s.liveView(r), nil
	}

	if r.SourceKind == recording.SourceBrowser {
		return s.uploadStop(ctx, r)
	}

	s.recMu.Lock()
	sess := s.recSessions[req.ID]
	s.recMu.Unlock()

	if sess == nil || (r.State != string(recording.Recording) && r.State != string(recording.Paused)) {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be stopped.", r.State)
	}

	if err := s.stopSession(ctx, req.ID, sess, "stopped"); err != nil {
		return nil, asError(err)
	}

	r, err = s.store.Recordings().Get(ctx, req.ID)
	if err != nil {
		return nil, asError(err)
	}

	return s.liveView(r), nil
}

// RecordingPause stops every source of a recording from writing. Anyone with the media scope may: it only
// reduces what is captured (SDD-025 section 16.4).
func (s *svc) RecordingPause(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingPause", "")
	if err != nil {
		return nil, err
	}

	return s.moveRecording(ctx, c, req.ID, recording.Recording, recording.EventPause)
}

// RecordingResume restarts the capture of a paused recording. It restarts the capture of the screen, so only a
// person may, and the check comes before anything else is looked at.
func (s *svc) RecordingResume(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.person(ctx, "RecordingResume")
	if err != nil {
		return nil, err
	}

	return s.moveRecording(ctx, c, req.ID, recording.Paused, recording.EventResume)
}

// moveRecording is a pause or a resume: the recording must be in `from` (the target state is a no-op answered as
// it is), the helper is told, and the row moves.
func (s *svc) moveRecording(
	ctx context.Context, c Caller, id string, from recording.State, ev recording.Event,
) (*Recording, error) {
	r, err := s.recordingFor(ctx, c, id)
	if err != nil {
		return nil, err
	}

	to, terr := recording.Next(from, ev)
	if terr != nil {
		return nil, internalError()
	}

	if r.State == string(to) {
		return s.liveView(r), nil
	}

	s.recMu.Lock()
	sess := s.recSessions[id]
	s.recMu.Unlock()

	if sess == nil {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be %s.", r.State, pastOf(ev))
	}

	sess.move.Lock()
	defer sess.move.Unlock()

	// Read again under the lock: another pause or resume may have moved it meanwhile.
	if r, err = s.store.Recordings().Get(ctx, id); err != nil {
		return nil, asError(err)
	}

	if r.State == string(to) {
		return s.liveView(r), nil
	}

	if r.State != string(from) {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be %s.", r.State, pastOf(ev))
	}

	var cerr error
	if ev == recording.EventPause {
		cerr = sess.cs.Pause()
	} else {
		cerr = sess.cs.Resume()
	}

	if cerr != nil {
		return nil, captureError(cerr)
	}

	got, uerr := s.updateRecording(ctx, id, func(x *recordings.Row) error {
		next, nerr := recording.Next(recording.State(x.State), ev)
		if nerr != nil {
			return newError(command.CodeInvalidState, "the recording is %s and cannot be %s.", x.State, pastOf(ev))
		}

		x.State = string(next)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	s.auditRecording(ctx, c, "recording."+string(ev), got)

	return s.liveView(got), nil
}

func pastOf(ev recording.Event) string {
	if ev == recording.EventPause {
		return "paused"
	}

	return "resumed"
}

// RecordingEvents reads the pointer events of a finished recording. The file is read through the reader of
// core/recording, which discards any line that is not a header, a move or a button edge, so what a caller gets
// holds no key and no other member (SDD-025 section 16.6).
func (s *svc) RecordingEvents(ctx context.Context, req RecordingEventsRequest) (*RecordingEventsResult, error) {
	c, err := s.authorize(ctx, "RecordingEvents", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if r.State != string(recording.Ready) {
		return nil, newError(
			command.CodeInvalidState,
			"the recording is %s and its events are read once it is ready.",
			r.State,
		)
	}

	if req.Kind != "" && req.Kind != "clicks" && req.Kind != "all" {
		return nil, newError(command.CodeValidation, "kind must be clicks or all.").withPath("kind")
	}

	q := recording.EventsQuery{FromMS: req.FromMS, Clicks: req.Kind != "all", Limit: req.Limit}
	if _, qerr := recording.NormalizeQuery(q); qerr != nil {
		return nil, newError(command.CodeValidation, "%s.", qerr.Error()).withPath("limit")
	}

	out := &RecordingEventsResult{ID: r.ID, Events: []RecordingEvent{}}

	f, oerr := os.Open(filepath.Join(filepath.Dir(s.recordingPath(r.ID)), recording.EventsFile))
	if oerr != nil {
		out.Text = "Recording " + r.ID + " has no pointer events: the person did not ask for them."

		return out, nil
	}

	defer func() { _ = f.Close() }()

	res, rerr := recording.ReadEvents(f, q)
	if rerr != nil {
		return nil, newError(CodeCaptureFailed, "the pointer events of the recording cannot be read.").
			withDetail("reason", "events_unreadable")
	}

	out.Width, out.Height, out.Total, out.Truncated = res.Width, res.Height, res.Total, res.Truncated
	for _, e := range res.Events {
		out.Events = append(out.Events, RecordingEvent{
			TMS: e.TMS, Kind: string(e.Kind), Button: e.Button, X: e.X, Y: e.Y,
		})
	}

	out.Text = "Recording " + r.ID + " has " + strconv.Itoa(
		res.Total,
	) + " pointer events of the kind asked for, on a " +
		strconv.Itoa(
			res.Width,
		) + " by " + strconv.Itoa(
		res.Height,
	) + " source."

	return out, nil
}

// RecordingCancel discards a recording in progress (anyone with the media scope), or deletes a finished one that
// was not added (a person only).
func (s *svc) RecordingCancel(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingCancel", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	st, rv := recording.State(r.State), recording.Review(r.Review)

	if !st.Final() && r.SourceKind == recording.SourceBrowser {
		return s.uploadDiscard(ctx, r)
	}

	if !st.Final() {
		s.recMu.Lock()
		sess := s.recSessions[req.ID]
		s.recMu.Unlock()

		if sess == nil {
			return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be cancelled.", r.State)
		}

		sess.discard.Store(true)
		sess.cs.Kill()

		select {
		case <-sess.done:
		case <-time.After(15 * time.Second):
		}

		r, err = s.store.Recordings().Get(ctx, req.ID)
		if err != nil {
			return nil, asError(err)
		}

		return s.liveView(r), nil
	}

	if !recording.CanDelete(st, rv) {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be deleted.", r.State).
			withHint("an added recording is a library asset: remove it as an asset")
	}

	if !isPerson(c) {
		return nil, privilegedError("RecordingCancel")
	}

	got, uerr := s.updateRecording(ctx, req.ID, func(x *recordings.Row) error {
		x.State, x.Review = string(recording.Cancelled), ""

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	_ = os.RemoveAll(filepath.Dir(s.recordingPath(req.ID)))
	s.auditRecording(ctx, c, "recording.delete", got)

	return s.liveView(got), nil
}

func (s *svc) auditRecording(ctx context.Context, c Caller, action string, r recordings.Row) {
	_ = s.store.Write(context.WithoutCancel(ctx), func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), c.Actor, action, r.ProjectID, r.ID)
	})
}

// RecordingGet returns one recording.
func (s *svc) RecordingGet(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingGet", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	return s.liveView(r), nil
}

// RecordingList returns the recordings of a project that the caller may see.
func (s *svc) RecordingList(ctx context.Context, req RecordingListRequest) (*RecordingListResult, error) {
	c, err := s.authorizeProject(ctx, "RecordingList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.Recordings().ListByProject(ctx, req.ProjectID)
	if err != nil {
		return nil, asError(err)
	}

	out := &RecordingListResult{Recordings: make([]Recording, 0, len(rows))}

	for i := range rows {
		if recording.VisibleTo(recording.Review(rows[i].Review), isPerson(c)) {
			out.Recordings = append(out.Recordings, *s.liveView(rows[i]))
		}
	}

	return out, nil
}

// RecordingConfirm is the person's yes: the recording becomes a library asset of its project by the ordinary
// import, and agents with access to the project can read it from then on (ADR-0051 item 7).
func (s *svc) RecordingConfirm(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.person(ctx, "RecordingConfirm")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if _, rerr := recording.Confirm(recording.State(r.State), recording.Review(r.Review)); rerr != nil {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be added.", r.State)
	}

	path := s.recordingPath(r.ID)
	folder := filepath.Dir(path)

	// Every track is a file of its own and becomes an asset of its own, the screen first (SDD-025 section 16.8).
	tracks := decodeTracks(r.Tracks)
	if len(tracks) == 0 {
		tracks = []recording.Track{{Kind: recording.TrackScreen, File: recording.TrackScreen.File()}}
	}

	files := make([]ImportFile, 0, len(tracks))

	for _, t := range tracks {
		p := filepath.Join(folder, t.File)
		if _, serr := os.Stat(p); serr != nil {
			return nil, newError(CodeCaptureFailed, "the file of the recording is gone.").withDetail("reason", "io")
		}

		files = append(files, ImportFile{Path: p, recording: true})
	}

	res, ierr := s.MediaImport(ctx, MediaImportRequest{
		ProjectID: r.ProjectID,
		Files:     files,
		Info:      &AssetInfo{Tags: []string{"recording"}},
	})
	if ierr != nil {
		return nil, ierr
	}

	if len(res.Assets) != len(tracks) {
		return nil, newError(CodeCaptureFailed, "the recording could not be imported.").withDetail("reason", "io")
	}

	for i := range res.Assets {
		if res.Assets[i].Error != nil {
			return nil, res.Assets[i].Error
		}
	}

	for i := range tracks {
		tracks[i].AssetHash, tracks[i].AssetID = res.Assets[i].Hash, res.Assets[i].AssetID
	}

	tj, merr := json.Marshal(tracks)
	if merr != nil {
		return nil, internalError()
	}

	asset := res.Assets[0]

	got, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
		rv, rerr := recording.Confirm(recording.State(x.State), recording.Review(x.Review))
		if rerr != nil {
			return rerr
		}

		x.Review, x.AssetHash, x.AssetID, x.Tracks = string(rv), asset.Hash, asset.AssetID, string(tj)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	// The import copied the bytes into the content store; the media files are not needed any more. The pointer
	// events stay: they are read after the recording was added, and go with the recording.
	for _, t := range tracks {
		_ = os.Remove(filepath.Join(folder, t.File))
	}

	if _, serr := os.Stat(filepath.Join(folder, recording.EventsFile)); serr != nil {
		_ = os.RemoveAll(folder)
	}

	s.auditRecording(ctx, c, "recording.confirm", got)

	return s.liveView(got), nil
}

// RecordingDismiss is "Not now": the recording stays out of the project and invisible to agents until a person
// adds or deletes it.
func (s *svc) RecordingDismiss(ctx context.Context, req RecordingRef) (*Recording, error) {
	c, err := s.person(ctx, "RecordingDismiss")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	got, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
		rv, rerr := recording.Dismiss(recording.State(x.State), recording.Review(x.Review))
		if rerr != nil {
			return newError(command.CodeInvalidState, "the recording is %s and cannot be dismissed.", x.State)
		}

		x.Review = string(rv)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	s.auditRecording(ctx, c, "recording.dismiss", got)

	return s.liveView(got), nil
}

// RecordingPlace puts a recording a person added on a timeline as one batch: one clip at the end of the first
// video track that is not locked (a track is made when there is none), so one undo removes it.
func (s *svc) RecordingPlace(ctx context.Context, req RecordingPlaceRequest) (*EditResult, error) {
	c, err := s.authorize(ctx, "RecordingPlace", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if !recording.Placeable(recording.State(r.State), recording.Review(r.Review)) {
		return nil, newError(command.CodeInvalidState, "the recording is not in the project yet.").
			withHint("a person adds the recording to the project first")
	}

	pg, err := s.ProjectGet(
		ctx,
		ProjectGetRequest{ProjectID: r.ProjectID, TimelineID: req.TimelineID, Detail: "document"},
	)
	if err != nil {
		return nil, err
	}

	doc := pg.Document
	if doc == nil {
		return nil, internalError()
	}

	// The tracks of the recording and the asset each became. A recording made before tracks existed has the screen.
	rtracks := decodeTracks(r.Tracks)
	if len(rtracks) == 0 {
		rtracks = []recording.Track{{Kind: recording.TrackScreen, AssetHash: r.AssetHash}}
	}

	assetOf := map[recording.TrackKind]string{}

	for _, t := range rtracks {
		hash := t.AssetHash
		if hash == "" && t.Kind == recording.TrackScreen {
			hash = r.AssetHash
		}

		for id := range doc.Assets {
			if hash != "" && doc.Assets[id].Hash == hash {
				assetOf[t.Kind] = id

				break
			}
		}

		if assetOf[t.Kind] == "" {
			return nil, newError("E_ASSET_NOT_READY", "the recording is still being prepared for the timeline.").
				withHint("try again in a moment")
		}
	}

	plan := newPlacement(s, doc)
	if perr := plan.place(rtracks, assetOf); perr != nil {
		return nil, perr
	}

	return s.Edit(ctx, EditRequest{
		ProjectID: r.ProjectID, TimelineID: req.TimelineID, BaseRevision: pg.Revision, Operations: plan.ops,
	})
}

// placement builds the batch that puts the tracks of a recording on a timeline (SDD-025 section 16.9): the
// screen on the first unlocked visual track as before, the camera on a visual track of its own, each sound on an
// audio track of its own, every clip at the start the screen clip gets plus the start the helper reported.
type placement struct {
	s       *svc
	doc     *timeline.Document
	ops     []command.Operation
	taken   map[string]bool
	fpsNum  int
	fpsDen  int
	trackOf map[recording.TrackKind]string
}

func newPlacement(s *svc, doc *timeline.Document) *placement {
	return &placement{
		s: s, doc: doc, taken: map[string]bool{}, fpsNum: max(doc.Settings.FPS.Num, 1),
		fpsDen: max(doc.Settings.FPS.Den, 1), trackOf: map[recording.TrackKind]string{},
	}
}

// end is the end, in frames, of the last item of a track.
func (p *placement) end(trackID string) int {
	end := 0

	for id := range p.doc.Items {
		if it := p.doc.Items[id]; it.Track == trackID {
			end = max(end, it.Start+it.Duration)
		}
	}

	return end
}

// unlocked is the unlocked tracks of a kind in the order of the timeline, the ones already used left out.
func (p *placement) unlocked(kind timeline.TrackKind, name string) []timeline.Track {
	var out []timeline.Track

	for _, t := range p.doc.Tracks {
		if t.Kind == kind && !t.Locked && !p.taken[t.ID] && (name == "" || t.Name == name) {
			out = append(out, t)
		}
	}

	slices.SortFunc(out, func(a, b timeline.Track) int { return strings.Compare(a.Order+a.ID, b.Order+b.ID) })

	return out
}

func (p *placement) addTrack(kind timeline.TrackKind, name string) (string, error) {
	id := p.s.newIDs("trk")

	args, err := json.Marshal(struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Name string `json:"name"`
	}{id, string(kind), name})
	if err != nil {
		return "", internalError()
	}

	p.ops = append(p.ops, command.Operation{Op: "track.add", Args: args})

	return id, nil
}

func (p *placement) place(tracks []recording.Track, assetOf map[recording.TrackKind]string) error {
	offsets := recording.PlaceStarts(tracks)
	multi := len(tracks) > 1
	startFrame := 0

	for _, t := range tracks {
		kind := timeline.TrackVisual
		if t.Kind.Audio() {
			kind = timeline.TrackAudio
		}

		// The screen takes the first unlocked visual track, as before; the others reuse a track of their own name
		// that has room at that start, and otherwise get one.
		var trackID string

		var err error

		cands := p.unlocked(kind, "")
		if t.Kind != recording.TrackScreen {
			cands = p.unlocked(kind, t.Kind.Label())
		}

		for _, c := range cands {
			if t.Kind == recording.TrackScreen || p.end(c.ID) <= startFrame {
				trackID = c.ID

				break
			}
		}

		if trackID == "" {
			if trackID, err = p.addTrack(kind, t.Kind.Label()); err != nil {
				return err
			}
		}

		p.taken[trackID] = true

		if t.Kind == recording.TrackScreen {
			startFrame = p.end(trackID)
		}

		op, arg := "clip.add", struct {
			TrackID string `json:"track_id"`
			AssetID string `json:"asset_id"`
			Start   string `json:"start,omitempty"`
		}{TrackID: trackID, AssetID: assetOf[t.Kind]}

		if t.Kind.Audio() {
			op = "audio.add"
		}

		if multi {
			frames := int(offsets[t.Kind] * int64(p.fpsNum) / (1000 * int64(p.fpsDen)))
			arg.Start = strconv.Itoa(startFrame+frames) + "f"
		}

		args, merr := json.Marshal(arg)
		if merr != nil {
			return internalError()
		}

		p.ops = append(p.ops, command.Operation{Op: op, Args: args})
	}

	return nil
}

// liveView is the surface form of a row, with the counters of the helper while the recording runs.
func (s *svc) liveView(r recordings.Row) *Recording {
	var levels *RecordingLevels

	if st := recording.State(
		r.State,
	); st == recording.Recording || st == recording.Paused ||
		st == recording.Finishing {
		s.recMu.Lock()
		sess := s.recSessions[r.ID]
		s.recMu.Unlock()

		if sess != nil {
			p := sess.lastProgress()
			r.DurationMs, r.Frames, r.Dropped, r.Bytes = p.ElapsedMS, p.Frames, p.Dropped, p.Bytes
			l := sess.lastLevels()
			levels = &RecordingLevels{Microphone: l.Microphone, System: l.System}
		}
	}

	var up *RecordingUpload

	if r.SourceKind == recording.SourceBrowser && r.State == string(recording.Recording) {
		up = s.uploadView(r)

		u := s.uploadFor(r)
		u.cmu.Lock()
		r.DurationMs, r.Bytes = u.elapsedMS, u.total()
		u.cmu.Unlock()
	}

	v := s.recordingView(r)
	v.Levels, v.Upload = levels, up

	return v
}

// recordingView is the surface form of a row. The source label is untrusted text (SDD-007 section 8).
func (s *svc) recordingView(r recordings.Row) *Recording {
	v := &Recording{
		ID: r.ID, ProjectID: r.ProjectID, State: r.State, Review: r.Review,
		Source: RecordingSource{Kind: r.SourceKind, Name: r.SourceLabel}, Width: r.Width, Height: r.Height,
		FPS: r.FPS, Encoder: r.Encoder, DurationMS: r.DurationMs, Frames: r.Frames, Dropped: r.Dropped, Bytes: r.Bytes,
		StopReason: r.StopReason, AssetID: r.AssetID, Actor: r.Actor, CreatedAt: time.UnixMilli(r.CreatedMs).UTC(),
	}

	if r.StartedMs > 0 {
		t := time.UnixMilli(r.StartedMs).UTC()
		v.StartedAt = &t
	}

	if r.EndedMs > 0 {
		t := time.UnixMilli(r.EndedMs).UTC()
		v.EndedAt = &t
	}

	if r.ErrorCode != "" {
		v.Error = &Error{Code: r.ErrorCode, Message: r.ErrorMessage}
	}

	v.Tracks, v.Pauses, v.EventsCount = []RecordingTrack{}, []RecordingPause{}, r.EventsCount

	for _, t := range decodeTracks(r.Tracks) {
		v.Tracks = append(v.Tracks, RecordingTrack{
			Kind: string(t.Kind), StartMS: t.StartMS, DurationMS: t.DurationMS, Bytes: t.Bytes, Lost: t.Lost,
			AssetID: t.AssetID,
		})
	}

	for _, p := range decodePauses(r.Pauses) {
		v.Pauses = append(v.Pauses, RecordingPause{FromMS: p.FromMS, ToMS: p.ToMS})
	}

	v.Text = recordingText(v)

	return v
}

func recordingText(v *Recording) string {
	secs := strconv.FormatFloat(float64(v.DurationMS)/1000, 'f', 1, 64)

	switch recording.State(v.State) {
	case recording.Prepared:
		return "Recording " + v.ID + " is starting."
	case recording.Recording:
		return "Recording " + v.ID + " is running: " + secs + " s so far."
	case recording.Paused:
		return "Recording " + v.ID + " is paused at " + secs + " s."
	case recording.Finishing:
		return "Recording " + v.ID + " is finishing the file."
	case recording.Ready:
		switch recording.Review(v.Review) {
		case recording.ReviewAdded:
			// The asset id is what an agent needs to place the recording with the edit tool (SDD-025 section 11).
			return "Recording " + v.ID + " is in the project as asset " + v.AssetID + " (" + secs + " s)." +
				trackText(v)
		case recording.ReviewDismissed:
			return "Recording " + v.ID + " is kept out of the project until a person adds or deletes it."
		case recording.ReviewPending:
		}

		return "Recording " + v.ID + " is ready (" + secs + " s) and waits for a person to add it to the project."
	case recording.Failed:
		return "Recording " + v.ID + " failed."
	case recording.Cancelled:
	}

	return "Recording " + v.ID + " was cancelled."
}

// trackText names the other tracks of an added recording with their assets, and the pointer events it holds, for
// the text of job_get (SDD-025 section 16.8). A recording of the screen alone adds nothing.
func trackText(v *Recording) string {
	var parts []string

	for _, t := range v.Tracks {
		if t.Kind != string(recording.TrackScreen) && t.AssetID != "" {
			parts = append(parts, t.Kind+" is asset "+t.AssetID)
		}
	}

	out := ""
	if len(parts) > 0 {
		out = " Its other tracks: " + strings.Join(parts, ", ") + "."
	}

	if v.EventsCount > 0 {
		out += " It has " + strconv.FormatInt(v.EventsCount, 10) + " pointer events, read with recording:events."
	}

	return out
}

// recordingJobState is the job state an MCP caller sees for a recording (SDD-025 section 11).
func recordingJobState(st recording.State) string {
	switch st {
	case recording.Prepared:
		return JobQueued
	case recording.Recording, recording.Paused, recording.Finishing:
		return JobRunning
	case recording.Ready:
		return JobSucceeded
	case recording.Failed:
		return JobFailed
	case recording.Cancelled:
	}

	return JobCancelled
}

// recordingJobStop is job_cancel of a recording: handled is true when the id is one. It is RecordingStop, so it
// keeps what was written (SDD-025 section 11).
func (s *svc) recordingJobStop(ctx context.Context, id string) (job *Job, handled bool, err error) {
	if !strings.HasPrefix(id, "rec_") {
		return nil, false, nil
	}

	v, err := s.RecordingStop(ctx, RecordingRef{ID: id})
	if err != nil {
		return nil, true, err
	}

	return s.recordingJob(v), true, nil
}

// recordingJob is a recording read as a job of kind capture, for job_get and job_cancel.
func (s *svc) recordingJob(v *Recording) *Job {
	j := &Job{
		JobID: v.ID, Kind: "capture", ProjectID: &v.ProjectID, State: recordingJobState(recording.State(v.State)),
		Message: v.Text, Error: v.Error, Actor: v.Actor, CreatedAt: v.CreatedAt, StartedAt: v.StartedAt,
		FinishedAt: v.EndedAt, Text: v.Text,
	}
	if j.State == JobSucceeded {
		j.Progress = 1
	}

	return j
}

// recoverRecordings settles every recording the last process left open (SDD-025 section 10). It runs at start-up,
// before requests: no session of this process exists yet.
func (s *svc) recoverRecordings(ctx context.Context) error {
	rows, err := s.store.Recordings().Open(ctx)
	if err != nil {
		return err
	}

	for i := range rows {
		r := rows[i]

		if r.SourceKind == recording.SourceBrowser {
			if uerr := s.recoverUpload(ctx, r); uerr != nil {
				return uerr
			}

			continue
		}

		path := s.recordingPath(r.ID)

		var info CaptureInfo

		readable := false
		// reason is why a recording that is not readable failed: no file at all, a file with no decodable frame,
		// or a file this process has no way to read (no capture here now), which is kept for a person to delete.
		reason := "no_frames"

		switch _, statErr := os.Stat(path); {
		case statErr != nil:
		case s.capture == nil:
			reason = "helper_lost"
		default:
			var ierr error
			if info, ierr = s.capture.Inspect(ctx, path); ierr == nil && info.DurationMS > 0 {
				readable = true
			}
		}

		next, serr := recording.Settle(recording.State(r.State), readable)
		if serr != nil {
			continue
		}

		_, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
			x.State, x.EndedMs = string(next), s.nowMs()

			if readable {
				x.Review, x.StopReason, x.DurationMs, x.Width, x.Height = string(recording.ReviewPending), "recovered",
					info.DurationMS, info.Width, info.Height
				if st, e := os.Stat(path); e == nil {
					x.Bytes = st.Size()
				}

				x.Tracks, x.Pauses, x.EventsCount = settledTracks(*x, filepath.Dir(path), nil)
				if n := tracksBytes(x.Tracks); n > 0 {
					x.Bytes = n
				}

				return nil
			}

			x.ErrorCode, x.ErrorMessage = CodeCaptureFailed, "the server stopped while this recording was running ("+reason+")"

			return nil
		})
		if uerr != nil {
			return uerr
		}
	}

	return nil
}

// ShutdownRecordings ends the recordings of a Service made by New when the process closes. It is not an operation:
// no caller of the API may drive it.
func ShutdownRecordings(s Service) {
	if impl, ok := s.(*svc); ok {
		impl.shutdownRecordings()
	}
}

// shutdownRecordings ends every recording of this process when the service closes: a graceful stop, so the
// file is complete. It is not part of the Service interface.
func (s *svc) shutdownRecordings() {
	s.recMu.Lock()
	all := make([]*recSession, 0, len(s.recSessions))

	for _, sess := range s.recSessions {
		all = append(all, sess)
	}
	s.recMu.Unlock()

	var wg sync.WaitGroup

	for _, sess := range all {
		wg.Go(func() {
			_ = sess.cs.Stop()

			select {
			case <-sess.done:
			case <-time.After(10 * time.Second):
				sess.cs.Kill()
			}
		})
	}

	wg.Wait()

	// An assembly that does not end in time leaves its recording finishing, and the next start assembles it again.
	s.waitAssemblies(10 * time.Second)
}
