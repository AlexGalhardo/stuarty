package servicetest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// A rule the person wrote approves a scheduled post without asking.
func TestApprovalRuleApprovesAScheduledPost(t *testing.T) {
	c := newCalendar(t)

	if _, err := c.svc.ApprovalRuleSet(c.browser(ui), service.ApprovalRuleSetRequest{
		Kind: service.KindPublishScheduled, ConnectionID: c.connID,
	}); err != nil {
		t.Fatal(err)
	}

	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	got := c.schedule(p, id)
	if got.Target(id).State != corepub.TargetScheduled {
		t.Fatalf("state = %s, want scheduled by the rule", got.Target(id).State)
	}

	pending, _ := c.svc.ApprovalList(c.as(token), service.ApprovalListRequest{})
	if len(pending.Requests) != 0 {
		t.Errorf("%d requests wait for a person", len(pending.Requests))
	}

	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 || c.state(p, id) != corepub.TargetPublished {
		t.Fatalf("started %d, state %s", n, c.state(p, id))
	}
}

// The cost of a post is in the approval and counts against the ceilings; the ledger is settled when it goes.
func TestCostOfAScheduledPostCountsAgainstTheCeilings(t *testing.T) {
	c := newCalendar(t)
	c.conn.CostMicros, c.conn.CostCurrency = 500000, "USD"

	p := c.create(c.target(c.at(time.Hour)), c.target(c.at(2*time.Hour)))
	a, b := p.Targets[0].ID, p.Targets[1].ID

	perItem, perDay := int64(600000), int64(800000)
	if _, err := c.svc.CostCeilingSet(c.browser(ui), service.CostCeilingSetRequest{
		Currency: "USD", PerItemMicros: &perItem, PerDayMicros: &perDay,
	}); err != nil {
		t.Fatal(err)
	}

	c.schedule(p, a)

	req, err := c.svc.ApprovalGet(c.as(token), service.ApprovalRef{ID: c.get(p).Target(a).ApprovalID})
	if err != nil || req.Estimate.Micros != 500000 || req.Estimate.Currency != "USD" {
		t.Fatalf("request = %+v, %v", req, err)
	}

	c.approve(p, a)

	// The first post reserved half a dollar of the day: the second would be over the ceiling of the day.
	_, err = c.svc.PublicationSchedule(c.as(token), service.PublicationScheduleRequest{ID: p.ID, TargetID: b})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeCostCeiling {
		t.Fatalf("the second post = %v, want E_COST_CEILING", err)
	}

	if c.state(p, b) != corepub.TargetDraft {
		t.Errorf("a refused post is %s", c.state(p, b))
	}

	// Cancelling the first gives its half dollar back.
	if _, err := c.svc.PublicationCancel(
		c.as(token),
		service.PublicationCancelRequest{ID: p.ID, TargetID: a},
	); err != nil {
		t.Fatal(err)
	}

	c.schedule(p, b)
	c.approve(p, b)
	c.clock.advance(2 * time.Hour)

	if n := c.tick(); n != 1 || c.state(p, b) != corepub.TargetPublished {
		t.Fatalf("started %d, state %s", n, c.state(p, b))
	}

	sum, err := c.svc.CostSummary(c.as(token), service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	spent := int64(0)
	for _, l := range sum.Today {
		if l.Scope == "workspace" {
			spent += l.Micros
		}
	}

	if spent != 500000 {
		t.Errorf("spent today = %d, want 500000: %+v", spent, sum.Today)
	}
}

// An agent plans and schedules, and cannot approve: nothing leaves the machine (T-48).
func TestSecurityPublicationAgentCannotApproveASend(t *testing.T) {
	c := newCalendar(t)
	agentCtx := c.as(agent)

	p, err := c.svc.PublicationCreate(agentCtx, service.PublicationCreateRequest{
		ProjectID: c.project, Title: "Agent plan", Export: c.exp, Targets: []pub.TargetInput{c.target(c.at(time.Hour))},
	})
	if err != nil {
		t.Fatal(err)
	}

	id := p.Targets[0].ID

	got, err := c.svc.PublicationSchedule(agentCtx, service.PublicationScheduleRequest{ID: p.ID, TargetID: id})
	if err != nil || got.Target(id).State != corepub.TargetWaitingApproval {
		t.Fatalf("schedule: %+v, %v", got, err)
	}

	for _, ctx := range []context.Context{agentCtx, c.as(token)} {
		_, err = c.svc.ApprovalDecide(
			ctx,
			service.ApprovalDecideRequest{ID: got.Target(id).ApprovalID, Decision: "approve"},
		)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != service.CodePrivilegedSession {
			t.Fatalf("an agent decided: %v", err)
		}
	}

	c.clock.advance(5 * time.Hour)

	if n := c.tick(); n != 0 || uploads(c.net) != 0 {
		t.Fatalf("an unapproved post went: %d uploads", uploads(c.net))
	}

	// Send now, retry and reschedule of an agent ask again, and send nothing either.
	if _, err := c.svc.PublicationSendNow(
		agentCtx,
		service.PublicationSendNowRequest{ID: p.ID, TargetID: id},
	); err != nil {
		t.Fatal(err)
	}

	if n := c.tick(); n != 0 || uploads(c.net) != 0 {
		t.Fatalf("send now of an agent went without approval: %d uploads", uploads(c.net))
	}
}

// What is sent is what was approved: a file that changed after the approval is never sent, and a snapshot
// that does not match its digest is not either.
func TestSecurityPublicationSendsExactlyWhatWasApproved(t *testing.T) {
	for name, tamper := range map[string]func(c *calendar, p *service.Publication, id string){
		"the file changes": func(c *calendar, _ *service.Publication, _ string) {
			f := filepath.Join(string(c.dir), "exports", c.project, "job_0000000801.mp4")
			if err := os.WriteFile(f, []byte("a different video!"), 0o600); err != nil {
				c.t.Fatal(err)
			}
		},
		"the snapshot changes": func(c *calendar, _ *service.Publication, id string) {
			_, err := c.store.DB().ExecContext(context.Background(),
				`UPDATE publication_targets SET snapshot = replace(snapshot, 'Week one', 'Something else') WHERE id = ?`, id)
			if err != nil {
				c.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newCalendar(t)
			p := c.create(c.target(c.at(time.Hour)))
			id := p.Targets[0].ID

			c.schedule(p, id)
			c.approve(p, id)
			tamper(c, p, id)
			c.clock.advance(time.Hour)
			c.tick()

			got := c.get(p).Target(id)
			if got.State != corepub.TargetFailed || uploads(c.net) != 0 || got.Error == nil ||
				got.Error.Kind != "integrity" {
				t.Fatalf("target = %+v, error = %+v, %d uploads", got, got.Error, uploads(c.net))
			}
		})
	}
}

// A key limited to another project does not see, change or schedule a publication.
func TestSecurityPublicationOfAnotherProjectIsNotFound(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	other := c.newProject("another")

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:other", Scopes: service.AllScopes, ProjectID: other,
	})

	checks := map[string]func() error{
		"get": func() error {
			_, err := c.svc.PublicationGet(limited, service.PublicationGetRequest{ID: p.ID})

			return err
		},
		"schedule": func() error {
			_, err := c.svc.PublicationSchedule(
				limited,
				service.PublicationScheduleRequest{ID: p.ID, TargetID: p.Targets[0].ID},
			)

			return err
		},
		"cancel": func() error {
			_, err := c.svc.PublicationCancel(limited, service.PublicationCancelRequest{ID: p.ID})

			return err
		},
		"create": func() error {
			_, err := c.svc.PublicationCreate(
				limited,
				service.PublicationCreateRequest{ProjectID: c.project, Title: "x", Export: c.exp},
			)

			return err
		},
	}

	for name, run := range checks {
		var se *service.Error
		if err := run(); !errors.As(err, &se) || (se.Code != "E_NOT_FOUND" && se.Code != service.CodeForbidden) {
			t.Errorf("%s = %v", name, err)
		}
	}

	list, err := c.svc.PublicationList(limited, service.PublicationListRequest{})
	if err != nil || len(list.Publications) != 0 {
		t.Errorf("list = %+v, %v", list, err)
	}

	// A key limited to a scope without render cannot ask for a send.
	reader := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead}},
	)
	if _, err := c.svc.PublicationSchedule(
		reader,
		service.PublicationScheduleRequest{ID: p.ID, TargetID: p.Targets[0].ID},
	); err == nil {
		t.Error("a read key scheduled a post")
	}
}

// A request nobody answered in a week expires, and the target goes back to a draft that says so: the scheduler
// sees to it, so the calendar never shows a post waiting for an answer that cannot come.
func TestApprovalThatExpiresReturnsTheTargetToADraft(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	c.schedule(p, id)
	c.clock.advance(8 * 24 * time.Hour)
	c.tick()

	got := c.get(p).Target(id)
	if got.State != corepub.TargetDraft || got.Error == nil || got.Error.Code != "approval_expired" ||
		got.Error.Hint == "" ||
		got.ApprovalID != "" ||
		uploads(c.net) != 0 {
		t.Fatalf("target = %+v, error = %+v", got, got.Error)
	}

	// It can be planned again with a time that has not come.
	if _, err := c.svc.PublicationSchedule(c.as(token), service.PublicationScheduleRequest{
		ID: p.ID, TargetID: id, At: c.at(time.Hour), Zone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}

	if c.state(p, id) != corepub.TargetWaitingApproval {
		t.Errorf("state = %s", c.state(p, id))
	}
}

// VV-325: what a person reads in the approval queue names the account, never the id of the connection.
func TestApprovalSummaryOfAPostHasNoConnectionID(t *testing.T) {
	c := newCalendar(t)

	p, err := c.svc.PublicationCreate(c.as(agent), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "Plan", Export: c.exp, Targets: []pub.TargetInput{c.target(c.at(time.Hour))},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.svc.PublicationSchedule(c.as(agent), service.PublicationScheduleRequest{
		ID: p.ID, TargetID: p.Targets[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := c.svc.ApprovalGet(c.as(token), service.ApprovalRef{ID: got.Target(p.Targets[0].ID).ApprovalID})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(req.Summary, "con_") || !strings.Contains(req.Summary, "Fake channel") {
		t.Errorf("the summary = %q, want the account and no connection id", req.Summary)
	}
}
