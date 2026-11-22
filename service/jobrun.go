package service

import (
	"context"
	"crypto/sha256"
	hexenc "encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service/logging"
)

const (
	cancelPollInterval = 500 * time.Millisecond
	progressInterval   = 250 * time.Millisecond
	progressStep       = 0.01
	finishTimeout      = 15 * time.Second
	maxAttempts        = 3
	staleQueuedAge     = time.Minute
	msgShutdown        = "interrupted by shutdown"
	msgRestarted       = "restarted after an interruption"
)

// emitJob publishes job.updated: on the stream of the project of the job, or,
// for an ingest job, on the stream of every project that holds its file.
func (s *svc) emitJob(ctx context.Context, j *JobRow) {
	ev := Event{
		Type: "job.updated", JobID: j.ID, Kind: j.Kind, State: j.State, Progress: j.Progress, Message: j.Message,
		Error: EventError(parseError(j.Error)), At: s.now(),
	}

	if j.Kind == JobExport && len(j.Result) > 0 {
		var res JobResult
		if json.Unmarshal(j.Result, &res) == nil {
			ev.Result = &res
		}
	}

	if j.ProjectID != nil {
		ev.ProjectID = *j.ProjectID
		s.emit(ev)

		return
	}

	if j.AssetHash == nil {
		// No project has a stream for it, and a webhook still hears that the job ended (SDD-022 section 3).
		s.fanWebhooks(ev)

		return
	}

	refs := s.referrers(ctx, *j.AssetHash)
	if len(refs) == 0 {
		s.fanWebhooks(ev)

		return
	}

	for i := range refs {
		ev.ProjectID, ev.AssetID = refs[i].ProjectID, refs[i].AssetID
		s.emit(ev)
	}
}

// emitCurrent re-reads a job and publishes it.
func (s *svc) emitCurrent(ctx context.Context, id string) {
	if j, err := s.store.Jobs().Get(ctx, id); err == nil {
		s.emitJob(ctx, &j)
	}
}

// progress throttles the writes of a running job (SDD-005 section 14.6): at
// most one every 250 ms, and only when the value moved by 0.01 or the message
// changed. Throttling uses the wall clock, not the service clock, which the
// tests hold still.
type progress struct {
	s    *svc
	job  *JobRow
	mu   sync.Mutex
	last time.Time
	val  float64
	msg  string
	seen bool
}

func (p *progress) report(ctx context.Context, v float64, msg string) {
	v = min(max(v, 0), 0.99)

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.seen && (now.Sub(p.last) < progressInterval || math.Abs(v-p.val) < progressStep && msg == p.msg) {
		return
	}

	// A step back is never written: progress only grows.
	if p.seen && v < p.val {
		v = p.val
	}

	p.seen, p.last, p.val, p.msg = true, now, v, msg

	updated, err := p.s.store.Jobs().Progress(ctx, p.job.ID, v, msg, p.s.nowMs())
	if err != nil || !updated {
		return
	}

	cur := *p.job
	cur.State, cur.Progress, cur.Message = JobRunning, v, msg
	p.s.emitJob(ctx, &cur)

	if ingestsAsset(p.job) {
		if p.s.store.Assets().SetIngest(ctx, *p.job.AssetHash, IngestIngesting, v, nil, p.s.nowMs()) == nil {
			p.s.emitAsset(ctx, *p.job.AssetHash, IngestIngesting, v, nil)
		}
	}
}

// RunJob is the handler of the queue messages (SDD-005 section 14.3). It
// claims the job, runs it with a context that a cancellation can end, and
// finishes it. A job never ends because a handler returned an error: the
// outcome is written here.
func (s *svc) RunJob(ctx context.Context, jobID string) (err error) {
	job, err := s.store.Jobs().Get(ctx, jobID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return err
	}

	if sem := s.sems[job.Kind]; sem != nil {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	claimed, err := s.store.Jobs().Claim(ctx, jobID, s.nowMs())
	if err != nil {
		return err
	}

	if !claimed {
		s.settleUnclaimed(ctx, jobID)

		return nil
	}

	// A panic of a handler fails the job: the queue would only fail the message
	// and leave the row running, holding a concurrency slot for ever.
	defer func() {
		if p := recover(); p != nil {
			s.logJob(ctx, slog.LevelError, "job panicked", &job,
				slog.Any("panic", p), slog.String("stack", string(debug.Stack())))

			fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
			defer fcancel()

			_ = s.finishFailed(fctx, &job, JobFailed, internalError())
			err = nil
		}
	}()

	fresh, gerr := s.store.Jobs().Get(ctx, jobID)
	if gerr != nil {
		fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
		defer fcancel()

		_ = s.finishFailed(fctx, &job, JobFailed, internalError())

		return gerr
	}

	job = fresh

	jctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.running.add(jobID, cancel)
	defer s.running.remove(jobID)

	stop := make(chan struct{})
	defer close(stop)

	go s.watchCancel(jctx, cancel, jobID, stop)

	s.emitJob(ctx, &job)

	s.logJob(ctx, slog.LevelInfo, "job started", &job)

	if ingestsAsset(&job) {
		if s.store.Assets().SetIngest(ctx, *job.AssetHash, IngestIngesting, 0, nil, s.nowMs()) == nil {
			s.emitAsset(ctx, *job.AssetHash, IngestIngesting, 0, nil)
		}
	}

	pw := &progress{s: s, job: &job}

	var (
		result json.RawMessage
		herr   error
	)

	switch job.Kind {
	case JobIngest:
		if isProfileUpgrade(&job) {
			result, herr = s.runPeaksUpgrade(jctx, &job, pw)

			break
		}

		result, herr = s.runIngest(jctx, &job, pw)
	case JobExport:
		result, herr = s.runExport(jctx, &job, pw)
	case JobExportBatch:
		result, herr = s.runExportBatch(jctx, &job, pw)
	case JobTranscribe:
		result, herr = s.runTranscribe(jctx, &job, pw)
	default:
		if k := s.kindOf(job.Kind); k != nil {
			var cost *core.Money

			result, cost, herr = s.runApprovable(jctx, k, &job, pw)

			// The ledger is settled after the outcome is final: a run a shutdown interrupted is requeued
			// and keeps its reservation (SDD-015 section 6).
			serr := s.settle(ctx, jctx, &job, result, herr)
			s.finishCost(ctx, &job, cost)

			return serr
		}

		herr = newError(command.CodeInternal, "jobs of the kind %s are not available in this build.", job.Kind)
	}

	return s.settle(ctx, jctx, &job, result, herr)
}

// settleUnclaimed handles a delivery that found the job not queued: another
// delivery owns it or it is final (nothing to do), or a queued job whose
// cancellation was requested but not yet recorded (finish it).
func (s *svc) settleUnclaimed(ctx context.Context, id string) {
	row, err := s.store.Jobs().Get(ctx, id)
	if err != nil || row.State != JobQueued || !row.CancelRequested {
		return
	}

	if after, cerr := s.store.Jobs().RequestCancel(ctx, id, s.nowMs()); cerr == nil && after.State == JobCancelled {
		s.afterQueuedCancel(ctx, &after)
	}
}

// watchCancel reads cancel_requested every 500 ms, which covers a job that
// was cancelled by another path than the in-process registry.
func (s *svc) watchCancel(ctx context.Context, cancel context.CancelFunc, id string, stop <-chan struct{}) {
	t := time.NewTicker(cancelPollInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-t.C:
			if row, err := s.store.Jobs().Get(ctx, id); err == nil && row.CancelRequested {
				cancel()

				return
			}
		}
	}
}

// settle writes the outcome of a run: succeeded, cancelled, failed, or back to
// queued when a shutdown interrupted it. Writes use a context that survives the
// shutdown, so the outcome is recorded.
func (s *svc) settle(parent, jctx context.Context, job *JobRow, result json.RawMessage, herr error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), finishTimeout)
	defer cancel()

	now := s.nowMs()

	if herr == nil {
		ok, err := s.store.Jobs().Finish(ctx, job.ID, JobSucceeded, result, nil, now)
		if err == nil && ok {
			s.logJob(ctx, slog.LevelInfo, "job succeeded", job)
			s.emitCurrent(ctx, job.ID)
		}

		return err
	}

	if jctx.Err() != nil {
		row, err := s.store.Jobs().Get(ctx, job.ID)
		if err == nil && row.CancelRequested {
			return s.finishFailed(ctx, job, JobCancelled, newError(CodeCancelled, "the job was cancelled."))
		}

		// A queue deadline ends the job; only a shutdown requeues it.
		if perr := parent.Err(); perr != nil {
			if errors.Is(perr, context.DeadlineExceeded) {
				return s.finishFailed(ctx, job, JobFailed, timeoutError(job.Kind))
			}

			s.requeueInterrupted(ctx, job)

			return perr
		}
	}

	return s.finishFailed(ctx, job, JobFailed, fromEngine(herr))
}

// finishFailed records a failed or cancelled job and, for an ingest, the
// failed asset with the same error (SDD-011 section 2.6).
func (s *svc) finishFailed(ctx context.Context, job *JobRow, state string, e *Error) error {
	ej := errorJSON(e)

	ok, err := s.store.Jobs().Finish(ctx, job.ID, state, nil, ej, s.nowMs())
	if err != nil {
		return err
	}

	if ok {
		s.noteUpgradeEnded(job, state)
	}

	if ok && ingestsAsset(job) {
		if aerr := s.store.Assets().SetIngest(ctx, *job.AssetHash, IngestFailed, 0, ej, s.nowMs()); aerr == nil {
			s.emitAsset(ctx, *job.AssetHash, IngestFailed, 0, e)
		}
	}

	if ok {
		s.logJob(
			ctx, levelOf(state), "job "+state, job,
			slog.String("code", e.Code), slog.Any("reason", e.Details["reason"]),
		)
		s.emitCurrent(ctx, job.ID)
	}

	return nil
}

// requeueInterrupted puts a job a shutdown interrupted back in the queue; the
// next start runs it from the beginning (SDD-005 section 14.5).
func (s *svc) requeueInterrupted(ctx context.Context, job *JobRow) {
	ok, err := s.store.Jobs().Requeue(ctx, job.ID, msgShutdown, s.nowMs())
	if err != nil || !ok {
		return
	}

	s.logJob(ctx, slog.LevelWarn, "job requeued", job)

	if ingestsAsset(job) {
		if s.store.Assets().SetIngest(ctx, *job.AssetHash, IngestPending, 0, nil, s.nowMs()) == nil {
			s.emitAsset(ctx, *job.AssetHash, IngestPending, 0, nil)
		}
	}

	s.emitCurrent(ctx, job.ID)
}

// missingFFmpeg is the discovery error when ffmpeg or ffprobe cannot be used.
// The real ingest panics on a nil runner; a substitute ingester does not use one.
func (s *svc) missingFFmpeg() error {
	if s.media.FFmpegMissing != nil {
		return s.media.FFmpegMissing
	}

	if s.media.Runner != nil || !s.defaultIngest {
		return nil
	}

	return &ffmpeg.Error{
		Code:    ffmpeg.CodeFFmpegMissing,
		Reason:  "ffmpeg_missing",
		Message: "No usable FFmpeg was found (looked at: the configured path, next to the program, the PATH).",
		Hint: `Install FFmpeg 7 or newer, or set ` + ffmpeg.EnvFFmpeg +
			` to its path. Run "stuarty doctor" for details.`,
	}
}

// runIngest is steps 1 to 7 of SDD-011 section 7.1 for one file, then the
// asset.add batches of section 2.5. A file whose asset is already ready (a
// crash between the registration and the end of the job) skips straight to the
// batches.
func (s *svc) runIngest(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	if job.AssetHash == nil {
		return nil, internalError()
	}

	hash := *job.AssetHash
	hex := strings.TrimPrefix(hash, "sha256:")
	timings := map[string]int64{}
	var warnings []string

	ar, err := s.store.Assets().Get(ctx, hash)
	if err != nil {
		return nil, err
	}

	if ar.IngestState != IngestReady {
		if derr := s.checkDisk(); derr != nil {
			return nil, derr
		}

		in := ingest.Input{
			Runner:       s.media.Runner,
			Caps:         s.media.Caps,
			Hash:         hex,
			ArtifactsDir: filepath.Join(s.workspace, "artifacts"),
			JobID:        job.ID,
			Ceilings:     s.limits.Ceilings,
			SoundMaps:    true,
			OnProgress:   func(p ingest.Progress) { pw.report(ctx, p.Fraction, p.Message) },
		}

		isLUT, lerr := s.ingestLUT(ctx, hash)
		if lerr != nil {
			return nil, lerr
		}

		if !isLUT {
			if miss := s.missingFFmpeg(); miss != nil {
				return nil, miss
			}

			res, rerr := s.media.Ingest(ctx, in)
			if rerr != nil {
				return nil, rerr
			}

			if err := s.registerIngest(ctx, hash, res); err != nil {
				return nil, err
			}

			timings, warnings = res.TimingsMS, res.Warnings
		}

		s.emitAsset(ctx, hash, IngestReady, 1, nil)
	}

	warnings = append(warnings, s.completePending(ctx, hash, job.ID)...)

	return marshalOrNull(map[string]any{"timings_ms": timings, "warnings": warnings}), nil
}

// ingestLUT handles an original that is a .cube table: it is read and checked in full, and the asset is
// ready as a lut with no artifact and no picture facts (SDD-011 section 7.1). A file that does not look
// like a .cube table is left to the media ingest (false, nil); one that does and is not valid fails the
// job (E_INGEST_FAILED, reason decode_failed).
func (s *svc) ingestLUT(ctx context.Context, hash string) (bool, error) {
	if s.media.Content == nil {
		return false, nil
	}

	f, err := s.media.Content.Open(strings.TrimPrefix(hash, "sha256:"))
	if err != nil {
		return false, err
	}

	defer func() { _ = f.Close() }()

	head := make([]byte, cubeSniffWindow)

	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}

	if !sniffCube(head[:n]) {
		return false, nil
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}

	if _, err := checkCube(f); err != nil {
		return false, &Error{
			Code: CodeIngestFailed, Message: "the file is not a valid .cube table: " + err.Error() + ".",
			Hint:    "export a 3D table (LUT_3D_SIZE 2 to 65) as a .cube file from the grading tool",
			Details: map[string]any{"reason": "decode_failed", "step": "probe"},
		}
	}

	art, err := s.publishCube(hash)
	if err != nil {
		return false, err
	}

	now := s.nowMs()
	art.CreatedMs = now

	return true, s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Assets().PutArtifact(ctx, art); err != nil {
			return err
		}

		if err := tx.Assets().SetFacts(ctx, hash, AssetFacts{Kind: string(timeline.AssetLUT)}, now); err != nil {
			return err
		}

		return tx.Assets().SetIngest(ctx, hash, IngestReady, 1, nil, now)
	})
}

// publishCube writes the checked table as the normalized artifact of the asset, the file the export
// reads (engine/export lutExt): a byte copy of the original, saved read-only under its final name by a
// rename, so a concurrent job that wrote it first leaves an identical file.
func (s *svc) publishCube(hash string) (ArtifactRow, error) {
	id := strings.TrimPrefix(hash, "sha256:")
	name := ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "cube")
	dir := filepath.Join(s.workspace, "artifacts", id)
	dst := filepath.Join(dir, name)
	row := ArtifactRow{
		AssetHash: hash, Kind: "normalized", Profile: ingest.ProfileNormalized, Ext: "cube",
		RelPath: "artifacts/" + id + "/" + name,
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return row, err
	}

	src, err := s.media.Content.Open(id)
	if err != nil {
		return row, err
	}

	defer func() { _ = src.Close() }()

	part := filepath.Join(dir, "."+name+".part")

	out, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return row, err
	}

	sum := sha256.New()

	size, err := io.Copy(io.MultiWriter(out, sum), src)
	if err == nil {
		err = out.Sync()
	}

	if cerr := out.Close(); err == nil {
		err = cerr
	}

	if err == nil {
		err = os.Chmod(part, 0o400)
	}

	if err == nil {
		if rerr := os.Rename(part, dst); rerr != nil {
			if _, serr := os.Stat(dst); serr != nil {
				err = rerr
			}
		}
	}

	if err != nil {
		_ = os.Chmod(part, 0o600)
		_ = os.Remove(part)

		return row, err
	}

	row.SizeBytes, row.SHA256 = size, hexenc.EncodeToString(sum.Sum(nil))

	return row, nil
}

// registerIngest records the artifacts and the facts and makes the asset
// ready, in one transaction (SDD-005 section 5.5, Facts).
func (s *svc) registerIngest(ctx context.Context, hash string, res *ingest.Result) error {
	now := s.nowMs()

	return s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var version *int

		for _, a := range res.Artifacts {
			meta, _ := json.Marshal(a.Meta)
			if a.Meta == nil {
				meta = nil
			}

			if a.Kind == "normalized" {
				v := a.Profile
				version = &v
			}

			if err := tx.Assets().PutArtifact(ctx, ArtifactRow{
				AssetHash: hash, Kind: a.Kind, Profile: a.Profile, Variant: a.Variant, Ext: a.Ext, RelPath: a.RelPath,
				SizeBytes: a.Size, SHA256: a.SHA256, Meta: meta, CreatedMs: now,
			}); err != nil {
				return err
			}
		}

		f := AssetFacts{
			Kind:              string(res.Kind),
			NormalizedVersion: version,
			HasAudio:          res.Facts.HasAudio,
			HasAlpha:          res.Facts.HasAlpha,
		}

		if res.Facts.Width > 0 {
			w, h := res.Facts.Width, res.Facts.Height
			f.Width, f.Height = &w, &h
		}

		if res.Kind != ingest.KindImage && res.Summary != nil {
			d := res.Summary.DurationMS
			f.DurationMs = &d
		}

		if res.Facts.FrameRate.Num > 0 && res.Facts.FrameRate.Den > 0 {
			n, d := int(res.Facts.FrameRate.Num), int(res.Facts.FrameRate.Den)
			f.FrameRateNum, f.FrameRateDen = &n, &d
		}

		if res.Facts.FrameCount > 0 {
			c := int64(res.Facts.FrameCount)
			f.FrameCount = &c
		}

		// An animated image: the delays of its frames, one per frame (SDD-011 section 4.7).
		if res.Kind == ingest.KindImage && len(res.Facts.FrameDelays) > 1 {
			f.FrameDelays = res.Facts.FrameDelays
		}

		if res.Facts.SampleCount > 0 {
			c := res.Facts.SampleCount
			f.SampleCount = &c
		}

		if err := tx.Assets().SetFacts(ctx, hash, f, now); err != nil {
			return err
		}

		return tx.Assets().SetIngest(ctx, hash, IngestReady, 1, nil, now)
	})
}

// completePending issues the asset.add of every project that waits for the
// file. A refusal (the document is full, the project is in the trash) leaves
// that row pending and is reported as a warning; the asset is ready anyway.
func (s *svc) completePending(ctx context.Context, hash, jobID string) []string {
	ar, err := s.store.Assets().Get(ctx, hash)
	if err != nil {
		return nil
	}

	pending, err := s.store.Assets().PendingForHash(ctx, hash)
	if err != nil {
		return nil
	}

	var warnings []string

	for i := range pending {
		pa := pending[i]
		head, perr := s.store.Projects().Get(ctx, pa.ProjectID)
		if perr != nil || head.State == ProjectTrashed {
			continue
		}

		key := "ingest." + jobID + "." + pa.ProjectID
		if !idempotencyKeyOK(key) {
			key = ""
		}

		if aerr := s.addAsset(ctx, &head, pa, ar, key); aerr != nil {
			warnings = append(warnings, pa.ProjectID+": "+aerr.Code+": "+aerr.Message)
		}
	}

	return warnings
}

// checkDisk refuses work while the volume of the workspace is under the free
// space floor (SDD-007 section 9.2 item 4).
// exportSizeLimitReason is details.reason of an export that passed STUARTY_MAX_EXPORT_BYTES, the
// same one engine/export gives (SDD-007 section 9.1).
const exportSizeLimitReason = "size_limit"

// exportSizeDetails adds the limit and the ceiling to an export that the size watchdog ended
// mid-run: the engine knows the run crossed the cap, not how far past it the file was.
func (s *svc) exportSizeDetails(err error) error {
	fe, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok || fe.Reason != exportSizeLimitReason || fe.Details["limit"] != nil {
		return err
	}

	return fromEngine(err).
		withDetail("limit", "export_bytes").withDetail("ceiling", s.limits.MaxExportBytes).
		withDetail("variable", "STUARTY_MAX_EXPORT_BYTES")
}

func (s *svc) checkDisk() *Error {
	if s.limits.MinFreeBytes <= 0 || s.workspace == "" {
		return nil
	}

	free, err := s.media.FreeBytes(s.workspace)
	if err == nil && free < uint64(s.limits.MinFreeBytes) {
		return newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space.").
			withHint("free space on the disk of the workspace; the person running the server can lower "+
				"STUARTY_MIN_FREE_BYTES").
			withLimit("free_bytes", free, s.limits.MinFreeBytes).withDetail("reason", "disk_low").
			withDetail("variable", "STUARTY_MIN_FREE_BYTES")
	}

	return nil
}

// runExport renders the project and finishes only after the output passed its
// verification (ADR-0018 item 6): the exporter verifies, and the service checks
// again that every check is ok and that the file on disk is the one reported.
func (s *svc) runExport(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	var in exportInput
	if err := json.Unmarshal(job.Input, &in); err != nil {
		return nil, internalError()
	}

	if in.Format == bundleExportFormat {
		return s.runBundleExport(ctx, job, pw, in)
	}

	if s.exporter == nil && s.media.FFmpegMissing != nil {
		return nil, s.media.FFmpegMissing
	}

	if in.Format == FormatPNG {
		return s.runStillExport(ctx, job, pw, in)
	}

	if s.exporter == nil || job.ProjectID == nil {
		return nil, internalError()
	}

	row, err := s.store.Projects().Get(ctx, *job.ProjectID)
	if err != nil {
		return nil, err
	}

	// The document of the revision recorded at the start, not the head.
	doc, derr := decodeRow(&ProjectRow{Document: in.Document})
	if derr != nil {
		return nil, derr
	}

	nested, nerr := decodeNested(in.Nested)
	if nerr != nil {
		return nil, nerr
	}

	if in.Preset != "" {
		if p, ok := exportcore.Lookup(in.Preset); ok {
			if perr := presetFit(doc, &p, in.End-in.Start, exportcore.Codec(in.Codec)); perr != nil {
				return nil, perr
			}
		}
	}

	if err := s.checkDisk(); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.workspace, "exports", row.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, internalError()
	}

	out, aerr := filepath.Abs(filepath.Join(dir, exportFileName(job.ID, in.Format)))
	if aerr != nil {
		return nil, internalError()
	}

	res, err := s.exporter.Export(ctx, ExportInput{
		JobID: job.ID, Doc: doc, Nested: nested, OutPath: out, Start: in.Start, End: in.End, Format: in.Format,
		MaxBytes: s.limits.MaxExportBytes, Preset: in.Preset, Codec: in.Codec,
		OnProgress: func(done, total int) {
			if total > 0 {
				pw.report(ctx, float64(done)/float64(total), "Rendering ("+strconv.Itoa(done*100/total)+"%)")
			}
		},
	})
	if err != nil {
		_ = os.Remove(out)

		return nil, s.exportSizeDetails(err)
	}

	if verr := verifyOutput(out, res); verr != nil {
		_ = os.Remove(out)

		return nil, verr
	}

	if s.limits.MaxExportBytes > 0 && res.Bytes > s.limits.MaxExportBytes {
		_ = os.Remove(out)

		have, ceiling := sizesApart(res.Bytes, s.limits.MaxExportBytes)

		return nil, newError(
			CodeExportFailed,
			"The export is %s; the export limit is %s (STUARTY_MAX_EXPORT_BYTES).",
			have,
			ceiling,
		).
			withHint("export a shorter range or a smaller frame size; the person running the server can raise "+
				"STUARTY_MAX_EXPORT_BYTES").
			withDetail("reason", exportSizeLimitReason).
			withLimit("export_bytes", res.Bytes, s.limits.MaxExportBytes).
			withDetail("variable", "STUARTY_MAX_EXPORT_BYTES")
	}

	fps := doc.Settings.FPS
	frames := int64(res.Frames)

	if frames == 0 {
		frames = in.End - in.Start
	}

	width, height := doc.Settings.Width, doc.Settings.Height
	if res.Width > 0 && res.Height > 0 {
		width, height = res.Width, res.Height
	}

	export := ExportResult{
		Revision:     in.Revision,
		Format:       formatName(in.Format),
		Path:         out,
		DownloadURL:  "/api/v1/jobs/" + job.ID + "/file",
		Bytes:        res.Bytes,
		SHA256:       res.SHA256,
		Duration:     timeValue(fps, frames),
		Width:        width,
		Height:       height,
		FPS:          fpsString(fps.Num, fps.Den),
		VideoCodec:   res.VideoCodec,
		AudioCodec:   res.AudioCodec,
		LoudnessLUFS: res.LoudnessLUFS,
		Checks:       res.Checks,

		Preset: in.Preset, CodecFallback: res.CodecFallback, Notes: res.Notes,
	}

	if export.Checks == nil {
		export.Checks = []VerifyCheck{}
	}

	raw, merr := json.Marshal(JobResult{Export: &export})
	if merr != nil {
		_ = os.Remove(out)

		return nil, internalError()
	}

	return raw, nil
}

// verifyOutput is the service side of the verification gate: every check ran
// ok and the file is on disk with the size the exporter reported.
func verifyOutput(path string, res *ExportOutput) *Error {
	var failed []VerifyCheck

	for _, c := range res.Checks {
		if !c.OK {
			failed = append(failed, c)
		}
	}

	if len(failed) == 0 {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Size() > 0 && st.Size() == res.Bytes {
			return nil
		}

		failed = append(
			failed,
			VerifyCheck{Name: "file", OK: false, Detail: "the output is missing or is not the size reported"},
		)
	}

	return newError(CodeVerifyFailed, "The exported file failed %d check(s) and was deleted.", len(failed)).
		withHint("export again; if it keeps failing, report it with the output of doctor").
		withDetail("checks", failed)
}

// Recover is the start-up recovery (SDD-005 section 14.5): every job found
// running belongs to an owner that is gone. It is requeued with a new queue
// message, or failed after three attempts; queued jobs older than a minute get
// one more message, which the claim makes harmless when the old one survived.
func (s *svc) Recover(ctx context.Context) error {
	var changed []JobRow

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		var err error
		if changed, err = s.recoverRunning(ctx, tx, now); err != nil {
			return err
		}

		stale, err := tx.Jobs().QueuedBefore(ctx, now-staleQueuedAge.Milliseconds())
		if err != nil {
			return err
		}

		for i := range stale {
			if err := s.enqueueKind(ctx, tx, &stale[i]); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for i := range changed {
		s.emitJob(ctx, &changed[i])
	}

	return nil
}

// recoverRunning requeues or fails every running job; it returns the rows it
// changed, for their events.
func (s *svc) recoverRunning(ctx context.Context, tx Tx, now int64) ([]JobRow, error) {
	running, err := tx.Jobs().Running(ctx)
	if err != nil {
		return nil, err
	}

	var changed []JobRow

	for i := range running {
		j := running[i]

		if j.Attempts >= maxAttempts {
			if err := s.failInterrupted(ctx, tx, &j, now); err != nil {
				return nil, err
			}

			changed = append(changed, j)

			continue
		}

		ok, err := tx.Jobs().Requeue(ctx, j.ID, msgRestarted, now)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}

		j.State, j.Message = JobQueued, msgRestarted

		if ingestsAsset(&j) {
			if err := tx.Assets().SetIngest(ctx, *j.AssetHash, IngestPending, 0, nil, now); err != nil {
				return nil, err
			}
		}

		if err := s.enqueueKind(ctx, tx, &j); err != nil {
			return nil, err
		}

		changed = append(changed, j)
	}

	return changed, nil
}

func (s *svc) enqueueKind(ctx context.Context, tx Tx, j *JobRow) error {
	if s.enqueue == nil {
		return nil
	}

	return s.enqueue.Enqueue(ctx, tx, queueKindFor(j.Kind), j.ID)
}

// queueKindFor is the queue message kind of a job kind. A transcription runs
// on the ingest queue (its timeout fits a long transcription).
func queueKindFor(jobKind string) string {
	switch jobKind {
	case JobIngest, JobTranscribe:
		return QueueKindIngest
	case JobExport:
		return QueueKindExport
	case JobExportBatch:
		return QueueKindExportBatch
	default:
		return QueueKindFrame
	}
}

// failCode is the error code of a job of the kind that was stopped.
func failCode(kind string) string {
	switch kind {
	case JobExport, JobExportBatch:
		return CodeExportFailed
	case JobIngest:
		return CodeIngestFailed
	default:
		return command.CodeInternal
	}
}

// timeoutError is the error of a job that outran its queue deadline.
func timeoutError(kind string) *Error {
	return newError(failCode(kind), "the job ran past its time limit and was stopped.").
		withDetail("reason", "timeout").
		withHint("run it again with a shorter range or a smaller project")
}

// failInterrupted ends a job interrupted three times (SDD-005 section 14.5).
func (s *svc) failInterrupted(ctx context.Context, tx Tx, j *JobRow, now int64) error {
	e := newError(failCode(j.Kind), "the job was interrupted %d times and given up.", j.Attempts).
		withDetail("reason", "interrupted").withHint("run it again")
	ej := errorJSON(e)

	ok, err := tx.Jobs().Finish(ctx, j.ID, JobFailed, nil, ej, now)
	if err != nil || !ok {
		return err
	}

	j.State, j.Error = JobFailed, ej
	s.logJob(ctx, slog.LevelError, "job failed", j, slog.String("code", e.Code), slog.String("reason", "interrupted"))

	ended := *j
	tx.AfterCommit(func() { s.noteUpgradeEnded(&ended, JobFailed) })

	if ingestsAsset(j) {
		return tx.Assets().SetIngest(ctx, *j.AssetHash, IngestFailed, 0, ej, now)
	}

	return nil
}

// logJob writes one line about a job with its identifier, kind, actor and
// project. It names codes and reasons only: a message or a file name may hold
// text from the media, which stays out of the log (ADR-0020).
func (s *svc) logJob(ctx context.Context, level slog.Level, msg string, job *JobRow, extra ...slog.Attr) {
	project := ""
	if job.ProjectID != nil {
		project = *job.ProjectID
	}

	attrs := append(logging.Who("", job.ID, job.Actor, project), slog.String("kind", job.Kind))
	s.log.LogAttrs(ctx, level, msg, append(attrs, extra...)...)
}

func levelOf(state string) slog.Level {
	if state == JobFailed {
		return slog.LevelError
	}

	return slog.LevelInfo
}
