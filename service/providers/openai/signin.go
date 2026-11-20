package openai

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// The sign-in with ChatGPT of the clients that are open source or run on the machine of the person
// (SDD-016 section 12.1).
const (
	// DefaultEndpoint is the base URL of the API; a plan token is used on it too.
	DefaultEndpoint = "https://api.openai.com/v1"
	// DefaultAuthURL and DefaultTokenURL are the endpoints of the provider.
	DefaultAuthURL  = "https://auth.openai.com/api/accounts/authorize"
	DefaultTokenURL = "https://auth.openai.com/api/accounts/oauth/token" //nolint:gosec // an address, not a credential
	// Resource is what the plan token is for; it is sent in both requests.
	Resource = "https://api.openai.com/v1"
	// RegistrationClient is the client id of a first sign-in on a host. It is never kept.
	RegistrationClient = "dynamic_agent_client"
	// PlanScope is the scope without which the plan cannot be used.
	PlanScope = "chatgpt.tokens.use.direct"
	// AgentName is the name the person sees on the page of the provider, the same on every host.
	AgentName = "Stuarty"
)

// Scopes of the sign-in: the identity and the use of the plan.
var Scopes = []string{
	"openid", "profile", "email", "offline_access", "resource.invoke", PlanScope,
}

var clientRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// SignInConfig points the sign-in at other addresses, for the tests and the fake of the end-to-end suite.
// The zero value, with a HostID, is OpenAI itself.
type SignInConfig struct {
	AuthURL, TokenURL, Endpoint string
	// HostID is the stable identifier of this host (oauth.HostID).
	HostID string
	// Now is the clock; default time.Now.
	Now func() time.Time
}

// SignIn is the sign-in with ChatGPT: an OpenID Connect authorization code flow with PKCE for a public
// client, whose access token is the bearer of the Responses API and whose refresh token rotates.
type SignIn struct{ cfg SignInConfig }

// NewSignIn returns the sign-in; an empty address is the one of OpenAI.
func NewSignIn(cfg SignInConfig) *SignIn {
	if cfg.AuthURL == "" {
		cfg.AuthURL = DefaultAuthURL
	}

	if cfg.TokenURL == "" {
		cfg.TokenURL = DefaultTokenURL
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &SignIn{cfg: cfg}
}

// Describe implements oauth.SignIn: the connection reaches the API and the token endpoint, so the
// refresh passes through its gate.
func (s *SignIn) Describe() oauth.Description {
	hosts := []string{hostOf(s.cfg.Endpoint)}
	if h := hostOf(s.cfg.TokenURL); h != hosts[0] {
		hosts = append(hosts, h)
	}

	return oauth.Description{
		Provider: ID, Name: "ChatGPT", Label: "ChatGPT", Endpoint: s.cfg.Endpoint, Hosts: hosts,
		Accepts: []string{connections.AcceptText},
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

// ValidateRedirect implements oauth.SignIn: exactly http://127.0.0.1:<port>/auth/callback, the only form
// the provider documents (only the port may change, and localhost is not accepted).
func (*SignIn) ValidateRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != oauth.CallbackPath {
		return errors.New("must be http://127.0.0.1:<port>" + oauth.CallbackPath)
	}

	port, perr := strconv.Atoi(u.Port())
	if u.Scheme != "http" || u.Hostname() != "127.0.0.1" || perr != nil || port < 1 || port > 65535 {
		return errors.New("OpenAI accepts only http://127.0.0.1:<port>" + oauth.CallbackPath +
			": open Stuarty at 127.0.0.1 on this machine to sign in with ChatGPT")
	}

	return nil
}

// nonceOf derives the OpenID nonce of a flow from its PKCE challenge, so it needs no storage of its own:
// the verifier that comes back with the code gives the challenge, and the challenge gives the nonce.
func nonceOf(challenge string) string {
	sum := sha256.Sum256([]byte("stuarty-nonce:" + challenge))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL implements oauth.SignIn.
func (s *SignIn) AuthorizeURL(in oauth.BeginInput) (string, error) {
	client := in.Existing.ClientID
	if client == "" {
		client = RegistrationClient
	}

	return oauth.AuthorizeURL(s.cfg.AuthURL, client, in.RedirectURI, in.State, in.Challenge, Scopes, url.Values{
		"resource": {Resource}, "nonce": {nonceOf(in.Challenge)}, "ext_agent_host_id": {s.cfg.HostID},
		"agent_name_hint": {AgentName},
	}), nil
}

// Complete implements oauth.SignIn: the code for the tokens, the identity token checked, the issued client
// id kept.
func (s *SignIn) Complete(ctx context.Context, d oauth.Doer, in oauth.CompleteInput) (oauth.Envelope, error) {
	client, err := issuedClient(in.Existing.ClientID, in.ClientID)
	if err != nil {
		return oauth.Envelope{}, err
	}

	tok, err := oauth.Request(ctx, d, s.cfg.TokenURL, url.Values{
		"grant_type": {"authorization_code"}, "code": {in.Code}, "code_verifier": {in.Verifier},
		"client_id": {client}, "redirect_uri": {in.RedirectURI}, "resource": {Resource},
	})
	if err != nil {
		return oauth.Envelope{}, err
	}

	// A scope that came back without the plan is a sign-in that cannot be used. An answer that lists no
	// scope is left to the first request to say.
	if tok.Scope != "" && !slices.Contains(strings.Fields(tok.Scope), PlanScope) {
		return oauth.Envelope{}, &oauth.RefusedError{Reason: oauth.ReasonNoAccess}
	}

	now := in.Now
	if now.IsZero() {
		now = s.cfg.Now()
	}

	claims, ok := checkIDToken(tok.IDToken, client, nonceOf(oauth.Challenge(in.Verifier)), hostOf(s.cfg.TokenURL), now)
	if !ok {
		return oauth.Envelope{}, &oauth.RefusedError{Reason: oauth.ReasonIdentity}
	}

	life := tok.ExpiresIn
	if life <= 0 {
		life = time.Hour
	}

	return oauth.Envelope{
		Provider: ID, ClientID: client, HostID: s.cfg.HostID, AccessToken: tok.AccessToken,
		RefreshToken: tok.RefreshToken, ExpiresAtMs: now.Add(life).UnixMilli(), Scope: tok.Scope,
		Account: providers.CleanName(claims.Email),
	}, nil
}

// issuedClient decides which client the exchange uses. A first sign-in must come back with the client the
// provider issued; a later one keeps the client of the account, and a different one is refused.
func issuedClient(kept, fromRedirect string) (string, error) {
	switch {
	case kept != "" && fromRedirect != "" && fromRedirect != kept:
		return "", &oauth.RefusedError{Reason: oauth.ReasonClientMismatch}
	case kept != "":
		return kept, nil
	case fromRedirect == "" || fromRedirect == RegistrationClient || !clientRe.MatchString(fromRedirect):
		return "", &oauth.RefusedError{Reason: oauth.ReasonRegistration}
	}

	return fromRedirect, nil
}

// Refresh implements oauth.SignIn: the standard refresh_token grant on the token endpoint, with the client
// that was issued. The rotating refresh token that comes back replaces the old one in the caller.
func (s *SignIn) Refresh(ctx context.Context, d oauth.Doer, e oauth.Envelope) (oauth.Token, error) {
	return oauth.Request(ctx, d, s.cfg.TokenURL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {e.RefreshToken}, "client_id": {e.ClientID},
	})
}

type idClaims struct {
	Issuer   string          `json:"iss"`
	Audience json.RawMessage `json:"aud"`
	Expires  int64           `json:"exp"`
	Nonce    string          `json:"nonce"`
	Email    string          `json:"email"`
}

// checkIDToken reads the claims of an identity token that came straight from the token endpoint over the
// gated connection, and checks the issuer (the host of the token endpoint), the audience (the client),
// the expiry and the nonce. It does not check the signature: the token was not handed over by anyone
// else (OpenID Connect Core section 3.1.3.7), and the address of the keys of the provider is not
// documented (SDD-016 section 12.1).
func checkIDToken(raw, client, nonce, issuerHost string, now time.Time) (idClaims, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return idClaims{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idClaims{}, false
	}

	var c idClaims
	if json.Unmarshal(payload, &c) != nil {
		return idClaims{}, false
	}

	iss, err := url.Parse(c.Issuer)
	if err != nil || iss.Hostname() == "" || iss.Hostname() != issuerHost || c.Nonce != nonce ||
		c.Expires <= now.Unix() || !audienceHas(c.Audience, client) {
		return idClaims{}, false
	}

	return c, true
}

// audienceHas reads aud, which is a string or a list of strings.
func audienceHas(raw json.RawMessage, client string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == client
	}

	var many []string

	return json.Unmarshal(raw, &many) == nil && slices.Contains(many, client)
}
