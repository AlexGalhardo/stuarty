package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// signInTimeout is how long the exchange of a code may take.
const signInTimeout = 30 * time.Second

// The states of a sign-in connection (SDD-016 section 12.4).
const (
	SignInReady        = "ready"
	SignInNeedsSignIn  = "needs_sign_in"
	signInProviderCode = "needs_sign_in"
)

// ProviderSignInBeginRequest starts the sign-in of an AI provider in the browser (SDD-016 section 12).
type ProviderSignInBeginRequest struct {
	// Provider is the id of the provider, for example openai or openrouter.
	Provider string `json:"provider"`
	// Label names a new connection; empty is the default name of the provider.
	Label string `json:"label,omitempty"`
	// ConnectionID signs in again an existing connection that was made by a sign-in.
	ConnectionID string `json:"connection_id,omitempty"`
	// RedirectURI is the address of the sign-in page of this server, where the provider sends the browser.
	RedirectURI string `json:"redirect_uri"`
}

// ProviderSignInBeginResult has the address the person opens to consent. It holds no secret.
type ProviderSignInBeginResult struct {
	ConnectionID string `json:"connection_id"`
	AuthorizeURL string `json:"authorize_url"`
}

// ProviderSignInCompleteRequest is what the provider sent back to the redirect address.
type ProviderSignInCompleteRequest struct {
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
	// Error is the error the provider reported instead of a code (access_denied...).
	Error string `json:"error,omitempty"`
	// ClientID is the client the provider issued at the first sign-in of a host (OpenAI).
	ClientID string `json:"client_id,omitempty"`
}

// ProviderSignIn is a connection made by a sign-in, with who it is for. It never carries a token.
type ProviderSignIn struct {
	ConnectionID string `json:"connection_id"`
	Provider     string `json:"provider"`
	Label        string `json:"label"             untrusted:"true"`
	Account      string `json:"account,omitempty" untrusted:"true"`
	// State is ready, or needs_sign_in when the provider no longer accepts the sign-in or it was not finished.
	State string `json:"state"`
}

// ProviderSignInListRequest has no member.
type ProviderSignInListRequest struct{}

// ProviderSignInListResult is the sign-in connections, oldest first.
type ProviderSignInListResult struct {
	SignIns []ProviderSignIn `json:"sign_ins"`
}

func (s *svc) signInMethod(provider string) (oauth.SignIn, error) {
	ids := s.signInSet.IDs()
	if len(ids) == 0 {
		return nil, newError(command.CodeNotFound, "no sign-in to an AI provider is built into this server.")
	}

	m, ok := s.signInSet.Get(provider)
	if !ok {
		return nil, validationAt("provider", "the provider must be one of %s.", strings.Join(ids, ", "))
	}

	return m, nil
}

// ProviderSignInBegin makes the connection (or takes an existing one) and answers the address of the
// consent screen. Only a person in the browser may begin a sign-in.
func (s *svc) ProviderSignInBegin(
	ctx context.Context,
	req ProviderSignInBeginRequest,
) (*ProviderSignInBeginResult, error) {
	c, err := s.person(ctx, "ProviderSignInBegin")
	if err != nil {
		return nil, err
	}

	method, err := s.signInMethod(req.Provider)
	if err != nil {
		return nil, err
	}

	if rerr := method.ValidateRedirect(req.RedirectURI); rerr != nil {
		return nil, validationAt("redirect_uri", "%s.", rerr.Error()).
			withHint("open Stuarty at the address the provider accepts and sign in from there")
	}

	desc := method.Describe()
	connID := strings.TrimSpace(req.ConnectionID)

	var existing oauth.Envelope

	if connID == "" {
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = desc.Label
		}

		spec, ferr := connections.Normalize(connections.Spec{
			Kind: desc.Provider, Label: label, Endpoint: desc.Endpoint, Hosts: desc.Hosts, Accepts: desc.Accepts,
		}, s.conns.Kinds())
		if ferr != nil {
			return nil, connectionError(ferr)
		}

		placeholder, merr := oauth.PendingEnvelope(desc.Provider, "", "").Marshal()
		if merr != nil {
			return nil, asError(merr)
		}

		created, ierr := s.insertConnection(ctx, c, spec, placeholder)
		if ierr != nil {
			return nil, ierr
		}

		connID = created.ID
	} else {
		var gerr error

		if existing, gerr = s.signInEnvelope(ctx, connID, desc.Provider); gerr != nil {
			return nil, gerr
		}
	}

	state, challenge := s.signFlows.Begin(oauth.Pending{
		ConnectionID: connID, Network: desc.Provider, RedirectURI: req.RedirectURI, Actor: c.Actor, Browser: c.Browser,
	})

	authorize, aerr := method.AuthorizeURL(oauth.BeginInput{
		State: state, Challenge: challenge, RedirectURI: req.RedirectURI, Existing: existing,
	})
	if aerr != nil {
		return nil, asError(aerr)
	}

	if werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), c.Actor, "provider.signin.begin", "", connID)
	}); werr != nil {
		return nil, asError(werr)
	}

	return &ProviderSignInBeginResult{ConnectionID: connID, AuthorizeURL: authorize}, nil
}

// signInEnvelope reads the stored form of a connection that must be a sign-in of the provider.
func (s *svc) signInEnvelope(ctx context.Context, connID, provider string) (oauth.Envelope, error) {
	conn, err := s.conns.Get(ctx, connID)
	if err != nil {
		return oauth.Envelope{}, connectionError(err)
	}

	if conn.Kind != provider {
		return oauth.Envelope{}, validationAt("connection_id", "the connection is not a %s connection.", provider)
	}

	var (
		env oauth.Envelope
		ok  bool
	)

	werr := s.conns.WithStored(ctx, connID, signInTimeout, func(cred connections.Credential) error {
		env, ok = oauth.ParseEnvelope(cred.Secret)

		return nil
	})
	if werr != nil {
		return oauth.Envelope{}, connectionError(werr)
	}

	if !ok {
		return oauth.Envelope{}, validationAt("connection_id", "the connection holds an API key, not a sign-in.").
			withHint("make a new connection with the sign-in, or remove this one and sign in")
	}

	return env, nil
}

// ProviderSignInComplete is the redirect of the sign-in: it trades the code for the credential through the
// gated client of the connection and keeps it in the vault. The state is taken first and always.
func (s *svc) ProviderSignInComplete(
	ctx context.Context,
	req ProviderSignInCompleteRequest,
) (*ProviderSignIn, error) {
	c, err := s.person(ctx, "ProviderSignInComplete")
	if err != nil {
		return nil, err
	}

	p, terr := s.signFlows.Take(req.State, c.Actor, c.Browser)
	if terr != nil {
		return nil, validationAt("state", "the sign-in is unknown, was used already, or took too long.").
			withHint("start the sign-in again from the Connections page")
	}

	method, err := s.signInMethod(p.Network)
	if err != nil {
		return nil, err
	}

	if req.Error != "" || req.Code == "" {
		return nil, validationAt("code", "the provider did not give a code: the sign-in was refused or cancelled.").
			withHint("start the sign-in again and accept the access")
	}

	var env oauth.Envelope

	werr := s.conns.WithStored(ctx, p.ConnectionID, signInTimeout, func(cred connections.Credential) error {
		existing, _ := oauth.ParseEnvelope(cred.Secret)

		var xerr error

		env, xerr = method.Complete(ctx, cred.Client, oauth.CompleteInput{
			Code: req.Code, Verifier: p.Verifier, RedirectURI: p.RedirectURI, ClientID: strings.TrimSpace(req.ClientID),
			Existing: existing, Now: s.now(),
		})

		return xerr
	})
	if werr != nil {
		return nil, signInError(werr)
	}

	raw, merr := env.Marshal()
	if merr != nil {
		return nil, asError(merr)
	}

	defer clear(raw)

	if perr := s.conns.PutSecret(p.ConnectionID, raw); perr != nil {
		return nil, connectionError(perr)
	}

	// A catalogue read with the old credential is the catalogue of another account.
	s.providers.Forget(p.ConnectionID)

	if aerr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), c.Actor, "provider.signin", "", p.ConnectionID)
	}); aerr != nil {
		return nil, asError(aerr)
	}

	row, gerr := s.conns.Get(ctx, p.ConnectionID)
	if gerr != nil {
		return nil, connectionError(gerr)
	}

	return &ProviderSignIn{
		ConnectionID: row.ID, Provider: p.Network, Label: row.Label, Account: env.Account, State: SignInReady,
	}, nil
}

// signInError turns what a failed exchange can be into an error the person can read, written here from the
// kind of failure and never from the answer of the provider.
func signInError(err error) error {
	if re, ok := errors.AsType[*oauth.RefusedError](err); ok {
		sentence := map[string]string{
			oauth.ReasonRegistration:   "the provider did not register Stuarty for this sign-in.",
			oauth.ReasonClientMismatch: "the provider answered for another client than the one of this account.",
			oauth.ReasonIdentity:       "the provider's answer is not for this sign-in.",
			oauth.ReasonNoAccess:       "the provider did not grant the access Stuarty asked for.",
		}[re.Reason]

		return newError(CodeProvider, "%s", sentence).withDetail("kind", providers.KindAuth).
			withDetail("code", re.Reason).withHint("start the sign-in again from the Connections page")
	}

	if te, ok := errors.AsType[*oauth.TokenError](err); ok {
		kind := providers.KindAuth
		if te.Status >= 500 || te.Status == 429 {
			kind = providers.KindUnavailable
		}

		return providerError(&providers.Error{Kind: kind, Status: te.Status, Code: providers.SafeCode(te.Code)})
	}

	return providerError(err)
}

// ProviderSignInList lists the connections made by a sign-in, with who each is for and whether it works.
func (s *svc) ProviderSignInList(ctx context.Context, _ ProviderSignInListRequest) (*ProviderSignInListResult, error) {
	if _, err := s.connectionCaller(ctx, "ProviderSignInList"); err != nil {
		return nil, err
	}

	list, lerr := s.conns.List(ctx)
	if lerr != nil {
		return nil, connectionError(lerr)
	}

	out := &ProviderSignInListResult{SignIns: []ProviderSignIn{}}

	for i := range list {
		var (
			env oauth.Envelope
			ok  bool
		)

		// A connection whose credential cannot be read is not listed: it may be a key, and nothing says it
		// is not. The reason is for the Connections page to show from ConnectionGet.
		werr := s.conns.WithStored(ctx, list[i].ID, signInTimeout, func(cred connections.Credential) error {
			env, ok = oauth.ParseEnvelope(cred.Secret)

			return nil
		})
		if werr != nil || !ok {
			continue
		}

		state := SignInReady
		if !env.Ready() {
			state = SignInNeedsSignIn
		}

		out.SignIns = append(out.SignIns, ProviderSignIn{
			ConnectionID: list[i].ID, Provider: list[i].Kind, Label: list[i].Label, Account: env.Account, State: state,
		})
	}

	return out, nil
}

// isSignIn says whether the credential of a connection is the stored form of a sign-in.
func (s *svc) isSignIn(ctx context.Context, connID string) bool {
	var ok bool

	_ = s.conns.WithStored(ctx, connID, signInTimeout, func(cred connections.Credential) error {
		_, ok = oauth.ParseEnvelope(cred.Secret)

		return nil
	})

	return ok
}
