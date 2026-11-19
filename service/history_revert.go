package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// maxRebuildSteps is how many revisions back a document is rebuilt from the log (SDD-005 section 12.4).
const maxRebuildSteps = 5000

// revertOp is the name the log gives the step that a revert writes; it is not a command of any registry.
const revertOp = "history.revert"

// HistoryRevert takes the timeline back to the document it had at a revision, as one new revision of the
// caller (SDD-003 section 2.5.1, SDD-005 section 12.4). The log is never rewritten.
func (s *svc) HistoryRevert(ctx context.Context, req HistoryRevertRequest) (*HistoryRevertResult, error) {
	c, err := s.authorizeProject(ctx, "HistoryRevert", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if (req.Revision != 0) == (req.Checkpoint != "") {
		return nil, newError(command.CodeValidation, "name one revision to go back to: revision or checkpoint.").
			withPath("revision").withHint("send revision, or checkpoint with the name of a checkpoint")
	}

	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	var res *HistoryRevertResult

	err = s.store.WriteProject(ctx, req.ProjectID, func(ctx context.Context, tx ProjectTx) error {
		crashpoint.Hit("commit.after_lock")

		row := tx.Project()
		if !visible(c, row.OwnerUserID) {
			return ErrNotFound
		}

		if row.State == ProjectTrashed {
			return newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
				withHint("%s", hintRestore)
		}

		target, terr := revertTarget(ctx, tx.Checkpoints(), row.ID, req.Revision, req.Checkpoint)
		if terr != nil {
			return terr
		}

		doc, derr := decodeRow(&row)
		if derr != nil {
			return derr
		}

		crashpoint.Hit("commit.after_load")

		want, rerr := s.documentAt(ctx, tx.Operations(), row.ID, doc, row.Revision, target, "revision")
		if rerr != nil {
			return rerr
		}

		out := command.Revert(doc, want)

		base, berr := s.checkRevertBase(ctx, tx, c.Actor, &req, row.Revision, doc, &out)
		if berr != nil {
			return berr
		}

		crashpoint.Hit("commit.after_apply")

		res = s.revertResult(c.Actor, row.ID, row.Revision, base, target, doc, &out)
		if out.NoOp() {
			res.Applied, res.DryRun, res.Same = false, req.DryRun, true
			res.Text = revertText(res)

			return nil
		}

		// A revert can bring back a composition that now closes a loop or nests too deep (SDD-004 §3.16).
		graph := newProductionGraph(ctx, tx, row.ID)
		if nerr := checkNesting(graph, out.Doc); nerr != nil {
			return nerr
		}

		if req.DryRun {
			res.Revision, res.Applied, res.DryRun = row.Revision+1, false, true
			res.Text = revertText(res)

			return nil
		}

		ops := marshalOrNull([]command.Operation{{
			Op: revertOp, Args: marshalOrNull(map[string]int64{"revision": target}),
		}})
		b := &batch{
			kind: OpEdit, actor: c.Actor, baseRevision: base, ops: ops, inverse: out.Inverse, diff: out.Diff,
			before: doc, after: out.Doc, closeOpenRedo: true, checkpoint: true,
		}

		rev, err := s.commit(ctx, tx, b)
		if err != nil {
			return err
		}

		res.Revision, res.Applied = rev, true
		res.Text = revertText(res)

		crashpoint.Hit("commit.before_commit")

		ev := Event{
			Type: "batch.committed", ProjectID: row.ID, Revision: rev, Kind: string(OpEdit), Actor: c.Actor,
			Description: cutRunes(res.Description, eventDescription), At: s.now().UTC(),
		}
		s.publishAfterCommit(tx, graph, &ev)

		return nil
	})
	if err != nil {
		return nil, projectErr(req.ProjectID, err)
	}

	return res, nil
}

// revertTargetOf reads the revision a stored revert went back to from the operations of its log row.
func revertTargetOf(raw json.RawMessage) int64 {
	var ops []struct {
		Args struct {
			Revision int64 `json:"revision"`
		} `json:"args"`
	}

	if json.Unmarshal(raw, &ops) != nil || len(ops) == 0 {
		return 0
	}

	return ops[0].Args.Revision
}

// revertTarget is the revision a revert goes back to: the one asked for, or the one a checkpoint marks.
func revertTarget(
	ctx context.Context,
	repo CheckpointRepo,
	project string,
	revision int64,
	name string,
) (int64, error) {
	if name == "" {
		return revision, nil
	}

	cp, err := repo.Get(ctx, project, name)
	if errors.Is(err, ErrNotFound) {
		return 0, checkpointNotFound(name)
	}

	return cp.Revision, err
}

func (s *svc) revertResult(
	actor, project string,
	current, base, target int64,
	before *timeline.Document,
	out *command.BatchResult,
) *HistoryRevertResult {
	findings, totals := s.findings(before, out.Doc)

	return &HistoryRevertResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       project, Revision: current, BaseRevision: base, Actor: actor,
		Created: createdIDs(out.Diff), Effective: []OpEffect{}, Diff: nonNilChanges(out.Diff),
		Findings: findings, Lint: totals, Description: revertDescription(target, out.Doc, out.Diff),
		Kind: "revert", Target: target,
	}
}

func revertDescription(target int64, doc *timeline.Document, diff []command.Change) string {
	head := "Reverted to r" + strconv.FormatInt(target, 10) + "."
	if len(diff) == 0 {
		return head
	}

	return head + "\n" + describe(doc, diff)
}

// revertText is the text of a revert (SDD-003 section 3.7.1): the edit text with the state reverted, or the
// one line that says nothing was written.
func revertText(r *HistoryRevertResult) string {
	if r.Same {
		return fmt.Sprintf("reverted nothing: the document already equals r%d", r.Target)
	}

	state := "reverted"
	if r.DryRun {
		state = "dry run, would revert as"
	}

	return renderEdit(&r.EditResult, state, ", to r"+strconv.FormatInt(r.Target, 10), nil)
}

// checkRevertBase is the conflict check of a revert (SDD-002 A.7): with a base revision older than the current
// one, a revert conflicts when another actor changed, since then, an object that the revert changes. It returns
// the base revision to record.
func (s *svc) checkRevertBase(
	ctx context.Context,
	tx ProjectTx,
	actor string,
	req *HistoryRevertRequest,
	current int64,
	doc *timeline.Document,
	out *command.BatchResult,
) (int64, error) {
	base := req.BaseRevision
	if base == 0 {
		return current, nil
	}

	asEdit := EditRequest{ProjectID: req.ProjectID, BaseRevision: base, Operations: revertScope(out.Diff)}

	return base, s.checkBase(ctx, tx, actor, &asEdit, current, doc)
}

// revertScope names, as the operations of an edit, what a revert changes: the identifiers of its objects, and
// the project or the settings when they differ, so that the conflict check of an edit applies as it is.
func revertScope(diff []command.Change) []command.Operation {
	ids := make([]string, 0, len(diff))
	ops := []command.Operation{}

	for _, ch := range diff {
		switch ch.Kind {
		case command.KindProjectUpdated:
			ops = append(ops, command.Operation{Op: "project.rename"})
		case command.KindSettingsUpdated:
			ops = append(ops, command.Operation{Op: "settings.update"})
		default:
			ids = append(ids, ch.ID)
		}
	}

	return append(ops, command.Operation{Op: revertOp, Args: marshalOrNull(map[string][]string{"ids": ids})})
}

// HistoryDiff returns the difference between two revisions (SDD-003 section 2.5.1).
func (s *svc) HistoryDiff(ctx context.Context, req HistoryDiffRequest) (*HistoryDiffResult, error) {
	c, err := s.authorizeProject(ctx, "HistoryDiff", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if (req.From != 0) == (req.FromCheckpoint != "") {
		return nil, newError(command.CodeValidation, "name where the diff starts: from or from_checkpoint.").
			withPath("from").withHint("send from with a revision, or from_checkpoint with the name of a checkpoint")
	}

	row, err := s.store.Projects().Get(ctx, req.ProjectID)
	if err != nil || !visible(c, row.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	from := req.From

	if req.FromCheckpoint != "" {
		cp, cerr := s.store.Checkpoints().Get(ctx, row.ID, req.FromCheckpoint)
		if errors.Is(cerr, ErrNotFound) {
			return nil, checkpointNotFound(req.FromCheckpoint)
		}

		if cerr != nil {
			return nil, asError(cerr)
		}

		from = cp.Revision
	}

	to := req.Until
	if to == 0 {
		to = row.Revision
	}

	cur, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	repo := s.store.Operations()

	a, aerr := s.documentAt(ctx, repo, row.ID, cur, row.Revision, from, "from")
	if aerr != nil {
		return nil, aerr
	}

	b, berr := s.documentAt(ctx, repo, row.ID, cur, row.Revision, to, "to")
	if berr != nil {
		return nil, berr
	}

	entries := command.DiffEntries(a, b)
	lines := make([]string, len(entries))

	for i := range entries {
		lines[i] = entries[i].Line
	}

	return &HistoryDiffResult{
		UntrustedNotice: UntrustedNotice, ProjectID: row.ID, Revision: row.Revision, From: from, To: to,
		Count: len(entries), Entries: append([]DiffEntry{}, entries...), Text: textview.Diff(row.ID, from, to, lines),
	}, nil
}

// documentAt is the document of a timeline at a revision, rebuilt from the log by walking back from the current
// one (SDD-005 section 12.4). path names the request member that held the revision.
func (s *svc) documentAt(
	ctx context.Context,
	repo OperationRepo,
	project string,
	current *timeline.Document,
	currentRev, target int64,
	path string,
) (*timeline.Document, error) {
	if target < 1 || target > currentRev {
		return nil, newError(command.CodeValidation, "revision %d does not exist; the timeline is at revision %d.",
			target, currentRev).withPath(path).withHint("use a revision from 1 to %d", currentRev)
	}

	if target == currentRev {
		return current, nil
	}

	if currentRev-target > maxRebuildSteps {
		return nil, newError(command.CodeLimit, "revision %d is more than %d revisions behind revision %d.",
			target, maxRebuildSteps, currentRev).withPath(path).
			withHint("go back in steps, through a checkpoint nearer to the current revision")
	}

	inverses := make([][]command.Primitive, 0, currentRev-target)

	for rev := currentRev; rev > target; rev-- {
		row, err := repo.Get(ctx, project, rev)
		if err != nil {
			return nil, err
		}

		if row.Kind == OpMigrate || row.Kind == OpImport {
			return nil, newError(command.CodeInvalidState,
				"revision %d is before the migration or import at revision %d, which the log cannot go back across.",
				target, rev).withPath(path).withHint("use a revision from %d on", rev)
		}

		var prims []command.Primitive
		if err := json.Unmarshal(row.Inverse, &prims); err != nil {
			return nil, internalError()
		}

		inverses = append(inverses, prims)
	}

	doc, err := command.RebuildAt(current, inverses)
	if err != nil {
		return nil, s.rebuildFailed(project, currentRev, target, err.Error())
	}

	want, err := repo.Get(ctx, project, target)
	if err != nil {
		return nil, err
	}

	if got := sha256Hex(timeline.Canonical(doc)); got != want.DocSHA256 {
		return nil, s.rebuildFailed(project, currentRev, target, "the rebuilt document does not match the log")
	}

	return doc, nil
}

// rebuildFailed is the answer when the log and the document disagree, which is a defect and never something to
// continue from: E_INTERNAL, with the revisions so that the report can be traced.
func (s *svc) rebuildFailed(project string, current, target int64, why string) error {
	_ = why

	return internalError().withDetail("project_id", project).withDetail("revision", target).
		withDetail("current_revision", current)
}
