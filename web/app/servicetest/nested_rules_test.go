package servicetest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

func wantReason(t *testing.T, what string, err error, code, reason string) {
	t.Helper()

	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != code || se.Details["reason"] != reason {
		t.Errorf("%s: %v, want %s with reason %s", what, err, code, reason)
	}
}

func (e *env) mustDescribe(project string) *service.ProjectDescribeResult {
	e.t.Helper()

	got, err := e.svc.ProjectDescribe(e.as(ui), service.ProjectDescribeRequest{ProjectID: project})
	if err != nil {
		e.t.Fatal(err)
	}

	return got
}

// TestNestedRevertKeepsTheRules: a revert that would bring back a composition closing a loop is refused like the
// edit that would make it, also as a dry run; a revert of a nested timeline tells the timelines that use it.
func TestNestedRevertKeepsTheRules(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	parent, child, childTrack := nestedProject(t, e)

	if _, err := e.textIn(parent, e.editIn(t, parent, op("track.add", `{"kind":"visual"}`)).Created[0], "2s",
		"own"); err != nil {
		t.Fatal(err)
	}

	withComposition := e.head(parent).Revision

	doc, err := timeline.Decode(e.document(parent))
	if err != nil {
		t.Fatal(err)
	}

	var comp string

	for id := range doc.Items {
		if doc.Items[id].Type == timeline.ItemComposition {
			comp = id
		}
	}

	e.editIn(t, parent, op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, comp)))
	e.editIn(t, child, op("clip.add_composition",
		fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":"5s"}`, childTrack, parent)))

	for _, dry := range []bool{true, false} {
		_, rerr := e.svc.HistoryRevert(e.as(ui), service.HistoryRevertRequest{
			ProjectID: parent, TimelineID: parent, Revision: withComposition, DryRun: dry,
		})
		wantReason(t, fmt.Sprintf("revert to the loop (dry run %v)", dry), rerr,
			command.CodeInvalidState, "composition_cycle")
	}

	if got := e.compositionTargets(parent); len(got) != 0 {
		t.Fatalf("the refused revert left the compositions %v", got)
	}

	before := len(e.nestedEvents())
	childRev := e.head(child).Revision

	e.editIn(t, parent, op("track.add", `{"kind":"visual","name":"Top"}`))

	// The child holds the parent now: a revert of the parent is an edit of a timeline the child uses.
	if _, err := e.svc.HistoryRevert(e.as(ui), service.HistoryRevertRequest{
		ProjectID: parent, TimelineID: parent, Revision: e.head(parent).Revision - 1,
	}); err != nil {
		t.Fatal(err)
	}

	evs := e.nestedEvents()[before:]
	if len(evs) != 2 {
		t.Fatalf("the edit and the revert of the parent published %d nested.updated, want 2: %+v", len(evs), evs)
	}

	if last := evs[1]; last.ProjectID != child || last.TimelineID != parent ||
		last.Revision != e.head(parent).Revision {
		t.Errorf("after the revert the last nested.updated is %+v, want one on %s for %s at revision %d",
			last, child, parent, e.head(parent).Revision)
	}

	if e.head(child).Revision != childRev {
		t.Errorf("the child changed revision from %d to %d", childRev, e.head(child).Revision)
	}
}

// TestNestedRestoreKeepsTheDepth: a timeline restored from the trash is held to the depth limit, counting what the
// timelines it holds became while it was there.
func TestNestedRestoreKeepsTheDepth(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	x := e.newProject("x")
	ctx := e.as(ui)

	add := func(title string) (timelineID, itemID string) {
		res, err := e.svc.ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: x, Kind: "timeline", Title: title})
		if err != nil {
			t.Fatal(err)
		}

		return res.Item.TimelineID, res.Item.ID
	}

	track := func(tl string) string {
		return e.editIn(t, tl, op("track.add", `{"kind":"visual"}`)).Created[0]
	}

	nest := func(host, child string) {
		e.editIn(t, host, op("clip.add_composition",
			fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":"0s"}`, track(host), child)))
	}

	held, heldItem := add("T")
	y, _ := add("Y")

	if _, err := e.textIn(y, track(y), "0s", "y"); err != nil {
		t.Fatal(err)
	}

	nest(held, y)
	nest(x, held)

	xItem := ""

	for _, it := range e.mustDescribe(x).Items {
		if it.TimelineID == x {
			xItem = it.ID
		}
	}

	// X goes to the trash first, then T, which no active timeline uses any more; X comes back still naming T.
	for _, id := range []string{xItem, heldItem} {
		if _, err := e.svc.ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: x, ItemID: id}); err != nil {
			t.Fatalf("trash %s: %v", id, err)
		}
	}

	if _, err := e.svc.ProjectItemRestore(ctx, service.ProjectItemTrashRequest{ProjectID: x, ItemID: xItem}); err != nil {
		t.Fatalf("restore X: %v", err)
	}

	// While T is in the trash, Y comes to hold three levels.
	z3, _ := add("Z3")
	if _, err := e.textIn(z3, track(z3), "0s", "z"); err != nil {
		t.Fatal(err)
	}

	z2, _ := add("Z2")
	nest(z2, z3)

	z1, _ := add("Z1")
	nest(z1, z2)
	nest(y, z1)

	_, err := e.svc.ProjectItemRestore(ctx, service.ProjectItemTrashRequest{ProjectID: x, ItemID: heldItem})
	wantReason(t, "restoring T under X over Y of depth 3", err, command.CodeLimit, "composition_depth")
}
