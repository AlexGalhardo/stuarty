package agenthost

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// hostile members that no allowlist may carry to a person's screen or a transcript.
var hostileValues = []string{"sk-hostile-secret-value", "hostile-bearer", "sk-hostile-in-args"}

func TestParsePermissionKeepsOnlyTheAllowlistOfAStuartyTool(t *testing.T) {
	t.Parallel()

	req, err := ParsePermission(
		json.RawMessage(`{"sessionId":"s","toolCall":{"toolCallId":"c","title":"mcp__stuarty__edit",` +
			`"kind":"other","rawInput":{"project_id":"prj_a4k9m2x7bq","item_id":"ast_3c5v8n2w6d","token":"sk-hostile-secret-value",` +
			`"authorization":"Bearer hostile-bearer","operations":[{"op":"animation.apply_preset","args":{"api_key":"sk-hostile-in-args"}}]}},` +
			`"options":[{"optionId":"o","name":"Once","kind":"allow_once"}]}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	want := chat.PermissionDetail{
		Project:    "prj_a4k9m2x7bq",
		Item:       "ast_3c5v8n2w6d",
		Operations: []string{"animation.apply_preset"},
	}

	if req.Detail == nil || req.Detail.Project != want.Project || req.Detail.Item != want.Item ||
		len(req.Detail.Operations) != 1 || req.Detail.Operations[0] != want.Operations[0] {
		t.Fatalf("Detail = %+v, want %+v", req.Detail, want)
	}

	raw := mustJSON(t, req)

	for _, secret := range hostileValues {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the parsed request carries %q: %s", secret, raw)
		}
	}
}

func TestParsePermissionKeepsNothingForAnotherTool(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"Run shell command", "library", "evil__stuarty__edit", ""} {
		req, err := ParsePermission(json.RawMessage(`{"sessionId":"s","toolCall":{"toolCallId":"c","title":` +
			`"` + title + `","rawInput":{"project_id":"prj_a4k9m2x7bq","item_id":"ast_3c5v8n2w6d"}},"options":[]}`))
		if err != nil {
			t.Fatal(err)
		}

		if req.Detail != nil {
			t.Errorf("title %q: Detail = %+v, want none", title, req.Detail)
		}
	}
}

func TestProcessPermissionOfAStuartyToolStoresOnlyTheAllowlist(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("permission-stuarty"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	ask := f.next(kind(chat.KindPermissionAsked))

	d := ask.Permission.Detail
	if d == nil || d.Project != "prj_a4k9m2x7bq" || d.Item != "ast_3c5v8n2w6d" || len(d.Operations) != 1 ||
		d.Operations[0] != "animation.apply_preset" {
		t.Fatalf("detail of the event = %+v", d)
	}

	raw := mustJSON(t, ask)

	for _, secret := range hostileValues {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the event carries %q: %s", secret, raw)
		}
	}

	waiting := e.host.Permissions()
	if len(waiting) != 1 {
		t.Fatalf("Permissions = %d, want 1", len(waiting))
	}

	p := waiting[0]
	named := p.RequestID == ask.Permission.RequestID && p.SessionID == s.ID && p.ProjectID == "prj_a"
	asked := p.Profile == "permission-stuarty" && p.Title == "mcp__stuarty__edit" && !p.AskedAt.IsZero()

	if !named || !asked || p.Answer != "" || p.AnsweredAt != nil {
		t.Errorf("waiting permission = %+v", p)
	}

	if err := e.host.Answer(s.ID, p.RequestID, chat.AnswerReject); err != nil {
		t.Fatal(err)
	}

	after := e.host.Permissions()
	if len(after) != 1 || after[0].Answer != chat.AnswerReject || after[0].AnsweredAt == nil {
		t.Errorf("after the answer = %+v", after)
	}
}

func TestProcessPermissionOfAnotherToolKeepsOnlyItsTitle(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("permission-other"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	ask := f.next(kind(chat.KindPermissionAsked))

	if ask.Permission.Detail != nil || ask.Permission.Title != "Run shell command" {
		t.Errorf("permission = %+v", ask.Permission)
	}

	if raw := mustJSON(t, ask); strings.Contains(string(raw), "evil.example") {
		t.Errorf("the input of an unknown tool was kept: %s", raw)
	}
}

func TestPermissionsLeaveWithTheSessionAndAnnounceEveryChange(t *testing.T) {
	t.Parallel()

	var changes atomic.Int64

	e := newEnv(t, Limits{}, func(o *Options) { o.OnRequestChange = func() { changes.Add(1) } })
	s := e.start(t, "prj_a", e.profile("permission"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindPermissionAsked))

	if changes.Load() < 1 {
		t.Error("asking a permission was not announced")
	}

	before := changes.Load()

	e.host.End(s.ID, chat.EndUser)

	if changes.Load() <= before {
		t.Error("the end of the session, which cancels its permission, was not announced")
	}

	if got := e.host.Permissions(); len(got) != 0 {
		t.Errorf("Permissions of an ended session = %+v", got)
	}
}

func TestScrubDetailRemovesTheSecretsOfTheSession(t *testing.T) {
	t.Parallel()

	r := newRecorder(newMemStore(), "cht_1000000001", func() time.Time { return time.Unix(0, 0) }, nil)
	r.AddSecret("tok-123456")

	d := r.ScrubDetail(
		&chat.PermissionDetail{Project: "prj_a", Item: "tok-123456", Operations: []string{"x-tok-123456-y"}},
	)

	if raw := mustJSON(t, d); strings.Contains(string(raw), "tok-123456") {
		t.Errorf("a secret survived: %s", raw)
	}

	if r.ScrubDetail(nil) != nil {
		t.Error("a nil detail became something")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}
