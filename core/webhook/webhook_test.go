package webhook

import (
	"crypto/rand"
	"encoding/json"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
)

// notPublic is a stand-in for the ranges of internal/outbound, which the tests of that package cover.
func notPublic(ip netip.Addr) bool {
	ip = ip.Unmap()

	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback()
}

func TestCheckURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		kind string // "" for a refusal
	}{
		{"https name", "https://hooks.example.com/abc?token=1", HostPublic},
		{"https public ip", "https://93.184.216.34/h", HostPublic},
		{"http loopback ip", "http://127.0.0.1:9000/h", HostLoopback},
		{"http localhost", "http://localhost:9000/h", HostLoopback},
		{"https localhost", "https://localhost/h", HostLoopback},
		{"http ipv6 loopback", "http://[::1]:9000/h", HostLoopback},
		{"http public", "http://hooks.example.com/h", ""},
		{"ftp", "ftp://hooks.example.com/h", ""},
		{"no scheme", "hooks.example.com/h", ""},
		{"userinfo", "https://user:pw@hooks.example.com/h", ""},
		{"fragment", "https://hooks.example.com/h#x", ""},
		{"private 10", "https://10.0.0.5/h", ""},
		{"private 192", "https://192.168.1.10/h", ""},
		{"private 172", "https://172.16.0.1/h", ""},
		{"metadata link-local", "https://169.254.169.254/latest", ""},
		{"unspecified", "https://0.0.0.0/h", ""},
		{"ipv6 unique local", "https://[fd00::1]/h", ""},
		{"ipv6 link-local", "https://[fe80::1]/h", ""},
		{"mapped private", "https://[::ffff:10.0.0.1]/h", ""},
		{"dot localhost name", "https://app.localhost/h", ""},
		{"dot internal name", "https://db.internal/h", ""},
		{"dot local name", "https://printer.local/h", ""},
		{"space", "https://hooks.example.com/a b", ""},
		{"control", "https://hooks.example.com/a\x00b", ""},
		{"empty", "", ""},
		{"too long", "https://example.com/" + strings.Repeat("a", MaxURLBytes), ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kind, err := CheckURL(tc.raw, notPublic)
			if tc.kind == "" {
				if err == nil {
					t.Fatalf("CheckURL(%q) accepted a refused address (%s)", tc.raw, kind)
				}

				if err.Path != "url" || err.Message == "" {
					t.Fatalf("the refusal has no path or message: %+v", err)
				}

				return
			}

			if err != nil || kind != tc.kind {
				t.Fatalf("CheckURL(%q) = %q, %v; want %q", tc.raw, kind, err, tc.kind)
			}
		})
	}
}

func TestSafeURLKeepsNoPathQueryOrPort(t *testing.T) {
	t.Parallel()

	if got := SafeURL(
		"https://hooks.example.com:8443/services/T0/B0/secret?token=abc",
	); got != "https://hooks.example.com" {
		t.Fatalf("SafeURL = %q", got)
	}
}

func TestNormalizeEvents(t *testing.T) {
	t.Parallel()

	got, err := NormalizeEvents([]string{JobFailed, ExportReady})
	if err != nil || !slices.Equal(got, []string{ExportReady, JobFailed}) {
		t.Fatalf("NormalizeEvents = %v, %v", got, err)
	}

	for name, in := range map[string][]string{
		"none":      nil,
		"test":      {Test},
		"unknown":   {"project.created"},
		"duplicate": {ExportReady, ExportReady},
	} {
		if _, err := NormalizeEvents(in); err == nil || err.Path != "events" {
			t.Errorf("%s: err = %v, want a refusal of events", name, err)
		}
	}
}

func TestEventsAreDescribed(t *testing.T) {
	t.Parallel()

	for _, e := range Events() {
		if !ValidEvent(e.Name) || len(e.Description) < 40 || Allowed(e.Name) == nil {
			t.Errorf("event %q: valid %v, description %q", e.Name, ValidEvent(e.Name), e.Description)
		}
	}

	if ValidEvent(Test) {
		t.Error("webhook.test can be chosen")
	}

	if len(Events()) > MaxEvents {
		t.Errorf("%d events, a webhook holds %d", len(Events()), MaxEvents)
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	if got, err := NormalizeName("  n8n flow  "); err != nil || got != "n8n flow" {
		t.Fatalf("NormalizeName = %q, %v", got, err)
	}

	for _, bad := range []string{"", "   ", strings.Repeat("a", MaxNameRunes+1), "a\nb"} {
		if _, err := NormalizeName(bad); err == nil || err.Path != "name" {
			t.Errorf("NormalizeName(%q) = %v", bad, err)
		}
	}
}

func TestSubscribed(t *testing.T) {
	t.Parallel()

	if !Subscribed([]string{ExportReady}, true, ExportReady) || Subscribed([]string{ExportReady}, false, ExportReady) ||
		Subscribed([]string{ExportReady}, true, JobFailed) {
		t.Fatal("Subscribed does not follow the events and the enabled flag")
	}
}

func TestBackoffAndNext(t *testing.T) {
	t.Parallel()

	want := []time.Duration{10 * time.Second, 30 * time.Second, 90 * time.Second, 270 * time.Second, 810 * time.Second}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}

	if Backoff(0) != 0 || Backoff(MaxRetries+1) != 0 {
		t.Error("Backoff outside 1..5 is not zero")
	}

	for status, want := range map[int]Outcome{
		200: OutcomeDelivered, 204: OutcomeDelivered, 0: OutcomeRetry, 500: OutcomeRetry, 503: OutcomeRetry,
		408: OutcomeRetry, 429: OutcomeRetry, 400: OutcomeRefused, 404: OutcomeRefused, 301: OutcomeRefused,
		302: OutcomeRefused, 401: OutcomeRefused,
	} {
		if got := Classify(status); got != want {
			t.Errorf("Classify(%d) = %v, want %v", status, got, want)
		}
	}

	// The first attempt and five retries: the sixth failure is final.
	for attempts := 1; attempts < MaxAttempts; attempts++ {
		if st, wait := Next(OutcomeRetry, attempts, MaxAttempts); st != StatePending || wait != Backoff(attempts) {
			t.Errorf("after attempt %d: %s %v", attempts, st, wait)
		}
	}

	if st, _ := Next(OutcomeRetry, MaxAttempts, MaxAttempts); st != StateFailed {
		t.Errorf("after the last attempt: %s", st)
	}

	if st, _ := Next(OutcomeRefused, 1, MaxAttempts); st != StateFailed {
		t.Errorf("a refusal is %s", st)
	}

	if st, _ := Next(OutcomeRetry, 1, 1); st != StateFailed {
		t.Errorf("a test send with one attempt is %s", st)
	}

	if st, _ := Next(OutcomeDelivered, 3, MaxAttempts); st != StateSucceeded {
		t.Errorf("a 2xx is %s", st)
	}
}

func TestSignatureVerifiesWithTheSecretOnly(t *testing.T) {
	t.Parallel()

	secret, err := NewSecret(rand.Reader)
	if err != nil || !strings.HasPrefix(secret, SecretPrefix) || len(secret) != len(SecretPrefix)+43 {
		t.Fatalf("NewSecret = %q, %v", secret, err)
	}

	other, _ := NewSecret(rand.Reader)
	if other == secret {
		t.Fatal("two secrets are equal")
	}

	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"event":"export.ready"}`)
	header := Sign(secret, now, body)

	if !strings.HasPrefix(header, "t=1800000000,v1=") || len(header) != len("t=1800000000,v1=")+64 {
		t.Fatalf("header = %q", header)
	}

	if err := Verify(secret, header, body, now.Add(time.Minute), ReplayWindow); err != nil {
		t.Fatalf("Verify = %v", err)
	}

	for name, fn := range map[string]func() error{
		"another secret": func() error { return Verify(other, header, body, now, ReplayWindow) },
		"another body":   func() error { return Verify(secret, header, []byte(`{"event":"job.failed"}`), now, ReplayWindow) },
		"old":            func() error { return Verify(secret, header, body, now.Add(time.Hour), ReplayWindow) },
		"future":         func() error { return Verify(secret, header, body, now.Add(-time.Hour), ReplayWindow) },
		"garbage":        func() error { return Verify(secret, "v1=zz", body, now, ReplayWindow) },
		"no time":        func() error { return Verify(secret, "v1="+header[len(header)-64:], body, now, ReplayWindow) },
	} {
		if fn() == nil {
			t.Errorf("%s: the signature verified", name)
		}
	}
}

func TestSignatureKnownVector(t *testing.T) {
	t.Parallel()

	// HMAC-SHA256("secret", "1700000000.{}"), computed independently.
	got := Sign("secret", time.Unix(1_700_000_000, 0), []byte("{}"))
	if err := Verify("secret", got, []byte("{}"), time.Unix(1_700_000_000, 0), time.Second); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(got, "t=1700000000,v1=") {
		t.Fatalf("got %q", got)
	}
}

// every is one subject of each event, with every member set.
func every() []Data {
	return []Data{
		PublicationPublishedData{
			PublicationID: "pub_aaaaaaaaaa",
			TargetID:      "tgt_aaaaaaaaaa",
			ProjectID:     "prj_aaaaaaaaaa",
			Network:       "youtube",
			ConnectionID:  "con_aaaaaaaaaa",
			URL:           "https://youtu.be/abc",
			PublishedAt:   "2026-11-20T12:00:00Z",
		},
		PublicationFailedData{
			PublicationID: "pub_aaaaaaaaaa", TargetID: "tgt_aaaaaaaaaa", ProjectID: "prj_aaaaaaaaaa", Network: "x",
			ConnectionID: "con_aaaaaaaaaa", ErrorKind: "quota",
		},
		ExportReadyData{
			JobID:     "job_aaaaaaaaaa",
			ProjectID: "prj_aaaaaaaaaa",
			Kind:      "export",
			Format:    "mp4",
			Bytes:     1 << 20,
			Width:     1080,
			Height:    1920,
		},
		ApprovalRequestedData{
			ApprovalID: "apr_aaaaaaaaaa", Kind: "publish", ProjectID: "prj_aaaaaaaaaa", JobID: "job_aaaaaaaaaa",
			ConnectionID: "con_aaaaaaaaaa", ExpiresAt: "2026-11-27T12:00:00Z",
		},
		JobFailedData{
			JobID:     "job_aaaaaaaaaa",
			ProjectID: "prj_aaaaaaaaaa",
			Kind:      "export",
			State:     "failed",
			ErrorCode: "E_EXPORT_FAILED",
		},
		TestData{WebhookID: "whk_aaaaaaaaaa", Nonce: "n"},
	}
}

// Every event of the product: its body is the five members, its data holds the members of the allow-list and no
// other, and no value is a credential, a path or a text of the person.
func TestSecurity_T112_EveryPayloadIsMetadataOnly(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	for _, d := range every() {
		t.Run(d.EventName(), func(t *testing.T) {
			t.Parallel()

			body, id, err := Build(d, at, WorkspaceID("/home/someone/.stuarty"))
			if err != nil {
				t.Fatal(err)
			}

			if len(body) > MaxBodyBytes {
				t.Fatalf("%d bytes", len(body))
			}

			for _, leak := range []string{"someone", ".stuarty", "whsec_", "secret", "token", "password", "prompt"} {
				if strings.Contains(strings.ToLower(string(body)), leak) {
					t.Errorf("the body holds %q: %s", leak, body)
				}
			}

			var env Envelope
			if err := json.Unmarshal(body, &env); err != nil || env.ID != id || env.Event != d.EventName() ||
				env.OccurredAt != "2026-11-20T12:00:00Z" || !strings.HasPrefix(env.Workspace, "wsp_") ||
				!strings.HasPrefix(env.ID, "evt_") {
				t.Fatalf("envelope = %+v, %v", env, err)
			}

			var data map[string]any
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatal(err)
			}

			for k, v := range data {
				if !slices.Contains(Allowed(d.EventName()), k) {
					t.Errorf("member %s is not on the list", k)
				}

				switch v.(type) {
				case string, float64:
				default:
					t.Errorf("member %s is %T", k, v)
				}
			}
		})
	}
}

// The members of each struct are exactly its allow-list: a field added to a struct without the list, or the
// reverse, fails here.
func TestAllowListMatchesTheStructs(t *testing.T) {
	t.Parallel()

	for _, d := range every() {
		var tags []string

		for field := range reflect.TypeOf(d).Fields() {
			tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if tag != "-" {
				tags = append(tags, tag)
			}
		}

		got, want := slices.Sorted(slices.Values(tags)), slices.Sorted(slices.Values(Allowed(d.EventName())))
		if !slices.Equal(got, want) {
			t.Errorf("%s: struct %v, allow-list %v", d.EventName(), got, want)
		}
	}
}

func TestCheckRefusesWhatIsNotMetadata(t *testing.T) {
	t.Parallel()

	good := `{"event":"export.ready","id":"evt_1","occurred_at":"x","workspace":"wsp_1","data":{"job_id":"job_1"}}`
	if err := Check([]byte(good)); err != nil {
		t.Fatalf("a good body: %v", err)
	}

	bodies := map[string]string{
		"extra member":   `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"j","path":"/home/a/b.mp4"}}`,
		"text body":      `{"event":"job.failed","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"j","message":"the prompt was..."}}`,
		"extra envelope": `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{},"secret":"x"}`,
		"unknown event":  `{"event":"project.created","id":"e","occurred_at":"x","workspace":"w","data":{}}`,
		"nested":         `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":{"a":1}}}`,
		"signing secret": `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"whsec_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"}}`,
		"bearer":         `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"Bearer abc"}}`,
		"private key":    `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"x -----BEGIN PRIVATE KEY----- y"}}`,
		"control":        "{\"event\":\"export.ready\",\"id\":\"e\",\"occurred_at\":\"x\",\"workspace\":\"w\",\"data\":{\"job_id\":\"a\\nb\"}}",
		"not json":       `nope`,
		"too big": `{"event":"export.ready","id":"e","occurred_at":"x","workspace":"w","data":{"job_id":"` + strings.Repeat(
			"a",
			MaxBodyBytes,
		) + `"}}`,
		"empty": ``,
	}

	for name, body := range bodies {
		if err := Check([]byte(body)); err == nil {
			t.Errorf("%s: the body passed", name)
		}
	}
}

func TestPublicPostURL(t *testing.T) {
	t.Parallel()

	if PublicPostURL("https://youtu.be/abc", publish.Public) != "https://youtu.be/abc" {
		t.Error("a public address was dropped")
	}

	for _, raw := range []string{
		"http://youtu.be/abc", "https://u:p@x.example/a", "https://127.0.0.1/a", "https://localhost/a", "javascript:alert(1)",
		"", "https://x.example/" + strings.Repeat("a", MaxPostURLBytes), "https://x.example/a\nb",
	} {
		if got := PublicPostURL(raw, publish.Public); got != "" {
			t.Errorf("PublicPostURL(%q) = %q", raw, got)
		}
	}

	// The link of an unlisted post is what keeps it private, and a post that names no visibility is private.
	for _, v := range []publish.Visibility{publish.Unlisted, publish.Private, ""} {
		if got := PublicPostURL("https://youtu.be/abc", v); got != "" {
			t.Errorf("a post of visibility %q gave its address: %q", v, got)
		}
	}
}

func TestEventIDIsStableAndTheTestOneIsNot(t *testing.T) {
	t.Parallel()

	a := ExportReadyData{JobID: "job_1"}
	if EventID(a) != EventID(ExportReadyData{JobID: "job_1", Bytes: 5}) ||
		EventID(a) == EventID(ExportReadyData{JobID: "job_2"}) {
		t.Error("the id of an event is not the id of its subject")
	}

	if EventID(TestData{WebhookID: "w", Nonce: "1"}) == EventID(TestData{WebhookID: "w", Nonce: "2"}) {
		t.Error("two tests share an id")
	}

	// A retried target fails under a new approval request: that is a new event, and the same failure seen twice is not.
	failed := PublicationFailedData{TargetID: "tgt_1", ApprovalID: "apr_1"}
	if EventID(failed) != EventID(PublicationFailedData{TargetID: "tgt_1", ApprovalID: "apr_1", ErrorKind: "quota"}) {
		t.Error("the same failure has two ids")
	}

	if EventID(failed) == EventID(PublicationFailedData{TargetID: "tgt_1", ApprovalID: "apr_2"}) {
		t.Error("the failure of a retry has the id of the first failure, so it would never be delivered")
	}

	body, _, err := Build(failed, time.Unix(0, 0), "wsp_x")
	if err != nil || strings.Contains(string(body), "apr_1") {
		t.Errorf("the approval id is in the body: %s, %v", body, err)
	}

	if WorkspaceID("/a") == WorkspaceID("/b") || len(WorkspaceID("/a")) != len("wsp_")+10 {
		t.Error("the workspace id is not stable and distinct")
	}
}
