package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

const (
	maxImportFiles   = 50
	maxWaitMS        = 25000
	maxDisplayName   = 200
	waitPollInterval = 25 * time.Millisecond
	maxMediaLimit    = 200
	defMediaLimit    = 50
	maxArtifactJSON  = 4 << 20
)

// asErr is asError for a caller that needs the *Error.
func asErr(err error) *Error {
	if e, ok := errors.AsType[*Error](asError(err)); ok {
		return e
	}

	return internalError()
}

// loadProject reads a project the caller may see; a missing one and one the
// caller may not see look the same.
func (s *svc) loadProject(ctx context.Context, c Caller, id string) (ProjectRow, error) {
	row, err := s.store.Projects().Get(ctx, id)
	if err != nil || !visible(c, row.OwnerUserID) {
		return ProjectRow{}, projectNotFound(id, err)
	}

	return row, nil
}

func projectFPS(row *ProjectRow) timeval.FPS {
	return timeval.FPS{Num: int64(row.Summary.FPSNum), Den: int64(row.Summary.FPSDen)}
}

// displayName is the name of an asset: the base name of the path, normalised,
// with control, bidirectional, invisible and separator characters replaced by "_" and cut
// to 200 characters (SDD-007 section 7.1 step 14). The directory is never kept.
func displayName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}

	var b strings.Builder

	n := 0

	for _, r := range norm.NFC.String(strings.ToValidUTF8(p, "_")) {
		if n == maxDisplayName {
			break
		}

		switch {
		case unicode.IsControl(r), untrusted.Removed(r), r == '/', r == '\\':
			r = '_'
		}

		b.WriteRune(r)

		n++
	}

	return b.String()
}

func (s *svc) MediaImport(ctx context.Context, req MediaImportRequest) (*MediaImportResult, error) {
	c, err := s.authorizeProject(ctx, "MediaImport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if n := len(req.Files); n < 1 || n > maxImportFiles {
		return nil, newError(command.CodeValidation, "the call has %d files; it must have 1 to %d.", n, maxImportFiles).
			withPath("files")
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	// The ids are checked before the first file is read, so that one bad id fails the call and not
	// just its entry after the other files were imported (VV-105).
	for i, f := range req.Files {
		if f.AssetID != "" && !timeline.ValidID(timeline.PrefixAsset, f.AssetID) {
			return nil, newError(command.CodeValidation, "the asset id of file %d is not valid.", i).
				withPath(fmt.Sprintf("files[%d].asset_id", i)).
				withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u")
		}
	}

	rec, rerr := checkImportBlocks(&req)
	if rerr != nil {
		return nil, rerr
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if row.State == ProjectTrashed {
		return nil, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	res := &MediaImportResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       row.ID,
		Assets:          make([]ImportedAsset, len(req.Files)),
	}

	for i, f := range req.Files {
		res.Assets[i] = s.importFile(ctx, c, &row, f, req.DryRun, &rec)
		if res.Assets[i].Error != nil {
			res.Failed++
		}
	}

	if len(req.Files) == 1 && res.Failed == 1 {
		return nil, res.Assets[0].Error
	}

	if req.WaitMS > 0 {
		s.waitAssets(ctx, &row, res.Assets, time.Duration(req.WaitMS)*time.Millisecond)
	}

	// Failed counts the final state of every file, including an ingest that failed
	// while the call waited, so the CLI exit code and the batch total agree with the entries.
	res.Failed = 0

	for i := range res.Assets {
		s.refreshImported(ctx, c, &row, &res.Assets[i])

		if res.Assets[i].Error != nil {
			// A file that did not enter this project was not deduplicated, even when its bytes were already
			// in the store from an earlier refused import; a retry of an asset the project has stays one (VV-193).
			if res.Assets[i].newInProject {
				res.Assets[i].Deduplicated = false
			}

			res.Failed++
		}
	}

	res.Imported = len(res.Assets) - res.Failed

	if head, herr := s.store.Projects().Get(ctx, row.ID); herr == nil {
		res.Revision = head.Revision
	}

	lines := make([]textview.ImportLine, len(res.Assets))
	for i := range res.Assets {
		a := &res.Assets[i]
		lines[i] = textview.ImportLine{
			ID: a.AssetID, Kind: a.Kind, State: a.State, Name: a.Name, Deduplicated: a.Deduplicated, JobID: a.JobID,
		}

		if a.Error != nil {
			lines[i].ErrorCode, lines[i].ErrorMessage, lines[i].ErrorHint = a.Error.Code, a.Error.Message, a.Error.Hint
		}
	}

	res.Text = textview.Import(lines)

	s.offerReplacements(ctx, c, row.ID, rec.origin.RenderKey, req.DryRun, res)

	return res, nil
}

// importFile is the flow of SDD-011 section 2.3 for one file. It never fails
// the call: a refusal is the Error of the entry.
func (s *svc) importFile(
	ctx context.Context,
	c Caller,
	project *ProjectRow,
	f ImportFile,
	dryRun bool,
	rec *libraryRecord,
) ImportedAsset {
	if f.Hash != "" {
		return s.reuseFile(ctx, c, project, f, dryRun, rec)
	}

	out := ImportedAsset{Name: displayName(f.Path), State: string(IngestPending)}

	refuse := func(err error) ImportedAsset {
		out.State = string(IngestFailed)
		out.Error = fromEngine(err)

		return out
	}

	if s.media.Guard == nil || s.media.Content == nil {
		return refuse(internalError())
	}

	if f.AssetID != "" && !timeline.ValidID(timeline.PrefixAsset, f.AssetID) {
		return refuse(newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id").
			withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u"))
	}

	file, err := s.media.Guard.OpenImport(f.Path)
	if err != nil {
		return refuse(s.withRootsHint(c, err))
	}

	defer func() { _ = file.Close() }()

	st, err := file.Stat()
	if err != nil {
		return refuse(err)
	}

	if lerr := s.checkImportSize(st.Size()); lerr != nil {
		return refuse(forCaller(c, lerr))
	}

	if dryRun {
		return out
	}

	return s.putAndAttach(ctx, c.Actor, c.OwnerUserID, project, file, f.AssetID, out, rec)
}

// putAndAttach is the part of an import from the open file to the linked asset: the copy into the store, the
// row of the asset, its link to the project and the library side (SDD-011 sections 2.3 and 14). A file read from
// a path and a file a provider generated (SDD-016 section 6.1) share it. It never fails the call: a refusal is
// the Error of the entry.
func (s *svc) putAndAttach(
	ctx context.Context,
	actor, owner string,
	project *ProjectRow,
	file io.Reader,
	wantedID string,
	out ImportedAsset,
	rec *libraryRecord,
) ImportedAsset {
	refuse := func(err error) ImportedAsset {
		out.State = string(IngestFailed)
		out.Error = fromEngine(err)

		return out
	}

	// The read side of the store lock covers the copy into store/ and the row
	// of the asset, so an emptying cannot delete a file this import found.
	s.storeMu.RLock()

	put, err := s.media.Content.Put(ctx, file, s.limits.MaxImportBytes)
	if err != nil {
		s.storeMu.RUnlock()

		return refuse(err)
	}

	hash := "sha256:" + put.Hash
	out.Deduplicated = put.Deduplicated

	var att attached

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		ar, _, uerr := tx.Assets().Upsert(ctx, AssetRow{
			Hash: hash, SizeBytes: put.Size, OriginalName: out.Name, IngestState: IngestPending,
			CreatedMs: now, UpdatedMs: now,
		})
		if uerr != nil {
			return uerr
		}

		var aerr error
		if att, aerr = s.attach(ctx, tx, project, ar, out.Name, wantedID, actor, now); aerr != nil {
			return aerr
		}

		return assetlib.RecordImport(ctx, tx.AssetLibrary(), owner, hash, rec.origin, rec.info, actor, now)
	})
	s.storeMu.RUnlock()

	if werr != nil {
		// A refusal of the library rules (too many tags) is the caller's, not an internal fault.
		return refuse(libraryRefusal(werr))
	}

	out.AssetID, out.JobID, out.Kind, out.Hash = att.pa.AssetID, att.jobID, att.asset.Kind, hash
	out.newInProject = att.fresh

	if att.addNow != nil {
		if aerr := s.addAsset(ctx, project, *att.addNow, att.asset, ""); aerr != nil {
			out.Error = aerr
		}
	}

	return out
}

// attached is what linking an asset row to a project made: the use, the job when the asset still needs an
// ingest, and the use to add to the document at once when the asset is ready.
type attached struct {
	asset  AssetRow
	pa     ProjectAssetRow
	addNow *ProjectAssetRow
	jobID  string
	fresh  bool // the project did not have the file before
}

// attach links an asset row that is in the store to a project, inside the import's transaction, and queues the
// ingest the asset still needs: the part of an import that a file read from a path and a file named by its hash
// share (SDD-011 sections 2.3 and 14.3).
func (s *svc) attach(
	ctx context.Context,
	tx Tx,
	project *ProjectRow,
	ar AssetRow,
	name, wanted, actor string,
	now int64,
) (attached, error) {
	hash := ar.Hash

	if ar.IngestState == IngestFailed {
		if err := tx.Assets().SetIngest(ctx, hash, IngestPending, 0, nil, now); err != nil {
			return attached{}, err
		}

		ar.IngestState, ar.Progress, ar.IngestError = IngestPending, 0, nil
	}

	_, ferr := tx.Assets().FindProjectAssetByHash(ctx, project.ID, hash)
	att := attached{asset: ar, fresh: ferr != nil}

	row, perr := s.linkProjectAsset(ctx, tx, project.ID, hash, name, wanted, actor, now)
	if perr != nil {
		return attached{}, perr
	}

	att.pa = row

	// Registered before the enqueue: an inline queue runs the job in its own
	// after-commit hook, and this event must come first.
	tx.AfterCommit(func() { s.emitAssetFor(project.ID, row.AssetID, ar.IngestState, ar.Progress, nil) })

	switch ar.IngestState {
	case IngestReady:
		if row.State == ProjectAssetPending {
			att.addNow = &row
		}
	default:
		id, jerr := s.ensureIngestJob(ctx, tx, hash, actor, now)
		if jerr != nil {
			return attached{}, jerr
		}

		att.jobID = id
	}

	return att, nil
}

// sizesApart formats a size and the ceiling it exceeds so that they never read
// the same: when the rounded figures match, both are given in exact bytes.
func sizesApart(have, ceiling int64) (string, string) {
	a, b := textview.Size(have), textview.Size(ceiling)
	if a != b {
		return a, b
	}

	return fmt.Sprintf("%d bytes", have), fmt.Sprintf("%d bytes", ceiling)
}

// checkImportSize is steps 3 of SDD-011 section 2.3 before a byte is copied.
func (s *svc) checkImportSize(size int64) *Error {
	if size == 0 {
		return fromEngine(&Error{
			Code: "E_UNSUPPORTED_MEDIA", Message: "The file is empty.", Details: map[string]any{"reason": "empty_file"},
		})
	}

	if size > s.limits.MaxImportBytes {
		have, ceiling := sizesApart(size, s.limits.MaxImportBytes)

		return newError("E_LIMIT_EXCEEDED", "The file is %s; the import limit is %s (STUARTY_MAX_IMPORT_BYTES).",
			have, ceiling).
			withHint("import a smaller file; the person running the server can raise STUARTY_MAX_IMPORT_BYTES").
			withDetail("limit", "import_bytes").withDetail("value", size).
			withDetail("ceiling", s.limits.MaxImportBytes).withDetail("variable", "STUARTY_MAX_IMPORT_BYTES")
	}

	return s.checkFreeSpace(size)
}

// checkFreeSpace refuses when the disk of the workspace would keep less than the reserve after size more bytes.
func (s *svc) checkFreeSpace(size int64) *Error {
	if s.limits.MinFreeBytes > 0 && s.workspace != "" {
		need := uint64(size) + uint64(s.limits.MinFreeBytes) //nolint:gosec // both are positive

		if free, err := s.media.FreeBytes(s.workspace); err == nil && free < need {
			return newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space for this file.").
				withHint("free space on the disk of the workspace; the person running the server can lower "+
					"STUARTY_MIN_FREE_BYTES").
				withLimit("free_bytes", free, need).withDetail("reason", "disk_low").
				withDetail("variable", "STUARTY_MIN_FREE_BYTES")
		}
	}

	return nil
}

// linkProjectAsset finds or creates the project_assets row of a hash: a new
// row gets an identifier, a trashed one comes back as pending, any other is
// returned unchanged (SDD-005 section 5.5).
func (s *svc) linkProjectAsset(
	ctx context.Context,
	tx Tx,
	projectID, hash, name, wanted, actor string,
	now int64,
) (ProjectAssetRow, error) {
	row, err := tx.Assets().FindProjectAssetByHash(ctx, projectID, hash)

	switch {
	case err == nil && row.State == ProjectAssetTrashed:
		if err := tx.Assets().
			SetProjectAssetState(ctx, projectID, row.AssetID, ProjectAssetPending, actor, now, nil); err != nil {
			return ProjectAssetRow{}, err
		}

		row.State = ProjectAssetPending

		return row, nil
	case err == nil:
		return row, nil
	case !isNotFound(err):
		return ProjectAssetRow{}, err
	}

	id := wanted
	if id == "" {
		id = s.newIDs(timeline.PrefixAsset)
	}

	row = ProjectAssetRow{
		ProjectID: projectID, AssetID: id, AssetHash: hash, State: ProjectAssetPending, Name: name,
		ImportedBy: actor, ImportedMs: now,
	}

	if err := tx.Assets().PutProjectAsset(ctx, row); err != nil {
		if isExists(err) {
			return ProjectAssetRow{}, newError(
				command.CodeIDExists,
				"an asset with the identifier %s already exists.",
				id,
			).
				withPath("asset_id")
		}

		return ProjectAssetRow{}, err
	}

	return row, nil
}

func isExists(err error) bool { return errors.Is(err, ErrExists) }

// ensureIngestJob returns the active ingest job of a hash, or creates and
// enqueues one. The partial unique index of SDD-005 section 5.6 makes one
// active job per file a database fact.
func (s *svc) ensureIngestJob(ctx context.Context, tx Tx, hash, actor string, now int64) (string, error) {
	return s.ensureIngestJobFor(ctx, tx, hash, actor, now, map[string]any{"asset_hash": hash, "reason": "import"})
}

// ensureIngestJobFor is ensureIngestJob for a job whose input is given: an import, or the profile upgrade of
// SDD-011 section 6.3.
func (s *svc) ensureIngestJobFor(
	ctx context.Context,
	tx Tx,
	hash, actor string,
	now int64,
	in map[string]any,
) (string, error) {
	active, err := tx.Jobs().ActiveIngestForHash(ctx, hash)
	if err == nil {
		return active.ID, nil
	}

	if !isNotFound(err) {
		return "", err
	}

	input := marshalOrNull(in)
	h := hash
	job := JobRow{
		ID: s.newIDs("job"), Kind: JobIngest, AssetHash: &h, Actor: actor, State: JobQueued, Input: input,
		CreatedMs: now, UpdatedMs: now,
	}

	lim := &JobLimit{Kind: JobIngest, Max: s.limits.MaxQueuedIngests + s.limits.MaxIngests}
	if in["reason"] == reasonProfileUpgrade {
		lim.Reason, lim.MaxPerReason = reasonProfileUpgrade, maxActiveProfileUpgrades
	}

	if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
		return "", err
	}

	// Registered before the enqueue: an inline queue runs the job in its own
	// after-commit hook, and the queued event must come first.
	queued := job
	tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

	if s.enqueue != nil {
		if err := s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID); err != nil {
			return "", err
		}
	}

	return job.ID, nil
}

// addAsset issues the internal asset.add of SDD-011 section 2.5 for one pending
// row, as its importer, and leaves the row active on success. key is the
// idempotency key; empty when no job stands behind the call.
func (s *svc) addAsset(ctx context.Context, project *ProjectRow, pa ProjectAssetRow, ar AssetRow, key string) *Error {
	asset := timeline.Asset{
		ID:       pa.AssetID,
		Hash:     ar.Hash,
		Kind:     timeline.AssetKind(ar.Kind),
		Name:     pa.Name,
		HasAudio: ar.Facts.HasAudio,
		HasAlpha: ar.Facts.HasAlpha,
	}

	f := ar.Facts
	if f.Width != nil {
		asset.Width = *f.Width
	}

	if f.Height != nil {
		asset.Height = *f.Height
	}

	if asset.Kind == timeline.AssetVideo && f.FrameRateNum != nil && f.FrameRateDen != nil {
		asset.FrameRate = timeline.Rational{Num: *f.FrameRateNum, Den: *f.FrameRateDen}
	}

	if asset.Kind == timeline.AssetVideo && f.FrameCount != nil {
		asset.FrameCount = int(*f.FrameCount)
	}

	// An animated image has one delay per frame, and its frame count is the number of delays.
	if asset.Kind == timeline.AssetImage && len(f.FrameDelays) > 1 {
		asset.FrameCount, asset.FrameDelays = len(f.FrameDelays), f.FrameDelays
	}

	if f.SampleCount != nil {
		asset.SampleCount = int(*f.SampleCount)
	}

	args, err := json.Marshal(map[string]any{"asset": asset})
	if err != nil {
		return internalError()
	}

	head, err := s.store.Projects().Get(ctx, project.ID)
	if err != nil {
		return asErr(err)
	}

	caller := Caller{Actor: pa.ImportedBy, Scopes: []string{ScopeEdit, ScopeMedia}}
	if head.OwnerUserID != nil {
		caller.OwnerUserID = *head.OwnerUserID
	}

	_, err = s.edit(WithCaller(ctx, caller), EditRequest{
		ProjectID: project.ID, TimelineID: project.ID, BaseRevision: head.Revision, IdempotencyKey: key,
		Operations: []command.Operation{{Op: "asset.add", Args: args}},
	}, true)
	if err != nil {
		return asErr(err)
	}

	return nil
}

// waitAssets holds the call until every accepted file is ready or failed, or
// the time passes. Waiting never fails the call.
func (s *svc) waitAssets(ctx context.Context, project *ProjectRow, assets []ImportedAsset, d time.Duration) {
	deadline := time.Now().Add(d)

	for {
		done := true

		for i := range assets {
			if assets[i].AssetID == "" || assets[i].Error != nil {
				continue
			}

			pa, err := s.store.Assets().GetProjectAsset(ctx, project.ID, assets[i].AssetID)
			if err != nil {
				continue
			}

			ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
			if err != nil {
				continue
			}

			if ar.IngestState == IngestPending || ar.IngestState == IngestIngesting {
				done = false

				continue
			}

			// A ready asset is in the document only once its asset.add has
			// committed: until then the revision that the call returns would
			// be stale for the caller's next edit. The ingest job is the one
			// that commits it, so the wait ends with the job at the latest
			// (a refused add leaves the row pending and is only a warning).
			if ar.IngestState == IngestReady && pa.State == ProjectAssetPending {
				if _, importing := s.activeImportJob(ctx, pa.AssetHash); importing {
					done = false
				}
			}
		}

		if done || time.Now().After(deadline) || ctx.Err() != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitPollInterval):
		}
	}
}

// refreshImported fills an entry from the database: state, progress, kind,
// the probe once ready and the error of a failed ingest.
func (s *svc) refreshImported(ctx context.Context, c Caller, project *ProjectRow, a *ImportedAsset) {
	if a.AssetID == "" {
		return
	}

	pa, err := s.store.Assets().GetProjectAsset(ctx, project.ID, a.AssetID)
	if err != nil {
		return
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return
	}

	a.State, a.Progress, a.Kind = string(ar.IngestState), ar.Progress, ar.Kind

	// The file is ready for every project the moment the shared ingest ends, but
	// it is in this project's document only when its asset.add has committed,
	// which the ingest job does last. Until then the asset is not usable: an
	// immediate clip.add would answer E_NOT_FOUND.
	if ar.IngestState == IngestReady && pa.State == ProjectAssetPending {
		if active, importing := s.activeImportJob(ctx, pa.AssetHash); importing {
			a.State, a.Progress = string(IngestIngesting), math.Min(ar.Progress, 0.99)
			if a.JobID == "" {
				a.JobID = active.ID
			}

			return
		}
	}

	if ar.IngestState == IngestFailed && a.Error == nil {
		a.Error = forCaller(c, parseError(ar.IngestError))
	}

	if ar.IngestState == IngestReady {
		a.Progress = 1
		a.Probe = s.probeOf(ctx, project, &ar)
	}
}

// assetDurationFrames is the duration of an asset in project frames, the one value every
// surface shows: the same count the document holds and the one a clip can use (SDD-001
// section 3.2, whole frames of the project rate), so the probe, media.list and the events agree.
// Without the frame or sample count it falls back to the probed milliseconds.
func assetDurationFrames(f *AssetFacts, fps timeval.FPS, probedMS int64) int64 {
	switch {
	case f.FrameCount != nil && f.FrameRateNum != nil && f.FrameRateDen != nil && *f.FrameRateNum > 0 && *f.FrameCount > 0:
		return *f.FrameCount * fps.Num * int64(*f.FrameRateDen) / (fps.Den * int64(*f.FrameRateNum))
	case f.SampleCount != nil && *f.SampleCount > 0:
		return *f.SampleCount * fps.Num / (48000 * fps.Den)
	default:
		return fps.FrameAtMillis(probedMS)
	}
}

// probeOf is the ProbeSummary of a ready asset, from its probe artifact.
func (s *svc) probeOf(ctx context.Context, project *ProjectRow, ar *AssetRow) *ProbeSummary {
	sum := s.readSummary(ctx, ar.Hash)
	if sum == nil {
		return nil
	}

	fps := projectFPS(project)
	p := &ProbeSummary{Kind: string(sum.Kind), Container: sum.Container, SizeBytes: sum.SizeBytes}

	if sum.Kind != ingest.KindImage && fps.Valid() {
		frames := assetDurationFrames(&ar.Facts, fps, sum.DurationMS)
		tv := timeValue(timeline.Rational{Num: int(fps.Num), Den: int(fps.Den)}, frames)
		p.Duration = &tv
	}

	if sum.Kind == ingest.KindImage && len(ar.Facts.FrameDelays) > 1 {
		p.Animated, p.Frames = true, len(ar.Facts.FrameDelays)

		for _, d := range ar.Facts.FrameDelays {
			p.LoopDurationMS += int64(d)
		}
	}

	if v := sum.Video; v != nil {
		p.Width, p.Height = v.DisplayWidth, v.DisplayHeight
		p.VideoCodec = v.Codec
		p.FrameRate = v.NominalFPS.String()
	}

	if a := sum.Audio; a != nil {
		p.AudioCodec, p.SampleRate, p.Channels, p.HasAudio = a.Codec, a.SampleRate, a.Channels, true
	}

	return p
}

// readSummary reads the summary of probe-1.json of an asset, which engine/ingest
// writes as {"summary": ..., "ffprobe": ...} (SDD-011 section 3.2); nil when it
// cannot be read.
func (s *svc) readSummary(ctx context.Context, hash string) *ingest.Summary {
	arts, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil
	}

	raw := s.readArtifact(arts, "probe", "")
	if raw == nil {
		return nil
	}

	var doc struct {
		Summary *ingest.Summary `json:"summary"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}

	return doc.Summary
}

// readArtifact reads a small JSON artifact of a kind; nil when it is missing,
// outside the workspace or too large.
func (s *svc) readArtifact(arts []ArtifactRow, kind, variant string) []byte {
	i := slices.IndexFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == kind && a.Variant == variant && a.Ext == "json"
	})
	if i < 0 || s.workspace == "" {
		return nil
	}

	p, ok := s.artifactPath(arts[i].RelPath)
	if !ok {
		return nil
	}

	f, err := os.Open(p)
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)

	for len(buf) <= maxArtifactJSON {
		n, rerr := f.Read(chunk)
		buf = append(buf, chunk[:n]...)

		if rerr != nil {
			return buf
		}
	}

	return nil
}

// artifactPath resolves a slash path relative to the workspace and refuses
// one that would leave it.
func (s *svc) artifactPath(rel string) (string, bool) {
	clean := path.Clean(rel)
	if clean != rel || path.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.ContainsAny(rel, `\:`) {
		return "", false
	}

	return filepath.Join(s.workspace, filepath.FromSlash(rel)), true
}

// asset events

// referrers are the project_assets rows of every project that holds the hash.
func (s *svc) referrers(ctx context.Context, hash string) []ProjectAssetRow {
	ids, err := s.store.Assets().ProjectsReferencing(ctx, hash)
	if err != nil {
		return nil
	}

	out := make([]ProjectAssetRow, 0, len(ids))

	for _, id := range ids {
		if row, ferr := s.store.Assets().FindProjectAssetByHash(ctx, id, hash); ferr == nil {
			out = append(out, row)
		}
	}

	return out
}

func (s *svc) emitAssetFor(projectID, assetID string, st IngestState, progress float64, e *Error) {
	s.emit(Event{
		Type: "asset.updated", ProjectID: projectID, AssetID: assetID, State: string(st), Progress: progress,
		Error: e, At: s.now(),
	})
}

// emitAsset publishes asset.updated on the stream of every project that holds
// the hash, each with its own asset id.
func (s *svc) emitAsset(ctx context.Context, hash string, st IngestState, progress float64, e *Error) {
	refs := s.referrers(ctx, hash)
	for i := range refs {
		s.emitAssetFor(refs[i].ProjectID, refs[i].AssetID, st, progress, e)
	}
}

// MediaList and MediaInspect

func offsetCursor(kind, project string, off int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(kind + ":" + project + ":" + strconv.Itoa(off)))
}

func decodeOffset(cur, kind, project string) (int, *Error) {
	raw, err := base64.RawURLEncoding.DecodeString(cur)
	parts := strings.SplitN(string(raw), ":", 3)

	if err != nil || len(parts) != 3 || parts[0] != kind || parts[1] != project {
		return 0, newError(command.CodeValidation, "the cursor is not valid for this list.").
			withPath("cursor").withHint("start again without a cursor")
	}

	off, err := strconv.Atoi(parts[2])
	if err != nil || off < 0 {
		return 0, newError(command.CodeValidation, "the cursor is not valid for this list.").withPath("cursor")
	}

	return off, nil
}

func (s *svc) MediaList(ctx context.Context, req MediaListRequest) (*MediaListResult, error) {
	c, err := s.authorizeProject(ctx, "MediaList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	off := 0

	if req.Cursor != "" {
		var derr *Error
		if off, derr = decodeOffset(req.Cursor, "media", row.ID); derr != nil {
			return nil, derr
		}
	}

	rows, err := s.store.Assets().ListProjectAssets(ctx, row.ID)
	if err != nil {
		return nil, asError(err)
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	limit := clampLimit(req.Limit, defMediaLimit, maxMediaLimit)
	fps := projectFPS(&row)
	total := len(rows)

	entries := make([]MediaEntry, 0, limit)
	lines := make([]textview.MediaLine, 0, limit)

	for i := off; i < total && len(entries) < limit; i++ {
		ar, gerr := s.store.Assets().Get(ctx, rows[i].AssetHash)
		if gerr != nil {
			return nil, asError(gerr)
		}

		e := s.entryOf(ctx, c, &rows[i], &ar, fps)
		entries = append(entries, e)
		lines = append(lines, mediaLineOf(&e))
	}

	if n := textview.FitMediaList(fps, lines); n < len(lines) {
		entries, lines = entries[:n], lines[:n]
	}

	res := &MediaListResult{
		UntrustedNotice: UntrustedNotice, ProjectID: row.ID, Assets: entries,
		Text: textview.MediaList(row.ID, fps, lines, total),
	}

	if next := off + len(entries); next < total {
		cur := offsetCursor("media", row.ID, next)
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

func mediaLineOf(e *MediaEntry) textview.MediaLine {
	l := textview.MediaLine{
		ID: e.AssetID, Kind: e.Kind, State: e.State, Name: e.Name, DurationFrame: e.DurationFrames,
		Width: e.Width, Height: e.Height, HasAudio: e.HasAudio, Percent: int(e.Progress * 100),
	}

	if e.Error != nil {
		l.ErrorCode, l.ErrorMessage = e.Error.Code, e.Error.Message
	}

	return l
}

func (s *svc) entryOf(ctx context.Context, c Caller, pa *ProjectAssetRow, ar *AssetRow, fps timeval.FPS) MediaEntry {
	e := MediaEntry{
		AssetID: pa.AssetID, Name: pa.Name, Kind: ar.Kind, State: string(ar.IngestState), Progress: ar.Progress,
		HasAudio: ar.Facts.HasAudio, Trashed: pa.State == ProjectAssetTrashed,
	}

	if ar.IngestState == IngestFailed {
		e.Error = forCaller(c, parseError(ar.IngestError))
	}

	if ar.IngestState != IngestReady {
		return e
	}

	e.Progress = 1

	if ar.Facts.Width != nil && ar.Facts.Height != nil {
		e.Width, e.Height = *ar.Facts.Width, *ar.Facts.Height
	}

	if ar.Facts.DurationMs != nil && ar.Kind != string(timeline.AssetImage) && fps.Valid() {
		frames := assetDurationFrames(&ar.Facts, fps, *ar.Facts.DurationMs)
		secs := round3(fps.Seconds(frames))
		e.DurationFrames, e.DurationSeconds = &frames, &secs
	}

	if arts, err := s.store.Assets().Artifacts(ctx, ar.Hash); err == nil {
		e.Strip = stripOf(arts)
	}

	return e
}

// isThumbExt is the file type of a poster or a sheet: JPEG, or PNG for a source with alpha (SDD-011 4.5).
func isThumbExt(ext string) bool { return ext == "jpg" || ext == "png" }

func stripOf(arts []ArtifactRow) *Strip {
	i := slices.IndexFunc(
		arts,
		func(a ArtifactRow) bool { return a.Kind == "thumbnails" && a.Variant == "" && isThumbExt(a.Ext) },
	)
	if i < 0 || len(arts[i].Meta) == 0 {
		return nil
	}

	var l ingest.ThumbLayout
	if json.Unmarshal(arts[i].Meta, &l) != nil || l.Count < 1 {
		return nil
	}

	return &Strip{TileW: l.TileW, TileH: l.TileH, Columns: l.Columns, Count: l.Count, IntervalMS: l.IntervalMS}
}

var inspectSections = []string{"probe", "transcript", "silences", "loudness", "beats"}

func (s *svc) MediaInspect(ctx context.Context, req MediaInspectRequest) (*MediaInspectResult, error) {
	c, err := s.authorizeProject(ctx, "MediaInspect", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	sections := req.Sections
	if len(sections) == 0 {
		sections = []string{"probe"}
	}

	for _, sec := range sections {
		if !slices.Contains(inspectSections, sec) {
			return nil, newError(command.CodeValidation, "the section %q is not known.", sec).
				withPath("sections").withHint("use probe, transcript, silences, loudness or beats")
		}
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	pa, err := s.store.Assets().GetProjectAsset(ctx, row.ID, req.AssetID)
	if err != nil {
		return nil, newError(command.CodeNotFound, "the project has no asset %s.", req.AssetID).
			withHint("list the assets with media_inspect and no asset_id")
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return nil, asError(err)
	}

	res := &MediaInspectResult{
		UntrustedNotice: UntrustedNotice, AssetID: pa.AssetID, Kind: ar.Kind, State: string(ar.IngestState),
		Progress: ar.Progress, Available: []string{}, Untrusted: &UntrustedData{Name: pa.Name},
	}

	var text strings.Builder

	text.WriteString(textview.Note + "\n")
	fmt.Fprintf(&text, "asset %s %s %s %s", pa.AssetID, orDash(ar.Kind), ar.IngestState, textview.Quote(pa.Name, 80))

	if ar.IngestState != IngestReady {
		if ar.IngestState == IngestFailed {
			if res.Error = forCaller(c, parseError(ar.IngestError)); res.Error != nil {
				fmt.Fprintf(&text, "\nFAILED %s: %s", res.Error.Code, res.Error.Message)
			}
		} else {
			fmt.Fprintf(&text, "\n%d%%", int(ar.Progress*100))
		}

		res.Text = text.String()

		return res, nil
	}

	res.Progress = 1
	arts, _ := s.store.Assets().Artifacts(ctx, ar.Hash)
	res.Strip = stripOf(arts)

	for _, sec := range sections {
		switch sec {
		case "probe":
			text.WriteString(s.probeSection(ctx, &row, &ar, pa.AssetID, res))
		case "loudness":
			if l := s.loudnessOf(arts); l != nil {
				res.Loudness = l
				res.Available = append(res.Available, "loudness")
				text.WriteString("\n" + loudnessText(l))
			} else {
				text.WriteString("\nloudness: none (the asset has no audio)")
			}
		case "beats":
			text.WriteString(s.beatsSection(arts, res))
		case "silences":
			text.WriteString(s.silencesSection(arts, &row, req.MinSilence, res))
		case "transcript":
			section, terr := s.transcriptSection(ctx, arts, pa.AssetID, req.Cursor, res)
			if terr != nil {
				return nil, terr
			}

			text.WriteString(section)
		}
	}

	res.Text = text.String()

	return res, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

func probeText(fps timeval.FPS, p *ProbeSummary) string {
	size := textview.Size(p.SizeBytes)

	if p.Kind == string(ingest.KindImage) {
		if p.Animated {
			return fmt.Sprintf("animated image %dx%d | %d frames | loop %.3f s | %s",
				p.Width, p.Height, p.Frames, float64(p.LoopDurationMS)/1000, size)
		}

		return fmt.Sprintf("image %dx%d | %s", p.Width, p.Height, size)
	}

	parts := []string{p.Container}

	if p.Width > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", p.Width, p.Height))
	}

	if p.FrameRate != "" {
		parts = append(parts, p.FrameRate+"fps")
	}

	if p.Duration != nil {
		parts = append(parts, textview.Cell(fps, p.Duration.Frame))
	}

	if p.VideoCodec != "" {
		parts = append(parts, p.VideoCodec)
	}

	if p.HasAudio {
		parts = append(parts, fmt.Sprintf("audio %s %dHz %dch", p.AudioCodec, p.SampleRate, p.Channels))
	}

	return strings.Join(append(parts, size), " | ")
}

func (s *svc) loudnessOf(arts []ArtifactRow) *Loudness {
	raw := s.readArtifact(arts, "loudness", "")
	if raw == nil {
		return nil
	}

	var l struct {
		IntegratedLUFS *float64 `json:"integrated_lufs"`
		LRALU          float64  `json:"lra_lu"`
		TruePeakDBTP   *float64 `json:"true_peak_dbtp"`
	}

	if json.Unmarshal(raw, &l) != nil {
		return nil
	}

	return &Loudness{IntegratedLUFS: l.IntegratedLUFS, RangeLU: l.LRALU, TruePeakDBTP: l.TruePeakDBTP}
}

func loudnessText(l *Loudness) string {
	f := func(p *float64) string {
		if p == nil {
			return "-inf"
		}

		return strconv.FormatFloat(*p, 'f', 1, 64)
	}

	return fmt.Sprintf(
		"loudness: integrated %s LUFS | range %.1f LU | true peak %s dBTP",
		f(l.IntegratedLUFS),
		l.RangeLU,
		f(l.TruePeakDBTP),
	)
}

// maxBeatsShown is how many beats of the grid the section lists.
const maxBeatsShown = 16

// beatsSection fills res.Beats from beats-1.json: the tempo, how sure the detector is and the first beats.
// A grid without beats says why, so an agent knows audio.mark_beats will refuse the asset.
func (s *svc) beatsSection(arts []ArtifactRow, res *MediaInspectResult) string {
	raw := s.readArtifact(arts, "beats", "")
	if raw == nil {
		return "\nbeats: none (the asset has no audio, or it is not analysed yet)"
	}

	var doc struct {
		SampleRate int     `json:"sample_rate"`
		TempoBPM   float64 `json:"tempo_bpm"`
		Confidence float64 `json:"confidence"`
		Beats      []int64 `json:"beats"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 {
		return "\nbeats: none (the asset has no audio, or it is not analysed yet)"
	}

	grid := &BeatGrid{TempoBPM: doc.TempoBPM, Confidence: doc.Confidence, Count: len(doc.Beats), FirstMS: []int64{}}

	for i, b := range doc.Beats {
		if i == maxBeatsShown {
			break
		}

		grid.FirstMS = append(grid.FirstMS, b*1000/int64(doc.SampleRate))
	}

	res.Beats = grid
	res.Available = append(res.Available, "beats")

	if len(doc.Beats) == 0 {
		grid.Reason = "no steady pulse was found (speech, ambience, silence or a very irregular rhythm)"

		return "\nbeats: none, " + grid.Reason + "; audio.mark_beats needs a grid"
	}

	return fmt.Sprintf("\nbeats: %.1f BPM, confidence %.2f, %d beats; first at %v ms",
		grid.TempoBPM, grid.Confidence, grid.Count, grid.FirstMS)
}

// silencesSection fills res.Silences from silence-1.json: the ranges of the
// -35 dB set at least min_silence long, as whole project frames (SDD-011
// section 9.1), and returns the text.
func (s *svc) silencesSection(arts []ArtifactRow, row *ProjectRow, minSilence string, res *MediaInspectResult) string {
	fps := projectFPS(row)
	minMS := int64(400)

	if minSilence != "" {
		f, err := timeval.Parse(minSilence, fps)
		if err != nil {
			return "\nsilences: min_silence is not a time"
		}

		minMS = fps.Millis(f)
	}

	raw := s.readArtifact(arts, "silence", "")
	if raw == nil {
		return "\nsilences: none (the asset has no audio)"
	}

	var doc struct {
		SampleRate int `json:"sample_rate"`
		Sets       []struct {
			ThresholdDB int `json:"threshold_db"`
			Ranges      []struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"ranges"`
		} `json:"sets"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 {
		return "\nsilences: none (the asset has no audio)"
	}

	var b strings.Builder

	rate := int64(doc.SampleRate)
	total := int64(0)

	for _, set := range doc.Sets {
		if set.ThresholdDB != -35 {
			continue
		}

		for _, r := range set.Ranges {
			if (r.End-r.Start)*1000/rate < minMS {
				continue
			}

			start, end := fps.FramesInside(r.Start, r.End, rate)

			if end <= start {
				continue
			}

			res.Silences = append(res.Silences, TimeRange{
				Start: timeValue(timeline.Rational{Num: int(fps.Num), Den: int(fps.Den)}, start),
				End:   timeValue(timeline.Rational{Num: int(fps.Num), Den: int(fps.Den)}, end),
			})

			total += end - start
		}
	}

	res.Available = append(res.Available, "silences")
	fmt.Fprintf(
		&b,
		"\nsilences (min %dms, threshold -35 dB): %d ranges, %s in total",
		minMS,
		len(res.Silences),
		textview.Dur(fps, total),
	)

	for i, r := range res.Silences {
		if i == 60 {
			fmt.Fprintf(&b, "\n  ... and %d more", len(res.Silences)-i)
			break
		}

		fmt.Fprintf(&b, "\n  %s", textview.Range(fps, r.Start.Frame, r.End.Frame))
	}

	return b.String()
}

// OpenFile resolves the byte-serving routes of an asset (SDD-003 section 6.5).
func (s *svc) OpenFile(ctx context.Context, ref FileRef) (*File, error) {
	c, err := s.authorizeProject(ctx, "OpenFile", ref.ProjectID)
	if err != nil {
		return nil, err
	}

	if ref.Kind == "template" {
		return s.openTemplateFile(ctx, c, ref.ID)
	}

	row, lerr := s.loadProject(ctx, c, ref.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if ref.Kind == "bitmap" {
		return s.openBitmap(row.ID, ref.ID)
	}

	if ref.Kind == "audio_preview" {
		return s.openAudioPreview(row.ID, ref.ID, ref.Variant)
	}

	pa, err := s.store.Assets().GetProjectAsset(ctx, row.ID, ref.ID)
	if err != nil {
		return nil, newError(command.CodeNotFound, "the project has no asset %s.", ref.ID)
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return nil, asError(err)
	}

	if ar.IngestState != IngestReady {
		return nil, newError("E_ASSET_NOT_READY", "the asset is %s; its files are not ready.", ar.IngestState).
			withHint("wait for the asset to be ready")
	}

	arts, err := s.store.Assets().Artifacts(ctx, ar.Hash)
	if err != nil {
		return nil, asError(err)
	}

	var (
		kind, variant, ext, tag, ctype string
		profileInTag, immutable        bool
	)

	short := strings.TrimPrefix(ar.Hash, "sha256:")[:16]

	switch ref.Kind {
	case "proxy":
		kind, tag, profileInTag = "proxy", "proxy-"+short, true
	case "chunks":
		// Like the proxy it indexes: same scope, not immutable. A proxy written before the index existed
		// has none and answers 404; importing the file again writes it (SDD-011 section 4.4.1).
		kind, ext, tag, ctype, profileInTag = "chunks", "json", "chunks-"+short, "application/json", true
	case "thumbnail":
		w := ref.Variant
		if w == "" {
			w = "320"
		}

		if w != "160" && w != "320" && w != "640" {
			return nil, newError(command.CodeValidation, "the width %q is not offered.", w).
				withPath("width").withHint("use 160, 320 or 640")
		}

		kind, variant, ext, tag, ctype, immutable = "thumbnails", "w"+w, "jpg", "thumb-"+short+"-w"+w, "image/jpeg", true
	case "strip":
		kind, ext, tag, ctype, profileInTag, immutable = "thumbnails", "jpg", "strip-"+short, "image/jpeg", true, true
	case "peaks":
		kind, ext, tag, ctype, profileInTag, immutable = "peaks", "bin", "peaks-"+short, "application/octet-stream", true, true
	case "lut":
		if ar.Kind != string(timeline.AssetLUT) {
			return nil, newError(command.CodeNotFound, "the asset is not a lookup table.")
		}

		kind, ext, tag, ctype, immutable = "normalized", "cube", "lut-"+short, "application/octet-stream", true
	default:
		return nil, newError(command.CodeValidation, "the file kind %q is not served here.", ref.Kind).withPath("kind")
	}

	if arts, err = s.ensurePeaks(ctx, ref.Kind, ar, arts); err != nil {
		return nil, err
	}

	i := slices.IndexFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == kind && a.Variant == variant &&
			(ext == "" || a.Ext == ext || (kind == "thumbnails" && isThumbExt(a.Ext)))
	})
	if i < 0 {
		return nil, newError(command.CodeNotFound, "the asset has no %s file.", ref.Kind).
			withHint("an asset without video or audio has no such file")
	}

	if kind == "thumbnails" && arts[i].Ext == "png" {
		ctype = "image/png"
	}

	if ref.Kind == "proxy" {
		ctype = proxyContentType(arts[i].Ext, ar.Kind)
	}

	art := arts[i]
	if _, ok := s.artifactPath(art.RelPath); !ok {
		return nil, internalError()
	}

	f := &File{
		Path: art.RelPath, ContentType: ctype, Size: art.SizeBytes,
		ETag: etag(tag, art.Profile, profileInTag), ModTime: msTime(art.CreatedMs),
		Immutable: immutable, MaxAge: 86400,
	}

	if ref.Kind == "lut" {
		if f.Body, err = lutBody(s.artifactPath, art.RelPath); err != nil {
			return nil, err
		}

		f.Size = int64(len(f.Body))
	}

	return f, nil
}

// maxPeaksWAV is the largest normalized WAV whose peaks a request computes (SDD-011 section 6.5, SDD-007 section
// 9): about 93 minutes of 48 kHz stereo 16-bit audio, which is 690 MB an hour.
const maxPeaksWAV = 1 << 30

func hasPeaks(arts []ArtifactRow) bool {
	return slices.ContainsFunc(arts, func(a ArtifactRow) bool { return a.Kind == "peaks" && a.Ext == "bin" })
}

// ensurePeaks makes sure that an audio asset asked for its peaks has the artifact: when it has none, the server
// computes it (SDD-011 section 6.5). Any other request gets the rows back unchanged.
func (s *svc) ensurePeaks(ctx context.Context, kind string, ar AssetRow, arts []ArtifactRow) ([]ArtifactRow, error) {
	if kind != "peaks" || ar.Kind != string(timeline.AssetAudio) || hasPeaks(arts) {
		return arts, nil
	}

	// One computation at a time in the workspace: a page of sounds asks for every waveform at once, and the
	// requests that wait find the artifact the first one wrote instead of computing it again.
	if s.peaksGate != nil {
		select {
		case s.peaksGate <- struct{}{}:
			defer func() { <-s.peaksGate }()
		case <-ctx.Done():
			return nil, asError(ctx.Err())
		}

		fresh, err := s.store.Assets().Artifacts(ctx, ar.Hash)
		if err != nil {
			return nil, asError(err)
		}

		if hasPeaks(fresh) {
			return fresh, nil
		}

		arts = fresh
	}

	return s.computePeaks(ctx, ar, arts)
}

// computePeaks builds the peaks of an audio asset that has none from its stored normalized WAV, with the
// function ingest uses (SDD-011 section 4.6), registers the artifact and returns the new list of rows.
func (s *svc) computePeaks(ctx context.Context, ar AssetRow, arts []ArtifactRow) ([]ArtifactRow, error) {
	hash := ar.Hash

	i := slices.IndexFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == "normalized" && a.Ext == "wav"
	})
	if i < 0 {
		return nil, newError(command.CodeNotFound, "the asset has no peaks file.").
			withHint("an asset without audio has no such file")
	}

	src, ok := s.artifactPath(arts[i].RelPath)
	if !ok {
		return nil, internalError()
	}

	// A long file is the work of a job, not of a request (SDD-011 section 6.3.1).
	if arts[i].SizeBytes > maxPeaksWAV {
		return nil, s.peaksUpgradePending(ctx, ar, arts[i].SizeBytes)
	}

	pk, err := ingest.PeaksOfFileContext(ctx, src)
	if err != nil {
		if ctx.Err() != nil {
			return nil, asError(ctx.Err())
		}

		return nil, internalError()
	}

	return s.storePeaks(ctx, hash, pk)
}

// storePeaks writes the peaks file of an asset, registers it as an artifact and returns the new list of rows.
func (s *svc) storePeaks(ctx context.Context, hash string, pk *ingest.Peaks) ([]ArtifactRow, error) {
	body := pk.Marshal()
	rel := "artifacts/" + strings.TrimPrefix(hash, "sha256:") + "/" +
		ingest.ArtifactName("peaks", ingest.ProfilePeaks, "", "bin")

	dst, ok := s.artifactPath(rel)
	if !ok {
		return nil, internalError()
	}

	if err := writeFileAtomic(dst, body); err != nil {
		return nil, internalError()
	}

	sum := sha256.Sum256(body)

	if err := s.store.Assets().PutArtifact(ctx, ArtifactRow{
		AssetHash: hash, Kind: "peaks", Profile: ingest.ProfilePeaks, Ext: "bin", RelPath: rel,
		SizeBytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), CreatedMs: s.nowMs(),
	}); err != nil {
		return nil, asError(err)
	}

	rows, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil, asError(err)
	}

	return rows, nil
}

func proxyContentType(ext, assetKind string) string {
	switch ext {
	case "webm":
		// The proxy of a source with alpha (SDD-011 4.4); the browser plays it with its alpha.
		return "video/webm"
	case "mp4":
		if assetKind == string(timeline.AssetAudio) {
			return "audio/mp4"
		}

		return "video/mp4"
	case "png":
		return "image/png"
	default:
		return "image/jpeg"
	}
}

func etag(tag string, profile int, withProfile bool) string {
	if withProfile {
		tag += "-" + strconv.Itoa(profile)
	}

	return `"` + tag + `"`
}

// withRootsHint names the import roots in a refused path for the caller who may see
// the server's paths: an administrator, or anyone in local mode. A member of a
// network server gets the generic hint (SDD-007 section 7.3).
func (s *svc) withRootsHint(c Caller, err error) error {
	pe, ok := errors.AsType[*pathguard.Error](err)
	if !ok || pe.Code != pathguard.CodePathNotAllowed {
		return err
	}

	roots := s.media.Guard.Roots()
	if len(roots) == 0 || (c.OwnerUserID != "" && !slices.Contains(c.Scopes, ScopeAdmin)) {
		return err
	}

	out := *pe
	out.Hint += " Import roots: " + strings.Join(roots, ", ") + "."

	return &out
}
