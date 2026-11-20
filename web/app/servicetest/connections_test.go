package servicetest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// strayConnector is a connector that sends the credential to a host its connection does not list, as
// a defective connector or a URL taken from a response would.
type strayConnector struct{ target string }

func (s strayConnector) Test(ctx context.Context, c connections.Credential) (connections.TestResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.target, http.NoBody)
	if err != nil {
		return connections.TestResult{}, err
	}

	req.Header.Set("Authorization", "Bearer "+string(c.Secret))

	res, err := c.Client.Do(req)
	if err != nil {
		return connections.TestResult{}, err
	}

	_ = res.Body.Close()

	return connections.TestResult{OK: true, Status: res.StatusCode}, nil
}

// Acceptance criterion 2 of epic 04 (SDD-014 sections 6 and 7): a request through a connection to a
// host outside its list fails with the stable code E_CONNECTION_DESTINATION (HTTP 403) and reaches
// no network: no dial is attempted and the other host receives nothing.
func TestConnectionToAnotherHostIsADestinationError(t *testing.T) {
	listed := newDestination(t)
	other := newDestination(t)

	var mu sync.Mutex

	dials := 0
	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		mu.Lock()
		dials++
		mu.Unlock()

		return nil
	})
	t.Cleanup(restore)

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		m := connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Now: e.clock.now,
			Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), vault.NewMemoryProvider(nil)),
		})
		// The other server listens on the same address under another name, so only the host rule
		// can refuse it.
		m.Register("stray", strayConnector{target: strings.Replace(other.srv.URL, "127.0.0.1", "localhost", 1)})
		o.Connections = m
	})

	req := connectionRequest(listed.srv.URL, "tok-STRAY-CANARY")
	req.Kind = "stray"

	created, err := e.svc.ConnectionCreate(e.as(ui), req)
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.ConnectionTest(e.as(ui), service.ConnectionRef{ID: created.ID})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeConnectionDestination || se.HTTPStatus() != http.StatusForbidden {
		t.Fatalf("a request to another host: %v, want E_CONNECTION_DESTINATION and 403", err)
	}

	if strings.Contains(se.Error(), "tok-STRAY-CANARY") {
		t.Error("the error carries the credential")
	}

	mu.Lock()
	defer mu.Unlock()

	if dials != 0 || other.hits() != 0 || listed.hits() != 0 {
		t.Errorf("a refused request reached the network: %d dial(s), other %d, listed %d", dials, other.hits(),
			listed.hits())
	}
}

// destination is a fake server that records the Authorization header it receives.
type destination struct {
	srv  *httptest.Server
	mu   sync.Mutex
	auth []string
}

func newDestination(t *testing.T) *destination {
	t.Helper()

	d := &destination{}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.auth = append(d.auth, r.Header.Get("Authorization"))
		d.mu.Unlock()
		_, _ = w.Write([]byte("pong"))
	}))

	t.Cleanup(d.srv.Close)

	return d
}

func (d *destination) hits() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.auth)
}

// connEnv is an environment whose service has a vault in the workspace and a
// master key kept in memory.
type connEnv struct {
	*env
	vaultPath string
}

func newConnEnv(tb testing.TB, keys vault.KeyProvider) *connEnv {
	tb.Helper()

	c := &connEnv{}

	c.env = newEnvWith(tb, func(e *env, o *service.Options) {
		c.vaultPath = filepath.Join(string(e.dir), "vault.json")
		o.Workspace = string(e.dir)
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Vault: vault.Open(c.vaultPath, keys), Now: e.clock.now,
		})
	})

	return c
}

func connectionRequest(endpoint, secret string) service.ConnectionCreateRequest {
	return service.ConnectionCreateRequest{
		Kind: connections.KindTest, Label: "Test destination", Endpoint: endpoint,
		Accepts: []string{connections.AcceptText}, Secret: secret,
	}
}

func as(scopes ...string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: "key:key_aaaaaaaaaa", Scopes: scopes})
}

func TestConnectionLifecycleThroughTheService(t *testing.T) {
	dst := newDestination(t)
	e := newConnEnv(t, vault.NewMemoryProvider(nil))
	ctx := e.as(ui)

	list, err := e.svc.ConnectionList(ctx, service.ConnectionListRequest{})
	if err != nil || len(list.Connections) != 0 {
		t.Fatalf("an empty workspace: %+v, %v", list, err)
	}

	const secret = "sk-test-SECRET-9f8e7d6c"

	created, err := e.svc.ConnectionCreate(ctx, connectionRequest(dst.srv.URL+"/ping", secret))
	if err != nil {
		t.Fatal(err)
	}

	if !created.SecretSet || !strings.HasPrefix(created.ID, "con_") || len(created.Hosts) != 1 {
		t.Fatalf("created %+v", created)
	}

	// No result of any operation carries the secret.
	if raw, merr := json.Marshal(created); merr != nil || strings.Contains(string(raw), secret) {
		t.Errorf("the created connection carries the secret: %v", merr)
	}

	got, err := e.svc.ConnectionGet(ctx, service.ConnectionRef{ID: created.ID})
	if err != nil || got.ID != created.ID || got.LastUsedAt != nil {
		t.Fatalf("get %+v, %v", got, err)
	}

	tested, err := e.svc.ConnectionTest(ctx, service.ConnectionRef{ID: created.ID})
	if err != nil || !tested.OK || tested.Status != 200 || !strings.Contains(tested.Message, "pong") {
		t.Fatalf("test %+v, %v", tested, err)
	}

	if dst.hits() != 1 || dst.auth[0] != "Bearer "+secret {
		t.Errorf("the destination saw %v", dst.auth)
	}

	got, _ = e.svc.ConnectionGet(ctx, service.ConnectionRef{ID: created.ID})
	if got.LastUsedAt == nil {
		t.Error("the last use was not recorded")
	}

	// The vault file holds the entry id and no plaintext.
	raw, err := os.ReadFile(e.vaultPath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), created.ID) {
		t.Error("the vault file holds the secret in clear or lacks the entry")
	}

	removed, err := e.svc.ConnectionRemove(ctx, service.ConnectionRef{ID: created.ID})
	if err != nil || removed.SecretSet {
		t.Fatalf("remove %+v, %v", removed, err)
	}

	raw, _ = os.ReadFile(e.vaultPath)
	if strings.Contains(string(raw), created.ID+`"`) && strings.Contains(string(raw), `"`+created.ID+`": {`) {
		t.Error("the vault still holds the entry of a removed connection")
	}

	if _, err := e.svc.ConnectionTest(
		ctx,
		service.ConnectionRef{ID: created.ID},
	); code(
		err,
	) != service.CodeConnectionNotFound {
		t.Errorf("a test after removal: %v, want E_CONNECTION_NOT_FOUND", err)
	}

	if _, err := e.svc.ConnectionGet(
		ctx,
		service.ConnectionRef{ID: created.ID},
	); code(
		err,
	) != service.CodeConnectionNotFound {
		t.Errorf("get after removal: %v", err)
	}

	if dst.hits() != 1 {
		t.Errorf("traffic after removal: %d requests", dst.hits())
	}

	audit, err := e.svc.AuditList(ctx, service.AuditListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var actions []string

	for _, r := range audit.Entries {
		if strings.HasPrefix(r.Action, "connection.") {
			actions = append(actions, r.Action)

			if r.SubjectID != created.ID {
				t.Errorf("audit row %+v has the wrong subject", r)
			}
		}
	}

	if strings.Join(actions, ",") != "connection.remove,connection.create" &&
		strings.Join(actions, ",") != "connection.create,connection.remove" {
		t.Errorf("audit actions %v", actions)
	}

	if raw, merr := json.Marshal(audit); merr != nil || strings.Contains(string(raw), secret) {
		t.Error("the audit list carries the secret")
	}
}

func TestConnectionScopes(t *testing.T) {
	dst := newDestination(t)
	e := newConnEnv(t, vault.NewMemoryProvider(nil))

	created, err := e.svc.ConnectionCreate(e.as(ui), connectionRequest(dst.srv.URL, "tok"))
	if err != nil {
		t.Fatal(err)
	}

	reader := as(service.ScopeRead)
	editor := as(service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender)

	for name, ctx := range map[string]context.Context{"read": reader, "editor": editor} {
		if _, err := e.svc.ConnectionList(ctx, service.ConnectionListRequest{}); err != nil {
			t.Errorf("%s: list: %v", name, err)
		}

		if _, err := e.svc.ConnectionGet(ctx, service.ConnectionRef{ID: created.ID}); err != nil {
			t.Errorf("%s: get: %v", name, err)
		}

		if _, err := e.svc.ConnectionCreate(
			ctx,
			connectionRequest(dst.srv.URL, "tok"),
		); code(
			err,
		) != service.CodeForbidden {
			t.Errorf("%s: create: %v, want E_FORBIDDEN", name, err)
		}

		if _, err := e.svc.ConnectionRemove(
			ctx,
			service.ConnectionRef{ID: created.ID},
		); code(
			err,
		) != service.CodeForbidden {
			t.Errorf("%s: remove: %v, want E_FORBIDDEN", name, err)
		}

		if _, err := e.svc.ConnectionTest(
			ctx,
			service.ConnectionRef{ID: created.ID},
		); code(
			err,
		) != service.CodeForbidden {
			t.Errorf("%s: test: %v, want E_FORBIDDEN", name, err)
		}
	}

	if dst.hits() != 0 {
		t.Error("a refused test reached the destination")
	}

	// A key limited to a project cannot use connections at all.
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_bbbbbbbbbb", Scopes: service.AllScopes, ProjectID: "prj_0000000001",
	})
	if _, err := e.svc.ConnectionList(limited, service.ConnectionListRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("project-limited list: %v", err)
	}

	// No caller at all.
	if _, err := e.svc.ConnectionList(
		context.Background(),
		service.ConnectionListRequest{},
	); code(
		err,
	) != service.CodeUnauthorized {
		t.Errorf("no caller: %v", err)
	}
}

func TestConnectionCreateValidation(t *testing.T) {
	e := newConnEnv(t, vault.NewMemoryProvider(nil))
	ctx := e.as(ui)

	for name, tc := range map[string]struct {
		mutate func(*service.ConnectionCreateRequest)
		path   string
	}{
		"kind":     {func(r *service.ConnectionCreateRequest) { r.Kind = "nope" }, "kind"},
		"endpoint": {func(r *service.ConnectionCreateRequest) { r.Endpoint = "http://example.com" }, "endpoint"},
		"secret":   {func(r *service.ConnectionCreateRequest) { r.Secret = "" }, "secret"},
		"accepts":  {func(r *service.ConnectionCreateRequest) { r.Accepts = nil }, "accepts"},
	} {
		req := connectionRequest("https://example.com", "tok")
		tc.mutate(&req)

		_, err := e.svc.ConnectionCreate(ctx, req)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != "E_VALIDATION" || se.Path != tc.path {
			t.Errorf("%s: %v, want E_VALIDATION at %s", name, err, tc.path)
		}
	}

	list, _ := e.svc.ConnectionList(ctx, service.ConnectionListRequest{})
	if len(list.Connections) != 0 {
		t.Errorf("a refused request left %d connections", len(list.Connections))
	}

	if _, err := os.Stat(e.vaultPath); err == nil {
		t.Error("a refused request created the vault")
	}
}

func TestConnectionLimit(t *testing.T) {
	e := newConnEnv(t, vault.NewMemoryProvider(nil))
	ctx := e.as(ui)

	for i := range connections.MaxConnections {
		if _, err := e.svc.ConnectionCreate(
			ctx,
			connectionRequest(fmt.Sprintf("https://h%d.example.com", i), "tok"),
		); err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
	}

	if _, err := e.svc.ConnectionCreate(
		ctx,
		connectionRequest("https://more.example.com", "tok"),
	); code(
		err,
	) != "E_LIMIT_EXCEEDED" {
		t.Errorf("the 51st connection: %v, want E_LIMIT_EXCEEDED", err)
	}
}

func TestVaultLockedErrors(t *testing.T) {
	// No key source: a connection cannot be created, and nothing is left behind.
	e := newConnEnv(t, vault.NoProvider())

	_, err := e.svc.ConnectionCreate(e.as(ui), connectionRequest("https://example.com", "tok"))

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeVaultLocked || se.Details["reason"] != "no_key_provider" ||
		!strings.Contains(se.Hint, "STUARTY_MASTER_KEY_FILE") {
		t.Fatalf("%v (%+v)", err, se)
	}

	if se.HTTPStatus() != http.StatusServiceUnavailable {
		t.Errorf("status %d", se.HTTPStatus())
	}

	list, _ := e.svc.ConnectionList(e.as(ui), service.ConnectionListRequest{})
	if len(list.Connections) != 0 {
		t.Error("a failed creation left a connection")
	}

	// The default service of a workspace with no manager behaves the same.
	plain := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	if _, err := plain.svc.ConnectionCreate(
		plain.as(ui),
		connectionRequest("https://example.com", "tok"),
	); code(
		err,
	) != service.CodeVaultLocked {
		t.Errorf("default manager: %v", err)
	}

	if res, err := plain.svc.ConnectionList(
		plain.as(ui),
		service.ConnectionListRequest{},
	); err != nil ||
		len(res.Connections) != 0 {
		t.Errorf("default manager list: %+v, %v", res, err)
	}
}

func TestVaultKeyLostAndWrong(t *testing.T) {
	dst := newDestination(t)
	key := vault.NewMemoryProvider(nil)
	e := newConnEnv(t, key)

	created, err := e.svc.ConnectionCreate(e.as(ui), connectionRequest(dst.srv.URL, "tok"))
	if err != nil {
		t.Fatal(err)
	}

	// A second service over the same workspace, as if the key were not there any more.
	for name, tc := range map[string]struct {
		keys   vault.KeyProvider
		reason string
	}{
		"missing": {vault.NewMemoryProvider(nil), "key_missing"},
		"wrong":   {vault.NewMemoryProvider(bytes32(7)), "wrong_key"},
	} {
		lost := newEnvWith(t, func(_ *env, o *service.Options) {
			o.Workspace = string(e.dir)
			o.Connections = connections.NewManager(connections.Options{
				Store: o.Store.Connections(), Vault: vault.Open(filepath.Join(string(e.dir), "vault.json"), tc.keys),
			})
		})

		// The connection lives in the first workspace's database, so copy its row by creating the
		// same one through this service's store: the vault file is the shared part.
		row := connections.Row{
			ID: created.ID, Kind: created.Kind, Label: created.Label, Endpoint: created.Endpoint,
			Hosts: created.Hosts, Accepts: created.Accepts, SecretRef: created.ID,
		}
		if err := lost.store.Connections().Insert(context.Background(), row); err != nil {
			t.Fatal(err)
		}

		_, err := lost.svc.ConnectionTest(lost.as(ui), service.ConnectionRef{ID: created.ID})

		var se *service.Error
		if !errors.As(err, &se) || se.Code != service.CodeVaultLocked || se.Details["reason"] != tc.reason {
			t.Errorf("%s: %v, want E_VAULT_LOCKED %s", name, err, tc.reason)
		}

		if !strings.Contains(se.Hint, "vault.json") {
			t.Errorf("%s: the hint does not say how to recover: %q", name, se.Hint)
		}
	}

	if dst.hits() != 0 {
		t.Error("a locked vault let a request through")
	}
}

// SDD-014 section 8: the owner removes the key variables from its environment, so doctor reports the
// key source in its own check: the provider by name, whether the vault exists, and a hint to recover
// when the key does not open it. Neither the key nor the secret is in the report.
func TestDoctorReportsTheVault(t *testing.T) {
	master := bytes32(0x3C)
	e := newConnEnv(t, vault.NewMemoryProvider(master))
	ctx := service.WithCaller(context.Background(), operator())

	vaultCheck := func(svc service.Service) service.DoctorCheck {
		t.Helper()

		res, err := svc.Doctor(ctx, service.DoctorRequest{Full: true})
		if err != nil {
			t.Fatal(err)
		}

		raw, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(raw), "tok-DOCTOR-CANARY") ||
			strings.Contains(string(raw), base64.StdEncoding.EncodeToString(master)) {
			t.Error("the doctor report holds the secret or the master key")
		}

		for _, c := range res.Checks {
			if c.Name == "vault" {
				return c
			}
		}

		t.Fatal("no vault check in the doctor report")

		return service.DoctorCheck{}
	}

	if c := vaultCheck(e.svc); !c.OK || c.Detail != "master key from memory; no vault file yet" {
		t.Errorf("before the first credential: %+v", c)
	}

	dst := newDestination(t)
	if _, err := e.svc.ConnectionCreate(e.as(ui), connectionRequest(dst.srv.URL, "tok-DOCTOR-CANARY")); err != nil {
		t.Fatal(err)
	}

	if c := vaultCheck(e.svc); !c.OK || c.Detail != "master key from memory; vault.json present" {
		t.Errorf("with a credential: %+v", c)
	}

	lost := newEnvWith(t, func(_ *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Vault: vault.Open(e.vaultPath, vault.NoProvider()),
		})
	})

	if c := vaultCheck(lost.svc); c.OK || c.Detail != "master key from none; vault.json present" ||
		!strings.Contains(c.Hint, "STUARTY_MASTER_KEY") {
		t.Errorf("without the key: %+v", c)
	}
}

func bytes32(b byte) []byte {
	out := make([]byte, vault.KeySize)
	for i := range out {
		out[i] = b
	}

	return out
}

// VV-269: a key file that holds no valid key is a failed vault check with the hint of the error, not an
// internal failure of the whole report; the bytes of the file are not in it.
func TestDoctorReportsAMasterKeyFileThatIsNotAKey(t *testing.T) {
	master := bytes32(0x3D)
	e := newConnEnv(t, vault.NewMemoryProvider(master))
	ctx := service.WithCaller(context.Background(), operator())

	dst := newDestination(t)
	if _, err := e.svc.ConnectionCreate(e.as(ui), connectionRequest(dst.srv.URL, "tok-DOCTOR-CANARY")); err != nil {
		t.Fatal(err)
	}

	keyFile := filepath.Join(t.TempDir(), "wrong.key")
	if err := os.WriteFile(keyFile, []byte("no-such-key-CANARY\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bad := newEnvWith(t, func(_ *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Connections = connections.NewManager(connections.Options{
			Store: o.Store.Connections(), Vault: vault.Open(e.vaultPath, vault.FileProvider{Path: keyFile}),
		})
	})

	res, err := bad.svc.Doctor(ctx, service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatalf("doctor failed instead of reporting: %v", err)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), "no-such-key-CANARY") {
		t.Error("the doctor report holds the bytes of the key file")
	}

	for _, c := range res.Checks {
		if c.Name == "vault" {
			if c.OK || !strings.Contains(c.Hint, "32 bytes") {
				t.Errorf("vault check: %+v", c)
			}

			return
		}
	}

	t.Fatal("no vault check in the doctor report")
}
