package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

const (
	maxBatchBytes     = 512 << 10
	maxConflictLag    = 1000
	maxConflictText   = 4000
	hintRestore       = "restore the project first"
	hintReadAgain     = "read the project again with project_get and plan the batch against its current revision"
	errReplayedMarker = "replayed"
	hintConflict      = "details.diff lists what changed since your revision; re-plan the batch against current_revision and send it again"
)

// errReplay rolls the transaction back after an idempotent replay found its
// stored result: nothing is written (SDD-005 section 9.2 step 3).
var errReplay = errors.New(errReplayedMarker)

func (s *svc) Edit(ctx context.Context, req EditRequest) (*EditResult, error) {
	return s.edit(ctx, req, false)
}

// EditInternal is Edit for the commands only the service itself issues, such
// as the asset.add batch that closes an ingest (SDD-005 section 5.5). The scope
// of the command is still checked against the caller; only the refusal of an
// internal command is lifted. It returns an error for a Service that was not
// made by New. Test-only: product code never calls it;
// TestSecurity_T05_EditInternalOnlyFromTests fails the build when it does.
func EditInternal(ctx context.Context, s Service, req EditRequest) (*EditResult, error) {
	impl, ok := s.(*svc)
	if !ok {
		return nil, internalError()
	}

	return impl.edit(ctx, req, true)
}

func (s *svc) edit(ctx context.Context, req EditRequest, allowInternal bool) (*EditResult, error) {
	c, err := s.authorizeProject(ctx, "Edit", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	hash, cerr := s.checkBatch(c, &req, allowInternal)
	if cerr != nil {
		return nil, cerr
	}

	// Reads still work on a full disk; a write is refused (SDD-007 section 9.2).
	if derr := s.checkDisk(); derr != nil && !req.DryRun {
		return nil, forCaller(c, derr)
	}

	var (
		res      *EditResult
		replayed *EditResult
	)

	err = s.store.WriteProject(ctx, req.ProjectID, func(ctx context.Context, tx ProjectTx) error {
		crashpoint.Hit("commit.after_lock")

		row := tx.Project()
		if !visible(c, row.OwnerUserID) {
			return ErrNotFound
		}

		if row.State == ProjectTrashed {
			return newError(
				command.CodeInvalidState,
				"the project %s is in the trash.",
				row.ID,
			).withHint("%s", hintRestore)
		}

		if stored, err := s.replay(ctx, tx, c.Actor, &req, hash); err != nil || stored != nil {
			replayed = stored
			if err == nil {
				err = errReplay
			}

			return err
		}

		doc, derr := decodeRow(&row)
		if derr != nil {
			return derr
		}

		crashpoint.Hit("commit.after_load")

		if err := s.checkBase(ctx, tx, c.Actor, &req, row.Revision, doc); err != nil {
			return err
		}

		var ids idRecorder

		ids.gen = s.newIDs

		// The analysis the commands need is read here and stored with the
		// operation, so a replay of the log gives the same document.
		resolvedOps, rerr := s.resolveOps(ctx, tx, doc, req.Operations)
		if rerr != nil {
			return rerr
		}

		req.Operations = resolvedOps

		graph := newProductionGraph(ctx, tx, row.ID)

		out, aerr := s.applyChecked(graph, doc, req.Operations, &ids)
		if aerr != nil {
			return aerr
		}

		crashpoint.Hit("commit.after_apply")

		if verr := checkFollowedName(ctx, tx, &row, out.Doc.Name); verr != nil {
			return verr
		}

		res = s.editResult(c.Actor, &req, row.Revision, doc, &out, &ids)
		if out.NoOp() {
			// Nothing changed: no revision, no undo step, no event and no stored key (VV-050).
			res.Applied, res.DryRun = false, req.DryRun

			return nil
		}

		if req.DryRun {
			// The revision a dry run shows is the one the batch would get (SDD-003 section 3.5).
			res.Revision, res.Applied, res.DryRun = row.Revision+1, false, true
			return nil
		}

		b := &batch{
			kind:          OpEdit,
			actor:         c.Actor,
			baseRevision:  req.BaseRevision,
			ops:           storedOps(req.Operations, out.Effective, &ids),
			inverse:       out.Inverse,
			diff:          out.Diff,
			before:        doc,
			after:         out.Doc,
			key:           req.IdempotencyKey,
			requestHash:   hash,
			closeOpenRedo: true,
			spawned:       out.Spawned,
			checkpoint:    true,
			result: func(rev int64) ([]byte, error) {
				stored := *res
				stored.Revision = rev
				return json.Marshal(&stored)
			},
		}

		rev, err := s.commit(ctx, tx, b)
		if err != nil {
			return err
		}

		res.Revision, res.Applied = rev, true

		crashpoint.Hit("commit.before_commit")

		ev := Event{
			Type: "batch.committed", ProjectID: row.ID, Revision: rev, Kind: string(OpEdit), Actor: c.Actor,
			Description: cutRunes(res.Description, eventDescription), At: s.now().UTC(),
		}

		s.publishAfterCommit(tx, graph, &ev)

		return nil
	})

	switch {
	case errors.Is(err, errReplay):
		replayed.Replayed = true
		return replayed, nil
	case err != nil:
		return nil, s.assetStillIngesting(ctx, req.ProjectID, projectErr(req.ProjectID, err))
	}

	return res, nil
}

// assetStillIngesting turns E_NOT_FOUND for an asset id into E_ASSET_NOT_READY when the project
// has imported that asset and its ingest has not finished: the asset joins the document only when
// it is ready, so the core cannot tell it from a wrong id (VV-022).
func (s *svc) assetStillIngesting(ctx context.Context, projectID string, err error) error {
	se, ok := errors.AsType[*Error](err)
	if !ok || se.Code != command.CodeNotFound || se.Details["kind"] != "asset" {
		return err
	}

	id, _ := se.Details["id"].(string)

	pa, gerr := s.store.Assets().GetProjectAsset(ctx, projectID, id)
	if gerr != nil {
		return err
	}

	ar, gerr := s.store.Assets().Get(ctx, pa.AssetHash)
	if gerr != nil || ar.IngestState == IngestReady || ar.IngestState == IngestFailed {
		return err
	}

	progress := math.Min(math.Max(ar.Progress, 0), 1)

	out := newError(
		"E_ASSET_NOT_READY",
		"the asset %s is %s (%d%% done); it cannot be used until its ingest ends.",
		id,
		ar.IngestState,
		int(math.Round(progress*100)),
	).
		withHint("call media_inspect with asset_id %s until its state is ready, then repeat the edit", id).
		withDetail("asset_id", id).
		withDetail("state", string(ar.IngestState)).
		withDetail("progress", progress)
	out.OpIndex, out.Path = se.OpIndex, se.Path

	return out
}

// projectErr maps a missing project to E_NOT_FOUND and everything else as asError does.
func projectErr(id string, err error) error {
	if errors.Is(err, ErrNotFound) {
		return projectNotFound(id, err)
	}

	return asError(err)
}

// checkBatch is step 0 of SDD-005 section 9.2: the shape checks, outside the
// lock and the transaction, and the scope of every command. It returns the
// request hash.
func (s *svc) checkBatch(c Caller, req *EditRequest, allowInternal bool) (string, error) {
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
		return "", newError(
			command.CodeValidation,
			"base_revision is %d; it cannot be negative.",
			req.BaseRevision,
		).withPath("base_revision")
	}

	if req.IdempotencyKey != "" && !idempotencyKeyOK(req.IdempotencyKey) {
		return "", newError(
			command.CodeValidation,
			"the idempotency key must have 8 to 64 characters from A-Z a-z 0-9 . _ ~ -.",
		).
			withPath("idempotency_key")
	}

	for i, op := range req.Operations {
		cmd, ok := s.reg.Get(op.Op)
		if !ok || cmd.Internal && !allowInternal {
			return "", s.unknownOp(i)
		}

		if !allowInternal && analysisOps[op.Op] && callerSuppliedResolved(op.Args) {
			idx := i

			rerr := newError(
				command.CodeValidation,
				"%s does not take a resolved argument: the service fills it from the analysis of the media.",
				op.Op,
			).
				withHint("remove resolved from the arguments").
				withPath(fmt.Sprintf("operations[%d].args.resolved", i))
			rerr.OpIndex = &idx

			return "", rerr
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

// unknownOp is E_UNKNOWN_COMMAND for a name outside the registry and for an
// internal command, listing the public names (SDD-003 section 2.4 rule 5).
func (s *svc) unknownOp(i int) *Error {
	var known []string

	for _, c := range s.reg.List() {
		if !c.Internal {
			known = append(known, c.Name)
		}
	}

	idx := i

	e := newError(command.CodeUnknownCommand, "operation %d names a command that does not exist.", i).
		withHint("use one of the known commands: %s", strings.Join(known, ", ")).
		withPath(fmt.Sprintf("operations[%d].op", i)).withDetail("known", known)
	e.OpIndex = &idx

	return e
}

// replay is step 3: the stored result of the same key and the same operations.
func (s *svc) replay(
	ctx context.Context,
	tx ProjectTx,
	actor string,
	req *EditRequest,
	hash string,
) (*EditResult, error) {
	if req.IdempotencyKey == "" {
		return nil, nil
	}

	stored, err := tx.Idempotency().Get(ctx, req.ProjectID, actor, req.IdempotencyKey, s.nowMs())
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

	var res EditResult
	if err := json.Unmarshal(stored.Result, &res); err != nil {
		return nil, internalError()
	}

	return &res, nil
}

// checkBase is step 4: base against the current revision, and the conflict
// check against what other actors did in between (SDD-002 A.7).
func (s *svc) checkBase(
	ctx context.Context,
	tx ProjectTx,
	actor string,
	req *EditRequest,
	current int64,
	doc *timeline.Document,
) error {
	switch {
	case req.BaseRevision > current:
		return newError(
			command.CodeValidation,
			"base_revision %d is ahead of the project, which is at revision %d.",
			req.BaseRevision,
			current,
		).
			withPath("base_revision").
			withHint("read the project with project_get and use its revision")
	case req.BaseRevision == current:
		return nil
	case req.BaseRevision < current-maxConflictLag:
		return newError(
			command.CodeRevisionConflict,
			"base_revision %d is more than %d revisions behind revision %d.",
			req.BaseRevision,
			maxConflictLag,
			current,
		).
			withHint("%s", hintReadAgain).
			withDetail("base_revision", req.BaseRevision).
			withDetail("current_revision", current)
	}

	others, err := tx.Operations().Since(ctx, req.ProjectID, req.BaseRevision, OperationFilter{ExcludeActor: actor})
	if err != nil {
		return err
	}

	var intervening []command.Change

	for i := range others {
		intervening = append(intervening, decodeDiff(others[i].Diff)...)
	}

	conflict := command.Conflicts(req.Operations, intervening)
	if conflict == nil {
		return nil
	}

	all, err := tx.Operations().Since(ctx, req.ProjectID, req.BaseRevision, OperationFilter{})
	if err != nil {
		return err
	}

	text, truncated := conflictText(doc, all)
	by := conflictAuthor(others, conflict)

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
		withDetail("conflict", map[string]any{"collection": conflict.Collection, "id": conflict.ID, "revision": by.Revision, "actor": by.Actor}).
		withDetail("diff", text).
		withDetail("diff_truncated", truncated)
}

func decodeDiff(raw json.RawMessage) []command.Change {
	var out []command.Change
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}

	return out
}

// conflictAuthor is the first batch of another actor that touched the object.
func conflictAuthor(rows []OperationRow, c *command.ConflictInfo) command.Touch {
	kind, _, _ := strings.Cut(c.Kind, ".")

	for i := range rows {
		for _, ch := range decodeDiff(rows[i].Diff) {
			if ch.ID == c.ID && strings.HasPrefix(ch.Kind, kind+".") {
				return command.Touch{Revision: rows[i].Revision, Actor: rows[i].Actor}
			}
		}
	}

	return command.Touch{}
}

// conflictText is the text of every batch in (base, current] from all actors:
// a header "r<revision> <actor>" and the DescribeDiff lines of each, at most
// 4,000 characters, ending with "... N more changes" when cut (SDD-003
// section 2.4 rule 3).
func conflictText(doc *timeline.Document, rows []OperationRow) (text string, truncated bool) {
	type item struct {
		line   string
		change bool
	}

	var items []item

	for i := range rows {
		items = append(items, item{fmt.Sprintf("r%d %s", rows[i].Revision, rows[i].Actor), false})

		for l := range strings.SplitSeq(command.DescribeDiff(doc, decodeDiff(rows[i].Diff)), "\n") {
			items = append(items, item{l, true})
		}
	}

	var out []string

	size := 0

	for i, it := range items {
		if size+len(it.line)+1 > maxConflictText {
			more := 0

			for _, rest := range items[i:] {
				if rest.change {
					more++
				}
			}

			return strings.Join(append(out, fmt.Sprintf("... %d more changes", more)), "\n"), true
		}

		out = append(out, it.line)
		size += len(it.line) + 1
	}

	return strings.Join(out, "\n"), false
}

// idRecorder records the identifiers the commands asked for, so that the
// stored operation carries them (SDD-002 A.3 rule 4).
type idRecorder struct {
	gen func(string) string
	ids []string
}

func (r *idRecorder) next(prefix string) string {
	id := r.gen(prefix)
	r.ids = append(r.ids, id)

	return id
}

// storedOps writes the identifiers the service generated into the arguments
// of the operations that used them: "new_id" or "new_ids" when the operation
// reports them under that name, "id" otherwise.
func storedOps(ops []command.Operation, effective []map[string]any, rec *idRecorder) json.RawMessage {
	out := make([]command.Operation, len(ops))

	for i, op := range ops {
		out[i] = command.Operation{Op: op.Op, Args: fillIDs(op.Args, effective[i], rec.ids)}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage("[]")
	}

	return b
}

func fillIDs(args json.RawMessage, effective map[string]any, generated []string) json.RawMessage {
	tree := map[string]any{}

	if len(args) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(args)))
		dec.UseNumber()

		if dec.Decode(&tree) != nil {
			return args
		}
	}

	if effectiveIDs, ok := effective["new_ids"].([]string); ok {
		if _, has := tree["new_ids"]; !has && len(effectiveIDs) > 0 {
			tree["new_ids"] = effectiveIDs
		}
	}

	for _, g := range generated {
		switch {
		case effective["new_id"] == g:
			setIfMissing(tree, "new_id", g)
		case valueIs(effective, g):
			setIfMissing(tree, "id", g)
		}
	}

	b, err := json.Marshal(tree)
	if err != nil {
		return args
	}

	return b
}

func setIfMissing(tree map[string]any, key string, v any) {
	if _, has := tree[key]; !has {
		tree[key] = v
	}
}

func valueIs(m map[string]any, s string) bool {
	for _, v := range m {
		if v == s {
			return true
		}
	}

	return false
}

// editResult builds the result of a batch (steps 6 and 16): created ids,
// effective values per operation, the net diff, the text and the new lint
// findings. Revision and Applied are set by the caller.
func (s *svc) editResult(
	actor string,
	req *EditRequest,
	current int64,
	before *timeline.Document,
	out *command.BatchResult,
	_ *idRecorder,
) *EditResult {
	created := createdIDs(out.Diff)
	effective := make([]OpEffect, len(req.Operations))

	for i, op := range req.Operations {
		eff := out.Effective[i]
		if eff == nil {
			eff = map[string]any{}
		}

		effective[i] = OpEffect{Index: i, Op: op.Op, Effective: eff, Created: createdBy(eff, created)}
	}

	created = inOperationOrder(created, effective)

	findings, totals := s.findings(before, out.Doc)

	return &EditResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       req.ProjectID, Revision: current, BaseRevision: req.BaseRevision, Actor: actor,
		Created: created, Effective: effective, Diff: nonNilChanges(out.Diff),
		Findings: findings, Lint: totals, Description: describe(out.Doc, out.Diff),
	}
}

// inOperationOrder orders the created ids of a batch as its operations created
// them (SDD-003): the diff lists them by id, which is random, so a caller that
// reads created[0] would get a different object on each run. An id that no
// operation reports keeps its place after the others.
func inOperationOrder(created []string, effective []OpEffect) []string {
	out := make([]string, 0, len(created))
	seen := make(map[string]bool, len(created))

	for _, e := range effective {
		for _, id := range e.Created {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}

	for _, id := range created {
		if !seen[id] {
			out = append(out, id)
		}
	}

	return out
}

// createdBy lists the created ids that an operation reports in its effective
// values: the identifier it used, whether the caller gave it or not.
func createdBy(effective map[string]any, created []string) []string {
	var out []string

	for _, k := range slices.Sorted(maps.Keys(effective)) {
		switch v := effective[k].(type) {
		case string:
			if slices.Contains(created, v) && !slices.Contains(out, v) {
				out = append(out, v)
			}
		case []string:
			for _, id := range v {
				if slices.Contains(created, id) && !slices.Contains(out, id) {
					out = append(out, id)
				}
			}
		}
	}

	return out
}

// analysisOps are the commands whose `resolved` argument only the service may fill (SDD-002 section A.9).
var analysisOps = map[string]bool{
	"clip.remove_silences":     true,
	"caption.from_transcript":  true,
	"caption.retime_from_clip": true,
	"audio.duck":               true,
	"audio.mark_beats":         true,
}

// callerSuppliedResolved reports whether the arguments carry the internal `resolved` member.
func callerSuppliedResolved(args json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return false
	}

	_, ok := m["resolved"]

	return ok
}
