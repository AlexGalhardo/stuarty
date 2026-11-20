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
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/lint"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
)

// defaultImageImportWaitMS is how long ImageImport waits for the ingest of a file when the caller gives no
// wait_ms, as media_import does over MCP: an import is of little use before the file is ready.
const defaultImageImportWaitMS = 10000

// imageSource is the file an import reads: a ready image of the workspace, and the name the caller knows it by.
type imageSource struct {
	hash   string
	name   string
	width  int
	height int
}

// ImageImport brings an image file into an image document (SDD-021 section 10.1).
func (s *svc) ImageImport(ctx context.Context, req ImageImportRequest) (*ImageImportResult, error) {
	c, err := imageAuthorize(ctx, "ImageImport")
	if err != nil {
		return nil, err
	}

	if cerr := checkImageImport(&req, false); cerr != nil {
		return nil, cerr
	}

	return s.runImageImport(ctx, c, &req, nil)
}

// runImageImport is the flow of ImageImport and ImageImportFile once the request is checked: the file is made
// ready (file, when it came as the bytes of the request), then a document or a layer is made of it.
func (s *svc) runImageImport(
	ctx context.Context,
	c Caller,
	req *ImageImportRequest,
	file *ImageImportFileRequest,
) (*ImageImportResult, error) {
	// An image the caller cannot see is refused before a file is read.
	if req.ImageID != "" {
		if _, _, lerr := s.loadImage(ctx, s.store.Images(), c, req.ImageID); lerr != nil {
			return nil, lerr
		}
	}

	src, early, err := s.imageImportSource(ctx, c, req, file)
	if err != nil || early != nil {
		return early, err
	}

	if req.ImageID != "" {
		return s.importIntoImage(ctx, req, src)
	}

	return s.importNewImage(ctx, c, req, src)
}

// checkImageImport is the part of the rules that needs no store. An upload carries its file, so it takes neither
// a path nor a hash (fromBody).
func checkImageImport(req *ImageImportRequest, fromBody bool) error {
	switch {
	case fromBody && (req.Path != "" || req.Hash != ""):
		return newError(command.CodeValidation, "an upload carries its own file: path and hash are not given.").
			withPath("path").withHint("use image:import for a file of the server or of the workspace")
	case !fromBody && (req.Path == "") == (req.Hash == ""):
		return newError(command.CodeValidation, "give the path of an image file or the hash of one in the workspace.").
			withPath("path").withHint("use path for a file on this machine, or hash for a file asset:list shows")
	}

	if req.Hash != "" {
		if _, err := parseHash(req.Hash); err != nil {
			return err
		}
	}

	if req.WaitMS < 0 || req.WaitMS > maxWaitMS {
		return newError(command.CodeValidation, "wait_ms is %d; it must be 0 to %d.", req.WaitMS, maxWaitMS).
			withPath("wait_ms")
	}

	if req.ImageID != "" {
		if !validImageID(req.ImageID) {
			return newError(command.CodeValidation, "the id %q is not an image id.", req.ImageID).withPath("image_id")
		}

		if req.Width != 0 || req.Height != 0 || req.Preset != "" {
			return newError(command.CodeValidation, "width, height and preset size a new document, not a layer.").
				withPath("width").withHint("leave them out when image_id is given")
		}

		if req.BaseRevision < 0 {
			return newError(command.CodeValidation, "base_revision is %d; it cannot be negative.", req.BaseRevision).
				withPath("base_revision")
		}

		return nil
	}

	if req.BaseRevision != 0 || req.IdempotencyKey != "" {
		return newError(command.CodeValidation, "base_revision and idempotency_key are for an existing image.").
			withPath("idempotency_key").withHint("give image_id, or leave them out for a new document")
	}

	return nil
}

// imageImportSource makes the file ready: a path enters the content store and its ingest is waited for, a hash is
// looked up. It returns the ready source, or an early answer (a dry run, a pending ingest), or an error; nothing
// of a document is written here.
func (s *svc) imageImportSource(
	ctx context.Context,
	c Caller,
	req *ImageImportRequest,
	file *ImageImportFileRequest,
) (imageSource, *ImageImportResult, error) {
	var (
		hash, name string
		err        error
	)

	switch {
	case file != nil:
		var early *ImageImportResult

		if hash, name, early, err = s.storeBody(ctx, c, req, file); err != nil || early != nil {
			return imageSource{}, early, err
		}
	case req.Path != "":
		var early *ImageImportResult

		if hash, name, early, err = s.storeImageFile(ctx, c, req); err != nil || early != nil {
			return imageSource{}, early, err
		}
	default:
		if hash, name, err = s.libraryImageFile(ctx, c, req.Hash); err != nil {
			return imageSource{}, nil, err
		}
	}

	row, pending, err := s.awaitImage(ctx, c, hash, req)
	if err != nil {
		return imageSource{}, nil, err
	}

	if pending != nil {
		pending.Name = name

		return imageSource{}, pending, nil
	}

	if row.Kind != "image" || row.Facts.Width == nil || row.Facts.Height == nil {
		return imageSource{}, nil, newError(
			"E_UNSUPPORTED_MEDIA",
			"the file is not an image, so it cannot be a layer.",
		).
			withHint("use a PNG, a JPEG or another still picture; a video goes in a project with media:import").
			withDetail("reason", "not_an_image").
			withDetail("kind", row.Kind)
	}

	return imageSource{hash: hash, name: name, width: *row.Facts.Width, height: *row.Facts.Height}, nil, nil
}

// storeImageFile reads a path into the content store and queues its ingest: the part of importFile that needs
// no project (SDD-011 section 2.3). A dry run reads nothing past the size check.
func (s *svc) storeImageFile(
	ctx context.Context,
	c Caller,
	req *ImageImportRequest,
) (hash, name string, early *ImageImportResult, err error) {
	if s.media.Guard == nil || s.media.Content == nil {
		return "", "", nil, internalError()
	}

	name = displayName(req.Path)

	file, oerr := s.media.Guard.OpenImport(req.Path)
	if oerr != nil {
		return "", "", nil, fromEngine(s.withRootsHint(c, oerr))
	}

	defer func() { _ = file.Close() }()

	st, serr := file.Stat()
	if serr != nil {
		return "", "", nil, fromEngine(serr)
	}

	if lerr := s.checkImportSize(st.Size()); lerr != nil {
		return "", "", nil, forCaller(c, lerr)
	}

	if req.DryRun {
		return "", "", &ImageImportResult{
			UntrustedNotice: UntrustedNotice, State: ImageImportPending, Name: name, DryRun: true,
			Findings: []lint.Finding{}, Text: withNote(fmt.Sprintf("dry run: %q would be imported", name)),
		}, nil
	}

	hash, err = s.putImageFile(ctx, c, file, name, s.limits.MaxImportBytes)
	if err != nil {
		return "", "", nil, err
	}

	return hash, name, nil, nil
}

// putImageFile copies r into the content store and records the file and its ingest: what a path and an upload
// share. The read side of the store lock covers the copy into store/ and the rows of the file, so an emptying
// cannot delete a file this import found (as in importFile). limit is the most it reads; one byte more is
// E_LIMIT_EXCEEDED.
func (s *svc) putImageFile(
	ctx context.Context,
	c Caller,
	r io.Reader,
	name string,
	limit int64,
) (string, error) {
	if derr := s.checkDisk(); derr != nil {
		return "", forCaller(c, derr)
	}

	s.storeMu.RLock()
	defer s.storeMu.RUnlock()

	put, perr := s.media.Content.Put(ctx, r, limit)
	if perr != nil {
		return "", forCaller(c, fromEngine(perr))
	}

	hash := "sha256:" + put.Hash

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		ar, _, uerr := tx.Assets().Upsert(ctx, AssetRow{
			Hash:         hash,
			SizeBytes:    put.Size,
			OriginalName: name,
			IngestState:  IngestPending,
			CreatedMs:    now,
			UpdatedMs:    now,
		})
		if uerr != nil {
			return uerr
		}

		if ar.IngestState == IngestFailed {
			if err := tx.Assets().SetIngest(ctx, hash, IngestPending, 0, nil, now); err != nil {
				return err
			}

			ar.IngestState = IngestPending
		}

		if ar.IngestState != IngestReady {
			if _, jerr := s.ensureIngestJob(ctx, tx, hash, c.Actor, now); jerr != nil {
				return jerr
			}
		}

		return assetlib.RecordImport(ctx, tx.AssetLibrary(), c.OwnerUserID, hash,
			asset.Origin{Kind: asset.Imported}, assetlib.ImportInfo{}, c.Actor, now)
	})
	if werr != nil {
		return "", forCaller(c, fromEngine(libraryRefusal(werr)))
	}

	return hash, nil
}

// libraryImageFile checks that the caller may use the file of a hash, and gives the name it is known by to them.
// A hash that is absent and one that is not the caller's answer the same (SDD-011 section 14.3).
func (s *svc) libraryImageFile(ctx context.Context, c Caller, in string) (hash, name string, err error) {
	hash, err = parseHash(in)
	if err != nil {
		return "", "", err
	}

	notThere := newError(command.CodeValidation, "the workspace has no file with this hash.").
		withPath("hash").withHint("list the files with asset:list")

	scopedName, scoped, ok := s.imageAssetAccess(ctx, c)(hash)
	if !ok {
		return "", "", notThere
	}

	row, gerr := s.store.Assets().Get(ctx, hash)
	if isNotFound(gerr) {
		return "", "", notThere
	}

	if gerr != nil {
		return "", "", asError(gerr)
	}

	name = displayName(row.OriginalName)
	if scoped {
		name = displayName(scopedName)
	}

	if name == "" {
		name = "Image"
	}

	return hash, name, nil
}

// awaitImage waits up to wait_ms for the ingest of a file and returns its row once it is ready. A file whose
// ingest failed answers the error of the ingest; one still running answers the pending result.
func (s *svc) awaitImage(
	ctx context.Context,
	c Caller,
	hash string,
	req *ImageImportRequest,
) (AssetRow, *ImageImportResult, error) {
	wait := time.Duration(req.WaitMS) * time.Millisecond
	if req.WaitMS == 0 {
		wait = defaultImageImportWaitMS * time.Millisecond
	}

	deadline := time.Now().Add(wait)

	for {
		row, err := s.store.Assets().Get(ctx, hash)
		if err != nil {
			return AssetRow{}, nil, asError(err)
		}

		switch row.IngestState {
		case IngestReady:
			return row, nil, nil
		case IngestFailed:
			if e := parseError(row.IngestError); e != nil {
				return AssetRow{}, nil, forCaller(c, e)
			}

			return AssetRow{}, nil, newError("E_INGEST_FAILED", "the ingest of the file failed.")
		case IngestPending, IngestIngesting:
			// still running: wait, or answer pending when the time is up
		}

		if time.Now().After(deadline) || ctx.Err() != nil {
			res := &ImageImportResult{
				UntrustedNotice: UntrustedNotice, State: ImageImportPending, Hash: hash,
				DryRun: req.DryRun, Findings: []lint.Finding{},
			}

			if j, jerr := s.store.Jobs().ActiveIngestForHash(ctx, hash); jerr == nil {
				res.JobID = j.ID
			}

			res.Text = fmt.Sprintf("file %s is still being ingested (job %s); import the hash again when it is ready",
				hash, res.JobID)

			return AssetRow{}, res, nil
		}

		select {
		case <-ctx.Done():
		case <-time.After(waitPollInterval):
		}
	}
}

// importIntoImage puts the file in an existing document: one layer.add_image through ImageEdit, so the base
// revision, the idempotency key, the dry run, the conflict check and the event are the ones of an edit.
func (s *svc) importIntoImage(
	ctx context.Context,
	req *ImageImportRequest,
	src imageSource,
) (*ImageImportResult, error) {
	name := req.Name
	if name == "" {
		name = src.name
	}

	add, err := coreimage.ImportLayer(src.hash, name)
	if err != nil {
		return nil, fromCommand(err)
	}

	edit, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: req.ImageID, BaseRevision: req.BaseRevision, Operations: []command.Operation{add},
		IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
	})
	if err != nil {
		return nil, err
	}

	res := &ImageImportResult{
		UntrustedNotice: UntrustedNotice, State: ImageImportReady, ImageID: edit.ImageID,
		Revision: edit.Revision, Hash: src.hash, Name: src.name, Width: src.width, Height: src.height,
		Applied: edit.Applied, DryRun: edit.DryRun, Replayed: edit.Replayed, Findings: edit.Findings, Lint: edit.Lint,
		Text: ImageEditText(edit),
	}

	if len(edit.Created) > 0 {
		res.LayerID = edit.Created[0]
	}

	return res, nil
}

// importNewImage makes a document from the file: image.create and layer.add_image in one transaction, so a
// refusal at any step leaves no document and no log row.
func (s *svc) importNewImage(
	ctx context.Context,
	c Caller,
	req *ImageImportRequest,
	src imageSource,
) (*ImageImportResult, error) {
	ops, berr := coreimage.ImportBatch(
		coreimage.ImportSource{Hash: src.hash, Width: src.width, Height: src.height, Name: src.name},
		coreimage.ImportCanvas{Preset: req.Preset, Width: req.Width, Height: req.Height}, req.Name,
	)
	if berr != nil {
		return nil, fromCommand(berr)
	}

	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	var res *ImageImportResult

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		id := s.newIDs(coreimage.PrefixImage)

		out, aerr := coreimage.ApplyBatch(coreimage.New(id), ops, s.imageEnv(ctx, tx, c, s.newIDs))
		if aerr != nil {
			return fromCommand(aerr)
		}

		doc := out.Doc
		findings := coreimage.Lint(doc, coreimage.LintEnv{})

		res = &ImageImportResult{
			UntrustedNotice: UntrustedNotice, State: ImageImportReady, ImageID: id, Revision: 1,
			Created: true, Hash: src.hash, Name: src.name, Width: doc.Width, Height: doc.Height, DryRun: req.DryRun,
			Applied: !req.DryRun, Findings: findings, Lint: totals(findings),
			Text: coreimage.Describe(doc, 1, findings),
		}

		for i := range doc.Layers {
			res.LayerID = doc.Layers[i].ID
		}

		if req.DryRun {
			res.ImageID, res.Revision = "", 0

			return nil
		}

		return s.createImportedImage(ctx, tx, c, id, doc, ops)
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// createImportedImage writes the row and the first log row of an imported document.
func (s *svc) createImportedImage(
	ctx context.Context,
	tx Tx,
	c Caller,
	id string,
	doc *coreimage.Document,
	ops []command.Operation,
) error {
	now := s.nowMs()
	u := imageUpdate(doc, now)

	var owner *string
	if c.OwnerUserID != "" {
		o := c.OwnerUserID
		owner = &o
	}

	row := ImageRow{
		ID: id, Name: u.Name, Template: u.Template, Revision: 1, SchemaVersion: u.SchemaVersion,
		Width: u.Width, Height: u.Height, Layers: u.Layers, CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor,
		OwnerUserID: owner, Document: u.Document, DocSHA256: u.DocSHA256,
	}

	if err := tx.Images().Create(ctx, row); err != nil {
		if errors.Is(err, ErrExists) {
			return newError(command.CodeIDExists, "the image %s already exists.", id).withPath("id")
		}

		return err
	}

	return tx.Images().AppendOp(ctx, ImageOpRow{
		ImageID: id, Revision: 1, Kind: OpEdit, Actor: c.Actor, BaseRevision: 0, AtMs: now, Ops: marshalOr(ops),
		Inverse: json.RawMessage("[]"), Diff: json.RawMessage("[]"), DocSHA256: u.DocSHA256,
	})
}
