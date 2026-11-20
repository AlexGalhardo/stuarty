// Package googletest is a fake Google for the tests of the optional Google sign-in (SDD-014 section
// 12). It is a server on loopback that publishes signing keys, answers the token endpoint with an ID
// token it signs, and can be told to sign a bad one. No test reaches the real Google and no real
// account is involved.
package googletest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/auth/jwt"
)

// KeyID is the kid of the keys of the fake.
const KeyID = "fake-key-1"

// Grant is what the token endpoint answers for one authorization code.
type Grant struct {
	Subject string
	Email   string
	// Nonce is the nonce claim; the test copies it from the authorization address.
	Nonce string
	// Audience overrides aud; empty is the client id of the token request.
	Audience string
	// Issuer overrides iss; empty is the address of the fake.
	Issuer string
	// Unverified makes email_verified false.
	Unverified bool
	// Expired makes exp an hour in the past.
	Expired bool
	// BadSignature signs the token with a key that is not the published one.
	BadSignature bool
}

// Server is the fake Google.
type Server struct {
	// URL is the base address: the issuer, and where /certs and /token are.
	URL string

	srv   *httptest.Server
	key   *rsa.PrivateKey
	other *rsa.PrivateKey

	mu     sync.Mutex
	grants map[string]Grant
	next   int
	hits   map[string]int
	forms  []url.Values
}

// New starts the fake and stops it when the test ends.
func New(tb testing.TB) *Server {
	tb.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatal(err)
	}

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatal(err)
	}

	s := &Server{key: key, other: other, grants: map[string]Grant{}, hits: map[string]int{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/certs", s.certs)
	mux.HandleFunc("/token", s.token)
	s.srv = httptest.NewServer(mux)
	s.URL = s.srv.URL

	tb.Cleanup(s.srv.Close)

	return s
}

// Host is host:port of the fake, the host a connection must list for the gate to let the server reach it.
func (s *Server) Host() string {
	u, _ := url.Parse(s.URL)

	return u.Host
}

// Grant registers an authorization code that the token endpoint will accept once.
func (s *Server) Grant(g Grant) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	code := "code-" + strconv.Itoa(s.next)
	s.grants[code] = g

	return code
}

// Hits is how many requests the path got.
func (s *Server) Hits(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.hits[path]
}

// TotalHits is how many requests the fake got in all.
func (s *Server) TotalHits() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, v := range s.hits {
		n += v
	}

	return n
}

// Forms are the bodies of the token requests the fake got, in order.
func (s *Server) Forms() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]url.Values(nil), s.forms...)
}

func (s *Server) record(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hits[path]++
}

// IDToken signs the ID token that a grant describes for the audience.
func (s *Server) IDToken(g Grant, audience string) string {
	if g.Audience != "" {
		audience = g.Audience
	}

	issuer := g.Issuer
	if issuer == "" {
		issuer = s.URL
	}

	exp := time.Now().Add(time.Hour)
	if g.Expired {
		exp = time.Now().Add(-time.Hour)
	}

	signer := s.key
	if g.BadSignature {
		signer = s.other
	}

	tok, err := jwt.Sign(jwt.RS256(KeyID, signer), jwt.Claims{
		Issuer: issuer, Subject: g.Subject, Audience: []string{audience},
		ExpiresAt: exp, IssuedAt: time.Now().Add(-time.Minute),
		Extra: map[string]any{"nonce": g.Nonce, "email": g.Email, "email_verified": !g.Unverified},
	})
	if err != nil {
		panic(err)
	}

	return tok
}

func (s *Server) certs(w http.ResponseWriter, _ *http.Request) {
	s.record("/certs")

	set, err := jwt.PublicJWKS(jwt.RS256(KeyID, s.key))
	if err != nil {
		http.Error(w, "keys", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, set)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "json", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	s.record("/token")

	if err := r.ParseForm(); err != nil || r.Method != http.MethodPost {
		http.Error(w, "form", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.forms = append(s.forms, r.PostForm)
	g, ok := s.grants[r.PostForm.Get("code")]
	delete(s.grants, r.PostForm.Get("code"))
	s.mu.Unlock()

	if !ok || r.PostForm.Get("code_verifier") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": "fake-access-token-never-stored", "token_type": "Bearer",
		"id_token": s.IDToken(g, r.PostForm.Get("client_id")),
	})
}
