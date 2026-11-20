package destinations

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// T-86: a webhook that is not explicitly local reaches nothing inside, whatever its name says or later resolves to,
// and no redirect carries the post to another host.
func TestSecurity_T86_WebhookRefusesInternalAddresses(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	internal := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer internal.Close()

	p, _ := testPost(t)
	secret := hookSecretJSON(t, "")

	send := func(d *webhook, c connections.Credential) error {
		_, err := d.Send(context.Background(), &SendInput{Cred: c, Package: p, Delivery: "tgt_0123456789"})

		return err
	}

	// The connection was made with an address that looked fine and now leads inside (a name that changed after it
	// was checked is the same case: the dial itself is judged). The public client refuses it; no byte is sent.
	pub := connectionstest.New(t, KindWebhook, internal.URL, secret)

	err := pub.Manager.WithPublic(context.Background(), connectionstest.ID, func(c connections.Credential) error {
		return send(newWebhook(testConfig(), KindWebhook), c)
	})
	if !errors.Is(err, outbound.ErrNotPublic) {
		t.Errorf("err = %v, want the gate's refusal of a non-public address", err)
	}

	if hits.Load() != 0 {
		t.Fatal("a request reached an internal server through a public webhook")
	}

	// The same address as an explicit local webhook is delivered: the user chose it.
	loc := connectionstest.New(t, KindWebhookLocal, internal.URL, secret)

	loc.Do(t, func(_ context.Context, c connections.Credential) {
		if err := send(newWebhook(testConfig(), KindWebhookLocal), c); err != nil {
			t.Errorf("a local webhook was refused: %v", err)
		}
	})

	if hits.Load() != 1 {
		t.Errorf("the local webhook delivered %d times, want 1", hits.Load())
	}

	// A receiver that redirects to an internal address is not followed.
	hits.Store(0)

	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()

	red := connectionstest.New(t, KindWebhookLocal, redirecting.URL, secret)

	red.Do(t, func(_ context.Context, c connections.Credential) {
		err := send(newWebhook(testConfig(), KindWebhookLocal), c)

		if se, ok := social.AsError(err); !ok || se.Reason != "redirect" {
			t.Errorf("err = %v, want a rejection for the redirect", err)
		}
	})

	if hits.Load() != 0 {
		t.Error("a redirect was followed to another host")
	}

	// The refusals of the address rule at creation, for the names a person would try.
	d := newWebhook(testConfig(), KindWebhook)

	for _, host := range []string{"localhost", "127.0.0.1", "[::1]", "10.0.0.1", "169.254.169.254", "metadata.localhost", "0.0.0.0"} {
		if err := d.ValidateConfig(
			[]byte(secret),
			"https://"+host+"/hook",
		); err == nil ||
			!strings.Contains(err.Error(), "blog_webhook_local") {
			t.Errorf("%s: %v", host, err)
		}
	}
}
