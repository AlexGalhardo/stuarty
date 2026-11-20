package servicetest

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// nestedProject makes a production with a timeline p that holds a composition of a second timeline, "Intro", that
// holds one text. It returns the ids of the two timelines and of the composition's track.
func nestedProject(t *testing.T, e *env) (parent, child, childTrack string) {
	t.Helper()

	parent = e.newProject("p")

	item, err := e.svc.ProjectItemAdd(
		e.as(ui),
		service.ProjectItemAddRequest{ProjectID: parent, Kind: "timeline", Title: "Intro"},
	)
	if err != nil {
		t.Fatal(err)
	}

	child = item.Item.TimelineID

	childTrack = e.editIn(t, child, op("track.add", `{"kind":"visual","name":"V"}`)).Created[0]
	if _, err := e.textIn(child, childTrack, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	parentTrack := e.editIn(t, parent, op("track.add", `{"kind":"visual","name":"V"}`)).Created[0]
	e.editIn(t, parent, op("clip.add_composition",
		fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":"0s"}`, parentTrack, child)))

	return parent, child, childTrack
}

func (e *env) nestedEvents() []service.Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	var out []service.Event

	for i := range e.events {
		if e.events[i].Type == "nested.updated" {
			out = append(out, e.events[i])
		}
	}

	return out
}

// TestNestedInvalidationEvent: an edit of a timeline that another holds as a composition publishes nested.updated
// on the stream of the other, and changes its plan without any edit of it (RF-083).
func TestNestedInvalidationEvent(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	parent, child, _ := nestedProject(t, e)

	if got := e.nestedEvents(); len(got) != 0 {
		t.Fatalf("editing the parent published nested.updated: %+v", got)
	}

	before, err := e.svc.PlanWindow(
		e.as(ui),
		service.PlanWindowRequest{ProjectID: parent, TimelineID: parent, From: 0, Until: 30},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(before.Layers) != 1 || !strings.Contains(before.Layers[0].Item, "/") {
		t.Fatalf(
			"the plan of the parent has the layers %+v, want the text of the child as one nested layer",
			before.Layers,
		)
	}

	other := e.editIn(t, child, op("track.add", `{"kind":"visual","name":"V2"}`)).Created[0]

	res, err := e.textIn(child, other, "0s", "second")
	if err != nil {
		t.Fatal(err)
	}

	evs := e.nestedEvents()
	if len(evs) != 2 || evs[1].ProjectID != parent || evs[1].TimelineID != child || evs[1].Revision != res.Revision {
		t.Fatalf(
			"nested.updated = %+v, want two (track, text), the last on %s for %s at revision %d",
			evs,
			parent,
			child,
			res.Revision,
		)
	}

	head := e.head(parent)

	after, err := e.svc.PlanWindow(
		e.as(ui),
		service.PlanWindowRequest{ProjectID: parent, TimelineID: parent, From: 0, Until: 60},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(after.Layers) != 2 ||
		after.Revision != head.Revision {
		t.Errorf("after the edit of the child the plan is %+v at revision %d, want 2 layers at %d",
			after.Layers, after.Revision, head.Revision)
	}

	if undone, uerr := e.svc.Undo(
		e.as(ui),
		service.UndoRequest{ProjectID: child, TimelineID: child},
	); uerr != nil ||
		!undone.Applied {
		t.Fatalf("undo: %v", uerr)
	}

	if got := e.nestedEvents(); len(got) != 3 {
		t.Errorf("an undo of the child published %d nested.updated in all, want 3", len(got))
	}
}

// TestNestedInvalidationRules: the loop and the use of a timeline are refused by the service.
func TestNestedInvalidationRules(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	parent, child, childTrack := nestedProject(t, e)

	_, err := e.editInErr(child, op("clip.add_composition",
		fmt.Sprintf(`{"track_id":%q,"timeline_id":%q}`, childTrack, parent)))

	if se, ok := errors.AsType[*service.Error](
		err,
	); !ok || se.Code != command.CodeInvalidState ||
		se.Details["reason"] != "composition_cycle" {
		t.Errorf("a loop was not refused as a cycle: %v", err)
	}

	items, gerr := e.svc.ProjectDescribe(e.as(ui), service.ProjectDescribeRequest{ProjectID: parent})
	if gerr != nil {
		t.Fatal(gerr)
	}

	var itemID string

	for i := range items.Items {
		if items.Items[i].TimelineID == child {
			itemID = items.Items[i].ID
		}
	}

	_, terr := e.svc.ProjectItemTrash(e.as(ui), service.ProjectItemTrashRequest{ProjectID: parent, ItemID: itemID})
	if se, ok := errors.AsType[*service.Error](
		terr,
	); !ok || se.Code != command.CodeInUse ||
		se.Details["used_by"] == nil {
		t.Errorf("trashing a timeline that is used: %v", terr)
	}
}

// TestNestedInvalidationCreateFromSelection: the new timeline of composition.create_from_selection is stored with
// the commit, in the production, and an undo restores the items and leaves it.
func TestNestedInvalidationCreateFromSelection(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	p := e.newProject("p")
	track := e.editIn(t, p, op("track.add", `{"kind":"visual","name":"V"}`)).Created[0]

	res, err := e.textIn(p, track, "0s", "hi")
	if err != nil {
		t.Fatal(err)
	}

	text := res.Created[0]

	made := e.editIn(
		t,
		p,
		op("composition.create_from_selection", fmt.Sprintf(`{"item_ids":[%q],"name":"Intro"}`, text)),
	)
	if len(made.Created) == 0 {
		t.Fatalf("no composition item was created: %+v", made)
	}

	got, gerr := e.svc.ProjectDescribe(e.as(ui), service.ProjectDescribeRequest{ProjectID: p})
	if gerr != nil {
		t.Fatal(gerr)
	}

	timelines := 0

	for i := range got.Items {
		if got.Items[i].Kind == "timeline" {
			timelines++
		}
	}

	if timelines != 2 {
		t.Fatalf("the production has %d timelines after the command, want 2", timelines)
	}

	if _, uerr := e.svc.Undo(e.as(ui), service.UndoRequest{ProjectID: p, TimelineID: p}); uerr != nil {
		t.Fatalf("undo: %v", uerr)
	}

	doc := e.head(p)
	if doc.Revision != made.Revision+1 {
		t.Errorf("revision after undo = %d, want %d", doc.Revision, made.Revision+1)
	}
}

// editIn edits the timeline tl at its current revision.
func (e *env) editIn(t *testing.T, tl string, ops ...command.Operation) *service.EditResult {
	t.Helper()

	res, err := e.editInErr(tl, ops...)
	if err != nil {
		t.Fatalf("edit of %s: %v", tl, err)
	}

	return res
}

func (e *env) editInErr(tl string, ops ...command.Operation) (*service.EditResult, error) {
	e.t.Helper()

	return e.svc.Edit(e.as(ui), service.EditRequest{
		ProjectID: tl, TimelineID: tl, BaseRevision: e.head(tl).Revision, Operations: ops,
	})
}

func (e *env) textIn(tl, track, start, text string) (*service.EditResult, error) {
	e.t.Helper()

	return e.editInErr(tl, op("text.add",
		fmt.Sprintf(`{"track_id":%q,"start":%q,"duration":"1s","text":%q}`, track, start, text)))
}
