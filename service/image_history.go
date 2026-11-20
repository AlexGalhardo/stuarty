package service

import (
	"context"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// imageRevertOp is the name the log gives the step that the revert of an image writes; it is not a command of the
// image registry (SDD-021 section 4).
const imageRevertOp = "image.revert"

// ImageRevert takes an image back to the document it had at a revision, as one new revision of the caller
// (SDD-021 section 4, SDD-005 section 12.4). The log is never rewritten.
func (s *svc) ImageRevert(ctx context.Context, req ImageRevertRequest) (*ImageRevertResult, error) {
	c, err := imageAuthorize(ctx, "ImageRevert")
	if err != nil {
		return nil, err
	}

	if (req.Revision != 0) == (req.Checkpoint != "") {
		return nil, newError(command.CodeValidation, "name one revision to go back to: revision or checkpoint.").
			withPath("revision").withHint("send revision, or checkpoint with the name of a checkpoint")
	}

	if req.BaseRevision < 0 {
		return nil, newError(command.CodeValidation, "base_revision is %d; it cannot be negative.", req.BaseRevision).
			withPath("base_revision")
	}

	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	var res *ImageRevertResult

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		row, doc, lerr := s.loadImage(ctx, tx.Images(), c, req.ImageID)
		if lerr != nil {
			return lerr
		}

		target, terr := revertTarget(ctx, tx.ImageCheckpoints(), imageNoun, row.ID, req.Revision, req.Checkpoint)
		if terr != nil {
			return terr
		}

		want, rerr := s.imageAt(ctx, tx.Images(), row.ID, doc, row.Revision, target, "revision")
		if rerr != nil {
			return rerr
		}

		out := coreimage.Revert(doc, want)

		base, berr := s.checkImageRevertBase(ctx, tx, c.Actor, &req, row.Revision, doc, &out)
		if berr != nil {
			return berr
		}

		res = s.imageRevertResult(c.Actor, row.ID, row.Revision, base, target, doc, &out)

		switch {
		case out.NoOp():
			res.Applied, res.DryRun, res.Same = false, req.DryRun, true
			res.Text = imageRevertText(res)

			return nil
		case req.DryRun:
			res.Revision, res.Applied, res.DryRun = row.Revision+1, false, true
			res.Text = imageRevertText(res)

			return nil
		}

		ops := []command.Operation{{Op: imageRevertOp, Args: marshalOr(map[string]int64{"revision": target})}}

		rev, err := s.commitImage(ctx, tx, &row, &imageBatch{
			kind: OpEdit, actor: c.Actor, base: base, ops: ops, doc: out.Doc, inverse: out.Inverse, diff: out.Diff,
			closeRedo: true, checkpoint: true,
		})
		if err != nil {
			return err
		}

		res.Revision, res.Applied = rev, true
		res.Text = imageRevertText(res)

		return nil
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// ImageDiff returns the difference between two revisions of an image (SDD-003 section 2.5.1). It compares the
// two documents rebuilt from the log, never the log itself, so it is right across undo and redo.
func (s *svc) ImageDiff(ctx context.Context, req ImageDiffRequest) (*ImageDiffResult, error) {
	c, err := imageAuthorize(ctx, "ImageDiff")
	if err != nil {
		return nil, err
	}

	if (req.From != 0) == (req.FromCheckpoint != "") {
		return nil, newError(command.CodeValidation, "name where the diff starts: from or from_checkpoint.").
			withPath("from").withHint("send from with a revision, or from_checkpoint with the name of a checkpoint")
	}

	row, cur, lerr := s.loadImage(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	from, ferr := revertTarget(ctx, s.store.ImageCheckpoints(), imageNoun, row.ID, req.From, req.FromCheckpoint)
	if ferr != nil {
		return nil, ferr
	}

	to := req.Until
	if to == 0 {
		to = row.Revision
	}

	a, aerr := s.imageAt(ctx, s.store.Images(), row.ID, cur, row.Revision, from, "from")
	if aerr != nil {
		return nil, aerr
	}

	b, berr := s.imageAt(ctx, s.store.Images(), row.ID, cur, row.Revision, to, "until")
	if berr != nil {
		return nil, berr
	}

	entries := coreimage.DiffEntries(a, b)
	lines := make([]string, len(entries))

	for i := range entries {
		lines[i] = entries[i].Line
	}

	return &ImageDiffResult{
		UntrustedNotice: UntrustedNotice, ImageID: row.ID, Revision: row.Revision, From: from,
		To: to, Count: len(entries), Entries: append([]DiffEntry{}, entries...),
		Text: textview.ImageDiff(row.ID, from, to, lines),
	}, nil
}

// imageRevertResult is the result of a revert before it is written: the diff it makes, the findings it brings
// and the target.
func (s *svc) imageRevertResult(
	actor, id string,
	current, base, target int64,
	before *coreimage.Document,
	out *coreimage.BatchResult,
) *ImageRevertResult {
	edit := ImageEditRequest{ImageID: id, BaseRevision: base}
	res := s.imageEditResult(actor, &edit, current, before, out)
	res.Effective = []OpEffect{}

	return &ImageRevertResult{ImageEditResult: *res, Kind: "revert", Target: target}
}

// imageRevertText is the text of a revert of an image (SDD-003 section 3.7.1): the state and the target, what
// changed, and the lint line; or the one line that says nothing was written.
func imageRevertText(r *ImageRevertResult) string {
	if r.Same {
		return fmt.Sprintf("reverted nothing: the image already equals r%d", r.Target)
	}

	state := "reverted"
	if r.DryRun {
		state = "dry run, would revert as"
	}

	text := fmt.Sprintf("%s r%d (base r%d) by %s: changes=%d, to r%d", state, r.Revision, r.BaseRevision, r.Actor,
		len(r.Diff), r.Target)

	if r.Description != "" {
		text += "\n" + r.Description
	}

	text += fmt.Sprintf("\nlint errors=%d warnings=%d", r.Lint.Errors, r.Lint.Warnings)

	return withNote(text)
}

// checkImageRevertBase is the conflict check of a revert (SDD-002 A.7): with a base revision older than the
// current one, a revert conflicts when another actor changed, since then, a layer that the revert changes. It
// returns the base revision to record.
func (s *svc) checkImageRevertBase(
	ctx context.Context,
	tx Tx,
	actor string,
	req *ImageRevertRequest,
	current int64,
	doc *coreimage.Document,
	out *coreimage.BatchResult,
) (int64, error) {
	base := req.BaseRevision
	if base == 0 {
		return current, nil
	}

	asEdit := ImageEditRequest{ImageID: req.ImageID, BaseRevision: base, Operations: imageRevertScope(out.Diff)}

	return base, s.checkImageBase(ctx, tx, actor, &asEdit, current, doc)
}

// imageRevertScope names, as the operations of an edit, what a revert changes: the layers it touches, and the
// canvas when the document differs, so that the conflict check of an edit applies as it is.
func imageRevertScope(diff []command.Change) []command.Operation {
	ids := make([]string, 0, len(diff))
	ops := []command.Operation{}

	for _, ch := range diff {
		switch ch.Kind {
		case coreimage.KindDocumentUpdated:
			ops = append(ops, command.Operation{Op: "canvas.resize"})
		case coreimage.KindLayerAdded, coreimage.KindLayerUpdated, coreimage.KindLayerRemoved:
			ids = append(ids, ch.ID)
		}
	}

	return append(ops, command.Operation{Op: imageRevertOp, Args: marshalOr(map[string][]string{"ids": ids})})
}

// imageAt is the document of an image at a revision, rebuilt from the log by walking back from the current one
// (SDD-005 section 12.4). path names the request member that held the revision. The rows are read once, from the
// target on.
func (s *svc) imageAt(
	ctx context.Context,
	repo ImageRepo,
	id string,
	current *coreimage.Document,
	currentRev, target int64,
	path string,
) (*coreimage.Document, error) {
	var rows []ImageOpRow

	log := &revisionLog[*coreimage.Document]{
		noun: imageNoun, idKey: "image_id", id: id, current: current, revision: currentRev,
		canonical: coreimage.Canonical, rebuild: coreimage.Rebuild,
		row: func(ctx context.Context, rev int64) (logRow, error) {
			if rows == nil {
				got, err := repo.OpsSince(ctx, id, target-1)
				if err != nil {
					return logRow{}, err
				}

				rows = got
			}

			if i := rev - target; i < 0 || i >= int64(len(rows)) || rows[i].Revision != rev {
				return logRow{}, internalError().withDetail("image_id", id).withDetail("revision", rev)
			}

			r := &rows[rev-target]

			return logRow{Kind: r.Kind, Inverse: r.Inverse, DocSHA256: r.DocSHA256}, nil
		},
		blocks: func(kind OperationKind) string {
			if kind == OpEdit || kind == OpUndo || kind == OpRedo {
				return ""
			}

			return "a step of kind " + string(kind)
		},
	}

	return rebuildAt(ctx, log, target, path)
}

// imageCheckpointScope is what every checkpoint operation of an image starts with: the caller scope, and the
// revision of the image it may see.
func (s *svc) imageCheckpointScope(ctx context.Context, method, id string) (Caller, int64, error) {
	c, err := imageAuthorize(ctx, method)
	if err != nil {
		return Caller{}, 0, err
	}

	row, err := s.store.Images().Get(ctx, id)
	if err != nil || !visible(c, row.OwnerUserID) {
		return Caller{}, 0, imageNotFound(id)
	}

	return c, row.Revision, nil
}

// ImageCheckpointCreate puts a name and a note on a revision of an image (SDD-003 section 2.5.1).
func (s *svc) ImageCheckpointCreate(ctx context.Context, req ImageCheckpointCreateRequest) (*Checkpoint, error) {
	c, head, err := s.imageCheckpointScope(ctx, "ImageCheckpointCreate", req.ImageID)
	if err != nil {
		return nil, err
	}

	asTimeline := CheckpointCreateRequest{Name: req.Name, Note: req.Note, Revision: req.Revision}

	return s.createCheckpoint(ctx, s.store.ImageCheckpoints(), imageNoun, req.ImageID, c.Actor, head, &asTimeline)
}

// ImageCheckpointList lists the checkpoints of an image, newest revision first.
func (s *svc) ImageCheckpointList(
	ctx context.Context,
	req ImageCheckpointListRequest,
) (*ImageCheckpointListResult, error) {
	_, head, err := s.imageCheckpointScope(ctx, "ImageCheckpointList", req.ImageID)
	if err != nil {
		return nil, err
	}

	cps, text, err := s.listCheckpoints(ctx, s.store.ImageCheckpoints(), req.ImageID, head)
	if err != nil {
		return nil, err
	}

	return &ImageCheckpointListResult{
		UntrustedNotice: UntrustedNotice, ImageID: req.ImageID, Revision: head,
		Checkpoints: cps, Text: text,
	}, nil
}

// ImageCheckpointRename changes the name of a checkpoint of an image, and its note when given.
func (s *svc) ImageCheckpointRename(ctx context.Context, req ImageCheckpointRenameRequest) (*Checkpoint, error) {
	if _, _, err := s.imageCheckpointScope(ctx, "ImageCheckpointRename", req.ImageID); err != nil {
		return nil, err
	}

	return s.renameCheckpoint(ctx, s.store.ImageCheckpoints(), imageNoun, req.ImageID, req.Name, req.NewName, req.Note)
}

// ImageCheckpointRemove removes a checkpoint of an image. No revision is removed.
func (s *svc) ImageCheckpointRemove(ctx context.Context, req ImageCheckpointRemoveRequest) (*Checkpoint, error) {
	if _, _, err := s.imageCheckpointScope(ctx, "ImageCheckpointRemove", req.ImageID); err != nil {
		return nil, err
	}

	return s.removeCheckpoint(ctx, s.store.ImageCheckpoints(), imageNoun, req.ImageID, req.Name)
}
