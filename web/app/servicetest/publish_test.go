package servicetest

import (
	"context"
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

	lazy := &lazyRunner{}

	withQueue := func(o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			t.Fatal(err)
		}

		o.Enqueue = jobs.Dispatcher{Queue: q}
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

	res, merr := json.Marshal(

		service.JobResult{Export: &service.ExportResult{Format: strings.TrimPrefix(ext, "."), Bytes: int64(len(data))}},
	)
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

func TestPublishNow_AFileThatChangedAfterApprovalIsNotSent(t *testing.T) {
	p := newPubEnv(t)
	exp := p.seedExport(p.project, "job_0000000908", ".mp4", []byte("fake mp4 bytes"))

	job, err := p.now(p.as(token), p.args(exp))
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(string(p.dir), "exports", p.project, "job_0000000908.mp4")
	if werr := os.WriteFile(file, []byte("another file, longer than the one that was shown"), 0o600); werr != nil {
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
