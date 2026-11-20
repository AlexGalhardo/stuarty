package servicetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// uploads is how many posts the fake network was sent.
func uploads(n *fakeNet) int {
	count := 0

	for _, r := range n.seen() {
		if r == "POST /videos" {
			count++
		}
	}

	return count
}

// calendar is a pubEnv with the scheduler of its service in hand.
type calendar struct {
	*pubEnv
	sched *pub.Scheduler
	exp   string
}

func newCalendar(t *testing.T) *calendar {
	t.Helper()

	p := newPubEnv(t)
	sched, ok := service.SchedulerOf(p.svc)

	if !ok {
		t.Fatal("the service has no scheduler")
	}

	return &calendar{
		pubEnv: p,
		sched:  sched,
		exp:    p.seedExport(p.project, "job_0000000801", ".mp4", []byte("fake mp4 bytes")),
	}
}

func (c *calendar) at(d time.Duration) string {
	return c.clock.now().Add(d).UTC().Format("2006-01-02T15:04")
}

func (c *calendar) create(targets ...pub.TargetInput) *service.Publication {
	c.t.Helper()

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "Week one", Text: "A caption", Export: c.exp, Targets: targets,
	})
	if err != nil {
		c.t.Fatal(err)
	}

	return p
}

func (c *calendar) target(at string) pub.TargetInput {
	return pub.TargetInput{
		ConnectionID: c.connID, At: at, Zone: "UTC", Fields: pub.Fields{MadeForKids: new(false)},
	}
}

func (c *calendar) get(p *service.Publication) *service.Publication {
	c.t.Helper()

	got, err := c.svc.PublicationGet(c.as(token), service.PublicationGetRequest{ID: p.ID})
	if err != nil {
		c.t.Fatal(err)
	}

	return got
}

func (c *calendar) schedule(p *service.Publication, targetID string) *service.Publication {
	c.t.Helper()

	got, err := c.svc.PublicationSchedule(c.as(token), service.PublicationScheduleRequest{ID: p.ID, TargetID: targetID})
	if err != nil {
		c.t.Fatal(err)
	}

	return got
}

// approve is the person at the Approvals page: it approves the request the target waits on.
func (c *calendar) approve(p *service.Publication, targetID string) {
	c.t.Helper()

	t := c.get(p).Target(targetID)
	if t == nil || t.ApprovalID == "" {
		c.t.Fatalf("target %s has no request: %+v", targetID, t)
	}

	if _, err := c.svc.ApprovalDecide(
		c.browser(ui), service.ApprovalDecideRequest{ID: t.ApprovalID, Decision: "approve"},
	); err != nil {
		c.t.Fatal(err)
	}
}

// tick runs one pass of the scheduler and waits for the sends it started.
func (c *calendar) tick() int {
	c.t.Helper()

	n, err := c.sched.Tick(context.Background())
	if err != nil {
		c.t.Fatal(err)
	}

	c.sched.Wait()

	return n
}

func (c *calendar) state(p *service.Publication, targetID string) corepub.TargetState {
	c.t.Helper()

	return c.get(p).Target(targetID).State
}

// Acceptance criterion 1 of epic 20: a publication with two targets scheduled at different times publishes
// each at its time, once, after approval.
func TestSchedulerPublishesEachTargetAtItsTimeOnceAfterApproval(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)), c.target(c.at(2*time.Hour)))
	first, second := p.Targets[0].ID, p.Targets[1].ID

	c.schedule(p, first)
	c.schedule(p, second)

	// Nothing waits on its own: both ask a person, and a pass of the scheduler sends nothing.
	if got := c.get(p); got.Target(first).State != corepub.TargetWaitingApproval ||
		got.Target(second).State != corepub.TargetWaitingApproval {
		t.Fatalf("states = %s, %s", got.Target(first).State, got.Target(second).State)
	}

	pending, err := c.svc.ApprovalList(c.as(token), service.ApprovalListRequest{})
	if err != nil || len(pending.Requests) != 2 || pending.Requests[0].Kind != service.KindPublishScheduled {
		t.Fatalf("pending = %+v, %v", pending, err)
	}

	for _, r := range pending.Requests {
		for _, want := range []string{"Week one", "Fake network", "Fake channel", "private", "UTC", "job_0000000801.mp4", "14 bytes"} {
			if !strings.Contains(r.Summary, want) {
				t.Errorf("the summary a person reads lacks %q: %s", want, r.Summary)
			}
		}
	}

	c.clock.advance(3 * time.Hour)

	if n := c.tick(); n != 0 || uploads(c.net) != 0 {
		t.Fatalf("a post left before the approval: %d started, %d uploads", n, uploads(c.net))
	}

	c.clock.advance(-3 * time.Hour)
	c.approve(p, first)
	c.approve(p, second)

	if c.state(p, first) != corepub.TargetScheduled || c.state(p, second) != corepub.TargetScheduled {
		t.Fatalf("states = %s, %s, want scheduled", c.state(p, first), c.state(p, second))
	}

	// Before the first time: nothing.
	if n := c.tick(); n != 0 || uploads(c.net) != 0 {
		t.Fatalf("a post left early: %d uploads", uploads(c.net))
	}

	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d sends, want 1", n)
	}

	got := c.get(p)
	if got.Target(first).State != corepub.TargetPublished || got.Target(second).State != corepub.TargetScheduled ||
		uploads(c.net) != 1 {
		t.Fatalf(
			"after the first time: %s, %s, %d uploads",
			got.Target(first).State,
			got.Target(second).State,
			uploads(c.net),
		)
	}

	if f := got.Target(
		first,
	); f.URL == "" || f.RemoteID == "" || f.PublishedAt == nil || f.Attempts != 1 ||
		f.Error != nil {
		t.Errorf("first target = %+v", f)
	}

	// More passes at the same time send nothing again.
	if n := c.tick(); n != 0 || uploads(c.net) != 1 {
		t.Errorf("a second pass sent: %d started, %d uploads", n, uploads(c.net))
	}

	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 || c.state(p, second) != corepub.TargetPublished {
		t.Fatalf("second time: %d started, state %s", n, c.state(p, second))
	}

	if uploads(c.net) != 2 || len(c.conn.Uploaded()) != 2 {
		t.Errorf("uploads = %d, titles = %v", uploads(c.net), c.conn.Uploaded())
	}

	if n := c.tick(); n != 0 || uploads(c.net) != 2 {
		t.Errorf("a pass after both sent sent again: %d uploads", uploads(c.net))
	}

	// The updates went out as publication.updated events.
	c.mu.Lock()
	defer c.mu.Unlock()

	updated := 0

	for i := range c.events {
		if c.events[i].Type == service.EventPublicationUpdated && c.events[i].Publication != nil {
			updated++
		}
	}

	if updated < 6 {
		t.Errorf("%d publication.updated events", updated)
	}
}

// Acceptance criterion 2: two servers on the same workspace publish a target exactly once. The second server
// opens the database of the workspace on its own, as a second process does, so the two claims race on the
// file and not inside one connection pool.
func TestSchedulerTwoServersOnTheSameWorkspacePublishOnce(t *testing.T) {
	var saved service.Options

	e := newSocialEnvWith(t, func(o *service.Options) { saved = *o })

	own, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = own.Close() })

	saved.Store = own

	second, err := service.New(saved)
	if err != nil {
		t.Fatal(err)
	}

	s1, _ := service.SchedulerOf(e.svc)
	s2, _ := service.SchedulerOf(second)

	if s1 == s2 || s1.Owner() == s2.Owner() {
		t.Fatal("the two servers must be two schedulers")
	}

	project := e.newProject("two servers")
	person := e.browser(ui)
	begun := e.begin(person)

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	p := &pubEnv{socialEnv: e, pub: nil, project: project, connID: begun.ConnectionID}
	exp := p.seedExport(project, "job_0000000802", ".mp4", []byte("fake mp4 bytes"))

	const posts = 8

	var ids []string

	pubn, err := e.svc.PublicationCreate(e.as(token), service.PublicationCreateRequest{
		ProjectID: project, Title: "Both", Export: exp,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := range posts {
		at := e.clock.now().Add(time.Hour + time.Duration(i)*time.Second).Format("2006-01-02T15:04:05")

		got, err := e.svc.PublicationUpdate(e.as(token), service.PublicationUpdateRequest{
			ID: pubn.ID, Targets: []pub.TargetInput{{
				ConnectionID: begun.ConnectionID, At: at, Zone: "UTC", Fields: pub.Fields{MadeForKids: new(false)},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		pubn = got
		ids = append(ids, got.Targets[i].ID)
	}

	for _, id := range ids {
		if _, err := e.svc.PublicationSchedule(
			e.as(token),
			service.PublicationScheduleRequest{ID: pubn.ID, TargetID: id},
		); err != nil {
			t.Fatal(err)
		}

		got, _ := e.svc.PublicationGet(e.as(token), service.PublicationGetRequest{ID: pubn.ID})
		if _, err := e.svc.ApprovalDecide(
			person, service.ApprovalDecideRequest{ID: got.Target(id).ApprovalID, Decision: "approve"},
		); err != nil {
			t.Fatal(err)
		}
	}

	e.clock.advance(time.Hour + posts*time.Second)

	// Both servers wake at the same instant, many times over: every target is claimed by one of them.
	var wg sync.WaitGroup

	for _, s := range []*pub.Scheduler{s1, s2} {
		wg.Go(func() {
			for range 5 {
				if _, err := s.Tick(context.Background()); err != nil {
					t.Error(err)
				}
			}
		})
	}

	wg.Wait()
	s1.Wait()
	s2.Wait()

	if got := uploads(e.net); got != posts {
		t.Fatalf("the network got %d uploads for %d targets", got, posts)
	}

	final, _ := e.svc.PublicationGet(e.as(token), service.PublicationGetRequest{ID: pubn.ID})
	for _, tg := range final.Targets {
		if tg.State != corepub.TargetPublished || tg.Attempts != 1 {
			t.Errorf("target %s: %s after %d attempts", tg.ID, tg.State, tg.Attempts)
		}
	}
}

// Acceptance criterion 3: a target whose time passed while the product was stopped is not sent until a
// person decides, unless the late window is on.
func TestSchedulerMissedTimeIsNeverSentOnItsOwn(t *testing.T) {
	c := newCalendar(t)
	late := 240

	p := c.create(c.target(c.at(time.Hour)), c.target(c.at(time.Hour)))
	p2, err := c.svc.PublicationUpdate(c.as(token), service.PublicationUpdateRequest{
		ID: p.ID, Targets: []pub.TargetInput{{ID: p.Targets[1].ID, LateWindowMinutes: &late}},
	})
	if err != nil {
		t.Fatal(err)
	}

	strict, windowed := p2.Targets[0].ID, p2.Targets[1].ID

	c.schedule(p, strict)
	c.schedule(p, windowed)
	c.approve(p, strict)
	c.approve(p, windowed)

	// The product stops for three hours across both times.
	c.clock.advance(4 * time.Hour)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d sends, want only the one with a late window", n)
	}

	if c.state(p, strict) != corepub.TargetMissed || c.state(p, windowed) != corepub.TargetPublished {
		t.Fatalf("states = %s, %s", c.state(p, strict), c.state(p, windowed))
	}

	m := c.get(p).Target(strict)
	if m.Error == nil || m.Error.Code != "missed" || m.Error.Hint == "" || uploads(c.net) != 1 {
		t.Errorf("missed target = %+v, uploads %d", m, uploads(c.net))
	}

	// It stays missed, however long the product runs.
	c.clock.advance(24 * time.Hour)

	if n := c.tick(); n != 0 || c.state(p, strict) != corepub.TargetMissed || uploads(c.net) != 1 {
		t.Fatalf("a missed target moved: %d started, %s, %d uploads", n, c.state(p, strict), uploads(c.net))
	}

	// The person chooses to send it now: another request, and then it goes.
	got, err := c.svc.PublicationSendNow(c.as(token), service.PublicationSendNowRequest{ID: p.ID, TargetID: strict})
	if err != nil || got.Target(strict).State != corepub.TargetWaitingApproval {
		t.Fatalf("send now: %+v, %v", got.Target(strict), err)
	}

	c.approve(p, strict)

	if n := c.tick(); n != 1 || c.state(p, strict) != corepub.TargetPublished || uploads(c.net) != 2 {
		t.Fatalf("after send now: %d started, %s, %d uploads", n, c.state(p, strict), uploads(c.net))
	}
}

// Acceptance criterion 4: editing an approved target requires approval again.
func TestPublicationEditAfterApprovalNeedsApprovalAgain(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	first := c.schedule(p, id).Target(id).ApprovalID
	c.approve(p, id)

	if c.state(p, id) != corepub.TargetScheduled {
		t.Fatalf("state = %s", c.state(p, id))
	}

	title := "A better title"

	got, err := c.svc.PublicationUpdate(c.as(token), service.PublicationUpdateRequest{ID: p.ID, Title: &title})
	if err != nil {
		t.Fatal(err)
	}

	tg := got.Target(id)
	if tg.State != corepub.TargetWaitingApproval || tg.ApprovalID == first {
		t.Fatalf("target = %+v", tg)
	}

	// The old request stays what it was, an approval of the old post, and is decided: it approves nothing more.
	old, err := c.svc.ApprovalGet(c.as(token), service.ApprovalRef{ID: first})
	if err != nil || old.State != approval.StateApproved {
		t.Fatalf("old request = %+v, %v", old, err)
	}

	if _, err := c.svc.ApprovalDecide(
		c.browser(ui),
		service.ApprovalDecideRequest{ID: first, Decision: "approve"},
	); err == nil {
		t.Error("the old request was approved")
	}

	fresh, err := c.svc.ApprovalGet(c.as(token), service.ApprovalRef{ID: tg.ApprovalID})
	if err != nil || fresh.State != approval.StatePending || !strings.Contains(fresh.Summary, title) {
		t.Fatalf("new request = %+v, %v", fresh, err)
	}

	// Nothing goes until the new request is approved.
	c.clock.advance(2 * time.Hour)

	if n := c.tick(); n != 0 || uploads(c.net) != 0 {
		t.Fatalf("an edited post went without approval: %d uploads", uploads(c.net))
	}
}

// Acceptance criterion 5: a target shows its URL when published, or an actionable error when failed, and can be retried.
func TestSchedulerFailureIsActionableAndCanBeRetried(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	c.conn.Fail = social.NewError(social.KindNotEligible, "fake", "", 403, errors.New("nope"))

	c.schedule(p, id)
	c.approve(p, id)
	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d", n)
	}

	failed := c.get(p).Target(id)
	if failed.State != corepub.TargetFailed || failed.Error == nil || failed.Error.Kind != social.KindNotEligible ||
		failed.Error.Hint == "" || failed.Error.Message == "" || failed.URL != "" {
		t.Fatalf("target = %+v, error = %+v", failed, failed.Error)
	}

	// A permanent error is not retried by the scheduler.
	c.clock.advance(time.Hour)

	if n := c.tick(); n != 0 || uploads(c.net) != 1 {
		t.Fatalf("a permanent error was retried: %d uploads", uploads(c.net))
	}

	// The person fixes the channel and retries: a new request, then it goes.
	c.conn.Fail = nil

	got, err := c.svc.PublicationRetry(c.as(token), service.PublicationRetryRequest{ID: p.ID, TargetID: id})
	if err != nil || got.Target(id).State != corepub.TargetWaitingApproval || got.Target(id).Error != nil {
		t.Fatalf("retry: %+v, %v", got.Target(id), err)
	}

	c.approve(p, id)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d after the retry", n)
	}

	done := c.get(p).Target(id)
	if done.State != corepub.TargetPublished || !strings.HasPrefix(done.URL, "https://fake.example/watch/") ||
		done.Error != nil {
		t.Errorf("target = %+v", done)
	}
}

// A transient error is retried with a backoff, and the post goes once it passes.
func TestSchedulerRetriesATransientErrorWithBackoff(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	c.conn.Fail = social.NewError(social.KindUnavailable, "fake", "", 503, errors.New("down"))

	c.schedule(p, id)
	c.approve(p, id)
	c.clock.advance(time.Hour)
	c.tick()

	tg := c.get(p).Target(id)
	if tg.State != corepub.TargetScheduled || tg.Attempts != 1 || tg.NextAttemptAt == nil || tg.Error == nil ||
		!tg.Error.Retryable {
		t.Fatalf("after the first failure: %+v, %+v", tg, tg.Error)
	}

	if !tg.NextAttemptAt.Equal(c.clock.now().Add(time.Minute)) {
		t.Errorf("next attempt = %s, want a minute from now", tg.NextAttemptAt)
	}

	// Not yet.
	c.clock.advance(30 * time.Second)

	if n := c.tick(); n != 0 {
		t.Fatalf("retried too soon: %d", n)
	}

	// The network comes back.
	c.conn.Fail = nil

	c.clock.advance(31 * time.Second)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d on the retry", n)
	}

	if done := c.get(p).Target(id); done.State != corepub.TargetPublished || done.Attempts != 2 || done.Error != nil {
		t.Errorf("target = %+v, error = %+v", done, done.Error)
	}
}

// The calendar lists by project, state and time range.
func TestPublicationListByRangeAndState(t *testing.T) {
	c := newCalendar(t)
	a := c.create(c.target(c.at(24 * time.Hour)))
	b := c.create(c.target(c.at(10 * 24 * time.Hour)))

	c.schedule(a, a.Targets[0].ID)

	from := c.clock.now().Format(time.RFC3339)
	to := c.clock.now().Add(5 * 24 * time.Hour).Format(time.RFC3339)

	list, err := c.svc.PublicationList(
		c.as(token),
		service.PublicationListRequest{ProjectID: c.project, From: from, To: to},
	)
	if err != nil || len(list.Publications) != 1 || list.Publications[0].ID != a.ID || list.Total != 1 {
		t.Fatalf("range = %+v, %v", list, err)
	}

	list, err = c.svc.PublicationList(c.as(token), service.PublicationListRequest{State: "draft"})
	if err != nil || len(list.Publications) != 1 || list.Publications[0].ID != b.ID {
		t.Fatalf("draft = %+v, %v", list, err)
	}

	if _, err := c.svc.PublicationList(c.as(token), service.PublicationListRequest{From: "tomorrow"}); err == nil {
		t.Error("a bad range was accepted")
	}

	if _, err := c.svc.PublicationList(c.as(token), service.PublicationListRequest{State: "bogus"}); err == nil {
		t.Error("a bad state was accepted")
	}
}

// A time that does not exist in its zone is moved and the person is told; the instant is recorded in UTC.
func TestPublicationTimeZoneAndDaylightSaving(t *testing.T) {
	c := newCalendar(t)

	got, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "Spring forward", Export: c.exp, Targets: []pub.TargetInput{
			{
				ConnectionID: c.connID,
				At:           "2027-03-14T02:30",
				Zone:         "America/New_York",
				Fields:       pub.Fields{MadeForKids: new(false)},
			},
			{
				ConnectionID: c.connID,
				At:           "2027-03-14T09:00",
				Zone:         "America/Sao_Paulo",
				Fields:       pub.Fields{MadeForKids: new(false)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	gap, plain := got.Targets[0], got.Targets[1]
	if !gap.ScheduledAt.Equal(time.Date(2027, 3, 14, 7, 30, 0, 0, time.UTC)) || gap.LocalTime != "2027-03-14 03:30" ||
		len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "does not exist") {
		t.Errorf("gap target = %+v, notes %q", gap, got.Notes)
	}

	if !plain.ScheduledAt.Equal(time.Date(2027, 3, 14, 12, 0, 0, 0, time.UTC)) ||
		plain.LocalTime != "2027-03-14 09:00" {
		t.Errorf("plain target = %+v", plain)
	}

	// Read back from the database, the instant and the zone are what was written.
	again := c.get(got)
	if !again.Targets[0].ScheduledAt.Equal(gap.ScheduledAt.UTC()) || again.Targets[0].Zone != "America/New_York" {
		t.Errorf("stored target = %+v", again.Targets[0])
	}
}

// A publication is an item of its project: the overview and the text view of the production show it, and the
// production counts what waits for a person and what went out (ADR-0039, RF-303).
func TestPublicationIsAnItemOfItsProject(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	describe := func() *service.ProjectDescribeResult {
		res, err := c.svc.ProjectDescribe(c.as(token), service.ProjectDescribeRequest{ProjectID: c.project})
		if err != nil {
			t.Fatal(err)
		}

		return res
	}

	item := func(res *service.ProjectDescribeResult) *service.ProjectItem {
		for i := range res.Items {
			if res.Items[i].ID == p.ID {
				return &res.Items[i]
			}
		}

		return nil
	}

	res := describe()
	if it := item(res); it == nil || it.Kind != "publication" || it.Title != "Week one" || it.Status != "" ||
		res.Production.PendingApprovals != 0 || res.Production.Published != 0 || !strings.Contains(res.Text, "Week one") {
		t.Fatalf("a draft: item %+v, production %+v\n%s", item(res), res.Production, res.Text)
	}

	c.schedule(p, id)

	if res = describe(); item(res).Status != "pending_approval" || res.Production.PendingApprovals != 1 {
		t.Fatalf("waiting: item %+v, production %+v", item(res), res.Production)
	}

	c.approve(p, id)
	c.clock.advance(time.Hour)
	c.tick()

	if res = describe(); item(res).Status != "published" || res.Production.Published != 1 ||
		res.Production.PendingApprovals != 0 {
		t.Fatalf("published: item %+v, production %+v", item(res), res.Production)
	}

	overview, err := c.svc.ProjectGet(c.as(token), service.ProjectGetRequest{ProjectID: c.project})
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for i := range overview.Items {
		found = found || overview.Items[i].Kind == "publication" && overview.Items[i].Status == "published"
	}

	if !found || overview.Production.Published != 1 {
		t.Errorf("the overview of the project lacks the publication: %+v", overview.Items)
	}
}
