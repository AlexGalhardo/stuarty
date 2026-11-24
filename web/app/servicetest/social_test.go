package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/web/app/testpublish"
)

const (
	clientSecretCanary = testpublish.ClientSecretCanary
	refreshCanary      = testpublish.RefreshCanary
	redirect           = testpublish.Redirect
)

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
	net  *testpublish.FakeNet
	conn *fake.Connector
	ops  socialOps
}

// newSocialEnv is an environment with the fake network registered, one line, as the epic asks.
func newSocialEnv(t *testing.T) *socialEnv {
	t.Helper()

	return newSocialEnvWith(t, nil)
}

// newSocialEnvWith is newSocialEnv with a change to the options of the service.
func newSocialEnvWith(t *testing.T, extra func(o *service.Options)) *socialEnv {
	t.Helper()

	n := testpublish.NewFakeNet(t)
	conn := fake.New(n.Srv.URL)
	reg := social.NewRegistry()

	if err := reg.Register(conn); err != nil {
		t.Fatal(err)
	}

	return newSocialEnvFor(t, n, conn, reg, extra)
}

// newSocialEnvFor is the environment over a network, its connector and the registry that holds it.
func newSocialEnvFor(
	t *testing.T, n *testpublish.FakeNet, conn *fake.Connector, reg *social.Registry, extra func(o *service.Options),
) *socialEnv {
	t.Helper()

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), vault.NewMemoryProvider(nil)),
		})
		o.Social = reg

		if extra != nil {
			extra(o)
		}
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

	if e.net.Count() != 0 {
		t.Fatalf("beginning the sign-in made a request: %v", e.net.Seen())
	}

	u, _ := url.Parse(begun.AuthorizeURL)
	q := u.Query()

	if !strings.HasPrefix(begun.AuthorizeURL, e.net.Srv.URL+"/authorize?") || q.Get("client_id") != "client-id-1" ||
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

	form := e.net.Forms[0]
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

	if e.net.Count() != 0 {
		t.Errorf("a refused sign-in made requests: %v", e.net.Seen())
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
	e.net.BadCode = true
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

// VV-323: a sign-in that fails does not leave a connection with no account behind, whether the person
// cancelled or the network refused the code; signing in again from a connection that was there stays possible.
func TestSocialFailedSignInLeavesNoConnection(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	count := func() int {
		list, err := e.svc.ConnectionList(person, service.ConnectionListRequest{})
		if err != nil {
			t.Fatal(err)
		}

		return len(list.Connections)
	}

	begun := e.begin(person)
	if count() != 1 {
		t.Fatalf("begin made %d connections, want 1", count())
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Error: "access_denied"},
	); err == nil {
		t.Fatal("a cancelled consent was accepted")
	}

	if count() != 0 {
		t.Errorf("a cancelled sign-in left %d connections", count())
	}

	e.net.BadCode = true
	begun = e.begin(person)

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "stale"},
	); err == nil {
		t.Fatal("a refused code was accepted")
	}

	if count() != 0 {
		t.Errorf("a refused code left %d connections", count())
	}

	// A connection that was signed in before keeps standing when a later sign-in fails.
	e.net.BadCode = false
	begun = e.begin(person)

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, begun.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	again, err := e.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: fake.Network, ConnectionID: begun.ConnectionID, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, again.AuthorizeURL), Error: "access_denied"},
	); err == nil {
		t.Fatal("a cancelled consent was accepted")
	}

	if count() != 1 {
		t.Errorf("a failed sign-in again removed the connection that was there: %d", count())
	}
}

// VV-323: two sign-ins on the same connection. The first one made it; a second one, begun from that connection
// ("Sign in again"), signs in. The first one failing afterwards must not remove a connection that now holds an
// account.
func TestSocialFailedSignInKeepsAConnectionAnotherSignInFinished(t *testing.T) {
	e := newSocialEnv(t)
	person := e.browser(ui)

	first := e.begin(person)

	second, err := e.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: fake.Network, ConnectionID: first.ConnectionID, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, second.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, first.AuthorizeURL), Error: "access_denied"},
	); err == nil {
		t.Fatal("a cancelled consent was accepted")
	}

	list, err := e.ops.SocialAccountList(person, service.SocialAccountListRequest{})
	if err != nil || len(list.Accounts) != 1 || list.Accounts[0].State != service.AccountReady {
		t.Fatalf("the signed-in connection is gone or changed: %+v, %v", list, err)
	}

	// The same while the second sign-in is still open: the first one failing leaves the connection for it.
	third := e.begin(person)

	fourth, err := e.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: fake.Network, ConnectionID: third.ConnectionID, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, third.AuthorizeURL), Error: "access_denied"},
	); err == nil {
		t.Fatal("a cancelled consent was accepted")
	}

	if _, err := e.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(t, fourth.AuthorizeURL), Code: "c"},
	); err != nil {
		t.Fatalf("the open sign-in lost its connection: %v", err)
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
