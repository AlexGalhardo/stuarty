// Package oauth is the generic authorization-code flow with PKCE that a connector signs in with
// (SDD-020 section 6; epics 12 and 24 reuse it). It holds no network code of its own: it builds the
// authorization address, keeps the pending flows in memory and trades a code or a refresh token for
// tokens through the gated client it is given, so the token host must be a host of the connection.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Limits of the pending flows.
const (
	// TTL is how long a flow may wait for the person to come back from the consent screen.
	TTL = 10 * time.Minute
	// MaxPending is the most flows that wait at once; the oldest is dropped first.
	MaxPending = 20
	// maxTokenBody is the most of a token answer that is read.
	maxTokenBody = 64 << 10
)

// Errors of the flow.
var (
	// ErrUnknownState is a state that was never issued, was used already, or was dropped.
	ErrUnknownState = errors.New("oauth: unknown or used state")
	// ErrExpired is a state whose flow waited longer than TTL.
	ErrExpired = errors.New("oauth: the sign-in took too long")
	// ErrWrongActor is a flow completed by another caller than the one that began it.
	ErrWrongActor = errors.New("oauth: the sign-in belongs to another session")
)

// Pending is a flow that has begun and waits for the redirect.
type Pending struct {
	// State is the random value that comes back on the redirect; it is the key of the flow.
	State string
	// Verifier is the PKCE verifier; it never leaves the process.
	Verifier     string
	ConnectionID string
	Network      string
	RedirectURI  string
	// Actor is who began the flow and Browser whether it was a browser session.
	Actor   string
	Browser bool
	// Created is true when the connection was made by this very flow, so a sign-in that fails does not leave it
	// behind with no account (VV-323).
	Created bool
	Expires time.Time
}

// Flows keeps the pending flows.
type Flows struct {
	now func() time.Time

	mu      sync.Mutex
	pending map[string]Pending
	order   []string
}

// NewFlows returns an empty set; a nil clock is time.Now.
func NewFlows(now func() time.Time) *Flows {
	if now == nil {
		now = time.Now
	}

	return &Flows{now: now, pending: map[string]Pending{}}
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}

// Challenge is the S256 PKCE challenge of a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Begin makes the state and the verifier of a flow, keeps the flow and returns the state and the
// PKCE challenge for the authorization address.
func (f *Flows) Begin(p Pending) (state, challenge string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sweepLocked()

	p.State = random(32)
	p.Verifier = random(48)
	p.Expires = f.now().Add(TTL)

	for len(f.order) >= MaxPending {
		delete(f.pending, f.order[0])
		f.order = f.order[1:]
	}

	f.pending[p.State] = p
	f.order = append(f.order, p.State)

	return p.State, Challenge(p.Verifier)
}

// PendingFor says whether a flow that has not expired is still open on the connection.
func (f *Flows) PendingFor(connectionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sweepLocked()

	for _, p := range f.pending {
		if p.ConnectionID == connectionID {
			return true
		}
	}

	return false
}

func (f *Flows) sweepLocked() {
	now := f.now()
	kept := f.order[:0]

	for _, s := range f.order {
		if p, ok := f.pending[s]; ok && now.After(p.Expires) {
			delete(f.pending, s)

			continue
		}

		kept = append(kept, s)
	}

	f.order = kept
}

// Take returns the flow of a state and removes it, whatever happens next: a state is used once. The
// caller must be the one that began the flow (ErrWrongActor, and the flow is gone all the same, so a
// guess cannot be retried).
func (f *Flows) Take(state, actor string, browser bool) (Pending, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	p, ok := f.pending[state]
	if !ok {
		return Pending{}, ErrUnknownState
	}

	delete(f.pending, state)

	f.order = removeString(f.order, state)

	if f.now().After(p.Expires) {
		return Pending{}, ErrExpired
	}

	// A flow begun in a browser is completed by the same actor. One begun elsewhere (the CLI) is
	// completed by a browser session: the person who saw the consent screen.
	if (p.Browser && p.Actor != actor) || !browser {
		return Pending{}, ErrWrongActor
	}

	return p, nil
}

func removeString(list []string, s string) []string {
	for i, x := range list {
		if x == s {
			return append(list[:i], list[i+1:]...)
		}
	}

	return list
}

// AuthorizeURL builds the address the person opens to consent.
func AuthorizeURL(authURL, clientID, redirectURI, state, challenge string, scopes []string, extra url.Values) string {
	q := url.Values{}

	maps.Copy(q, extra)

	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	sep := "?"
	if strings.Contains(authURL, "?") {
		sep = "&"
	}

	return authURL + sep + q.Encode()
}

// Doer is the gated client of a connection.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Token is what a token endpoint answers. It is held in memory for the length of one call.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	Scope        string
	// IDToken is the OpenID Connect ID token when the answer carries one (epics 12 and 24), empty
	// otherwise; it is not verified here.
	IDToken string
}

// TokenError is a refusal of the token endpoint. Code is the OAuth error code (invalid_grant...), which
// passed codePattern; the description of the endpoint is not kept.
type TokenError struct {
	Status int
	Code   string
}

func (e *TokenError) Error() string {
	if e.Code == "" {
		return "oauth: the token endpoint refused the request"
	}

	return "oauth: the token endpoint refused the request: " + e.Code
}

var codePattern = regexp.MustCompile(`^[a-z_]{1,40}$`)

// Request posts a form to a token endpoint through the gated client and reads the answer.
func Request(ctx context.Context, c Doer, tokenURL string, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := c.Do(req)
	if err != nil {
		return Token{}, err
	}

	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxTokenBody))
	if err != nil {
		return Token{}, err
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
	}

	_ = json.Unmarshal(body, &out)

	if res.StatusCode != http.StatusOK || out.AccessToken == "" {
		code := out.Error
		if !codePattern.MatchString(code) {
			code = ""
		}

		return Token{}, &TokenError{Status: res.StatusCode, Code: code}
	}

	return Token{
		AccessToken: out.AccessToken, RefreshToken: out.RefreshToken,
		ExpiresIn: lifetime(out.ExpiresIn), Scope: out.Scope, IDToken: out.IDToken,
	}, nil
}

// lifetime is the expires_in of a token answer as a duration, checked as a number of seconds first: a value
// that is not positive or does not fit a Duration is no lifetime (0, which the caller replaces with its default),
// never one that overflowed into a negative or a tiny wait (found by FuzzTokenResponse).
func lifetime(secs int64) time.Duration {
	if secs <= 0 || secs > int64(math.MaxInt64/time.Second) {
		return 0
	}

	return time.Duration(secs) * time.Second
}

// Exchange trades an authorization code (with the PKCE verifier) for tokens.
func Exchange(
	ctx context.Context, c Doer, tokenURL, clientID, clientSecret, code, verifier, redirectURI string,
) (Token, error) {
	return Request(ctx, c, tokenURL, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier},
		"client_id": {clientID}, "client_secret": {clientSecret}, "redirect_uri": {redirectURI},
	})
}

// Refresh trades a refresh token for an access token.
func Refresh(ctx context.Context, c Doer, tokenURL, clientID, clientSecret, refreshToken string) (Token, error) {
	return Request(ctx, c, tokenURL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {refreshToken},
		"client_id": {clientID}, "client_secret": {clientSecret},
	})
}
