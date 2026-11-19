package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

const (
	clientSecretCanary = "GOCSPX-client-secret-CANARY-77"
	refreshCanary      = "refresh-token-CANARY-88"
	redirect           = "http://127.0.0.1:7878/social/callback"
)

// fakeNet is a fake social network that records every request that reaches it: the recording
// destination of the tests of epic 18.
type fakeNet struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []string
	forms    []url.Values
	videos   map[string]string // id to the state it answers
	badCode  bool
}

func newFakeNet(t *testing.T) *fakeNet {
	t.Helper()

	n := &fakeNet{videos: map[string]string{}}

	n.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		defer n.mu.Unlock()

		n.requests = append(n.requests, r.Method+" "+r.URL.Path)

		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			n.forms = append(n.forms, r.PostForm)

			if n.badCode {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))

				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at-1", "refresh_token": refreshCanary, "expires_in": 3600,
			})
		case r.URL.Path == "/account":
			_, _ = w.Write([]byte(`{"id":"UC-FAKE","title":"Fake channel"}`))
		case r.URL.Path == "/videos" && r.Method == http.MethodPost:
			id := "vid" + string(rune('a'+len(n.videos)))
			n.videos[id] = "published"

			_ = json.NewEncoder(w).
				Encode(map[string]string{"id": id, "url": "https://fake.example/watch/" + id, "state": "published"})
		case strings.HasPrefix(r.URL.Path, "/videos/"):
			id := strings.TrimPrefix(r.URL.Path, "/videos/")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "state": n.videos[id]})
		default:
			_, _ = w.Write([]byte("pong"))
		}
	}))

	t.Cleanup(n.srv.Close)

	return n
}

func (n *fakeNet) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return len(n.requests)
}

func (n *fakeNet) seen() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return append([]string(nil), n.requests...)
}

// socialOps is the part of the service the sign-in tests use (the methods join service.Service with
// the surfaces of task 7).
type socialOps interface {
	SocialNetworkList(
		ctx context.Context,
		req service.SocialNetworkListRequest,
	) (*service.SocialNetworkListResult, error)
	SocialConnectBegin(
		ctx context.Context,
		req service.SocialConnectBeginRequest,
	) (*service.SocialConnectBeginResult, error)
	SocialConnectComplete(ctx context.Context, req service.SocialConnectCompleteRequest) (*service.SocialAccount, error)
	SocialAccountList(
		ctx context.Context,
		req service.SocialAccountListRequest,
	) (*service.SocialAccountListResult, error)
}

type socialEnv struct {
	*env
	net  *fakeNet
	conn *fake.Connector
	ops  socialOps
}

// newSocialEnv is an environment with the fake network registered, one line, as the epic asks.
func newSocialEnv(t *testing.T) *socialEnv {
	t.Helper()

	n := newFakeNet(t)
	conn := fake.New(n.srv.URL)
	reg := social.NewRegistry()

	if err := reg.Register(conn); err != nil {
		t.Fatal(err)
	}

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), vault.NewMemoryProvider(nil)),
		})
		o.Social = reg
	})

	ops, ok := e.svc.(socialOps)
	if !ok {
		t.Fatal("the service has no social operations")
	}

	return &socialEnv{env: e, net: n, conn: conn, ops: ops}
}

// browser is a person at the app: the session cookie, every scope.
func (e *env) browser(actor string) context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: actor, Scopes: service.AllScopes, Browser: true},
	)
}

func stateOf(t *testing.T, authorize string) string {
	t.Helper()

	u, err := url.Parse(authorize)
	if err != nil || u.Query().Get("state") == "" {
		t.Fatalf("authorize address %q: %v", authorize, err)
	}

	return u.Query().Get("state")
}

func (e *socialEnv) begin(ctx context.Context) *service.SocialConnectBeginResult {
	e.t.Helper()

	res, err := e.ops.SocialConnectBegin(ctx, service.SocialConnectBeginRequest{
		Network: fake.Network, ClientID: "client-id-1", ClientSecret: clientSecretCanary, RedirectURI: redirect,
	})
	if err != nil {
		e.t.Fatal(err)
	}

	return res
}

func TestSocialSignInEndToEnd(t *testing.T) {
	e := newSocialEnv(t)
	ui1 := e.browser(ui)

	nets, err := e.ops.SocialNetworkList(ui1, service.SocialNetworkListRequest{})
	if err != nil || len(nets.Networks) != 1 || nets.Networks[0].Network != fake.Network ||
		nets.RedirectPath != "/social/callback" {
		t.Fatalf("networks = %+v, %v", nets, err)
	}

	begun := e.begin(ui1)

	if e.net.count() != 0 {
		t.Fatalf("beginning the sign-in made a request: %v", e.net.seen())
	}

	u, _ := url.Parse(begun.AuthorizeURL)
	q := u.Query()

	if !strings.HasPrefix(begun.AuthorizeURL, e.net.srv.URL+"/authorize?") || q.Get("client_id") != "client-id-1" ||
		q.Get("redirect_uri") != redirect || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Errorf("authorize address = %s", begun.AuthorizeURL)
	}

	if strings.Contains(begun.AuthorizeURL, clientSecretCanary) {
		t.Error("the authorization address carries the client secret")
	}

	before, err := e.ops.SocialAccountList(ui1, service.SocialAccountListRequest{})
	if err != nil || len(before.Accounts) != 1 || before.Accounts[0].State != service.AccountNeedsSignIn ||
		before.Accounts[0].ConnectionID != begun.ConnectionID {
		t.Fatalf("accounts before the sign-in = %+v, %v", before, err)
	}

	acc, err := e.ops.SocialConnectComplete(
		ui1,
		service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "the-code"},
	)
	if err != nil || acc.State != service.AccountReady || acc.AccountID != "UC-FAKE" ||
		acc.AccountTitle != "Fake channel" {
		t.Fatalf("complete = %+v, %v", acc, err)
	}

	form := e.net.forms[0]
	if form.Get("code") != "the-code" || form.Get("client_secret") != clientSecretCanary ||
		form.Get("code_verifier") == "" {
		t.Errorf("the token request = %v", form)
	}

	after, err := e.ops.SocialAccountList(ui1, service.SocialAccountListRequest{})
	if err != nil || len(after.Accounts) != 1 || after.Accounts[0].State != service.AccountReady {
		t.Fatalf("accounts after = %+v, %v", after, err)
	}

	for name, v := range map[string]any{"begin": begun, "complete": acc, "accounts": after} {
		raw, _ := json.Marshal(v) //nolint:errchkjson // a result type of the service, only searched for the canaries
		if strings.Contains(string(raw), clientSecretCanary) || strings.Contains(string(raw), refreshCanary) {
			t.Errorf("the result of %s carries a secret", name)
		}
	}

	conn, err := e.svc.ConnectionGet(ui1, service.ConnectionRef{ID: begun.ConnectionID})
	if err != nil || conn.Kind != fake.Network || !conn.SecretSet {
		t.Fatalf("connection = %+v, %v", conn, err)
	}
}

func TestSocialSignInStateIsSingleUseAndBound(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	begun := e.begin(person)
	state := stateOf(t, begun.AuthorizeURL)

	// An agent with the local token is not a person at the browser.
	if _, err := e.ops.SocialConnectComplete(
		e.as(token),
		service.SocialConnectCompleteRequest{State: state, Code: "c"},
	); !isCode(
		err,
		service.CodePrivilegedSession,
	) {
		t.Errorf("the local token completing a sign-in = %v, want E_PRIVILEGED_SESSION", err)
	}

	// Another browser session than the one that began it.
	if _, err := e.ops.SocialConnectComplete(
		e.browser("local:other"),
		service.SocialConnectCompleteRequest{State: state, Code: "c"},
	); !isCode(
		err,
		"E_VALIDATION",
	) {
		t.Errorf("another session completing = %v, want E_VALIDATION on the state", err)
	}

	// That attempt spent the state.
	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: state, Code: "c"},
	); !isCode(
		err,
		"E_VALIDATION",
	) {
		t.Errorf("the spent state = %v", err)
	}

	if e.net.count() != 0 {
		t.Errorf("a refused sign-in made requests: %v", e.net.seen())
	}

	// A guessed state.
	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: "guess", Code: "c"},
	); !isCode(
		err,
		"E_VALIDATION",
	) {
		t.Errorf("a guessed state = %v", err)
	}

	// A replay after a success.
	begun = e.begin(person)
	state = stateOf(t, begun.AuthorizeURL)

	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: state, Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: state, Code: "c"},
	); !isCode(
		err,
		"E_VALIDATION",
	) {
		t.Errorf("a replayed state = %v", err)
	}
}

func TestSocialSignInRefusedByTheNetwork(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	// The person pressed Cancel on the consent screen.
	begun := e.begin(person)
	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Error: "access_denied"},
	); !isCode(
		err,
		"E_VALIDATION",
	) {
		t.Errorf("a cancelled consent = %v", err)
	}

	// The network refuses the code.
	e.net.badCode = true
	begun = e.begin(person)

	_, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "stale"},
	)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodePublish || se.Details["kind"] != social.KindAuth ||
		se.Hint == "" {
		t.Fatalf("a refused code = %v, want an actionable E_PUBLISH of kind auth", err)
	}

	list, _ := e.ops.SocialAccountList(person, service.SocialAccountListRequest{})
	for _, a := range list.Accounts {
		if a.State != service.AccountNeedsSignIn {
			t.Errorf("account %+v is ready after a refused sign-in", a)
		}
	}
}

func TestSocialBeginValidatesTheRequest(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	for name, req := range map[string]service.SocialConnectBeginRequest{
		"an unknown network":    {Network: "nope", ClientID: "a", ClientSecret: "b", RedirectURI: redirect},
		"no client":             {Network: fake.Network, RedirectURI: redirect},
		"a redirect elsewhere":  {Network: fake.Network, ClientID: "a", ClientSecret: "b", RedirectURI: "https://evil.example/steal"},
		"a plain http redirect": {Network: fake.Network, ClientID: "a", ClientSecret: "b", RedirectURI: "http://example.com/social/callback"},
		"a redirect with query": {Network: fake.Network, ClientID: "a", ClientSecret: "b", RedirectURI: redirect + "?x=1"},
		"a missing connection":  {Network: fake.Network, ConnectionID: "con_zzzzzzzzzz", RedirectURI: redirect},
	} {
		if _, err := e.ops.SocialConnectBegin(person, req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if list, _ := e.svc.ConnectionList(person, service.ConnectionListRequest{}); len(list.Connections) != 0 {
		t.Errorf("a refused begin left %d connection(s)", len(list.Connections))
	}

	// A key limited to a project, and a key without the admin scope, cannot sign in.
	if _, err := e.ops.SocialConnectBegin(
		as(service.ScopeRead),
		service.SocialConnectBeginRequest{
			Network:      fake.Network,
			ClientID:     "a",
			ClientSecret: "b",
			RedirectURI:  redirect,
		},
	); !isCode(
		err,
		service.CodeForbidden,
	) {
		t.Errorf("a read key beginning a sign-in = %v", err)
	}
}

func TestSocialSignInAgainReusesTheClient(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	begun := e.begin(person)
	if _, err := e.ops.SocialConnectComplete(
		person,
		service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	again, err := e.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: fake.Network, ConnectionID: begun.ConnectionID, RedirectURI: redirect,
	})
	if err != nil || again.ConnectionID != begun.ConnectionID ||
		!strings.Contains(again.AuthorizeURL, "client_id=client-id-1") {
		t.Fatalf("sign in again = %+v, %v", again, err)
	}

	if list, _ := e.svc.ConnectionList(person, service.ConnectionListRequest{}); len(list.Connections) != 1 {
		t.Errorf("signing in again made a new connection: %d", len(list.Connections))
	}
}
