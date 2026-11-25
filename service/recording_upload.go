package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// A recording that a browser captures and uploads (SDD-025 section 17). The rules are in core/recording
// (upload.go): the order of the chunks, the limits and the names. This file applies them to the disk and the row.

const (
	partsDir = "parts"
	partExt  = ".part"
	// incomingName is the file a chunk is written to before it is checked; chunks of one recording are serialised,
	// so one name is enough and no name comes from the caller.
	incomingName = "incoming.tmp"
)

// RecordingUploadBeginRequest starts a recording that the browser will upload (SDD-025 section 17.3).
type RecordingUploadBeginRequest struct {
	ProjectID string `json:"project_id"`
	// Container is webm or mp4, what the browser records.
	Container string `json:"container"`
	// Camera says that a second track, the camera, will be uploaded.
	Camera bool `json:"camera,omitempty"`
	// Label is optional plain text about what was shared; untrusted and sealed in the database.
	Label string `json:"label,omitempty" untrusted:"true"`
}

// RecordingUploadChunkRequest is one chunk of a track: the bytes are Body, and nothing else is read from it.
type RecordingUploadChunkRequest struct {
	ID    string
	Track string
	Index int
	// SHA256 is the lowercase hexadecimal digest of the body, which the server checks.
	SHA256 string
	// ElapsedMS is the clock of the recorder; informational, and checked against the duration limit.
	ElapsedMS int64
	Body      io.Reader
	// Size is the declared length of the body, or -1.
	Size int64
}

// RecordingUploadChunkResult says where the track is after a chunk.
type RecordingUploadChunkResult struct {
	ID    string `json:"recording_id"`
	Track string `json:"track"`
	Index int    `json:"index"`
	// NextIndex is the chunk the track waits for.
	NextIndex int `json:"next_index"`
	// Duplicate is true when the chunk was already stored and the same: nothing changed.
	Duplicate bool `json:"duplicate"`
	// Bytes is what the server holds for the recording, every track.
	Bytes int64  `json:"bytes"`
	Text  string `json:"text"`
}

// RecordingUploadCount is the number of chunks the browser sent on a track.
type RecordingUploadCount struct {
	Kind   string `json:"kind"`
	Chunks int    `json:"chunks"`
}

// RecordingUploadFinishRequest ends an upload: the server assembles what it has.
type RecordingUploadFinishRequest struct {
	ID        string                 `json:"recording_id"`
	Tracks    []RecordingUploadCount `json:"tracks,omitempty"`
	ElapsedMS int64                  `json:"elapsed_ms,omitempty"`
}

// RecordingUploadTrack is where one track of an upload is.
type RecordingUploadTrack struct {
	Kind      string `json:"kind"`
	NextIndex int    `json:"next_index"`
	Bytes     int64  `json:"bytes"`
}

// RecordingUpload is what a client reads to resume an upload: present while a browser recording receives chunks.
type RecordingUpload struct {
	Container     string                 `json:"container"`
	MaxChunkBytes int64                  `json:"max_chunk_bytes"`
	Tracks        []RecordingUploadTrack `json:"tracks"`
}

// upload is the state of a browser recording in this process. mu serialises the chunks of the recording; cmu guards
// the counters, so a read of the state never waits for a slow body.
type upload struct {
	mu sync.Mutex

	cmu       sync.Mutex
	next      map[recording.TrackKind]int
	bytes     map[recording.TrackKind]int64
	elapsedMS int64
	lastMS    int64
	savedAt   int64
}

func (u *upload) total() int64 {
	var n int64
	for _, b := range u.bytes {
		n += b
	}

	return n
}

// assembly is the work that makes the chunks of a recording a file.
type assembly struct {
	done    chan struct{}
	discard atomic.Bool
	cancel  context.CancelFunc
}

func diskLowError() *Error {
	return newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space to record.").
		withHint("free space on the disk of the workspace; the person running the server can lower "+
			"STUARTY_MIN_FREE_BYTES").
		withDetail("reason", "disk_low").withDetail("variable", "STUARTY_MIN_FREE_BYTES")
}

func (s *svc) recordingFolder(id string) string {
	return filepath.Join(s.workspace, recordingsDir, id)
}

func partPath(folder string, kind recording.TrackKind, index int) string {
	return filepath.Join(folder, partsDir, string(kind), fmt.Sprintf("%06d%s", index, partExt))
}

// scanParts counts the chunks of a track that are on the disk from 0 on without a gap, and their bytes.
func scanParts(folder string, kind recording.TrackKind) (next int, bytes int64, last time.Time) {
	for {
		st, err := os.Stat(partPath(folder, kind, next))
		if err != nil || !st.Mode().IsRegular() {
			return next, bytes, last
		}

		next++
		bytes += st.Size()

		if st.ModTime().After(last) {
			last = st.ModTime()
		}
	}
}

// uploadFor is the state of a browser recording, built from the disk the first time it is asked for.
func (s *svc) uploadFor(r recordings.Row) *upload {
	s.recMu.Lock()
	defer s.recMu.Unlock()

	if u := s.recUploads[r.ID]; u != nil {
		return u
	}

	u := &upload{next: map[recording.TrackKind]int{}, bytes: map[recording.TrackKind]int64{}, lastMS: r.StartedMs}
	folder := s.recordingFolder(r.ID)

	for _, t := range decodeTracks(r.Tracks) {
		n, b, last := scanParts(folder, t.Kind)
		u.next[t.Kind], u.bytes[t.Kind] = n, b

		if !last.IsZero() && last.UnixMilli() > u.lastMS {
			u.lastMS = last.UnixMilli()
		}
	}

	if s.recUploads == nil {
		s.recUploads = map[string]*upload{}
	}

	s.recUploads[r.ID] = u

	return u
}

func (s *svc) dropUpload(id string) {
	s.recMu.Lock()
	delete(s.recUploads, id)
	s.recMu.Unlock()
}

// uploadView is the state a client resumes from, and the live counters of a browser recording.
func (s *svc) uploadView(r recordings.Row) *RecordingUpload {
	u := s.uploadFor(r)

	u.cmu.Lock()
	defer u.cmu.Unlock()

	out := &RecordingUpload{
		Container: r.Encoder, MaxChunkBytes: recording.MaxChunkBytes, Tracks: []RecordingUploadTrack{},
	}

	for _, t := range decodeTracks(r.Tracks) {
		out.Tracks = append(out.Tracks, RecordingUploadTrack{
			Kind: string(t.Kind), NextIndex: u.next[t.Kind], Bytes: u.bytes[t.Kind],
		})
	}

	return out
}

// RecordingUploadBegin starts a recording that the browser captures and uploads. Only a person does, and the
// check is the first thing done, before the project or the disk is looked at (SDD-025 section 17.3).
func (s *svc) RecordingUploadBegin(ctx context.Context, req RecordingUploadBeginRequest) (*Recording, error) {
	c, err := s.person(ctx, "RecordingUploadBegin")
	if err != nil {
		return nil, err
	}

	spec := recording.UploadSpec{ProjectID: req.ProjectID, Container: req.Container, Camera: req.Camera}
	if fields := recording.ValidateUpload(spec); len(fields) > 0 {
		return nil, newError(command.CodeValidation, "%s is not valid (%s).", fields[0].Field, fields[0].Reason).
			withPath(fields[0].Field)
	}

	if _, err := s.authorizeProject(ctx, "RecordingUploadBegin", req.ProjectID); err != nil {
		return nil, err
	}

	if head, gerr := s.store.Productions().Get(ctx, req.ProjectID); gerr != nil || head.State == ProjectTrashed {
		return nil, projectNotFound(req.ProjectID, gerr)
	}

	if lerr := s.recordingLimits().CheckStart(s.freeBytes()); lerr != nil {
		return nil, diskLowError()
	}

	s.settleIdleUploads(ctx)

	asked, jerr := json.Marshal(recording.UploadTracks(spec))
	if jerr != nil {
		return nil, internalError()
	}

	started, _ := recording.Next(recording.Prepared, recording.EventStarted)
	id, now := s.newIDs("rec"), s.nowMs()

	row := recordings.Row{
		ID: id, ProjectID: req.ProjectID, State: string(started), SourceKind: recording.SourceBrowser,
		SourceLabel: recording.CleanLabel(req.Label, maxLabelRunes), Encoder: req.Container, Actor: c.Actor,
		CreatedMs: now, StartedMs: now, Tracks: string(asked),
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		open, err := tx.Recordings().Open(ctx)
		if err != nil {
			return err
		}

		mine, all := 0, 0

		for i := range open {
			// Only a recording that still receives chunks counts: one that is being assembled holds no tab.
			if open[i].SourceKind != recording.SourceBrowser || open[i].State != string(recording.Recording) {
				continue
			}

			all++

			if open[i].Actor == c.Actor {
				mine++
			}
		}

		if mine >= recording.MaxOpenUploadsPerActor {
			return newError(CodeQueueFull, "you already have a recording in progress in the browser.").
				withLimit("recordings", mine, recording.MaxOpenUploadsPerActor).
				withHint("stop or finish the recording in progress first")
		}

		if all >= recording.MaxOpenUploadsPerWorkspace {
			return newError(CodeQueueFull, "%d recordings are already being uploaded.", all).
				withLimit("recordings", all, recording.MaxOpenUploadsPerWorkspace).
				withHint("wait for one to finish")
		}

		if err := tx.Recordings().Insert(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "recording.upload_begin", req.ProjectID, id)
	})
	if werr != nil {
		return nil, asError(werr)
	}

	folder := s.recordingFolder(id)

	for _, t := range recording.UploadTracks(spec) {
		if err := os.MkdirAll(filepath.Join(folder, partsDir, string(t.Kind)), 0o700); err != nil {
			s.failRecording(ctx, id, CodeCaptureFailed, "io: the folder of the recording could not be made")

			return nil, newError(CodeCaptureFailed, "the folder of the recording could not be made.").
				withDetail("reason", "io")
		}
	}

	return s.liveView(row), nil
}

func uploadValidation(path, format string, args ...any) *Error {
	return newError(command.CodeValidation, format, args...).withPath(path)
}

func uploadState(reason, format string, args ...any) *Error {
	return newError(command.CodeInvalidState, format, args...).withDetail("reason", reason)
}

// RecordingUploadChunk stores one chunk of a track of a browser recording, in order and with a matching digest.
// The first answer for every caller that is not a person is E_PRIVILEGED_SESSION (SDD-025 section 17.4).
func (s *svc) RecordingUploadChunk(
	ctx context.Context, req RecordingUploadChunkRequest,
) (*RecordingUploadChunkResult, error) {
	c, err := s.person(ctx, "RecordingUploadChunk")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if r.SourceKind != recording.SourceBrowser || r.State != string(recording.Recording) {
		return nil, uploadState("not_receiving", "the recording is %s and receives no chunk.", r.State)
	}

	kind := recording.TrackKind(req.Track)
	if !recording.UploadKind(kind) || !hasTrack(decodeTracks(r.Tracks), kind) {
		return nil, uploadValidation(
			"track", "the track %q is not one this recording asked for.", command.SafeKey(req.Track),
		)
	}

	if !recording.ValidSHA256(req.SHA256) {
		return nil, uploadValidation("sha256", "sha256 is the lowercase hexadecimal digest of the chunk.")
	}

	if req.ElapsedMS < 0 {
		return nil, uploadValidation("elapsed_ms", "elapsed_ms is 0 or more.")
	}

	if req.Body == nil {
		return nil, uploadValidation("body", "the request carries no chunk.")
	}

	up := s.uploadFor(r)

	up.mu.Lock()
	defer up.mu.Unlock()

	// A finish or a cancel that came in while this request waited for the lock.
	if cur, gerr := s.store.Recordings().Find(ctx, r.ID); gerr != nil || cur.State != string(recording.Recording) {
		return nil, uploadState("not_receiving", "the recording is no longer receiving chunks.")
	}

	up.cmu.Lock()
	next, stored, elapsed := up.next[kind], up.total(), up.elapsedMS
	up.cmu.Unlock()

	folder := s.recordingFolder(r.ID)

	switch recording.JudgeChunk(next, req.Index) {
	case recording.ChunkMalformed:
		return nil, uploadValidation("index", "index is a chunk number, 0 or more.")
	case recording.ChunkTooMany:
		return nil, newError("E_LIMIT_EXCEEDED", "the chunk index %d is out of range.", req.Index).
			withDetail("reason", "chunks").withLimit("chunks", req.Index, recording.MaxChunksPerTrack-1)
	case recording.ChunkOutOfOrder:
		return nil, uploadState("out_of_order", "the track %s waits for chunk %d, not %d.", kind, next, req.Index).
			withDetail("expected", next).
			withHint("send the chunk the track waits for; read upload.tracks of the recording")
	case recording.ChunkRetry:
		return s.retriedChunk(r.ID, folder, kind, req, next, stored)
	case recording.ChunkAccept:
	}

	limits := s.recordingLimits()
	if why := limits.Check(max(elapsed, req.ElapsedMS), stored, s.freeBytes()); why != recording.StopNone {
		return nil, uploadLimit(why)
	}

	dir := filepath.Join(folder, partsDir, string(kind))
	tmp := filepath.Join(dir, incomingName)

	n, sum, werr := writeChunk(tmp, req.Body)
	if werr != nil {
		_ = os.Remove(tmp)

		return nil, werr
	}

	if sum != req.SHA256 {
		_ = os.Remove(tmp)

		return nil, uploadValidation("sha256", "the digest of the chunk is not the one declared.").
			withDetail("reason", "checksum_mismatch")
	}

	if !limits.FitsAfter(stored, n) {
		_ = os.Remove(tmp)

		return nil, uploadLimit(recording.StopSize)
	}

	if err := os.Rename(tmp, partPath(folder, kind, req.Index)); err != nil {
		_ = os.Remove(tmp)

		return nil, newError(CodeCaptureFailed, "the chunk could not be stored.").withDetail("reason", "io")
	}

	up.cmu.Lock()
	up.next[kind]++
	up.bytes[kind] += n
	up.elapsedMS = max(up.elapsedMS, req.ElapsedMS)
	up.lastMS = s.nowMs()
	res := &RecordingUploadChunkResult{
		ID: r.ID, Track: string(kind), Index: req.Index, NextIndex: up.next[kind], Bytes: up.total(),
	}
	save, elapsedNow, total := up.elapsedMS-up.savedAt >= progressSaveMS, up.elapsedMS, up.total()

	if save {
		up.savedAt = up.elapsedMS
	}
	up.cmu.Unlock()

	if save {
		_, _ = s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
			if x.State == string(recording.Recording) {
				x.DurationMs, x.Bytes = elapsedNow, total
			}

			return nil
		})
	}

	res.Text = "Chunk " + strconv.Itoa(req.Index) + " of the " + string(kind) + " track of " + r.ID + " is stored."

	return res, nil
}

func hasTrack(tracks []recording.Track, kind recording.TrackKind) bool {
	for _, t := range tracks {
		if t.Kind == kind {
			return true
		}
	}

	return false
}

// uploadLimit is the refusal for a limit of an upload; what was received is kept (SDD-025 section 17.4).
func uploadLimit(why recording.StopReason) *Error {
	switch why {
	case recording.StopDuration:
		return newError("E_LIMIT_EXCEEDED", "the recording is as long as it may be.").
			withDetail("reason", "duration").withDetail("variable", "STUARTY_MAX_RECORDING_SECONDS").
			withHint("finish the recording: what was received is kept")
	case recording.StopSize:
		return newError("E_LIMIT_EXCEEDED", "the recording is as large as it may be.").
			withDetail("reason", "size").withDetail("variable", "STUARTY_MAX_IMPORT_BYTES").
			withHint("finish the recording: what was received is kept")
	case recording.StopDisk, recording.StopNone:
	}

	return diskLowError()
}

// ErrUploadBodyTooLarge is what the body of a chunk reports when its reader crossed the limit of the route; the HTTP
// adapter maps the framework's own error to it, and any other caller of the Service may return it from a reader.
var ErrUploadBodyTooLarge = errors.New("service: the body of the chunk is over the limit")

func tooLarge(err error) bool {
	_, big := errors.AsType[*http.MaxBytesError](err)

	return big || errors.Is(err, ErrUploadBodyTooLarge)
}

// writeChunk copies a body into a new file, hashing as it goes, and reads at most the chunk limit plus one byte:
// memory is never the size of the body, and a body over the limit is refused when it crosses it.
func writeChunk(path string, body io.Reader) (int64, string, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, "", newError(CodeCaptureFailed, "the chunk could not be stored.").withDetail("reason", "io")
	}

	h := sha256.New()

	n, cerr := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, recording.MaxChunkBytes+1))
	if clerr := f.Close(); cerr == nil {
		cerr = clerr
	}

	switch {
	case cerr != nil:
		if tooLarge(cerr) {
			return 0, "", newError(CodePayloadTooLarge, "the chunk is over %d bytes.", recording.MaxChunkBytes)
		}

		return 0, "", newError(command.CodeValidation, "the chunk could not be read.").withPath("body")
	case n > recording.MaxChunkBytes:
		return 0, "", newError(CodePayloadTooLarge, "the chunk is over %d bytes.", recording.MaxChunkBytes).
			withLimit("chunk_bytes", n, recording.MaxChunkBytes)
	case n == 0:
		return 0, "", uploadValidation("body", "the chunk is empty.")
	}

	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// retriedChunk answers a chunk whose index is already stored: the same bytes are a success that changes nothing,
// other bytes are a conflict.
func (s *svc) retriedChunk(
	id, folder string, kind recording.TrackKind, req RecordingUploadChunkRequest, next int, stored int64,
) (*RecordingUploadChunkResult, error) {
	h := sha256.New()

	n, err := io.Copy(h, io.LimitReader(req.Body, recording.MaxChunkBytes+1))
	if err != nil && tooLarge(err) || n > recording.MaxChunkBytes {
		return nil, newError(CodePayloadTooLarge, "the chunk is over %d bytes.", recording.MaxChunkBytes)
	}

	if err != nil {
		return nil, uploadValidation("body", "the chunk could not be read.")
	}

	if n == 0 {
		return nil, uploadValidation("body", "the chunk is empty.")
	}

	if hex.EncodeToString(h.Sum(nil)) != req.SHA256 {
		return nil, uploadValidation("sha256", "the digest of the chunk is not the one declared.").
			withDetail("reason", "checksum_mismatch")
	}

	have, herr := fileDigest(partPath(folder, kind, req.Index))
	if herr != nil || have != req.SHA256 {
		return nil, uploadState(
			"chunk_conflict",
			"chunk %d of the %s track was stored with other bytes.",
			req.Index,
			kind,
		)
	}

	return &RecordingUploadChunkResult{
		ID: id, Track: string(kind), Index: req.Index, NextIndex: next, Duplicate: true, Bytes: stored,
		Text: "Chunk " + strconv.Itoa(req.Index) + " of " + id + " was already stored.",
	}, nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// RecordingUploadFinish ends the upload of a browser recording: the server checks that the chunks the browser says
// it sent are there, and assembles them in the background (SDD-025 section 17.5).
func (s *svc) RecordingUploadFinish(ctx context.Context, req RecordingUploadFinishRequest) (*Recording, error) {
	c, err := s.person(ctx, "RecordingUploadFinish")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if r.SourceKind != recording.SourceBrowser {
		return nil, uploadState("not_receiving", "the recording is not one that a browser uploads.")
	}

	if recording.State(r.State).Final() || r.State == string(recording.Finishing) {
		return s.liveView(r), nil
	}

	if req.ElapsedMS < 0 {
		return nil, uploadValidation("elapsed_ms", "elapsed_ms is 0 or more.")
	}

	got, err := s.finishUpload(ctx, r, "stopped", req.ElapsedMS, req.Tracks)
	if err != nil {
		return nil, err
	}

	s.auditRecording(ctx, c, "recording.upload_finish", got)

	return s.liveView(got), nil
}

// finishUpload moves a browser recording that receives chunks to finishing and starts the assembly. It serialises
// with the chunks, and checks the counts the browser declared against what is stored.
func (s *svc) finishUpload(
	ctx context.Context, r recordings.Row, reason string, elapsedMS int64, counts []RecordingUploadCount,
) (recordings.Row, error) {
	up := s.uploadFor(r)

	up.mu.Lock()
	defer up.mu.Unlock()

	asked := decodeTracks(r.Tracks)

	up.cmu.Lock()
	for _, n := range counts {
		kind := recording.TrackKind(n.Kind)
		if !recording.UploadKind(kind) || !hasTrack(asked, kind) {
			up.cmu.Unlock()

			return r, uploadValidation(
				"tracks",
				"the track %q is not one this recording asked for.",
				command.SafeKey(n.Kind),
			)
		}

		if n.Chunks != up.next[kind] {
			have := up.next[kind]
			up.cmu.Unlock()

			return r, uploadState("chunks_missing", "the %s track has %d chunks stored and the browser sent %d.",
				kind, have, n.Chunks).
				withDetail("kind", n.Kind).withDetail("expected", n.Chunks).withDetail("received", have).
				withHint("send the missing chunks, or stop the recording to keep what the server has")
		}
	}

	elapsed := max(up.elapsedMS, elapsedMS)
	up.cmu.Unlock()

	var began bool

	got, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
		if x.State == string(recording.Finishing) {
			return nil
		}

		next, terr := recording.Next(recording.State(x.State), recording.EventStop)
		if terr != nil {
			return uploadState("not_receiving", "the recording is %s and cannot be finished.", x.State)
		}

		x.State, began = string(next), true
		if x.StopReason == "" {
			x.StopReason = reason
		}

		if elapsed > 0 {
			x.DurationMs = elapsed
		}

		return nil
	})
	if uerr != nil {
		return r, asError(uerr)
	}

	if began {
		s.startAssembly(ctx, r.ID, "")
	}

	return got, nil
}

// startAssembly runs the assembly of a recording in the background. force is the stop reason the recovery of
// start-up writes over the one the row has.
func (s *svc) startAssembly(ctx context.Context, id, force string) {
	actx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	a := &assembly{done: make(chan struct{}), cancel: cancel}

	s.recMu.Lock()
	if s.recAssembling == nil {
		s.recAssembling = map[string]*assembly{}
	}

	s.recAssembling[id] = a
	s.recMu.Unlock()

	go func() {
		defer func() {
			cancel()

			s.recMu.Lock()
			delete(s.recAssembling, id)
			s.recMu.Unlock()

			close(a.done)
		}()

		s.settleUpload(actx, id, a, force)
	}()
}

// settleIdleUploads finishes the browser recordings that received nothing for the idle time: the tab that
// recorded them is gone (SDD-025 section 17.6). It runs before a new upload begins.
func (s *svc) settleIdleUploads(ctx context.Context) {
	open, err := s.store.Recordings().Open(ctx)
	if err != nil {
		return
	}

	now := s.nowMs()

	for i := range open {
		r := open[i]
		if r.SourceKind != recording.SourceBrowser || r.State != string(recording.Recording) {
			continue
		}

		up := s.uploadFor(r)

		up.cmu.Lock()
		idle := now-up.lastMS >= recording.UploadIdleSeconds*1000
		up.cmu.Unlock()

		if idle {
			_, _ = s.finishUpload(ctx, r, "abandoned", 0, nil)
		}
	}
}

// uploadStop is the stop of a browser recording, for any caller with the media scope: it keeps what was received.
func (s *svc) uploadStop(ctx context.Context, r recordings.Row) (*Recording, error) {
	if r.State != string(recording.Recording) {
		return nil, newError(command.CodeInvalidState, "the recording is %s and cannot be stopped.", r.State)
	}

	got, err := s.finishUpload(ctx, r, "stopped", 0, nil)
	if err != nil {
		return nil, err
	}

	return s.liveView(got), nil
}

// uploadDiscard is the cancel of a browser recording that is not final: it is discarded with its chunks, and an
// assembly that is running is stopped.
func (s *svc) uploadDiscard(ctx context.Context, r recordings.Row) (*Recording, error) {
	s.recMu.Lock()
	a := s.recAssembling[r.ID]
	s.recMu.Unlock()

	if a != nil {
		a.discard.Store(true)
		a.cancel()

		select {
		case <-a.done:
		case <-time.After(15 * time.Second):
		}
	} else {
		up := s.uploadFor(r)
		up.mu.Lock()

		_, uerr := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
			next, terr := recording.Next(recording.State(x.State), recording.EventCancel)
			if terr != nil {
				return newError(command.CodeInvalidState, "the recording is %s and cannot be cancelled.", x.State)
			}

			x.State, x.Review, x.EndedMs = string(next), "", s.nowMs()

			return nil
		})

		up.mu.Unlock()

		if uerr != nil {
			return nil, asError(uerr)
		}

		_ = os.RemoveAll(s.recordingFolder(r.ID))
		s.dropUpload(r.ID)
	}

	got, err := s.store.Recordings().Get(ctx, r.ID)
	if err != nil {
		return nil, asError(err)
	}

	return s.liveView(got), nil
}

// waitAssemblies waits for the assemblies that are running, up to the time given; one that is not done stays
// `finishing` and is assembled again by the recovery of the next start.
func (s *svc) waitAssemblies(within time.Duration) {
	s.recMu.Lock()
	all := make([]*assembly, 0, len(s.recAssembling))

	for _, a := range s.recAssembling {
		all = append(all, a)
	}
	s.recMu.Unlock()

	limit := time.After(within)

	for _, a := range all {
		select {
		case <-a.done:
		case <-limit:
			return
		}
	}
}

// recoverUpload settles a browser recording the last process left open: its chunks are assembled when an FFmpeg is
// there, and the recording fails with the reason when it is not (SDD-025 section 17.6).
func (s *svc) recoverUpload(ctx context.Context, r recordings.Row) error {
	if r.State == string(recording.Recording) {
		if _, err := s.updateRecording(ctx, r.ID, func(x *recordings.Row) error {
			next, terr := recording.Next(recording.State(x.State), recording.EventStop)
			if terr != nil {
				return terr
			}

			x.State = string(next)

			return nil
		}); err != nil {
			return err
		}
	}

	s.settleUpload(ctx, r.ID, nil, "recovered")

	return nil
}

// settleUpload assembles the chunks of a recording that is finishing and writes the end of the row. A discarded
// recording is removed. force, when set, is the stop reason written over the one the row has.
func (s *svc) settleUpload(ctx context.Context, id string, a *assembly, force string) {
	row, err := s.store.Recordings().Find(ctx, id)
	if err != nil {
		return
	}

	folder := s.recordingFolder(id)
	res := s.assembleUpload(ctx, row)

	if a != nil && a.discard.Load() {
		_, _ = s.updateRecording(context.WithoutCancel(ctx), id, func(x *recordings.Row) error {
			x.State, x.Review, x.EndedMs = string(recording.Cancelled), "", s.nowMs()

			return nil
		})
		_ = os.RemoveAll(folder)
		s.dropUpload(id)

		return
	}

	_, _ = s.updateRecording(context.WithoutCancel(ctx), id, func(x *recordings.Row) error {
		x.EndedMs = s.nowMs()

		if force != "" {
			x.StopReason = force
		}

		if res.failure != "" {
			if next, terr := recording.Next(recording.State(x.State), recording.EventFailed); terr == nil {
				x.State, x.Review = string(next), ""
			}

			x.ErrorCode, x.ErrorMessage = CodeCaptureFailed, res.failure

			return nil
		}

		// A row that is no longer finishing (a cancel came first) is left as it is.
		if next, terr := recording.Next(recording.State(x.State), recording.EventFinished); terr == nil {
			tj, _ := json.Marshal(res.tracks)
			x.State, x.Review, x.Tracks = string(next), string(recording.ReviewPending), string(tj)
			x.Width, x.Height, x.DurationMs, x.Bytes = res.width, res.height, res.durationMS, res.bytes

			if x.StopReason == "" {
				x.StopReason = "stopped"
			}
		}

		return nil
	})

	if res.failure == "" {
		_ = os.RemoveAll(filepath.Join(folder, partsDir))
	}

	s.dropUpload(id)
}

// uploadResult is what the assembly made, or why it could not: the failure starts with the reason of
// E_CAPTURE_FAILED (SDD-025 section 17.5).
type uploadResult struct {
	tracks        []recording.Track
	width, height int
	durationMS    int64
	bytes         int64
	failure       string
}

func failedUpload(reason, format string, args ...any) uploadResult {
	return uploadResult{failure: reason + ": " + recording.CleanLabel(fmt.Sprintf(format, args...), 400)}
}
