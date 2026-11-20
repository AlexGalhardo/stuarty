package destinations

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AlexGalhardo/stuarty/service/social"
)

const hookSecret = "a-long-signing-secret-of-32-bytes"

type delivery struct {
	header http.Header
	body   []byte
}

// receiver is the automation of the creator: it records every delivery and answers what the test says.
type receiver struct {
	srv *httptest.Server

	mu      sync.Mutex
	got     []delivery
	answers []func(w http.ResponseWriter)
}

func newReceiver(t *testing.T, answers ...func(w http.ResponseWriter)) *receiver {
	t.Helper()

	r := &receiver{answers: answers}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)

		r.mu.Lock()
		defer r.mu.Unlock()

		r.got = append(r.got, delivery{header: req.Header.Clone(), body: body})

		if n := len(r.got); n <= len(r.answers) {
			r.answers[n-1](w)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	t.Cleanup(r.srv.Close)

	return r
}

func status(code int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func hookSecretJSON(t *testing.T, extra string) string {
	t.Helper()

	s := `{"signing_secret":"` + hookSecret + `"` + extra + `}`

	return s
}

// clock moves by a second each time it is read, so every attempt of a send has its own timestamp.
func clock() func() time.Time {
	var mu sync.Mutex

	t := fixedNow

	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()

		t = t.Add(time.Second)

		return t
	}
}

type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.d = append(s.d, d)

	return nil
}

// sendHook delivers the test post to a receiver on loopback, so the kind is the local webhook.
func sendHook(t *testing.T, r *receiver, secret string, prev *Remote, s *sleeps) (Result, error) {
	t.Helper()

	p, _ := testPost(t)

	d := newWebhook(Config{Now: clock(), Sleep: s.sleep}, KindWebhookLocal)

	return d.Send(context.Background(), &SendInput{
		Cred: cred(r.srv.URL, secret, gateLike()), Package: p, Prev: prev, Delivery: "tgt_0123456789",
	})
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	raw, err := os.ReadFile("testdata/webhook-payload.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("mem:///payload.json", doc); err != nil {
		t.Fatal(err)
	}

	s, err := c.Compile("mem:///payload.json")
	if err != nil {
		t.Fatal(err)
	}

	return s
}

// A delivery carries a valid signature and a timestamp, and its body is the documented payload.
func TestWebhookSignedPayload(t *testing.T) {
	t.Parallel()

	r := newReceiver(t)
	s := &sleeps{}

	res, err := sendHook(
		t,
		r,
		hookSecretJSON(t, `,"media_base_url":"https://stuarty.example"`),
		nil,
		s,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(r.got) != 1 || res.RemoteID != "tgt_0123456789" || res.State != StatusPublished {
		t.Fatalf("got %d deliveries, result %+v", len(r.got), res)
	}

	d := r.got[0]
	ts := d.header.Get(HeaderTimestamp)

	if d.header.Get("Content-Type") != "application/json" || d.header.Get(HeaderDelivery) != "tgt_0123456789" ||
		ts == "" {
		t.Errorf("headers: %v", d.header)
	}

	now := time.Unix(fixedNow.Unix()+5, 0)
	if err := Verify(hookSecret, ts, "tgt_0123456789", d.header.Get(HeaderSignature), d.body, now); err != nil {
		t.Errorf("a receiver cannot verify the delivery: %v", err)
	}

	var doc any
	if err := json.Unmarshal(d.body, &doc); err != nil {
		t.Fatal(err)
	}

	if err := compileSchema(t).Validate(doc); err != nil {
		t.Errorf("the payload does not match its schema: %v", err)
	}

	var p webhookPayload

	_ = json.Unmarshal(d.body, &p)

	if p.Event != "post.published" || p.Status != "published" || p.Post.Slug != "how-i-cut-a-video" ||
		len(p.Images) != 3 ||
		p.Post.Cover != "images/cover.png" ||
		!strings.Contains(p.Post.HTML, `src="images/1-the-desk.png"`) ||
		p.Images[0].URL != "https://stuarty.example/api/v1/posts/how-i-cut-a-video/media/cover.png" {
		t.Errorf("payload = %+v", p)
	}
}

func TestWebhookUpdateEventAndNoMediaBase(t *testing.T) {
	t.Parallel()

	r := newReceiver(t)

	if _, err := sendHook(
		t,
		r,
		hookSecretJSON(t, ""),
		&Remote{RemoteID: "x"},
		&sleeps{},
	); err != nil {
		t.Fatal(err)
	}

	var p webhookPayload

	_ = json.Unmarshal(r.got[0].body, &p)

	if p.Event != "post.updated" || p.Images[0].URL != "" {
		t.Errorf("event %q, url %q", p.Event, p.Images[0].URL)
	}
}

// A signature binds the body, the timestamp and the delivery id; an old delivery is refused (T-87).
func TestSecurity_T87_WebhookSignatureBindsTimeAndDelivery(t *testing.T) {
	t.Parallel()

	body := []byte(`{"post":"x"}`)
	ts := strconv.FormatInt(fixedNow.Unix(), 10)
	sig := Sign(hookSecret, ts, "tgt_a", body)
	now := fixedNow.Add(time.Minute)

	if err := Verify(hookSecret, ts, "tgt_a", sig, body, now); err != nil {
		t.Fatalf("a good delivery: %v", err)
	}

	cases := map[string]func() error{
		"another body": func() error { return Verify(hookSecret, ts, "tgt_a", sig, []byte(`{"post":"y"}`), now) },
		"another timestamp": func() error {
			return Verify(hookSecret, strconv.FormatInt(fixedNow.Unix()+1, 10), "tgt_a", sig, body, now)
		},
		"another delivery": func() error { return Verify(hookSecret, ts, "tgt_b", sig, body, now) },
		"another secret":   func() error { return Verify(hookSecret+"x", ts, "tgt_a", sig, body, now) },
		"a replay later": func() error {
			return Verify(hookSecret, ts, "tgt_a", sig, body, fixedNow.Add(ReplayWindow+time.Second))
		},
		"from the future": func() error {
			return Verify(hookSecret, ts, "tgt_a", sig, body, fixedNow.Add(-ReplayWindow-time.Second))
		},
		"no version": func() error {
			return Verify(hookSecret, ts, "tgt_a", strings.TrimPrefix(sig, SignatureVersion), body, now)
		},
		"a bad timestamp": func() error { return Verify(hookSecret, "yesterday", "tgt_a", sig, body, now) },
	}

	for name, f := range cases {
		if f() == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// The real send signs with the delivery id and a timestamp of its own at every attempt.
	r := newReceiver(t, status(http.StatusServiceUnavailable))

	if _, err := sendHook(t, r, hookSecretJSON(t, ""), nil, &sleeps{}); err != nil {
		t.Fatal(err)
	}

	if len(r.got) != 2 || r.got[0].header.Get(HeaderTimestamp) == r.got[1].header.Get(HeaderTimestamp) ||
		!bytes.Equal(
			r.got[0].body,
			r.got[1].body,
		) || r.got[0].header.Get(HeaderDelivery) != r.got[1].header.Get(HeaderDelivery) {
		t.Errorf("a retry must keep the body and the delivery id and move the timestamp: %d deliveries", len(r.got))
	}
}

// A transient failure is retried with a backoff; the others are not.
func TestWebhookRetries(t *testing.T) {
	t.Parallel()

	t.Run("transient then ok", func(t *testing.T) {
		t.Parallel()

		r := newReceiver(t, status(http.StatusBadGateway), status(http.StatusServiceUnavailable))
		s := &sleeps{}

		if _, err := sendHook(t, r, hookSecretJSON(t, ""), nil, s); err != nil {
			t.Fatal(err)
		}

		if len(r.got) != 3 || len(s.d) != 2 || s.d[0] != time.Second || s.d[1] != 2*time.Second {
			t.Errorf("%d deliveries, waits %v", len(r.got), s.d)
		}
	})

	t.Run("retry after is honoured and capped", func(t *testing.T) {
		t.Parallel()

		slow := func(sec string) func(w http.ResponseWriter) {
			return func(w http.ResponseWriter) {
				w.Header().Set("Retry-After", sec)
				w.WriteHeader(http.StatusTooManyRequests)
			}
		}
		r := newReceiver(t, slow("7"), slow("9999"))
		s := &sleeps{}

		if _, err := sendHook(t, r, hookSecretJSON(t, ""), nil, s); err != nil {
			t.Fatal(err)
		}

		if len(s.d) != 2 || s.d[0] != 7*time.Second || s.d[1] != 30*time.Second {
			t.Errorf("waits %v", s.d)
		}
	})

	t.Run("gives up and says unavailable", func(t *testing.T) {
		t.Parallel()

		r := newReceiver(t, slices.Repeat([]func(http.ResponseWriter){status(503)}, 5)...)
		s := &sleeps{}

		_, err := sendHook(t, r, hookSecretJSON(t, ""), nil, s)

		se, ok := social.AsError(err)
		if !ok || se.Kind != social.KindUnavailable || se.Reason != "http_503" || len(r.got) != 4 || len(s.d) != 3 {
			t.Errorf("err %v, %d deliveries, waits %v", err, len(r.got), s.d)
		}
	})

	for name, c := range map[string]struct {
		code   int
		reason string
	}{
		"bad request":  {400, "http_400"},
		"unauthorized": {401, "http_401"},
		"not found":    {404, "http_404"},
		"redirect":     {302, "redirect"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newReceiver(t, func(w http.ResponseWriter) {
				w.Header().Set("Location", "https://other.example/x")
				w.WriteHeader(c.code)
				_, _ = w.Write([]byte("SECRET-TEXT-OF-THE-RECEIVER"))
			})
			s := &sleeps{}

			_, err := sendHook(t, r, hookSecretJSON(t, ""), nil, s)

			se, ok := social.AsError(err)
			if !ok || se.Kind != social.KindRejected || se.Reason != c.reason || len(r.got) != 1 || len(s.d) != 0 ||
				strings.Contains(err.Error(), "SECRET-TEXT") {
				t.Errorf("err %v, %d deliveries", err, len(r.got))
			}
		})
	}
}

func TestWebhookSettingsAndAddressRules(t *testing.T) {
	t.Parallel()

	public := newWebhook(testConfig(), KindWebhook)
	local := newWebhook(testConfig(), KindWebhookLocal)
	ok := []byte(hookSecretJSON(t, ""))

	for _, endpoint := range []string{"https://hooks.example.com/stuarty", "https://8.8.8.8/x", "https://203.0.113.9:8443/p"} {
		if err := public.ValidateConfig(ok, endpoint); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}

	for _, endpoint := range []string{
		"http://127.0.0.1:8080/x", "https://localhost/x", "https://app.localhost/x", "https://10.1.2.3/x", "https://192.168.0.5/x",
		"https://172.20.0.1/x", "https://[::1]/x", "https://[fd00::1]/x", "https://169.254.169.254/x", "https://100.64.1.1/x",
		"https://0.0.0.0/x", "https://[::ffff:10.0.0.1]/x", "https://LOCALHOST./x",
	} {
		if err := public.ValidateConfig(ok, endpoint); err == nil {
			t.Errorf("%s was accepted by the public webhook", endpoint)
		}

		if err := local.ValidateConfig(ok, endpoint); err != nil {
			t.Errorf("%s was refused by the local webhook: %v", endpoint, err)
		}
	}

	for name, secret := range map[string]string{
		"short secret":   `{"signing_secret":"short"}`,
		"no secret":      `{}`,
		"unknown member": hookSecretJSON(t, `,"retries":9`),
		"http media":     hookSecretJSON(t, `,"media_base_url":"http://stuarty.example"`),
		"media query":    hookSecretJSON(t, `,"media_base_url":"https://stuarty.example/?a=1"`),
		"media user":     hookSecretJSON(t, `,"media_base_url":"https://u:p@stuarty.example"`),
	} {
		if err := public.ValidateConfig([]byte(secret), "https://hooks.example.com/x"); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if res, err := public.Test(
		context.Background(),
		cred("https://hooks.example.com/x", string(ok), nil),
	); err != nil ||
		!res.OK {
		t.Errorf("test = %+v %v", res, err)
	}

	if res, _ := public.Test(context.Background(), cred("https://10.0.0.1/x", string(ok), nil)); res.OK {
		t.Error("a private address tested ok for a public webhook")
	}
}
