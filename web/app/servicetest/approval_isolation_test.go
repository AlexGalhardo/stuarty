package servicetest

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

var memberScopes = []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender}

func networkPerson(user string, scopes []string, browser bool) context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "user:" + user, Scopes: scopes, OwnerUserID: user, Browser: browser,
	})
}

// costView is what a caller reads of the limits and the spending, as sorted lists of the projects they name; the
// empty string is the workspace.
type costView struct {
	ceilings, rules, today, days []string
	workspaceSpent               *int64
}

func (e *env) costView(t *testing.T, who context.Context) costView {
	t.Helper()

	sum, err := e.svc.CostSummary(who, service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	rules, err := e.svc.ApprovalRuleList(who, service.ApprovalRuleListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var v costView

	name := func(p *string) string {
		if p == nil {
			return ""
		}

		return *p
	}

	for _, c := range sum.Ceilings {
		v.ceilings = append(v.ceilings, name(c.ProjectID))
	}

	for _, r := range rules.Rules {
		v.rules = append(v.rules, name(r.ProjectID))
	}

	for _, l := range sum.Today {
		if l.Scope == "workspace" {
			m := l.Micros
			v.workspaceSpent = &m

			continue
		}

		v.today = append(v.today, fmt.Sprintf("%s=%d", l.ProjectID, l.Micros))
	}

	for _, d := range sum.Days {
		v.days = append(v.days, fmt.Sprintf("%s=%d", d.ProjectID, d.Micros))
	}

	for _, s := range [][]string{v.ceilings, v.rules, v.today, v.days} {
		slices.Sort(s)
	}

	return v
}

// VV-372 (SDD-015 section 4): in network mode a member reads the spending, the ceilings and the rules of their own
// projects and not of another user's, and no workspace-wide spending; an administrator reads everything.
func TestSecurity_T06_CostAndRulesAreScopedToTheOwner(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	ctx := context.Background()

	anaCtx := networkPerson("usr_0000000001", memberScopes, false)
	bobCtx := networkPerson("usr_0000000002", memberScopes, false)
	adminCtx := networkPerson("usr_0000000003", service.AllScopes, true)

	mine, err := testproject.Create(anaCtx, e.svc, "Ana project")
	if err != nil {
		t.Fatal(err)
	}

	theirs, err := testproject.Create(bobCtx, e.svc, "Bob project")
	if err != nil {
		t.Fatal(err)
	}

	day := approval.Day(e.clock.now())

	for i, l := range []struct {
		project string
		micros  int64
	}{{mine.ProjectID, 100}, {theirs.ProjectID, 7000}, {"", 90_000}} {
		if _, err := e.store.DB().ExecContext(ctx, `INSERT INTO cost_ledger (id, request_id, job_id, connection_id,
 project_id, kind, currency, estimate_micros, actual_micros, state, day, created_ms, settled_ms)
 VALUES (?, 'apr_h', ?, '', ?, 'text.generate', 'USD', ?, ?, 'settled', ?, 1, 1)`,
			fmt.Sprintf("led_isolate00%d", i), fmt.Sprintf("job_h%d", i), l.project, l.micros, l.micros, day); err != nil {
			t.Fatal(err)
		}
	}

	limit := int64(1_000_000)

	for _, project := range []string{"", mine.ProjectID, theirs.ProjectID} {
		if _, err := e.svc.CostCeilingSet(adminCtx, service.CostCeilingSetRequest{
			ProjectID: project, Currency: "USD", PerDayMicros: &limit,
		}); err != nil {
			t.Fatal(err)
		}

		if _, err := e.svc.ApprovalRuleSet(adminCtx, service.ApprovalRuleSetRequest{
			Kind: testKind, ProjectID: project, Currency: "USD", PerItemMicros: &limit,
		}); err != nil {
			t.Fatal(err)
		}
	}

	own := []string{"", mine.ProjectID}
	slices.Sort(own)

	got := e.costView(t, anaCtx)
	if !slices.Equal(got.ceilings, own) || !slices.Equal(got.rules, own) {
		t.Errorf("a member reads the ceilings %v and the rules %v, want %v", got.ceilings, got.rules, own)
	}

	want := []string{fmt.Sprintf("%s=100", mine.ProjectID)}
	if !slices.Equal(got.today, want) || !slices.Equal(got.days, want) {
		t.Errorf("a member reads the spending %v and %v, want %v", got.today, got.days, want)
	}

	if got.workspaceSpent != nil {
		t.Errorf("a member reads the spending of the workspace, %d", *got.workspaceSpent)
	}

	all := e.costView(t, adminCtx)
	if len(all.ceilings) != 3 || len(all.rules) != 3 || len(all.today) != 2 {
		t.Errorf("an administrator reads %+v, want every ceiling, rule and project", all)
	}

	if all.workspaceSpent == nil || *all.workspaceSpent != 97_100 {
		t.Errorf("the workspace line of an administrator = %v, want 97100", all.workspaceSpent)
	}

	// Another user's view is the mirror of it.
	other := e.costView(t, bobCtx)
	if len(other.today) != 1 || other.today[0] != fmt.Sprintf("%s=7000", theirs.ProjectID) {
		t.Errorf("the other member reads %v, want only their project", other.today)
	}

	// Local mode has no owner: every caller reads everything.
	local := e.costView(t, e.as(token))
	if len(local.today) != 2 || local.workspaceSpent == nil {
		t.Errorf("a local caller reads %+v, want everything", local)
	}
}
