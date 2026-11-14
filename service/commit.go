package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service/crashpoint"
)

const (
	idempotencyTTLMs = 24 * 60 * 60 * 1000
	maxFindings      = 10
	maxDescription   = 4000
	maxDescLines     = 40
	eventDescription = 500
)

// batch is what the commit transaction writes: the log row, the document and
// everything derived from them (SDD-005 section 9.2 steps 7 to 13).
type batch struct {
	kind           OperationKind
	actor          string
	baseRevision   int64
	targetRevision *int64 // undo and redo: the step consumed
	ops            json.RawMessage
	inverse        []command.Primitive
	diff           []command.Change
	skipped        json.RawMessage
	before, after  *timeline.Document
	key            string
	requestHash    string
	result         func(revision int64) ([]byte, error) // the result to store under the key, when there is one
	closeTarget    bool                                 // undo and redo close the step they consume
	closeOpenRedo  bool                                 // a new edit ends the actor's redo chain
}

// commit runs steps 7 to 14 inside the open transaction. It returns the new
// revision N = R + 1.
func (s *svc) commit(ctx context.Context, tx ProjectTx, b *batch) (int64, error) {
	row := tx.Project()
	newRev := row.Revision + 1
	now := s.nowMs()

	canon := timeline.Canonical(b.after)
	sha := sha256Hex(canon)

	inverse, err := json.Marshal(nonNilPrimitives(b.inverse))
	if err != nil {
		return 0, internalError()
	}

	diff, err := json.Marshal(nonNilChanges(b.diff))
	if err != nil {
		return 0, internalError()
	}

	op := OperationRow{
		ProjectID: row.ID, Revision: newRev, Kind: b.kind, Actor: b.actor, BaseRevision: b.baseRevision,
		TargetRevision: b.targetRevision, AtMs: now, Ops: b.ops, Inverse: inverse, Diff: diff, Skipped: b.skipped,
		SchemaVersion: b.after.SchemaVersion, DocSHA256: sha,
	}

	if b.key != "" {
		key := b.key
		op.IdempotencyKey = &key
	}

	if err := tx.Operations().Append(ctx, op); err != nil {
		return 0, err
	}

	crashpoint.Hit("commit.after_operation_append")

	if b.closeTarget && b.targetRevision != nil {
		if err := tx.Operations().Close(ctx, row.ID, *b.targetRevision, newRev); err != nil {
			return 0, err
		}
	}

	if b.closeOpenRedo {
		if err := tx.Operations().CloseOpenRedo(ctx, row.ID, b.actor, newRev); err != nil {
			return 0, err
		}
	}

	update := ProjectUpdate{
		Name: b.after.Name, SchemaVersion: b.after.SchemaVersion, Document: canon, DocSHA256: sha,
		Summary: summarize(b.after), UpdatedMs: now,
	}
	if err := tx.Projects().Commit(ctx, row.ID, row.Revision, update); err != nil {
		return 0, err // ErrStale here is a bug in the lock discipline: asError makes it E_INTERNAL
	}

	crashpoint.Hit("commit.after_project_commit")

	if err := s.syncAssets(ctx, tx, row.ID, b, now); err != nil {
		return 0, err
	}

	crashpoint.Hit("commit.after_assets_sync")

	if b.key != "" && b.result != nil {
		stored, err := b.result(newRev)
		if err != nil {
			return 0, internalError()
		}

		err = tx.Idempotency().Put(ctx, IdempotencyRow{
			ProjectID: row.ID, Actor: b.actor, Key: b.key, RequestHash: b.requestHash, Revision: newRev,
			Result: stored, CreatedMs: now, ExpiresMs: now + idempotencyTTLMs,
		})
		if err != nil {
			return 0, err
		}

		crashpoint.Hit("commit.after_idempotency_put")

		if _, err := tx.Idempotency().PurgeExpired(ctx, now); err != nil {
			return 0, err
		}
	}

	return newRev, nil
}

func nonNilPrimitives(p []command.Primitive) []command.Primitive {
	if p == nil {
		return []command.Primitive{}
	}

	return p
}

func nonNilChanges(c []command.Change) []command.Change {
	if c == nil {
		return []command.Change{}
	}

	return c
}

// syncAssets keeps project_assets in step with the diff of the batch: an asset
// that appears in the document is active, one that leaves it is trashed with
// its object kept for a restore (SDD-005 section 5.5). Undo and redo need no
// special case because their diffs say the same.
func (s *svc) syncAssets(ctx context.Context, tx ProjectTx, projectID string, b *batch, now int64) error {
	for _, ch := range b.diff {
		switch ch.Kind {
		case command.KindAssetAdded:
			if err := s.assetAdded(ctx, tx, projectID, ch, b.actor, now); err != nil {
				return err
			}
		case command.KindAssetRemoved:
			snap, _ := json.Marshal(ch.Object)
			if err := tx.Assets().SetProjectAssetState(ctx, projectID, ch.ID, ProjectAssetTrashed, b.actor, now, snap); err != nil && !isNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (s *svc) assetAdded(ctx context.Context, tx ProjectTx, projectID string, ch command.Change, actor string, now int64) error {
	snap, _ := json.Marshal(ch.Object)

	err := tx.Assets().SetProjectAssetState(ctx, projectID, ch.ID, ProjectAssetActive, actor, now, snap)
	if err == nil || !isNotFound(err) {
		return err
	}

	hash, _ := ch.Object["hash"].(string)
	name, _ := ch.Object["name"].(string)

	return tx.Assets().PutProjectAsset(ctx, ProjectAssetRow{
		ProjectID: projectID, AssetID: ch.ID, AssetHash: hash, State: ProjectAssetActive, Name: name,
		Snapshot: snap, ImportedBy: actor, ImportedMs: now,
	})
}

func isNotFound(err error) bool { return err == ErrNotFound }

// findings runs the fast rules on both documents and returns the findings the
// batch introduced (at most ten) with the totals after it (SDD-003 section 2.4
// rule 2).
func (s *svc) findings(before, after *timeline.Document) ([]lint.Finding, LintTotals) {
	b := lint.Lint(before, s.lintEnv, lint.Fast)
	a := lint.Lint(after, s.lintEnv, lint.Fast)
	errs, warns := lint.Totals(a)

	added := lint.New(b, a)
	if len(added) > maxFindings {
		added = added[:maxFindings]
	}

	if added == nil {
		added = []lint.Finding{}
	}

	return added, LintTotals{Errors: errs, Warnings: warns}
}

// describe is DescribeDiff cut at 40 lines and 4,000 characters, the rest
// counted (SDD-003 section 2.4 rule 6).
func describe(doc *timeline.Document, diff []command.Change) string {
	lines := strings.Split(command.DescribeDiff(doc, diff), "\n")
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

// createdIDs lists the objects a diff added, in diff order (net of the batch).
func createdIDs(diff []command.Change) []string {
	out := []string{}

	for _, ch := range diff {
		if strings.HasSuffix(ch.Kind, ".added") && !strings.HasPrefix(ch.Kind, "font.") {
			out = append(out, ch.ID)
		}
	}

	return out
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n])
}
