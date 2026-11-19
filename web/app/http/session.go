package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"

	"github.com/AlexGalhardo/vukem/web"
)

// launch is GET /auth/launch?code=...: the one route that takes a credential in
// a query string, and only a launch code (SDD-007 section 3.4 item 4). The host
// and Sec-Fetch-Site checks have already run in the guard.
func (h *apiHandler) launch(c *web.Ctx) error {
	if h.Launch.Locked() {
		return &service.Error{
			Code: service.CodeRateLimited, Message: "too many failed attempts.", Retryable: true,
			Details: map[string]any{"retry_after": 60},
		}
	}

	privileged, ok := h.Launch.Redeem(c.Query("code"))
	if !ok {
		return &service.Error{
			Code: service.CodeUnauthorized, Message: "the launch code is not valid.",
			Hint: "run stuarty open for a new one",
		}
	}

	id, err := h.Sessions.Create(privileged)
	if err != nil {
		return err
	}

	// Secure only over TLS: loopback is plain HTTP and Safari drops a Secure cookie there (SDD-007 section 3.5).
	c.SetCookie(&http.Cookie{ //nolint:gosec // see above
		Name: auth.LocalCookieName(
			browserPort(c.Request()),
		),
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   auth.SessionMaxAge,
		Secure:   c.Request().TLS != nil,
	})
	c.SetHeader("Cache-Control", "no-store")
	c.SetHeader("Referrer-Policy", "no-referrer")

	return c.RedirectStatus(http.StatusSeeOther, "/")
}

// systemLaunch is POST /api/v1/system/launch: a new launch code (SDD-007 section 3.4 item 3). The
// bearer token alone cannot tell the desktop shell from an agent, so only a request that also
// proves the launcher secret gets a code whose session has the browser-only privileges (section
// 5.3); `stuarty open` and any other bearer caller get a session without them.
func (h *apiHandler) systemLaunch(c *web.Ctx, _ noIn) (launchOut, error) {
	if err := requireJSONOrEmptyBody(c.Request()); err != nil {
		return launchOut{}, err
	}

	privileged := auth.IsLauncher(h.LauncherSecret, c.Request().Header.Get(auth.LauncherHeader))

	code, err := h.Launch.Issue(privileged)
	if err != nil {
		return launchOut{}, err
	}

	return launchOut{Code: code, URL: "http://127.0.0.1:" + listenPort(c.Request()) + "/auth/launch?code=" + code}, nil
}

// requireJSONOrEmptyBody refuses a body that is not JSON on a route that takes no input: the framework
// binds nothing for it, so a body such as `not json` would otherwise pass unseen (VV-141).
func requireJSONOrEmptyBody(r *http.Request) error {
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return &service.Error{
			Code:    "E_VALIDATION",
			Message: "the request body could not be read.",
			Hint:    "send a JSON object or no body",
		}
	}

	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || json.Valid(raw) {
		return nil
	}

	return &service.Error{
		Code:    "E_VALIDATION",
		Message: "the request body is not valid JSON.",
		Hint:    "send a JSON object or no body",
	}
}
