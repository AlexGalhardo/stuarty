package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// publicationToolCall is how an operation of the calendar is a call of the MCP tool publication (SDD-003 section
// 4.2, SDD-020 section 29): the members have the names of the Service requests, except the end of a range, which
// is until (the outbound check refuses a member named to). The other four operations have no action.
func publicationToolCall(call string, args map[string]any) (tool string, out map[string]any, ok bool) {
	switch call {
	case "PublicationCreate":
		args["action"] = "create"
	case "PublicationGet":
		args["action"] = "show"
	case "PublicationList":
		args["action"] = "list"

		if v, has := args["to"]; has {
			delete(args, "to")

			args["until"] = v
		}
	case "PublicationSchedule":
		args["action"] = "schedule"
	case "PublicationDuplicate":
		args["action"] = "duplicate"
	case "CalendarPlan":
		args["action"] = "plan"

		if v, has := args["to"]; has {
			delete(args, "to")

			args["until"] = v
		}
	case "CalendarDaySet":
		args["action"] = "day"
	case "CalendarSlotSave", "CalendarSlotDelete":
		args["action"] = "slot"

		if v, has := args["id"]; has {
			delete(args, "id")

			args["slot"] = v
		}

		if v, has := args["connection_id"]; has {
			delete(args, "connection_id")

			args["account"] = v
		}

		if call == "CalendarSlotDelete" {
			args["remove"] = true
		}
	default:
		return "", nil, false
	}

	return "publication", args, true
}

// VV-282 and D-143: an agent that speaks only MCP plans, schedules, reads and lists a publication, and the
// answers are those of HTTP, the CLI and the Service, byte for byte; scheduling leaves each target waiting for a
// person.
func TestPublicationToolIsTheSameOnEverySurface(t *testing.T) {
	got := map[string][]string{}

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith, "mcp": newMCPWith,
	} {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t, newDriver)
			d, svc := fx.d, fx.d.service()
			kids := false

			targets := []pub.TargetInput{
				{
					ConnectionID: fx.connection, At: "2026-12-01T09:00", Zone: "America/Sao_Paulo",
					Fields: pub.Fields{MadeForKids: &kids},
				},
				{
					ConnectionID: fx.connection, At: "2026-12-02T09:30", Zone: "UTC",
					Fields: pub.Fields{MadeForKids: &kids, Title: "Second"},
				},
			}

			mk := func(call string, v any) step {
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}

				return step{Call: call, Args: raw}
			}

			var results []string

			run := func(s step) json.RawMessage {
				raw, _, fail := d.do(t, s)
				if fail != nil {
					t.Fatalf("%s %s: %+v", d.name(), s.Call, fail)
				}

				results = append(results, normalize(t, d, raw))

				return raw
			}

			raw := run(mk("PublicationCreate", service.PublicationCreateRequest{
				ProjectID: fx.project, Title: "Week one", Text: "A caption", Export: fx.export, Targets: targets,
			}))

			var made service.Publication
			if err := json.Unmarshal(raw, &made); err != nil || len(made.Targets) != 2 {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			first, second := made.Targets[0].ID, made.Targets[1].ID

			run(mk("PublicationGet", service.PublicationGetRequest{ID: made.ID}))
			run(mk("PublicationList", service.PublicationListRequest{ProjectID: fx.project, State: "draft"}))
			run(mk("PublicationSchedule", service.PublicationScheduleRequest{ID: made.ID, TargetID: first}))
			run(mk("PublicationSchedule", service.PublicationScheduleRequest{
				ID: made.ID, TargetID: second, At: "2026-12-03T10:00", Zone: "Europe/Lisbon",
			}))
			run(mk("PublicationList", service.PublicationListRequest{
				ProjectID: fx.project, From: "2026-12-02", To: "2026-12-03", Zone: "Pacific/Pago_Pago",
			}))

			var day service.PublicationListResult
			if err := json.Unmarshal(
				[]byte(results[len(results)-1]), &day,
			); err != nil || len(day.Publications) != 1 {
				t.Fatalf("%s: the day in the zone = %s, %v", d.name(), results[len(results)-1], err)
			}

			read := run(mk("PublicationGet", service.PublicationGetRequest{ID: made.ID}))

			var after service.Publication
			if err := json.Unmarshal(read, &after); err != nil {
				t.Fatal(err)
			}

			for _, tg := range after.Targets {
				if tg.State != "waiting_approval" || tg.ApprovalID == "" {
					t.Errorf("%s: target %s is %s, want waiting_approval with an approval", d.name(), tg.ID, tg.State)
				}
			}

			pending, err := svc.ApprovalList(callerCtx(), service.ApprovalListRequest{})
			if err != nil || len(pending.Requests) != 2 {
				t.Fatalf("%s: pending = %+v, %v", d.name(), pending, err)
			}

			for _, r := range pending.Requests {
				if r.Kind != service.KindPublishScheduled || r.State != "pending" {
					t.Errorf("%s: request %+v, want a pending publish.scheduled", d.name(), r)
				}
			}

			if n := fx.uploads.Load(); n != 0 {
				t.Errorf("%s: %d requests left the machine before a person approved", d.name(), n)
			}

			got[name] = results
		})
	}

	base := got["in-process"]
	for name, other := range got {
		if len(other) != len(base) {
			t.Fatalf("%s answered %d calls, in-process %d", name, len(other), len(base))
		}

		for i := range base {
			if canonical(t, json.RawMessage(base[i])) != canonical(t, json.RawMessage(other[i])) {
				t.Errorf("call %d differs between in-process and %s:\n%s\n%s", i, name, base[i], other[i])
			}
		}
	}
}

// publicationToolOf calls the tool publication.
func publicationToolOf(t *testing.T, cs *sdk.ClientSession, args map[string]any) (*sdk.CallToolResult, error) {
	t.Helper()

	return cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "publication", Arguments: args})
}

// refused is true when a call did not run: a protocol error, or a result that is an error.
func refused(res *sdk.CallToolResult, err error) bool {
	return err != nil || (res != nil && res.IsError)
}

func publicationOf(t *testing.T, res *sdk.CallToolResult) service.Publication {
	t.Helper()

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var p service.Publication
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}

	return p
}

// errorCodeOf is the code in the envelope on the last line of the text of an error result (SDD-003 section 1.3).
func errorCodeOf(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()

	if res == nil || !res.IsError || len(res.Content) == 0 {
		t.Fatalf("the result is %+v, want an error", res)
	}

	tc, _ := res.Content[0].(*sdk.TextContent)
	if tc == nil {
		t.Fatalf("the content is %T, want text", res.Content[0])
	}

	last := tc.Text[strings.LastIndex(tc.Text, "\n")+1:]

	var doc service.ErrorDocument
	if err := json.Unmarshal([]byte(last), &doc); err != nil {
		t.Fatalf("no envelope in %q", tc.Text)
	}

	return doc.Code
}

func connectedMCP(t *testing.T, fx publishFixture) (*mcpDriver, *sdk.ClientSession) {
	t.Helper()

	d, ok := fx.d.(*mcpDriver)
	if !ok {
		t.Fatalf("the driver is %T, want *mcpDriver", fx.d)
	}

	return d, d.cs
}

// planThroughTheTool plans a publication of one target and schedules it, with the tool alone.
func planThroughTheTool(t *testing.T, cs *sdk.ClientSession, fx publishFixture) service.Publication {
	t.Helper()

	created, err := publicationToolOf(t, cs, map[string]any{
		"action": "create", "project_id": fx.project, "title": "Launch", "export": fx.export,
		"targets": []any{map[string]any{
			"connection_id": fx.connection, "at": "2026-12-01T09:00", "zone": "UTC",
			"fields": map[string]any{"made_for_kids": false},
		}},
	})
	if refused(created, err) {
		t.Fatalf("create: %v %+v", err, created)
	}

	made := publicationOf(t, created)

	scheduled, err := publicationToolOf(t, cs, map[string]any{"action": "schedule", "id": made.ID})
	if refused(scheduled, err) {
		t.Fatalf("schedule: %v %+v", err, scheduled)
	}

	return publicationOf(t, scheduled)
}

// T-160: the tool of the calendar publishes nothing. An agent that plans and schedules over MCP alone leaves the
// target waiting for a person with a pending request, nothing reaches the network, and no action or member of
// the tool sends, retries, changes, cancels or approves.
func TestSecurity_T160_PublicationToolPublishesNothing(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	d, cs := connectedMCP(t, fx)
	svc := d.service()

	p := planThroughTheTool(t, cs, fx)
	if len(p.Targets) != 1 || p.Targets[0].State != "waiting_approval" || p.Targets[0].ApprovalID == "" {
		t.Fatalf("the target = %+v, want waiting_approval with an approval", p.Targets)
	}

	for _, action := range []string{
		"update", "send_now", "send-now", "retry", "cancel", "approve", "reject", "decide", "publish", "delete",
	} {
		args := map[string]any{"action": action, "id": p.ID, "target_id": p.Targets[0].ID}
		if res, err := publicationToolOf(t, cs, args); !refused(res, err) {
			t.Errorf("the action %s answered %+v, want a refusal", action, res)
		}
	}

	for _, member := range []string{"approve", "decision", "publish", "send", "send_now", "approval_id", "state"} {
		for _, action := range []string{"schedule", "show"} {
			args := map[string]any{"action": action, "id": p.ID, member: true}
			if res, err := publicationToolOf(t, cs, args); !refused(res, err) {
				t.Errorf("%s with %s answered %+v, want a refusal", action, member, res)
			}
		}
	}

	after, err := svc.PublicationGet(callerCtx(), service.PublicationGetRequest{ID: p.ID})
	if err != nil || len(after.Targets) != 1 || after.Targets[0].State != "waiting_approval" ||
		after.Targets[0].ApprovalID != p.Targets[0].ApprovalID {
		t.Fatalf("after the guesses the publication is %+v, %v", after, err)
	}

	pending, err := svc.ApprovalList(callerCtx(), service.ApprovalListRequest{})
	if err != nil || len(pending.Requests) != 1 || pending.Requests[0].State != "pending" ||
		pending.Requests[0].Kind != service.KindPublishScheduled {
		t.Fatalf("pending = %+v, %v", pending, err)
	}

	if n := fx.uploads.Load(); n != 0 {
		t.Errorf("%d requests left the machine through the tool", n)
	}
}

// T-161: what a person scheduled is out of reach of the tool. A person plans and schedules; the agent cannot
// cancel, send now, retry, change or approve it, and its reads change nothing.
func TestSecurity_T161_PublicationToolCannotCancelOrApproveWhatAPersonScheduled(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	d, cs := connectedMCP(t, fx)
	svc := d.service()
	person := service.WithCaller(
		callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	kids := false

	made, err := svc.PublicationCreate(person, service.PublicationCreateRequest{
		ProjectID: fx.project, Title: "Mine", Export: fx.export, Targets: []pub.TargetInput{{
			ConnectionID: fx.connection, At: "2026-12-01T09:00", Zone: "UTC", Fields: pub.Fields{MadeForKids: &kids},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduled, err := svc.PublicationSchedule(person, service.PublicationScheduleRequest{ID: made.ID})
	if err != nil || scheduled.Targets[0].State != "waiting_approval" {
		t.Fatalf("the person's schedule = %+v, %v", scheduled, err)
	}

	before, err := json.Marshal(scheduled)
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"cancel", "update", "send_now", "retry", "approve", "reject"} {
		args := map[string]any{"action": action, "id": made.ID, "target_id": scheduled.Targets[0].ID}
		if res, cerr := publicationToolOf(t, cs, args); !refused(res, cerr) {
			t.Errorf("the action %s answered %+v, want a refusal", action, res)
		}
	}

	for _, args := range []map[string]any{
		{"action": "show", "id": made.ID}, {"action": "list", "project_id": fx.project},
	} {
		if res, cerr := publicationToolOf(t, cs, args); refused(res, cerr) {
			t.Fatalf("%v: %v %+v", args, cerr, res)
		}
	}

	after, err := svc.PublicationGet(person, service.PublicationGetRequest{ID: made.ID})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(raw, before) {
		t.Errorf("the tool changed what the person scheduled:\n%s\n%s", before, raw)
	}

	pending, err := svc.ApprovalList(person, service.ApprovalListRequest{})
	if err != nil || len(pending.Requests) != 1 || pending.Requests[0].ID != scheduled.Targets[0].ApprovalID ||
		pending.Requests[0].State != "pending" {
		t.Fatalf("pending = %+v, %v", pending, err)
	}

	if n := fx.uploads.Load(); n != 0 {
		t.Errorf("%d requests left the machine", n)
	}

	// D-145: the action schedule cannot move a post a person approved, on any surface that is not the person's.
	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"mcp": newMCPWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run("reschedule over "+name, func(t *testing.T) {
			fx := newPublishFixture(t, newDriver)
			approved := approvedByAPerson(t, fx)

			raw, err := json.Marshal(service.PublicationScheduleRequest{
				ID: approved.ID, TargetID: approved.Targets[0].ID, At: "2027-06-01T09:00", Zone: "UTC",
			})
			if err != nil {
				t.Fatal(err)
			}

			res, _, fail := fx.d.do(t, step{Call: "PublicationSchedule", Args: raw})
			if fail == nil || fail.Code != service.CodePrivilegedSession {
				t.Fatalf("%s rescheduled what a person approved: %s %+v", name, res, fail)
			}

			stillApproved(t, fx, approved)
		})
	}

	t.Run("reschedule with an agent key", func(t *testing.T) {
		fx := newPublishFixture(t, newDirectWith)
		approved := approvedByAPerson(t, fx)
		agent := service.WithCaller(
			callerCtx(), service.Caller{Actor: "key:agent", Scopes: service.AllScopes},
		)

		_, err := fx.d.service().PublicationSchedule(agent, service.PublicationScheduleRequest{
			ID: approved.ID, TargetID: approved.Targets[0].ID, At: "2027-06-01T09:00", Zone: "UTC",
		})
		if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != service.CodePrivilegedSession ||
			se.Hint == "" {
			t.Fatalf("a key rescheduled what a person approved: %v", err)
		}

		stillApproved(t, fx, approved)
	})
}

// approvedByAPerson is a publication of one target that a person planned, scheduled and approved in the app.
func approvedByAPerson(t *testing.T, fx publishFixture) service.Publication {
	t.Helper()

	svc := fx.d.service()
	person := service.WithCaller(
		callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)
	kids := false

	made, err := svc.PublicationCreate(person, service.PublicationCreateRequest{
		ProjectID: fx.project, Title: "Approved", Export: fx.export, Targets: []pub.TargetInput{{
			ConnectionID: fx.connection, At: "2026-12-01T09:00", Zone: "UTC", Fields: pub.Fields{MadeForKids: &kids},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduled, err := svc.PublicationSchedule(person, service.PublicationScheduleRequest{ID: made.ID})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ApprovalDecide(person, service.ApprovalDecideRequest{
		ID: scheduled.Targets[0].ApprovalID, Decision: "approve",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.PublicationGet(person, service.PublicationGetRequest{ID: made.ID})
	if err != nil || !got.Targets[0].State.Approved() {
		t.Fatalf("the person's approved post = %+v, %v", got, err)
	}

	return *got
}

// stillApproved checks that the target is as the person left it: the same state, time and request, nothing sent.
func stillApproved(t *testing.T, fx publishFixture, want service.Publication) {
	t.Helper()

	got, err := fx.d.service().PublicationGet(callerCtx(), service.PublicationGetRequest{ID: want.ID})
	if err != nil {
		t.Fatal(err)
	}

	g, w := got.Targets[0], want.Targets[0]
	if g.State != w.State || g.ApprovalID != w.ApprovalID || !g.ScheduledAt.Equal(*w.ScheduledAt) {
		t.Errorf("the approved post changed: %+v, want %+v", g, w)
	}

	if n := fx.uploads.Load(); n != 0 {
		t.Errorf("%d requests left the machine", n)
	}
}

// D-145: a person in the app still moves a post they approved; it is asked for again with the new time.
func TestAPersonReschedulesWhatTheyApproved(t *testing.T) {
	fx := newPublishFixture(t, newDirectWith)
	approved := approvedByAPerson(t, fx)
	person := service.WithCaller(
		callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	moved, err := fx.d.service().PublicationSchedule(person, service.PublicationScheduleRequest{
		ID: approved.ID, TargetID: approved.Targets[0].ID, At: "2027-06-01T09:00", Zone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	tg := moved.Targets[0]
	if tg.State != "waiting_approval" || tg.ApprovalID == approved.Targets[0].ApprovalID ||
		tg.ScheduledAt.UTC().Format("2006-01-02T15:04") != "2027-06-01T09:00" {
		t.Errorf("the person's reschedule = %+v", tg)
	}
}

// T-162: the tool returns no credential. The connection holds a client secret and tokens in the vault; no answer
// of any action carries them, and no member of the tool takes a secret, a token or a destination.
func TestSecurity_T162_PublicationToolReturnsNoCredential(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	_, cs := connectedMCP(t, fx)

	p := planThroughTheTool(t, cs, fx)

	var seen []string

	for _, args := range []map[string]any{
		{"action": "show", "id": p.ID},
		{"action": "list", "project_id": fx.project},
	} {
		res, err := publicationToolOf(t, cs, args)
		if refused(res, err) {
			t.Fatalf("%v: %v %+v", args, err, res)
		}

		raw, merr := json.Marshal(res)
		if merr != nil {
			t.Fatal(merr)
		}

		seen = append(seen, string(raw))
	}

	for _, text := range seen {
		lower := strings.ToLower(text)
		for _, word := range []string{
			"secret", "access_token", "refresh_token", "client_secret", "bearer", "authorization", "password",
		} {
			if strings.Contains(lower, word) {
				t.Errorf("an answer of the tool contains %q: %s", word, text)
			}
		}
	}

	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, tool := range list.Tools {
		if tool.Name != "publication" {
			continue
		}

		found = true

		schema, merr := json.Marshal(tool.InputSchema)
		if merr != nil {
			t.Fatal(merr)
		}

		var in struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if uerr := json.Unmarshal(schema, &in); uerr != nil {
			t.Fatal(uerr)
		}

		for member := range in.Properties {
			for _, bad := range []string{"secret", "token", "credential", "password", "key", "url", "endpoint"} {
				if strings.Contains(member, bad) {
					t.Errorf("the publication tool has the member %s", member)
				}
			}
		}
	}

	if !found {
		t.Error("tools/list has no publication tool")
	}
}

// T-163: the tool holds the scopes and the project limit of the Service. A key with read alone reads and cannot
// plan or schedule; a key with edit and no render plans and cannot schedule; a key limited to another project
// finds nothing.
func TestSecurity_T163_PublicationToolHoldsTheScopesAndTheProjectLimit(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	d, _ := connectedMCP(t, fx)
	svc := d.service()
	person := service.WithCaller(
		callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	kids := false

	made, err := svc.PublicationCreate(person, service.PublicationCreateRequest{
		ProjectID: fx.project, Title: "Scoped", Export: fx.export, Targets: []pub.TargetInput{{
			ConnectionID: fx.connection, At: "2026-12-01T09:00", Zone: "UTC", Fields: pub.Fields{MadeForKids: &kids},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	other, err := testproject.CreateWithTimeline(callerCtx(), svc, "other")
	if err != nil {
		t.Fatal(err)
	}

	callers := map[string]service.Caller{
		"reader": {Actor: "key:reader", Scopes: []string{service.ScopeRead}},
		"editor": {Actor: "key:editor", Scopes: []string{service.ScopeRead, service.ScopeEdit}},
		"other":  {Actor: "key:other", Scopes: service.AllScopes, ProjectID: other.ProjectID},
	}

	handler, err := mcp.Handler(svc, mcp.Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, known := callers[r.Header.Get("X-Test-Caller")]
		if !known {
			http.Error(w, "no caller", http.StatusUnauthorized)

			return
		}

		handler.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), c)))
	}))
	t.Cleanup(srv.Close)

	session := func(who string) *sdk.ClientSession {
		hc := *srv.Client()
		hc.Transport = headerTransport{who: who, next: srv.Client().Transport}

		cs, cerr := sdk.NewClient(&sdk.Implementation{Name: who, Version: "1"}, nil).Connect(
			t.Context(), &sdk.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &hc}, nil,
		)
		if cerr != nil {
			t.Fatal(cerr)
		}

		t.Cleanup(func() { _ = cs.Close() })

		return cs
	}

	create := map[string]any{"action": "create", "project_id": fx.project, "title": "Nope", "export": fx.export}
	schedule := map[string]any{"action": "schedule", "id": made.ID}
	show := map[string]any{"action": "show", "id": made.ID}
	list := map[string]any{"action": "list", "project_id": fx.project}

	reader := session("reader")

	for _, args := range []map[string]any{show, list} {
		if res, cerr := publicationToolOf(t, reader, args); refused(res, cerr) {
			t.Errorf("reader %v: %v %+v", args, cerr, res)
		}
	}

	for _, args := range []map[string]any{create, schedule} {
		res, cerr := publicationToolOf(t, reader, args)
		if cerr != nil || errorCodeOf(t, res) != service.CodeForbidden {
			t.Errorf("reader %v answered %v %+v, want E_FORBIDDEN", args, cerr, res)
		}
	}

	editor := session("editor")

	if res, cerr := publicationToolOf(t, editor, create); refused(res, cerr) {
		t.Errorf("editor create: %v %+v", cerr, res)
	}

	res, cerr := publicationToolOf(t, editor, schedule)
	if cerr != nil || errorCodeOf(t, res) != service.CodeForbidden {
		t.Errorf("editor schedule answered %v %+v, want E_FORBIDDEN", cerr, res)
	}

	limited := session("other")

	for _, args := range []map[string]any{show, schedule, create} {
		res, cerr := publicationToolOf(t, limited, args)
		if cerr != nil || res == nil || !res.IsError {
			t.Errorf("the key of another project: %v answered %v %+v, want a refusal", args, cerr, res)

			continue
		}

		if code := errorCodeOf(t, res); code != command.CodeNotFound && code != service.CodeForbidden {
			t.Errorf("the key of another project: %v answered %s, want E_NOT_FOUND or E_FORBIDDEN", args, code)
		}
	}

	// Naming the project of the plan is refused; naming none lists what the key may see, which is nothing.
	res, cerr = publicationToolOf(t, limited, list)
	if cerr != nil || errorCodeOf(t, res) != command.CodeNotFound && errorCodeOf(t, res) != service.CodeForbidden {
		t.Errorf("the key of another project, list of %s: %v %+v, want a refusal", fx.project, cerr, res)
	}

	res, cerr = publicationToolOf(t, limited, map[string]any{"action": "list"})
	if refused(res, cerr) {
		t.Fatalf("the key of another project, list: %v %+v", cerr, res)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var listed service.PublicationListResult
	if err := json.Unmarshal(raw, &listed); err != nil || len(listed.Publications) != 0 {
		t.Errorf("the key of another project lists %s (%v), want none", raw, err)
	}

	after, err := svc.PublicationGet(person, service.PublicationGetRequest{ID: made.ID})
	if err != nil || after.Targets[0].State != "draft" {
		t.Errorf("after the refusals the target is %+v, %v, want a draft", after, err)
	}
}

// headerTransport names the caller of a request, as the guard of the API does after it reads a token.
type headerTransport struct {
	who  string
	next http.RoundTripper
}

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Test-Caller", h.who)

	return h.next.RoundTrip(r)
}
