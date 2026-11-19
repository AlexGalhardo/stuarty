package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/http/middleware"

	vauth "github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"
)

type loginIn struct {
	Email    string `json:"email"    validate:"required" doc:"The address the user was created with."`
	Password string `json:"password" validate:"required"`
}

type sessionOut struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Role   string   `json:"role"    doc:"admin or member."`
	Scopes []string `json:"scopes"  doc:"What a browser session of this user may do."`
}

func sessionOf(u auth.User) sessionOut {
	return sessionOut{UserID: u.ID, Email: u.Email, Role: u.Role, Scopes: u.Scopes()}
}

// notInThisMode answers the routes that exist in network mode only, the way a
// route that does not exist would (SDD-012 section 4).
func notInThisMode() error {
	return &service.Error{Code: command.CodeNotFound, Message: "this route exists in network mode only."}
}

// LoginLimits are the three limits of the login route, on vukem's keys: per
// email and address, per email, and per address (the shape that vukem's
// authentication scaffold uses). Deps.LoginLimits replaces them in tests.
type LoginLimits struct {
	PerEmailAndAddress, PerEmail, PerAddress ratelimit.Limiter
}

// DefaultLoginLimits are 5 attempts a minute per email and address, 20 in 10
// minutes per email and 30 a minute per address.
func DefaultLoginLimits() *LoginLimits {
	return &LoginLimits{
		PerEmailAndAddress: ratelimit.NewMemory(5, time.Minute),
		PerEmail:           ratelimit.NewMemory(20, 10*time.Minute),
		PerAddress:         ratelimit.NewMemory(30, time.Minute),
	}
}

type loginThrottles struct {
	limits          *LoginLimits
	emailAndAddress ratelimit.KeyFunc
	email           ratelimit.KeyFunc
}

func newLoginThrottles(l *LoginLimits) loginThrottles {
	if l == nil {
		l = DefaultLoginLimits()
	}

	ip, account := vauth.ThrottleKeys("email")

	return loginThrottles{limits: l, emailAndAddress: ip, email: account}
}

// middleware refuses the attempt before the password is hashed: hashing is the
// expensive step an attacker would like to make us repeat.
func (t loginThrottles) middleware(next http.Handler) http.Handler {
	return middleware.Limit(t.limits.PerAddress, ratelimit.ByIP)(
		middleware.Limit(t.limits.PerEmailAndAddress, t.emailAndAddress)(
			middleware.Limit(t.limits.PerEmail, t.email)(next)))
}

// login is POST /auth/login: the one route that turns a password into a session
// (SDD-007 section 4.1). A wrong email and a wrong password are the same answer.
func (h *apiHandler) login(c *web.Ctx, in loginIn) (sessionOut, error) {
	if h.Network == nil {
		return sessionOut{}, notInThisMode()
	}

	// An address that spent its failures is refused before the password is looked at.
	if e := h.limits.failed.Blocked(ratelimit.ByIP(c.Request())); e != nil {
		return sessionOut{}, e
	}

	u, err := h.Auth.Guard.Attempt(c, map[string]string{"email": in.Email, "password": in.Password})
	if err != nil {
		if _, ok := errors.AsType[*errs.ValidationError](err); ok {
			if denied := h.limits.failed.Record(c.Request(), ratelimit.ByIP(c.Request())); denied != nil {
				return sessionOut{}, denied
			}

			c.SetHeader("WWW-Authenticate", "Bearer")

			return sessionOut{}, &service.Error{
				Code: service.CodeUnauthorized, Message: "the email or the password is not right.",
			}
		}

		return sessionOut{}, err
	}

	c.SetHeader("Cache-Control", "no-store")

	return sessionOf(u), nil
}

// logout is POST /auth/logout: the session ends and its cookie is cleared.
func (h *apiHandler) logout(c *web.Ctx, _ noIn) (noIn, error) {
	if h.Network == nil {
		return noIn{}, notInThisMode()
	}

	return noIn{}, h.Auth.Guard.Logout(c)
}
