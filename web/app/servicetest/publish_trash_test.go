package servicetest

import (
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// A post of an export whose timeline is in the trash cannot be planned at all.
func TestPublication_AnExportOfATimelineInTheTrashIsRefused(t *testing.T) {
	c := newCalendar(t)
	item, second := c.addSecondTimeline()
	exp := c.seedExport(second, "job_0000000971", ".mp4", []byte("the second cut"))

	if _, err := c.svc.ProjectItemTrash(
		c.as(ui),
		service.ProjectItemTrashRequest{ProjectID: c.project, ItemID: item},
	); err != nil {
		t.Fatal(err)
	}

	_, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "Week one", Text: "A caption", Export: exp,
		Targets: []pub.TargetInput{c.target(c.at(time.Hour))},
	})
	if code(err) != "E_INVALID_STATE" {
		t.Fatalf("a publication of a trashed export = %v, want E_INVALID_STATE", err)
	}
}

// An approved post waits for its time: if the timeline goes to the trash meanwhile, nothing is sent, and the target
// says why. After the timeline is restored, a retry asks a person again.
func TestPublication_ATimelineTrashedAfterApprovalIsNotSent(t *testing.T) {
	c := newCalendar(t)
	item, second := c.addSecondTimeline()
	c.exp = c.seedExport(second, "job_0000000972", ".mp4", []byte("the second cut"))

	p := c.create(c.target(c.at(time.Hour)))
	id := p.Targets[0].ID

	c.schedule(p, id)
	c.approve(p, id)

	if _, err := c.svc.ProjectItemTrash(
		c.as(ui),
		service.ProjectItemTrashRequest{ProjectID: c.project, ItemID: item},
	); err != nil {
		t.Fatal(err)
	}

	before := c.net.count()

	c.clock.advance(time.Hour)
	c.tick()

	if got := c.state(p, id); c.net.count() != before || got == corepub.TargetPublished {
		t.Fatalf("a timeline trashed after the approval was sent: target %s, %d requests", got, c.net.count()-before)
	}

	failed := c.get(p).Target(id)
	if failed.State != corepub.TargetFailed || failed.Error == nil || failed.Error.Code != "E_INVALID_STATE" {
		t.Fatalf("the target = %s, %+v; want it failed with E_INVALID_STATE", failed.State, failed.Error)
	}

	if _, err := c.svc.ProjectItemRestore(
		c.as(ui),
		service.ProjectItemTrashRequest{ProjectID: c.project, ItemID: item},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := c.svc.PublicationRetry(
		c.as(token),
		service.PublicationRetryRequest{ID: p.ID, TargetID: id},
	); err != nil {
		t.Fatal(err)
	}

	c.tick()

	if got := c.state(p, id); got != corepub.TargetWaitingApproval || c.net.count() != before {
		t.Errorf("a retry after the restore = %s, %d requests; want a person asked", got, c.net.count()-before)
	}
}

// The same when the person threw away the output item of the export: it stays in the project, in its trash.
func TestPublishNow_AnOutputItemInTheTrashIsRefused(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000973", ".mp4", []byte("fake mp4 bytes"))

	added, err := p.svc.ProjectItemAdd(p.as(ui), service.ProjectItemAddRequest{
		ProjectID: p.project, Kind: "output", Title: "Cut", JobID: exp,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := service.ProjectItemTrashRequest{ProjectID: p.project, ItemID: added.Item.ID}
	if _, err := p.svc.ProjectItemTrash(p.as(ui), req); err != nil {
		t.Fatal(err)
	}

	now := service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)}
	if _, err := p.pub.PublishNow(p.as(token), now); code(err) != "E_INVALID_STATE" {
		t.Fatalf("the export of an output item in the trash = %v, want E_INVALID_STATE", err)
	}

	if _, err := p.svc.ProjectItemRestore(p.as(ui), req); err != nil {
		t.Fatal(err)
	}

	if job, err := p.pub.PublishNow(p.as(token), now); err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("after the restore = %+v, %v", job, err)
	}
}

// Approved while it waits in the queue: the post is stopped by the check right before it is sent.
func TestPublishNow_TheLastCheckStopsATrashedExport(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000975", ".mp4", []byte("fake mp4 bytes"))

	added, err := p.svc.ProjectItemAdd(p.as(ui), service.ProjectItemAddRequest{
		ProjectID: p.project, Kind: "output", Title: "Cut", JobID: exp,
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.svc.ProjectItemTrash(p.as(ui), service.ProjectItemTrashRequest{
		ProjectID: p.project, ItemID: added.Item.ID,
	}); err != nil {
		t.Fatal(err)
	}

	before := p.net.count()

	p.decide(job, "approve")

	if got := p.job(job.JobID); got.State == service.JobSucceeded || p.net.count() != before {
		t.Fatalf("the post of a trashed output item = %s, %d requests", got.State, p.net.count()-before)
	}
}
