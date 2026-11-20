package servicetest

import (
	"context"
	"strings"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// A scheduled post goes only to the account the person approved (SDD-020 sections 17 and 26, T-60): the
// connection signs in again and another Page is chosen after the approval, before the time, and at its time
// nothing is sent to either Page.
func TestSecurity_T60_AScheduledPostGoesOnlyToTheAccountApproved(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	sched, ok := service.SchedulerOf(m.svc)
	if !ok {
		t.Fatal("the service has no scheduler")
	}

	tick := func() int {
		t.Helper()

		n, err := sched.Tick(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		sched.Wait()

		return n
	}

	conn, _ := m.connect("facebook")

	if _, err := m.selectAccount(person, conn, "P2"); err != nil {
		t.Fatal(err)
	}

	exp := m.seedReel("job_0000000975", 1080, 1920, 20)
	plan := func() (*service.Publication, string) {
		t.Helper()

		p, err := m.svc.PublicationCreate(m.as(token), service.PublicationCreateRequest{
			ProjectID: m.project, Title: "My first reel", Text: "About it", Export: exp,
			Targets: []pub.TargetInput{{
				ConnectionID: conn, At: m.clock.now().Add(time.Hour).UTC().Format(time.RFC3339), Zone: "UTC",
				Fields: pub.Fields{Visibility: "public"},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		id := p.Targets[0].ID

		got, err := m.svc.PublicationSchedule(m.as(token), service.PublicationScheduleRequest{ID: p.ID, TargetID: id})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := m.svc.ApprovalDecide(
			person, service.ApprovalDecideRequest{ID: got.Target(id).ApprovalID, Decision: "approve"},
		); err != nil {
			t.Fatal(err)
		}

		return p, id
	}

	read := func(p *service.Publication, id string) pub.Target {
		t.Helper()

		got, err := m.svc.PublicationGet(m.as(token), service.PublicationGetRequest{ID: p.ID})
		if err != nil {
			t.Fatal(err)
		}

		return *got.Target(id)
	}

	p, id := plan()

	if st := read(p, id).State; st != corepub.TargetScheduled {
		t.Fatalf("state after the approval = %s", st)
	}

	// After the approval the connection signs in again and the person chooses the other Page.
	begun, err := m.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: "facebook", ConnectionID: conn, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, cerr := m.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(m.tt, begun.AuthorizeURL), Code: "c3"},
	); cerr != nil {
		t.Fatal(cerr)
	}

	if _, serr := m.selectAccount(person, conn, "P1"); serr != nil {
		t.Fatal(serr)
	}

	before := m.graph.count()

	m.clock.advance(time.Hour)

	if n := tick(); n != 1 {
		t.Fatalf("started %d sends, want the one due", n)
	}

	got := read(p, id)
	if got.State != corepub.TargetFailed || got.Error == nil || got.Error.Code != "E_INVALID_STATE" ||
		!strings.Contains(got.Error.Hint, "approve") {
		t.Fatalf("target = %+v, error %+v; want failed, since the account is not the one approved", got, got.Error)
	}

	for _, call := range m.graph.seen()[before:] {
		if strings.Contains(call, "video_reels") || strings.Contains(call, "/P1") || strings.Contains(call, "/P2") {
			t.Errorf("a request went out for a post whose account changed: %s", call)
		}
	}

	// A post approved for the Page chosen now goes to that Page and to no other.
	q, qid := plan()
	before = m.graph.count()

	m.clock.advance(time.Hour)

	if n := tick(); n != 1 {
		t.Fatalf("started %d sends for the second post", n)
	}

	if got := read(q, qid); got.State != corepub.TargetPublished {
		t.Fatalf("second post = %+v, error %+v", got, got.Error)
	}

	for _, call := range m.graph.seen()[before:] {
		if strings.Contains(call, "/P2") {
			t.Errorf("a request went to the Page that is no longer chosen: %s", call)
		}
	}
}
