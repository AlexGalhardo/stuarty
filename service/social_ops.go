package service

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The account states of SDD-020 section 7.6.
const (
	AccountReady       = "ready"
	AccountNeedsSignIn = "needs_sign_in"
	// AccountNeedsAccount is a sign-in that reached several accounts and none is chosen yet (SDD-020
	// section 17).
	AccountNeedsAccount = "needs_account"
	maxClientFieldBytes = 2048
)

// SocialNetworkListRequest has no member.
type SocialNetworkListRequest struct{}

// SocialNetworkListResult is the networks of the build.
type SocialNetworkListResult struct {
	Networks []social.Description `json:"networks"`
	// RedirectPath is the path of the sign-in redirect on this server, to be added to the origin of the
	// app and pasted in the developer project of the network.
	RedirectPath string `json:"redirect_path"`
}

// SocialConnectBeginRequest starts the sign-in of a network with the user's own app (SDD-020 section 6).
type SocialConnectBeginRequest struct {
	Network string `json:"network"`
	// Label names the connection; empty is the name of the network.
	Label string `json:"label,omitempty"`
	// ClientID and ClientSecret are those of the developer project the person created; the secret goes into
	// the vault and is in no result. Leave both out with ConnectionID to sign in again.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	// ConnectionID signs in again an existing connection of the network.
	ConnectionID string `json:"connection_id,omitempty"`
	// RedirectURI is the origin of the app plus the redirect path; it must be the address the person
	// authorised in the developer project.
	RedirectURI string `json:"redirect_uri"`
}

// SocialConnectBeginResult has the address the person opens to consent. It holds no secret.
type SocialConnectBeginResult struct {
	ConnectionID string `json:"connection_id"`
	AuthorizeURL string `json:"authorize_url"`
}

// SocialConnectCompleteRequest is what the network sent back to the redirect address.
type SocialConnectCompleteRequest struct {
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
	// Error is the error the network reported instead of a code (access_denied...).
	Error string `json:"error,omitempty"`
}

// SocialAccount is a connection of a social network with the account it signs in as. It never carries a
// token.
type SocialAccount struct {
	ConnectionID string `json:"connection_id"`
	Network      string `json:"network"`
	Label        string `json:"label"                   untrusted:"true"`
	AccountID    string `json:"account_id,omitempty"`
	AccountTitle string `json:"account_title,omitempty" untrusted:"true"`
	// State is ready, needs_sign_in when no refresh token is kept, or needs_account when the sign-in reached
	// several accounts and a person has not chosen one.
	State string `json:"state"`
	// Choices are the accounts a person chooses from when State is needs_account; never a token.
	Choices []SocialChoice `json:"choices,omitempty"`
	// Price is what a post to the network costs, for a network that charges for one (X): the amounts the
	// connection is estimated at, with the page they were read from and the date they were read or edited.
	Price *publish.Price `json:"price,omitempty"`
}

// SocialPriceSetRequest corrects the price of a post on a network that charges for one, when the network
// changes it (SDD-020 section 39).
type SocialPriceSetRequest struct {
	ConnectionID string `json:"connection_id"`
	// PostMicros is the price of a post and LinkMicros the price of a post with a link, in millionths of the
	// currency of the network.
	PostMicros int64 `json:"post_micros"`
	LinkMicros int64 `json:"link_micros"`
}

// SocialChoice is an account a sign-in reached.
type SocialChoice struct {
	ID    string `json:"id"`
	Title string `json:"title" untrusted:"true"`
}

// SocialAccountSelectRequest chooses the account of a connection whose sign-in reached several.
type SocialAccountSelectRequest struct {
	ConnectionID string `json:"connection_id"`
	// AccountID is the id of one of the choices of the connection.
	AccountID string `json:"account_id"`
}

// SocialAccountListRequest has no member.
type SocialAccountListRequest struct{}

// SocialAccountListResult is the social connections, oldest first.
type SocialAccountListResult struct {
	Accounts []SocialAccount `json:"accounts"`
}

func (s *svc) socialRegistry() (*social.Registry, error) {
	if s.social == nil {
		return nil, newError(command.CodeNotFound, "no social network is built into this server.")
	}

	return s.social, nil
}

// SocialNetworkList describes the networks the build can publish to.
func (s *svc) SocialNetworkList(ctx context.Context, _ SocialNetworkListRequest) (*SocialNetworkListResult, error) {
	if _, err := s.connectionCaller(ctx, "SocialNetworkList"); err != nil {
		return nil, err
	}

	if s.social == nil {
		return &SocialNetworkListResult{Networks: []social.Description{}, RedirectPath: social.RedirectPath}, nil
	}

	return &SocialNetworkListResult{Networks: s.social.List(), RedirectPath: social.RedirectPath}, nil
}

// validRedirect accepts the redirect address of the sign-in: https, or http for a loopback host, with the
// path of social.RedirectPath and nothing else after it.
func validRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != social.RedirectPath {
		return errors.New("must be the address of this app followed by " + social.RedirectPath)
	}

	switch u.Scheme {
	case "https":
	case "http":
		if h := strings.ToLower(u.Hostname()); h != "localhost" && h != "127.0.0.1" && h != "::1" {
			return errors.New("http is only for localhost; use https")
		}
	default:
		return errors.New("the scheme must be https")
	}

	return nil
}

func validationAt(path, format string, args ...any) *Error {
	return newError(command.CodeValidation, format, args...).withPath(path)
}

// SocialConnectBegin makes the connection (or takes an existing one) with the client of the person's own
// app in the vault, and answers the address of the consent screen.
func (s *svc) SocialConnectBegin(
	ctx context.Context,
	req SocialConnectBeginRequest,
) (*SocialConnectBeginResult, error) {
	c, err := s.connectionCaller(ctx, "SocialConnectBegin")
	if err != nil {
		return nil, err
	}

	reg, err := s.socialRegistry()
	if err != nil {
		return nil, err
	}

	conn, cerr := reg.Get(req.Network)
	if cerr != nil {
		return nil, validationAt("network", "the network must be one of %s.", strings.Join(reg.Networks(), ", "))
	}

	if rerr := validRedirect(req.RedirectURI); rerr != nil {
		return nil, validationAt("redirect_uri", "%s.", rerr.Error())
	}

	if len(req.ClientID) > maxClientFieldBytes || len(req.ClientSecret) > maxClientFieldBytes {
		return nil, validationAt(
			"client_secret",
			"the client id and the client secret are at most %d bytes.",
			maxClientFieldBytes,
		)
	}

	desc := conn.Describe()
	connID, clientID := req.ConnectionID, strings.TrimSpace(req.ClientID)
	madeHere := false

	if connID == "" {
		secret, berr := conn.Begin(req.ClientID, req.ClientSecret)
		if berr != nil {
			return nil, validationAt(
				"client_id",
				"the client id and the client secret of your %s project are required.",
				desc.Name,
			)
		}

		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = desc.Name
		}

		spec, ferr := connections.Normalize(connections.Spec{
			Kind: desc.Network, Label: label, Endpoint: desc.Endpoint, Hosts: desc.Hosts,
			Accepts: []string{connections.AcceptVideo, connections.AcceptImage, connections.AcceptMetadata},
		}, s.conns.Kinds())
		if ferr != nil {
			return nil, connectionError(ferr)
		}

		created, ierr := s.insertConnection(ctx, c, spec, secret)
		if ierr != nil {
			return nil, ierr
		}

		connID, madeHere = created.ID, true
	} else {
		existing, gerr := s.conns.Get(ctx, connID)
		if gerr != nil {
			return nil, connectionError(gerr)
		}

		if existing.Kind != desc.Network {
			return nil, validationAt("connection_id", "the connection is not a %s connection.", desc.Name)
		}

		werr := s.conns.With(ctx, connID, func(cred connections.Credential) error {
			id, ok := conn.ClientID(cred.Secret)
			if !ok {
				return validationAt("connection_id", "the connection holds no client of your %s project.", desc.Name)
			}

			clientID = id

			return nil
		})
		if werr != nil {
			return nil, connectionError(werr)
		}
	}

	state, challenge := s.flows.Begin(oauth.Pending{
		ConnectionID: connID, Network: desc.Network, RedirectURI: req.RedirectURI, Actor: c.Actor, Browser: c.Browser,
		Created: madeHere,
	})

	// A network whose consent address is not the generic one (TikTok's, SDD-020 section 34) builds its own.
	if a, ok := conn.(social.Authorizer); ok {
		return &SocialConnectBeginResult{
			ConnectionID: connID,
			AuthorizeURL: a.AuthorizeURL(clientID, req.RedirectURI, state, challenge),
		}, nil
	}

	auth := conn.AuthConfig()

	return &SocialConnectBeginResult{
		ConnectionID: connID,
		AuthorizeURL: oauth.AuthorizeURL(
			auth.AuthURL,
			clientID,
			req.RedirectURI,
			state,
			challenge,
			auth.Scopes,
			auth.Extra,
		),
	}, nil
}

// SocialConnectComplete is the redirect of the sign-in: it trades the code for the tokens through the
// gated client of the connection, keeps the refresh token in the vault and reads who the account is.
func (s *svc) SocialConnectComplete(ctx context.Context, req SocialConnectCompleteRequest) (*SocialAccount, error) {
	c, err := s.connectionCaller(ctx, "SocialConnectComplete")
	if err != nil {
		return nil, err
	}

	if !c.Browser {
		return nil, privilegedError()
	}

	reg, err := s.socialRegistry()
	if err != nil {
		return nil, err
	}

	// The state is taken first and always: a refused sign-in, a wrong actor and a replay all spend it.
	p, terr := s.flows.Take(req.State, c.Actor, c.Browser)
	if terr != nil {
		return nil, validationAt("state", "the sign-in is unknown, was used already, or took too long.").
			withHint("start the sign-in again from the Connections page")
	}

	if req.Error != "" || req.Code == "" {
		s.dropUnfinished(ctx, c, reg, p)

		return nil, validationAt("code", "the network did not give a code: the sign-in was refused or cancelled.").
			withHint("start the sign-in again and accept the permissions")
	}

	conn, cerr := reg.Get(p.Network)
	if cerr != nil {
		return nil, validationAt("network", "the network is not built into this server.")
	}

	var newSecret []byte

	werr := s.conns.With(ctx, p.ConnectionID, func(cred connections.Credential) error {
		var xerr error

		newSecret, xerr = conn.Exchange(ctx, cred, req.Code, p.Verifier, p.RedirectURI)

		return xerr
	})
	if werr != nil {
		s.dropUnfinished(ctx, c, reg, p)

		return nil, s.socialError(werr)
	}

	defer clear(newSecret)

	if perr := s.conns.PutSecret(p.ConnectionID, newSecret); perr != nil {
		return nil, connectionError(perr)
	}

	now := s.nowMs()

	if aerr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "social.connect", "", p.ConnectionID)
	}); aerr != nil {
		return nil, asError(aerr)
	}

	row, gerr := s.conns.Get(ctx, p.ConnectionID)
	if gerr != nil {
		return nil, connectionError(gerr)
	}

	out := SocialAccount{ConnectionID: row.ID, Network: p.Network, Label: row.Label}
	describeAccount(conn, newSecret, &out)

	return &out, nil
}

// dropUnfinished removes the connection a failed sign-in made, so a person who tries again does not meet a
// "Fake network" with no account for every attempt (VV-323). A connection that was there before the sign-in
// began, and so may hold an account, is never touched. The failure the person reads is the sign-in's, so an
// error here is not added to it.
//
// The connection is also kept when another sign-in began on it ("Sign in again" while this one was open) and is
// still pending, or has finished: a sign-in of its own, or the accounts to choose from, are in its secret now.
func (s *svc) dropUnfinished(ctx context.Context, c Caller, reg *social.Registry, p oauth.Pending) {
	if !p.Created || s.flows.PendingFor(p.ConnectionID) {
		return
	}

	conn, cerr := reg.Get(p.Network)
	if cerr != nil {
		return
	}

	var state SocialAccount

	if s.conns.With(ctx, p.ConnectionID, func(cred connections.Credential) error {
		describeAccount(conn, cred.Secret, &state)

		return nil
	}) != nil || state.State != AccountNeedsSignIn {
		return
	}

	row, err := s.store.Connections().Get(ctx, p.ConnectionID)
	if err != nil {
		return
	}

	if s.conns.DeleteSecret(row.SecretRef) != nil {
		return
	}

	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Connections().Delete(ctx, row.ID); err != nil {
			return err
		}

		return appendAudit(ctx, tx, s.nowMs(), c.Actor, "connection.remove", "", row.ID)
	})
}

// describeAccount fills the account and the state of a connection from its secret: ready with the account it
// signs in as, needs_account with the choices when the sign-in reached several and none is chosen, and
// needs_sign_in otherwise. It reads no token out.
func describeAccount(conn social.Connector, secret []byte, out *SocialAccount) {
	if pr, ok := social.EffectivePrice(conn, secret); ok {
		out.Price = &pr
	}

	if a, ok := conn.SignedIn(secret); ok {
		out.AccountID, out.AccountTitle, out.State, out.Choices = a.ID, a.Title, AccountReady, nil

		return
	}

	out.State = AccountNeedsSignIn

	if ch, ok := conn.(social.Chooser); ok {
		for _, c := range ch.Choices(secret) {
			out.Choices = append(out.Choices, SocialChoice{ID: c.ID, Title: c.Title})
		}

		if len(out.Choices) > 0 {
			out.State = AccountNeedsAccount
		}
	}
}

// SocialAccountSelect chooses which of the accounts a sign-in reached the connection publishes to. It is for
// a person in the browser: where a post goes is not an agent's to change (SDD-020 section 17, T-56). The
// secret is written again with the chosen account's token alone.
func (s *svc) SocialAccountSelect(ctx context.Context, req SocialAccountSelectRequest) (*SocialAccount, error) {
	c, err := s.connectionCaller(ctx, "SocialAccountSelect")
	if err != nil {
		return nil, err
	}

	if !c.Browser {
		return nil, privilegedError()
	}

	reg, err := s.socialRegistry()
	if err != nil {
		return nil, err
	}

	row, gerr := s.conns.Get(ctx, req.ConnectionID)
	if gerr != nil {
		return nil, connectionError(gerr)
	}

	conn, cerr := reg.Get(row.Kind)
	if cerr != nil {
		return nil, validationAt("connection_id", "the connection is not one of a social network.")
	}

	chooser, ok := conn.(social.Chooser)
	if !ok {
		return nil, validationAt("connection_id", "the connection signs in as one account: there is none to choose.")
	}

	var (
		newSecret []byte
		refused   error
	)

	werr := s.conns.With(ctx, row.ID, func(cred connections.Credential) error {
		newSecret, refused = chooser.Choose(cred.Secret, req.AccountID)

		return nil
	})
	if werr != nil {
		return nil, connectionError(werr)
	}

	if refused != nil {
		return nil, validationAt("account_id", "that account is not one the sign-in reached.").
			withHint("choose one of the accounts listed for the connection, or sign in again")
	}

	defer clear(newSecret)

	if perr := s.conns.PutSecret(row.ID, newSecret); perr != nil {
		return nil, connectionError(perr)
	}

	now := s.nowMs()

	if aerr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "social.select", "", row.ID)
	}); aerr != nil {
		return nil, asError(aerr)
	}

	out := SocialAccount{ConnectionID: row.ID, Network: row.Kind, Label: row.Label}
	describeAccount(conn, newSecret, &out)

	return &out, nil
}

// SocialPriceSet corrects the price of a post on a network that charges for one (X), when the network changes
// it (SDD-020 section 39). It is for a person in the browser: what a post costs is what the approval shows and
// the ceilings count, so an agent does not change it (T-69). The amounts are kept with the date of the edit in
// the secret document of the connection, and change the estimate of a request made after them, never one
// already made.
func (s *svc) SocialPriceSet(ctx context.Context, req SocialPriceSetRequest) (*SocialAccount, error) {
	c, err := s.connectionCaller(ctx, "SocialPriceSet")
	if err != nil {
		return nil, err
	}

	if !c.Browser {
		return nil, privilegedError()
	}

	reg, err := s.socialRegistry()
	if err != nil {
		return nil, err
	}

	row, gerr := s.conns.Get(ctx, req.ConnectionID)
	if gerr != nil {
		return nil, connectionError(gerr)
	}

	conn, cerr := reg.Get(row.Kind)
	if cerr != nil {
		return nil, validationAt("connection_id", "the connection is not one of a social network.")
	}

	priced, ok := conn.(social.Priced)
	if !ok {
		return nil, validationAt("connection_id", "the network of the connection has no price to edit.")
	}

	price := publish.Price{
		PostMicros: req.PostMicros, LinkMicros: req.LinkMicros, Edited: s.now().UTC().Format("2006-01-02"),
	}
	if verr := price.Valid(); verr != nil {
		return nil, validationAt("post_micros", "a price is a whole number of millionths, above 0 and at most %d.",
			publish.MaxPriceMicros).withHint("for $0.015 write 15000, and for $0.20 write 200000")
	}

	var (
		newSecret []byte
		refused   error
	)

	// The read and the write are one step under the lock of the connection: a refresh token that a send is
	// writing meanwhile is neither lost under the price nor writes the former price back (SDD-020 section 34).
	werr := s.conns.UpdateSecret(ctx, row.ID, func(current []byte) ([]byte, error) {
		next, serr := priced.SetPrice(current, price)
		if serr != nil {
			refused = serr

			return nil, serr
		}

		newSecret = bytes.Clone(next)

		return next, nil
	})
	if refused != nil {
		return nil, validationAt("connection_id", "the connection holds no client of your app: sign in first.")
	}

	if werr != nil {
		return nil, connectionError(werr)
	}

	defer clear(newSecret)

	now := s.nowMs()

	if aerr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "social.price", "", row.ID)
	}); aerr != nil {
		return nil, asError(aerr)
	}

	out := SocialAccount{ConnectionID: row.ID, Network: row.Kind, Label: row.Label}
	describeAccount(conn, newSecret, &out)

	return &out, nil
}

// SocialAccountList lists the connections of the social networks with who each signs in as.
func (s *svc) SocialAccountList(ctx context.Context, _ SocialAccountListRequest) (*SocialAccountListResult, error) {
	if _, err := s.connectionCaller(ctx, "SocialAccountList"); err != nil {
		return nil, err
	}

	out := &SocialAccountListResult{Accounts: []SocialAccount{}}

	if s.social == nil {
		return out, nil
	}

	reg := s.social

	list, lerr := s.conns.List(ctx)
	if lerr != nil {
		return nil, connectionError(lerr)
	}

	for i := range list {
		conn, cerr := reg.Get(list[i].Kind)
		if cerr != nil {
			continue
		}

		acc := SocialAccount{
			ConnectionID: list[i].ID,
			Network:      list[i].Kind,
			Label:        list[i].Label,
			State:        AccountNeedsSignIn,
		}

		werr := s.conns.With(ctx, list[i].ID, func(cred connections.Credential) error {
			describeAccount(conn, cred.Secret, &acc)

			return nil
		})
		// A connection whose credential is missing or whose vault is locked still lists, as needing a
		// sign-in; the reason is for the Connections page to show from ConnectionGet.
		_ = werr

		out.Accounts = append(out.Accounts, acc)
	}

	return out, nil
}
