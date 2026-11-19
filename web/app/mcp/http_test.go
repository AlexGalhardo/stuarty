package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
)

// authenticate stands in for the guard of the API: it puts the Caller named by
// a test header in the request context, as the bearer token check does.
func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if actor := r.Header.Get("X-Test-Actor"); actor != "" {
			r = r.WithContext(service.WithCaller(r.Context(), service.Caller{Actor: actor, Scopes: service.AllScopes}))
		}

		next.ServeHTTP(w, r)
	})
}

type actorTransport struct {
	actor string
	next  http.RoundTripper
}

func (a actorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if a.actor != "" {
		r.Header.Set("X-Test-Actor", a.actor)
	}

	return a.next.RoundTrip(r)
}

func httpClient(t *testing.T, srv *httptest.Server, actor string) *sdk.ClientSession {
	t.Helper()

	// A copy: srv.Client() is shared, and every client needs its own credential.
	own := *srv.Client()
	hc := &own
	hc.Transport = actorTransport{actor: actor, next: srv.Client().Transport}

	client := sdk.NewClient(&sdk.Implementation{Name: "http-client", Version: "1"}, nil)

	cs, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// TestMCPCallerPropagation (SDD-003 section 4.5 rule 4): two calls with two
// credentials are attributed to two different actors, and a call with none is
// refused before it reaches the Service.
func TestMCPCallerPropagation(t *testing.T) {
	t.Parallel()

	h, err := Handler(newService(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(authenticate(h))
	defer srv.Close()

	alice := httpClient(t, srv, "key:alice")
	bob := httpClient(t, srv, "key:bob")

	id, _ := structured(t, ok(t, call(t, alice, "project_create", map[string]any{"name": "Shared"})))["project_id"].(string)

	edit := func(cs *sdk.ClientSession, base int) map[string]any {
		return structured(t, ok(t, call(t, cs, "edit", map[string]any{
			"project_id": id, "base_revision": base,
			"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual"}}},
		})))
	}

	if a := edit(alice, 0)["actor"]; a != "key:alice" {
		t.Fatalf("first actor = %v", a)
	}

	if b := edit(bob, 1)["actor"]; b != "key:bob" {
		t.Fatalf("second actor = %v", b)
	}

	anon := httpClient(t, srv, "")
	res := call(t, anon, "project_list", map[string]any{})

	if !res.IsError || !strings.Contains(textOf(t, res), "error E_UNAUTHORIZED:") {
		t.Fatalf("a call with no credentials must be E_UNAUTHORIZED: %q", textOf(t, res))
	}
}

func TestHTTPIsStateless(t *testing.T) {
	t.Parallel()

	h, err := Handler(newService(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(authenticate(h))
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req, err := http.NewRequestWithContext(t.Context(), method, srv.URL, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("X-Test-Actor", "key:a")
		req.Header.Set("Accept", "application/json, text/event-stream")

		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = res.Body.Close()

		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s answered %d, want 405", method, res.StatusCode)
		}
	}

	cs := httpClient(t, srv, "key:a")
	ok(t, call(t, cs, "project_list", map[string]any{}))
}

// TestHTTPUnknownMethodIsJSONRPC (VV-195): a method the server does not know is
// a JSON-RPC -32601 error that carries the id of the request, not plain text.
func TestHTTPUnknownMethodIsJSONRPC(t *testing.T) {
	t.Parallel()

	h, err := Handler(newService(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(authenticate(h))
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":7,"method":"bogus/method"}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("X-Test-Actor", "key:a")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}

	var out struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	if out.JSONRPC != "2.0" || string(out.ID) != "7" || out.Error.Code != -32601 ||
		!strings.Contains(out.Error.Message, "bogus/method") {
		t.Fatalf("answer = %+v (status %d)", out, res.StatusCode)
	}
}

// TestHTTPMalformedBodyKeepsItsAnswer: only the unknown method is rewritten.
func TestHTTPMalformedBodyKeepsItsAnswer(t *testing.T) {
	t.Parallel()

	h, err := Handler(newService(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(authenticate(h))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("X-Test-Actor", "key:a")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}
