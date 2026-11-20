package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth/oauthtest"
)

const redirect = "http://127.0.0.1:9/auth/callback"

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

func newClock() *clock { return &clock{t: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)} }

// begin plays the sign-in up to the redirect: the flow, the consent screen of the fake and the state and
// the code that come back.
func begin(t *testing.T, f *oauth.Flows, fake *oauthtest.Server) (code, verifier string) {
	t.Helper()

	s, challenge := f.Begin(oauth.Pending{ConnectionID: "con_1", Actor: "local:ui", Browser: true})
	auth := oauth.AuthorizeURL(fake.URL()+"/auth", "client-1", redirect, s, challenge, []string{"openid"}, nil)

	back, err := url.Parse(fake.Consent(t, auth))
	if err != nil {
		t.Fatal(err)
	}

	if back.Query().Get("state") != s {
		t.Fatalf("the state came back as %q, want %q", back.Query().Get("state"), s)
	}

	p, terr := f.Take(s, "local:ui", true)
	if terr != nil {
		t.Fatal(terr)
	}

	return back.Query().Get("code"), p.Verifier
}

func TestFlowHappyPath(t *testing.T) {
	t.Parallel()

	fake := oauthtest.New(t)
	flows := oauth.NewFlows(nil)
	code, verifier := begin(t, flows, fake)

	tok, err := oauth.Exchange(
		context.Background(),
		fake.Server.Client(),
		fake.URL()+"/api/accounts/oauth/token",
		"client-1",
		"",
		code,
		verifier,
		redirect,
	)
	if err != nil {
		t.Fatal(err)
	}

	if tok.AccessToken == "" || tok.RefreshToken == "" || tok.ExpiresIn != time.Hour || tok.IDToken == "" {
		t.Fatalf("token = %+v", tok)
	}
}

func TestFlowWrongVerifierIsRefused(t *testing.T) {
	t.Parallel()

	fake := oauthtest.New(t)
	code, _ := begin(t, oauth.NewFlows(nil), fake)

	_, err := oauth.Exchange(
		context.Background(),
		fake.Server.Client(),
		fake.URL()+"/api/accounts/oauth/token",
		"client-1",
		"",
		code,
		"not-the-verifier",
		redirect,
	)

	te, ok := errors.AsType[*oauth.TokenError](err)
	if !ok || te.Code != "invalid_grant" {
		t.Fatalf("a wrong verifier = %v, want invalid_grant", err)
	}
}

func TestFlowCodeIsSingleUse(t *testing.T) {
	t.Parallel()

	fake := oauthtest.New(t)
	code, verifier := begin(t, oauth.NewFlows(nil), fake)
	tokenURL := fake.URL() + "/api/accounts/oauth/token"

	if _, err := oauth.Exchange(
		context.Background(),
		fake.Server.Client(),
		tokenURL,
		"client-1",
		"",
		code,
		verifier,
		redirect,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := oauth.Exchange(
		context.Background(),
		fake.Server.Client(),
		tokenURL,
		"client-1",
		"",
		code,
		verifier,
		redirect,
	); err == nil {
		t.Fatal("a code was exchanged twice")
	}
}

func TestFlowReplayedAndExpiredStateAreRefused(t *testing.T) {
	t.Parallel()

	clk := newClock()
	flows := oauth.NewFlows(clk.now)

	state, _ := flows.Begin(oauth.Pending{Actor: "local:ui", Browser: true})
	if _, err := flows.Take(state, "local:ui", true); err != nil {
		t.Fatal(err)
	}

	if _, err := flows.Take(state, "local:ui", true); !errors.Is(err, oauth.ErrUnknownState) {
		t.Errorf("a replayed state = %v, want ErrUnknownState", err)
	}

	late, _ := flows.Begin(oauth.Pending{Actor: "local:ui", Browser: true})

	clk.advance(oauth.TTL + time.Second)

	if _, err := flows.Take(late, "local:ui", true); !errors.Is(err, oauth.ErrExpired) {
		t.Errorf("an expired state = %v, want ErrExpired", err)
	}
}

func TestEnvelopeRoundTripAndForeignSecrets(t *testing.T) {
	t.Parallel()

	in := oauth.Envelope{Provider: "openai", ClientID: "oaiapp_x", AccessToken: "a", RefreshToken: "r", ExpiresAtMs: 5}

	raw, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	got, ok := oauth.ParseEnvelope(raw)
	if !ok || got.AccessToken != "a" || got.RefreshToken != "r" || got.ClientID != "oaiapp_x" || got.ExpiresAtMs != 5 {
		t.Fatalf("round trip = %+v, %v", got, ok)
	}

	for _, secret := range []string{"", "sk-test-0123456789", `{"format":"other","version":1,"provider":"x"}`, `{"a":1}`, "{"} {
		if _, isEnv := oauth.ParseEnvelope([]byte(secret)); isEnv {
			t.Errorf("%q was taken for a sign-in credential", secret)
		}
	}

	if oauth.PendingEnvelope("openai", "", "").Ready() {
		t.Error("a sign-in that is not finished is ready")
	}
}

// planSignIn is the part of a provider that the refresh needs: the token endpoint of the fake.
type planSignIn struct{ tokenURL string }

func (planSignIn) Describe() oauth.Description   { return oauth.Description{Provider: "openai"} }
func (planSignIn) ValidateRedirect(string) error { return nil }
func (planSignIn) AuthorizeURL(oauth.BeginInput) (string, error) {
	return "", errors.New("not used")
}

func (planSignIn) Complete(context.Context, oauth.Doer, oauth.CompleteInput) (oauth.Envelope, error) {
	return oauth.Envelope{}, errors.New("not used")
}

func (p planSignIn) Refresh(ctx context.Context, d oauth.Doer, e oauth.Envelope) (oauth.Token, error) {
	return oauth.Request(ctx, d, p.tokenURL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {e.RefreshToken}, "client_id": {e.ClientID},
	})
}

type rig struct {
	fake *oauthtest.Server
	clk  *clock
	h    *connectionstest.Harness
}

// newRig signs in against the fake and keeps the credential in a connection of a manager that renews it.
func newRig(t *testing.T) *rig {
	t.Helper()

	fake := oauthtest.New(t)
	clk := newClock()

	code, verifier := begin(t, oauth.NewFlows(nil), fake)
	tokenURL := fake.URL() + "/api/accounts/oauth/token"

	tok, err := oauth.Exchange(
		context.Background(),
		fake.Server.Client(),
		tokenURL,
		"client-1",
		"",
		code,
		verifier,
		redirect,
	)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := oauth.Envelope{
		Provider: "openai", ClientID: "client-1", AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		ExpiresAtMs: clk.now().Add(tok.ExpiresIn).UnixMilli(),
	}.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	h := connectionstest.New(t, "openai", fake.URL()+"/v1", string(raw))
	h.Manager.SetRefresher(oauth.NewRefresher(oauth.NewSignIns(planSignIn{tokenURL: tokenURL}), clk.now))

	return &rig{fake: fake, clk: clk, h: h}
}

// use runs a closure through the manager and returns the secret it was given.
func (r *rig) use(t *testing.T) (secret string, signedIn bool, err error) {
	t.Helper()

	err = r.h.Manager.With(context.Background(), connectionstest.ID, func(c connections.Credential) error {
		secret, signedIn = string(c.Secret), c.SignedIn

		return nil
	})

	return secret, signedIn, err
}

func (r *rig) stored(t *testing.T) oauth.Envelope {
	t.Helper()

	var env oauth.Envelope

	err := r.h.Manager.WithStored(
		context.Background(),
		connectionstest.ID,
		time.Second,
		func(c connections.Credential) error {
			var ok bool

			env, ok = oauth.ParseEnvelope(c.Secret)
			if !ok {
				t.Fatal("the stored secret is not a sign-in credential")
			}

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return env
}

func TestRefreshOfAFreshTokenMakesNoRequest(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	want := r.stored(t).AccessToken

	got, signedIn, err := r.use(t)
	if err != nil || got != want || !signedIn {
		t.Fatalf("use = %q, %v, %v; the closure gets the access token alone", got, signedIn, err)
	}

	if n := r.fake.Refreshes(); n != 0 {
		t.Errorf("%d refresh requests for a token that has not expired", n)
	}
}

func TestRefreshRenewsAnExpiredTokenAndKeepsTheNewOnes(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	old := r.stored(t)

	r.clk.advance(time.Hour)

	got, signedIn, err := r.use(t)
	if err != nil || !signedIn || got == "" || got == old.AccessToken {
		t.Fatalf("use after expiry = %q, %v, %v", got, signedIn, err)
	}

	now := r.stored(t)
	if now.AccessToken != got || now.RefreshToken == old.RefreshToken || now.RefreshToken == "" || now.NeedsSignIn {
		t.Fatalf("the renewed tokens were not stored together: %+v", now)
	}

	// The next use finds a fresh token and asks for nothing.
	if _, _, err := r.use(t); err != nil || r.fake.Refreshes() != 1 {
		t.Errorf("a second use = %v after %d refreshes, want 1", err, r.fake.Refreshes())
	}
}

func TestRefreshConcurrentUsesMakeOneRequest(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.fake.DelayRefresh(40 * time.Millisecond)
	r.clk.advance(time.Hour)

	const callers = 8

	var (
		wg      sync.WaitGroup
		secrets [callers]string
		errs    [callers]error
	)

	for i := range callers {
		wg.Go(func() { secrets[i], _, errs[i] = r.use(t) })
	}

	wg.Wait()

	for i := range callers {
		if errs[i] != nil || secrets[i] == "" || secrets[i] != secrets[0] {
			t.Fatalf("caller %d: %q, %v (caller 0 has %q)", i, secrets[i], errs[i], secrets[0])
		}
	}

	if n := r.fake.Refreshes(); n != 1 {
		t.Fatalf("%d refresh requests for %d concurrent uses: a rotating refresh token was spent twice", n, callers)
	}
}

func TestRefreshRefusedGrantMarksTheSignInAndKeepsTheTokens(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	before := r.stored(t)

	r.fake.FailRefresh(http.StatusBadRequest, "invalid_grant")
	r.clk.advance(time.Hour)

	if _, _, err := r.use(t); !errors.Is(err, connections.ErrNeedsSignIn) {
		t.Fatalf("a refused grant = %v, want ErrNeedsSignIn", err)
	}

	after := r.stored(t)
	if !after.NeedsSignIn || after.RefreshToken != before.RefreshToken || after.ClientID != before.ClientID {
		t.Fatalf("the entry after a refused grant: %+v", after)
	}

	// A marked sign-in is not tried again: it waits for a person.
	if _, _, err := r.use(t); !errors.Is(err, connections.ErrNeedsSignIn) || r.fake.Refreshes() != 1 {
		t.Errorf("second use = %v after %d refreshes, want ErrNeedsSignIn after 1", err, r.fake.Refreshes())
	}
}

func TestRefreshFailureThatSaysNothingAboutTheGrantKeepsTheSignIn(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.clk.advance(time.Hour)

	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		r.fake.FailRefresh(status, "temporarily_unavailable")

		if _, _, err := r.use(t); !errors.Is(err, connections.ErrSignInUnavailable) {
			t.Fatalf("status %d = %v, want ErrSignInUnavailable", status, err)
		}

		if r.stored(t).NeedsSignIn {
			t.Fatalf("status %d marked the sign-in as needing a new one", status)
		}
	}

	r.fake.FailRefresh(0, "")

	if got, _, err := r.use(t); err != nil || got == "" {
		t.Fatalf("after the provider recovered: %q, %v", got, err)
	}
}

func TestRefreshLeavesAnAPIKeyAlone(t *testing.T) {
	t.Parallel()

	h := connectionstest.New(t, "openai", "http://127.0.0.1:9/v1", "sk-test-0123456789")
	h.Manager.SetRefresher(oauth.NewRefresher(oauth.NewSignIns(), nil))

	h.Do(t, func(_ context.Context, c connections.Credential) {
		if string(c.Secret) != "sk-test-0123456789" || c.SignedIn {
			t.Errorf("an API key was changed: %q signed in %v", c.Secret, c.SignedIn)
		}
	})
}
