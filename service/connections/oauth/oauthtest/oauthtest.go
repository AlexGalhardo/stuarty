// Package oauthtest is a fake authorization server and provider API on loopback, for the tests of the
// sign-in flow (epic 12, SDD-016 section 12): it plays the OpenAI authorization and token endpoints, the
// OpenRouter key exchange and the model and text endpoints of both, checks PKCE, single-use codes and
// rotating refresh tokens, and records every token it issued so a test can look for one where it must
// not be. It is a test helper: the product never imports it, and it opens no connection to a real
// provider.
package oauthtest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Resource is the value of the resource parameter the plan sign-in must send.
const Resource = "https://api.openai.com/v1"

// Server is the fake. Its address is Server.URL.
type Server struct {
	// Server is the started server.
	Server *httptest.Server
	// Strict makes the fake refuse an authorization request or an exchange that lacks what the
	// documentation of the plan sign-in requires (the resource, the scope, the nonce, the state).
	Strict bool

	mu      sync.Mutex
	n       int
	codes   map[string]grant
	refresh map[string]string // current refresh token -> client id
	access  map[string]bool
	keys    map[string]bool
	issued  []string

	issuedClient string
	lifetime     int
	failStatus   int
	failCode     string
	refreshes    int
	exchanges    int
	refreshDelay time.Duration
}

type grant struct {
	challenge, redirect, client, nonce string
	resource                           bool
	used                               bool
}

// New starts the fake. The sign-in registers a client with the id issued, such as "oaiapp_test".
func New(t *testing.T) *Server {
	t.Helper()

	s := &Server{
		codes: map[string]grant{}, refresh: map[string]string{}, access: map[string]bool{}, keys: map[string]bool{},
		issuedClient: "oaiapp_test0123456789", lifetime: 3600,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/accounts/oauth/token", s.token)
	mux.HandleFunc("/api/v1/auth/keys", s.exchangeKey)
	mux.HandleFunc("/v1/models", s.planModels)
	mux.HandleFunc("/v1/responses", s.planResponses)
	mux.HandleFunc("/api/v1/models", s.routerModels)
	mux.HandleFunc("/api/v1/chat/completions", s.routerChat)
	mux.HandleFunc("/api/accounts/authorize", s.authorize)
	mux.HandleFunc("/auth", s.authorize)

	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)

	return s
}

// URL is the base address of the server.
func (s *Server) URL() string { return s.Server.URL }

// Host is the host and port of the server, as a connection lists it.
func (s *Server) Host() string { return strings.TrimPrefix(s.Server.URL, "http://") }

// SetLifetime is the expires_in of the next access tokens, in seconds.
func (s *Server) SetLifetime(seconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lifetime = seconds
}

// FailRefresh makes every refresh answer the status and the OAuth error code; status 0 stops it.
func (s *Server) FailRefresh(status int, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failStatus, s.failCode = status, code
}

// DelayRefresh makes each refresh wait, so concurrent callers meet in it.
func (s *Server) DelayRefresh(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refreshDelay = d
}

// Refreshes is how many refresh requests were made, and Exchanges how many code exchanges.
func (s *Server) Refreshes() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.refreshes
}

// Exchanges is how many code exchanges were made.
func (s *Server) Exchanges() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.exchanges
}

// IssuedClient is the client id the registration answers with.
func (s *Server) IssuedClient() string { return s.issuedClient }

// Tokens is every access token, refresh token, ID token and key the server issued, for a test that looks
// for one in a place it must not be.
func (s *Server) Tokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.issued...)
}

// Consent plays the person who accepts on the consent screen: it reads the authorization address and
// returns the address the provider sends the browser back to, with a code, the state and (for the plan
// sign-in) the issued client id.
func (s *Server) Consent(t *testing.T, authorizeURL string) string {
	t.Helper()

	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatal(err)
	}

	back, err := s.consent(u.Query())
	if err != nil {
		t.Fatal(err)
	}

	return back
}

func (s *Server) consent(q url.Values) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	redirect := q.Get("redirect_uri")
	client := q.Get("client_id")
	plan := redirect != ""

	if !plan {
		redirect = q.Get("callback_url")
	}

	if redirect == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		return "", fmt.Errorf("oauthtest: not an authorization request: %s", q.Encode())
	}

	if plan && s.Strict && (client == "" || q.Get("state") == "" || q.Get("nonce") == "" ||
		q.Get("resource") != Resource || q.Get("response_type") != "code" ||
		!strings.Contains(q.Get("scope"), "chatgpt.tokens.use.direct")) {
		return "", fmt.Errorf("oauthtest: the plan sign-in request is incomplete: %s", q.Encode())
	}

	s.n++
	code := fmt.Sprintf("code-%d", s.n)

	if plan && client == "dynamic_agent_client" {
		client = s.issuedClient
	}

	s.codes[code] = grant{
		challenge: q.Get("code_challenge"), redirect: redirect, client: client, nonce: q.Get("nonce"),
		resource: q.Get("resource") != "",
	}

	back, err := url.Parse(redirect)
	if err != nil {
		return "", err
	}

	b := back.Query()
	b.Set("code", code)

	if st := q.Get("state"); st != "" {
		b.Set("state", st)
	}

	if plan && q.Get("client_id") == "dynamic_agent_client" {
		b.Set("client_id", s.issuedClient)
	}

	back.RawQuery = b.Encode()

	return back.String(), nil
}

// authorize is the consent screen of a browser: it accepts at once and redirects back.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	back, err := s.consent(r.URL.Query())
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	//nolint:gosec // a fake that sends the browser back to the redirect it was given
	http.Redirect(w, r, back, http.StatusFound)
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) //nolint:errchkjson // a fake: every value is a map of plain values
}

func (s *Server) newToken(prefix string) string {
	s.n++
	tok := fmt.Sprintf("%s-SECRET-%d-%s", prefix, s.n, strings.Repeat("x", 12))
	s.issued = append(s.issued, tok)

	return tok
}

func (s *Server) idToken(client, nonce string) string {
	enc := func(v any) string {
		raw, _ := json.Marshal(v) //nolint:errchkjson // a fake: every value is a map of plain values

		return base64.RawURLEncoding.EncodeToString(raw)
	}

	tok := enc(map[string]string{"alg": "none"}) + "." + enc(map[string]any{
		"iss": s.Server.URL, "aud": client, "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce,
		"email": "person@example.test",
	}) + ".sig"
	s.issued = append(s.issued, tok)

	return tok
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.ParseForm() != nil {
		write(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.codeGrant(w, r.PostForm)
	case "refresh_token":
		s.refreshGrant(w, r.PostForm)
	default:
		write(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (s *Server) codeGrant(w http.ResponseWriter, f url.Values) {
	s.exchanges++

	g, ok := s.codes[f.Get("code")]
	if !ok || g.used || challenge(f.Get("code_verifier")) != g.challenge || f.Get("redirect_uri") != g.redirect ||
		f.Get("client_id") != g.client || (g.resource && f.Get("resource") != Resource) ||
		(s.Strict && f.Get("resource") != Resource) {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "no"})

		return
	}

	g.used = true
	s.codes[f.Get("code")] = g

	access, refresh := s.newToken("access"), s.newToken("refresh")
	s.access[access] = true
	s.refresh[refresh] = g.client

	write(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "expires_in": s.lifetime, "token_type": "Bearer",
		"scope":    "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
		"id_token": s.idToken(g.client, g.nonce),
	})
}

func (s *Server) refreshGrant(w http.ResponseWriter, f url.Values) {
	s.refreshes++

	if d := s.refreshDelay; d > 0 {
		s.mu.Unlock()
		time.Sleep(d)
		s.mu.Lock()
	}

	if s.failStatus != 0 {
		write(w, s.failStatus, map[string]string{"error": s.failCode, "error_description": "the grant"})

		return
	}

	old := f.Get("refresh_token")

	client, ok := s.refresh[old]
	if !ok || client != f.Get("client_id") {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})

		return
	}

	delete(s.refresh, old)

	access, refresh := s.newToken("access"), s.newToken("refresh")
	s.access[access] = true
	s.refresh[refresh] = client

	write(w, http.StatusOK, map[string]any{
		"access_token": access, "refresh_token": refresh, "expires_in": s.lifetime, "token_type": "Bearer",
		"scope": "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
	})
}

func (s *Server) exchangeKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code     string `json:"code"`
		Verifier string `json:"code_verifier"`
		Method   string `json:"code_challenge_method"`
	}

	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" ||
		json.NewDecoder(r.Body).Decode(&in) != nil {
		write(w, http.StatusMethodNotAllowed, map[string]string{"error": "method"})

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.exchanges++

	g, ok := s.codes[in.Code]
	if !ok || g.used || in.Method != "S256" || challenge(in.Verifier) != g.challenge {
		write(w, http.StatusForbidden, map[string]string{"error": "Invalid code or code_verifier"})

		return
	}

	g.used = true
	s.codes[in.Code] = g

	key := s.newToken("sk-or-v1")
	s.keys[key] = true

	write(w, http.StatusOK, map[string]string{"key": key})
}

func (s *Server) bearer(r *http.Request, keys bool) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	s.mu.Lock()
	defer s.mu.Unlock()

	if keys {
		return s.keys[tok]
	}

	return s.access[tok]
}

func (s *Server) planModels(w http.ResponseWriter, r *http.Request) {
	if !s.bearer(r, false) {
		write(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_token"}})

		return
	}

	write(w, http.StatusOK, map[string]any{"models": []map[string]string{
		{"slug": "gpt-fake-1", "display_name": "GPT Fake 1", "visibility": "list"},
		{"slug": "gpt-fake-hidden", "display_name": "Hidden", "visibility": "hide"},
	}})
}

func (s *Server) planResponses(w http.ResponseWriter, r *http.Request) {
	if !s.bearer(r, false) {
		write(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_token"}})

		return
	}

	var in struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
		Store  *bool  `json:"store"`
	}

	if json.NewDecoder(r.Body).Decode(&in) != nil || !in.Stream || in.Store == nil || *in.Store {
		write(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "bad_request"}})

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	for _, ev := range []string{
		`{"type":"response.created","response":{"id":"resp_1"}}`,
		`{"type":"response.output_text.delta","delta":"Hello "}`,
		`{"type":"response.output_text.delta","delta":"from the plan."}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":5}}}`,
	} {
		_, _ = fmt.Fprintf(w, "event: x\ndata: %s\n\n", ev)
	}
}

func (s *Server) routerModels(w http.ResponseWriter, r *http.Request) {
	if !s.bearer(r, true) {
		write(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_key"}})

		return
	}

	write(w, http.StatusOK, map[string]any{"data": []map[string]any{{
		"id": "fake/model-a", "name": "Fake A", "context_length": 8000,
		"top_provider": map[string]any{"max_completion_tokens": 2000},
		"pricing":      map[string]string{"prompt": "0.000001", "completion": "0.000002"},
	}}})
}

func (s *Server) routerChat(w http.ResponseWriter, r *http.Request) {
	if !s.bearer(r, true) {
		write(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_key"}})

		return
	}

	write(w, http.StatusOK, map[string]any{
		"model": "fake/model-a",
		"choices": []map[string]any{
			{"message": map[string]string{"content": "Hello from the router."}, "finish_reason": "stop"},
		},
		"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 4},
	})
}
