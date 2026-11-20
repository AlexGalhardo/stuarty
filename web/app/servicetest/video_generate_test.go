package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

const (
	clipModel = "clip-1"
	clipTask  = "operations/task-0001"
)

// clipMaker is a video provider that opens no connection: the epic leaves the real adapter to the approval of
// the provider (SDD-016 section 2.3), so the job is tested against a scripted one that counts what it is asked.
type clipMaker struct {
	submits, polls, fetches, cancels atomic.Int32

	mu sync.Mutex
	// pending is how many polls answer "pending" before "done"; a negative number never ends.
	pending int
	// failed makes the task end failed with this error; fetchErr makes the download fail.
	failed   error
	fetchErr error
	// submitErr makes the submission fail; confirm is what Cancel answers.
	submitErr error
	confirm   bool
	clip      []byte
	// frames are the first frames the provider received.
	frames [][]byte
	// echoID makes the provider answer a task id that repeats the key.
	echoID bool
}

func newClipMaker() *clipMaker {
	return &clipMaker{
		pending: 1, confirm: true,
		clip: append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00"), []byte("isomiso2avc1mp41 fake clip")...),
	}
}

func (c *clipMaker) set(f func(c *clipMaker)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	f(c)
}

func (*clipMaker) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: "clipmaker", Name: "Clip Maker", Capabilities: []string{provider.CapabilityVideo},
		CredentialKinds: []string{provider.CredentialAPIKey}, DefaultEndpoint: "http://127.0.0.1:1/v1",
	}
}

func (*clipMaker) Models(context.Context, connections.Credential) ([]providers.Model, error) {
	return []providers.Model{{ID: clipModel, Capability: provider.CapabilityVideo}}, nil
}

func (*clipMaker) VideoLimits(string) provider.VideoLimits {
	return provider.VideoLimits{MinSeconds: 2, MaxSeconds: 10, Aspects: []string{"16:9", "9:16"}}
}

func (c *clipMaker) Submit(
	_ context.Context, cred connections.Credential, _ provider.VideoRequest, frame *providers.VideoFrame,
) (string, error) {
	c.submits.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if frame != nil {
		c.frames = append(c.frames, frame.Data)
	}

	if c.submitErr != nil {
		return "", c.submitErr
	}

	if c.echoID {
		return "operations/" + apiKey, nil
	}

	_ = cred

	return clipTask, nil
}

func (c *clipMaker) Poll(_ context.Context, _ connections.Credential, id string) (providers.VideoStatus, error) {
	n := c.polls.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if id != clipTask {
		return providers.VideoStatus{}, &providers.Error{Kind: providers.KindBadRequest, Status: 404}
	}

	if c.failed != nil {
		return providers.VideoStatus{State: providers.VideoFailed, Err: c.failed}, nil
	}

	if c.pending < 0 || int(n) <= c.pending {
		return providers.VideoStatus{
			State: providers.VideoPending, Progress: 0.5, PollAfter: 10 * time.Millisecond,
		}, nil
	}

	return providers.VideoStatus{State: providers.VideoDone}, nil
}

func (c *clipMaker) Fetch(
	_ context.Context, _ connections.Credential, _ string, _ int64,
) (providers.GeneratedFile, error) {
	c.fetches.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.fetchErr != nil {
		return providers.GeneratedFile{}, c.fetchErr
	}

	return providers.GeneratedFile{Data: c.clip}, nil
}

func (c *clipMaker) Cancel(context.Context, connections.Credential, string) (bool, error) {
	c.cancels.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.confirm, nil
}

type videoEnv struct {
	*mediaEnv

	maker *clipMaker
	conn  string
	prj   string
	// images are the hashes the fake ingest classes as images.
	images sync.Map
}

// newVideoEnv is an environment with the media machinery, the scripted video provider registered, a connection to
// it with a key in the vault, and a queue that holds what is enqueued until the test runs it, so a test decides
// when a run starts and when it is interrupted.
func newVideoEnv(tb testing.TB) *videoEnv {
	tb.Helper()

	v := &videoEnv{maker: newClipMaker()}

	cfg := mediaCfg{}
	cfg.ingest = func(_ context.Context, in ingest.Input) (*ingest.Result, error) {
		res, err := fakeResult(v.ws, in)
		if err != nil {
			return nil, err
		}

		if _, isImage := v.images.Load(in.Hash); isImage {
			res.Kind, res.Summary.Kind = ingest.KindImage, ingest.KindImage
		}

		return res, nil
	}
	cfg.with = func(e *env, o *service.Options) {
		m := connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), vault.NewMemoryProvider(nil)),
		})
		reg := providers.NewRegistry(
			m, providers.NewPriceBook(filepath.Join(string(e.dir), "providers", providers.PriceFile)), e.clock.now,
		)

		if err := reg.Register(v.maker); err != nil {
			tb.Fatal(err)
		}

		o.Connections, o.Providers = m, reg
	}

	v.mediaEnv = newMediaEnv(tb, cfg)
	v.hold.Store(true)

	c, err := v.svc.ConnectionCreate(v.as(ui), service.ConnectionCreateRequest{
		Kind: "clipmaker", Label: "my clips", Endpoint: "http://127.0.0.1:1/v1", Accepts: []string{"text"},
		Secret: apiKey,
	})
	if err != nil {
		tb.Fatal(err)
	}

	v.conn, v.prj = c.ID, v.newProject("p")

	micros := int64(400_000)
	if _, err := v.svc.ProviderPriceSet(v.asPerson(), service.ProviderPriceSetRequest{
		Provider: "clipmaker", Model: clipModel, Currency: "USD", Unit: "second", UnitMicros: &micros,
	}); err != nil {
		tb.Fatal(err)
	}

	return v
}

func (v *videoEnv) req(seconds int, prompt string) service.VideoGenerateRequest {
	return service.VideoGenerateRequest{
		ProjectID: v.prj, ConnectionID: v.conn,
		Model: clipModel, Prompt: prompt, Seconds: seconds, Aspect: "9:16",
	}
}

func (v *videoEnv) start(seconds int, prompt string) (*service.Job, error) {
	return v.svc.VideoGenerate(v.as(token), v.req(seconds, prompt))
}

// approved submits, checks that the job waits for a person, approves it and returns the job, which is queued: it
// runs when the test calls run.
func (v *videoEnv) approved() *service.Job {
	v.t.Helper()

	job, err := v.start(5, "p")
	if err != nil {
		v.t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval {
		v.t.Fatalf("the job is %s, want waiting_approval", job.State)
	}

	if n := v.maker.submits.Load(); n != 0 {
		v.t.Fatalf("the provider was asked %d times before a person approved", n)
	}

	if err := v.approve(job.ApprovalID); err != nil {
		v.t.Fatal(err)
	}

	return v.job(job.JobID)
}

// run runs a queued job to its end. The queue is open meanwhile, so the ingest of what the job stores runs.
func (v *videoEnv) run(id string) *service.Job {
	v.t.Helper()

	v.hold.Store(false)
	defer v.hold.Store(true)

	if err := v.runner.RunJob(context.Background(), id); err != nil {
		v.t.Fatal(err)
	}

	return v.job(id)
}

func (v *videoEnv) spent() int64 { return v.spentToday() }

// Acceptance criteria 1 and 2 of epic 11: with no rule the job waits for a person and the provider is not
// asked; after approval it ends with a clip whose origin is complete, and the ledger holds the real cost.
func TestVideoGenerate_WaitsForApprovalByDefaultAndStoresTheClipWithACompleteOrigin(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)

	job, err := v.start(5, "A red fox running through snow")
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.Kind != "video.generate" || job.Estimate == nil ||
		*job.Estimate != (core.Money{Micros: 2_000_000, Currency: "USD"}) {
		t.Fatalf("job = %+v (estimate %+v)", job, job.Estimate)
	}

	if n := v.maker.submits.Load() + v.maker.polls.Load(); n != 0 {
		t.Fatalf("the provider was reached %d times before a person approved", n)
	}

	req, err := v.svc.ApprovalGet(v.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.Estimate != (core.Money{Micros: 2_000_000, Currency: "USD"}) ||
		!strings.Contains(req.Summary, "5 seconds") || !strings.Contains(req.Summary, "A red fox") {
		t.Fatalf("request = %+v, %v", req, err)
	}

	if aerr := v.approve(job.ApprovalID); aerr != nil {
		t.Fatal(aerr)
	}

	done := v.run(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated == nil {
		t.Fatalf("job after approval = %+v", done)
	}

	res := done.Result.Generated
	if res.Provider != "clipmaker" || res.Model != clipModel || res.Requested != 1 || res.Delivered != 1 ||
		len(res.Items) != 1 || res.Cost != (core.Money{Micros: 2_000_000, Currency: "USD"}) || len(res.Notes) != 0 {
		t.Fatalf("result = %+v", res)
	}

	it := res.Items[0]
	if it.AssetID == "" || !strings.HasPrefix(it.Hash, "sha256:") || it.Kind != "video" || it.State != "ready" {
		t.Errorf("item = %+v", it)
	}

	a, err := v.svc.AssetGet(v.as(token), service.AssetRef{Hash: it.Hash})
	if err != nil {
		t.Fatal(err)
	}

	o := a.Origin
	if o.Kind != "generated" || o.Provider != "clipmaker" || o.Model != clipModel ||
		o.Prompt != "A red fox running through snow" || o.CostMicros == nil || *o.CostMicros != 2_000_000 ||
		a.CreatedBy != token || !strings.HasPrefix(o.Source, "job ") {
		t.Errorf("origin = %+v, created by %q", o, a.CreatedBy)
	}

	if n := v.maker.submits.Load(); n != 1 {
		t.Errorf("submissions = %d, want 1", n)
	}

	if n := v.spent(); n != 2_000_000 {
		t.Errorf("spent today = %d, want 2000000", n)
	}
}

func TestVideoGenerate_ARuleTheUserWroteLiftsTheApproval(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)

	perItem := int64(3_000_000)
	if _, err := v.svc.ApprovalRuleSet(v.asPerson(), service.ApprovalRuleSetRequest{
		Kind: "video.generate", ConnectionID: v.conn, Currency: "USD", PerItemMicros: &perItem,
	}); err != nil {
		t.Fatal(err)
	}

	job, err := v.start(5, "p")
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobQueued {
		t.Fatalf("a rule covers it, so the job is queued: %+v", job)
	}

	if done := v.run(job.JobID); done.State != service.JobSucceeded {
		t.Fatalf("job = %+v", done)
	}

	// Above the limit of the rule a person is asked again.
	job, err = v.start(8, "p")
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("an estimate over the rule waits for a person: %+v, %v", job, err)
	}
}

func TestVideoGenerate_ACeilingRefusesAndDryRunWritesNothing(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.setCeiling("", 1_000_000, 0)

	if _, err := v.start(5, "p"); code(err) != service.CodeCostCeiling {
		t.Fatalf("got %v, want E_COST_CEILING", err)
	}

	v.setCeiling("", 5_000_000, 0)

	dry := v.req(5, "p")
	dry.DryRun = true

	job, err := v.svc.VideoGenerate(v.as(token), dry)
	if err != nil || job.Estimate == nil || job.Estimate.Micros != 2_000_000 {
		t.Fatalf("got %+v, %v", job, err)
	}

	list, err := v.svc.ApprovalList(v.as(token), service.ApprovalListRequest{State: "all"})
	if err != nil || list.Total != 0 {
		t.Errorf("a refused request and a dry run left requests: %+v, %v", list, err)
	}

	if v.maker.submits.Load() != 0 {
		t.Error("the provider was asked")
	}
}

func TestVideoGenerate_ValidationFollowsTheLimitsOfTheModel(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)

	for name, tc := range map[string]struct {
		mutate func(r *service.VideoGenerateRequest)
		path   string
	}{
		"too short for the model": {func(r *service.VideoGenerateRequest) { r.Seconds = 1 }, "args.seconds"},
		"too long for the model":  {func(r *service.VideoGenerateRequest) { r.Seconds = 11 }, "args.seconds"},
		"an aspect the model lacks": {
			func(r *service.VideoGenerateRequest) { r.Aspect = "1:1" }, "args.aspect",
		},
		"no prompt":             {func(r *service.VideoGenerateRequest) { r.Prompt = "" }, "args.prompt"},
		"an unknown asset":      {func(r *service.VideoGenerateRequest) { r.FirstFrame = "ast_nothere" }, "args.first_frame"},
		"an unknown connection": {func(r *service.VideoGenerateRequest) { r.ConnectionID = "con_nothere" }, ""},
	} {
		r := v.req(5, "p")
		tc.mutate(&r)

		_, err := v.svc.VideoGenerate(v.as(token), r)

		se, ok := svcErr(err)
		if !ok {
			t.Errorf("%s: got %v, want a service error", name, err)

			continue
		}

		if tc.path != "" && se.Path != tc.path {
			t.Errorf("%s: path %q, want %q (%v)", name, se.Path, tc.path, se)
		}
	}

	if v.maker.submits.Load() != 0 {
		t.Error("a request that fails validation reached the provider")
	}
}

func TestVideoGenerate_AnUnknownPriceIsRefused(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)

	r := v.req(5, "p")
	r.Model = "clip-unpriced"

	if _, err := v.svc.VideoGenerate(v.as(token), r); code(err) != service.CodePriceUnknown {
		t.Fatalf("got %v, want E_PRICE_UNKNOWN", err)
	}
}

// Acceptance criterion 3 of epic 11: a restart during the polling makes exactly one submission. The run is
// interrupted as a shutdown interrupts it, the job is queued again with the task id in its stored input, and the
// next run resumes at the polling.
func TestVideoGenerate_ARestartDuringThePollingMakesOneSubmission(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.maker.set(func(c *clipMaker) { c.pending = -1 })

	job := v.approved()

	ctx, stop := context.WithCancel(context.Background())
	ended := make(chan error, 1)

	go func() { ended <- v.runner.RunJob(ctx, job.JobID) }()

	eventually(t, "two polls", func() bool { return v.maker.polls.Load() >= 2 })
	stop()

	if err := <-ended; err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("the interrupted run returned %v", err)
	}

	row, err := v.store.Jobs().Get(context.Background(), job.JobID)
	if err != nil || row.State != service.JobQueued {
		t.Fatalf("the interrupted job = %+v, %v", row, err)
	}

	var saved struct {
		TaskID string `json:"task_id"`
	}

	if json.Unmarshal(row.Input, &saved) != nil || saved.TaskID != clipTask {
		t.Fatalf("the stored input has no task id: %s", row.Input)
	}

	if n := v.spent(); n != 2_000_000 {
		t.Errorf("the reservation of an interrupted job stays counted: spent = %d", n)
	}

	v.maker.set(func(c *clipMaker) { c.pending = 0 })

	done := v.run(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated.Delivered != 1 {
		t.Fatalf("the resumed job = %+v", done)
	}

	if n := v.maker.submits.Load(); n != 1 {
		t.Fatalf("submissions = %d, want exactly 1 across the restart", n)
	}

	if n := v.spent(); n != 2_000_000 {
		t.Errorf("a restarted job is charged once: spent = %d", n)
	}
}

// A job a shutdown queued again keeps the task its first run started, and the provider goes on making the clip:
// a cancel before the job runs again asks the provider to stop it and records the cost unless the provider
// confirms that nothing is charged, instead of giving the reservation back (SDD-016 section 6.4).
func TestVideoGenerate_ACancelAfterARestartAbandonsTheTaskAndRecordsWhatIsOwed(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		confirm bool
		spent   int64
	}{
		"a provider that confirms the cancel is paid nothing": {true, 0},
		"a provider that does not confirm may have charged":   {false, 2_000_000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := newVideoEnv(t)
			v.maker.set(func(c *clipMaker) { c.pending, c.confirm = -1, tc.confirm })

			job := v.approved()

			ctx, stop := context.WithCancel(context.Background())
			ended := make(chan error, 1)

			go func() { ended <- v.runner.RunJob(ctx, job.JobID) }()

			eventually(t, "a poll", func() bool { return v.maker.polls.Load() >= 1 })
			stop()

			if err := <-ended; !errors.Is(err, context.Canceled) {
				t.Fatalf("the interrupted run returned %v", err)
			}

			if st := v.job(job.JobID).State; st != service.JobQueued {
				t.Fatalf("the interrupted job is %s, want queued", st)
			}

			if _, err := v.svc.JobCancel(v.as(token), service.JobCancelRequest{JobID: job.JobID}); err != nil {
				t.Fatal(err)
			}

			if st := v.job(job.JobID).State; st != service.JobCancelled {
				t.Fatalf("job = %s, want cancelled", st)
			}

			if n := v.maker.cancels.Load(); n != 1 {
				t.Errorf("cancels = %d, want 1: the task the first run started is still at the provider", n)
			}

			if n := v.spent(); n != tc.spent {
				t.Errorf("spent = %d, want %d", n, tc.spent)
			}

			if n := v.maker.submits.Load(); n != 1 {
				t.Errorf("submissions = %d, want 1", n)
			}
		})
	}
}

// A job cancelled while queued before it ever ran has no task at the provider: nothing is asked of it and the
// reservation goes back.
func TestVideoGenerate_ACancelBeforeTheFirstRunAsksTheProviderNothing(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	job := v.approved()

	if _, err := v.svc.JobCancel(v.as(token), service.JobCancelRequest{JobID: job.JobID}); err != nil {
		t.Fatal(err)
	}

	if st := v.job(job.JobID).State; st != service.JobCancelled {
		t.Fatalf("job = %s, want cancelled", st)
	}

	if n := v.maker.submits.Load() + v.maker.polls.Load() + v.maker.cancels.Load(); n != 0 {
		t.Errorf("the provider was reached %d times", n)
	}

	if n := v.spent(); n != 0 {
		t.Errorf("spent = %d, want 0", n)
	}
}

func TestVideoGenerate_ACancelAsksTheProviderAndRecordsWhatIsOwed(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		confirm bool
		spent   int64
	}{
		"a provider that confirms the cancel is paid nothing": {true, 0},
		"a provider that does not confirm may have charged":   {false, 2_000_000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := newVideoEnv(t)
			v.maker.set(func(c *clipMaker) { c.pending, c.confirm = -1, tc.confirm })

			job := v.approved()
			ended := make(chan error, 1)

			go func() { ended <- v.runner.RunJob(context.Background(), job.JobID) }()

			eventually(t, "two polls", func() bool { return v.maker.polls.Load() >= 2 })

			if _, err := v.svc.JobCancel(v.as(token), service.JobCancelRequest{JobID: job.JobID}); err != nil {
				t.Fatal(err)
			}

			if err := <-ended; err != nil {
				t.Fatal(err)
			}

			done := v.job(job.JobID)
			if done.State != service.JobCancelled {
				t.Fatalf("job = %+v", done)
			}

			if n := v.maker.cancels.Load(); n != 1 {
				t.Errorf("cancels = %d, want 1", n)
			}

			if n := v.spent(); n != tc.spent {
				t.Errorf("spent = %d, want %d", n, tc.spent)
			}

			if v.maker.fetches.Load() != 0 {
				t.Error("a cancelled job fetched a clip")
			}
		})
	}
}

func TestVideoGenerate_AFailedTaskIsAStableErrorAndCostsNothing(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		err  error
		kind string
	}{
		"a refused prompt": {&providers.Error{Kind: providers.KindRefused}, "refused"},
		"an unavailable":   {&providers.Error{Kind: providers.KindUnavailable, Status: 503}, "unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := newVideoEnv(t)
			v.maker.set(func(c *clipMaker) { c.failed = tc.err })

			done := v.run(v.approved().JobID)
			if done.State != service.JobFailed || done.Error == nil || done.Error.Code != service.CodeProvider ||
				done.Error.Details["kind"] != tc.kind {
				t.Fatalf("job = %+v (error %+v)", done, done.Error)
			}

			if n := v.spent(); n != 0 {
				t.Errorf("a failed task costs nothing: spent = %d", n)
			}

			if v.maker.fetches.Load() != 0 {
				t.Error("a failed task was fetched")
			}
		})
	}
}

func TestVideoGenerate_ARefusedSubmissionCostsNothing(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.maker.set(func(c *clipMaker) { c.submitErr = &providers.Error{Kind: providers.KindAuth, Status: 401} })

	done := v.run(v.approved().JobID)
	if done.State != service.JobFailed || done.Error == nil || done.Error.Details["kind"] != "auth" {
		t.Fatalf("job = %+v (error %+v)", done, done.Error)
	}

	if n := v.spent(); n != 0 {
		t.Errorf("spent = %d", n)
	}
}

func TestVideoGenerate_ATaskIdThatEchoesTheKeyIsRefused(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.maker.set(func(c *clipMaker) { c.echoID = true })

	done := v.run(v.approved().JobID)
	if done.State != service.JobFailed || done.Error == nil || done.Error.Details["kind"] != "unreadable" {
		t.Fatalf("job = %+v (error %+v)", done, done.Error)
	}

	if strings.Contains(fmt.Sprintf("%+v %+v", done, done.Error), apiKey) {
		t.Error("the key is in the job")
	}
}

func TestVideoGenerate_AFileThatIsNotAClipIsNotKeptButIsPaidFor(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.maker.set(func(c *clipMaker) { c.clip = []byte("<html>not a video</html>") })

	done := v.run(v.approved().JobID)
	if done.State != service.JobFailed || done.Error == nil || done.Error.Details["kind"] != "unreadable" {
		t.Fatalf("job = %+v (error %+v)", done, done.Error)
	}

	// The provider made the clip and billed it, whatever the product makes of the file.
	if n := v.spent(); n != 2_000_000 {
		t.Errorf("spent = %d, want 2000000", n)
	}

	if l, err := v.svc.AssetList(
		v.as(token),
		service.AssetListRequest{Origin: "generated"},
	); err != nil ||
		l.Total != 0 {
		t.Errorf("assets = %+v, %v", l, err)
	}
}

func TestVideoGenerate_ADownloadThatFailsIsPaidForAndFails(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)
	v.maker.set(func(c *clipMaker) { c.fetchErr = &providers.Error{Kind: providers.KindUnavailable, Status: 502} })

	done := v.run(v.approved().JobID)
	if done.State != service.JobFailed {
		t.Fatalf("job = %+v", done)
	}

	if n := v.spent(); n != 2_000_000 {
		t.Errorf("a finished clip is billed even if the download failed: spent = %d", n)
	}
}

// Acceptance criterion 4 of epic 11: an image asset of the project can be the first frame, and a file that is
// not an image cannot.
func TestVideoGenerate_AnImageAssetIsTheFirstFrame(t *testing.T) {
	t.Parallel()

	v := newVideoEnv(t)

	png := pngOf(3)
	sum := sha256.Sum256(png)
	v.images.Store(hex.EncodeToString(sum[:]), true)

	frame := v.importOne(v.prj, v.write("frame.png", string(png))).Assets[0]
	v.ingested(frame.JobID)

	r := v.req(5, "the fox wakes")
	r.FirstFrame = frame.AssetID

	job, err := v.svc.VideoGenerate(v.as(token), r)
	if err != nil {
		t.Fatal(err)
	}

	if req, gerr := v.svc.ApprovalGet(v.as(token), service.ApprovalRef{ID: job.ApprovalID}); gerr != nil ||
		!strings.Contains(req.Summary, frame.AssetID) {
		t.Fatalf("the person is told the clip starts from an image: %+v, %v", req, gerr)
	}

	if aerr := v.approve(job.ApprovalID); aerr != nil {
		t.Fatal(aerr)
	}

	if done := v.run(job.JobID); done.State != service.JobSucceeded {
		t.Fatalf("job = %+v (error %+v)", done, done.Error)
	}

	if len(v.maker.frames) != 1 || !bytes.Equal(v.maker.frames[0], png) {
		t.Fatalf("the provider received %d frames; want the bytes of the image asset", len(v.maker.frames))
	}

	// A video asset is not an image.
	clip := v.importOne(v.prj, v.write("c.mp4", clipBytes)).Assets[0]
	v.ingested(clip.JobID)

	r.FirstFrame = clip.AssetID

	if _, err := v.svc.VideoGenerate(v.as(token), r); code(err) != "E_TYPE_MISMATCH" {
		t.Fatalf("a video as the first frame: %v", err)
	}
}

// ingested runs the ingest job of an import that the held queue left queued.
func (v *videoEnv) ingested(id string) {
	v.t.Helper()

	if err := v.runner.RunJob(context.Background(), id); err != nil {
		v.t.Fatal(err)
	}

	if st := v.job(id).State; st != service.JobSucceeded {
		v.t.Fatalf("the ingest job is %s", st)
	}
}
