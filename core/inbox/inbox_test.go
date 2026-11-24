package inbox

import (
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/command"
)

var t0 = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

func editable() Editable { return OfRegistry(command.FullRegistry()) }

func TestFromPermissionOfAStuartyTool(t *testing.T) {
	p := Permission{
		SessionID: "cht_aaaaaaaaaa",
		ProjectID: "prj_aaaaaaaaaa",
		Profile:   "claude",
		RequestID: "cpr_aaaaaaaaaa",
		Title:     "mcp__stuarty__edit",
		Skill:     "caption-style",
		AskedAt:   at(1),
		Detail: &chat.PermissionDetail{
			Project:    "prj_aaaaaaaaaa",
			Item:       "ast_bbbbbbbbbb",
			Operations: []string{"animation.list_presets"},
		},
	}

	r := FromPermission(&p, editable())

	if r.ID != "chat:cpr_aaaaaaaaaa" || r.Source != SourceChat || r.SourceID != "cpr_aaaaaaaaaa" ||
		r.State != StateWaiting {
		t.Fatalf("identity and state: %+v", r)
	}

	if r.Agent != "claude" || r.Skill != "caption-style" || r.Tool != "mcp__stuarty__edit" ||
		r.Item != "ast_bbbbbbbbbb" || r.Destination != "prj_aaaaaaaaaa" || r.SessionID != "cht_aaaaaaaaaa" {
		t.Errorf("members: %+v", r)
	}

	if r.ActionKey != "ui.agent_activity.tool.edit" || !r.Undoable || r.Cost != nil || r.AnsweredAt != nil {
		t.Errorf("key, undo, cost: %+v", r)
	}
}

func TestFromPermissionOfAnotherToolReportsNothing(t *testing.T) {
	// The detail is ignored for a tool that is not Stuarty's: a title that merely looks like one
	// of ours must not give the call an item, a destination or an undo.
	for _, title := range []string{"Run shell command", "library", "other__edit", "Bash: rm -rf /"} {
		p := Permission{
			RequestID: "cpr_aaaaaaaaaa",
			Title:     title,
			ProjectID: "prj_aaaaaaaaaa",
			Detail: &chat.PermissionDetail{
				Project:    "prj_x",
				Item:       "ast_x",
				Operations: []string{"animation.list_presets"},
			},
		}

		r := FromPermission(&p, editable())

		if r.Item != "" || r.Destination != "" || r.ActionKey != "" || r.Undoable {
			t.Errorf("%q: reported %+v", title, r)
		}

		if r.Tool != title {
			t.Errorf("%q: the title is kept as the tool, got %q", title, r.Tool)
		}
	}
}

func TestChatStates(t *testing.T) {
	cases := map[chat.Answer]State{
		"":                StateWaiting,
		chat.AnswerAllow:  StateApproved,
		chat.AnswerReject: StateRejected,
		chat.AnswerCancel: StateCancelled,
	}

	for answer, want := range cases {
		r := FromPermission(&Permission{RequestID: "cpr_aaaaaaaaaa", Answer: answer}, nil)
		if r.State != want {
			t.Errorf("answer %q: state %q, want %q", answer, r.State, want)
		}
	}
}

func TestUndoable(t *testing.T) {
	e := editable()

	cases := []struct {
		name string
		tool string
		ops  []string
		want bool
	}{
		{"an edit of known commands", "edit", []string{"animation.list_presets", "asset.remove"}, true},
		{"an edit with one unknown command", "edit", []string{"animation.list_presets", "no.such_command"}, false},
		{"an internal command", "edit", []string{"asset.add"}, false},
		{"an edit that names nothing", "edit", nil, false},
		{"a publication tool", "job_start", []string{"animation.list_presets"}, false},
		{"history", "history", []string{"undo"}, false},
		{"no tool", "", []string{"animation.list_presets"}, false},
	}

	for _, c := range cases {
		if got := Undoable(c.tool, c.ops, e); got != c.want {
			t.Errorf("%s: Undoable = %v, want %v", c.name, got, c.want)
		}
	}

	if Undoable("edit", []string{"animation.list_presets"}, nil) {
		t.Error("with no registry nothing is undoable")
	}
}

func TestFromApproval(t *testing.T) {
	a := Approval{
		ID:           "apr_aaaaaaaaaa",
		Actor:        "key:abc",
		Kind:         "publish.upload",
		Summary:      "Publish the video",
		ConnectionID: "con_aaaaaaaaaa",
		ProjectID:    "prj_aaaaaaaaaa",
		Estimate:     approval.Money{Micros: 1_500_000, Currency: "USD"},
		State:        "pending",
		CreatedAt:    at(2),
	}

	r := FromApproval(&a)

	if r.ID != "approval:apr_aaaaaaaaaa" || r.Source != SourceApproval || r.State != StateWaiting ||
		r.Tool != "publish.upload" {
		t.Fatalf("identity: %+v", r)
	}

	if r.Cost == nil || r.Cost.Micros != 1_500_000 || r.Cost.Currency != "USD" || r.Undoable {
		t.Errorf("cost and undo: %+v", r)
	}

	if r.Destination != "con_aaaaaaaaaa / prj_aaaaaaaaaa" || r.Summary != "Publish the video" || r.SessionID != "" {
		t.Errorf("where: %+v", r)
	}

	a.Estimate = approval.Money{}
	a.ConnectionID = ""

	if got := FromApproval(&a); got.Cost != nil || got.Destination != "prj_aaaaaaaaaa" {
		t.Errorf("free action with no connection: %+v", got)
	}

	for in, want := range map[string]State{
		"pending": StateWaiting, "approved": StateApproved, "rejected": StateRejected, "expired": StateExpired,
	} {
		a.State = in
		if got := FromApproval(&a).State; got != want {
			t.Errorf("approval state %q became %q, want %q", in, got, want)
		}
	}
}

func TestOrder(t *testing.T) {
	rows := []Row{
		{ID: "chat:old-wait", State: StateWaiting, CreatedAt: at(1)},
		{ID: "approval:answered-early", State: StateApproved, CreatedAt: at(0), AnsweredAt: new(at(5))},
		{ID: "approval:new-wait", State: StateWaiting, CreatedAt: at(9)},
		{ID: "chat:answered-late", State: StateRejected, CreatedAt: at(2), AnsweredAt: new(at(8))},
		{ID: "approval:expired", State: StateExpired, CreatedAt: at(3)},
		{ID: "chat:tie-b", State: StateWaiting, CreatedAt: at(1)},
	}

	got := Order(rows)

	want := []string{
		"approval:new-wait",
		"chat:old-wait",
		"chat:tie-b",
		"chat:answered-late",
		"approval:answered-early",
		"approval:expired",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}

	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("row %d = %s, want %s (all: %v)", i, got[i].ID, id, ids(got))
		}
	}

	if rows[0].ID != "chat:old-wait" {
		t.Error("Order changed its input")
	}
}

func TestOrderKeepsEveryWaitingRowAndCapsTheRest(t *testing.T) {
	var rows []Row

	for i := range MaxRecent + 7 {
		rows = append(
			rows,
			Row{
				ID:         "approval:done-" + string(rune('a'+i)),
				State:      StateApproved,
				CreatedAt:  at(i),
				AnsweredAt: new(at(i)),
			},
		)
	}

	for i := range 30 {
		rows = append(rows, Row{ID: "chat:wait-" + string(rune('a'+i)), State: StateWaiting, CreatedAt: at(i)})
	}

	got := Order(rows)

	if len(got) != 30+MaxRecent {
		t.Fatalf("got %d rows, want %d: every waiting row and %d settled", len(got), 30+MaxRecent, MaxRecent)
	}

	if got[30].State == StateWaiting || got[29].State != StateWaiting {
		t.Error("the waiting rows are not all first")
	}

	if want := "approval:done-" + string(rune('a'+MaxRecent+6)); got[30].ID != want {
		t.Errorf("the newest settled row comes first: got %s, want %s", got[30].ID, want)
	}
}

func TestOrderOfNothing(t *testing.T) {
	if got := Order(nil); got == nil || len(got) != 0 {
		t.Errorf("Order(nil) = %#v, want an empty list that encodes as []", got)
	}
}

func ids(rows []Row) []string {
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].ID
	}

	return out
}
