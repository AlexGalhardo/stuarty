package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"

	vauth "github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/database"
)

// The optional Google sign-in of network mode (SDD-007 section 4.7, SDD-014 section 12). This file is the
// rules of it: the owner's settings, the identities users linked, and the flow with its checks. The HTTP
// adapter only carries requests to it.
const (
	// GoogleProvider is the provider column of user_identities.
	GoogleProvider = "google"
	// GoogleCallbackPath is where Google sends the browser back; APP_URL followed by it is the redirect address.
	GoogleCallbackPath = "/login/google/callback"

	// MaxAllowedEmails is the longest list of addresses an owner may restrict sign-in to.
	MaxAllowedEmails = 100

	statePrefixSignIn = "l."
	statePrefixLink   = "k."
	flowSignIn        = "google:login"
	flowLink          = "google:link"

	keyEnabled    = "google.enabled"
	keyConnection = "google.connection_id"
	keyAllowed    = "google.allowed_emails"

	maxClientFieldBytes = 1024
)

// Errors of the Google sign-in. None carries a token, a code or the text of Google.
var (
	// ErrGoogleOff is a request for Google sign-in while the owner has it off, or has chosen no connection.
	ErrGoogleOff = errors.New("google sign-in is off on this server")
	// ErrNotLinked is a Google identity that no user linked, or whose user is disabled.
	ErrNotLinked = errors.New("this Google account is not linked to a user of this server")
	// ErrNotAllowed is an e-mail that is not on the owner's list.
	ErrNotAllowed = errors.New("this Google account is not on the list the owner allows")
	// ErrFlow is a state that is unknown, used, expired, begun by another browser or for another purpose.
	ErrFlow = errors.New("the sign-in is unknown, was used already, took too long or began in another browser")
	// ErrDenied is a redirect that carries no code: the person cancelled or Google refused.
	ErrDenied = errors.New("google did not give a code: the sign-in was refused or cancelled")
	// ErrAlreadyLinked is a user who has a Google identity already.
	ErrAlreadyLinked = errors.New("this user has a Google account linked already; unlink it first")
	// ErrIdentityTaken is a Google identity that another user linked.
	ErrIdentityTaken = errors.New("this Google account is linked to another user")
	// ErrNoIdentity is an unlink of a user who has none linked.
	ErrNoIdentity = errors.New("no Google account is linked")
	// ErrBadClient is a connection whose secret holds no client of a Google project.
	ErrBadClient = errors.New("the connection holds no client of a Google project")
)

// SettingsError is a setting the owner sent that is not acceptable; Path names the member.
type SettingsError struct {
	Path    string
	Message string
}

func (e *SettingsError) Error() string { return e.Path + ": " + e.Message }

// Endpoints are the four addresses of the identity provider (SDD-014 section 12.1).
type Endpoints struct {
	// AuthURL is where the browser is sent; the server never opens it.
	AuthURL string
	// TokenURL and KeysURL are reached through the gate, with the hosts of the connection.
	TokenURL string
	KeysURL  string
	// Issuer is the exact iss of an ID token.
	Issuer string
}

// GoogleEndpoints are the addresses of Google.
func GoogleEndpoints() Endpoints {
	return Endpoints{ //nolint:gosec // G101: public addresses of Google, no credential
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		KeysURL:  "https://www.googleapis.com/oauth2/v3/certs",
		Issuer:   "https://accounts.google.com",
	}
}

// TestEndpoints are the addresses of a fake Google at base (STUARTY_TEST_GOOGLE_URL).
func TestEndpoints(base string) Endpoints {
	base = strings.TrimRight(base, "/")

	return Endpoints{AuthURL: base + "/authorize", TokenURL: base + "/token", KeysURL: base + "/certs", Issuer: base}
}

// ConnectionSource is what the sign-in needs of the registry of connections: the one place a credential
// is read and the one gated client exists (*connections.Manager).
type ConnectionSource interface {
	Get(ctx context.Context, id string) (connections.Connection, error)
	With(ctx context.Context, id string, fn func(connections.Credential) error) error
}

// GoogleOptions configure NewGoogle.
type GoogleOptions struct {
	DB          database.Executor
	Users       *Users
	Connections ConnectionSource
	// AppURL is APP_URL: the redirect address is it followed by GoogleCallbackPath.
	AppURL string
	// Endpoints is zero for Google's own.
	Endpoints Endpoints
	Now       func() time.Time
}

// Google is the optional Google sign-in of a server.
type Google struct {
	db       database.Executor
	users    *Users
	conns    ConnectionSource
	ep       Endpoints
	redirect string
	// signIns and links wait apart: anyone may begin a sign-in, and the store of waiting flows is capped, so
	// a flood of sign-ins would otherwise push out the linking a user began with their password.
	signIns  *oauth.Flows
	links    *oauth.Flows
	nonceKey [32]byte
	now      func() time.Time
}

// NewGoogle builds the sign-in over the workspace database and the connections.
func NewGoogle(o GoogleOptions) (*Google, error) {
	u, err := url.Parse(o.AppURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("auth: Google sign-in needs the APP_URL of the server")
	}

	if o.DB == nil || o.Users == nil || o.Connections == nil {
		return nil, errors.New("auth: Google sign-in needs the database, the users and the connections")
	}

	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Endpoints == (Endpoints{}) {
		o.Endpoints = GoogleEndpoints()
	}

	g := &Google{
		db: o.DB, users: o.Users, conns: o.Connections, ep: o.Endpoints,
		redirect: strings.TrimRight(o.AppURL, "/") + GoogleCallbackPath,
		signIns:  oauth.NewFlows(o.Now), links: oauth.NewFlows(o.Now), now: o.Now,
	}

	if _, err := rand.Read(g.nonceKey[:]); err != nil {
		return nil, err
	}

	return g, nil
}

// RedirectURL is the address the owner registers in the Google project.
func (g *Google) RedirectURL() string { return g.redirect }

// GoogleSettings are the owner's settings (SDD-014 section 12.3).
type GoogleSettings struct {
	Enabled       bool     `json:"enabled"`
	ConnectionID  string   `json:"connection_id"`
	AllowedEmails []string `json:"allowed_emails"`
}

// Settings reads the settings; an absent one is off, with no connection and no list.
func (g *Google) Settings(ctx context.Context) (GoogleSettings, error) {
	out := GoogleSettings{AllowedEmails: []string{}}

	rows, err := g.db.QueryContext(ctx, `SELECT key, value FROM auth_settings WHERE key LIKE 'google.%'`)
	if err != nil {
		return out, err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}

		switch key {
		case keyEnabled:
			out.Enabled = value == "true"
		case keyConnection:
			out.ConnectionID = value
		case keyAllowed:
			var list []string
			if json.Unmarshal([]byte(value), &list) == nil && list != nil {
				out.AllowedEmails = list
			}
		}
	}

	return out, rows.Err()
}

// Enabled says whether a person may begin a Google sign-in: the owner turned it on and chose a connection.
func (g *Google) Enabled(ctx context.Context) (bool, error) {
	s, err := g.Settings(ctx)

	return s.Enabled && s.ConnectionID != "", err
}

// SetSettings validates and stores the settings. Enabling needs a connection of kind youtube whose secret
// holds the client of the owner's Google project.
func (g *Google) SetSettings(ctx context.Context, s GoogleSettings) (GoogleSettings, error) {
	allowed, err := normalizeAllowed(s.AllowedEmails)
	if err != nil {
		return GoogleSettings{}, err
	}

	s.AllowedEmails = allowed

	if s.Enabled || s.ConnectionID != "" {
		if err := g.checkConnection(ctx, s.ConnectionID); err != nil {
			return GoogleSettings{}, err
		}
	}

	list, err := json.Marshal(s.AllowedEmails)
	if err != nil {
		return GoogleSettings{}, err
	}

	// Turning it off is written first and turning it on last, so a half-written change never leaves the
	// feature on with the old connection.
	steps := []struct{ key, value string }{
		{keyEnabled, "false"}, {keyConnection, s.ConnectionID}, {keyAllowed, string(list)},
	}
	if s.Enabled {
		steps = append(steps, struct{ key, value string }{keyEnabled, "true"})
	}

	for _, st := range steps {
		if err := g.put(ctx, st.key, st.value); err != nil {
			return GoogleSettings{}, err
		}
	}

	return s, nil
}

var errNoConnectionChosen = &SettingsError{
	Path:    "connection_id",
	Message: "choose the connection of your Google project.",
}

func (g *Google) checkConnection(ctx context.Context, id string) error {
	if id == "" {
		return errNoConnectionChosen
	}

	conn, err := g.conns.Get(ctx, id)
	if err != nil {
		if errors.Is(err, connections.ErrNotFound) {
			return &SettingsError{Path: "connection_id", Message: "there is no such connection."}
		}

		return err
	}

	if conn.Kind != publish.NetworkYouTube {
		return &SettingsError{
			Path: "connection_id", Message: "the connection must be the one of your Google project (kind youtube).",
		}
	}

	err = g.conns.With(ctx, id, func(c connections.Credential) error {
		_, _, cerr := googleClient(c.Secret)

		return cerr
	})
	if errors.Is(err, ErrBadClient) {
		return &SettingsError{Path: "connection_id", Message: ErrBadClient.Error() + "."}
	}

	return err
}

func (g *Google) put(ctx context.Context, key, value string) error {
	_, err := g.db.ExecContext(ctx,
		`INSERT INTO auth_settings (key, value, updated_ms) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_ms = excluded.updated_ms`,
		key, value, g.now().UnixMilli())

	return err
}

func normalizeAllowed(list []string) ([]string, error) {
	out := make([]string, 0, len(list))

	for _, raw := range list {
		email, err := NormalizeEmail(raw)
		if err != nil {
			return nil, &SettingsError{Path: "allowed_emails", Message: "every entry must be a plain e-mail address."}
		}

		if !slices.Contains(out, email) {
			out = append(out, email)
		}
	}

	if len(out) > MaxAllowedEmails {
		return nil, &SettingsError{
			Path: "allowed_emails", Message: fmt.Sprintf("at most %d addresses.", MaxAllowedEmails),
		}
	}

	return out, nil
}

// googleClient reads the client of the owner's Google project out of the secret of a connection.
func googleClient(secret []byte) (id, clientSecret string, err error) {
	var s struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}

	if json.Unmarshal(secret, &s) != nil || s.ClientID == "" || s.ClientSecret == "" ||
		len(s.ClientID) > maxClientFieldBytes || len(s.ClientSecret) > maxClientFieldBytes {
		return "", "", ErrBadClient
	}

	return s.ClientID, s.ClientSecret, nil
}

// LinkedIdentity is what a user may see of the identity they linked.
type LinkedIdentity struct {
	Email    string `json:"email"`
	LinkedMS int64  `json:"linked_ms"`
}

// Identity is the Google identity a user linked, if any.
func (g *Google) Identity(ctx context.Context, userID string) (LinkedIdentity, bool, error) {
	var li LinkedIdentity

	err := g.db.QueryRowContext(ctx,
		`SELECT email, created_ms FROM user_identities WHERE provider = ? AND user_id = ?`,
		GoogleProvider, userID).Scan(&li.Email, &li.LinkedMS)
	if errors.Is(err, sql.ErrNoRows) {
		return LinkedIdentity{}, false, nil
	}

	return li, err == nil, err
}

func (g *Google) link(ctx context.Context, userID string, id Identity) error {
	_, err := g.db.ExecContext(ctx,
		`INSERT INTO user_identities (provider, subject, user_id, email, created_ms) VALUES (?, ?, ?, ?, ?)`,
		GoogleProvider, id.Subject, userID, id.Email, g.now().UnixMilli())
	if err == nil {
		return nil
	}

	if uc, ok := g.db.(interface{ UniqueViolation(error) bool }); ok && uc.UniqueViolation(err) {
		if _, has, _ := g.Identity(ctx, userID); has {
			return ErrAlreadyLinked
		}

		return ErrIdentityTaken
	}

	return err
}

// Unlink removes the Google identity of a user. The password login is not touched.
func (g *Google) Unlink(ctx context.Context, userID string) error {
	res, err := g.db.ExecContext(ctx,
		`DELETE FROM user_identities WHERE provider = ? AND user_id = ?`, GoogleProvider, userID)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoIdentity
	}

	return nil
}

// ByIdentity is the user a Google identity was linked to: by the subject, never by the e-mail. A disabled
// user is as good as none.
func (u *Users) ByIdentity(ctx context.Context, provider, subject string) (User, error) {
	user, err := u.one(ctx, `WHERE id = (SELECT user_id FROM user_identities WHERE provider = ? AND subject = ?)`,
		provider, subject)
	if err == nil && user.Disabled {
		return User{}, ErrNotLinked
	}

	if errors.Is(err, vauth.ErrUserNotFound) {
		return User{}, ErrNotLinked
	}

	return user, err
}

// BindingActor is the actor a sign-in flow is bound to: the SHA-256 of the value of the sign-in cookie, so
// the cookie itself is in no table and no log.
func BindingActor(binding string) string {
	sum := sha256.Sum256([]byte(binding))

	return "gsi:" + hex.EncodeToString(sum[:])
}

// NewBinding makes the value of a sign-in cookie.
func NewBinding() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw[:]), nil
}

// nonce is the nonce of a flow: a keyed hash of its state. The key never leaves the process, so no one who
// is not holding the flow can make a token that carries it.
func (g *Google) nonce(flowState string) string {
	mac := hmac.New(sha256.New, g.nonceKey[:])
	mac.Write([]byte(flowState))

	return hex.EncodeToString(mac.Sum(nil))
}

// BeginSignIn starts a sign-in for the browser that holds the cookie value binding, and answers the
// address of Google's consent screen.
func (g *Google) BeginSignIn(ctx context.Context, binding string) (string, error) {
	return g.begin(ctx, statePrefixSignIn, flowSignIn, BindingActor(binding))
}

// BeginLink starts the linking of the Google account of the user whose browser session this is.
func (g *Google) BeginLink(ctx context.Context, userID string) (string, error) {
	return g.begin(ctx, statePrefixLink, flowLink, "user:"+userID)
}

func (g *Google) begin(ctx context.Context, prefix, purpose, actor string) (string, error) {
	s, err := g.Settings(ctx)
	if err != nil {
		return "", err
	}

	if !s.Enabled || s.ConnectionID == "" {
		return "", ErrGoogleOff
	}

	var clientID string

	err = g.conns.With(ctx, s.ConnectionID, func(c connections.Credential) error {
		var cerr error

		clientID, _, cerr = googleClient(c.Secret)

		return cerr
	})
	if err != nil {
		return "", err
	}

	state, challenge := g.flowsOf(purpose).Begin(oauth.Pending{
		ConnectionID: s.ConnectionID, Network: purpose, RedirectURI: g.redirect, Actor: actor, Browser: true,
	})

	return oauth.AuthorizeURL(
		g.ep.AuthURL, clientID, g.redirect, prefix+state, challenge, []string{"openid", "email"},
		url.Values{"nonce": {g.nonce(state)}, "prompt": {"select_account"}},
	), nil
}

// CompleteRequest is what the redirect page hands over.
type CompleteRequest struct {
	State string
	Code  string
	// Error is what Google sent instead of a code.
	Error string
	// Binding is the value of the sign-in cookie of the request, or empty when it has none.
	Binding string
	// SessionUserID is the user of the browser session of the request, or empty when it has none.
	SessionUserID string
}

// Completed is the result of a finished flow: SignedIn is the user to open a session for, or Linked is
// the identity that was linked.
type Completed struct {
	SignedIn *User
	Linked   *LinkedIdentity
}

// Complete finishes a flow: it spends the state, trades the code for the ID token through the gated client
// of the connection, verifies the token and then either finds the user that linked the identity (sign-in)
// or links it to the user of the session (linking). The user is found by the subject and never by the
// e-mail.
func (g *Google) Complete(ctx context.Context, req CompleteRequest) (Completed, error) {
	purpose, flowState, ok := splitState(req.State)
	if !ok {
		return Completed{}, ErrFlow
	}

	s, err := g.Settings(ctx)
	if err != nil {
		return Completed{}, err
	}

	if !s.Enabled || s.ConnectionID == "" {
		return Completed{}, ErrGoogleOff
	}

	// The state is taken first and always: a refusal, another browser and a replay all spend it.
	actor := "gsi:" // no cookie, no actor: Take refuses and the state is gone
	if purpose == flowSignIn && req.Binding != "" {
		actor = BindingActor(req.Binding)
	}

	if purpose == flowLink {
		actor = "user:" + req.SessionUserID // no session: "user:", which no flow was begun by
	}

	// A state shown with the prefix of the other purpose is spent there too, so it cannot be tried again.
	_, _ = g.flowsOf(otherPurpose(purpose)).Take(flowState, "", true)

	p, err := g.flowsOf(purpose).Take(flowState, actor, true)
	if err != nil || p.Network != purpose {
		return Completed{}, ErrFlow
	}

	// The owner may have chosen another connection while the person was at Google.
	if s.ConnectionID != p.ConnectionID {
		return Completed{}, ErrFlow
	}

	if req.Error != "" || req.Code == "" {
		return Completed{}, ErrDenied
	}

	id, err := g.exchange(ctx, s.ConnectionID, p, flowState, req.Code)
	if err != nil {
		return Completed{}, err
	}

	if len(s.AllowedEmails) > 0 && !slices.Contains(s.AllowedEmails, id.Email) {
		return Completed{}, ErrNotAllowed
	}

	if purpose == flowLink {
		if err := g.link(ctx, req.SessionUserID, id); err != nil {
			return Completed{}, err
		}

		li, _, err := g.Identity(ctx, req.SessionUserID)

		return Completed{Linked: &li}, err
	}

	user, err := g.users.ByIdentity(ctx, GoogleProvider, id.Subject)
	if err != nil {
		return Completed{}, err
	}

	// The e-mail is only shown; keep it as Google says it now is.
	_, _ = g.db.ExecContext(ctx, `UPDATE user_identities SET email = ? WHERE provider = ? AND subject = ?`,
		id.Email, GoogleProvider, id.Subject)

	return Completed{SignedIn: &user}, nil
}

func (g *Google) exchange(
	ctx context.Context, connectionID string, p oauth.Pending, flowState, code string,
) (Identity, error) {
	var id Identity

	err := g.conns.With(ctx, connectionID, func(c connections.Credential) error {
		clientID, clientSecret, err := googleClient(c.Secret)
		if err != nil {
			return err
		}

		tok, err := oauth.Exchange(
			ctx,
			c.Client,
			g.ep.TokenURL,
			clientID,
			clientSecret,
			code,
			p.Verifier,
			p.RedirectURI,
		)
		if err != nil {
			return err
		}

		if tok.IDToken == "" {
			return ErrIDToken
		}

		id, err = VerifyIDToken(ctx, IDTokenConfig{
			Issuer: g.ep.Issuer, KeysURL: g.ep.KeysURL, ClientID: clientID, Nonce: g.nonce(flowState),
			Client: c.Client, Now: g.now,
		}, tok.IDToken)

		return err
	})

	return id, err
}

// flowsOf is the store of the flows of a purpose.
func (g *Google) flowsOf(purpose string) *oauth.Flows {
	if purpose == flowLink {
		return g.links
	}

	return g.signIns
}

func otherPurpose(purpose string) string {
	if purpose == flowLink {
		return flowSignIn
	}

	return flowLink
}

// splitState reads the state the browser carries: l. or k. and the state of the flow.
func splitState(raw string) (purpose, flowState string, ok bool) {
	switch {
	case strings.HasPrefix(raw, statePrefixSignIn):
		return flowSignIn, strings.TrimPrefix(raw, statePrefixSignIn), len(raw) > len(statePrefixSignIn)
	case strings.HasPrefix(raw, statePrefixLink):
		return flowLink, strings.TrimPrefix(raw, statePrefixLink), len(raw) > len(statePrefixLink)
	}

	return "", "", false
}
