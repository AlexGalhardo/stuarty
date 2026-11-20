package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// CallbackPath is the page the provider sends the browser back to after the consent of a sign-in to an
// AI provider (SDD-016 section 12, SDD-007 section 3.6 item 4).
const CallbackPath = "/auth/callback"

// ErrLocalOnly is the refusal of a sign-in that the build offers on the desktop and on a local server only,
// returned by SignIn.ValidateRedirect on a server in network mode (Sign in with ChatGPT, SDD-016 section 12.3).
var ErrLocalOnly = errors.New("oauth: this sign-in is offered on the desktop and on a local server only")

// RefreshMargin is how long before its expiry an access token is renewed.
const RefreshMargin = 60 * time.Second

// defaultLifetime is the life given to an access token whose answer did not say.
const defaultLifetime = time.Hour

// Envelope is the stored form of a sign-in credential: one vault entry (SDD-014 section 12). It is never
// returned by an operation and never written outside the vault.
type Envelope struct {
	Format   string `json:"format"`
	Version  int    `json:"version"`
	Provider string `json:"provider"`
	// ClientID is the client the provider issued to this host, when it issues one.
	ClientID string `json:"client_id,omitempty"`
	HostID   string `json:"host_id,omitempty"`
	// AccessToken is the bearer token, or the key an OpenRouter sign-in issued.
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	// ExpiresAtMs is when the access token stops working; zero is a credential that does not expire.
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty"`
	Scope       string `json:"scope,omitempty"`
	// Account is who the sign-in is for, as the provider says (untrusted text), for the card.
	Account string `json:"account,omitempty"`
	// NeedsSignIn is set when the grant is no longer good, or the sign-in was never finished.
	NeedsSignIn bool `json:"needs_sign_in,omitempty"`
}

// EnvelopeFormat is the value of Envelope.Format.
const EnvelopeFormat = "stuarty-signin"

// PendingEnvelope is the placeholder of a sign-in that is not finished: it holds no token.
func PendingEnvelope(provider, clientID, hostID string) Envelope {
	return Envelope{
		Format: EnvelopeFormat, Version: 1, Provider: provider, ClientID: clientID, HostID: hostID,
		NeedsSignIn: true,
	}
}

// ParseEnvelope reads the stored form; ok is false for any other secret, such as an API key.
func ParseEnvelope(secret []byte) (Envelope, bool) {
	if len(secret) == 0 || secret[0] != '{' {
		return Envelope{}, false
	}

	var e Envelope
	if json.Unmarshal(secret, &e) != nil || e.Format != EnvelopeFormat || e.Version != 1 || e.Provider == "" {
		return Envelope{}, false
	}

	return e, true
}

// Marshal is the stored form.
func (e Envelope) Marshal() ([]byte, error) {
	e.Format, e.Version = EnvelopeFormat, 1

	return json.Marshal(e) //nolint:gosec // the stored form of a credential: it goes to the vault and nowhere else
}

// Ready says whether the credential can be used now, or renewed.
func (e Envelope) Ready() bool { return !e.NeedsSignIn && e.AccessToken != "" }

// Description says what a sign-in makes: the connection the person gets.
type Description struct {
	// Provider is the id of the provider and the kind of its connections.
	Provider string
	Name     string
	// Label is the name of a new connection when the person gave none.
	Label string
	// Endpoint and Hosts are those of the connection: the hosts include the one of the token endpoint, so
	// the exchange and the refresh pass through the gate of the connection.
	Endpoint string
	Hosts    []string
	Accepts  []string
}

// BeginInput is what a sign-in needs to build the address of the consent screen.
type BeginInput struct {
	State, Challenge, RedirectURI string
	// Existing is the stored form of a connection that signs in again; the zero value for a new one.
	Existing Envelope
}

// CompleteInput is what a sign-in needs to finish.
type CompleteInput struct {
	Code, Verifier, RedirectURI string
	// ClientID is what the redirect carried (the client a provider issued at the registration).
	ClientID string
	Existing Envelope
	Now      time.Time
}

// SignIn is the sign-in of one provider (SDD-016 section 12). The package of the provider implements it;
// the flow of state, verifier and refresh is the same for all, and lives here and in the service.
type SignIn interface {
	Describe() Description
	// ValidateRedirect checks the address the provider will send the browser back to.
	ValidateRedirect(uri string) error
	// AuthorizeURL is the address the person opens to consent.
	AuthorizeURL(in BeginInput) (string, error)
	// Complete trades the code for the credential, through the gated client of the connection.
	Complete(ctx context.Context, d Doer, in CompleteInput) (Envelope, error)
	// Refresh trades a refresh token for a new access token, through the same client.
	Refresh(ctx context.Context, d Doer, e Envelope) (Token, error)
}

// SignIns is the set of the sign-ins of a build, by provider id.
type SignIns struct{ byID map[string]SignIn }

// NewSignIns returns the set of the given sign-ins.
func NewSignIns(list ...SignIn) *SignIns {
	s := &SignIns{byID: map[string]SignIn{}}

	for _, x := range list {
		s.byID[x.Describe().Provider] = x
	}

	return s
}

// Get returns the sign-in of a provider.
func (s *SignIns) Get(provider string) (SignIn, bool) {
	if s == nil {
		return nil, false
	}

	x, ok := s.byID[provider]

	return x, ok
}

// IDs lists the providers that can be signed in to, sorted.
func (s *SignIns) IDs() []string {
	if s == nil {
		return nil
	}

	out := make([]string, 0, len(s.byID))
	for id := range s.byID {
		out = append(out, id)
	}

	slices.Sort(out)

	return out
}

// Refresher renews the sign-in credentials of a manager (connections.Refresher): the access token before it
// expires, under the lock of the connection that the manager holds.
type Refresher struct {
	signIns *SignIns
	now     func() time.Time
}

// NewRefresher returns the refresher over a set of sign-ins; a nil clock is time.Now.
func NewRefresher(signIns *SignIns, now func() time.Time) *Refresher {
	if now == nil {
		now = time.Now
	}

	return &Refresher{signIns: signIns, now: now}
}

// Prepare implements connections.Refresher.
func (r *Refresher) Prepare(ctx context.Context, c connections.Credential) (connections.Prepared, error) {
	env, ok := ParseEnvelope(c.Secret)
	if !ok {
		return connections.Prepared{}, nil
	}

	if !env.Ready() {
		return connections.Prepared{}, connections.ErrNeedsSignIn
	}

	now := r.now()

	if env.ExpiresAtMs == 0 || now.Add(RefreshMargin).Before(time.UnixMilli(env.ExpiresAtMs)) {
		return connections.Prepared{Use: []byte(env.AccessToken), SignedIn: true}, nil
	}

	method, ok := r.signIns.Get(c.Connection.Kind)
	if !ok {
		// This build cannot renew it; the entry is not marked, a build that can will.
		return connections.Prepared{}, connections.ErrNeedsSignIn
	}

	if env.RefreshToken == "" {
		return r.flagged(env, connections.ErrNeedsSignIn)
	}

	tok, err := method.Refresh(ctx, c.Client, env)
	if err != nil {
		return r.failed(env, err)
	}

	env.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		env.RefreshToken = tok.RefreshToken
	}

	if tok.Scope != "" {
		env.Scope = tok.Scope
	}

	life := tok.ExpiresIn
	if life <= 0 {
		life = defaultLifetime
	}

	env.ExpiresAtMs = now.Add(life).UnixMilli()

	stored, merr := env.Marshal()
	if merr != nil {
		return connections.Prepared{}, merr
	}

	return connections.Prepared{Store: stored, Use: []byte(env.AccessToken), SignedIn: true}, nil
}

// failed classifies a failed refresh: the refusal of the grant marks the sign-in, anything that says
// nothing about the grant leaves it as it is.
func (r *Refresher) failed(env Envelope, err error) (connections.Prepared, error) {
	if errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic) || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return connections.Prepared{}, err
	}

	te, ok := errors.AsType[*TokenError](err)
	if !ok || te.Status >= 500 || te.Status == 429 {
		return connections.Prepared{}, connections.ErrSignInUnavailable
	}

	return r.flagged(env, connections.ErrNeedsSignIn)
}

// flagged remembers that the sign-in must be done again, and keeps everything else in the entry.
func (*Refresher) flagged(env Envelope, cause error) (connections.Prepared, error) {
	env.NeedsSignIn = true

	stored, err := env.Marshal()
	if err != nil {
		return connections.Prepared{}, err
	}

	return connections.Prepared{Store: stored}, cause
}
