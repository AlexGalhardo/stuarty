package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

const (
	defaultImageLimit = 20
	maxImageLimit     = 100
)

func imageNotFound(id string) *Error {
	return newError(command.CodeNotFound, "there is no image %s.", id).
		withHint("list the images with image:list")
}

// imageEnv is the environment of a batch inside a transaction: ids, the files of the workspace and the measurer.
func (s *svc) imageEnv(ctx context.Context, tx Repos, c Caller, ids func(string) string) coreimage.Env {
	allowed := s.imageAssetAccess(ctx, c)

	return coreimage.Env{
		NewIDs: ids,
		Asset: func(hash string) (coreimage.AssetInfo, bool) {
			scopedName, scoped, ok := allowed(hash)
			if !ok {
				return coreimage.AssetInfo{}, false
			}

			row, err := tx.Assets().Get(ctx, hash)
			if err != nil || row.Kind != "image" || row.IngestState != IngestReady ||
				row.Facts.Width == nil || row.Facts.Height == nil {
				return coreimage.AssetInfo{}, false
			}

			// the global name is the caller own only in local mode; in network mode the name of the project
			// entry is used even when it is empty, and an empty one gives a neutral name
			name := row.OriginalName
			if scoped {
				name = scopedName
			}

			if name == "" {
				name = "Image"
			}

			return coreimage.AssetInfo{
				Name: filepath.Base(name), Width: *row.Facts.Width, Height: *row.Facts.Height,
			}, true
		},
		Measure: rasterize.Measure(rasterize.Bundled()),
	}
}

// imageAssetAccess says whether the caller may put a file in an image. In local mode every file of the
// workspace is the caller own. In network mode the file must be held by a project of the caller, the rule of
// the asset library (AssetList): a hash alone proves nothing, and the name of the file of another user must not
// leak through an image. The projects are scanned once, when the first file is asked for.
// It also gives the name the file is known by to the caller: in network mode the name of the project entry of
// the caller, never the global one, which another user may have set.
func (s *svc) imageAssetAccess(ctx context.Context, c Caller) func(hash string) (name string, scoped, ok bool) {
	if c.OwnerUserID == "" {
		return func(string) (string, bool, bool) { return "", false, true }
	}

	var (
		held map[string]*libraryEntry
		done bool
	)

	return func(hash string) (string, bool, bool) {
		if !done {
			scan, err := s.libraryScan(ctx, c)
			if err != nil {
				scan = nil
			}

			held, done = scan, true
		}

		e, ok := held[hash]
		if !ok {
			return "", false, false
		}

		return e.name, true, true
	}
}

// imageAuthorize is authorize for the image operations: a key limited to one project reaches no image, since an
// image belongs to no project until the production exists (SDD-021 section 10).
func imageAuthorize(ctx context.Context, method string) (Caller, error) {
	c, err := authorize(ctx, method, "")
	if err != nil {
		return Caller{}, err
	}

	if c.ProjectID != "" {
		return Caller{}, newError(CodeForbidden, "the key is limited to one project and cannot use images.").
			withHint("use a key without a project limit")
	}

	return c, nil
}

// loadImage reads a document the caller may see.
func (s *svc) loadImage(
	ctx context.Context,
	repo ImageRepo,
	c Caller,
	id string,
) (ImageRow, *coreimage.Document, error) {
	row, err := repo.Get(ctx, id)
	if errors.Is(err, ErrNotFound) || err == nil && !visible(c, row.OwnerUserID) {
		return ImageRow{}, nil, imageNotFound(id)
	}

	if err != nil {
		return ImageRow{}, nil, asError(err)
	}

	doc, derr := coreimage.Decode(row.Document)
	if derr != nil {
		return ImageRow{}, nil, internalError()
	}

	return row, doc, nil
}

func imageUpdate(doc *coreimage.Document, now int64) ImageUpdate {
	canon := coreimage.Canonical(doc)

	return ImageUpdate{
		Name: doc.Name, Template: doc.Template, SchemaVersion: doc.SchemaVersion, Document: canon,
		DocSHA256: sha256Hex(canon), Width: doc.Width, Height: doc.Height, Layers: coreimage.Count(doc), UpdatedMs: now,
	}
}

func (s *svc) ImageCreate(ctx context.Context, req ImageCreateRequest) (*ImageCreateResult, error) {
	c, err := imageAuthorize(ctx, "ImageCreate")
	if err != nil {
		return nil, err
	}

	// In network mode the server picks the identifier: a client-chosen one would answer E_ID_EXISTS for the
	// image of another user (an existence oracle).
	if c.OwnerUserID != "" && req.ID != "" {
		return nil, newError(command.CodeValidation, "the id of an image is chosen by the server.").
			withPath("id").withHint("leave id out")
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, forCaller(c, derr)
	}

	var res *ImageCreateResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		id := req.ID
		if id == "" {
			id = s.newIDs(coreimage.PrefixImage)
		}

		doc, derr := s.newImageDocument(ctx, tx, c, &req, id)
		if derr != nil {
			return derr
		}

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
			OwnerUserID: owner,
			Document:    u.Document, DocSHA256: u.DocSHA256,
		}

		if err := tx.Images().Create(ctx, row); err != nil {
			if errors.Is(err, ErrExists) {
				return newError(command.CodeIDExists, "the image %s already exists.", id).withPath("id")
			}

			return err
		}

		ops := marshalOr([]command.Operation{creationOp(&req)})

		if err := tx.Images().AppendOp(ctx, ImageOpRow{
			ImageID: id, Revision: 1, Kind: OpEdit, Actor: c.Actor, BaseRevision: 0, AtMs: now, Ops: ops,
			Inverse: json.RawMessage("[]"), Diff: json.RawMessage("[]"), DocSHA256: u.DocSHA256,
		}); err != nil {
			return err
		}

		res = &ImageCreateResult{
			UntrustedNotice: UntrustedNotice, ImageID: id, Revision: 1, Width: doc.Width,
			Height: doc.Height, Text: coreimage.Describe(doc, 1, coreimage.Lint(doc, coreimage.LintEnv{})),
		}

		return nil
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// creationOp is the operation the log keeps for the first revision.
func creationOp(req *ImageCreateRequest) command.Operation {
	args := marshalOr(req)

	return command.Operation{Op: "image.create", Args: args}
}

// newImageDocument makes the first revision: blank, with the image.create command, or from a template.
func (s *svc) newImageDocument(
	ctx context.Context,
	tx Tx,
	c Caller,
	req *ImageCreateRequest,
	id string,
) (*coreimage.Document, error) {
	if !validImageID(id) {
		return nil, newError(command.CodeValidation, "the id %q is not an image id.", id).
			withPath("id").withHint("an image id is img_ and 10 characters from 0-9 and a-z, without i, l, o and u")
	}

	env := s.imageEnv(ctx, tx, c, s.newIDs)

	if req.TemplateID != "" {
		if req.Preset != "" || req.Width != 0 || req.Height != 0 || req.Template {
			return nil, newError(command.CodeValidation, "a document made from a template takes its size from it.").
				withPath("template_id").withHint("give template_id and values only")
		}

		_, tpl, err := s.loadImage(ctx, tx.Images(), c, req.TemplateID)
		if err != nil {
			return nil, err
		}

		doc, ferr := coreimage.FromTemplate(tpl, req.Values, id, env)
		if ferr != nil {
			return nil, fromCommand(ferr)
		}

		if req.Name != "" {
			doc.Name = req.Name
		}

		return doc, nil
	}

	if len(req.Values) > 0 {
		return nil, newError(command.CodeValidation, "values need a template_id.").withPath("values")
	}

	args := map[string]any{}
	if req.Preset != "" {
		args["preset"] = req.Preset
	}

	if req.Width != 0 {
		args["width"] = req.Width
	}

	if req.Height != 0 {
		args["height"] = req.Height
	}

	if req.Name != "" {
		args["name"] = req.Name
	}

	if req.Background != "" {
		args["background"] = req.Background
	}

	if req.Template {
		args["template"] = true
	}

	raw := marshalOr(args)

	out, err := coreimage.ApplyBatch(coreimage.New(id), []command.Operation{{Op: "image.create", Args: raw}}, env)
	if err != nil {
		return nil, fromCommand(err)
	}

	return out.Doc, nil
}

func validImageID(id string) bool { return timeline.ValidID(coreimage.PrefixImage, id) }

func (s *svc) ImageList(ctx context.Context, req ImageListRequest) (*ImageListResult, error) {
	c, err := imageAuthorize(ctx, "ImageList")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	if !slices.Contains([]string{"", "only", "none"}, req.Templates) {
		return nil, newError(
			command.CodeValidation,
			"templates is %q; use only or none.",
			req.Templates,
		).withPath("templates")
	}

	off := 0

	if req.Cursor != "" {
		o, derr := decodeOffset(req.Cursor, "images", "")
		if derr != nil {
			return nil, derr
		}

		off = o
	}

	limit := clampLimit(req.Limit, defaultImageLimit, maxImageLimit)

	heads, err := s.store.Images().List(ctx, ImageQuery{
		OwnerUserID: c.OwnerUserID, Offset: off, Limit: limit, Templates: req.Templates,
	})
	if err != nil {
		return nil, asError(err)
	}

	res := &ImageListResult{UntrustedNotice: UntrustedNotice, Images: []ImageSummary{}}

	if len(heads) > limit {
		heads = heads[:limit]
		next := offsetCursor("images", "", off+limit)
		res.NextCursor = &next
	}

	text := fmt.Sprintf("images: %d", len(heads))

	var textSb293 strings.Builder
	for i := range heads {
		h := &heads[i]
		res.Images = append(res.Images, ImageSummary{
			ImageID: h.ID, Name: h.Name, Template: h.Template, Revision: h.Revision, Width: h.Width, Height: h.Height,
			Layers: h.Layers, UpdatedMs: h.UpdatedMs,
		})
		fmt.Fprintf(&textSb293, "\n%s r%d %q %dx%d layers=%d template=%t",
			h.ID,
			h.Revision,
			h.Name,
			h.Width,
			h.Height,
			h.Layers,
			h.Template)
	}
	text += textSb293.String()

	res.Text = text

	return res, nil
}

func (s *svc) ImageDescribe(ctx context.Context, req ImageDescribeRequest) (*ImageDescribeResult, error) {
	c, err := imageAuthorize(ctx, "ImageDescribe")
	if err != nil {
		return nil, err
	}

	row, doc, lerr := s.loadImage(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	findings := coreimage.Lint(doc, coreimage.LintEnv{})

	return &ImageDescribeResult{
		UntrustedNotice: UntrustedNotice, ImageID: row.ID, Revision: row.Revision, Name: doc.Name,
		Width: doc.Width, Height: doc.Height, Background: doc.Background, Template: doc.Template, Layers: doc.Layers,
		Findings: findings, Lint: totals(findings), Text: coreimage.Describe(doc, row.Revision, findings),
	}, nil
}

func totals(fs []lint.Finding) LintTotals {
	errs, warns := lint.Totals(fs)

	return LintTotals{Errors: errs, Warnings: warns}
}

// newFindings are the findings the batch introduced (at most ten) and the totals after it.
func newFindings(before, after []lint.Finding) []lint.Finding {
	var out []lint.Finding

	for _, f := range after {
		if !slices.ContainsFunc(before, func(b lint.Finding) bool { return b.Code == f.Code && b.Item == f.Item }) {
			out = append(out, f)
		}
	}

	if len(out) > maxFindings {
		out = out[:maxFindings]
	}

	if out == nil {
		out = []lint.Finding{}
	}

	return out
}

func (s *svc) ImageEdit(ctx context.Context, req ImageEditRequest) (*ImageEditResult, error) {
	c, err := imageAuthorize(ctx, "ImageEdit")
	if err != nil {
		return nil, err
	}

	hash, cerr := s.checkImageBatch(c, &req)
	if cerr != nil {
		return nil, cerr
	}

	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	var res, replayed *ImageEditResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		row, doc, lerr := s.loadImage(ctx, tx.Images(), c, req.ImageID)
		if lerr != nil {
			return lerr
		}

		if stored, rerr := s.replayImage(ctx, tx, c.Actor, &req, hash); rerr != nil || stored != nil {
			replayed = stored
			if rerr == nil {
				rerr = errReplay
			}

			return rerr
		}

		if err := s.checkImageBase(ctx, tx, c.Actor, &req, row.Revision, doc); err != nil {
			return err
		}

		out, aerr := coreimage.ApplyBatch(doc, req.Operations, s.imageEnv(ctx, tx, c, s.newIDs))
		if aerr != nil {
			return fromCommand(aerr)
		}

		res = s.imageEditResult(c.Actor, &req, row.Revision, doc, &out)

		switch {
		case out.NoOp():
			res.Applied, res.DryRun = false, req.DryRun

			return nil
		case req.DryRun:
			res.Revision, res.Applied, res.DryRun = row.Revision+1, false, true

			return nil
		}

		rev, err := s.commitImage(ctx, tx, &row, &imageBatch{
			kind: OpEdit, actor: c.Actor, base: req.BaseRevision, ops: req.Operations, doc: out.Doc,
			inverse: out.Inverse, diff: out.Diff, key: req.IdempotencyKey, hash: hash, closeRedo: true,
			result: func(rev int64) json.RawMessage {
				stored := *res
				stored.Revision = rev
				raw := marshalOr(&stored)

				return raw
			},
		})
		if err != nil {
			return err
		}

		res.Revision, res.Applied = rev, true

		return nil
	})

	switch {
	case errors.Is(err, errReplay):
		replayed.Replayed = true

		return replayed, nil
	case err != nil:
		return nil, asError(err)
	}

	return res, nil
}

func (s *svc) checkImageBatch(c Caller, req *ImageEditRequest) (string, error) {
	switch n := len(req.Operations); {
	case n == 0:
		return "", newError(command.CodeValidation, "the batch has no operations.").
			withPath("operations").withHint("send 1 to %d operations", maxOperationsPerBatch)
	case n > maxOperationsPerBatch:
		return "", newError(
			CodePayloadTooLarge,
			"the batch has %d operations; the limit is %d.",
			n,
			maxOperationsPerBatch,
		).
			withHint("split it into batches of at most %d operations", maxOperationsPerBatch).
			withLimit("batch_operations", n, maxOperationsPerBatch)
	}

	size := 0
	for _, op := range req.Operations {
		size += len(op.Op) + len(op.Args)
	}

	if size > maxBatchBytes {
		return "", newError(CodePayloadTooLarge, "the batch is %d bytes; the limit is %d.", size, maxBatchBytes).
			withHint("split it into smaller batches").withLimit("batch_bytes", size, maxBatchBytes)
	}

	if req.BaseRevision < 0 {
		return "", newError(command.CodeValidation, "base_revision is %d; it cannot be negative.", req.BaseRevision).
			withPath("base_revision")
	}

	if req.IdempotencyKey != "" && !idempotencyKeyOK(req.IdempotencyKey) {
		return "", newError(command.CodeValidation,
			"the idempotency key must have 8 to 64 characters from A-Z a-z 0-9 . _ ~ -.").withPath("idempotency_key")
	}

	reg := coreimage.Registry()

	for i, op := range req.Operations {
		cmd, ok := reg.Get(op.Op)
		if !ok {
			idx := i
			e := fromCommand(command.AsError(reg.Unknown(op.Op)))
			e.OpIndex = &idx

			return "", e
		}

		if need := string(cmd.Scope); !slices.Contains(c.Scopes, need) {
			idx := i
			ferr := newError(CodeForbidden, "the caller lacks the %q scope that %s needs.", need, op.Op).
				withHint("use a key with the %s scope", need).withPath(fmt.Sprintf("operations[%d].op", i))
			ferr.OpIndex = &idx

			return "", ferr
		}
	}

	hash, err := requestHash(req.Operations)
	if err != nil {
		return "", newError(
			command.CodeValidation,
			"the arguments of an operation are not valid JSON.",
		).withPath("operations")
	}

	return hash, nil
}

func (s *svc) replayImage(
	ctx context.Context,
	tx Tx,
	actor string,
	req *ImageEditRequest,
	hash string,
) (*ImageEditResult, error) {
	if req.IdempotencyKey == "" {
		return nil, nil
	}

	stored, err := tx.Images().ByKey(ctx, req.ImageID, actor, req.IdempotencyKey)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if stored.RequestHash != hash {
		return nil, newError(
			command.CodeIdempotencyMismatch,
			"the idempotency key was used with different operations.",
		).
			withHint("use a new key for a different batch").
			withPath("idempotency_key")
	}

	var res ImageEditResult
	if err := json.Unmarshal(stored.Result, &res); err != nil {
		return nil, internalError()
	}

	return &res, nil
}

// checkImageBase is the base revision against the current one, and the conflict check against what other
// actors did in between (SDD-002 section A.7).
func (s *svc) checkImageBase(
	ctx context.Context,
	tx Tx,
	actor string,
	req *ImageEditRequest,
	current int64,
	doc *coreimage.Document,
) error {
	switch {
	case req.BaseRevision > current:
		return newError(command.CodeValidation, "base_revision %d is ahead of the image, which is at revision %d.",
			req.BaseRevision, current).withPath("base_revision").
			withHint("read the image with image:show and use its revision")
	case req.BaseRevision == current:
		return nil
	case req.BaseRevision < current-maxConflictLag:
		return newError(command.CodeRevisionConflict, "base_revision %d is more than %d revisions behind revision %d.",
			req.BaseRevision, maxConflictLag, current).
			withHint("read the image again with image:show and plan the batch against its current revision").
			withDetail("base_revision", req.BaseRevision).withDetail("current_revision", current)
	}

	ops, err := tx.Images().OpsSince(ctx, req.ImageID, req.BaseRevision)
	if err != nil {
		return err
	}

	var intervening []command.Change

	var all []command.Change

	for i := range ops {
		d := decodeDiff(ops[i].Diff)

		all = append(all, d...)

		if ops[i].Actor != actor {
			intervening = append(intervening, d...)
		}
	}

	conflict := coreimage.Conflicts(req.Operations, intervening)
	if conflict == nil {
		return nil
	}

	return newError(
		command.CodeRevisionConflict,
		"the batch refers to %s %s, which another actor changed after revision %d.",
		conflict.Collection,
		conflict.ID,
		req.BaseRevision,
	).
		withHint("%s", hintConflict).
		withDetail("base_revision", req.BaseRevision).
		withDetail("current_revision", current).
		withDetail("conflict", map[string]any{"collection": conflict.Collection, "id": conflict.ID}).
		withDetail("diff", coreimage.DescribeDiff(doc, all))
}

func (s *svc) imageEditResult(
	actor string,
	req *ImageEditRequest,
	rev int64,
	before *coreimage.Document,
	out *coreimage.BatchResult,
) *ImageEditResult {
	effective := make([]OpEffect, len(req.Operations))
	for i, op := range req.Operations {
		effective[i] = OpEffect{Index: i, Op: op.Op, Effective: out.Effective[i]}
	}

	created := []string{}

	for _, ch := range out.Diff {
		if ch.Kind == coreimage.KindLayerAdded {
			created = append(created, ch.ID)
		}
	}

	lb, la := coreimage.Lint(before, coreimage.LintEnv{}), coreimage.Lint(out.Doc, coreimage.LintEnv{})
	t := totals(la)
	t.LintScope = LintScopeFast

	diff := out.Diff
	if diff == nil {
		diff = []command.Change{}
	}

	return &ImageEditResult{
		UntrustedNotice: UntrustedNotice, ImageID: req.ImageID, Revision: rev,
		BaseRevision: req.BaseRevision, Applied: true, Actor: actor, Created: created, Effective: effective, Diff: diff,
		Findings: newFindings(lb, la), Lint: t, Description: cutDescription(coreimage.DescribeDiff(out.Doc, out.Diff)),
	}
}

// cutDescription cuts a description at 40 lines and 4,000 characters, the rest counted (SDD-003 section 2.4 rule 6).
func cutDescription(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	size := 0

	for i, l := range lines {
		if len(out) == maxDescLines || size+len(l)+1 > maxDescription {
			out = append(out, "... and "+strconv.Itoa(len(lines)-i)+" more changes")

			break
		}

		out = append(out, l)
		size += len(l) + 1
	}

	return strings.Join(out, "\n")
}

// imageBatch is what the commit of an image writes.
type imageBatch struct {
	kind      OperationKind
	actor     string
	base      int64
	target    *int64
	ops       []command.Operation
	doc       *coreimage.Document
	inverse   []command.Primitive
	diff      []command.Change
	skipped   json.RawMessage
	key       string
	hash      string
	closeRedo bool
	closeStep bool
	result    func(rev int64) json.RawMessage
}

// commitImage writes a batch inside the open transaction and returns the new revision.
func (s *svc) commitImage(ctx context.Context, tx Tx, row *ImageRow, b *imageBatch) (int64, error) {
	rev := row.Revision + 1
	now := s.nowMs()
	u := imageUpdate(b.doc, now)

	ops := marshalOr(b.ops)
	inverse := marshalOr(nonNilPrimitives(b.inverse))
	diff := marshalOr(nonNilChanges(b.diff))

	op := ImageOpRow{
		ImageID: row.ID, Revision: rev, Kind: b.kind, Actor: b.actor, BaseRevision: b.base, TargetRevision: b.target,
		AtMs: now, Ops: ops, Inverse: inverse, Diff: diff, Skipped: b.skipped, DocSHA256: u.DocSHA256,
	}

	if b.key != "" {
		key := b.key
		op.IdempotencyKey, op.RequestHash = &key, b.hash

		if b.result != nil {
			op.Result = b.result(rev)
		}
	}

	if err := tx.Images().AppendOp(ctx, op); err != nil {
		return 0, err
	}

	if b.closeStep && b.target != nil {
		if err := tx.Images().Close(ctx, row.ID, *b.target, rev); err != nil {
			return 0, err
		}
	}

	if b.closeRedo {
		if err := tx.Images().CloseOpenRedo(ctx, row.ID, b.actor, rev); err != nil {
			return 0, err
		}
	}

	if err := tx.Images().Commit(ctx, row.ID, row.Revision, u); err != nil {
		return 0, err
	}

	return rev, nil
}

func (s *svc) ImageUndo(ctx context.Context, req ImageStepRequest) (*ImageStepResult, error) {
	return s.imageStep(ctx, "ImageUndo", req, false)
}

func (s *svc) ImageRedo(ctx context.Context, req ImageStepRequest) (*ImageStepResult, error) {
	return s.imageStep(ctx, "ImageRedo", req, true)
}

// imageStep is ImageUndo and ImageRedo: the transaction of an edit with the differences of SDD-005 section 12.2.
func (s *svc) imageStep(ctx context.Context, method string, req ImageStepRequest, redo bool) (*ImageStepResult, error) {
	c, err := imageAuthorize(ctx, method)
	if err != nil {
		return nil, err
	}

	var res *ImageStepResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		row, doc, lerr := s.loadImage(ctx, tx.Images(), c, req.ImageID)
		if lerr != nil {
			return lerr
		}

		kinds, code, word := []OperationKind{OpEdit, OpRedo}, CodeNothingToUndo, "undo"
		if redo {
			kinds, code, word = []OperationKind{OpUndo}, CodeNothingToRedo, "redo"
		}

		target, terr := tx.Images().LastOpen(ctx, row.ID, c.Actor, kinds)
		if errors.Is(terr, ErrNotFound) {
			return newError(code, "there is nothing for you to %s on image %s.", word, row.ID).
				withHint("an actor undoes only its own steps, newest first")
		}

		if terr != nil {
			return terr
		}

		var prims []command.Primitive
		if err := json.Unmarshal(target.Inverse, &prims); err != nil {
			return internalError()
		}

		later, oerr := tx.Images().OpsSince(ctx, row.ID, target.Revision)
		if oerr != nil {
			return oerr
		}

		touched := othersTouched(later, c.Actor)

		out, skipped := coreimage.ApplyPrimitives(doc, prims, coreimage.GuardFrom(touched))
		if skipped == nil {
			skipped = []command.Skipped{}
		}

		edit := ImageEditRequest{ImageID: row.ID, BaseRevision: row.Revision}
		batch := coreimage.BatchResult{Doc: out.Doc, Diff: out.Diff, Inverse: out.Inverse, Effective: nil}
		base := s.imageEditResult(c.Actor, &edit, row.Revision, doc, &batch)
		base.Effective = []OpEffect{}

		res = &ImageStepResult{ImageEditResult: *base, Kind: word, Step: target.Revision, Skipped: skipped}

		if batch.NoOp() || req.DryRun {
			res.Applied, res.DryRun = false, req.DryRun
			if req.DryRun && !batch.NoOp() {
				res.Revision = row.Revision + 1
			}

			return nil
		}

		skippedJSON := marshalOr(skipped)
		kind, step := OpUndo, target.Revision

		if redo {
			kind = OpRedo
		}

		rev, cerr := s.commitImage(ctx, tx, &row, &imageBatch{
			kind: kind, actor: c.Actor, base: row.Revision, target: &step, doc: out.Doc, inverse: out.Inverse,
			diff: out.Diff, skipped: skippedJSON, closeStep: true,
		})
		if cerr != nil {
			return cerr
		}

		res.Revision, res.Applied = rev, true

		return nil
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// marshalOr encodes a value of a log row; a value that cannot be encoded is stored as null.
func marshalOr(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}

	return raw
}

// othersTouched is what actors other than the caller committed, as the guard of an undo reads it.
func othersTouched(rows []ImageOpRow, actor string) []coreimage.Touched {
	var out []coreimage.Touched

	for i := range rows {
		if rows[i].Actor != actor {
			out = append(out, coreimage.Touched{
				Revision: rows[i].Revision, Actor: rows[i].Actor, Diff: decodeDiff(rows[i].Diff),
			})
		}
	}

	return out
}
