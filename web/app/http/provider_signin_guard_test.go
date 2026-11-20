package httpapi_test

import (
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// Epic 12 (SDD-007 section 3.6 item 4, T-75, T-76): the sign-in page of an AI provider is exactly one more
// page a provider may send the browser to, with the same conditions as the one of a social network, and
// nothing else is let in.

func TestSecurity_T76_GuardExceptionIsOnePathAndOneNavigation(t *testing.T) {
	t.Parallel()

	if httpapi.ProviderSignInCallbackPath != oauth.CallbackPath {
		t.Fatalf("the guard names %q and the sign-in %q", httpapi.ProviderSignInCallbackPath, oauth.CallbackPath)
	}

	h := newHarness(t)

	navigate := map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
	}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}

		maps.Copy(out, navigate)
		maps.Copy(out, extra)

		return out
	}

	if res := h.do(
		req{path: httpapi.ProviderSignInCallbackPath, headers: navigate},
	); res.status == http.StatusForbidden {
		t.Errorf("the cross-site navigation to the provider sign-in page = %d, want it let in", res.status)
	}

	for name, r := range map[string]req{
		"a fetch": {
			path:    httpapi.ProviderSignInCallbackPath,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"},
		},
		"a frame": {
			path:    httpapi.ProviderSignInCallbackPath,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"},
		},
		"a POST":                 {method: http.MethodPost, path: httpapi.ProviderSignInCallbackPath, body: "{}", headers: navigate},
		"an Origin":              {path: httpapi.ProviderSignInCallbackPath, headers: with(map[string]string{"Origin": "http://evil.example"})},
		"a trailing slash":       {path: httpapi.ProviderSignInCallbackPath + "/", headers: navigate},
		"a longer path":          {path: httpapi.ProviderSignInCallbackPath + "x", headers: navigate},
		"a sibling of the path":  {path: "/auth/login", headers: navigate},
		"the launch address":     {path: "/auth/launch?code=x", headers: navigate},
		"the page of the social": {path: "/connections", headers: navigate},
		"the completion by a navigation": {
			method: http.MethodPost, path: "/api/v1/providers/signin/complete", body: `{"state":"x"}`, headers: navigate,
		},
		"the begin by a navigation": {
			method: http.MethodPost, path: "/api/v1/providers/signin", body: `{"provider":"openai"}`, headers: navigate,
		},
	} {
		if res := h.do(r); res.status != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", name, res.status)
		}
	}
}

// The provider sends the browser back by a cross-site navigation, which carries no SameSite=Strict session
// cookie: the page is drawn all the same, with nothing of the workspace in it, and the call it makes still
// needs the session.
func TestProviderSignInPageIsDrawnWithoutTheSessionCookie(t *testing.T) {
	t.Parallel()

	h := pagesHarness(t)

	navigation := map[string]string{
		"Accept":         "text/html",
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
	}

	res := h.do(req{path: "/auth/callback?state=s&code=c&client_id=oaiapp_x", noAuth: true, headers: navigation})
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `Providers\/SignInCallback`) {
		t.Fatalf("the provider sign-in page without a session = %d, want the page (%.200s)", res.status, res.body)
	}

	if strings.Contains(string(res.body), `"auth"`) || strings.Contains(string(res.body), "prj_") {
		t.Errorf("the provider sign-in page carries data of the workspace: %.400s", res.body)
	}

	for name, r := range map[string]req{
		"the completion": {
			method: http.MethodPost, path: "/api/v1/providers/signin/complete", body: `{"state":"s","code":"c"}`,
			noAuth: true, headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
		},
		"the begin": {
			method: http.MethodPost, path: "/api/v1/providers/signin", body: `{"provider":"openai"}`,
			noAuth: true, headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
		},
		"the list": {path: "/api/v1/providers/sign-ins", noAuth: true, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}},
	} {
		if got := h.do(r); got.status != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", name, got.status)
		}
	}

	if got := h.do(req{
		method: http.MethodPost, path: "/auth/callback", body: `{}`, noAuth: true,
		headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
	}); got.status != http.StatusUnauthorized && got.status != http.StatusMethodNotAllowed && got.status != http.StatusNotFound {
		t.Errorf("a POST of the page without a session = %d, want 401, 404 or 405", got.status)
	}
}
