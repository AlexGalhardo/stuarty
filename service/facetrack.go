package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/engine/aihost"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// JobFaceTrack is the kind of a face track job (SDD-024 section 6).
const JobFaceTrack = "face_track"

const (
	defaultFaceFPS = 5
	// faceArtifactKind is the kind of the artifact a face track job stores.
	faceArtifactKind = "facetrack"
	faceInstallText  = "stuarty tools:install " + aihost.SetName
)

// FaceTrackArgs are the arguments of JobStart for a face track.
type FaceTrackArgs struct {
	AssetID string `json:"asset_id"`
	// FPS is the frames analysed per second, 1 to 10; 5 when zero.
	FPS int `json:"fps,omitempty"`
	// StartMS and EndMS are the range of the asset; the whole asset when both are zero.
	StartMS int `json:"start_ms,omitempty"`
	EndMS   int `json:"end_ms,omitempty"`
}

// FaceTrackResult is the result of a finished face track job.
type FaceTrackResult struct {
	AssetID  string `json:"asset_id"`
	Samples  int    `json:"samples"`
	Faces    int    `json:"faces"`
	Provider string `json:"provider"`
	FPS      int    `json:"fps"`
}

// FaceDetector finds the faces of a range of a video. The product's one is the aihost.Host over the
// verified files of the face set; a test supplies a fake.
type FaceDetector interface {
	Detect(ctx context.Context, in *aihost.Input, progress func(done, total int)) (facetrack.Track, error)
}

type faceTrackInput struct {
	AssetID string `json:"asset_id"`
	FPS     int    `json:"fps"`
	StartMS int    `json:"start_ms"`
	EndMS   int    `json:"end_ms"`
	Key     string `json:"key"`
}

func faceKey(fps, start, end int) string {
	return fmt.Sprintf("%s-%d-%d-%d", aihost.ModelName, fps, start, end)
}

func decodeFaceTrackArgs(raw json.RawMessage) (FaceTrackArgs, *Error) {
	var a FaceTrackArgs

	if len(bytes.TrimSpace(raw)) == 0 {
		return a, newError(command.CodeValidation, "args.asset_id is required.").
			withPath("args.asset_id").withHint("name the asset to analyse")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&a); err != nil {
		return a, newError(command.CodeValidation, "the arguments of a face track are not valid: %s.",
			command.JSONProblem(err)).withPath("args")
	}

	if a.AssetID == "" {
		return a, newError(command.CodeValidation, "args.asset_id is required.").withPath("args.asset_id")
	}

	if a.FPS == 0 {
		a.FPS = defaultFaceFPS
	}

	if a.FPS < 1 || a.FPS > 10 {
		return a, newError(command.CodeValidation, "args.fps is %d; it must be 1 to 10.", a.FPS).withPath("args.fps")
	}

	if a.StartMS < 0 || a.EndMS < 0 || a.EndMS != 0 && a.EndMS <= a.StartMS {
		return a, newError(command.CodeValidation, "args.start_ms and args.end_ms do not make a range.").
			withPath("args.end_ms").withHint("give end_ms after start_ms, or neither for the whole asset")
	}

	return a, nil
}

// faceUnavailable is E_TRANSCRIBE_UNAVAILABLE for the face set: what is missing, its size and the command
// that installs it. It downloads nothing.
func (s *svc) faceUnavailable() *Error {
	e := newError(codeTranscribeOff,
		"Face tracking needs a local helper, its runtime and a face model, and they are not installed.").
		withHint("Run %q (or use the install dialog of the editor).", faceInstallText).
		withDetail("install_command", faceInstallText).
		withDetail("tool", aihost.SetName).withDetail("model", aihost.ModelVersion)

	offer := s.faceOffer()
	if offer == nil {
		return newError(codeTranscribeOff, "Face tracking is not available on this platform.").
			withDetail("tool", aihost.SetName).withDetail("model", aihost.ModelVersion)
	}

	missing := make([]map[string]any, 0, len(offer.Files))
	for _, f := range offer.Files {
		missing = append(missing, map[string]any{"name": f.Name, "url_host": f.Host, "bytes": f.Bytes})
	}

	return e.withDetail("missing", missing).withDetail("download_bytes", offer.DownloadBytes)
}

// faceReady is true when a detector can run now: an injected one, or the verified face set.
func (s *svc) faceReady() bool {
	if s.faces != nil {
		return true
	}

	_, ok := s.tools.FaceStack()

	return ok
}

// faceDetector is the detector a job runs: the injected one, or a host over the files that verify now.
func (s *svc) faceDetector() (FaceDetector, error) {
	if s.faces != nil {
		return s.faces, nil
	}

	paths, ok := s.tools.FaceStack()
	if !ok || s.media.Runner == nil {
		return nil, s.faceUnavailable()
	}

	return aihost.New(aihost.Config{
		Runner: s.media.Runner, Helper: paths.Helper, Runtime: paths.Runtime, Model: paths.Model,
	})
}

// faceVideoArtifact is the file a face track decodes: the proxy of the asset when there is one, else the
// normalized video.
func faceVideoArtifact(arts []ArtifactRow) (ArtifactRow, bool) {
	for _, kind := range []string{"proxy", "normalized"} {
		if i := slices.IndexFunc(arts, func(a ArtifactRow) bool { return a.Kind == kind && a.Ext != "json" }); i >= 0 {
			return arts[i], true
		}
	}

	return ArtifactRow{}, false
}

// startFaceTrack is JobStart for kind face_track. It refuses at once, with no job row and no download, when
// the set is not installed.
func (s *svc) startFaceTrack(ctx context.Context, c Caller, req JobStartRequest) (*Job, error) {
	args, aerr := decodeFaceTrackArgs(req.Args)
	if aerr != nil {
		return nil, aerr
	}

	project, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if project.State == ProjectTrashed {
		return nil, projectInTrash(project.ID)
	}

	pa, err := s.store.Assets().GetProjectAsset(ctx, project.ID, args.AssetID)
	if err != nil || pa.State == ProjectAssetTrashed {
		return nil, newError(command.CodeNotFound, "the project has no asset %s.", args.AssetID).
			withPath("args.asset_id").withHint("list the assets with media_inspect and no asset_id")
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return nil, asError(err)
	}

	if ar.IngestState != IngestReady {
		return nil, newError("E_ASSET_NOT_READY", "the asset is %s; it cannot be analysed yet.", ar.IngestState).
			withHint("wait for the asset to be ready")
	}

	if ar.Kind != "video" || ar.Facts.DurationMs == nil || ar.Facts.Width == nil || ar.Facts.Height == nil {
		return nil, newError(command.CodeInvalidState, "the asset %s is not a video with a known size.", args.AssetID).
			withPath("args.asset_id")
	}

	duration := int(*ar.Facts.DurationMs)
	if args.EndMS == 0 {
		args.StartMS, args.EndMS = 0, duration
	}

	args.EndMS = min(args.EndMS, duration)
	if args.StartMS >= args.EndMS {
		return nil, newError(
			command.CodeValidation,
			"args.start_ms is past the end of the asset.",
		).withPath("args.start_ms")
	}

	if secs := int64((args.EndMS - args.StartMS) / 1000); secs > s.limits.MaxTranscribeSeconds {
		return nil, newError("E_LIMIT_EXCEEDED", "The range is %d seconds; the limit is %d seconds "+
			"(STUARTY_MAX_TRANSCRIBE_SECONDS).", secs, s.limits.MaxTranscribeSeconds).
			withLimit("transcribe_seconds", secs, s.limits.MaxTranscribeSeconds).
			withDetail("variable", "STUARTY_MAX_TRANSCRIBE_SECONDS").
			withHint("analyse a shorter range with start_ms and end_ms")
	}

	if !s.faceReady() {
		return nil, s.faceUnavailable()
	}

	key := faceKey(args.FPS, args.StartMS, args.EndMS)
	projectID, hash := project.ID, pa.AssetHash

	job := JobRow{
		ID: s.newIDs(
			"job",
		),
		Kind:      JobFaceTrack,
		ProjectID: &projectID,
		AssetHash: &hash,
		Actor:     c.Actor,
		State:     JobQueued,
		Input: marshalOrNull(faceTrackInput{
			AssetID: pa.AssetID, FPS: args.FPS, StartMS: args.StartMS, EndMS: args.EndMS, Key: key,
		}),
	}

	if req.DryRun {
		return s.dryRunView(c, job), nil
	}

	lim := &JobLimit{Kind: JobFaceTrack, Max: 1, Actor: c.Actor, MaxPerActor: 1}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		job.CreatedMs, job.UpdatedMs = now, now

		if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
			return err
		}

		if err := tx.Audit().Append(ctx, AuditRow{
			AtMs: now, Actor: c.Actor, Action: "job.start", ProjectID: &projectID, SubjectID: &job.ID,
		}); err != nil {
			return err
		}

		queued := job
		tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

		if s.enqueue != nil {
			return s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, callerError(c, asError(err))
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	row, gerr := s.store.Jobs().Get(ctx, job.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobView(c, row, req.WaitMS > 0), nil
}

// runFaceTrack is the handler of a face track job: find the stored artifact or run the helper, validate,
// write, record. The same asset, range, fps and model give the same artifact, so a repeated job returns at
// once.
func (s *svc) runFaceTrack(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	var in faceTrackInput
	if job.AssetHash == nil || json.Unmarshal(job.Input, &in) != nil {
		return nil, internalError()
	}

	hash := *job.AssetHash
	hexHash := strings.TrimPrefix(hash, "sha256:")

	arts, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil, err
	}

	if body := s.readArtifact(arts, faceArtifactKind, in.Key); body != nil {
		return faceResult(in, body)
	}

	detector, derr := s.faceDetector()
	if derr != nil {
		return nil, derr
	}

	ar, err := s.store.Assets().Get(ctx, hash)
	if err != nil {
		return nil, err
	}

	video, ok := faceVideoArtifact(arts)
	if !ok || ar.Facts.Width == nil || ar.Facts.Height == nil {
		return nil, newError(command.CodeInvalidState, "the asset has no video to analyse yet.")
	}

	source, ok := s.artifactPath(video.RelPath)
	if !ok {
		return nil, internalError()
	}

	if cerr := s.checkDisk(); cerr != nil {
		return nil, cerr
	}

	var runDir *ffmpeg.RunDir

	if s.media.Runner != nil {
		if runDir, err = s.media.Runner.NewRunDir("facetrack-" + job.ID); err != nil {
			return nil, err
		}

		defer func() { _ = runDir.Remove() }()
	}

	pw.report(ctx, 0.03, "starting the face helper")

	track, terr := detector.Detect(ctx, &aihost.Input{
		Source:    source,
		InputDirs: []string{filepath.Join(s.workspace, "artifacts", hexHash)},
		WorkDir:   runDir,
		Width:     *ar.Facts.Width,
		Height:    *ar.Facts.Height,
		StartMS:   int64(in.StartMS),
		EndMS:     int64(in.EndMS),
		FPS:       in.FPS,
	}, func(done, total int) {
		if total > 0 {
			pw.report(ctx, 0.05+0.9*float64(done)/float64(total), "looking for faces")
		}
	})
	if terr != nil {
		return nil, faceFailure(terr)
	}

	if verr := track.Validate(); verr != nil {
		return nil, newError(
			command.CodeInternal,
			"the face track failed verification.",
		).withDetail("reason", "protocol")
	}

	pw.report(ctx, 0.97, "writing the face track")

	body, merr := json.Marshal(track)
	if merr != nil {
		return nil, internalError()
	}

	rel := "artifacts/" + hexHash + "/" + faceArtifactKind + "-1-" + in.Key + ".json"
	if werr := writeFileAtomic(filepath.Join(s.workspace, filepath.FromSlash(rel)), body); werr != nil {
		return nil, internalError()
	}

	sum := sha256.Sum256(body)

	if perr := s.store.Assets().PutArtifact(ctx, ArtifactRow{
		AssetHash: hash, Kind: faceArtifactKind, Profile: 1, Variant: in.Key, Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), CreatedMs: s.nowMs(),
	}); perr != nil {
		return nil, perr
	}

	return faceResult(in, body)
}

// faceFailure turns an error of the host into a service error with the reason of the helper.
func faceFailure(err error) error {
	if e, ok := errors.AsType[*aihost.Error](err); ok {
		return newError(command.CodeInternal, "Face tracking failed: %s.", e.Message).withDetail("reason", e.Reason)
	}

	return err
}

func faceResult(in faceTrackInput, body []byte) (json.RawMessage, error) {
	var t facetrack.Track
	if json.Unmarshal(body, &t) != nil || t.Validate() != nil {
		return nil, newError(
			command.CodeInternal,
			"the stored face track cannot be read.",
		).withDetail("reason", "output_missing")
	}

	faces := 0
	for i := range t.Samples {
		faces += len(t.Samples[i].Faces)
	}

	raw, err := json.Marshal(JobResult{FaceTrack: &FaceTrackResult{
		AssetID: in.AssetID, Samples: len(t.Samples), Faces: faces, Provider: t.Provider, FPS: t.FPS,
	}})
	if err != nil {
		return nil, internalError()
	}

	return raw, nil
}
