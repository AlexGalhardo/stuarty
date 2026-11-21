package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

// publishOps is the part of the service the publication tests use.
type publishOps interface {
	PublishValidate(ctx context.Context, req service.PublishValidateRequest) (*service.PublishValidateResult, error)
	PublishNow(ctx context.Context, req service.PublishNowRequest) (*service.Job, error)
	PublishStatus(ctx context.Context, req service.PublishStatusRequest) (*service.PublishStatusResult, error)
}

type pubEnv struct {
	*socialEnv
	pub     publishOps
	project string
	connID  string
}

// newPubEnv is a social environment whose jobs run inline after the commit, with a project and a
// connection signed in to the fake network.
func newPubEnv(t *testing.T) *pubEnv {
	t.Helper()

	return newPubEnvWith(t, nil, nil, nil)
}

// newPubEnvWith is newPubEnv over a given network, connector and registry; nil takes the fake network.
func newPubEnvWith(t *testing.T, n *fakeNet, conn *fake.Connector, reg *social.Registry) *pubEnv {
	t.Helper()

	return newPubEnvWrapped(t, n, conn, reg, nil)
}

// newPubEnvWrapped is newPubEnvWith with the store the service reads through wrapped, to fail one read.
func newPubEnvWrapped(
	t *testing.T, n *fakeNet, conn *fake.Connector, reg *social.Registry, wrap func(service.Store) service.Store,
) *pubEnv {
	t.Helper()

	lazy := &lazyRunner{}

	withQueue := func(o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			t.Fatal(err)
		}

		o.Enqueue = jobs.Dispatcher{Queue: q}

		if wrap != nil {
			o.Store = wrap(o.Store)
		}
	}

	var e *socialEnv
	if n == nil {
		e = newSocialEnvWith(t, withQueue)
	} else {
		e = newSocialEnvFor(t, n, conn, reg, withQueue)
	}

	r, ok := service.RunnerOf(e.svc)
	if !ok {
		t.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r = r
	lazy.mu.Unlock()

	pub, ok := e.svc.(publishOps)
	if !ok {
		t.Fatal("the service has no publish operations")
	}

	p := &pubEnv{socialEnv: e, pub: pub, project: e.newProject("p")}

	person := e.browser(ui)
	begun := e.begin(person)

	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	p.connID = begun.ConnectionID

	return p
}

// seedExport writes a succeeded export job of the project and the file it made.
func (p *pubEnv) seedExport(project, id, ext string, data []byte) string {
	p.t.Helper()

	dir := filepath.Join(string(p.dir), "exports", project)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		p.t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, id+ext), data, 0o600); err != nil {
		p.t.Fatal(err)
	}

	// The digest is recorded as the exporter records it (service/exporter), so a run checks it.
	sum := sha256.Sum256(data)

	res, merr := json.Marshal(service.JobResult{Export: &service.ExportResult{
		Format: strings.TrimPrefix(ext, "."), Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}})
	if merr != nil {
		p.t.Fatal(merr)
	}

	werr := p.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		row := service.JobRow{
			ID:        id,
			Kind:      service.JobExport,
			ProjectID: &project,
			Actor:     ui,
			State:     service.JobQueued,
			Input:     json.RawMessage(`{}`),
		}
		if err := tx.Jobs().CreateLimited(ctx, row, nil); err != nil {
			return err
		}

		if _, err := tx.Jobs().Claim(ctx, id, p.clock.now().UnixMilli()); err != nil {
			return err
		}

		_, err := tx.Jobs().Finish(ctx, id, service.JobSucceeded, res, nil, p.clock.now().UnixMilli())

		return err
	})
	if werr != nil {
		p.t.Fatal(werr)
	}

	return id
}

func (p *pubEnv) args(exportID string) service.PublishArgs {
	return service.PublishArgs{
		ConnectionID: p.connID, Export: exportID, Title: "My first short", Description: "About it",
		Tags: []string{"stuarty"}, MadeForKids: new(false),
	}
}

func (p *pubEnv) now(ctx context.Context, args service.PublishArgs) (*service.Job, error) {
	return p.pub.PublishNow(ctx, service.PublishNowRequest{ProjectID: p.project, PublishArgs: args})
}

func (p *pubEnv) decide(job *service.Job, decision string) *approval.Request {
	p.t.Helper()

	req, err := p.svc.ApprovalDecide(
		p.browser(ui),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: decision},
	)
	if err != nil {
		p.t.Fatal(err)
	}

	return req
}

func (p *pubEnv) job(id string) *service.Job {
	p.t.Helper()

	j, err := p.svc.JobGet(p.as(token), service.JobGetRequest{JobID: id})
	if err != nil {
		p.t.Fatal(err)
	}

	return j
}

// Acceptance criteria 1 and 2 of epic 18: nothing leaves the machine before a person approved, and
// after that the export ends published with the remote id and the address.
func TestPublishNow_NothingLeavesBeforeApprovalThenPublishes(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000901", ".mp4", []byte("fake mp4 bytes"))
	before := p.net.count()

	job, err := p.now(p.as(token), p.args(exp))
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.Kind != service.JobPublishUpload {
		t.Fatalf("job = %+v, want a publish.upload waiting for approval", job)
	}

	if p.net.count() != before || len(p.conn.Uploaded()) != 0 {
		t.Fatalf("a request left before the approval: %v", p.net.seen()[before:])
	}

	req, err := p.svc.ApprovalGet(p.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Fake network", "Fake channel", "My first short", "private", "job_0000000901.mp4", "14 bytes"} {
		if !strings.Contains(req.Summary, want) {
			t.Errorf("the summary a person reads lacks %q: %s", want, req.Summary)
		}
	}

	if req.ConnectionID == nil || *req.ConnectionID != p.connID || req.Estimate.Micros != 0 {
		t.Errorf("request = %+v", req)
	}

	// Reading the status of a waiting publication sends nothing either.
	st, err := p.pub.PublishStatus(p.as(token), service.PublishStatusRequest{JobID: job.JobID})
	if err != nil || st.State != "waiting_approval" || p.net.count() != before {
		t.Fatalf("status = %+v, %v, requests %d -> %d", st, err, before, p.net.count())
	}

	p.decide(job, "approve")

	done := p.job(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Publish == nil {
		t.Fatalf("after approval: %+v (error %+v)", done, done.Error)
	}

	got := done.Result.Publish
	if got.RemoteID != "vida" || got.URL != "https://fake.example/watch/vida" || got.State != "published" ||
		got.Network != "fake" || got.ConnectionID != p.connID {
		t.Errorf("result = %+v", got)
	}

	if up := p.conn.Uploaded(); len(up) != 1 || up[0] != "My first short" {
		t.Errorf("uploaded = %v", up)
	}

	st, err = p.pub.PublishStatus(p.as(token), service.PublishStatusRequest{JobID: job.JobID})
	if err != nil || st.State != "published" || st.Result == nil || st.Result.RemoteID != "vida" {
		t.Errorf("status = %+v, %v", st, err)
	}

	if !strings.Contains(done.Text, "https://fake.example/watch/vida") {
		t.Errorf("the job text does not give the address: %s", done.Text)
	}
}

func TestPublishNow_RejectingSendsNothing(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000902", ".mp4", []byte("fake mp4 bytes"))
	before := p.net.count()

	job, err := p.now(p.as(token), p.args(exp))
	if err != nil {
		t.Fatal(err)
	}

	p.decide(job, "reject")

	if done := p.job(job.JobID); done.State != service.JobCancelled {
		t.Errorf("state = %s, want cancelled", done.State)
	}

	if p.net.count() != before {
		t.Errorf("a rejected publication sent: %v", p.net.seen()[before:])
	}

	st, _ := p.pub.PublishStatus(p.as(token), service.PublishStatusRequest{JobID: job.JobID})
	if st == nil || st.State != "failed" {
		t.Errorf("status = %+v, want failed", st)
	}
}

// JobStart with the kind is the same code as PublishNow: there is no way round the approval.
func TestPublishNow_JobStartIsTheSamePath(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000903", ".mp4", []byte("fake mp4 bytes"))
	before := p.net.count()

	raw, merr := json.Marshal(p.args(exp))
	if merr != nil {
		t.Fatal(merr)
	}

	job, err := p.svc.JobStart(
		p.as(token),
		service.JobStartRequest{ProjectID: p.project, Kind: service.JobPublishUpload, Args: raw},
	)
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("JobStart = %+v, %v", job, err)
	}

	dry, err := p.pub.PublishNow(
		p.as(token),
		service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp), DryRun: true},
	)
	if err != nil || dry.State != service.JobWaitingApproval {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}

	if p.net.count() != before {
		t.Errorf("requests left: %v", p.net.seen()[before:])
	}

	pending, _ := p.svc.ApprovalList(p.as(token), service.ApprovalListRequest{})
	if len(pending.Requests) != 1 {
		t.Errorf("%d requests pending, want 1 (the dry run writes nothing)", len(pending.Requests))
	}
}

func TestPublishNow_RefusesWhatTheNetworkWouldRefuseBeforeAskingAPerson(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000904", ".mp4", []byte("fake mp4 bytes"))
	other := p.newProject("other")
	foreign := p.seedExport(other, "job_0000000905", ".mp4", []byte("another project"))
	notMP4 := p.seedExport(p.project, "job_0000000906", ".mov", []byte("a mov"))

	for name, edit := range map[string]func(a *service.PublishArgs){
		"a title over the limit":        func(a *service.PublishArgs) { a.Title = strings.Repeat("t", 101) },
		"no title":                      func(a *service.PublishArgs) { a.Title = "" },
		"an angle bracket":              func(a *service.PublishArgs) { a.Description = "a < b" },
		"kids not declared":             func(a *service.PublishArgs) { a.MadeForKids = nil },
		"an unknown visibility":         func(a *service.PublishArgs) { a.Visibility = "friends" },
		"an export of another project":  func(a *service.PublishArgs) { a.Export = foreign },
		"an export that is not an mp4":  func(a *service.PublishArgs) { a.Export = notMP4 },
		"an export that does not exist": func(a *service.PublishArgs) { a.Export = "job_9999999999" },
		"no connection":                 func(a *service.PublishArgs) { a.ConnectionID = "" },
		"a connection that is gone":     func(a *service.PublishArgs) { a.ConnectionID = "con_zzzzzzzzzz" },
	} {
		a := p.args(exp)
		edit(&a)

		_, err := p.now(p.as(token), a)

		var se *service.Error
		if !errors.As(err, &se) || (se.Code != "E_VALIDATION" && se.Code != service.CodeConnectionNotFound) {
			t.Errorf("%s: %v, want a validation error", name, err)
		}
	}

	if pending, _ := p.svc.ApprovalList(p.as(token), service.ApprovalListRequest{}); len(pending.Requests) != 0 {
		t.Errorf("a refused post made %d request(s) for a person", len(pending.Requests))
	}

	// PublishValidate says the same without making a job.
	a := p.args(exp)
	a.Title = ""

	v, err := p.pub.PublishValidate(p.as(token), service.PublishValidateRequest{ProjectID: p.project, PublishArgs: a})
	if err != nil || v.State != "invalid" || len(v.Issues) == 0 || v.Issues[0].Path != "title" {
		t.Fatalf("validate = %+v, %v", v, err)
	}

	ok, err := p.pub.PublishValidate(
		p.as(token),
		service.PublishValidateRequest{ProjectID: p.project, PublishArgs: p.args(exp)},
	)
	if err != nil || ok.State != "prepared" || ok.Summary == "" || len(ok.Issues) != 0 {
		t.Fatalf("validate = %+v, %v", ok, err)
	}
}

// Acceptance criterion 4 of epic 18: a quota error is an actionable message, and publishing again later
// works.
func TestPublishNow_QuotaErrorIsActionableAndCanBeRetried(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000907", ".mp4", []byte("fake mp4 bytes"))

	p.conn.Fail = social.NewError(social.KindQuota, "fake", "quotaExceeded", 403, nil)

	job, err := p.now(p.as(token), p.args(exp))
	if err != nil {
		t.Fatal(err)
	}

	p.decide(job, "approve")

	failed := p.job(job.JobID)
	if failed.State != service.JobFailed || failed.Error == nil || failed.Error.Code != service.CodePublish ||
		failed.Error.Details["kind"] != social.KindQuota || failed.Error.Hint == "" || !failed.Error.Retryable {
		t.Fatalf("failed = %+v, error %+v", failed, failed.Error)
	}

	if st, _ := p.pub.PublishStatus(
		p.as(token),
		service.PublishStatusRequest{JobID: job.JobID},
	); st == nil || st.State != "failed" ||
		st.Error == nil {
		t.Errorf("status = %+v", st)
	}

	// Later, the quota is back: publishing the same post again goes through approval and succeeds.
	p.conn.Fail = nil

	again, err := p.now(p.as(token), p.args(exp))
	if err != nil || again.State != service.JobWaitingApproval {
		t.Fatalf("again = %+v, %v", again, err)
	}

	p.decide(again, "approve")

	if done := p.job(again.JobID); done.State != service.JobSucceeded || done.Result.Publish.State != "published" {
		t.Errorf("the retry = %+v", done)
	}
}

// A file rewritten after the approval is not sent, whether it grew or kept the same size.
func TestPublishNow_AFileThatChangedAfterApprovalIsNotSent(t *testing.T) {
	for name, rewrite := range map[string]string{
		"longer":         "another file, longer than the one that was shown",
		"same size":      "FAKE MP4 BYTES",
		"same size, one": "fake mp4 bytez",
	} {
		t.Run(name, func(t *testing.T) {
			p := newPubEnv(t)
			exp := p.seedExport(p.project, "job_0000000908", ".mp4", []byte("fake mp4 bytes"))

			job, err := p.now(p.as(token), p.args(exp))
			if err != nil {
				t.Fatal(err)
			}

			file := filepath.Join(string(p.dir), "exports", p.project, "job_0000000908.mp4")
			if werr := os.WriteFile(file, []byte(rewrite), 0o600); werr != nil {
				t.Fatal(werr)
			}

			before := p.net.count()
			p.decide(job, "approve")

			if done := p.job(
				job.JobID,
			); done.State != service.JobFailed || done.Error == nil ||
				done.Error.Code != "E_INVALID_STATE" {
				t.Errorf("job = %+v, error %+v, want a refusal to send a changed file", done, done.Error)
			}

			if p.net.count() != before {
				t.Errorf("the changed file was sent: %v", p.net.seen()[before:])
			}
		})
	}
}

// The export of a timeline is kept under the id of the timeline; it belongs to its project, so a post of
// the project may name it, and the export of another project's timeline is still refused.
func TestPublishNow_AnExportOfATimelineOfTheProjectIsAccepted(t *testing.T) {
	p := newPubEnv(t)

	added, err := p.svc.ProjectItemAdd(
		p.as(ui),
		service.ProjectItemAddRequest{ProjectID: p.project, Kind: "timeline", Title: "Second cut"},
	)
	if err != nil {
		t.Fatal(err)
	}

	timeline := added.Item.TimelineID

	if added.Item.Kind != "timeline" || timeline == p.project {
		t.Fatalf("the second timeline is %+v", added.Item)
	}

	// The export of the first timeline, a publication kept on the second: both are the project's.
	exp := p.seedExport(p.project, "job_0000000921", ".mp4", []byte("fake mp4 bytes"))

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{
		ProjectID: p.project, TimelineID: timeline, PublishArgs: p.args(exp),
	})
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("the export of a timeline of the project = %+v, %v", job, err)
	}

	// And one of the second timeline, named by the project.
	second := p.seedExport(timeline, "job_0000000923", ".mp4", []byte("fake mp4 bytes"))

	if _, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{
		ProjectID: p.project, TimelineID: timeline, PublishArgs: p.args(second),
	}); err != nil {
		t.Fatalf("the export of the second timeline = %v", err)
	}

	other := p.newProject("other")
	foreign := p.seedExport(other, "job_0000000922", ".mp4", []byte("fake mp4 bytes"))

	if _, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{
		ProjectID: p.project, TimelineID: timeline, PublishArgs: p.args(foreign),
	}); !isCode(err, "E_VALIDATION") {
		t.Errorf("the export of another project = %v, want E_VALIDATION", err)
	}
}

// A post belongs to the project, so a project of two timelines does not ask which one the post is kept on
// (VV-315): publish:now accepts what publish:validate and publication:create already accept.
func TestPublishNow_ATwoTimelineProjectDoesNotAskForTheTimeline(t *testing.T) {
	p := newPubEnv(t)

	if _, err := p.svc.ProjectItemAdd(
		p.as(ui),
		service.ProjectItemAddRequest{ProjectID: p.project, Kind: "timeline", Title: "Second cut"},
	); err != nil {
		t.Fatal(err)
	}

	exp := p.seedExport(p.project, "job_0000000931", ".mp4", []byte("fake mp4 bytes"))

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{
		ProjectID: p.project, PublishArgs: p.args(exp),
	})
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("publish without a timeline = %+v, %v", job, err)
	}
}

// addSecondTimeline adds a timeline to the project and returns its item and its timeline id.
func (p *pubEnv) addSecondTimeline() (item, timeline string) {
	p.t.Helper()

	added, err := p.svc.ProjectItemAdd(
		p.as(ui),
		service.ProjectItemAddRequest{ProjectID: p.project, Kind: "timeline", Title: "Second cut"},
	)
	if err != nil {
		p.t.Fatal(err)
	}

	return added.Item.ID, added.Item.TimelineID
}

// firstTimelineItem is the item of the timeline that has the id of the project.
func (p *pubEnv) firstTimelineItem() string {
	p.t.Helper()

	got, err := p.svc.ProjectGet(p.as(ui), service.ProjectGetRequest{ProjectID: p.project})
	if err != nil {
		p.t.Fatal(err)
	}

	for i := range got.Items {
		if got.Items[i].Kind == "timeline" && got.Items[i].TimelineID == p.project {
			return got.Items[i].ID
		}
	}

	p.t.Fatal("the project has no first timeline")

	return ""
}

// VV-315: a post is kept on the timeline of its export, whichever timeline the request named. Trashing that
// timeline ends the post, so a video a person threw away is not sent after a later approval.
func TestPublishNow_TrashingTheTimelineOfTheExportEndsThePost(t *testing.T) {
	for _, named := range []bool{false, true} {
		p := newPubEnv(t)
		item, second := p.addSecondTimeline()
		exp := p.seedExport(second, "job_0000000941", ".mp4", []byte("fake mp4 bytes"))

		req := service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)}
		if named {
			req.TimelineID = p.project
		}

		job, err := p.pub.PublishNow(p.as(token), req)
		if err != nil {
			t.Fatal(err)
		}

		if job.ProjectID == nil || *job.ProjectID != second {
			t.Fatalf("named %v: the post is kept on %v, want its export's timeline %s", named, job.ProjectID, second)
		}

		before := p.net.count()

		if _, err := p.svc.ProjectItemTrash(
			p.as(ui),
			service.ProjectItemTrashRequest{ProjectID: p.project, ItemID: item},
		); err != nil {
			t.Fatal(err)
		}

		if got := p.job(job.JobID); got.State != service.JobCancelled {
			t.Errorf("named %v: after the timeline of the export went to the trash the post is %s", named, got.State)
		}

		if _, err := p.svc.ApprovalDecide(
			p.browser(ui),
			service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
		); err == nil {
			t.Errorf("named %v: the post of a trashed timeline was approved", named)
		}

		if p.net.count() != before {
			t.Errorf("named %v: the video of a trashed timeline was sent", named)
		}
	}
}

// VV-315: trashing a timeline the post was not made from leaves the post waiting for its person, and it publishes
// the export it named.
func TestPublishNow_TrashingAnotherTimelineKeepsThePost(t *testing.T) {
	p := newPubEnv(t)
	_, second := p.addSecondTimeline()
	exp := p.seedExport(second, "job_0000000942", ".mp4", []byte("fake mp4 bytes"))

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.svc.ProjectItemTrash(
		p.as(ui),
		service.ProjectItemTrashRequest{ProjectID: p.project, ItemID: p.firstTimelineItem()},
	); err != nil {
		t.Fatal(err)
	}

	if got := p.job(job.JobID); got.State != service.JobWaitingApproval {
		t.Fatalf("trashing the other timeline left the post %s", got.State)
	}

	before := p.net.count()

	p.decide(job, "approve")

	if got := p.job(job.JobID); got.State != service.JobSucceeded || p.net.count() == before {
		t.Errorf("the approved post = %s, %d requests", got.State, p.net.count()-before)
	}
}

// VV-315: with an export on each timeline, a post that names no timeline waits for a person and then sends the file
// of the export it named, and not the one of the first timeline.
func TestPublishNow_TwoTimelinesSendTheExportItNamed(t *testing.T) {
	p := newPubEnv(t)
	_, second := p.addSecondTimeline()

	p.seedExport(p.project, "job_0000000951", ".mp4", []byte("the first cut"))
	exp := p.seedExport(second, "job_0000000952", ".mp4", []byte("the second cut, longer"))
	before := p.net.count()

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)})
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("the post = %+v, %v", job, err)
	}

	if p.net.count() != before {
		t.Fatal("something was sent before a person approved")
	}

	p.decide(job, "approve")

	if got := p.job(job.JobID); got.State != service.JobSucceeded {
		t.Fatalf("the approved post = %+v", got)
	}

	p.net.mu.Lock()
	defer p.net.mu.Unlock()

	want := filepath.Join(second, "job_0000000952.mp4")
	if len(p.net.posted) != 1 || !strings.HasSuffix(p.net.posted[0], want) {
		t.Errorf("the file sent = %v, want the export of the second timeline %s", p.net.posted, want)
	}
}

// VV-315: the export of a timeline that is already in the trash is refused, named by its project.
func TestPublishNow_AnExportOfATrashedTimelineIsRefused(t *testing.T) {
	p := newPubEnv(t)
	item, second := p.addSecondTimeline()
	exp := p.seedExport(second, "job_0000000943", ".mp4", []byte("fake mp4 bytes"))

	if _, err := p.svc.ProjectItemTrash(
		p.as(ui),
		service.ProjectItemTrashRequest{ProjectID: p.project, ItemID: item},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := p.pub.PublishNow(
		p.as(token),
		service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)},
	); !isCode(err, "E_INVALID_STATE") {
		t.Errorf("the export of a trashed timeline = %v, want E_INVALID_STATE", err)
	}
}
