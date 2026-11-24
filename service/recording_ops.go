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
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// The error codes of SDD-023 (SDD-000 section 8).
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

// RecordingSourcesResult is what can be captured now.
type RecordingSourcesResult struct {
	Displays []RecordingSourceInfo `json:"displays"`
	Windows  []RecordingSourceInfo `json:"windows"`
}

// RecordingSource names the source of a recording.
type RecordingSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Name is the title the person saw in the picker; optional, untrusted and sealed in the database.
	Name string `json:"name,omitempty" untrusted:"true"`
}

// RecordingStartRequest is SDD-023 section 9.1. FPS 0 is 30.
type RecordingStartRequest struct {
	ProjectID string          `json:"project_id"`
	Source    RecordingSource `json:"source"`
	FPS       int             `json:"fps,omitempty"`
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

// Recording is a recording as a surface sees it (SDD-023 section 9.2).
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
	Actor      string          `json:"actor"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	EndedAt    *time.Time      `json:"ended_at,omitempty"`
	Text       string          `json:"text"`
}

// recSession is a recording that is running in this process.
type recSession struct {
	cs      CaptureSession
	done    chan struct{}
	discard bool // cancel: drop the file when the helper ends

	// progress is the last line the helper sent, kept in memory so a read shows the clock of a recording that
	// is running without a write to the database every second.
	pmu      sync.Mutex
	progress capturehost.Progress
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
	case errors.As(err, &he):
		return newError(CodeCaptureFailed, "the capture helper could not start the recording: %s.", he.Message).
			withDetail("reason", he.Code)
	case errors.Is(err, capturehost.ErrUnavailable):
		reason := "helper_missing"
		if strings.Contains(err.Error(), "protocol version") {
			reason = "protocol_version"
		}

		return captureUnavailable(reason, "the capture helper could not be started")
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

	return &RecordingSourcesResult{Displays: conv(src.Displays), Windows: conv(src.Windows)}, nil
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
		return nil, newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space to record.").
			withHint("free space on the disk of the workspace; the person running the server can lower "+
				"STUARTY_MIN_FREE_BYTES").
			withDetail("reason", "disk_low").withDetail("variable", "STUARTY_MIN_FREE_BYTES")
	}

	fps := recording.FPSOrDefault(req.FPS)
	id, now := s.newIDs("rec"), s.nowMs()
	row := recordings.Row{
		ID: id, ProjectID: req.ProjectID, State: string(recording.Prepared), SourceKind: req.Source.Kind,
		SourceLabel: recording.CleanLabel(req.Source.Name, maxLabelRunes), FPS: fps, Actor: c.Actor, CreatedMs: now,
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		open, err := tx.Recordings().Open(ctx)
		if err != nil {
			return err
		}

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

	cs, err := s.capture.Start(ctx, CaptureStart{
		RecordingID: id, Kind: req.Source.Kind, SourceID: req.Source.ID, FPS: fps, Output: out,
		MaxSeconds: limits.MaxSeconds,
	})
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

// updateRecording reads the row inside a transaction, applies fn and writes it back.
func (s *svc) updateRecording(
	ctx context.Context, id string, fn func(*recordings.Row) error,
) (recordings.Row, error) {
	var out recordings.Row

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		r, err := tx.Recordings().Get(ctx, id)
		if err != nil {
			return err
		}

		if err := fn(&r); err != nil {
			return err
		}

		out = r

		return tx.Recordings().Update(ctx, r)
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
	if sess.discard {
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

		if fin != nil && r.State == string(recording.Recording) {
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
			r.StopReason, r.ErrorCode, r.ErrorMessage = "recovered", CodeCaptureFailed, failedMessage(failed)
		default:
			r.State, r.Review = string(recording.Failed), ""
			r.ErrorCode, r.ErrorMessage = CodeCaptureFailed, failedMessage(failed)
		}

		return nil
	})
}

func failedMessage(f *capturehost.Failed) string {
	if f == nil {
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
// unless the caller is a person (SDD-023 section 4).
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

	s.recMu.Lock()
	sess := s.recSessions[req.ID]
	s.recMu.Unlock()

	if sess == nil || r.State != string(recording.Recording) {
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

	if !st.Final() {
		s.recMu.Lock()
		sess := s.recSessions[req.ID]
		s.recMu.Unlock()

		if sess == nil {
			return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be cancelled.", r.State)
		}

		sess.discard = true
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
	if _, serr := os.Stat(path); serr != nil {
		return nil, newError(CodeCaptureFailed, "the file of the recording is gone.").withDetail("reason", "io")
	}

	res, ierr := s.MediaImport(ctx, MediaImportRequest{
		ProjectID: r.ProjectID,
		Files:     []ImportFile{{Path: path, recording: true}},
		Info:      &AssetInfo{Tags: []string{"recording"}},
	})
	if ierr != nil {
		return nil, ierr
	}

	if len(res.Assets) != 1 || res.Assets[0].Error != nil {
		var cause *Error
		if len(res.Assets) == 1 {
			cause = res.Assets[0].Error
		}

		if cause != nil {
			return nil, cause
		}

		return nil, newError(CodeCaptureFailed, "the recording could not be imported.").withDetail("reason", "io")
	}

	asset := res.Assets[0]

	got, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
		rv, rerr := recording.Confirm(recording.State(x.State), recording.Review(x.Review))
		if rerr != nil {
			return rerr
		}

		x.Review, x.AssetHash, x.AssetID = string(rv), asset.Hash, asset.AssetID

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	// The import copied the bytes into the content store; the recording's own copy is not needed any more.
	_ = os.RemoveAll(filepath.Dir(path))

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

	assetID := ""

	for id := range doc.Assets {
		if doc.Assets[id].Hash == r.AssetHash {
			assetID = id

			break
		}
	}

	if assetID == "" {
		return nil, newError("E_ASSET_NOT_READY", "the recording is still being prepared for the timeline.").
			withHint("try again in a moment")
	}

	var tracks []timeline.Track

	for _, t := range doc.Tracks {
		if t.Kind == timeline.TrackVisual && !t.Locked {
			tracks = append(tracks, t)
		}
	}

	slices.SortFunc(tracks, func(a, b timeline.Track) int { return strings.Compare(a.Order+a.ID, b.Order+b.ID) })

	var ops []command.Operation

	trackID := ""
	if len(tracks) > 0 {
		trackID = tracks[0].ID
	} else {
		trackID = s.newIDs("trk")

		args, merr := json.Marshal(struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Name string `json:"name"`
		}{trackID, "visual", "Recording"})
		if merr != nil {
			return nil, internalError()
		}

		ops = append(ops, command.Operation{Op: "track.add", Args: args})
	}

	args, merr := json.Marshal(struct {
		TrackID string `json:"track_id"`
		AssetID string `json:"asset_id"`
	}{trackID, assetID})
	if merr != nil {
		return nil, internalError()
	}

	ops = append(ops, command.Operation{Op: "clip.add", Args: args})

	return s.Edit(ctx, EditRequest{
		ProjectID: r.ProjectID, TimelineID: req.TimelineID, BaseRevision: pg.Revision, Operations: ops,
	})
}

// liveView is the surface form of a row, with the counters of the helper while the recording runs.
func (s *svc) liveView(r recordings.Row) *Recording {
	if r.State == string(recording.Recording) || r.State == string(recording.Finishing) {
		s.recMu.Lock()
		sess := s.recSessions[r.ID]
		s.recMu.Unlock()

		if sess != nil {
			p := sess.lastProgress()
			r.DurationMs, r.Frames, r.Dropped, r.Bytes = p.ElapsedMS, p.Frames, p.Dropped, p.Bytes
		}
	}

	return s.recordingView(r)
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
	case recording.Finishing:
		return "Recording " + v.ID + " is finishing the file."
	case recording.Ready:
		switch recording.Review(v.Review) {
		case recording.ReviewAdded:
			return "Recording " + v.ID + " is in the project (" + secs + " s)."
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

// recordingJobState is the job state an MCP caller sees for a recording (SDD-023 section 11).
func recordingJobState(st recording.State) string {
	switch st {
	case recording.Prepared:
		return JobQueued
	case recording.Recording, recording.Finishing:
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
// keeps what was written (SDD-023 section 11).
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

// recoverRecordings settles every recording the last process left open (SDD-023 section 10). It runs at start-up,
// before requests: no session of this process exists yet.
func (s *svc) recoverRecordings(ctx context.Context) error {
	rows, err := s.store.Recordings().Open(ctx)
	if err != nil {
		return err
	}

	for i := range rows {
		r := rows[i]
		path := s.recordingPath(r.ID)

		var info CaptureInfo

		readable := false

		if s.capture != nil {
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

				return nil
			}

			reason := "helper_lost"
			if _, e := os.Stat(path); e != nil {
				reason = "no_frames"
			}

			x.ErrorCode, x.ErrorMessage = CodeCaptureFailed, "the server stopped while this recording was running ("+reason+")"

			return nil
		})
		if uerr != nil {
			return uerr
		}

		if !readable {
			_ = os.RemoveAll(filepath.Dir(path))
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
}
