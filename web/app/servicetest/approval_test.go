package servicetest

import (
	"context"
	"strings"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

const (
	token = "local:claude-code" // the local token with a label an agent chose
	key   = "key:key_0000000042"
	owner = "usr_0000000001"
)

// asPerson is a person in a privileged browser session: the cookie of the launch flow (SDD-007 3.4).
func (e *env) asPerson() context.Context {
	return service.WithCaller(
		context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes, Browser: true},
	)
}

// asKey is an API key that holds every scope: a key is an agent credential and never a person.
func (e *env) asKey() context.Context {
	return service.WithCaller(
		context.Background(), service.Caller{Actor: key, Scopes: service.AllScopes, OwnerUserID: owner},
	)
}

// addConnection makes a connection row; the approval queue needs only that it exists.
func (e *env) addConnection(id string) {
	e.t.Helper()

	err := e.store.Connections().Insert(context.Background(), connections.Row{
		ID: id, Kind: "test", Label: "x", Endpoint: "https://example.org/", Hosts: []string{"example.org"},
		Accepts: []string{"text"}, SecretRef: id, CreatedMs: e.clock.now().UnixMilli(), CreatedBy: ui,
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// addRequest writes a pending request the way a submission does, with no job.
func (e *env) addRequest() approval.Row {
	e.t.Helper()

	return e.addRequestFor("", "")
}

// addRequestFor is addRequest for a user (network mode) and a project.
func (e *env) addRequestFor(user, project string) approval.Row {
	e.t.Helper()

	now := e.clock.now().UnixMilli()
	row := approval.Row{
		ID: e.newID("apr"), Actor: token, OwnerUserID: user, Kind: "publish.post", Summary: "Post the launch video",
		ConnectionID: "", ProjectID: project, PayloadRef: "draft:1", Estimate: core.Money{},
		State: approval.StatePending, CreatedMs: now, ExpiresMs: now + approval.ExpireAfter.Milliseconds(),
	}

	if err := e.store.Approvals().InsertRequest(context.Background(), &row); err != nil {
		e.t.Fatal(err)
	}

	return row
}

// The four operations for a person refuse everyone else with one stable code (SDD-015 section 4,
// acceptance criterion 2 of epic 05), whatever scope or label the caller holds.
func TestApprovalPrivilegedOperationsRefuseTheLocalToken(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	req := e.addRequest()

	calls := map[string]func(ctx context.Context) error{
		"ApprovalDecide": func(ctx context.Context) error {
			_, err := e.svc.ApprovalDecide(ctx, service.ApprovalDecideRequest{ID: req.ID, Decision: "approve"})
			return err
		},
		"ApprovalRuleSet": func(ctx context.Context) error {
			_, err := e.svc.ApprovalRuleSet(ctx, service.ApprovalRuleSetRequest{Kind: "publish.post"})
			return err
		},
		"ApprovalRuleRemove": func(ctx context.Context) error {
			_, err := e.svc.ApprovalRuleRemove(ctx, service.ApprovalRef{ID: "rul_0000000001"})
			return err
		},
		"CostCeilingSet": func(ctx context.Context) error {
			_, err := e.svc.CostCeilingSet(ctx, service.CostCeilingSetRequest{Currency: "USD"})
			return err
		},
	}

	callers := map[string]context.Context{
		"the local token":                         e.as(token),
		"the token with the label of the browser": e.as(ui),
		"an API key with every scope":             e.asKey(),
		"a read-only key": service.WithCaller(
			context.Background(), service.Caller{Actor: key, Scopes: []string{service.ScopeRead}},
		),
	}

	for name, call := range calls {
		for who, ctx := range callers {
			se := serviceError(t, call(ctx))
			if se.Code != service.CodePrivilegedSession || se.HTTPStatus() != 403 {
				t.Errorf("%s by %s = %s (%d), want E_PRIVILEGED_SESSION 403", name, who, se.Code, se.HTTPStatus())
			}

			if se.Hint == "" {
				t.Errorf("%s by %s: the refusal has no hint", name, who)
			}
		}
	}

	// The request is untouched: nothing a refused call did stuck.
	got, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: req.ID})
	if err != nil || got.State != approval.StatePending {
		t.Fatalf("the request after the refusals = %+v, %v", got, err)
	}

	if rules, err := e.svc.ApprovalRuleList(
		e.as(token),
		service.ApprovalRuleListRequest{},
	); err != nil ||
		len(rules.Rules) != 0 {
		t.Fatalf("a refused caller wrote a rule: %+v, %v", rules, err)
	}
}

func TestApprovalReadsAreOpenToEveryCallerThatMayRead(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	req := e.addRequest()

	list, err := e.svc.ApprovalList(e.as(token), service.ApprovalListRequest{})
	if err != nil || list.Total != 1 || list.Requests[0].ID != req.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if list.UntrustedNotice == "" || !strings.Contains(list.Text, "<untrusted_media_text") {
		t.Errorf("the summary of a request is not delimited as untrusted text:\n%s", list.Text)
	}

	one, err := e.svc.ApprovalGet(e.asKey(), service.ApprovalRef{ID: req.ID})
	if err != nil || one.Summary != "Post the launch video" || one.State != approval.StatePending {
		t.Fatalf("get = %+v, %v", one, err)
	}

	if _, err := e.svc.ApprovalGet(
		e.as(token),
		service.ApprovalRef{ID: "apr_9999999999"},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("an unknown request = %v, want E_APPROVAL_NOT_FOUND", err)
	}

	if _, err := e.svc.ApprovalList(e.as(token), service.ApprovalListRequest{State: "x"}); code(err) != "E_VALIDATION" {
		t.Errorf("a bad state = %v, want E_VALIDATION", err)
	}

	if _, err := e.svc.CostSummary(e.as(token), service.CostSummaryRequest{}); err != nil {
		t.Errorf("CostSummary: %v", err)
	}
}

func TestApprovalDecide(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.asPerson()

	approved := e.addRequest()
	rejected := e.addRequest()

	got, err := e.svc.ApprovalDecide(ctx, service.ApprovalDecideRequest{ID: approved.ID, Decision: "approve"})
	if err != nil || got.State != approval.StateApproved || got.DecidedBy == nil || *got.DecidedBy != ui ||
		got.DecidedAt == nil || got.RuleID != nil {
		t.Fatalf("approve = %+v, %v", got, err)
	}

	got, err = e.svc.ApprovalDecide(
		ctx,
		service.ApprovalDecideRequest{ID: rejected.ID, Decision: "reject", Reason: "wrong channel"},
	)
	if err != nil || got.State != approval.StateRejected || got.Reason != "wrong channel" {
		t.Fatalf("reject = %+v, %v", got, err)
	}

	// A request is decided once.
	for _, d := range []string{"approve", "reject"} {
		_, err = e.svc.ApprovalDecide(ctx, service.ApprovalDecideRequest{ID: approved.ID, Decision: d})
		if code(err) != "E_INVALID_STATE" {
			t.Errorf("%s of a decided request = %v, want E_INVALID_STATE", d, err)
		}
	}

	bad := map[string]service.ApprovalDecideRequest{
		"a decision that is neither": {ID: approved.ID, Decision: "maybe"},
		"no decision":                {ID: approved.ID},
		"a reason that hides text":   {ID: approved.ID, Decision: "reject", Reason: "ok" + string(rune(0x202e))},
	}
	for name, req := range bad {
		if _, err := e.svc.ApprovalDecide(ctx, req); code(err) != "E_VALIDATION" {
			t.Errorf("%s = %v, want E_VALIDATION", name, err)
		}
	}

	if _, err := e.svc.ApprovalDecide(
		ctx,
		service.ApprovalDecideRequest{ID: "apr_9999999999", Decision: "approve"},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("an unknown request = %v, want E_APPROVAL_NOT_FOUND", err)
	}

	pending, err := e.svc.ApprovalList(ctx, service.ApprovalListRequest{})
	if err != nil || pending.Total != 0 {
		t.Errorf("pending after the decisions = %+v, %v", pending, err)
	}

	all, err := e.svc.ApprovalList(ctx, service.ApprovalListRequest{State: "all"})
	if err != nil || all.Total != 2 {
		t.Errorf("all = %+v, %v", all, err)
	}

	audit := e.audits("approval.approve", "approval.reject")
	if len(audit) != 2 {
		t.Errorf("audit rows = %d, want 2", len(audit))
	}
}

// Two people deciding at once decide once.
func TestApprovalDecideOnceUnderRace(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	req := e.addRequest()

	results := make(chan error, 8)

	for range 8 {
		go func() {
			_, err := e.svc.ApprovalDecide(e.asPerson(), service.ApprovalDecideRequest{ID: req.ID, Decision: "approve"})
			results <- err
		}()
	}

	won := 0

	for range 8 {
		if err := <-results; err == nil {
			won++
		} else if code(err) != "E_INVALID_STATE" {
			t.Errorf("a losing decision = %v", err)
		}
	}

	if won != 1 {
		t.Errorf("%d decisions won, want 1", won)
	}
}

func TestApprovalRequestExpires(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	req := e.addRequest()

	e.clock.advance(approval.ExpireAfter + 1)

	got, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: req.ID})
	if err != nil || got.State != approval.StateExpired {
		t.Fatalf("after the time = %+v, %v", got, err)
	}

	if _, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: req.ID, Decision: "approve"},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("approving an expired request = %v, want E_INVALID_STATE", err)
	}

	if n := len(e.audits("approval.expire")); n != 1 {
		t.Errorf("expire audit rows = %d, want 1", n)
	}

	if evs := e.eventsOf(
		service.EventApprovalDecided,
	); len(evs) != 1 ||
		evs[0].Approval.State != approval.StateExpired {
		t.Errorf("events = %+v", evs)
	}
}

func TestApprovalVisibilityInNetworkMode(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	proj := e.newProject("p")
	mine := e.addRequestFor(owner, proj)
	other := e.addRequestFor("usr_0000000002", "")

	user := func(scopes ...string) context.Context {
		return service.WithCaller(context.Background(), service.Caller{
			Actor: "user:" + owner, Scopes: scopes, OwnerUserID: owner, Browser: true,
		})
	}
	member := user(service.ScopeRead, service.ScopeEdit)
	admin := user(service.AllScopes...)

	// A member reads and decides the requests of their own user, and sees nothing of another's.
	list, err := e.svc.ApprovalList(member, service.ApprovalListRequest{})
	if err != nil || list.Total != 1 || list.Requests[0].ID != mine.ID {
		t.Fatalf("a member lists %+v, %v", list, err)
	}

	if _, err := e.svc.ApprovalGet(
		member,
		service.ApprovalRef{ID: other.ID},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("a member reads another user request = %v, want E_APPROVAL_NOT_FOUND", err)
	}

	if _, err := e.svc.ApprovalDecide(
		member,
		service.ApprovalDecideRequest{ID: other.ID, Decision: "approve"},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("a member decides another user request = %v, want E_APPROVAL_NOT_FOUND", err)
	}

	if got, err := e.svc.ApprovalDecide(
		member,
		service.ApprovalDecideRequest{ID: mine.ID, Decision: "approve"},
	); err != nil ||
		got.State != approval.StateApproved {
		t.Errorf("a member decides the own request = %+v, %v", got, err)
	}

	// An administrator reads and decides every request.
	if list, err = e.svc.ApprovalList(admin, service.ApprovalListRequest{State: "all"}); err != nil || list.Total != 2 {
		t.Errorf("an administrator lists %+v, %v", list, err)
	}

	if _, err := e.svc.ApprovalDecide(
		admin,
		service.ApprovalDecideRequest{ID: other.ID, Decision: "reject"},
	); err != nil {
		t.Errorf("an administrator decides: %v", err)
	}

	// A key limited to one project reads only that project's requests.
	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: key, Scopes: service.AllScopes, ProjectID: proj},
	)
	if list, err = e.svc.ApprovalList(
		limited,
		service.ApprovalListRequest{State: "all"},
	); err != nil ||
		list.Total != 1 ||
		list.Requests[0].ID != mine.ID {
		t.Errorf("a project key lists %+v, %v", list, err)
	}

	if _, err := e.svc.ApprovalGet(
		limited,
		service.ApprovalRef{ID: other.ID},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("a project key reads a request of no project = %v", err)
	}
}

func TestApprovalRulesLifecycle(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.asPerson()
	e.addConnection("con_0000000001")

	made, err := e.svc.ApprovalRuleSet(ctx, service.ApprovalRuleSetRequest{
		Kind: "publish.post", ConnectionID: "con_0000000001",
	})
	if err != nil || made.ID == "" || !made.Enabled || made.ConnectionID == nil || made.CreatedBy != ui {
		t.Fatalf("create = %+v, %v", made, err)
	}

	off := false

	replaced, err := e.svc.ApprovalRuleSet(
		ctx,
		service.ApprovalRuleSetRequest{ID: made.ID, Kind: "publish.story", Enabled: &off},
	)
	if err != nil || replaced.ID != made.ID || replaced.Enabled || replaced.Kind != "publish.story" ||
		replaced.ConnectionID != nil ||
		!replaced.CreatedAt.Equal(made.CreatedAt) {
		t.Fatalf("replace = %+v, %v", replaced, err)
	}

	list, err := e.svc.ApprovalRuleList(e.as(token), service.ApprovalRuleListRequest{})
	if err != nil || len(list.Rules) != 1 || list.Rules[0].Kind != "publish.story" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	micros := int64(500_000)
	bad := map[string]service.ApprovalRuleSetRequest{
		"no kind":                     {},
		"a kind with a space":         {Kind: "a b"},
		"a limit, no currency":        {Kind: "k", PerItemMicros: &micros},
		"a connection id that is not": {Kind: "k", ConnectionID: "x"},
		"a connection that is gone":   {Kind: "k", ConnectionID: "con_0000000099"},
		"a rule that does not exist":  {ID: "rul_9999999999", Kind: "k"},
	}

	for name, req := range bad {
		err := code(callRuleSet(ctx, e, req))
		if err != "E_VALIDATION" && (name != "a rule that does not exist" || err != service.CodeApprovalNotFound) {
			t.Errorf("%s = %s", name, err)
		}
	}

	if _, err := e.svc.ApprovalRuleRemove(ctx, service.ApprovalRef{ID: made.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ApprovalRuleRemove(
		ctx,
		service.ApprovalRef{ID: made.ID},
	); code(
		err,
	) != service.CodeApprovalNotFound {
		t.Errorf("removing twice = %v, want E_APPROVAL_NOT_FOUND", err)
	}

	if n := len(e.audits("approval.rule.set", "approval.rule.remove")); n != 3 {
		t.Errorf("rule audit rows = %d, want 3", n)
	}
}

func callRuleSet(ctx context.Context, e *env, req service.ApprovalRuleSetRequest) error {
	_, err := e.svc.ApprovalRuleSet(ctx, req)

	return err
}

func TestApprovalRulesAreLimited(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	for range core.MaxRules {
		if _, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{Kind: "k"}); err != nil {
			t.Fatal(err)
		}
	}

	if err := callRuleSet(e.asPerson(), e, service.ApprovalRuleSetRequest{Kind: "k"}); code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("the 101st rule = %v, want E_LIMIT_EXCEEDED", err)
	}
}

func TestCostCeilingSetAndSummary(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.asPerson()
	perDay := int64(2_000_000)

	sum, err := e.svc.CostCeilingSet(ctx, service.CostCeilingSetRequest{Currency: "USD", PerDayMicros: &perDay})
	if err != nil || len(sum.Ceilings) != 1 || sum.Ceilings[0].PerDay == nil || *sum.Ceilings[0].PerDay != perDay ||
		sum.Ceilings[0].ProjectID != nil || sum.Ceilings[0].UpdatedBy != ui {
		t.Fatalf("set = %+v, %v", sum, err)
	}

	proj := e.newProject("p")

	perItem := int64(1)
	if sum, err = e.svc.CostCeilingSet(
		ctx,
		service.CostCeilingSetRequest{ProjectID: proj, Currency: "USD", PerItemMicros: &perItem},
	); err != nil ||
		len(sum.Ceilings) != 2 {
		t.Fatalf("project ceiling = %+v, %v", sum, err)
	}

	// Setting again replaces; both limits empty removes.
	perDay = 3_000_000
	if sum, err = e.svc.CostCeilingSet(
		ctx,
		service.CostCeilingSetRequest{Currency: "USD", PerDayMicros: &perDay},
	); err != nil ||
		len(sum.Ceilings) != 2 {
		t.Fatalf("replace = %+v, %v", sum, err)
	}

	if sum, err = e.svc.CostCeilingSet(
		ctx,
		service.CostCeilingSetRequest{ProjectID: proj, Currency: "USD"},
	); err != nil ||
		len(sum.Ceilings) != 1 {
		t.Fatalf("remove = %+v, %v", sum, err)
	}

	for name, req := range map[string]service.CostCeilingSetRequest{
		"no currency":       {PerDayMicros: &perDay},
		"a negative limit":  {Currency: "USD", PerDayMicros: new(int64(-1))},
		"a bad project":     {ProjectID: "x", Currency: "USD", PerDayMicros: &perDay},
		"a lower-case code": {Currency: "usd", PerDayMicros: &perDay},
	} {
		if _, err := e.svc.CostCeilingSet(ctx, req); code(err) != "E_VALIDATION" {
			t.Errorf("%s = %v, want E_VALIDATION", name, err)
		}
	}

	if n := len(e.audits("approval.ceiling.set")); n != 4 {
		t.Errorf("ceiling audit rows = %d, want 4", n)
	}

	// A key limited to one project reads no costs and no rules.
	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: key, Scopes: service.AllScopes, ProjectID: proj},
	)
	if _, err := e.svc.CostSummary(limited, service.CostSummaryRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("CostSummary by a project key = %v, want E_FORBIDDEN", err)
	}

	if _, err := e.svc.ApprovalRuleList(
		limited,
		service.ApprovalRuleListRequest{},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("ApprovalRuleList by a project key = %v, want E_FORBIDDEN", err)
	}
}

// audits lists the audit rows of the actions.
func (e *env) audits(actions ...string) []service.AuditEntry {
	e.t.Helper()

	res, err := e.svc.AuditList(e.as(ui), service.AuditListRequest{Limit: 200})
	if err != nil {
		e.t.Fatal(err)
	}

	var out []service.AuditEntry

	for i := range res.Entries {
		for _, a := range actions {
			if res.Entries[i].Action == a {
				out = append(out, res.Entries[i])
			}
		}
	}

	return out
}
