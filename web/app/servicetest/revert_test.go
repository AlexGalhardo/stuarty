package servicetest

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// revertEnv is a project with a track and four texts, one per revision r2 to r5, and the document of
// each revision, so that a revert can be compared with the document it must give back.
type revertEnv struct {
	*env
	project, track string
	docs           map[int64][]byte
}

func newRevertEnv(t *testing.T) *revertEnv {
	t.Helper()

	r := &revertEnv{env: newEnv(t), docs: map[int64][]byte{}}
	r.project, r.track = r.withTrack()
	r.docs[1] = r.document(r.project)

	for i, text := range []string{"one", "two", "three", "four"} {
		res, err := r.addText(ui, r.project, r.track, int64(i+1), fmt.Sprintf("%ds", i*2), text)
		if err != nil {
			t.Fatal(err)
		}

		r.docs[res.Revision] = r.document(r.project)
	}

	return r
}

func (r *revertEnv) revert(actor string, req service.HistoryRevertRequest) (*service.HistoryRevertResult, error) {
	r.t.Helper()

	req.ProjectID = r.project

	return r.svc.HistoryRevert(r.as(actor), req)
}

// Criterion 1: the revert gives a document equal to the revision, as a new revision that undo and redo walk.
func TestRevertThroughTheService(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t) // r1..r5

	res, err := r.revert(ui, service.HistoryRevertRequest{Revision: 3})
	if err != nil {
		t.Fatal(err)
	}

	if !res.Applied || res.Revision != 6 || res.BaseRevision != 5 || res.Target != 3 || res.Kind != "revert" ||
		res.Same {
		t.Fatalf("result = %+v", res.EditResult)
	}

	if !bytes.Equal(r.document(r.project), r.docs[3]) {
		t.Fatal("the document after the revert is not the document of r3")
	}

	// The first line is the note about quoted text, the second the state, with the target at its end.
	first := strings.Split(res.Text, "\n")[1]
	if len(res.Diff) != 2 || !strings.HasPrefix(first, "reverted r6 (base r5) by local:ui: changes=2") ||
		!strings.HasSuffix(first, ", to r3") {
		t.Errorf("diff %d, text:\n%s", len(res.Diff), res.Text)
	}

	// The log kept everything: r1 to r6, the revert is an edit of the actor with its own inverse.
	if r.operationCount(r.project) != 6 {
		t.Fatalf("the log has %d rows, want 6", r.operationCount(r.project))
	}

	hist, err := r.svc.HistoryList(r.as(ui), service.HistoryListRequest{ProjectID: r.project, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}

	top := hist.Entries[0]
	if top.Revision != 6 || top.Kind != "edit" || len(top.Ops) != 1 || top.Ops[0] != "history.revert" ||
		top.Description != "Reverted to r3." || !top.UndoableByMe {
		t.Errorf("history entry = %+v", top)
	}

	// Undo of the revert gives the document before it; redo gives the reverted one again.
	if u, err := r.undo(ui, r.project); err != nil || u.Revision != 7 || u.Undone.Revision != 6 {
		t.Fatalf("undo = %+v, %v", u, err)
	}

	if !bytes.Equal(r.document(r.project), r.docs[5]) {
		t.Fatal("revert then undo does not give the document before the revert")
	}

	if _, err := r.redo(r.project); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(r.document(r.project), r.docs[3]) {
		t.Fatal("redo of the revert does not give the reverted document")
	}

	// The revert to every revision of a head that moved on, in any order.
	for _, rev := range []int64{1, 5, 2, 4, 3} {
		if _, err := r.revert(ui, service.HistoryRevertRequest{Revision: rev}); err != nil {
			t.Fatalf("revert to r%d: %v", rev, err)
		}

		if !bytes.Equal(r.document(r.project), r.docs[rev]) {
			t.Fatalf("revert to r%d: the document differs from r%d", rev, rev)
		}
	}
}

// Criterion 3: a named checkpoint survives further edits and can be reverted to by name.
func TestRevertToACheckpointByName(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t)

	if _, err := r.svc.CheckpointCreate(r.as(ui), service.CheckpointCreateRequest{
		ProjectID: r.project, Name: "two texts", Revision: 3,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.addText(ui, r.project, r.track, 5, "20s", "later"); err != nil {
		t.Fatal(err)
	}

	res, err := r.revert(agent, service.HistoryRevertRequest{Checkpoint: "two texts"})
	if err != nil || res.Target != 3 || res.Actor != agent || !res.Applied {
		t.Fatalf("revert by name = %+v, %v", res, err)
	}

	if !bytes.Equal(r.document(r.project), r.docs[3]) {
		t.Fatal("the document is not the one the checkpoint names")
	}

	// The agent's first edit of the day (the revert) made its own automatic checkpoint at r6, and the named one is still there.
	names := map[string]bool{}
	for _, c := range r.checkpoints(r.project) {
		names[c.Name] = true
	}

	if !names["two texts"] || !names["auto "+agent+" 2026-11-14"] {
		t.Errorf("checkpoints = %v", names)
	}

	if _, err := r.revert(ui, service.HistoryRevertRequest{Checkpoint: "missing"}); code(err) != command.CodeNotFound {
		t.Errorf("an unknown checkpoint: %v", err)
	}
}

// Criterion 2: a dry run returns the diff it would cause and changes nothing; the current revision is a no-op.
func TestRevertDryRunAndNoOp(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t)
	before, rows := r.document(r.project), r.operationCount(r.project)

	res, err := r.revert(ui, service.HistoryRevertRequest{Revision: 2, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	if res.Applied || !res.DryRun || res.Revision != 6 || len(res.Diff) != 3 ||
		!strings.Contains(res.Text, "\ndry run, would revert as r6") {
		t.Fatalf("dry run = %+v\n%s", res.EditResult, res.Text)
	}

	if !bytes.Equal(r.document(r.project), before) || r.operationCount(r.project) != rows {
		t.Fatalf("a dry run wrote something: %d rows", r.operationCount(r.project))
	}

	if n := len(r.checkpoints(r.project)); n != 0 {
		t.Errorf("a dry run made %d checkpoints", n)
	}

	same, err := r.revert(agent, service.HistoryRevertRequest{Revision: 5})
	if err != nil || same.Applied || !same.Same || same.Revision != 5 || len(same.Diff) != 0 ||
		same.Text != "reverted nothing: the document already equals r5" {
		t.Fatalf("revert to the current revision = %+v, %v\n%s", same, err, same.Text)
	}

	if r.operationCount(r.project) != rows || len(r.checkpoints(r.project)) != 0 {
		t.Error("a no-op revert wrote a revision or a checkpoint")
	}

	// A revision whose document is the current one, reached again by an undo, is the same.
	if _, err := r.undo(ui, r.project); err != nil { // r6 undoes r5
		t.Fatal(err)
	}

	if res, err := r.revert(ui, service.HistoryRevertRequest{Revision: 4}); err != nil || !res.Same || res.Applied {
		t.Fatalf("revert to a revision equal to the current document = %+v, %v", res, err)
	}
}

func TestRevertRefusals(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t)

	for name, tc := range map[string]struct {
		req  service.HistoryRevertRequest
		want string
		path string
	}{
		"neither":    {service.HistoryRevertRequest{}, command.CodeValidation, "revision"},
		"both":       {service.HistoryRevertRequest{Revision: 2, Checkpoint: "x"}, command.CodeValidation, "revision"},
		"ahead":      {service.HistoryRevertRequest{Revision: 6}, command.CodeValidation, "revision"},
		"negative":   {service.HistoryRevertRequest{Revision: -1}, command.CodeValidation, "revision"},
		"base ahead": {service.HistoryRevertRequest{Revision: 2, BaseRevision: 9}, command.CodeValidation, "base_revision"},
	} {
		_, err := r.revert(ui, tc.req)
		if code(err) != tc.want {
			t.Errorf("%s: %v, want %s", name, err, tc.want)

			continue
		}

		if se := serviceError(t, err); se.Path != tc.path {
			t.Errorf("%s: path %q, want %q", name, se.Path, tc.path)
		}
	}

	readOnly := service.WithCaller(r.as(ui), service.Caller{Actor: "key:reader", Scopes: []string{service.ScopeRead}})
	if _, err := r.svc.HistoryRevert(
		readOnly,
		service.HistoryRevertRequest{ProjectID: r.project, Revision: 2},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("revert with the read scope: %v", err)
	}

	if _, err := r.svc.ProjectTrash(r.as(ui), service.ProjectTrashRequest{ProjectID: r.project}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.revert(ui, service.HistoryRevertRequest{Revision: 2}); code(err) != command.CodeInvalidState {
		t.Errorf("revert in the trash: %v", err)
	}
}

// RF-120 and ADR-0009: a revert is a batch with a base revision. The objects another actor changed since the
// base make it conflict, with the text of what they did; with no base it goes back over them on purpose.
func TestRevertConflictsLikeAnyBatch(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t) // r5

	if _, err := r.addText(agent, r.project, r.track, 5, "20s", "by the agent"); err != nil { // r6
		t.Fatal(err)
	}

	_, err := r.revert(ui, service.HistoryRevertRequest{Revision: 3, BaseRevision: 5})
	if code(err) != command.CodeRevisionConflict {
		t.Fatalf("revert over the agent's new item = %v", err)
	}

	se := serviceError(t, err)
	if se.Details["current_revision"] != int64(6) || !strings.Contains(fmt.Sprint(se.Details["diff"]), "by the agent") {
		t.Errorf("conflict details = %+v", se.Details)
	}

	// With no base the person goes back over the agent's work on purpose, and the agent's item goes with it.
	res, err := r.revert(ui, service.HistoryRevertRequest{Revision: 3})
	if err != nil || !res.Applied || res.BaseRevision != 6 || res.Revision != 7 {
		t.Fatalf("revert with no base = %+v, %v", res, err)
	}

	if !bytes.Equal(r.document(r.project), r.docs[3]) {
		t.Fatal("the document is not the one of r3")
	}

	// A base that is the current revision is no conflict either.
	if _, err := r.revert(ui, service.HistoryRevertRequest{Revision: 2, BaseRevision: 7}); err != nil {
		t.Fatalf("revert with the current revision as base: %v", err)
	}
}

// ADR-0006: the undo of a revert skips what another actor changed after it.
func TestRevertUndoRespectsOtherActors(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t) // r5: texts "one" to "four", the last at 6s

	if _, err := r.revert(ui, service.HistoryRevertRequest{Revision: 4}); err != nil { // r6 removes the 4th text
		t.Fatal(err)
	}

	// The agent then adds a text over the place the revert freed: the undo of the revert cannot put the old one back.
	if _, err := r.addText(agent, r.project, r.track, 6, "6s", "taken"); err != nil { // r7
		t.Fatal(err)
	}

	res, err := r.undo(ui, r.project)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Skipped) == 0 || res.Skipped[0].Reason != command.SkipInvalidResult {
		t.Fatalf("the undo of the revert should skip the item that no longer fits: %+v", res.Skipped)
	}
}

func TestDiffThroughTheService(t *testing.T) {
	t.Parallel()

	r := newRevertEnv(t) // r1..r5

	res, err := r.svc.HistoryDiff(r.as(agent), service.HistoryDiffRequest{ProjectID: r.project, From: 2})
	if err != nil {
		t.Fatal(err)
	}

	if res.From != 2 || res.To != 5 || res.Count != 3 || len(res.Entries) != 3 || res.UntrustedNotice == "" {
		t.Fatalf("diff = %+v", res)
	}

	for _, e := range res.Entries {
		if e.Verb != command.VerbAdded || e.Noun != "text" {
			t.Errorf("entry = %+v", e)
		}
	}

	if !strings.Contains(res.Text, "diff "+r.project+" r2 -> r5 (3 changes)\nadded item") ||
		!strings.Contains(res.Text, `"three"`) {
		t.Errorf("text:\n%s", res.Text)
	}

	// Between two earlier revisions, by checkpoint, and across an undo.
	if _, err := r.svc.CheckpointCreate(
		r.as(ui),
		service.CheckpointCreateRequest{ProjectID: r.project, Name: "mid", Revision: 3},
	); err != nil {
		t.Fatal(err)
	}

	byName, err := r.svc.HistoryDiff(
		r.as(ui),
		service.HistoryDiffRequest{ProjectID: r.project, FromCheckpoint: "mid", To: 4},
	)
	if err != nil || byName.From != 3 || byName.Count != 1 {
		t.Fatalf("diff by checkpoint = %+v, %v", byName, err)
	}

	backwards, err := r.svc.HistoryDiff(r.as(ui), service.HistoryDiffRequest{ProjectID: r.project, From: 4, To: 3})
	if err != nil || backwards.Count != 1 || backwards.Entries[0].Verb != command.VerbRemoved {
		t.Fatalf("diff backwards = %+v, %v", backwards, err)
	}

	if _, err := r.undo(ui, r.project); err != nil { // r6 undoes r5
		t.Fatal(err)
	}

	same, err := r.svc.HistoryDiff(r.as(ui), service.HistoryDiffRequest{ProjectID: r.project, From: 4})
	if err != nil || same.Count != 0 || !strings.HasSuffix(same.Text, "(no changes)") {
		t.Fatalf("diff of a revision and a document equal to it (after the undo) = %+v, %v", same, err)
	}

	for name, req := range map[string]service.HistoryDiffRequest{
		"neither":      {},
		"both":         {From: 2, FromCheckpoint: "mid"},
		"from ahead":   {From: 99},
		"to ahead":     {From: 1, To: 99},
		"from unknown": {FromCheckpoint: "nope"},
	} {
		req.ProjectID = r.project

		if _, err := r.svc.HistoryDiff(r.as(ui), req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// Criterion 4: the text diff of a large change lists what changed within the budget and counts what it cut.
func TestDiffThroughTheServiceBudget(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack() // r1
	base := int64(1)

	for batch := range 3 {
		ops := make([]command.Operation, 0, 100)

		for i := range 100 {
			n := batch*100 + i
			ops = append(
				ops,
				op(
					"text.add",
					fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"line %d"}`, track, n*2, n),
				),
			)
		}

		res := e.mustEdit(ui, project, base, ops...)
		base = res.Revision
	}

	res, err := e.svc.HistoryDiff(e.as(agent), service.HistoryDiffRequest{ProjectID: project, From: 1})
	if err != nil {
		t.Fatal(err)
	}

	if res.Count != 300 || len(res.Entries) != 300 {
		t.Fatalf("count %d, entries %d", res.Count, len(res.Entries))
	}

	if n := len([]rune(res.Text)); n > 6400 {
		t.Fatalf("the text has %d characters, the budget is 6,400", n)
	}

	lines := strings.Split(res.Text, "\n")
	shown := len(lines) - 3 // note, header, trailer

	if want := fmt.Sprintf(
		"... and %d more changes: read the diff of a narrower range, or the history",
		300-shown,
	); lines[len(lines)-1] != want {
		t.Fatalf("trailer %q, want %q", lines[len(lines)-1], want)
	}
}
