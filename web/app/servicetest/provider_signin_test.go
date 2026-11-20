package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth/oauthtest"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/openrouter"
)

// Epic 12 (SDD-016 section 12, SDD-014 section 13): the sign-in to an AI provider, over the real service, the
// connection manager and the vault, with a fake authorization server on loopback.

const signInRedirect = "http://127.0.0.1:7878/auth/callback"

type signInEnv struct {
	*textEnv

	fake *oauthtest.Server
}

func newSignInEnv(t *testing.T) *signInEnv {
	t.Helper()

	return newSignInEnvIn(t, false)
}

// newSignInEnvIn is newSignInEnv for a server in network mode (network true), where the build refuses Sign
// in with ChatGPT (SDD-016 section 12.3).
func newSignInEnvIn(t *testing.T, network bool) *signInEnv {
	t.Helper()

	fake := oauthtest.New(t)
	fake.Strict = true

	te := newTextEnv(t, func(o *service.Options) {
		if err := o.Providers.Register(openrouter.New()); err != nil {
			t.Fatal(err)
		}

		o.SignIns = oauth.NewSignIns(
			openai.NewSignIn(openai.SignInConfig{
				AuthURL: fake.URL() + "/api/accounts/authorize", TokenURL: fake.URL() + "/api/accounts/oauth/token",
				Endpoint: fake.URL() + "/v1", HostID: "urn:uuid:11111111-2222-4333-8444-555555555555",
				Network: network,
			}),
			openrouter.NewSignIn(openrouter.SignInConfig{
				AuthURL:  fake.URL() + "/auth",
				KeysURL:  fake.URL() + "/api/v1/auth/keys",
				Endpoint: fake.URL() + "/api/v1",
			}),
		)
	})

	return &signInEnv{textEnv: te, fake: fake}
}

// begin starts a sign-in as a person and returns the address of the consent screen.
func (se *signInEnv) begin(
	ctx context.Context,
	provider string,
	connectionID string,
) (*service.ProviderSignInBeginResult, error) {
	return se.svc.ProviderSignInBegin(ctx, service.ProviderSignInBeginRequest{
		Provider: provider, ConnectionID: connectionID, RedirectURI: signInRedirect,
	})
}

// redirectOf plays the person on the consent screen of the fake: what the provider sends the browser back with.
func (se *signInEnv) redirectOf(authorize string) url.Values {
	se.t.Helper()

	u, err := url.Parse(se.fake.Consent(se.t, authorize))
	if err != nil {
		se.t.Fatal(err)
	}

	return u.Query()
}

func (se *signInEnv) complete(ctx context.Context, q url.Values) (*service.ProviderSignIn, error) {
	return se.svc.ProviderSignInComplete(ctx, service.ProviderSignInCompleteRequest{
		State: q.Get("state"), Code: q.Get("code"), ClientID: q.Get("client_id"),
	})
}

// signIn is a whole sign-in by a person, from the button to the redirect.
func (se *signInEnv) signIn(provider, connectionID string) *service.ProviderSignIn {
	se.t.Helper()

	ctx := se.asPerson()

	begun, err := se.begin(ctx, provider, connectionID)
	if err != nil {
		se.t.Fatal(err)
	}

	done, err := se.complete(ctx, se.redirectOf(begun.AuthorizeURL))
	if err != nil {
		se.t.Fatal(err)
	}

	if done.ConnectionID != begun.ConnectionID {
		se.t.Fatalf("the sign-in ended on %s, it began on %s", done.ConnectionID, begun.ConnectionID)
	}

	return done
}

func (se *signInEnv) connectionCount() int {
	se.t.Helper()

	list, err := se.svc.ConnectionList(se.as(token), service.ConnectionListRequest{})
	if err != nil {
		se.t.Fatal(err)
	}

	return len(list.Connections)
}

// leaks says which of the secrets appear in v, as every surface would show it.
func leaks(v any, secrets []string) []string {
	raw, _ := json.Marshal(v) //nolint:errchkjson // a test helper over results that always marshal

	var out []string

	for _, s := range secrets {
		if strings.Contains(string(raw), s) {
			out = append(out, s)
		}
	}

	return out
}

// Acceptance criterion 1 of epic 12: a sign-in begun from the Connections page ends with a connection whose
// tokens are only in the vault.
func TestSignInChatGPTEndsWithAConnectionWhoseTokensAreOnlyInTheVault(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	person := se.asPerson()

	begun, err := se.begin(person, "openai", "")
	if err != nil {
		t.Fatal(err)
	}

	u, _ := url.Parse(begun.AuthorizeURL)
	q := u.Query()

	if !strings.HasPrefix(begun.AuthorizeURL, se.fake.URL()+"/api/accounts/authorize?") ||
		q.Get("client_id") != "dynamic_agent_client" || q.Get("redirect_uri") != signInRedirect ||
		q.Get("code_challenge_method") != "S256" || q.Get("resource") != "https://api.openai.com/v1" {
		t.Fatalf("the consent address: %s", begun.AuthorizeURL)
	}

	if se.fake.Exchanges() != 0 {
		t.Fatal("beginning made a request to the provider")
	}

	// Until the person comes back, the connection exists and says it needs a sign-in.
	listed, err := se.svc.ProviderSignInList(person, service.ProviderSignInListRequest{})
	if err != nil || len(listed.SignIns) != 1 || listed.SignIns[0].State != service.SignInNeedsSignIn ||
		listed.SignIns[0].ConnectionID != begun.ConnectionID {
		t.Fatalf("before the redirect: %+v, %v", listed, err)
	}

	done, err := se.complete(person, se.redirectOf(begun.AuthorizeURL))
	if err != nil {
		t.Fatal(err)
	}

	if done.State != service.SignInReady || done.Provider != "openai" || done.Account != "person@example.test" ||
		done.Label != "ChatGPT" {
		t.Fatalf("done = %+v", done)
	}

	conn, err := se.svc.ConnectionGet(person, service.ConnectionRef{ID: done.ConnectionID})
	if err != nil || conn.Kind != "openai" || !conn.SecretSet || conn.Endpoint != se.fake.URL()+"/v1" ||
		len(conn.Accepts) != 1 || conn.Accepts[0] != "text" {
		t.Fatalf("connection = %+v, %v", conn, err)
	}

	// The tokens are in no result of any operation.
	secrets := append(se.fake.Tokens(), se.fake.IssuedClient())

	for name, v := range map[string]any{"begin": begun, "done": done, "connection": conn} {
		if found := leaks(v, secrets); len(found) != 0 {
			t.Errorf("%s shows %v", name, found)
		}
	}

	all, _ := se.svc.ConnectionList(person, service.ConnectionListRequest{})
	prov, _ := se.svc.ProviderList(person, service.ProviderListRequest{})
	signs, _ := se.svc.ProviderSignInList(person, service.ProviderSignInListRequest{})

	if found := leaks([]any{all, prov, signs}, secrets); len(found) != 0 {
		t.Errorf("a list shows %v", found)
	}

	// Nor in any file of the workspace: the vault is encrypted, the rest never held one.
	err = filepath.WalkDir(string(se.dir), func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}

		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file that cannot be read holds nothing for the scan
		}

		for _, s := range secrets {
			if strings.Contains(string(raw), s) {
				t.Errorf("%s holds a token in plaintext", path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignInOpenRouterEndsWithAKeyOnlyInTheVault(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openrouter", "")

	if done.State != service.SignInReady || done.Provider != "openrouter" || done.Label != "OpenRouter" {
		t.Fatalf("done = %+v", done)
	}

	person := se.asPerson()
	conn, err := se.svc.ConnectionGet(person, service.ConnectionRef{ID: done.ConnectionID})

	if err != nil || conn.Kind != "openrouter" || !conn.SecretSet {
		t.Fatalf("connection = %+v, %v", conn, err)
	}

	if found := leaks([]any{done, conn}, se.fake.Tokens()); len(found) != 0 {
		t.Errorf("a result shows %v", found)
	}

	// The key works: the catalogue is read with it and carries the prices the provider publishes.
	models, err := se.svc.ProviderModels(person, service.ProviderModelsRequest{ConnectionID: done.ConnectionID})
	if err != nil || len(models.Models) != 1 || models.Models[0].PriceSource != "catalogue" ||
		models.Models[0].Price == nil || models.Models[0].Price.InputMicrosPerMTok != 1_000_000 {
		t.Fatalf("models = %+v, %v", models, err)
	}
}

// Acceptance criterion 2: a replayed or foreign state is refused and creates nothing.
func TestSignInReplayedForeignAndExpiredStatesAreRefusedAndCreateNothing(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	person := se.asPerson()

	begun, err := se.begin(person, "openai", "")
	if err != nil {
		t.Fatal(err)
	}

	q := se.redirectOf(begun.AuthorizeURL)

	if _, err := se.complete(person, q); err != nil {
		t.Fatal(err)
	}

	connections, exchanges := se.connectionCount(), se.fake.Exchanges()

	for name, state := range map[string]string{
		"replayed": q.Get("state"), "foreign": "not-a-state-this-server-issued", "empty": "",
	} {
		_, err := se.svc.ProviderSignInComplete(person, service.ProviderSignInCompleteRequest{
			State: state, Code: q.Get("code"), ClientID: q.Get("client_id"),
		})
		if !isCode(err, "E_VALIDATION") {
			t.Errorf("%s state = %v, want E_VALIDATION", name, err)
		}
	}

	if se.connectionCount() != connections || se.fake.Exchanges() != exchanges {
		t.Errorf("a refused state made %d connections and %d exchanges",
			se.connectionCount()-connections, se.fake.Exchanges()-exchanges)
	}

	// A state of another session is spent by the attempt, so the person cannot use it either afterwards.
	again, err := se.begin(person, "openrouter", "")
	if err != nil {
		t.Fatal(err)
	}

	other := se.browser("local:another-session")
	q2 := se.redirectOf(again.AuthorizeURL)

	if _, err := se.complete(other, q2); !isCode(err, "E_VALIDATION") {
		t.Fatalf("a state begun by another session = %v, want E_VALIDATION", err)
	}

	if _, err := se.complete(person, q2); !isCode(err, "E_VALIDATION") {
		t.Fatalf("a state that was guessed once is spent: %v", err)
	}

	if se.fake.Exchanges() != exchanges {
		t.Error("a refused state reached the token endpoint")
	}

	// A state that waited longer than ten minutes is refused.
	late, err := se.begin(person, "openrouter", "")
	if err != nil {
		t.Fatal(err)
	}

	q3 := se.redirectOf(late.AuthorizeURL)

	se.clock.advance(oauth.TTL + time.Second)

	if _, err := se.complete(person, q3); !isCode(err, "E_VALIDATION") {
		t.Fatalf("an expired state = %v, want E_VALIDATION", err)
	}

	// A provider that sent no code (the person said no) ends the flow with nothing kept.
	denied, err := se.begin(person, "openrouter", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := se.svc.ProviderSignInComplete(person, service.ProviderSignInCompleteRequest{
		State: se.redirectOf(denied.AuthorizeURL).Get("state"), Error: "access_denied",
	}); !isCode(err, "E_VALIDATION") {
		t.Fatalf("a refusal of the person = %v, want E_VALIDATION", err)
	}

	list, _ := se.svc.ProviderSignInList(person, service.ProviderSignInListRequest{})
	for _, s := range list.SignIns {
		if s.ConnectionID != begun.ConnectionID && s.State == service.SignInReady {
			t.Errorf("a refused sign-in made a working connection: %+v", s)
		}
	}
}

// Acceptance criterion 3: TextGenerate works through a connection made by a sign-in, with the models the
// account grants.
func TestSignInTextGenerateWorksThroughAChatGPTConnection(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")
	person := se.asPerson()

	models, err := se.svc.ProviderModels(person, service.ProviderModelsRequest{ConnectionID: done.ConnectionID})
	if err != nil || len(models.Models) != 1 || models.Models[0].ID != "gpt-fake-1" ||
		models.Models[0].PriceSource != "catalogue" || models.Models[0].Price == nil ||
		models.Models[0].Price.InputMicrosPerMTok != 0 {
		t.Fatalf("the models of the account (the hidden one is not offered): %+v, %v", models, err)
	}

	job, err := se.svc.TextGenerate(se.as(token), service.TextGenerateRequest{
		ProjectID:    se.prj,
		ConnectionID: done.ConnectionID,
		Model:        "gpt-fake-1",
		Prompt:       "Caption a video.",
		MaxTokens:    50,
	})
	if err != nil {
		t.Fatal(err)
	}

	if job.State == service.JobWaitingApproval {
		if err := se.approve(job.ApprovalID); err != nil {
			t.Fatal(err)
		}
	}

	got := se.job(job.JobID)
	if got.State != service.JobSucceeded || got.Result == nil || got.Result.Text == nil ||
		got.Result.Text.Text != "Hello from the plan." || got.Result.Text.InputTokens != 12 ||
		got.Result.Text.OutputTokens != 5 || got.Result.Text.Cost.Micros != 0 {
		t.Fatalf("job = %+v", got)
	}

	// A model the account does not grant has no price: the request is refused, never estimated at zero.
	_, err = se.svc.TextGenerate(se.as(token), service.TextGenerateRequest{
		ProjectID: se.prj, ConnectionID: done.ConnectionID, Model: "gpt-not-granted", Prompt: "p", MaxTokens: 5,
	})
	if !isCode(err, service.CodePriceUnknown) {
		t.Errorf("a model the account does not grant = %v, want E_PRICE_UNKNOWN", err)
	}

	if found := leaks([]any{got, job}, se.fake.Tokens()); len(found) != 0 {
		t.Errorf("the job shows %v", found)
	}
}

func TestSignInTextGenerateWorksThroughAnOpenRouterConnection(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openrouter", "")
	person := se.asPerson()

	if _, err := se.svc.ProviderModels(
		person,
		service.ProviderModelsRequest{ConnectionID: done.ConnectionID},
	); err != nil {
		t.Fatal(err)
	}

	job, err := se.svc.TextGenerate(se.as(token), service.TextGenerateRequest{
		ProjectID:    se.prj,
		ConnectionID: done.ConnectionID,
		Model:        "fake/model-a",
		Prompt:       "Caption a video.",
		MaxTokens:    50,
	})
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.Estimate == nil || job.Estimate.Micros == 0 {
		t.Fatalf("a key that spends credits waits for a person and shows its estimate: %+v", job)
	}

	if err := se.approve(job.ApprovalID); err != nil {
		t.Fatal(err)
	}

	got := se.job(job.JobID)
	// 10 input tokens at 1 and 4 output tokens at 2 per million: 10 + 8 micros.
	if got.State != service.JobSucceeded || got.Result == nil || got.Result.Text == nil ||
		got.Result.Text.Text != "Hello from the router." || got.Result.Text.Cost.Micros != 18 {
		t.Fatalf("job = %+v", got)
	}
}

// Acceptance criterion 4: an expired token is refreshed without the user noticing; a failed refresh shows
// "needs sign-in" and deletes nothing.
func TestSignInExpiredTokenIsRefreshedAndAFailedRefreshShowsNeedsSignIn(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")
	person := se.asPerson()
	read := func() (*service.ProviderModelsResult, error) {
		return se.svc.ProviderModels(
			person,
			service.ProviderModelsRequest{ConnectionID: done.ConnectionID, Refresh: true},
		)
	}

	if _, err := read(); err != nil {
		t.Fatal(err)
	}

	// An hour and more later the access token has expired: the use renews it and nobody is told.
	se.clock.advance(2 * time.Hour)

	if _, err := read(); err != nil || se.fake.Refreshes() != 1 {
		t.Fatalf("after the expiry: %v with %d refreshes, want none and 1", err, se.fake.Refreshes())
	}

	// The grant is revoked on the account of the person: the next renewal fails for good.
	se.clock.advance(2 * time.Hour)
	se.fake.FailRefresh(400, "invalid_grant")

	_, err := read()

	se2, ok := errors.AsType[*service.Error](err)
	if !ok || se2.Code != "E_PROVIDER" || se2.Details["kind"] != "auth" || se2.Details["code"] != "needs_sign_in" ||
		!strings.Contains(se2.Hint, "signs in again") {
		t.Fatalf("a failed refresh = %v, want E_PROVIDER with the code needs_sign_in", err)
	}

	if found := leaks(se2, se.fake.Tokens()); len(found) != 0 {
		t.Errorf("the error shows %v", found)
	}

	listed, err := se.svc.ProviderSignInList(person, service.ProviderSignInListRequest{})
	if err != nil || len(listed.SignIns) != 1 || listed.SignIns[0].State != service.SignInNeedsSignIn ||
		listed.SignIns[0].Account != "person@example.test" {
		t.Fatalf("the card shows %+v, %v", listed, err)
	}

	// Nothing was deleted: the connection is there, and a text job through it says what to do.
	conn, err := se.svc.ConnectionGet(person, service.ConnectionRef{ID: done.ConnectionID})
	if err != nil || !conn.SecretSet {
		t.Fatalf("the connection after a failed refresh: %+v, %v", conn, err)
	}

	// A sign-in again of the same connection replaces the tokens and it works again.
	se.fake.FailRefresh(0, "")

	again := se.signIn("openai", done.ConnectionID)
	if again.ConnectionID != done.ConnectionID || again.State != service.SignInReady {
		t.Fatalf("sign in again = %+v", again)
	}

	if _, err := read(); err != nil {
		t.Fatalf("after signing in again: %v", err)
	}
}

// A failure that says nothing about the grant (the provider is down) is not a lost sign-in.
func TestSignInProviderDownDuringARefreshIsNotNeedsSignIn(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")
	person := se.asPerson()

	se.clock.advance(2 * time.Hour)
	se.fake.FailRefresh(503, "temporarily_unavailable")

	_, err := se.svc.ProviderModels(
		person,
		service.ProviderModelsRequest{ConnectionID: done.ConnectionID, Refresh: true},
	)

	se2, ok := errors.AsType[*service.Error](err)
	if !ok || se2.Code != "E_PROVIDER" || se2.Details["kind"] != "unavailable" {
		t.Fatalf("a provider that is down = %v, want E_PROVIDER unavailable", err)
	}

	listed, _ := se.svc.ProviderSignInList(person, service.ProviderSignInListRequest{})
	if len(listed.SignIns) != 1 || listed.SignIns[0].State != service.SignInReady {
		t.Fatalf("a provider that is down must not mark the sign-in: %+v", listed)
	}

	se.fake.FailRefresh(0, "")

	if _, err := se.svc.ProviderModels(
		person,
		service.ProviderModelsRequest{ConnectionID: done.ConnectionID, Refresh: true},
	); err != nil {
		t.Fatalf("when it is back: %v", err)
	}
}

// The refresh through the service is one request however many uses meet it (the race detector runs it).
func TestSignInConcurrentUsesAfterTheExpiryRefreshOnce(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")

	se.fake.DelayRefresh(40 * time.Millisecond)
	se.clock.advance(2 * time.Hour)

	var (
		wg   sync.WaitGroup
		errs [8]error
	)

	for i := range errs {
		wg.Go(func() {
			_, errs[i] = se.svc.ProviderModels(
				se.asPerson(), service.ProviderModelsRequest{ConnectionID: done.ConnectionID, Refresh: true},
			)
		})
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
	}

	if n := se.fake.Refreshes(); n != 1 {
		t.Fatalf("%d refreshes for 8 concurrent uses, want 1", n)
	}
}

func TestSignInOtherCases(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	person := se.asPerson()

	// Another provider, or none, is a validation error that lists what exists.
	_, err := se.begin(person, "anthropic", "")
	if !isCode(err, "E_VALIDATION") {
		t.Errorf("a provider with no sign-in = %v, want E_VALIDATION", err)
	}

	// A connection made with an API key is not a sign-in: signing in "again" would replace the key.
	_, err = se.begin(person, "openai", se.conn)
	if !isCode(err, "E_VALIDATION") {
		t.Errorf("sign in again of a key connection = %v, want E_VALIDATION", err)
	}

	// A sign-in of another provider's connection is refused.
	or := se.signIn("openrouter", "")

	if _, err := se.begin(person, "openai", or.ConnectionID); !isCode(err, "E_VALIDATION") {
		t.Errorf("sign in again with the wrong provider = %v, want E_VALIDATION", err)
	}

	// A connection that does not exist.
	if _, err := se.begin(person, "openai", "con_0000000000"); !isCode(err, service.CodeConnectionNotFound) {
		t.Errorf("an unknown connection = %v, want E_CONNECTION_NOT_FOUND", err)
	}
}

// SDD-016 section 12.4: when the person takes the access back on the account of the provider, a token or a key
// that has not expired is refused; the error asks for a new sign-in, not for a key, and deletes nothing.
func TestSignInRevokedAtTheProviderAsksForANewSignIn(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	person := se.asPerson()
	chat := se.signIn("openai", "")
	router := se.signIn("openrouter", "")

	se.fake.RevokeAll()

	for name, id := range map[string]string{"ChatGPT": chat.ConnectionID, "OpenRouter": router.ConnectionID} {
		_, err := se.svc.ProviderModels(person, service.ProviderModelsRequest{ConnectionID: id, Refresh: true})

		se2, ok := errors.AsType[*service.Error](err)
		if !ok || se2.Code != "E_PROVIDER" || se2.Details["code"] != "needs_sign_in" ||
			strings.Contains(se2.Hint, "connection:add") {
			t.Errorf("%s revoked at the provider = %v (hint %q), want needs_sign_in", name, err, hintOf(err))
		}
	}

	if se.connectionCount() != 3 {
		t.Errorf("a revoked sign-in removed a connection: %d left, want 3", se.connectionCount())
	}
}

func hintOf(err error) string {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se.Hint
	}

	return ""
}

// SDD-016 section 12.2: a sign-in grants text only. An image or a voice asked of a ChatGPT connection is
// refused before a job or an approval exists, even when a person has set a price for the model, and the
// token of the plan never reaches the image or the speech endpoint.
func TestSignInChatGPTConnectionGeneratesNoImageAndNoVoice(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")

	for unit, model := range map[string]string{"image": imageModel, "mchar": voiceModel} {
		micros := int64(40_000)
		if _, err := se.svc.ProviderPriceSet(se.asPerson(), service.ProviderPriceSetRequest{
			Provider: "openai", Model: model, Currency: "USD", Unit: unit, UnitMicros: &micros,
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, ierr := se.svc.ImageGenerate(se.as(token), service.ImageGenerateRequest{
		ProjectID: se.prj, ConnectionID: done.ConnectionID, ImageRequest: imageRequest(1, "a loaf"),
	})
	_, verr := se.svc.VoiceGenerate(se.as(token), service.VoiceGenerateRequest{
		ProjectID: se.prj, ConnectionID: done.ConnectionID, VoiceRequest: voiceRequest("Hello.", "mp3"),
	})

	for name, err := range map[string]error{"an image": ierr, "a voice": verr} {
		se2, ok := errors.AsType[*service.Error](err)
		if !ok || se2.Code != "E_VALIDATION" || se2.Path != "args.connection_id" {
			t.Errorf("%s through a sign-in = %v, want E_VALIDATION at args.connection_id", name, err)
		}
	}

	approvals, _ := se.svc.ApprovalList(se.as(token), service.ApprovalListRequest{State: "all"})
	if len(approvals.Requests) != 0 {
		t.Errorf("a refused generation left %d approval requests", len(approvals.Requests))
	}
}

// D-094, SDD-016 section 12.3: a server in network mode refuses Sign in with ChatGPT with the hint to use an
// API key, even when the browser is at 127.0.0.1, and makes no connection; OpenRouter still signs in.
func TestSignInWithChatGPTIsRefusedOnANetworkServer(t *testing.T) {
	t.Parallel()

	se := newSignInEnvIn(t, true)
	before := se.connectionCount()

	_, err := se.begin(se.asPerson(), "openai", "")

	se2, ok := errors.AsType[*service.Error](err)
	if !ok || se2.Code != "E_VALIDATION" || se2.Path != "provider" || !strings.Contains(se2.Hint, "API key") {
		t.Fatalf("Sign in with ChatGPT on a network server = %v, want E_VALIDATION at provider with a hint", err)
	}

	if se.connectionCount() != before || se.fake.Exchanges() != 0 {
		t.Fatalf("a refused sign-in made a connection or reached the provider")
	}

	if done := se.signIn("openrouter", ""); done.State != service.SignInReady {
		t.Fatalf("OpenRouter on a network server: %+v", done)
	}
}
