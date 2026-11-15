package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/database"
	vsession "github.com/AlexGalhardo/vukem/session"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
)

// Session lifetimes of network mode (SDD-007 section 4.1 item 4).
const (
	NetworkSessionIdle = 8 * time.Hour
	NetworkSessionMax  = 7 * 24 * time.Hour
)

// GuardName names the session guard; vukem keys the session values by it.
const GuardName = "web"

// NetworkOptions configure NewNetwork.
type NetworkOptions struct {
	// PlainHTTP drops the Secure flag and the __Host- prefix: the container
	// that is reachable only as localhost over plain HTTP (SDD-007 section 4.3
	// item 4). Every other deployment has Secure cookies.
	PlainHTTP bool
	Now       func() time.Time
	Log       *slog.Logger
	// Hasher is argon2id with vukem's parameters when nil.
	Hasher hash.Hasher
}

// Network is the credential side of network mode: users, the session guard over
// vukem sessions kept in the workspace database, and the API keys.
type Network struct {
	Users    *Users
	Guard    *auth.SessionGuard[User]
	Sessions *vsession.Manager

	db     *database.DB
	store  vsession.UserStore
	tokens *auth.Tokens
	now    func() time.Time
}

// NewNetwork builds the guard over db. The sessions table comes from the
// migrations of the store.
func NewNetwork(db *database.DB, o NetworkOptions) (*Network, error) {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Log == nil {
		o.Log = slog.Default()
	}

	users := NewUsers(db)
	users.Now = o.Now

	if o.Hasher != nil {
		users.Hasher = o.Hasher
	}

	store, err := vsession.NewSQLStore(db.SQL())
	if err != nil {
		return nil, err
	}

	manager, err := vsession.New(vsession.Config{
		CookieName:  CookieName,
		Secure:      !o.PlainHTTP,
		SameSite:    http.SameSiteLaxMode,
		IdleTimeout: NetworkSessionIdle,
		Lifetime:    NetworkSessionMax,
		Store:       store,
		Now:         o.Now,
		Log:         o.Log,
	})
	if err != nil {
		return nil, err
	}

	tokens := auth.NewTokens(db)
	tokens.Prefix = KeyPrefix
	tokens.Now = o.Now
	tokens.Log = o.Log

	return &Network{
		Users:    users,
		Guard:    auth.NewSessionGuard[User](GuardName, users, users.Hasher, nil),
		Sessions: manager,
		db:       db,
		store:    store,
		tokens:   tokens,
		now:      o.Now,
	}, nil
}

// Caller turns a user into the caller of a browser session.
func (u User) Caller() service.Caller {
	return service.Caller{Actor: "user:" + u.ID, Scopes: u.Scopes(), OwnerUserID: u.ID, Browser: true}
}

// SessionUser is the user of the session cookie of the request, if there is a
// live one. The session middleware must have run. A session whose user is gone
// or whose version moved is ended by the guard.
func (n *Network) SessionUser(c *web.Ctx) (User, bool, error) {
	u, err := n.Guard.User(c)

	switch {
	case err == nil:
		return u, true, nil
	case errors.Is(err, auth.ErrUnauthenticated):
		return User{}, false, nil
	default:
		return User{}, false, err
	}
}

// ChangePassword stores a new password and ends everything that the old one
// opened: every browser session and every API key of the user (SDD-007 section
// 4.1 item 4).
func (n *Network) ChangePassword(ctx context.Context, email, password string) (User, error) {
	user, err := n.Users.SetPassword(ctx, email, password)
	if err != nil {
		return User{}, err
	}

	if err := n.store.RevokeOthers(ctx, n.Guard.SessionUserID(user.ID), ""); err != nil {
		return user, err
	}

	return user, n.RevokeKeysOf(ctx, user.ID)
}
