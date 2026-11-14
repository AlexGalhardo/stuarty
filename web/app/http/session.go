package httpapi

import (
	"net/http"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
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

	if !h.Launch.Redeem(c.Query("code")) {
		return &service.Error{
			Code: service.CodeUnauthorized, Message: "the launch code is not valid.",
			Hint: "run vukem-video open for a new one",
		}
	}

	id, err := h.Sessions.Create()
	if err != nil {
		return err
	}

	// Secure only over TLS: loopback is plain HTTP and Safari drops a Secure cookie there (SDD-007 section 3.5).
	c.SetCookie(&http.Cookie{ //nolint:gosec // see above
		Name: auth.CookieName, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: auth.SessionMaxAge, Secure: c.Request().TLS != nil,
	})
	c.SetHeader("Cache-Control", "no-store")
	c.SetHeader("Referrer-Policy", "no-referrer")

	return c.RedirectStatus(http.StatusSeeOther, "/")
}

// systemLaunch is POST /api/v1/system/launch: a new launch code for `vukem-video
// open` (SDD-007 section 3.4 item 3).
func (h *apiHandler) systemLaunch(c *web.Ctx, _ noIn) (launchOut, error) {
	code, err := h.Launch.Issue()
	if err != nil {
		return launchOut{}, err
	}

	return launchOut{Code: code, URL: "http://127.0.0.1:" + listenPort(c.Request()) + "/auth/launch?code=" + code}, nil
}
