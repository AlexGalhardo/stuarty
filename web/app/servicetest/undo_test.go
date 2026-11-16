package servicetest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

func (e *env) undo(actor, project string) (*service.UndoResult, error) {
	e.t.Helper()

	return e.svc.Undo(e.as(actor), service.UndoRequest{ProjectID: project})
}

func (e *env) redo(project string) (*service.UndoResult, error) {
	e.t.Helper()

	return e.svc.Redo(e.as(ui), service.UndoRequest{ProjectID: project})
}

func (e *env) closedBy(project string, rev int64) *int64 {
	e.t.Helper()

	row, err := e.store.Operations().Get(context.Background(), project, rev)
	if err != nil {
		e.t.Fatal(err)
	}

	return row.ClosedBy
}

func eq(p *int64, want int64) bool { return p != nil && *p == want }

// SDD-005 section 12.1: the worked example, with the exact closed_by values.
func TestUndoRedoBookkeepingFollowsTheWorkedExample(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack() // r1: track.add by ui (the first edit of the example)

	mustText := func(base int64, start string) {
		t.Helper()

		if _, err := e.addText(ui, project, track, base, start, "x"); err != nil {
			t.Fatal(err)
		}
	}

	mustText(1, "0s") // r2
	mustUndo := func(wantRev int64) *service.UndoResult {
		t.Helper()

		res, err := e.undo(ui, project)
		if err != nil || res.Revision != wantRev {
			t.Fatalf("undo = %+v, %v; want r%d", res, err, wantRev)
		}

		return res
	}

	u3 := mustUndo(3) // undoes r2
	if u3.Undone == nil || u3.Undone.Revision != 2 || u3.Kind != "undo" || u3.BaseRevision != 2 || !u3.Applied {
		t.Errorf("undo result = %+v", u3)
	}

	u4 := mustUndo(4) // undoes r1
	if u4.Undone.Revision != 1 {
		t.Errorf("second undo undid r%d, want r1", u4.Undone.Revision)
	}

	r5, err := e.redo(project) // redoes undo r4
	if err != nil || r5.Revision != 5 || r5.Redone == nil || r5.Redone.Revision != 4 || r5.Kind != "redo" {
		t.Fatalf("redo = %+v, %v", r5, err)
	}

	mustText(5, "4s") // r6: a new edit ends the redo chain

	mustUndo(7) // undoes r6
	u8 := mustUndo(8)

	if u8.Undone.Revision != 5 || u8.Undone.Kind != "redo" {
		t.Errorf("the undo of a redo = %+v, want r5 (kind redo)", u8.Undone)
	}

	for rev, want := range map[int64]int64{1: 4, 2: 3, 3: 6, 4: 5, 5: 8, 6: 7} {
		if got := e.closedBy(project, rev); !eq(got, want) {
			t.Errorf("r%d closed_by = %v, want %d", rev, got, want)
		}
	}

	for _, rev := range []int64{7, 8} {
		if got := e.closedBy(project, rev); got != nil {
			t.Errorf("r%d closed_by = %d, want open", rev, *got)
		}
	}

	// Nothing is left to undo but the track's own creation, then nothing at all.
	if _, err := e.redo(project); code(err) != service.CodeNothingToRedo && err != nil {
		t.Errorf("redo = %v", err)
	}
}

// Check 15: undo then redo returns the original document byte for byte.
func TestUndoThenRedoRestoresTheDocumentBytes(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	if _, err := e.addText(ui, project, track, 1, "0s", "x"); err != nil {
		t.Fatal(err)
	}

	original := string(e.document(project))

	if _, err := e.undo(ui, project); err != nil {
		t.Fatal(err)
	}

	if string(e.document(project)) == original {
		t.Fatal("undo changed nothing")
	}

	res, err := e.redo(project)
	if err != nil || !res.Applied {
		t.Fatalf("redo = %+v, %v", res, err)
	}

	if got := string(e.document(project)); got != original {
		t.Errorf("document after undo and redo differs:\n%s\nwant\n%s", got, original)
	}

	// A dry run says what would happen and writes nothing.
	revision := e.head(project).Revision

	dry, err := e.svc.Undo(e.as(ui), service.UndoRequest{ProjectID: project, DryRun: true})
	if err != nil || dry.Applied || !dry.DryRun || len(dry.Diff) == 0 || e.head(project).Revision != revision {
		t.Errorf("dry run undo = %+v, %v", dry, err)
	}
}

// Check 16.
func TestNothingToUndoOrRedo(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project := e.newProject("p")

	for name, call := range map[string]func() error{
		"undo": func() error { _, err := e.undo(ui, project); return err },
		"redo": func() error { _, err := e.redo(project); return err },
	} {
		err := call()
		se := serviceError(t, err)

		want := service.CodeNothingToUndo
		if name == "redo" {
			want = service.CodeNothingToRedo
		}

		if se.Code != want || se.Details != nil || se.HTTPStatus() != 409 {
			t.Errorf("%s on a new project = %+v", name, se)
		}
	}

	// A new edit by the actor makes redo return E_NOTHING_TO_REDO.
	project, track := e.withTrack()
	if _, err := e.undo(ui, project); err != nil {
		t.Fatal(err)
	}

	if _, err := e.addText(ui, project, track, 2, "0s", "x"); code(err) == "" && err == nil {
		t.Fatal("the track was undone, so adding text to it must fail")
	}

	e.mustEdit(ui, project, 2, op("track.add", `{"kind":"visual"}`))

	if _, err := e.redo(project); code(err) != service.CodeNothingToRedo {
		t.Errorf("redo after a new edit = %v, want E_NOTHING_TO_REDO", err)
	}

	if _, err := e.svc.Undo(
		e.as(ui),
		service.UndoRequest{ProjectID: "prj_0000000777"},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("undo of a missing project = %v", err)
	}

	if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.undo(ui, project); code(err) != command.CodeInvalidState {
		t.Errorf("undo in the trash = %v, want E_INVALID_STATE", err)
	}
}

// Check 13, second half, and check 14: chains are per actor, and an undo skips
// exactly what another actor changed.
func TestUndoIsScopedToTheActorAndSkipsWhatOthersChanged(t *testing.T) {
	t.Parallel()

	t.Run("the chains are independent", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()

		if _, err := e.addText(agent, project, track, 1, "0s", "agent's"); err != nil {
			t.Fatal(err)
		}

		// ui has the track.add of r1 to undo, not the agent's text.
		res, err := e.undo(ui, project)
		if err != nil || res.Undone.Revision != 1 {
			t.Fatalf("ui undo = %+v, %v; want r1", res, err)
		}

		// The track holds the agent's item, so the removal of the track is skipped.
		if res.Applied || len(res.Skipped) == 0 || res.Skipped[0].Reason == "" {
			t.Errorf("result = %+v, want the track removal skipped", res)
		}

		ag, err := e.undo(agent, project)
		if err != nil || ag.Undone.Revision != 2 || ag.Undone.Actor != agent {
			t.Errorf("agent undo = %+v, %v", ag, err)
		}
	})

	t.Run("field changed by another actor", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		id := "itm_00000000aa"
		e.mustEdit(
			ui,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, track, id)),
		)
		e.mustEdit(agent, project, 2, op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"3s"}`, id))) // r3
		e.mustEdit(
			ui,
			project,
			3,
			op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"6s"}`, id)),
		) // r4 by ui, over the agent's move

		// The agent undoes its r3, but ui changed start since: that field is skipped.
		res, err := e.undo(agent, project)
		if err != nil {
			t.Fatal(err)
		}

		if res.Applied || len(res.Diff) != 0 || len(res.Skipped) != 1 {
			t.Fatalf("result = %+v", res)
		}

		sk := res.Skipped[0]
		if sk.Reason != command.SkipFieldChanged || sk.ByActor != ui || sk.ByRevision == nil || *sk.ByRevision != 4 ||
			len(sk.Fields) != 1 ||
			sk.Fields[0] != "start" {
			t.Errorf("skipped = %+v", sk)
		}

		// It is still a batch: the candidate was consumed, so the next undo reaches further back.
		if res.Revision != 5 || !eq(e.closedBy(project, 3), 5) {
			t.Errorf("revision %d, r3 closed_by %v", res.Revision, e.closedBy(project, 3))
		}
	})

	t.Run("object changed or removed by another actor", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		id := "itm_00000000aa"
		e.mustEdit(
			agent,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, track, id)),
		)
		e.mustEdit(ui, project, 2, op("text.set", fmt.Sprintf(`{"item_id":%q,"text":"changed"}`, id)))

		res, err := e.undo(agent, project) // would remove the item ui changed
		if err != nil || res.Applied || len(res.Skipped) != 1 || res.Skipped[0].Reason != command.SkipObjectChanged {
			t.Fatalf("undo = %+v, %v", res, err)
		}

		// Removal by another actor: the object is missing.
		e2 := newEnv(t)
		p2, t2 := e2.withTrack()
		e2.mustEdit(
			agent,
			p2,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, t2, id)),
		)
		e2.mustEdit(ui, p2, 2, op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, id)))

		res, err = e2.undo(agent, p2)
		if err != nil || res.Applied || len(res.Skipped) != 1 || res.Skipped[0].Reason != command.SkipObjectMissing {
			t.Fatalf("undo after a removal = %+v, %v", res, err)
		}
	})

	t.Run("a put that would overlap", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		id, other := "itm_00000000aa", "itm_00000000bb"
		e.mustEdit(
			agent,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, track, id)),
		)
		e.mustEdit(
			agent,
			project,
			2,
			op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, id)),
		) // r3
		e.mustEdit(
			ui,
			project,
			3,
			op(
				"text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"b","id":%q}`, track, other),
			),
		) // takes the place

		res, err := e.undo(agent, project) // would put itm_aa back over itm_bb
		if err != nil {
			t.Fatal(err)
		}

		if res.Applied || len(res.Skipped) != 1 || res.Skipped[0].Reason != command.SkipInvalidResult {
			t.Fatalf("result = %+v", res)
		}

		violations, _ := res.Skipped[0].Details["violations"].([]string)
		if len(violations) == 0 || violations[0] != "V_OVERLAP" {
			t.Errorf("details = %v, want V_OVERLAP", res.Skipped[0].Details)
		}
	})

	t.Run("a step that is skipped entirely is still consumed", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		id := "itm_00000000aa"
		e.mustEdit(
			agent,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, track, id)),
		)
		e.mustEdit(ui, project, 2, op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, id)))

		if _, err := e.undo(agent, project); err != nil {
			t.Fatal(err)
		}

		if _, err := e.undo(agent, project); code(err) != service.CodeNothingToUndo {
			t.Errorf("second undo = %v, want E_NOTHING_TO_UNDO: the stuck step was consumed", err)
		}
	})
}

func TestHistoryList(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	for i := range 4 {
		if _, err := e.addText(ui, project, track, int64(1+i), fmt.Sprintf("%ds", i*2), "x"); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := e.undo(ui, project); err != nil { // r6 undoes r5
		t.Fatal(err)
	}

	ctx := e.as(agent)

	page, err := e.svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: project, Limit: 3})
	if err != nil || len(page.Entries) != 3 || page.NextCursor == nil || page.Revision != 6 {
		t.Fatalf("page = %+v, %v", page, err)
	}

	top := page.Entries[0]
	if top.Revision != 6 || top.Kind != "undo" || top.UndoOf == nil || *top.UndoOf != 5 || top.Changes == 0 {
		t.Errorf("newest = %+v", top)
	}

	if e := page.Entries[1]; e.Revision != 5 || e.UndoneBy == nil || *e.UndoneBy != 6 || len(e.Ops) != 1 ||
		e.Ops[0] != "text.add" {
		t.Errorf("undone entry = %+v", e)
	}

	for _, want := range []string{"history " + project + " rev 6 (showing 3 of 6, newest first)", "undo-of:r5", "undone-by:r6", "next_cursor: "} {
		if !strings.Contains(page.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, page.Text)
		}
	}

	next, err := e.svc.HistoryList(
		ctx,
		service.HistoryListRequest{ProjectID: project, Cursor: *page.NextCursor, Limit: 10},
	)
	if err != nil || len(next.Entries) != 3 || next.Entries[0].Revision != 3 || next.NextCursor != nil {
		t.Fatalf("next page = %+v, %v", next, err)
	}

	only, err := e.svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: project, Actor: "local:nobody"})
	if err != nil || len(only.Entries) != 0 || !strings.Contains(only.Text, "showing 0 of 0") {
		t.Errorf("filter by an actor with no steps = %+v, %v", only, err)
	}

	bare := strings.TrimPrefix(top.Actor, "local:")
	if byBare, err := e.svc.HistoryList(
		ctx,
		service.HistoryListRequest{ProjectID: project, Actor: bare},
	); err != nil || len(byBare.Entries) == 0 {
		t.Errorf("the filter %q (no kind prefix) = %+v, %v; it must mean local:%s", bare, byBare, err, bare)
	}

	if _, err := e.svc.HistoryList(
		ctx,
		service.HistoryListRequest{ProjectID: project, Cursor: "nope"},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("bad cursor = %v", err)
	}

	if _, err := e.svc.HistoryList(
		ctx,
		service.HistoryListRequest{ProjectID: "prj_0000000777"},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("missing project = %v", err)
	}
}

// Check 17: no undo crosses a migration. The store half: a migrate row ends every chain.
func TestEventsAreOnePerCommitInRevisionOrder(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	if _, err := e.addText(ui, project, track, 1, "0s", "x"); err != nil {
		t.Fatal(err)
	}

	if _, err := e.undo(ui, project); err != nil {
		t.Fatal(err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	var kinds []string

	for _, ev := range e.events {
		if ev.ProjectID == project && ev.Type == "batch.committed" {
			kinds = append(kinds, fmt.Sprintf("%d:%s", ev.Revision, ev.Kind))
		}
	}

	if got := strings.Join(kinds, " "); got != "1:edit 2:edit 3:undo" {
		t.Errorf("events = %s", got)
	}
}

// SDD-012 section 15: the history says which step the caller can undo or redo.
func TestHistoryEntriesCarryDescriptionAndPerActorUndoFlags(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack() // r1 by ui

	if _, err := e.addText(agent, project, track, 1, "0s", "x"); err != nil { // r2 by agent
		t.Fatal(err)
	}

	list := func(actor string) map[int64]service.HistoryEntry {
		t.Helper()

		res, err := e.svc.HistoryList(e.as(actor), service.HistoryListRequest{ProjectID: project})
		if err != nil {
			t.Fatal(err)
		}

		out := map[int64]service.HistoryEntry{}
		for _, en := range res.Entries {
			out[en.Revision] = en
		}

		return out
	}

	byUI := list(ui)
	if !byUI[1].UndoableByMe || byUI[2].UndoableByMe || byUI[1].RedoableByMe {
		t.Errorf("ui flags = %+v", byUI)
	}

	if !list(agent)[2].UndoableByMe || list(agent)[1].UndoableByMe {
		t.Errorf("agent flags = %+v", list(agent))
	}

	if d := byUI[1].Description; d == "" || strings.Contains(d, "\n") {
		t.Errorf("description = %q, want one non-empty line", d)
	}

	if _, err := e.undo(ui, project); err != nil { // r3
		t.Fatal(err)
	}

	after := list(ui)
	if after[1].UndoableByMe || !after[3].RedoableByMe || after[3].Description != "Undid r1." {
		t.Errorf("after undo = %+v", after)
	}
}
