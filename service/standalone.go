package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

const defaultStandaloneWaitMS = 15000

// StandaloneVideoListRequest lists standalone video editor sessions (SDD-005 section 5.12.5, epic 105).
type StandaloneVideoListRequest struct {
	State string `json:"state,omitempty"` // default "active"; "trashed" | "all"
	Page
}

// StandaloneVideoCreateRequest creates a standalone video editing session, optionally importing
// a local path or workspace library asset and placing it onto track V1 at 0s.
type StandaloneVideoCreateRequest struct {
	Name      string `json:"name,omitempty"`
	Preset    string `json:"preset,omitempty"` // "9:16", "1:1", "16:9", "4:5"; default "9:16" (or video dims when omitted)
	FPS       string `json:"fps,omitempty"`    // default "30"
	Path      string `json:"path,omitempty"`
	AssetHash string `json:"asset_hash,omitempty"`
	WaitMS    int    `json:"wait_ms,omitempty"`
	ID        string `json:"id,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// StandaloneVideoUploadRequest creates a standalone video editing session from a browser-uploaded file.
type StandaloneVideoUploadRequest struct {
	Name     string
	Preset   string
	FPS      string
	WaitMS   int
	FileName string
	Body     io.Reader
	Size     int64
	DryRun   bool
}

// StandaloneVideoResult is the created standalone video session and optional placed clip.
type StandaloneVideoResult struct {
	Envelope
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id"`
	Name       string `json:"name"                  untrusted:"true"`
	Standalone bool   `json:"standalone"`
	Revision   int64  `json:"revision"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPS        string `json:"fps"`
	TrackID    string `json:"track_id,omitempty"`
	AssetID    string `json:"asset_id,omitempty"`
	AssetHash  string `json:"asset_hash,omitempty"`
	AssetState string `json:"asset_state,omitempty"`
	ClipID     string `json:"clip_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	Created    bool   `json:"created"`
	Text       string `json:"text"`
}

// StandalonePromoteRequest converts a standalone video session into a full production project.
type StandalonePromoteRequest struct {
	ProjectID   string      `json:"project_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Goal        ProjectGoal `json:"goal"`
	Kind        string      `json:"kind,omitempty"`
	DryRun      bool        `json:"dry_run,omitempty"`
}

// ImageOpenFromAssetRequest opens a standalone image document directly from a workspace asset hash.
type ImageOpenFromAssetRequest struct {
	AssetHash string `json:"asset_hash"`
	Name      string `json:"name,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Preset    string `json:"preset,omitempty"`
	WaitMS    int    `json:"wait_ms,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// StandaloneVideoList lists the caller's standalone video editor sessions, newest first.
func (s *svc) StandaloneVideoList(
	ctx context.Context,
	req StandaloneVideoListRequest,
) (*ProjectListResult, error) {
	c, err := s.authorize(ctx, "StandaloneVideoList", "")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	q := ProductionQuery{
		StandaloneOnly: true,
		After:          req.Cursor,
		Limit:          clampLimit(req.Limit, defaultProjectLimit, maxProjectLimit),
		OwnerUserID:    c.OwnerUserID,
		ProjectID:      c.ProjectID,
	}

	switch req.State {
	case "", "active":
		req.State = "active"
	case "trashed":
		q.State = ProjectTrashed
	case "all":
		q.IncludeTrashed = true
	default:
		return nil, newError(command.CodeValidation, "the state %q is not known.", req.State).
			withPath("state").withHint("use active, trashed or all")
	}

	heads, next, err := s.store.Productions().List(ctx, q)
	if err != nil {
		return nil, listError(err)
	}

	lines := func(hs []ProductionHead) []textview.ProjectLine {
		out := make([]textview.ProjectLine, 0, len(hs))
		for i := range hs {
			h := &hs[i]
			out = append(out, textview.ProjectLine{
				ID:            h.ID,
				Name:          h.Name,
				Revision:      h.Timeline.Revision,
				Width:         h.Timeline.Summary.Width,
				Height:        h.Timeline.Summary.Height,
				FPS:           timeval.FPS{Num: int64(h.Timeline.Summary.FPSNum), Den: int64(h.Timeline.Summary.FPSDen)},
				DurationFrame: h.Timeline.Summary.DurationFrames,
				Items:         h.Timeline.Summary.Items,
				Timelines:     h.Timelines,
				Trashed:       h.State == ProjectTrashed,
			})
		}

		return out
	}

	if n := textview.FitProjectList(lines(heads)); n < len(heads) {
		q.Limit = n
		if heads, next, err = s.store.Productions().List(ctx, q); err != nil {
			return nil, listError(err)
		}
	}

	total, err := s.store.Productions().Count(ctx, ProductionQuery{
		State:          q.State,
		IncludeTrashed: q.IncludeTrashed,
		StandaloneOnly: true,
		OwnerUserID:    q.OwnerUserID,
		ProjectID:      q.ProjectID,
	})
	if err != nil {
		return nil, asError(err)
	}

	res := &ProjectListResult{UntrustedNotice: UntrustedNotice, Projects: []ProjectSummary{}}
	for i := range heads {
		sum := productionSummary(&heads[i])
		sum.Kind = string(project.KindVideo)
		res.Projects = append(res.Projects, sum)
	}

	res.Text = textview.ProjectList(lines(heads), total, req.State)
	if next != "" {
		res.NextCursor = &next
		res.Text += "\nnext_cursor: " + next
	}

	return res, nil
}

// StandaloneVideoCreate opens a standalone video editing session without creating a production project.
func (s *svc) StandaloneVideoCreate(
	ctx context.Context,
	req StandaloneVideoCreateRequest,
) (*StandaloneVideoResult, error) {
	c, err := s.authorize(ctx, "StandaloneVideoCreate", "")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot create projects.").
			withHint("use a key without a project limit")
	}

	if c.OwnerUserID != "" && req.ID != "" {
		return nil, newError(command.CodeValidation, "the id of a project is chosen by the server.").
			withPath("id").withHint("leave id out")
	}

	if req.Path != "" && req.AssetHash != "" {
		return nil, newError(command.CodeValidation, "give either path or asset_hash, not both.").
			withPath("path").withHint("omit one of path or asset_hash")
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	hasMedia := req.Path != "" || req.AssetHash != ""
	if hasMedia {
		if _, merr := s.authorizeScope(ctx, ScopeMedia, ""); merr != nil {
			return nil, merr
		}
	}

	// Validate media source before creating any rows so bad paths or unknown hashes leave no orphan session.
	derivedName := ""

	if req.Path != "" {
		if s.media.Guard == nil || s.media.Content == nil {
			return nil, internalError()
		}

		f, oerr := s.media.Guard.OpenImport(req.Path)
		if oerr != nil {
			return nil, fromEngine(s.withRootsHint(c, oerr))
		}

		st, serr := f.Stat()
		_ = f.Close()

		if serr != nil {
			return nil, fromEngine(serr)
		}

		if lerr := s.checkImportSize(st.Size()); lerr != nil {
			return nil, forCaller(c, lerr)
		}

		derivedName = project.StandaloneNameFromFile(displayName(req.Path))
	}

	normalizedHash := ""
	if req.AssetHash != "" {
		hash, perr := parseHash(req.AssetHash)
		if perr != nil {
			return nil, perr
		}

		ar, ok, gerr := s.checkReusableAsset(ctx, c, hash)
		if gerr != nil {
			return nil, gerr
		}

		if !ok {
			return nil, newError(command.CodeValidation, "the library has no file with this hash.").
				withPath("asset_hash").withHint("list the files with asset:list")
		}

		normalizedHash = hash
		derivedName = project.StandaloneNameFromFile(ar.OriginalName)
	}

	name := req.Name
	if name == "" {
		if derivedName != "" {
			name = derivedName
		} else {
			name = project.DefaultStandaloneName
		}
	}

	if verr := project.CheckStandaloneTitle(name); verr != nil {
		return nil, violation(verr)
	}

	doc, derr := s.newDocument(ProjectCreateRequest{
		Name: name, Preset: req.Preset, FPS: req.FPS, ID: req.ID,
	})
	if derr != nil {
		return nil, derr
	}

	res := &StandaloneVideoResult{
		Envelope:   Envelope{UntrustedNotice: UntrustedNotice},
		ProjectID:  doc.ID,
		TimelineID: doc.ID,
		Name:       doc.Name,
		Standalone: true,
		Width:      doc.Settings.Width,
		Height:     doc.Settings.Height,
		FPS:        fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den),
		AssetHash:  normalizedHash,
		Text:       fmt.Sprintf("%s\ncreated standalone video %s %s (%dx%d)", textview.Note, doc.ID, textview.Quote(doc.Name, 60), doc.Settings.Width, doc.Settings.Height),
	}

	if req.DryRun {
		return res, nil
	}

	if err := s.createStandaloneRows(ctx, c, doc); err != nil {
		return nil, err
	}

	res.Created = true

	trackID, rev, terr := s.addStandaloneTrack(ctx, doc.ID, 0)
	if terr != nil {
		return nil, terr
	}

	res.TrackID, res.Revision = trackID, rev

	if !hasMedia {
		return res, nil
	}

	waitMS := req.WaitMS
	if waitMS == 0 {
		waitMS = defaultStandaloneWaitMS
	}

	imp, ierr := s.MediaImport(ctx, MediaImportRequest{
		ProjectID: doc.ID,
		Files:     []ImportFile{{Path: req.Path, Hash: normalizedHash}},
		WaitMS:    waitMS,
	})
	if ierr != nil {
		return nil, ierr
	}

	if len(imp.Assets) > 0 {
		a := imp.Assets[0]
		res.AssetID = a.AssetID
		res.AssetHash = a.Hash
		res.AssetState = a.State
		res.JobID = a.JobID
		res.Revision = imp.Revision

		if a.State == string(IngestReady) {
			if err := s.placeStandaloneAsset(ctx, res, &a, req.Preset != ""); err != nil {
				return nil, err
			}
		}
	}

	return res, nil
}

// StandaloneVideoUpload streams a browser-uploaded video file into the content store and creates
// a standalone video editing session with the file placed on track V1 at 0s (SDD-005 section 5.12.5).
func (s *svc) StandaloneVideoUpload(
	ctx context.Context,
	req StandaloneVideoUploadRequest,
) (*StandaloneVideoResult, error) {
	c, err := s.authorize(ctx, "StandaloneVideoUpload", "")
	if err != nil {
		return nil, err
	}

	if _, merr := s.authorizeScope(ctx, ScopeMedia, ""); merr != nil {
		return nil, merr
	}

	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot create projects.").
			withHint("use a key without a project limit")
	}

	if req.Body == nil {
		return nil, newError(command.CodeValidation, "the request carries no file.").
			withPath("body").withHint("send the video file as the body of the request")
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return nil, newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	if s.media.Content == nil {
		return nil, internalError()
	}

	fileName := displayName(req.FileName)
	if fileName == "" {
		fileName = "video.mp4"
	}

	name := req.Name
	if name == "" {
		name = project.StandaloneNameFromFile(fileName)
	}

	if verr := project.CheckStandaloneTitle(name); verr != nil {
		return nil, violation(verr)
	}

	doc, derr := s.newDocument(ProjectCreateRequest{
		Name: name, Preset: req.Preset, FPS: req.FPS,
	})
	if derr != nil {
		return nil, derr
	}

	if req.Size >= 0 {
		if lerr := s.checkImportSize(req.Size); lerr != nil {
			return nil, forCaller(c, lerr)
		}
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, forCaller(c, derr)
	}

	res := &StandaloneVideoResult{
		Envelope:   Envelope{UntrustedNotice: UntrustedNotice},
		ProjectID:  doc.ID,
		TimelineID: doc.ID,
		Name:       doc.Name,
		Standalone: true,
		Width:      doc.Settings.Width,
		Height:     doc.Settings.Height,
		FPS:        fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den),
		Text:       fmt.Sprintf("%s\ncreated standalone video %s %s (%dx%d)", textview.Note, doc.ID, textview.Quote(doc.Name, 60), doc.Settings.Width, doc.Settings.Height),
	}

	if req.DryRun {
		return res, nil
	}

	// Stream bytes into the content store first so oversized or empty uploads never leave orphan sessions.
	s.storeMu.RLock()

	put, perr := s.media.Content.Put(ctx, req.Body, s.limits.MaxImportBytes)
	if perr != nil {
		s.storeMu.RUnlock()

		return nil, forCaller(c, fromEngine(perr))
	}

	if put.Size == 0 {
		s.storeMu.RUnlock()

		return nil, forCaller(c, s.checkImportSize(0))
	}

	hash := "sha256:" + put.Hash
	row, berr := s.projectRowForDoc(c, doc)
	if berr != nil {
		s.storeMu.RUnlock()

		return nil, berr
	}

	var att attached

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := row.CreatedMs
		if err := s.writeStandaloneRowsTx(ctx, tx, c, &row); err != nil {
			return err
		}

		ar, _, uerr := tx.Assets().Upsert(ctx, AssetRow{
			Hash:         hash,
			SizeBytes:    put.Size,
			OriginalName: fileName,
			IngestState:  IngestPending,
			CreatedMs:    now,
			UpdatedMs:    now,
		})
		if uerr != nil {
			return uerr
		}

		var aerr error
		if att, aerr = s.attach(ctx, tx, &row, ar, fileName, "", c.Actor, now); aerr != nil {
			return aerr
		}

		return assetlib.RecordImport(ctx, tx.AssetLibrary(), c.OwnerUserID, hash,
			asset.Origin{Kind: asset.Imported}, assetlib.ImportInfo{}, c.Actor, now)
	})
	s.storeMu.RUnlock()

	if werr != nil {
		return nil, forCaller(c, fromEngine(libraryRefusal(werr)))
	}

	res.Created = true

	if att.addNow != nil {
		if aerr := s.addAsset(ctx, &row, *att.addNow, att.asset, ""); aerr != nil {
			return nil, aerr
		}
	}

	imported := []ImportedAsset{{
		AssetID:      att.pa.AssetID,
		Hash:         hash,
		Name:         fileName,
		Kind:         att.asset.Kind,
		State:        string(att.asset.IngestState),
		JobID:        att.jobID,
		Deduplicated: put.Deduplicated,
		newInProject: att.fresh,
	}}

	waitMS := req.WaitMS
	if waitMS == 0 {
		waitMS = defaultStandaloneWaitMS
	}

	s.waitAssets(ctx, &row, imported, time.Duration(waitMS)*time.Millisecond)
	s.refreshImported(ctx, c, &row, &imported[0])

	curRow, herr := s.store.Projects().Get(ctx, doc.ID)
	if herr != nil {
		return nil, asError(herr)
	}

	trackID, rev, terr := s.addStandaloneTrack(ctx, doc.ID, curRow.Revision)
	if terr != nil {
		return nil, terr
	}

	a := imported[0]
	res.TrackID = trackID
	res.Revision = rev
	res.AssetID = a.AssetID
	res.AssetHash = a.Hash
	res.AssetState = a.State
	res.JobID = a.JobID

	if a.Error != nil {
		return nil, a.Error
	}

	if a.State == string(IngestReady) {
		if err := s.placeStandaloneAsset(ctx, res, &a, req.Preset != ""); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// StandalonePromote converts a standalone video session into a full production project,
// preserving all timeline edits, assets, history, and exports (SDD-005 section 5.12.5).
func (s *svc) StandalonePromote(
	ctx context.Context,
	req StandalonePromoteRequest,
) (*ProjectUpdateResult, error) {
	c, err := s.authorizeProject(ctx, "StandalonePromote", req.ProjectID)
	if err != nil {
		return nil, err
	}

	kind := project.KindVideo
	if req.Kind != "" {
		var kerr *project.RuleError

		kind, kerr = project.ParseKind(req.Kind)
		if kerr != nil {
			return nil, violation(kerr)
		}
	}

	var (
		res       *ProjectUpdateResult
		renameDoc bool
		base      int64
	)

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		head, _, werr := s.writableProduction(ctx, tx, c, req.ProjectID)
		if werr != nil {
			return werr
		}

		ctxIn := project.Context{
			Name:        req.Name,
			Description: req.Description,
			Goal:        project.Goal(req.Goal),
		}

		if errs := project.CheckPromote(
			head.Standalone,
			ctxIn,
			project.CheckOptions{Today: s.today(), CheckDue: true},
		); len(errs) > 0 {
			if errs[0].Code == command.CodeInvalidState {
				return violation(errs[0])
			}

			return checkContextError(errs)
		}

		res = &ProjectUpdateResult{
			Envelope:    Envelope{UntrustedNotice: UntrustedNotice},
			ProjectID:   head.ID,
			Name:        ctxIn.Name,
			Description: ctxIn.Description,
			Goal:        ProjectGoal(ctxIn.Goal),
			Complete:    ctxIn.Complete(),
			Text:        textview.UpdatedProject(head.ID, ctxIn.Name, ctxIn.Complete()),
		}

		if req.DryRun {
			return nil
		}

		now := s.nowMs()
		if err := tx.Productions().Promote(ctx, head.ID, ctxIn, kind, now); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, "project.promote", head.ID, ""); err != nil {
			return err
		}

		tx.AfterCommit(s.emitProject(&head, head.State, c.Actor, now))

		res.Updated = true

		if ctxIn.Name != head.Name && head.Timeline.ID == head.ID {
			renameDoc, base = true, head.Timeline.Revision
		}

		return nil
	})
	if err != nil {
		if se, ok := errors.AsType[*Error](err); ok {
			return nil, se
		}

		return nil, projectNotFound(req.ProjectID, err)
	}

	if renameDoc {
		s.renameFirstTimeline(ctx, res.ProjectID, base, res.Name)
	}

	return res, nil
}

// ImageOpenFromAsset opens a standalone image document directly from a workspace asset hash (epic 105).
func (s *svc) ImageOpenFromAsset(
	ctx context.Context,
	req ImageOpenFromAssetRequest,
) (*ImageImportResult, error) {
	c, err := s.imageAuthorize(ctx, "ImageOpenFromAsset")
	if err != nil {
		return nil, err
	}

	if req.AssetHash == "" {
		return nil, newError(command.CodeValidation, "asset_hash is required.").
			withPath("asset_hash").withHint("send the content hash of an image asset from asset:list")
	}

	imp := ImageImportRequest{
		Hash:   req.AssetHash,
		Name:   req.Name,
		Width:  req.Width,
		Height: req.Height,
		Preset: req.Preset,
		WaitMS: req.WaitMS,
		DryRun: req.DryRun,
	}

	if cerr := checkImageImport(&imp, false); cerr != nil {
		return nil, cerr
	}

	return s.runImageImport(ctx, c, &imp, nil)
}

func (s *svc) checkReusableAsset(ctx context.Context, c Caller, hash string) (AssetRow, bool, error) {
	if c.OwnerUserID != "" || c.ProjectID != "" {
		scan, serr := s.libraryScan(ctx, c)
		if serr != nil {
			return AssetRow{}, false, serr
		}

		if _, held := scan[hash]; !held {
			return AssetRow{}, false, nil
		}
	}

	ar, err := s.store.Assets().Get(ctx, hash)
	if isNotFound(err) {
		return AssetRow{}, false, nil
	}

	if err != nil {
		return AssetRow{}, false, asError(err)
	}

	return ar, true, nil
}

func (s *svc) projectRowForDoc(c Caller, doc *timeline.Document) (ProjectRow, error) {
	raw := timeline.Canonical(doc)

	now := s.nowMs()
	row := ProjectRow{
		ProjectHead: ProjectHead{
			ID:            doc.ID,
			Name:          doc.Name,
			State:         ProjectActive,
			Revision:      0,
			SchemaVersion: doc.SchemaVersion,
			Summary:       summarize(doc),
			CreatedMs:     now,
			UpdatedMs:     now,
			CreatedBy:     c.Actor,
		},
		Document:  raw,
		DocSHA256: sha256Hex(raw),
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	return row, nil
}

func (s *svc) createStandaloneRows(ctx context.Context, c Caller, doc *timeline.Document) error {
	row, err := s.projectRowForDoc(c, doc)
	if err != nil {
		return err
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return s.writeStandaloneRowsTx(ctx, tx, c, &row)
	})
	if err != nil {
		if errors.Is(err, ErrExists) {
			return newError(command.CodeIDExists, "a project with the identifier %s already exists.", doc.ID).
				withHint("choose another id or leave it out").withPath("id")
		}

		return asError(err)
	}

	return nil
}

func (s *svc) writeStandaloneRowsTx(ctx context.Context, tx Tx, c Caller, row *ProjectRow) error {
	if _, herr := tx.Projects().Head(ctx, row.ID); herr == nil {
		return ErrExists
	} else if !errors.Is(herr, ErrNotFound) {
		return herr
	}

	head := ProductionHead{
		ID:          row.ID,
		Name:        row.Name,
		State:       ProjectActive,
		CreatedMs:   row.CreatedMs,
		UpdatedMs:   row.UpdatedMs,
		CreatedBy:   row.CreatedBy,
		OwnerUserID: row.OwnerUserID,
		Kind:        project.KindVideo,
		Standalone:  true,
	}

	if err := tx.Productions().Create(ctx, head); err != nil {
		return err
	}

	it := s.newItem(row.ID, string(project.Timeline), row.Name, 0, row.CreatedBy, row.CreatedMs)
	it.Ref = row.ID

	if err := tx.Productions().CreateItem(ctx, it); err != nil {
		return err
	}

	if err := tx.Projects().Create(ctx, *row); err != nil {
		return err
	}

	details, _ := json.Marshal(map[string]any{"standalone": true})

	return tx.Audit().Append(ctx, AuditRow{
		AtMs:      row.CreatedMs,
		Actor:     c.Actor,
		Action:    "project.create",
		ProjectID: &row.ID,
		Details:   details,
	})
}

func (s *svc) addStandaloneTrack(ctx context.Context, projectID string, baseRev int64) (string, int64, error) {
	args, err := json.Marshal(map[string]any{"kind": "visual", "name": "V1"})
	if err != nil {
		return "", 0, asError(err)
	}

	ed, err := s.Edit(ctx, EditRequest{
		ProjectID:    projectID,
		TimelineID:   projectID,
		BaseRevision: baseRev,
		Operations:   []command.Operation{{Op: "track.add", Args: args}},
	})
	if err != nil {
		return "", 0, err
	}

	trackID := ""
	if len(ed.Created) > 0 {
		trackID = ed.Created[0]
	} else if len(ed.Effective) > 0 {
		if id, ok := ed.Effective[0].Effective["id"].(string); ok {
			trackID = id
		} else if id, ok := ed.Effective[0].Effective["track_id"].(string); ok {
			trackID = id
		}
	}

	return trackID, ed.Revision, nil
}

func (s *svc) placeStandaloneAsset(
	ctx context.Context,
	res *StandaloneVideoResult,
	a *ImportedAsset,
	explicitPreset bool,
) error {
	if a.Kind != "video" && a.Kind != "image" {
		return nil
	}

	var ops []command.Operation

	if !explicitPreset && a.Hash != "" {
		if ar, err := s.store.Assets().Get(ctx, a.Hash); err == nil &&
			ar.Facts.Width != nil && ar.Facts.Height != nil {
			if w, h, ok := project.StandaloneDimensions(*ar.Facts.Width, *ar.Facts.Height); ok &&
				(w != res.Width || h != res.Height) {
				sArgs, merr := json.Marshal(map[string]any{"width": w, "height": h})
				if merr != nil {
					return asError(merr)
				}

				ops = append(ops, command.Operation{Op: "settings.set", Args: sArgs})
				res.Width, res.Height = w, h
			}
		}
	}

	if res.TrackID != "" && a.AssetID != "" {
		cArgs, err := json.Marshal(map[string]any{
			"track_id": res.TrackID,
			"asset_id": a.AssetID,
			"start":    "0s",
		})
		if err != nil {
			return asError(err)
		}

		ops = append(ops, command.Operation{Op: "clip.add", Args: cArgs})
	}

	if len(ops) == 0 {
		return nil
	}

	ed, err := s.Edit(ctx, EditRequest{
		ProjectID:    res.ProjectID,
		TimelineID:   res.TimelineID,
		BaseRevision: res.Revision,
		Operations:   ops,
	})
	if err != nil {
		return err
	}

	res.Revision = ed.Revision

	// Clip is the last operation in ops when added.
	if len(ed.Created) > 0 {
		res.ClipID = ed.Created[len(ed.Created)-1]
	} else if len(ed.Effective) > 0 {
		if id, ok := ed.Effective[len(ed.Effective)-1].Effective["id"].(string); ok {
			res.ClipID = id
		}
	}

	res.Text = fmt.Sprintf("%s (%dx%d, rev %d)", res.ProjectID, res.Width, res.Height, res.Revision)

	return nil
}
