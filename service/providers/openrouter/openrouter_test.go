package openrouter_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth/oauthtest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openrouter"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

const secret = "sk-or-v1-SECRET0123456789"

var auth = map[string]string{"Authorization": "Bearer " + secret}

func run(
	t *testing.T,
	fn func(ctx context.Context, c connections.Credential),
	xs ...providertest.Exchange,
) {
	t.Helper()

	rec := providertest.Serve(t, "testdata", xs...)
	connectionstest.New(t, openrouter.ID, rec.Endpoint("/api/v1"), secret).Do(t, fn)
}

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := openrouter.New().Descriptor()
	if d.ID != "openrouter" || !provider.ValidProviderID(d.ID) || d.DefaultEndpoint != "https://openrouter.ai/api/v1" {
		t.Fatalf("%+v", d)
	}

	if len(d.CredentialKinds) != 2 || d.CredentialKinds[0] != provider.CredentialAPIKey ||
		d.CredentialKinds[1] != provider.CredentialOAuth {
		t.Fatalf("a key or a sign-in: %+v", d.CredentialKinds)
	}

	var _ providers.TextGenerator = openrouter.New()
}

func TestModelsCarryLimitsAndThePriceOfTheCatalogue(t *testing.T) {
	t.Parallel()

	var got []providers.Model

	run(t, func(ctx context.Context, c connections.Credential) {
		var err error

		got, err = openrouter.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/api/v1/models", Header: auth, Response: "models.json"})

	if len(got) != 5 {
		t.Fatalf("got %d models", len(got))
	}

	byID := map[string]providers.Model{}
	for _, m := range got {
		byID[m.ID] = m
	}

	claude := byID["anthropic/claude-example"]
	if claude.ContextTokens != 200000 || claude.OutputTokens != 8192 || claude.Price == nil ||
		*claude.Price != (provider.Price{Currency: "USD", InputMicrosPerMTok: 3_000_000, OutputMicrosPerMTok: 15_000_000}) {
		t.Errorf("claude: %+v %+v", claude, claude.Price)
	}

	if p := byID["vendor/free-model:free"].Price; p == nil || p.InputMicrosPerMTok != 0 || p.OutputMicrosPerMTok != 0 {
		t.Errorf("a free model has a price of zero: %+v", p)
	}

	for _, id := range []string{"openrouter/auto", "vendor/odd-price"} {
		if byID[id].Price != nil {
			t.Errorf("%s has no usable price and must be refused, not priced: %+v", id, byID[id].Price)
		}
	}

	// A price per token with more digits than a micro is rounded up, never down.
	if p := byID["vendor/tiny"].Price; p == nil || p.InputMicrosPerMTok != 1100 || p.OutputMicrosPerMTok != 2_500_000 {
		t.Errorf("tiny: %+v", p)
	}

	if byID["vendor/tiny"].Name != "vendor/tiny" {
		t.Errorf("a model with no name is shown by its id, got %q", byID["vendor/tiny"].Name)
	}
}

func TestGenerateText(t *testing.T) {
	t.Parallel()

	temp := 0.7

	var res providers.TextResult

	run(t, func(ctx context.Context, c connections.Credential) {
		var err error

		res, err = openrouter.New().GenerateText(ctx, c, provider.TextRequest{
			Model: "anthropic/claude-example", System: "You write short captions.",
			Prompt: "Caption a video about sourdough.", MaxTokens: 200, Temperature: &temp,
		})
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/api/v1/chat/completions", Header: auth, WantBody: "chat_request.json",
		Response: "chat_ok.json",
	})

	if res.Text != "Sourdough, from starter to slice." || res.FinishReason != providers.FinishStop ||
		res.Usage.InputTokens != 24 || res.Usage.OutputTokens != 9 || res.Model != "anthropic/claude-example" {
		t.Fatalf("got %+v", res)
	}
}

func TestAKeyThatTheProviderEchoesNeverReachesAnError(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openrouter.New().Models(ctx, c)

		pe, ok := providers.AsError(err)
		if !ok || pe.Kind != providers.KindAuth || pe.Status != 401 || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("got %+v (%v)", pe, err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/api/v1/models", Status: 401, Response: "error_401.json"})
}

func TestValidateRedirect(t *testing.T) {
	t.Parallel()

	s := openrouter.NewSignIn(openrouter.SignInConfig{})

	for _, ok := range []string{
		"https://stuarty.example.com/auth/callback", "http://localhost:7878/auth/callback",
		"http://127.0.0.1:7878/auth/callback",
	} {
		if err := s.ValidateRedirect(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}

	for _, bad := range []string{
		"http://stuarty.example.com/auth/callback", "https://x.example/social/callback",
		"https://x.example/auth/callback/more", "https://x.example/auth/callback?next=//evil",
		"https://u:p@x.example/auth/callback", "https://x.example/auth/callback#f", "javascript:alert(1)",
		"/auth/callback", "", "http://[::1]x/auth/callback", "ftp://x.example/auth/callback",
	} {
		if err := s.ValidateRedirect(bad); err == nil {
			t.Errorf("%q was accepted as the redirect of a sign-in", bad)
		}
	}
}

func TestAuthorizeURLIsThePKCEAddressOfOpenRouter(t *testing.T) {
	t.Parallel()

	s := openrouter.NewSignIn(openrouter.SignInConfig{})

	raw, err := s.AuthorizeURL(oauth.BeginInput{
		State: "st", Challenge: "ch", RedirectURI: "https://stuarty.example.com/auth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}

	u, _ := url.Parse(raw)
	q := u.Query()

	if u.Scheme+"://"+u.Host+u.Path != "https://openrouter.ai/auth" ||
		q.Get("callback_url") != "https://stuarty.example.com/auth/callback" ||
		q.Get("code_challenge") != "ch" ||
		q.Get("code_challenge_method") != "S256" ||
		q.Get("state") != "st" {
		t.Fatalf("%s", raw)
	}

	if d := s.Describe(); d.Provider != "openrouter" || len(d.Hosts) != 1 || d.Hosts[0] != "openrouter.ai" {
		t.Fatalf("the connection of a sign-in reaches one host: %+v", d)
	}
}

// signedIn plays a whole sign-in against the fake: the consent, the exchange through the gated client of
// a connection, and the key kept in its vault.
func signedIn(t *testing.T, fake *oauthtest.Server) (*connectionstest.Harness, string) {
	t.Helper()

	s := openrouter.NewSignIn(openrouter.SignInConfig{
		AuthURL: fake.URL() + "/auth", KeysURL: fake.URL() + "/api/v1/auth/keys", Endpoint: fake.URL() + "/api/v1",
	})

	flows := oauth.NewFlows(nil)
	redirect := "http://127.0.0.1:9/auth/callback"
	state, challenge := flows.Begin(oauth.Pending{ConnectionID: connectionstest.ID, Actor: "local:ui", Browser: true})

	authURL, err := s.AuthorizeURL(oauth.BeginInput{State: state, Challenge: challenge, RedirectURI: redirect})
	if err != nil {
		t.Fatal(err)
	}

	back, _ := url.Parse(fake.Consent(t, authURL))
	if back.Query().Get("state") != state {
		t.Fatal("OpenRouter echoes the state")
	}

	p, terr := flows.Take(back.Query().Get("state"), "local:ui", true)
	if terr != nil {
		t.Fatal(terr)
	}

	placeholder, _ := oauth.PendingEnvelope(openrouter.ID, "", "").Marshal()
	h := connectionstest.New(t, openrouter.ID, fake.URL()+"/api/v1", string(placeholder))
	h.Manager.SetRefresher(oauth.NewRefresher(oauth.NewSignIns(s), nil))

	var env oauth.Envelope

	err = h.Manager.WithStored(
		context.Background(),
		connectionstest.ID,
		time.Second,
		func(c connections.Credential) error {
			var cerr error

			env, cerr = s.Complete(context.Background(), c.Client, oauth.CompleteInput{
				Code: back.Query().Get("code"), Verifier: p.Verifier, RedirectURI: redirect,
			})

			return cerr
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := env.Marshal()
	if err := h.Manager.PutSecret(connectionstest.ID, raw); err != nil {
		t.Fatal(err)
	}

	return h, env.AccessToken
}

func TestASignInMakesAKeyTheAdapterUsesLikeAPastedOne(t *testing.T) {
	t.Parallel()

	fake := oauthtest.New(t)
	h, key := signedIn(t, fake)

	if !strings.HasPrefix(key, "sk-or-v1-") {
		t.Fatalf("key %q", key)
	}

	h.Do(t, func(ctx context.Context, c connections.Credential) {
		if !c.SignedIn || string(c.Secret) != key {
			t.Fatalf("the adapter gets the key alone: %q signed in %v", c.Secret, c.SignedIn)
		}

		models, err := openrouter.New().Models(ctx, c)
		if err != nil || len(models) != 1 || models[0].Price == nil || models[0].Price.InputMicrosPerMTok != 1_000_000 {
			t.Fatalf("models %+v, %v", models, err)
		}

		res, err := openrouter.New().
			GenerateText(ctx, c, provider.TextRequest{Model: "fake/model-a", Prompt: "hi", MaxTokens: 10})
		if err != nil || res.Text != "Hello from the router." {
			t.Fatalf("text %+v, %v", res, err)
		}
	})
}

func TestACodeWithTheWrongVerifierOrReplayedMakesNoKey(t *testing.T) {
	t.Parallel()

	fake := oauthtest.New(t)
	s := openrouter.NewSignIn(openrouter.SignInConfig{
		AuthURL: fake.URL() + "/auth", KeysURL: fake.URL() + "/api/v1/auth/keys", Endpoint: fake.URL() + "/api/v1",
	})

	flows := oauth.NewFlows(nil)
	state, challenge := flows.Begin(oauth.Pending{Actor: "local:ui", Browser: true})
	authURL, _ := s.AuthorizeURL(
		oauth.BeginInput{State: state, Challenge: challenge, RedirectURI: "http://127.0.0.1:9/auth/callback"},
	)
	back, _ := url.Parse(fake.Consent(t, authURL))
	p, _ := flows.Take(state, "local:ui", true)

	h := connectionstest.New(t, openrouter.ID, fake.URL()+"/api/v1", "placeholder-secret")

	complete := func(verifier string) error {
		return h.Manager.With(context.Background(), connectionstest.ID, func(c connections.Credential) error {
			_, err := s.Complete(
				context.Background(),
				c.Client,
				oauth.CompleteInput{Code: back.Query().Get("code"), Verifier: verifier},
			)

			return err
		})
	}

	te, ok := errors.AsType[*oauth.TokenError](complete("not-the-verifier"))
	if !ok || te.Status != 403 {
		t.Fatalf("a wrong verifier = %v, want a refusal with status 403", te)
	}

	if err := complete(p.Verifier); err != nil {
		t.Fatalf("the refused attempt must not spend the code of the person: %v", err)
	}

	if err := complete(p.Verifier); err == nil {
		t.Fatal("a code made two keys")
	}
}
