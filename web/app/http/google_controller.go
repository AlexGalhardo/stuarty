package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/http/middleware"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/web"
)

// The optional Google sign-in of network mode (SDD-007 section 4.7, SDD-014 section 12). The rules are in
// auth.Google; these handlers carry a request to it, set and read the sign-in cookie, and open the session
// with the same guard as the password login.

// googleBindingMaxAge is how long the sign-in cookie lives: the time a flow may wait (oauth.TTL).
const googleBindingMaxAge = int(oauth.TTL / 1e9)

// googleOpen tells the routes of the Google sign-in that need no session: the three a person who is not
// signed in uses (the state of the feature, the start, the finish) and the page Google sends the browser to.
func googleOpen(r *http.Request) bool {
	switch r.URL.Path {
	case "/auth/google", auth.GoogleCallbackPath:
		return r.Method == http.MethodGet
	case "/auth/google/start", "/auth/google/complete":
		return r.Method == http.MethodPost
	}

	return false
}

type googleStatusOut struct {
	Enabled bool                 `json:"enabled" doc:"Whether this server lets people sign in with Google."`
	Linked  *auth.LinkedIdentity `json:"linked"  doc:"The Google account of the signed-in user, or null."`
}

type googleStartOut struct {
	AuthorizeURL string `json:"authorize_url" doc:"Open this address to sign in at Google."`
}

type googlePasswordIn struct {
	Password string `json:"password" validate:"required" doc:"The password of the signed-in user, asked again to change the link."`
}

type googleCompleteIn struct {
	State string `json:"state"           validate:"required" doc:"The state Google sent back to the redirect address."`
	Code  string `json:"code,omitempty"                      doc:"The authorization code Google sent back."`
	Error string `json:"error,omitempty"                     doc:"The error Google sent back instead of a code."`
}

type googleCompleteOut struct {
	User   *sessionOut          `json:"user,omitempty"   doc:"Set when the sign-in opened a session."`
	Linked *auth.LinkedIdentity `json:"linked,omitempty" doc:"Set when the flow linked a Google account."`
}

type googleSettingsIn struct {
	Enabled       bool     `json:"enabled"                  doc:"Turn Google sign-in on or off."`
	ConnectionID  string   `json:"connection_id,omitempty"  doc:"The connection of your Google project (kind youtube); required to turn it on."`
	AllowedEmails []string `json:"allowed_emails,omitempty" doc:"When not empty, only these Google e-mail addresses may sign in or link (at most 100)."`
}

type googleSettingsOut struct {
	auth.GoogleSettings
	RedirectURL string `json:"redirect_url" doc:"Add this address to the authorised redirect URIs of your Google project."`
}

// google is the sign-in, or E_NOT_FOUND where it does not exist: in local mode, and in a server built
// without it.
func (h *apiHandler) google() (*auth.Google, error) {
	if h.Network == nil || h.Auth == nil || h.Google == nil {
		return nil, notInThisMode()
	}

	return h.Google, nil
}

// gsiCookie is the sign-in cookie: the value that binds a flow to the browser that began it (SDD-014
// section 12.5). Secure and with the __Host- prefix except in the plain-HTTP container case.
func (h *apiHandler) gsiCookie(value string, maxAge int) *http.Cookie {
	secure := !h.Network.loopbackHTTP

	name := "stuarty_gsi"
	if secure {
		name = "__Host-stuarty_gsi"
	}

	// Secure is off only in the plain-HTTP container case of SDD-007 section 4.3 item 4.
	return &http.Cookie{ //nolint:gosec // G124: see above
		Name: name, Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	}
}

func (h *apiHandler) gsiBinding(r *http.Request) string {
	if ck, err := r.Cookie(h.gsiCookie("", 0).Name); err == nil {
		return ck.Value
	}

	return ""
}

// googleLimit counts the call against the per-address login limit (SDD-007 section 4.1 item 5).
func (h *apiHandler) googleLimit(c *web.Ctx) error {
	r := c.Request()

	if e := h.limits.failed.Blocked(ratelimit.ByIP(r)); e != nil {
		return e
	}

	if e := middleware.Allow(r, h.loginThrottle.limits.PerAddress, ratelimit.ByIP(r)); e != nil {
		return e
	}

	return nil
}

// googleFailed turns a refusal into the answer and counts it as a failed authentication of the address
// (SDD-007 section 4.4), which turns into E_RATE_LIMITED once the address spent its failures.
func (h *apiHandler) googleFailed(c *web.Ctx, err error) error {
	out := googleError(err)

	if se, ok := errors.AsType[*service.Error](out); ok && se.Code == service.CodeUnauthorized {
		if denied := h.limits.failed.Record(c.Request(), ratelimit.ByIP(c.Request())); denied != nil {
			return denied
		}
	}

	return out
}

// googleError maps what auth.Google refuses to the error of the API. No message carries a token, a code,
// a claim or the text of Google.
func googleError(err error) error {
	if err == nil {
		return nil
	}

	unauthorized := func(message string) error {
		return &service.Error{Code: service.CodeUnauthorized, Message: message}
	}

	if se, ok := errors.AsType[*auth.SettingsError](err); ok {
		return &service.Error{Code: command.CodeValidation, Message: se.Message, Path: se.Path}
	}

	switch {
	case errors.Is(err, auth.ErrGoogleOff):
		return &service.Error{
			Code: command.CodeNotFound, Message: "Google sign-in is off on this server.",
			Hint: "an administrator turns it on in Settings",
		}
	case errors.Is(err, auth.ErrNotLinked):
		return unauthorized("this Google account is not linked to a user of this server.")
	case errors.Is(err, auth.ErrNotAllowed):
		return unauthorized("this Google account is not on the list the owner of this server allows.")
	case errors.Is(err, auth.ErrIDToken):
		return unauthorized("the Google sign-in could not be verified.")
	case errors.Is(err, auth.ErrFlow), errors.Is(err, auth.ErrDenied):
		return &service.Error{
			Code: command.CodeValidation, Path: "state", Message: err.Error() + ".",
			Hint: "start the sign-in again",
		}
	case errors.Is(err, auth.ErrAlreadyLinked), errors.Is(err, auth.ErrIdentityTaken):
		return &service.Error{Code: command.CodeInvalidState, Message: err.Error() + "."}
	case errors.Is(err, auth.ErrNoIdentity):
		return &service.Error{Code: command.CodeNotFound, Message: err.Error() + "."}
	case errors.Is(err, auth.ErrBadClient):
		return &service.Error{
			Code: command.CodeValidation, Message: auth.ErrBadClient.Error() + ".", Path: "connection_id",
		}
	}

	if reason, hint, ok := connections.Locked(err); ok {
		return &service.Error{
			Code: service.CodeVaultLocked, Message: "the vault of credentials cannot be opened.",
			Details: map[string]any{"reason": reason}, Hint: hint,
		}
	}

	if _, ok := errors.AsType[*oauth.TokenError](err); ok {
		return unauthorized("Google did not accept the sign-in.")
	}

	if res := gateRefusal(err); res != nil {
		return res
	}

	if errors.Is(err, auth.ErrIDTokenKeys) {
		return &service.Error{
			Code: command.CodeInternal, Message: "Google could not be reached to check the sign-in.", Retryable: true,
		}
	}

	return err
}

// googleSession is the user of the browser session of a route that is not open. A key never is: linking
// and the settings are for a person at a browser.
func (h *apiHandler) googleSession(c *web.Ctx) (auth.User, error) {
	caller, ok := service.CallerFrom(c.Context())
	if !ok || caller.OwnerUserID == "" {
		return auth.User{}, &service.Error{
			Code:    service.CodeUnauthorized,
			Message: "the request carries no credentials.",
		}
	}

	if !caller.Browser || strings.HasPrefix(caller.Actor, "key:") {
		return auth.User{}, &service.Error{
			Code: service.CodeForbidden, Message: "only a logged-in session can change the Google account.",
			Hint: "log in in the browser",
		}
	}

	u, err := h.Auth.Users.ByID(c.Context(), caller.OwnerUserID)
	if err != nil {
		return auth.User{}, &service.Error{Code: service.CodeUnauthorized, Message: "the session is not valid."}
	}

	return u, nil
}

// confirmPassword asks the user for the password again: a stolen session alone cannot link or unlink an
// identity. A wrong password counts against the user and the address, like a failed login.
func (h *apiHandler) confirmPassword(c *web.Ctx, u auth.User, password string) error {
	r := c.Request()

	if e := middleware.Allow(r, h.loginThrottle.limits.PerEmail, "google-password:"+u.ID); e != nil {
		return e
	}

	_, err := h.Auth.Guard.Validate(c, map[string]string{"email": u.Email, "password": password})
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*errs.ValidationError](err); ok {
		// Not E_UNAUTHORIZED: the session is fine, and the page must not read this as a lost session.
		if denied := h.limits.failed.Record(r, ratelimit.ByIP(r)); denied != nil {
			return denied
		}

		return &service.Error{Code: command.CodeValidation, Path: "password", Message: "the password is not right."}
	}

	return err
}

// gateRefusal maps the refusal of the outbound gate, which a connector never rewrites, to the error of
// SDD-014 section 7; nil when err is not one.
func gateRefusal(err error) error {
	switch {
	case errors.Is(err, connections.ErrNotFound), errors.Is(err, outbound.ErrNoConnection):
		return &service.Error{
			Code:    service.CodeConnectionNotFound,
			Message: "the connection of the Google project does not exist or was removed.",
			Hint:    "an administrator chooses the connection in Settings",
		}
	case errors.Is(err, outbound.ErrDestination), errors.Is(err, outbound.ErrNotPublic):
		return &service.Error{
			Code:    service.CodeConnectionDestination,
			Message: "the request is for a destination the connection does not allow.",
			Hint:    "a connection reaches only the hosts it lists, over https",
		}
	}

	return nil
}

// googleStatus is GET /auth/google.
func (h *apiHandler) googleStatus(c *web.Ctx, _ noIn) (googleStatusOut, error) {
	g, err := h.google()
	if err != nil {
		return googleStatusOut{}, err
	}

	enabled, err := g.Enabled(c.Context())
	if err != nil {
		return googleStatusOut{}, err
	}

	out := googleStatusOut{Enabled: enabled}

	if user, ok, serr := h.Auth.SessionUser(c); serr == nil && ok {
		if li, has, ierr := g.Identity(c.Context(), user.ID); ierr == nil && has {
			out.Linked = &li
		}
	}

	c.SetHeader("Cache-Control", "no-store")

	return out, nil
}

// googleStart is POST /auth/google/start: the person who is not signed in begins a sign-in.
func (h *apiHandler) googleStart(c *web.Ctx, _ noIn) (googleStartOut, error) {
	g, err := h.google()
	if err != nil {
		return googleStartOut{}, err
	}

	if err := h.googleLimit(c); err != nil {
		return googleStartOut{}, err
	}

	binding, err := auth.NewBinding()
	if err != nil {
		return googleStartOut{}, err
	}

	authorize, err := g.BeginSignIn(c.Context(), binding)
	if err != nil {
		return googleStartOut{}, googleError(err)
	}

	c.SetCookie(h.gsiCookie(binding, googleBindingMaxAge))
	c.SetHeader("Cache-Control", "no-store")

	return googleStartOut{AuthorizeURL: authorize}, nil
}

// googleLink is POST /auth/google/link: a signed-in user, with their password, begins linking.
func (h *apiHandler) googleLink(c *web.Ctx, in googlePasswordIn) (googleStartOut, error) {
	g, err := h.google()
	if err != nil {
		return googleStartOut{}, err
	}

	if err := h.googleLimit(c); err != nil {
		return googleStartOut{}, err
	}

	user, err := h.googleSession(c)
	if err != nil {
		return googleStartOut{}, err
	}

	if err := h.confirmPassword(c, user, in.Password); err != nil {
		return googleStartOut{}, err
	}

	authorize, err := g.BeginLink(c.Context(), user.ID)
	if err != nil {
		return googleStartOut{}, googleError(err)
	}

	c.SetHeader("Cache-Control", "no-store")

	return googleStartOut{AuthorizeURL: authorize}, nil
}

// googleUnlink is POST /auth/google/unlink. The password login is not touched.
func (h *apiHandler) googleUnlink(c *web.Ctx, in googlePasswordIn) (googleStatusOut, error) {
	g, err := h.google()
	if err != nil {
		return googleStatusOut{}, err
	}

	if err := h.googleLimit(c); err != nil {
		return googleStatusOut{}, err
	}

	user, err := h.googleSession(c)
	if err != nil {
		return googleStatusOut{}, err
	}

	if err := h.confirmPassword(c, user, in.Password); err != nil {
		return googleStatusOut{}, err
	}

	if err := g.Unlink(c.Context(), user.ID); err != nil {
		return googleStatusOut{}, googleError(err)
	}

	enabled, err := g.Enabled(c.Context())

	return googleStatusOut{Enabled: enabled}, err
}

// googleComplete is POST /auth/google/complete: the page Google sent the browser to hands over the state
// and the code. A flow of sign-in is bound to the cookie of the browser that began it; a flow of linking, to
// the session. The sign-in opens the session with Guard.Login, which issues a new session id.
func (h *apiHandler) googleComplete(c *web.Ctx, in googleCompleteIn) (googleCompleteOut, error) {
	g, err := h.google()
	if err != nil {
		return googleCompleteOut{}, err
	}

	r := c.Request()

	// Step 6 of SDD-007 section 3.6: a state-changing request must show where it comes from and be JSON.
	if !ambientAllowed(r) {
		return googleCompleteOut{}, refused()
	}

	if err := h.googleLimit(c); err != nil {
		return googleCompleteOut{}, err
	}

	req := auth.CompleteRequest{State: in.State, Code: in.Code, Error: in.Error, Binding: h.gsiBinding(r)}

	if user, ok, serr := h.Auth.SessionUser(c); serr == nil && ok {
		req.SessionUserID = user.ID
	}

	// The cookie served its flow, whatever happens next.
	c.SetCookie(h.gsiCookie("", -1))
	c.SetHeader("Cache-Control", "no-store")

	done, err := g.Complete(c.Context(), req)
	if err != nil {
		return googleCompleteOut{}, h.googleFailed(c, err)
	}

	if done.SignedIn != nil {
		if err := h.Auth.Guard.Login(c, *done.SignedIn); err != nil {
			return googleCompleteOut{}, err
		}

		out := sessionOf(*done.SignedIn)

		return googleCompleteOut{User: &out}, nil
	}

	return googleCompleteOut{Linked: done.Linked}, nil
}

// googleSettingsGet is GET /auth/google/settings, for an administrator session.
func (h *apiHandler) googleSettingsGet(c *web.Ctx, _ noIn) (googleSettingsOut, error) {
	g, err := h.google()
	if err != nil {
		return googleSettingsOut{}, err
	}

	if err := auth.AdminSession(c.Context()); err != nil {
		return googleSettingsOut{}, err
	}

	s, err := g.Settings(c.Context())

	return googleSettingsOut{GoogleSettings: s, RedirectURL: g.RedirectURL()}, err
}

// googleSettingsPut is PUT /auth/google/settings, for an administrator session.
func (h *apiHandler) googleSettingsPut(c *web.Ctx, in googleSettingsIn) (googleSettingsOut, error) {
	g, err := h.google()
	if err != nil {
		return googleSettingsOut{}, err
	}

	if err := auth.AdminSession(c.Context()); err != nil {
		return googleSettingsOut{}, err
	}

	s, err := g.SetSettings(c.Context(), auth.GoogleSettings{
		Enabled: in.Enabled, ConnectionID: in.ConnectionID, AllowedEmails: in.AllowedEmails,
	})
	if err != nil {
		return googleSettingsOut{}, googleError(err)
	}

	return googleSettingsOut{GoogleSettings: s, RedirectURL: g.RedirectURL()}, nil
}
