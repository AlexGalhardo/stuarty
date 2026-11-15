package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/service/textview"
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
// with control, bidirectional and separator characters replaced by "_" and cut
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
		case unicode.IsControl(r), isBidi(r), r == '/', r == '\\':
			r = '_'
		}

		b.WriteRune(r)

		n++
	}

	return b.String()
}

func isBidi(r rune) bool {
	return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0x200E || r == 0x200F || r == 0x061C
}

func (s *svc) MediaImport(ctx context.Context, req MediaImportRequest) (*MediaImportResult, error) {
	c, err := authorize(ctx, "MediaImport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.heavyLimit(ctx, c); err != nil {
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
		res.Assets[i] = s.importFile(ctx, c, &row, f, req.DryRun)
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

	for i := range res.Assets {
		s.refreshImported(ctx, c, &row, &res.Assets[i])
	}

	if head, herr := s.store.Projects().Get(ctx, row.ID); herr == nil {
		res.Revision = head.Revision
	}

	lines := make([]textview.ImportLine, len(res.Assets))
	for i, a := range res.Assets {
		lines[i] = textview.ImportLine{
			ID: a.AssetID, Kind: a.Kind, State: a.State, Name: a.Name, Deduplicated: a.Deduplicated, JobID: a.JobID,
		}

		if a.Error != nil {
			lines[i].ErrorCode, lines[i].ErrorMessage = a.Error.Code, a.Error.Message
		}
	}

	res.Text = textview.Import(lines)

	return res, nil
}

// importFile is the flow of SDD-011 section 2.3 for one file. It never fails
// the call: a refusal is the Error of the entry.
func (s *svc) importFile(ctx context.Context, c Caller, project *ProjectRow, f ImportFile, dryRun bool) ImportedAsset {
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
		return refuse(newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id"))
	}

	file, err := s.media.Guard.OpenImport(f.Path)
	if err != nil {
		return refuse(err)
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

	var (
		jobID  string
		addNow *ProjectAssetRow
		asset  AssetRow
		pa     ProjectAssetRow
	)

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		ar, _, uerr := tx.Assets().Upsert(ctx, AssetRow{
			Hash: hash, SizeBytes: put.Size, OriginalName: out.Name, IngestState: IngestPending,
			CreatedMs: now, UpdatedMs: now,
		})
		if uerr != nil {
			return uerr
		}

		if ar.IngestState == IngestFailed {
			if err := tx.Assets().SetIngest(ctx, hash, IngestPending, 0, nil, now); err != nil {
				return err
			}

			ar.IngestState, ar.Progress, ar.IngestError = IngestPending, 0, nil
		}

		row, perr := s.linkProjectAsset(ctx, tx, project.ID, hash, out.Name, f.AssetID, c.Actor, now)
		if perr != nil {
			return perr
		}

		asset, pa = ar, row

		// Registered before the enqueue: an inline queue runs the job in its own
		// after-commit hook, and this event must come first.
		tx.AfterCommit(func() { s.emitAssetFor(project.ID, row.AssetID, ar.IngestState, ar.Progress, nil) })

		switch ar.IngestState {
		case IngestReady:
			if row.State == ProjectAssetPending {
				addNow = &row
			}
		default:
			id, jerr := s.ensureIngestJob(ctx, tx, hash, c.Actor, now)
			if jerr != nil {
				return jerr
			}

			jobID = id
		}

		return nil
	})
	s.storeMu.RUnlock()

	if werr != nil {
		return refuse(werr)
	}

	out.AssetID, out.JobID, out.Kind = pa.AssetID, jobID, asset.Kind

	if addNow != nil {
		if aerr := s.addAsset(ctx, project, *addNow, asset, ""); aerr != nil {
			out.Error = aerr
		}
	}

	return out
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

		return newError("E_LIMIT_EXCEEDED", "The file is %s; the import limit is %s (VUKEM_VIDEO_MAX_IMPORT_BYTES).",
			have, ceiling).
			withHint("import a smaller file; the person running the server can raise VUKEM_VIDEO_MAX_IMPORT_BYTES").
			withDetail("limit", "import_bytes").withDetail("value", size).
			withDetail("ceiling", s.limits.MaxImportBytes).withDetail("variable", "VUKEM_VIDEO_MAX_IMPORT_BYTES")
	}

	if s.limits.MinFreeBytes > 0 && s.workspace != "" {
		if free, err := s.media.FreeBytes(s.workspace); err == nil &&
			free < uint64(size)+uint64(s.limits.MinFreeBytes) { //nolint:gosec // both are positive
			return newError("E_LIMIT_EXCEEDED", "The disk of the workspace has too little free space for this file.").
				withHint("free space on the disk of the workspace; the person running the server can lower "+
					"VUKEM_VIDEO_MIN_FREE_BYTES").
				withDetail("limit", "free_bytes").withDetail("reason", "disk_low").
				withDetail("variable", "VUKEM_VIDEO_MIN_FREE_BYTES")
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
	active, err := tx.Jobs().ActiveIngestForHash(ctx, hash)
	if err == nil {
		return active.ID, nil
	}

	if !isNotFound(err) {
		return "", err
	}

	input := marshalOrNull(map[string]string{"asset_hash": hash, "reason": "import"})
	h := hash
	job := JobRow{
		ID: s.newIDs("job"), Kind: JobIngest, AssetHash: &h, Actor: actor, State: JobQueued, Input: input,
		CreatedMs: now, UpdatedMs: now,
	}

	lim := &JobLimit{Kind: JobIngest, Max: s.limits.MaxQueuedIngests + s.limits.MaxIngests}
	if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
		return "", err
	}

	// Registered before the enqueue: an inline queue runs the job in its own
	// after-commit hook, and the queued event must come first.
	queued := job
	tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

	if s.enqueue != nil {
		if err := s.enqueue.Enqueue(ctx, tx, QueueKindIngest, job.ID); err != nil {
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
		ID: pa.AssetID, Hash: ar.Hash, Kind: timeline.AssetKind(ar.Kind), Name: pa.Name, HasAudio: ar.Facts.HasAudio,
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
		ProjectID: project.ID, BaseRevision: head.Revision, IdempotencyKey: key,
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
				if _, jerr := s.store.Jobs().ActiveIngestForHash(ctx, pa.AssetHash); jerr == nil {
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
		if active, jerr := s.store.Jobs().ActiveIngestForHash(ctx, pa.AssetHash); jerr == nil {
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

// probeOf is the ProbeSummary of a ready asset, from its probe artifact.
func (s *svc) probeOf(ctx context.Context, project *ProjectRow, ar *AssetRow) *ProbeSummary {
	sum := s.readSummary(ctx, ar.Hash)
	if sum == nil {
		return nil
	}

	fps := projectFPS(project)
	p := &ProbeSummary{Kind: string(sum.Kind), Container: sum.Container, SizeBytes: sum.SizeBytes}

	if sum.Kind != ingest.KindImage && fps.Valid() {
		tv := timeValue(timeline.Rational{Num: int(fps.Num), Den: int(fps.Den)}, fps.FrameAtMillis(sum.DurationMS))
		p.Duration = &tv
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

// readSummary reads probe-1.json of an asset; nil when it cannot be read.
func (s *svc) readSummary(ctx context.Context, hash string) *ingest.Summary {
	arts, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil
	}

	raw := s.readArtifact(arts, "probe", "")
	if raw == nil {
		return nil
	}

	var sum ingest.Summary
	if json.Unmarshal(raw, &sum) != nil {
		return nil
	}

	return &sum
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
	c, err := authorize(ctx, "MediaList", req.ProjectID)
	if err != nil {
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
		frames := fps.FrameAtMillis(*ar.Facts.DurationMs)
		secs := round3(float64(*ar.Facts.DurationMs) / 1000)
		e.DurationFrames, e.DurationSeconds = &frames, &secs
	}

	if arts, err := s.store.Assets().Artifacts(ctx, ar.Hash); err == nil {
		e.Strip = stripOf(arts)
	}

	return e
}

func stripOf(arts []ArtifactRow) *Strip {
	i := slices.IndexFunc(
		arts,
		func(a ArtifactRow) bool { return a.Kind == "thumbnails" && a.Variant == "" && a.Ext == "jpg" },
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

var inspectSections = []string{"probe", "transcript", "silences", "loudness"}

func (s *svc) MediaInspect(ctx context.Context, req MediaInspectRequest) (*MediaInspectResult, error) {
	c, err := authorize(ctx, "MediaInspect", req.ProjectID)
	if err != nil {
		return nil, err
	}

	sections := req.Sections
	if len(sections) == 0 {
		sections = []string{"probe"}
	}

	for _, sec := range sections {
		if !slices.Contains(inspectSections, sec) {
			return nil, newError(command.CodeValidation, "the section %q is not known.", sec).
				withPath("sections").withHint("use probe, transcript, silences or loudness")
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

			start := ceilDiv(r.Start*fps.Num, rate*fps.Den)
			end := r.End * fps.Num / (rate * fps.Den)

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

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }

// OpenFile resolves the byte-serving routes of an asset (SDD-003 section 6.5).
func (s *svc) OpenFile(ctx context.Context, ref FileRef) (*File, error) {
	c, err := authorize(ctx, "OpenFile", ref.ProjectID)
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
	default:
		return nil, newError(command.CodeValidation, "the file kind %q is not served here.", ref.Kind).withPath("kind")
	}

	i := slices.IndexFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == kind && a.Variant == variant && (ext == "" || a.Ext == ext)
	})
	if i < 0 {
		return nil, newError(command.CodeNotFound, "the asset has no %s file.", ref.Kind).
			withHint("an asset without video or audio has no such file")
	}

	if ref.Kind == "proxy" {
		switch arts[i].Ext {
		case "mp4":
			ctype = "video/mp4"
			if ar.Kind == string(timeline.AssetAudio) {
				ctype = "audio/mp4"
			}
		case "png":
			ctype = "image/png"
		default:
			ctype = "image/jpeg"
		}
	}

	art := arts[i]
	if _, ok := s.artifactPath(art.RelPath); !ok {
		return nil, internalError()
	}

	return &File{
		Path: art.RelPath, ContentType: ctype, Size: art.SizeBytes,
		ETag: etag(tag, art.Profile, profileInTag), ModTime: msTime(art.CreatedMs),
		Immutable: immutable, MaxAge: 86400,
	}, nil
}

func etag(tag string, profile int, withProfile bool) string {
	if withProfile {
		tag += "-" + strconv.Itoa(profile)
	}

	return `"` + tag + `"`
}
