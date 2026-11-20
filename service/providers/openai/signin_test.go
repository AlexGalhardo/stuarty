package openai_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth/oauthtest"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
)

const (
	redirect = "http://127.0.0.1:1455/auth/callback"
	hostID   = "urn:uuid:11111111-2222-4333-8444-555555555555"
)

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

type rig struct {
	fake  *oauthtest.Server
	clk   *clock
	in    *openai.SignIn
	flows *oauth.Flows
}

func newRig(t *testing.T) *rig {
	t.Helper()

	fake := oauthtest.New(t)
	fake.Strict = true
	clk := &clock{t: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}

	return &rig{
		fake: fake, clk: clk, flows: oauth.NewFlows(clk.now),
		in: openai.NewSignIn(openai.SignInConfig{
			AuthURL: fake.URL() + "/api/accounts/authorize", TokenURL: fake.URL() + "/api/accounts/oauth/token",
			Endpoint: fake.URL() + "/v1", HostID: hostID, Now: clk.now,
		}),
	}
}

// consent plays the person: the flow begins, the consent screen of the fake sends the browser back, and the
// state is taken. It returns the redirect the provider sent and the verifier of the flow.
func (r *rig) consent(t *testing.T, existing oauth.Envelope) (back url.Values, verifier, authURL string) {
	t.Helper()

	state, challenge := r.flows.Begin(oauth.Pending{ConnectionID: "con_1", Actor: "local:ui", Browser: true})

	authURL, err := r.in.AuthorizeURL(
		oauth.BeginInput{State: state, Challenge: challenge, RedirectURI: redirect, Existing: existing},
	)
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(r.fake.Consent(t, authURL))
	if err != nil {
		t.Fatal(err)
	}

	if u.Query().Get("state") != state {
		t.Fatal("the state did not come back")
	}

	p, terr := r.flows.Take(state, "local:ui", true)
	if terr != nil {
		t.Fatal(terr)
	}

	return u.Query(), p.Verifier, authURL
}

// complete runs the exchange through the gated client of a connection to the fake.
func (r *rig) complete(t *testing.T, in oauth.CompleteInput) (oauth.Envelope, error) {
	t.Helper()

	h := connectionstest.New(t, openai.ID, r.fake.URL()+"/v1", "placeholder-secret")

	var env oauth.Envelope

	err := h.Manager.With(context.Background(), connectionstest.ID, func(c connections.Credential) error {
		var cerr error

		in.Now = r.clk.now()
		env, cerr = r.in.Complete(context.Background(), c.Client, in)

		return cerr
	})

	return env, err
}

func TestAuthorizeURLCarriesWhatTheProviderDocuments(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	_, _, raw := r.consent(t, oauth.Envelope{})

	u, _ := url.Parse(raw)
	q := u.Query()

	if q.Get("client_id") != "dynamic_agent_client" || q.Get("ext_agent_host_id") != hostID ||
		q.Get("agent_name_hint") != "Stuarty" || q.Get("resource") != "https://api.openai.com/v1" ||
		q.Get("response_type") != "code" || q.Get("redirect_uri") != redirect || q.Get("nonce") == "" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("state") == "" {
		t.Fatalf("a first sign-in: %s", raw)
	}

	for _, scope := range []string{"openid", "profile", "email", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"} {
		if !strings.Contains(" "+q.Get("scope")+" ", " "+scope+" ") {
			t.Errorf("scope %q missing from %q", scope, q.Get("scope"))
		}
	}

	// A sign-in again uses the client that was issued, never the registration one.
	_, _, again := r.consent(t, oauth.Envelope{ClientID: "oaiapp_kept"})
	if u2, _ := url.Parse(again); u2.Query().Get("client_id") != "oaiapp_kept" {
		t.Errorf("a sign-in again: %s", again)
	}
}

func TestDescribeReachesTheAPIAndTheTokenHost(t *testing.T) {
	t.Parallel()

	d := openai.NewSignIn(openai.SignInConfig{HostID: hostID}).Describe()
	if d.Provider != "openai" || d.Endpoint != "https://api.openai.com/v1" ||
		strings.Join(d.Hosts, ",") != "api.openai.com,auth.openai.com" {
		t.Fatalf("%+v", d)
	}
}

func TestValidateRedirectIsExactlyTheLoopbackAddress(t *testing.T) {
	t.Parallel()

	s := openai.NewSignIn(openai.SignInConfig{HostID: hostID})

	for _, ok := range []string{"http://127.0.0.1:1455/auth/callback", "http://127.0.0.1:7878/auth/callback"} {
		if err := s.ValidateRedirect(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}

	for _, bad := range []string{
		"http://localhost:7878/auth/callback", "https://127.0.0.1:7878/auth/callback", "http://127.0.0.1/auth/callback",
		"http://127.0.0.1:7878/social/callback", "http://127.0.0.1:7878/auth/callback?x=1",
		"http://127.0.0.1:7878/auth/callback#x", "http://u@127.0.0.1:7878/auth/callback",
		"http://0.0.0.0:7878/auth/callback", "http://[::1]:7878/auth/callback", "https://stuarty.example.com/auth/callback",
		"http://127.0.0.1:0/auth/callback", "http://127.0.0.1:70000/auth/callback", "", "/auth/callback",
		"http://127.0.0.1:7878/auth/callback/",
	} {
		if err := s.ValidateRedirect(bad); err == nil {
			t.Errorf("%q was accepted as the redirect of a sign-in with ChatGPT", bad)
		}
	}
}

func TestFirstSignInRegistersAndKeepsTheIssuedClient(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	back, verifier, _ := r.consent(t, oauth.Envelope{})

	issued := back.Get("client_id")
	if issued != r.fake.IssuedClient() {
		t.Fatalf("the redirect carries the issued client, got %q", issued)
	}

	env, err := r.complete(
		t,
		oauth.CompleteInput{Code: back.Get("code"), Verifier: verifier, RedirectURI: redirect, ClientID: issued},
	)
	if err != nil {
		t.Fatal(err)
	}

	if env.Provider != "openai" || env.ClientID != issued || env.ClientID == "dynamic_agent_client" ||
		env.HostID != hostID ||
		env.AccessToken == "" ||
		env.RefreshToken == "" ||
		env.Account != "person@example.test" ||
		env.ExpiresAtMs != r.clk.now().Add(time.Hour).UnixMilli() ||
		!strings.Contains(env.Scope, "chatgpt.tokens.use.direct") {
		t.Fatalf("%+v", env)
	}
}

func TestSignInAgainKeepsTheClientOfTheAccount(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	kept := oauth.Envelope{ClientID: r.fake.IssuedClient(), HostID: hostID}
	back, verifier, _ := r.consent(t, kept)

	// The provider may leave the client out of a redirect of a returning sign-in.
	if back.Get("client_id") != "" {
		t.Fatalf("the fake omits the client on a returning sign-in: %v", back)
	}

	env, err := r.complete(
		t,
		oauth.CompleteInput{Code: back.Get("code"), Verifier: verifier, RedirectURI: redirect, Existing: kept},
	)
	if err != nil || env.ClientID != kept.ClientID {
		t.Fatalf("%+v, %v", env, err)
	}
}

func TestSignInRefusals(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		existing oauth.Envelope
		client   func(issued string) string
		verifier string
		reason   string
	}{
		"no client came back": {
			client: func(string) string { return "" },
			reason: oauth.ReasonRegistration,
		},
		"the registration client came back": {
			client: func(string) string { return "dynamic_agent_client" },
			reason: oauth.ReasonRegistration,
		},
		"a client that is not an identifier": {
			client: func(string) string { return "a b;c" },
			reason: oauth.ReasonRegistration,
		},
		"another client than the one kept": {
			existing: oauth.Envelope{ClientID: "oaiapp_kept"},
			client:   func(string) string { return "oaiapp_other" },
			reason:   oauth.ReasonClientMismatch,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t)
			back, verifier, _ := r.consent(t, tc.existing)

			before := r.fake.Exchanges()

			_, err := r.complete(t, oauth.CompleteInput{
				Code: back.Get("code"), Verifier: verifier, RedirectURI: redirect,
				ClientID: tc.client(back.Get("client_id")), Existing: tc.existing,
			})

			re, ok := errors.AsType[*oauth.RefusedError](err)
			if !ok || re.Reason != tc.reason {
				t.Fatalf("got %v, want a refusal with reason %s", err, tc.reason)
			}

			if r.fake.Exchanges() != before {
				t.Error("a sign-in that is refused before the exchange must not spend the code")
			}
		})
	}
}

func TestSignInWithTheWrongVerifierMakesNoCredential(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	back, _, _ := r.consent(t, oauth.Envelope{})

	_, err := r.complete(t, oauth.CompleteInput{
		Code: back.Get("code"), Verifier: "not-the-verifier", RedirectURI: redirect, ClientID: back.Get("client_id"),
	})

	if te, ok := errors.AsType[*oauth.TokenError](err); !ok || te.Code != "invalid_grant" {
		t.Fatalf("got %v, want invalid_grant", err)
	}
}

func TestSignInWithoutThePlanScopeIsRefused(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.fake.SetScope("openid profile email")
	back, verifier, _ := r.consent(t, oauth.Envelope{})

	_, err := r.complete(
		t,
		oauth.CompleteInput{
			Code:        back.Get("code"),
			Verifier:    verifier,
			RedirectURI: redirect,
			ClientID:    back.Get("client_id"),
		},
	)

	if re, ok := errors.AsType[*oauth.RefusedError](err); !ok || re.Reason != oauth.ReasonNoAccess {
		t.Fatalf("got %v, want a refusal for the missing plan scope", err)
	}
}

// usable stores the credential of a finished sign-in in a connection of a manager that renews it.
func (r *rig) usable(t *testing.T) *connectionstest.Harness {
	t.Helper()

	back, verifier, _ := r.consent(t, oauth.Envelope{})

	env, err := r.complete(
		t,
		oauth.CompleteInput{
			Code:        back.Get("code"),
			Verifier:    verifier,
			RedirectURI: redirect,
			ClientID:    back.Get("client_id"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	h := connectionstest.New(t, openai.ID, r.fake.URL()+"/v1", string(raw))
	h.Manager.SetRefresher(oauth.NewRefresher(oauth.NewSignIns(r.in), r.clk.now))

	return h
}

func TestASignInIsUsedByTheAdapterAndRenewedWhenItExpires(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	h := r.usable(t)

	use := func() string {
		var token string

		h.Do(t, func(ctx context.Context, c connections.Credential) {
			if !c.SignedIn {
				t.Fatal("the manager says it is a sign-in")
			}

			token = string(c.Secret)

			models, err := openai.New().Models(ctx, c)
			if err != nil || len(models) != 1 || models[0].ID != "gpt-fake-1" {
				t.Fatalf("models %+v, %v", models, err)
			}

			res, err := openai.New().
				GenerateText(ctx, c, provider.TextRequest{Model: "gpt-fake-1", Prompt: "hi", MaxTokens: 10})
			if err != nil || res.Text != "Hello from the plan." || res.Usage.InputTokens != 12 ||
				res.Usage.OutputTokens != 5 {
				t.Fatalf("text %+v, %v", res, err)
			}
		})

		return token
	}

	first := use()

	if second := use(); second != first || r.fake.Refreshes() != 0 {
		t.Fatalf("a token that has not expired is used as it is (%d refreshes)", r.fake.Refreshes())
	}

	// An hour later the token is renewed without anyone noticing: the same call works with a new one.
	r.clk.advance(time.Hour)

	if renewed := use(); renewed == first || r.fake.Refreshes() != 1 {
		t.Fatalf("the token was not renewed (%d refreshes)", r.fake.Refreshes())
	}

	// The grant is revoked on the account: the next renewal says the sign-in must be done again.
	r.clk.advance(2 * time.Hour)
	r.fake.FailRefresh(400, "invalid_grant")

	err := h.Manager.With(context.Background(), connectionstest.ID, func(connections.Credential) error { return nil })
	if !errors.Is(err, connections.ErrNeedsSignIn) {
		t.Fatalf("a revoked grant = %v, want ErrNeedsSignIn", err)
	}
}
