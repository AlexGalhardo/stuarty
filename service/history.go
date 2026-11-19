package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 100
	defaultLintLimit    = 25
	maxLintLimit        = 100
	historyDescription  = 300
)

func (s *svc) HistoryList(ctx context.Context, req HistoryListRequest) (*HistoryListResult, error) {
	c, err := s.authorizeProject(ctx, "HistoryList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	head, err := s.store.Projects().Head(ctx, req.ProjectID)
	if err != nil || !visible(c, head.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	before, cerr := decodeRevisionCursor(req.Cursor, req.ProjectID)
	if cerr != nil {
		return nil, cerr
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	limit := clampLimit(req.Limit, defaultHistoryLimit, maxHistoryLimit)

	// "cli" stands for "local:cli": a filter with no kind prefix is a local actor,
	// not an empty answer.
	if req.Actor != "" && !strings.Contains(req.Actor, ":") {
		req.Actor = "local:" + req.Actor
	}

	rows, more, err := s.store.Operations().
		Page(ctx, req.ProjectID, HistoryQuery{Before: before, Actor: req.Actor, Limit: limit})
	if err != nil {
		return nil, asError(err)
	}

	total, err := s.store.Operations().Count(ctx, req.ProjectID, req.Actor)
	if err != nil {
		return nil, asError(err)
	}

	entries := make([]HistoryEntry, 0, len(rows))
	lines := make([]textview.HistoryLine, 0, len(rows))

	undo, redo := s.stepCandidates(ctx, req.ProjectID, c.Actor)

	for i := range rows {
		e, l := s.historyEntry(&rows[i])
		e.UndoableByMe = undo == rows[i].Revision
		e.RedoableByMe = redo == rows[i].Revision
		entries, lines = append(entries, e), append(lines, l)
	}

	// The text has a character cap: a page that does not fit ends early and
	// the cursor points past its last step.
	if n := textview.FitHistory(lines); n < len(rows) {
		entries, lines, more = entries[:n], lines[:n], true
	}

	res := &HistoryListResult{ProjectID: req.ProjectID, Revision: head.Revision, Entries: entries}
	res.Text = textview.History(req.ProjectID, head.Revision, lines, total)

	if more && len(entries) > 0 {
		next := encodeRevisionCursor(req.ProjectID, entries[len(entries)-1].Revision)
		res.NextCursor = &next
		res.Text += "\nnext_cursor: " + next
	}

	return res, nil
}

func (s *svc) historyEntry(row *OperationRow) (HistoryEntry, textview.HistoryLine) {
	ops := opNames(row.Ops)
	e := HistoryEntry{
		Revision: row.Revision, Kind: string(row.Kind), Actor: row.Actor, At: msTime(row.AtMs),
		Ops: ops, Changes: len(decodeDiff(row.Diff)), Description: s.stepDescription(row, ops),
	}

	if row.Kind == OpUndo {
		e.UndoOf = row.TargetRevision
	}

	if (row.Kind == OpEdit || row.Kind == OpRedo) && row.ClosedBy != nil && *row.ClosedBy != row.Revision {
		e.UndoneBy = row.ClosedBy
	}

	return e, textview.HistoryLine{
		Revision: row.Revision,
		Kind:     string(row.Kind),
		At:       e.At,
		Actor:    row.Actor,
		Ops:      ops,
		UndoOf:   e.UndoOf,
		UndoneBy: e.UndoneBy,
	}
}

func opNames(raw json.RawMessage) []string {
	var ops []struct {
		Op string `json:"op"`
	}

	_ = json.Unmarshal(raw, &ops)

	names := make([]string, len(ops))
	for i := range ops {
		names[i] = ops[i].Op
	}

	return names
}

type revisionCursor struct {
	V int    `json:"v"`
	P string `json:"p"`
	K int64  `json:"k"`
}

func encodeRevisionCursor(project string, rev int64) string {
	b := marshalOrNull(revisionCursor{V: 1, P: project, K: rev})
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeRevisionCursor reads a history cursor; the key is a revision, which
// never changes meaning, so it never goes stale (SDD-003 section 1.5 rule 3).
func decodeRevisionCursor(s, project string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(s)

	var c revisionCursor
	if err != nil || json.Unmarshal(raw, &c) != nil || c.V != 1 || c.P != project || c.K < 1 {
		return 0, newError(
			command.CodeValidation,
			"the cursor is not valid.",
		).withPath("cursor").
			withHint("repeat the call without cursor")
	}

	return c.K, nil
}

func (s *svc) Undo(ctx context.Context, req UndoRequest) (*UndoResult, error) {
	return s.step(ctx, "Undo", req, false)
}

func (s *svc) Redo(ctx context.Context, req UndoRequest) (*UndoResult, error) {
	return s.step(ctx, "Redo", req, true)
}

// step is Undo and Redo: the same transaction as an edit with the differences
// of SDD-005 section 12.2.
func (s *svc) step(ctx context.Context, method string, req UndoRequest, redo bool) (*UndoResult, error) {
	c, err := s.authorizeProject(ctx, method, req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	var res *UndoResult

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

		cand, err := candidate(ctx, tx, row.ID, c.Actor, redo)
		if err != nil {
			return err
		}

		doc, derr := decodeRow(&row)
		if derr != nil {
			return derr
		}

		crashpoint.Hit("commit.after_load")

		out, skipped, err := s.applyStep(ctx, tx, doc, &cand, c.Actor)
		if err != nil {
			return err
		}

		crashpoint.Hit("commit.after_apply")

		res = s.undoResult(c.Actor, &row, &cand, doc, &out, skipped, redo)
		if req.DryRun {
			res.Applied, res.DryRun = false, true
			return nil
		}

		skippedJSON := marshalOrNull(skippedOps(skipped))
		kind, target := OpUndo, cand.Revision

		if redo {
			kind = OpRedo
		}

		b := &batch{
			kind: kind, actor: c.Actor, baseRevision: row.Revision, targetRevision: &target, ops: json.RawMessage("[]"),
			inverse: out.Inverse, diff: out.Diff, skipped: skippedJSON, before: doc, after: out.Doc, closeTarget: true,
		}

		rev, err := s.commit(ctx, tx, b)
		if err != nil {
			return err
		}

		res.Revision = rev

		crashpoint.Hit("commit.before_commit")

		ev := Event{
			Type: "batch.committed", ProjectID: row.ID, Revision: rev, Kind: string(kind), Actor: c.Actor,
			Description: cutRunes(res.Description, eventDescription), At: s.now().UTC(),
		}
		tx.AfterCommit(func() {
			crashpoint.Hit("commit.after_commit")
			s.emit(ev)
			crashpoint.Hit("commit.after_publish")
		})

		return nil
	})
	if err != nil {
		return nil, projectErr(req.ProjectID, err)
	}

	return res, nil
}

// candidate is the step to undo, or for a redo the undo to redo; none is
// E_NOTHING_TO_UNDO or E_NOTHING_TO_REDO without details.
func candidate(ctx context.Context, tx ProjectTx, project, actor string, redo bool) (OperationRow, error) {
	var (
		row OperationRow
		err error
	)

	if redo {
		row, err = tx.Operations().RedoCandidate(ctx, project, actor)
	} else {
		row, err = tx.Operations().UndoCandidate(ctx, project, actor)
	}

	if errors.Is(err, ErrNotFound) {
		if redo {
			return row, newError(CodeNothingToRedo, "the actor has no step to redo.").withHint("undo a step first")
		}

		return row, newError(CodeNothingToUndo, "the actor has no step to undo.").withHint("make an edit first")
	}

	return row, err
}

// applyStep reverses the stored inverse of the candidate and applies it with
// the guard built from what other actors did since (SDD-005 section 12.2).
func (s *svc) applyStep(
	ctx context.Context,
	tx ProjectTx,
	doc *timeline.Document,
	cand *OperationRow,
	actor string,
) (command.Result, []command.Skipped, error) {
	var prims []command.Primitive
	if err := json.Unmarshal(cand.Inverse, &prims); err != nil {
		return command.Result{}, nil, internalError()
	}

	slices.Reverse(prims)

	others, err := tx.Operations().Since(ctx, cand.ProjectID, cand.Revision, OperationFilter{ExcludeActor: actor})
	if err != nil {
		return command.Result{}, nil, err
	}

	out, skipped := command.ApplyPrimitives(doc, prims, newGuard(others))

	return out, skipped, nil
}

// guard answers whether another actor changed what a primitive touches, from
// the diffs of their batches (SDD-005 section 12.2 step 3).
type guard struct {
	whole  map[string]command.Touch            // "collection id" added or removed as a whole
	fields map[string]map[string]command.Touch // "collection id" -> field -> who updated it
}

func newGuard(rows []OperationRow) *guard {
	g := &guard{whole: map[string]command.Touch{}, fields: map[string]map[string]command.Touch{}}

	for i := range rows {
		by := command.Touch{Revision: rows[i].Revision, Actor: rows[i].Actor}

		for _, ch := range decodeDiff(rows[i].Diff) {
			coll, kind, _ := cutKind(ch.Kind)
			if coll == "" {
				continue
			}

			key := coll + " " + ch.ID

			switch kind {
			case "added", "removed":
				if _, seen := g.whole[key]; !seen {
					g.whole[key] = by
				}
			case "updated":
				if g.fields[key] == nil {
					g.fields[key] = map[string]command.Touch{}
				}

				for field := range ch.Changes {
					if _, seen := g.fields[key][field]; !seen {
						g.fields[key][field] = by
					}
				}
			}
		}
	}

	return g
}

// cutKind splits "item.updated" into the collection ("items") and the verb.
func cutKind(kind string) (collection, verb string, ok bool) {
	for i := range len(kind) {
		if kind[i] == '.' {
			noun := kind[:i]

			switch noun {
			case "asset", "track", "item", "font":
				return noun + "s", kind[i+1:], true
			case "project":
				return "project", kind[i+1:], true
			case "settings":
				return "settings", kind[i+1:], true
			}

			return "", "", false
		}
	}

	return "", "", false
}

// Changed implements command.Guard. The field "*" asks about the whole object
// and "" about any change to it.
func (g *guard) Changed(collection, id, field string) (command.Touch, bool) {
	key := collection + " " + id

	switch field {
	case "*":
		t, ok := g.whole[key]
		return t, ok
	case "":
		if t, ok := g.whole[key]; ok {
			return t, true
		}

		for _, t := range g.fields[key] {
			return t, true
		}

		return command.Touch{}, false
	}

	t, ok := g.fields[key][field]

	return t, ok
}

func skippedOps(in []command.Skipped) []SkippedOp {
	out := make([]SkippedOp, 0, len(in))

	for i := range in {
		prim := marshalOrNull(in[i].Primitive)
		out = append(out, SkippedOp{
			PrimitiveIndex: in[i].PrimitiveIndex, Primitive: prim, Fields: in[i].Fields, Reason: in[i].Reason,
			ByRevision: in[i].ByRevision, ByActor: in[i].ByActor, Details: in[i].Details,
		})
	}

	return out
}

func (s *svc) undoResult(
	actor string,
	row *ProjectRow,
	cand *OperationRow,
	before *timeline.Document,
	out *command.Result,
	skipped []command.Skipped,
	redo bool,
) *UndoResult {
	findings, totals := s.findings(before, out.Doc)
	ref := &HistoryRef{Revision: cand.Revision, Kind: string(cand.Kind), Actor: cand.Actor, At: msTime(cand.AtMs)}

	r := &UndoResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       row.ID,
		Revision:        row.Revision,
		BaseRevision:    row.Revision,
		Applied:         len(out.Diff) > 0,
		Actor:           actor,
		Created:         createdIDs(out.Diff),
		Effective:       []OpEffect{},
		Diff:            nonNilChanges(out.Diff),
		Findings:        findings,
		Lint:            totals,
		Description:     describe(out.Doc, out.Diff),
		Kind:            "undo", Skipped: skippedOps(skipped),
	}

	if redo {
		r.Kind, r.Redone = "redo", ref
	} else {
		r.Undone = ref
	}

	return r
}

func (s *svc) Lint(ctx context.Context, req LintRequest) (*LintResult, error) {
	c, err := s.authorizeProject(ctx, "Lint", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if req.Severity != "" && req.Severity != "warning" && req.Severity != "error" {
		return nil, newError(command.CodeValidation, "the severity %q is not known.", req.Severity).
			withPath("severity").withHint("use warning or error")
	}

	row, err := s.store.Projects().Get(ctx, req.ProjectID)
	if err != nil || !visible(c, row.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	all := lint.Lint(doc, s.lintEnvFor(ctx, doc), lint.Full)
	errs, warns := lint.Totals(all)

	listed := all
	if req.Severity == "error" {
		listed = slices.DeleteFunc(
			slices.Clone(all),
			func(f lint.Finding) bool { return f.Severity != lint.SeverityError },
		)
	}

	from := 0

	if req.Cursor != "" {
		cur, cerr := decodeViewCursor(req.Cursor, row.ID, row.Revision)
		if cerr != nil {
			return nil, cerr
		}

		from, _ = strconv.Atoi(cur.K)
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	fps := timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}
	limit := clampLimit(req.Limit, defaultLintLimit, maxLintLimit)
	text, next := lint.Report(
		lint.Header{Revision: row.Revision, ProjectID: row.ID, FPS: fps, Limit: limit},
		listed,
		from,
	)

	end := len(listed)
	if next >= 0 {
		end = next
	}

	res := &LintResult{
		ProjectID: row.ID,
		Revision:  row.Revision,
		Errors:    errs,
		Warnings:  warns,
		Findings:  []lint.Finding{},
		Text:      text,
	}

	if from < len(listed) {
		res.Findings = listed[from:end]
	}

	if next >= 0 {
		cur := encodeViewCursor(viewCursor{V: 1, P: row.ID, R: row.Revision, K: strconv.Itoa(next), N: 2})
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

// stepCandidates are the revisions Undo and Redo would take for the actor now
// (0 for none): the only steps flagged undoable_by_me and redoable_by_me, by
// the per-actor rules of SDD-005 section 12.1.
func (s *svc) stepCandidates(ctx context.Context, project, actor string) (undo, redo int64) {
	if row, err := s.store.Operations().UndoCandidate(ctx, project, actor); err == nil {
		undo = row.Revision
	}

	if row, err := s.store.Operations().RedoCandidate(ctx, project, actor); err == nil {
		redo = row.Revision
	}

	return undo, redo
}

// stepDescription is one line for the history panel: the summaries of the
// distinct commands of an edit, or what an undo, a redo, a migration or an
// import did. Never media text (SDD-012 section 15).
func (s *svc) stepDescription(row *OperationRow, ops []string) string {
	switch row.Kind {
	case OpUndo:
		return "Undid r" + strconv.FormatInt(derefInt64(row.TargetRevision), 10) + "."
	case OpRedo:
		return "Redid r" + strconv.FormatInt(derefInt64(row.TargetRevision), 10) + "."
	case OpMigrate:
		return "Migrated the project to the current schema."
	case OpImport:
		return "Imported the project."
	case OpEdit:
	}

	if len(ops) == 1 && ops[0] == revertOp {
		return "Reverted to r" + strconv.FormatInt(revertTargetOf(row.Ops), 10) + "."
	}

	var parts []string

	for _, name := range ops {
		cmd, ok := s.reg.Get(name)
		if !ok || slices.Contains(parts, cmd.Summary) {
			continue
		}

		parts = append(parts, cmd.Summary)
	}

	return cutRunes(strings.Join(parts, " "), historyDescription)
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}

	return *p
}
