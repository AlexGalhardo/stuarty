package servicetest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

// received is one request a local receiver got.
type received struct {
	header http.Header
	body   []byte
}

// receiver is a server on loopback that plays the tool a webhook tells. Its handler can be changed by a test.
type receiver struct {
	srv  *httptest.Server
	mu   sync.Mutex
	got  []received
	next func(n int, w http.ResponseWriter, r *http.Request)
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()

	rc := &receiver{}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		rc.mu.Lock()
		rc.got = append(rc.got, received{header: r.Header.Clone(), body: body})
		n, next := len(rc.got), rc.next
		rc.mu.Unlock()

		if next != nil {
			next(n, w, r)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	t.Cleanup(rc.srv.Close)

	return rc
}

func (rc *receiver) all() []received {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	return append([]received(nil), rc.got...)
}

// hookEnv is a service with a vault that opens, so a webhook can keep its secret.
type hookEnv struct {
	*env
	conns *connections.Manager
	disp  *webhooks.Dispatcher
}

func newHookEnv(t *testing.T) *hookEnv {
	t.Helper()

	h := &hookEnv{}

	h.env = newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.WebhookTimeout = 300 * time.Millisecond
		h.conns = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), vault.NewMemoryProvider(nil)),
		})
		o.Connections = h.conns
	})

	d, ok := service.WebhookDispatcherOf(h.svc)
	if !ok {
		t.Fatal("the service has no webhook dispatcher")
	}

	h.disp = d

	return h
}

func (h *hookEnv) create(url string, events ...string) *service.WebhookSecretResult {
	h.t.Helper()

	if len(events) == 0 {
		events = []string{core.ExportReady, core.JobFailed}
	}

	res, err := h.svc.WebhookCreate(h.as(ui), service.WebhookCreateRequest{Name: "n8n", URL: url, Events: events})
	if err != nil {
		h.t.Fatalf("WebhookCreate: %v", err)
	}

	return res
}

func exportReady(job string) core.Data {
	return core.ExportReadyData{
		JobID:     job,
		ProjectID: "prj_0000000001",
		Kind:      "export",
		Format:    "mp4",
		Bytes:     99,
		Width:     1080,
		Height:    1920,
	}
}

func (h *hookEnv) deliveries(id string) []service.WebhookDelivery {
	h.t.Helper()

	res, err := h.svc.WebhookDeliveryList(h.as(ui), service.WebhookDeliveriesRequest{ID: id})
	if err != nil {
		h.t.Fatal(err)
	}

	return res.Deliveries
}

func TestWebhookDeliveryIsSignedAndVerifiesWithTheShownSecret(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL + "/hook?token=abc")

	h.disp.Publish(context.Background(), exportReady("job_0000000009"), h.clock.now())

	if n := h.disp.RunOnce(context.Background()); n != 1 {
		t.Fatalf("RunOnce took %d deliveries", n)
	}

	got := rc.all()
	if len(got) != 1 {
		t.Fatalf("the receiver got %d requests", len(got))
	}

	r := got[0]
	if r.header.Get("Content-Type") != "application/json" || r.header.Get("Stuarty-Event") != core.ExportReady ||
		r.header.Get("User-Agent") != core.UserAgent || !strings.HasPrefix(r.header.Get("Stuarty-Delivery"), "wdl_") {
		t.Fatalf("headers = %v", r.header)
	}

	if err := core.Verify(
		w.Secret,
		r.header.Get(core.HeaderSignature),
		r.body,
		h.clock.now(),
		core.ReplayWindow,
	); err != nil {
		t.Fatalf("the signature does not verify with the secret the person was shown: %v", err)
	}

	if err := core.Verify(
		"whsec_not-the-secret",
		r.header.Get(core.HeaderSignature),
		r.body,
		h.clock.now(),
		core.ReplayWindow,
	); err == nil {
		t.Fatal("the signature verifies with another secret")
	}

	if err := core.Check(r.body); err != nil {
		t.Fatal(err)
	}

	ds := h.deliveries(w.ID)
	if len(ds) != 1 || ds[0].State != core.StateSucceeded || ds[0].Attempts != 1 || ds[0].LastStatus != 204 ||
		len(ds[0].AttemptLog) != 1 || ds[0].AttemptLog[0].Status != 204 {
		t.Fatalf("deliveries = %+v", ds)
	}

	last, err := h.svc.WebhookGet(h.as(ui), service.WebhookRef{ID: w.ID})
	if err != nil || last.LastDelivery == nil || last.LastDelivery.State != core.StateSucceeded {
		t.Fatalf("WebhookGet = %+v, %v", last, err)
	}
}

func TestWebhookRetriesOn5xxWithBackoffAndKeepsTheSameBody(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	rc.next = func(n int, w http.ResponseWriter, _ *http.Request) {
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}

	w := h.create(rc.srv.URL)
	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())

	h.disp.RunOnce(ctx)

	// Nothing is due until the backoff has passed.
	if n := h.disp.RunOnce(ctx); n != 0 {
		t.Fatalf("a retry was made before its wait: %d", n)
	}

	h.clock.advance(core.Backoff(1))
	h.disp.RunOnce(ctx)
	h.clock.advance(core.Backoff(2))
	h.disp.RunOnce(ctx)

	got := rc.all()
	if len(got) != 3 {
		t.Fatalf("the receiver got %d requests, want 3", len(got))
	}

	if !bytes.Equal(got[0].body, got[2].body) {
		t.Error("a retry changed the body")
	}

	ds := h.deliveries(w.ID)
	if len(ds) != 1 || ds[0].State != core.StateSucceeded || ds[0].Attempts != 3 {
		t.Fatalf("deliveries = %+v", ds)
	}

	if l := ds[0].AttemptLog; len(l) != 3 || l[0].Status != 502 || l[0].Error != core.ErrCodeStatus ||
		l[2].Status != 200 {
		t.Fatalf("attempt log = %+v", l)
	}
}

func TestWebhookGivesUpAfterFiveRetries(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	rc.next = func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }

	w := h.create(rc.srv.URL)
	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(ctx)

	for retry := 1; retry <= core.MaxRetries; retry++ {
		h.clock.advance(core.Backoff(retry))
		h.disp.RunOnce(ctx)
	}

	h.clock.advance(24 * time.Hour)

	if n := h.disp.RunOnce(ctx); n != 0 {
		t.Fatalf("a delivery was tried after the last retry: %d", n)
	}

	if n := len(rc.all()); n != core.MaxAttempts {
		t.Fatalf("%d attempts, want %d", n, core.MaxAttempts)
	}

	if ds := h.deliveries(w.ID); ds[0].State != core.StateFailed || ds[0].Attempts != core.MaxAttempts {
		t.Fatalf("delivery = %+v", ds[0])
	}
}

func TestWebhookDoesNotRetryAnAnswerThatSaysNo(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	rc.next = func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }

	w := h.create(rc.srv.URL)
	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(ctx)
	h.clock.advance(time.Hour)
	h.disp.RunOnce(ctx)

	if n := len(rc.all()); n != 1 {
		t.Fatalf("a 403 was sent %d times", n)
	}

	if ds := h.deliveries(w.ID); ds[0].State != core.StateFailed || ds[0].LastStatus != 403 {
		t.Fatalf("delivery = %+v", ds[0])
	}
}

func TestWebhookTimeoutIsAnAttemptThatIsRetried(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	rc.next = func(_ int, _ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}

	w := h.create(rc.srv.URL)

	h.disp.Publish(context.Background(), exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(context.Background())

	ds := h.deliveries(w.ID)
	if ds[0].State != core.StatePending || ds[0].Attempts != 1 || ds[0].AttemptLog[0].Error != core.ErrCodeTimeout ||
		ds[0].AttemptLog[0].Status != 0 {
		t.Fatalf("delivery = %+v", ds[0])
	}
}

// T-114: a receiver that redirects is not followed to the other address, and the redirect is a failure that is not
// retried.
func TestSecurity_T114_WebhookRedirectIsNotFollowed(t *testing.T) {
	h := newHookEnv(t)
	other := newReceiver(t)
	rc := newReceiver(t)
	rc.next = func(_ int, w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.srv.URL, http.StatusFound)
	}

	w := h.create(rc.srv.URL)

	h.disp.Publish(context.Background(), exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(context.Background())
	h.clock.advance(time.Hour)
	h.disp.RunOnce(context.Background())

	if n := len(other.all()); n != 0 {
		t.Fatalf("the redirect was followed: %d request(s) to the other address", n)
	}

	if ds := h.deliveries(w.ID); ds[0].State != core.StateFailed || ds[0].LastStatus != 302 || ds[0].Attempts != 1 {
		t.Fatalf("delivery = %+v", ds[0])
	}
}

// T-115: the body of an answer is read to count it and is kept nowhere: not in a delivery, not in an attempt, not in
// the audit.
func TestSecurity_T115_WebhookAnswerBodyIsNeverKept(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	rc.next = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("ANSWER-CANARY " + strings.Repeat("x", 100)))
	}

	w := h.create(rc.srv.URL)

	h.disp.Publish(context.Background(), exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(context.Background())

	ds := h.deliveries(w.ID)
	if l := ds[0].AttemptLog; len(l) != 1 || l[0].ResponseBytes != len("ANSWER-CANARY ")+100 {
		t.Fatalf("attempt log = %+v", l)
	}

	raw := mustJSON(t, ds)

	audit, err := h.svc.AuditList(h.as(ui), service.AuditListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	auditRaw := mustJSON(t, audit)

	if strings.Contains(raw, "ANSWER-CANARY") || strings.Contains(auditRaw, "ANSWER-CANARY") {
		t.Fatal("the answer of the receiver was kept")
	}
}

func TestWebhookWaitsForALockedVaultWithoutCountingAnAttempt(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)

	secret, err := h.conns.Secret(w.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.conns.DeleteSecret(w.ID); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(ctx)

	if n := len(rc.all()); n != 0 {
		t.Fatalf("a delivery was sent with no secret: %d", n)
	}

	if ds := h.deliveries(w.ID); ds[0].State != core.StatePending || ds[0].Attempts != 0 || len(ds[0].AttemptLog) != 0 {
		t.Fatalf("delivery = %+v", ds[0])
	}

	// The vault opens again: the delivery is sent after its wait.
	if err := h.conns.PutSecret(w.ID, secret); err != nil {
		t.Fatal(err)
	}

	h.clock.advance(core.VaultLockedWait)
	h.disp.RunOnce(ctx)

	if n := len(rc.all()); n != 1 {
		t.Fatalf("the delivery was not sent after the unlock: %d", n)
	}

	if ds := h.deliveries(w.ID); ds[0].State != core.StateSucceeded {
		t.Fatalf("delivery = %+v", ds[0])
	}
}

func TestWebhookTestSendIsOneAttempt(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL, core.ExportReady)

	d, err := h.svc.WebhookTest(h.as(ui), service.WebhookRef{ID: w.ID})
	if err != nil || d.State != core.StateSucceeded || d.Event != core.Test || d.Kind != webhooks.KindTest {
		t.Fatalf("WebhookTest = %+v, %v", d, err)
	}

	got := rc.all()
	if len(got) != 1 || got[0].header.Get("Stuarty-Event") != core.Test {
		t.Fatalf("receiver got %v", got)
	}

	if err := core.Verify(
		w.Secret,
		got[0].header.Get(core.HeaderSignature),
		got[0].body,
		h.clock.now(),
		core.ReplayWindow,
	); err != nil {
		t.Fatal(err)
	}

	// A failing receiver: one attempt, no retry.
	rc.next = func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }

	d, err = h.svc.WebhookTest(h.as(ui), service.WebhookRef{ID: w.ID})
	if err != nil || d.State != core.StateFailed || d.Attempts != 1 {
		t.Fatalf("a failing test = %+v, %v", d, err)
	}

	// A webhook that is off can still be tested.
	off := false
	if _, err := h.svc.WebhookUpdate(h.as(ui), service.WebhookUpdateRequest{ID: w.ID, Enabled: &off}); err != nil {
		t.Fatal(err)
	}

	if d, err := h.svc.WebhookTest(h.as(ui), service.WebhookRef{ID: w.ID}); err != nil || d.Attempts != 1 {
		t.Fatalf("a test of a disabled webhook = %+v, %v", d, err)
	}
}

func TestWebhookRedeliverSendsTheStoredBodyAgainWithAFreshSignature(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)
	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())
	h.disp.RunOnce(ctx)

	first := h.deliveries(w.ID)[0]

	h.clock.advance(time.Hour)

	again, err := h.svc.WebhookRedeliver(h.as(ui), service.WebhookRedeliverRequest{ID: w.ID, DeliveryID: first.ID})
	if err != nil || again.ID == first.ID || again.Kind != webhooks.KindRedelivery || again.EventID != first.EventID {
		t.Fatalf("WebhookRedeliver = %+v, %v", again, err)
	}

	h.disp.RunOnce(ctx)

	got := rc.all()
	if len(got) != 2 || !bytes.Equal(got[0].body, got[1].body) ||
		got[0].header.Get(core.HeaderSignature) == got[1].header.Get(core.HeaderSignature) {
		t.Fatalf("receiver got %d request(s), same body or same signature", len(got))
	}

	if err := core.Verify(
		w.Secret,
		got[1].header.Get(core.HeaderSignature),
		got[1].body,
		h.clock.now(),
		core.ReplayWindow,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.WebhookRedeliver(
		h.as(ui),
		service.WebhookRedeliverRequest{ID: w.ID, DeliveryID: "wdl_9999999999"},
	); !isCode(
		err,
		service.CodeWebhookNotFound,
	) {
		t.Fatalf("an unknown delivery: %v", err)
	}
}

func TestWebhookOnlyReceivesTheEventsItChoseWhileEnabled(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL, core.ExportReady)
	ctx := context.Background()

	h.disp.Publish(ctx, core.JobFailedData{JobID: "job_0000000001", Kind: "export", State: "failed"}, h.clock.now())
	h.disp.Publish(ctx, exportReady("job_0000000002"), h.clock.now())
	// The same event twice is one delivery.
	h.disp.Publish(ctx, exportReady("job_0000000002"), h.clock.now())

	off := false
	if _, err := h.svc.WebhookUpdate(h.as(ui), service.WebhookUpdateRequest{ID: w.ID, Enabled: &off}); err != nil {
		t.Fatal(err)
	}

	h.disp.Publish(ctx, exportReady("job_0000000003"), h.clock.now())

	if n := len(h.deliveries(w.ID)); n != 1 {
		t.Fatalf("%d deliveries, want 1", n)
	}

	// While it is off nothing is sent; when it is on again the pending one goes.
	h.disp.RunOnce(ctx)

	if n := len(rc.all()); n != 0 {
		t.Fatalf("a disabled webhook received %d request(s)", n)
	}

	on := true
	if _, err := h.svc.WebhookUpdate(h.as(ui), service.WebhookUpdateRequest{ID: w.ID, Enabled: &on}); err != nil {
		t.Fatal(err)
	}

	h.disp.RunOnce(ctx)

	if n := len(rc.all()); n != 1 {
		t.Fatalf("receiver got %d request(s), want 1", n)
	}
}

// The export of an image is an export: its job is born finished and on no stream, and a webhook still hears it.
func TestWebhookHearsTheExportOfAnImage(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL, core.ExportReady)
	ctx := h.as(ui)

	created, err := h.svc.ImageCreate(ctx, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "SECRET TITLE"})
	if err != nil {
		t.Fatal(err)
	}

	out, err := h.svc.ImageExport(ctx, service.ImageExportRequest{ImageID: created.ImageID, Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}

	h.disp.RunOnce(context.Background())

	got := rc.all()
	if len(got) != 1 || got[0].header.Get("Stuarty-Event") != core.ExportReady {
		t.Fatalf("the receiver got %d request(s)", len(got))
	}

	body := string(got[0].body)
	if !strings.Contains(body, out.JobID) || !strings.Contains(body, `"kind":"image.export"`) ||
		strings.Contains(body, "SECRET TITLE") || strings.Contains(body, "exports") ||
		strings.Contains(body, "project_id") {
		t.Fatalf("body = %s", body)
	}

	if ds := h.deliveries(w.ID); len(ds) != 1 || ds[0].State != core.StateSucceeded {
		t.Fatalf("deliveries = %+v", ds)
	}
}

// Acceptance criterion 1: with no webhook registered nothing is written for an event and nothing is dialled, and with
// every webhook removed the same.
func TestWebhookNoneRegisteredOpensNoConnection(t *testing.T) {
	var dials atomic.Int32

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		dials.Add(1)

		return outbound.ErrNotLoopback
	})
	defer restore()

	h := newHookEnv(t)
	ctx := context.Background()

	h.disp.Publish(ctx, exportReady("job_0000000009"), h.clock.now())

	if n := h.disp.RunOnce(ctx); n != 0 || dials.Load() != 0 {
		t.Fatalf("RunOnce took %d, dials %d", n, dials.Load())
	}

	rc := newReceiver(t)
	w := h.create(rc.srv.URL)

	if _, err := h.svc.WebhookRemove(h.as(ui), service.WebhookRef{ID: w.ID}); err != nil {
		t.Fatal(err)
	}

	h.disp.Publish(ctx, exportReady("job_0000000008"), h.clock.now())
	h.disp.RunOnce(ctx)

	if dials.Load() != 0 || len(rc.all()) != 0 {
		t.Fatalf("dials %d, requests %d", dials.Load(), len(rc.all()))
	}

	if _, err := h.conns.Secret(w.ID); err == nil {
		t.Fatal("the secret of a removed webhook is still in the vault")
	}
}

// T-113: the signing secret is in the answer of the creation and of the rotation, and in nothing else: no read, no
// update, no delivery, no audit row, no error.
func TestSecurity_T113_WebhookSecretIsShownOnce(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)
	ctx := h.as(ui)

	if !strings.HasPrefix(w.Secret, core.SecretPrefix) || !w.SecretSet {
		t.Fatalf("created = %+v", w)
	}

	stored, err := h.conns.Secret(w.ID)
	if err != nil || string(stored) != w.Secret {
		t.Fatalf("the vault holds %q, %v", stored, err)
	}

	list, _ := h.svc.WebhookList(ctx, service.WebhookListRequest{})
	get, _ := h.svc.WebhookGet(ctx, service.WebhookRef{ID: w.ID})
	name := "renamed"
	upd, _ := h.svc.WebhookUpdate(ctx, service.WebhookUpdateRequest{ID: w.ID, Name: &name})
	test, _ := h.svc.WebhookTest(ctx, service.WebhookRef{ID: w.ID})
	dels, _ := h.svc.WebhookDeliveryList(ctx, service.WebhookDeliveriesRequest{ID: w.ID})
	events, _ := h.svc.WebhookEventList(ctx, service.WebhookEventListRequest{})
	audit, _ := h.svc.AuditList(ctx, service.AuditListRequest{})
	_, notFound := h.svc.WebhookGet(ctx, service.WebhookRef{ID: "whk_9999999999"})

	for name, v := range map[string]any{
		"list": list, "get": get, "update": upd, "test": test, "deliveries": dels, "events": events, "audit": audit,
		"error": notFound,
	} {
		raw := mustJSON(t, v)
		if notFound != nil && name == "error" {
			raw = notFound.Error()
		}

		if strings.Contains(raw, w.Secret) || strings.Contains(raw, `"secret"`) {
			t.Errorf("%s carries the secret: %s", name, raw)
		}
	}

	// A rotation answers with a new secret, once; the old one no longer signs.
	rot, err := h.svc.WebhookRotateSecret(ctx, service.WebhookRef{ID: w.ID})
	if err != nil || rot.Secret == w.Secret || !strings.HasPrefix(rot.Secret, core.SecretPrefix) {
		t.Fatalf("WebhookRotateSecret = %+v, %v", rot, err)
	}

	if _, err := h.svc.WebhookTest(ctx, service.WebhookRef{ID: w.ID}); err != nil {
		t.Fatal(err)
	}

	got := rc.all()
	last := got[len(got)-1]

	if core.Verify(
		w.Secret,
		last.header.Get(core.HeaderSignature),
		last.body,
		h.clock.now(),
		core.ReplayWindow,
	) == nil {
		t.Error("the old secret still verifies after a rotation")
	}

	if err := core.Verify(
		rot.Secret,
		last.header.Get(core.HeaderSignature),
		last.body,
		h.clock.now(),
		core.ReplayWindow,
	); err != nil {
		t.Errorf("the new secret does not verify: %v", err)
	}

	for _, line := range []string{string(h.conns.Redact([]byte(rot.Secret)))} {
		if strings.Contains(line, rot.Secret) {
			t.Error("the vault does not redact the secrets")
		}
	}
}

// T-110 at the service: an address that reaches an internal range is refused when it is saved, with a field path and
// no webhook, secret or row left behind.
func TestSecurity_T110_WebhookCreateRefusesInternalAddresses(t *testing.T) {
	h := newHookEnv(t)

	for _, url := range []string{
		"http://hooks.example.com/h", "https://10.0.0.5/h", "https://192.168.1.1/h", "https://169.254.169.254/latest/meta-data",
		"https://100.100.100.200/", "https://[fd00::1]/h", "https://[::ffff:10.0.0.1]/h", "https://db.internal/h",
		"https://user:pw@hooks.example.com/h", "ftp://hooks.example.com/h", "https://hooks.example.com/h#frag", "file:///etc/passwd",
	} {
		_, err := h.svc.WebhookCreate(
			h.as(ui),
			service.WebhookCreateRequest{Name: "x", URL: url, Events: []string{core.ExportReady}},
		)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != "E_VALIDATION" || se.Path != "url" {
			t.Errorf("%s: err = %v, want E_VALIDATION on url", url, err)
		}
	}

	list, _ := h.svc.WebhookList(h.as(ui), service.WebhookListRequest{})
	if len(list.Webhooks) != 0 {
		t.Fatalf("%d webhooks were left behind", len(list.Webhooks))
	}

	// An update cannot move a webhook to such an address either.
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)
	bad := "https://10.0.0.5/h"

	if _, err := h.svc.WebhookUpdate(h.as(ui), service.WebhookUpdateRequest{ID: w.ID, URL: &bad}); err == nil {
		t.Fatal("an update moved a webhook to a private address")
	}
}

func TestWebhookCreateValidatesAndLimits(t *testing.T) {
	h := newHookEnv(t)
	ctx := h.as(ui)

	for name, req := range map[string]service.WebhookCreateRequest{
		"no name":     {URL: "https://hooks.example.com/h", Events: []string{core.ExportReady}},
		"no events":   {Name: "x", URL: "https://hooks.example.com/h"},
		"test event":  {Name: "x", URL: "https://hooks.example.com/h", Events: []string{core.Test}},
		"bad event":   {Name: "x", URL: "https://hooks.example.com/h", Events: []string{"project.deleted"}},
		"long name":   {Name: strings.Repeat("a", 61), URL: "https://hooks.example.com/h", Events: []string{core.ExportReady}},
		"no address":  {Name: "x", Events: []string{core.ExportReady}},
		"dup events":  {Name: "x", URL: "https://hooks.example.com/h", Events: []string{core.ExportReady, core.ExportReady}},
		"control":     {Name: "x" + string(rune(1)) + "y", URL: "https://hooks.example.com/h", Events: []string{core.ExportReady}},
		"space in id": {Name: "x", URL: "https://hooks.example.com/a b", Events: []string{core.ExportReady}},
	} {
		if _, err := h.svc.WebhookCreate(ctx, req); !isCode(err, "E_VALIDATION") {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	for range core.MaxWebhooks {
		if _, err := h.svc.WebhookCreate(ctx, service.WebhookCreateRequest{
			Name: "ok", URL: "https://hooks.example.com/h", Events: []string{core.ExportReady},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := h.svc.WebhookCreate(ctx, service.WebhookCreateRequest{
		Name: "one too many", URL: "https://hooks.example.com/h", Events: []string{core.ExportReady},
	}); !isCode(err, "E_LIMIT_EXCEEDED") {
		t.Fatalf("the 21st webhook: %v", err)
	}
}

// Updates at once do not undo each other, and creations at once do not pass the limit.
func TestWebhookWritesAtOnceKeepEachChangeAndTheLimit(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)
	ctx := h.as(ui)

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(2)

		address := rc.srv.URL + "/moved"
		name := "renamed"

		go func() {
			defer wg.Done()

			if _, err := h.svc.WebhookUpdate(ctx, service.WebhookUpdateRequest{ID: w.ID, Name: &name}); err != nil {
				t.Errorf("update %d of the name: %v", i, err)
			}
		}()

		go func() {
			defer wg.Done()

			if _, err := h.svc.WebhookUpdate(ctx, service.WebhookUpdateRequest{ID: w.ID, URL: &address}); err != nil {
				t.Errorf("update %d of the address: %v", i, err)
			}
		}()
	}

	wg.Wait()

	got, err := h.svc.WebhookGet(ctx, service.WebhookRef{ID: w.ID})
	if err != nil || got.Name != "renamed" || got.URL != rc.srv.URL+"/moved" {
		t.Fatalf("after the updates: %+v, %v", got, err)
	}

	for range core.MaxWebhooks + 6 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := h.svc.WebhookCreate(ctx, service.WebhookCreateRequest{
				Name: "ok", URL: "https://hooks.example.com/h", Events: []string{core.ExportReady},
			})
			if err != nil && !isCode(err, "E_LIMIT_EXCEEDED") {
				t.Errorf("WebhookCreate: %v", err)
			}
		}()
	}

	wg.Wait()

	list, err := h.svc.WebhookList(ctx, service.WebhookListRequest{})
	if err != nil || len(list.Webhooks) != core.MaxWebhooks {
		t.Fatalf("%d webhooks after creations at once, %v", len(list.Webhooks), err)
	}
}

// T-116: a webhook is a destination, so only an administrator makes one: a caller without the scope, and a key limited to
// one project, are refused for every operation.
func TestSecurity_T116_WebhooksNeedTheAdminScopeAndNoProjectLimit(t *testing.T) {
	h := newHookEnv(t)
	rc := newReceiver(t)
	w := h.create(rc.srv.URL)

	readOnly := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_0000000001", Scopes: []string{service.ScopeRead}},
	)
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_0000000002", Scopes: service.AllScopes, ProjectID: "prj_0000000001",
	})

	calls := map[string]func(ctx context.Context) error{
		"list": func(ctx context.Context) error {
			_, err := h.svc.WebhookList(ctx, service.WebhookListRequest{})
			return err
		},
		"get": func(ctx context.Context) error {
			_, err := h.svc.WebhookGet(ctx, service.WebhookRef{ID: w.ID})
			return err
		},
		"create": func(ctx context.Context) error {
			_, err := h.svc.WebhookCreate(
				ctx,
				service.WebhookCreateRequest{Name: "x", URL: rc.srv.URL, Events: []string{core.ExportReady}},
			)
			return err
		},
		"update": func(ctx context.Context) error {
			_, err := h.svc.WebhookUpdate(ctx, service.WebhookUpdateRequest{ID: w.ID})
			return err
		},
		"remove": func(ctx context.Context) error {
			_, err := h.svc.WebhookRemove(ctx, service.WebhookRef{ID: w.ID})
			return err
		},
		"rotate": func(ctx context.Context) error {
			_, err := h.svc.WebhookRotateSecret(ctx, service.WebhookRef{ID: w.ID})
			return err
		},
		"test": func(ctx context.Context) error {
			_, err := h.svc.WebhookTest(ctx, service.WebhookRef{ID: w.ID})
			return err
		},
		"deliveries": func(ctx context.Context) error {
			_, err := h.svc.WebhookDeliveryList(ctx, service.WebhookDeliveriesRequest{ID: w.ID})
			return err
		},
		"redeliver": func(ctx context.Context) error {
			_, err := h.svc.WebhookRedeliver(
				ctx,
				service.WebhookRedeliverRequest{ID: w.ID, DeliveryID: "wdl_0000000001"},
			)
			return err
		},
	}

	for name, call := range calls {
		for who, ctx := range map[string]context.Context{"read-only key": readOnly, "project key": limited, "no caller": context.Background()} {
			if err := call(ctx); err == nil {
				t.Errorf("%s was allowed for a %s", name, who)
			}
		}
	}

	if n := len(rc.all()); n != 0 {
		t.Fatalf("a refused call reached the receiver: %d", n)
	}

	// The list of events carries nothing of the workspace: a read-only key may read it.
	if _, err := h.svc.WebhookEventList(readOnly, service.WebhookEventListRequest{}); err != nil {
		t.Fatalf("WebhookEventList for a read-only key: %v", err)
	}
}
