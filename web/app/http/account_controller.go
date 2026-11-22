package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"time"

	rules "github.com/AlexGalhardo/stuarty/core/account"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/web/app/auth"

	"github.com/AlexGalhardo/vukem/web"
)

// accountPaths are the routes of the local account that answer with no credential: the state, the sign-in and
// the recovery by key (SDD-007 section 3.9 item 7). The creation, the logout and the settings need a caller.
var accountPaths = map[string]string{
	BasePathAuth + "/account":         http.MethodGet,
	BasePathAuth + "/account/login":   http.MethodPost,
	BasePathAuth + "/account/recover": http.MethodPost,
}

// BasePathAuth is the prefix of the routes that turn a credential into a session.
const BasePathAuth = "/auth"

func accountOpen(r *http.Request) bool {
	method, ok := accountPaths[r.URL.Path]

	return ok && method == r.Method
}

type accountStateOut struct {
	State    string `json:"state"              doc:"none (no account yet), locked or unlocked."`
	Username string `json:"username,omitempty"`
	// IdleLockMinutes is the idle lock, 0 meaning off; shown to a person only.
	IdleLockMinutes *int `json:"idle_lock_minutes,omitempty"`
}

type accountCreateIn struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

type accountCreateOut struct {
	RecoveryKey string `json:"recovery_key" doc:"Shown once. It is not stored and cannot be shown again."`
}

type accountLoginIn struct {
	Identity string `json:"identity" doc:"The username or the e-mail address."`
	Password string `json:"password"`
}

type accountRecoverIn struct {
	RecoveryKey  string `json:"recovery_key"`
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

type accountPasswordIn struct {
	Current      string `json:"current"`
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

type accountSettingsIn struct {
	IdleLockMinutes int `json:"idle_lock_minutes" doc:"0 turns the idle lock off; at most 1440."`
}

// accountOr is the manager of the account, or the answer of a route that does not exist in this mode.
func (h *apiHandler) accountOr() (*account.Manager, error) {
	if h.Account == nil {
		return nil, notInThisModeLocal()
	}

	return h.Account, nil
}

// notInThisModeLocal answers the routes of the local account where there is none (network mode), the way a
// route that does not exist would.
func notInThisModeLocal() error {
	return &service.Error{Code: command.CodeNotFound, Message: "this route exists in local mode only."}
}

// person is a caller that typed the password in a browser (SDD-007 section 3.9 item 5).
func person(c *web.Ctx) error {
	if caller, ok := service.CallerFrom(c.Context()); !ok || !caller.Browser {
		return &service.Error{
			Code: service.CodePrivilegedSession, Message: "only a person signed in with the password can do this.",
			Hint: "sign in with your password in the app",
		}
	}

	return nil
}

func (h *apiHandler) accountState(c *web.Ctx, _ noIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	c.SetHeader("Cache-Control", "no-store")

	out := accountStateOut{State: string(m.State()), Username: m.Username()}

	if caller, ok := service.CallerFrom(c.Context()); ok && caller.Browser && m.State() == account.StateUnlocked {
		minutes := m.IdleLockMinutes()
		out.IdleLockMinutes = &minutes
	}

	return out, nil
}

func (h *apiHandler) accountCreate(c *web.Ctx, in accountCreateIn) (accountCreateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountCreateOut{}, err
	}

	// A hosted agent holds a project and no admin scope: it must not set the password of the studio and sign in
	// as a person with it.
	if caller, ok := service.CallerFrom(c.Context()); !ok || !slices.Contains(caller.Scopes, service.ScopeAdmin) {
		return accountCreateOut{}, &service.Error{
			Code: service.CodeForbidden, Message: "only a person opening Stuarty can create the account.",
			Hint: "open Stuarty and create the account there",
		}
	}

	// On a large workspace the creation outlasts the write timeout of the server, and its answer is the recovery
	// key, which exists nowhere else: it must reach the person whatever the time.
	rc := http.NewResponseController(c.Writer())
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	recovery, err := m.Create(c.Context(), rules.Input{
		Username: in.Username, Email: in.Email, Password: in.Password, Confirmation: in.Confirmation,
	}, h.AccountApply)
	if err != nil {
		return accountCreateOut{}, accountError(err)
	}

	c.SetHeader("Cache-Control", "no-store")

	return accountCreateOut{RecoveryKey: recovery}, nil
}

func (h *apiHandler) accountLogin(c *web.Ctx, in accountLoginIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := m.Login(c.Context(), in.Identity, in.Password); err != nil {
		return accountStateOut{}, accountError(err)
	}

	if err := h.startPersonSession(c); err != nil {
		m.Lock()

		return accountStateOut{}, err
	}

	minutes := m.IdleLockMinutes()

	return accountStateOut{State: string(m.State()), Username: m.Username(), IdleLockMinutes: &minutes}, nil
}

func (h *apiHandler) accountRecover(c *web.Ctx, in accountRecoverIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := m.Recover(c.Context(), in.RecoveryKey, in.Password, in.Confirmation); err != nil {
		return accountStateOut{}, accountError(err)
	}

	// A new password ends the sessions the old one opened, as a change does.
	h.Sessions.Clear()

	if err := h.startPersonSession(c); err != nil {
		m.Lock()

		return accountStateOut{}, err
	}

	minutes := m.IdleLockMinutes()

	return accountStateOut{State: string(m.State()), Username: m.Username(), IdleLockMinutes: &minutes}, nil
}

// accountLogout locks the studio: the keys are wiped, every session ends, and the cookie is cleared.
func (h *apiHandler) accountLogout(c *web.Ctx, _ noIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := person(c); err != nil {
		return accountStateOut{}, err
	}

	m.Lock()
	h.Sessions.Clear()
	h.clearSessionCookie(c)
	c.SetHeader("Cache-Control", "no-store")

	return accountStateOut{State: string(m.State()), Username: m.Username()}, nil
}

// accountPassword changes the password; the other sessions end, and the caller gets a new one.
func (h *apiHandler) accountPassword(c *web.Ctx, in accountPasswordIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := person(c); err != nil {
		return accountStateOut{}, err
	}

	if err := m.ChangePassword(c.Context(), in.Current, in.Password, in.Confirmation); err != nil {
		return accountStateOut{}, accountError(err)
	}

	h.Sessions.Clear()

	if err := h.startPersonSession(c); err != nil {
		m.Lock()

		return accountStateOut{}, err
	}

	minutes := m.IdleLockMinutes()

	return accountStateOut{State: string(m.State()), Username: m.Username(), IdleLockMinutes: &minutes}, nil
}

func (h *apiHandler) accountSettingsGet(c *web.Ctx, _ noIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := person(c); err != nil {
		return accountStateOut{}, err
	}

	minutes := m.IdleLockMinutes()

	return accountStateOut{State: string(m.State()), Username: m.Username(), IdleLockMinutes: &minutes}, nil
}

func (h *apiHandler) accountSettings(c *web.Ctx, in accountSettingsIn) (accountStateOut, error) {
	m, err := h.accountOr()
	if err != nil {
		return accountStateOut{}, err
	}

	if err := person(c); err != nil {
		return accountStateOut{}, err
	}

	if err := m.SetIdleLock(c.Context(), in.IdleLockMinutes); err != nil {
		return accountStateOut{}, accountError(err)
	}

	minutes := m.IdleLockMinutes()

	return accountStateOut{State: string(m.State()), Username: m.Username(), IdleLockMinutes: &minutes}, nil
}

// startPersonSession makes the session of a person: privileged, in the cookie of this browser.
func (h *apiHandler) startPersonSession(c *web.Ctx) error {
	id, err := h.Sessions.Create(true)
	if err != nil {
		return err
	}

	h.setSessionCookie(c, id, auth.SessionMaxAge)
	c.SetHeader("Cache-Control", "no-store")

	return nil
}

// accountError maps the errors of the account to the envelope of the product. None of them holds a value that
// was typed.
func accountError(err error) error {
	if ve, ok := errors.AsType[*account.ValidationError](err); ok {
		fields := make([]map[string]string, 0, len(ve.Problems))
		for _, p := range ve.Problems {
			fields = append(fields, map[string]string{"field": p.Field, "reason": p.Reason})
		}

		return &service.Error{
			Code: command.CodeValidation, Message: "some fields of the account are not valid.",
			Hint: "fix each field and send it again", Details: map[string]any{"fields": fields},
		}
	}

	if rl, ok := errors.AsType[*account.RateLimitedError](err); ok {
		seconds := int(rl.RetryAfter/time.Second) + 1

		return &service.Error{
			Code: service.CodeRateLimited, Message: "too many failed attempts.", Retryable: true,
			Hint: "wait a minute and try again", Details: map[string]any{"retry_after": seconds},
		}
	}

	switch {
	case errors.Is(err, account.ErrWrongPassword):
		return &service.Error{Code: service.CodeUnauthorized, Message: "the username or the password is not right."}
	case errors.Is(err, account.ErrWrongRecoveryKey):
		return &service.Error{Code: service.CodeUnauthorized, Message: "the recovery key is not right."}
	case errors.Is(err, account.ErrExists):
		return &service.Error{
			Code: command.CodeInvalidState, Message: "an account exists already.", Hint: "sign in with it",
		}
	case errors.Is(err, account.ErrLocked):
		return &service.Error{
			Code: service.CodeLocked, Message: "Stuarty is locked: unlock Stuarty with your password.",
			Hint: "unlock Stuarty with your password", Details: map[string]any{"reason": "locked"},
		}
	case errors.Is(err, account.ErrNoAccount):
		return &service.Error{Code: command.CodeNotFound, Message: "there is no account yet.", Hint: "create it first"}
	}

	return err
}
