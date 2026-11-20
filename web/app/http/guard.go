package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/web"
)

// relayHeaders reveal a tunnel or proxy: the peer is loopback while the client
// may not be (SDD-007 section 3.6 item 2).
var relayHeaders = []string{
	"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port", "X-Real-Ip", "Via",
}

// credentialParams are the query parameters that never carry a credential here
// (SDD-007 section 3.7, T-04).
var credentialParams = []string{"token", "key", "access_token", "api_key"}

const healthPath = "/up"

func refused() *service.Error {
	return &service.Error{Code: service.CodeForbidden, Message: "Request refused."}
}

// guard is the first thing every request meets in local mode (SDD-007 section
// 3.6): peer, relay, Host, Sec-Fetch-Site and Origin, then the rules that hold
// for every response (no CORS, OPTIONS is 405, no credential in a query). In
// network mode n replaces the first three and the origin values (4.3).
func guard(n *Network) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return guardWith(n, next) }
}

func guardWith(n *Network, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		if strings.HasPrefix(r.URL.Path, BasePath+"/") || strings.HasPrefix(r.URL.Path, "/auth/") ||
			r.URL.Path == "/openapi.json" {
			h.Set("Cache-Control", "no-store")
		}

		// Errors on the API are always problem+json, whatever the client prefers.
		// The handlers that answer with an image read what the client asked for
		// through requestedAccept, because the header below is rewritten for them.
		accept := r.Header.Get("Accept")
		if jsonPath(r.URL.Path) && !strings.Contains(accept, "json") {
			r.Header.Set("Accept", "application/json")
		}

		r = r.WithContext(context.WithValue(r.Context(), acceptKey{}, accept))

		if !requestAllowed(n, r) {
			web.Fail(w, r, refused())
			return
		}

		if r.Method == http.MethodOptions {
			h.Set("Allow", "GET, HEAD, POST")
			web.Fail(w, r, errs.ErrMethodNotAllowed)

			return
		}

		q := r.URL.Query()
		for _, name := range credentialParams {
			if q.Has(name) {
				web.Fail(w, r, &service.Error{
					Code:    "E_VALIDATION",
					Message: "a credential in the query string is refused.",
					Hint:    "send it in the Authorization header",
					Path:    name,
				})

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func requestAllowed(n *Network, r *http.Request) bool {
	if n != nil {
		return n.allowed(r)
	}

	return peerAllowed(r) && !hasRelay(r) && hostAllowed(r) && fetchSiteAllowed(r) && originAllowed(r)
}

type acceptKey struct{}

// requestedAccept is the Accept header as the client sent it.
func requestedAccept(r *http.Request) string {
	if a, ok := r.Context().Value(acceptKey{}).(string); ok {
		return a
	}

	return r.Header.Get("Accept")
}

// jsonPath tells the routes whose errors are always problem+json.
func jsonPath(p string) bool {
	// The sign-in page of an AI provider lives under /auth/ because the provider documents that address, but it
	// is a page: its errors are the ones of a page.
	if p == ProviderSignInCallbackPath {
		return false
	}

	return strings.HasPrefix(p, BasePath+"/") || strings.HasPrefix(p, "/auth/") || p == "/openapi.json"
}

func peerAddr(r *http.Request) (netip.Addr, bool) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}

	a := ap.Addr()
	if a.Zone() != "" {
		return netip.Addr{}, false
	}

	return a.Unmap(), true
}

func peerAllowed(r *http.Request) bool {
	a, ok := peerAddr(r)

	return ok && a.IsLoopback()
}

func hasRelay(r *http.Request) bool {
	for _, name := range relayHeaders {
		if _, ok := r.Header[http.CanonicalHeaderKey(name)]; ok {
			return true
		}
	}

	return false
}

// listenPort is the port of the socket that accepted the request.
func listenPort(r *http.Request) string {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return ""
	}

	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}

	return port
}

// browserPort is the port the browser used to reach this server (the Host header), which is not the
// listening port behind a published container port; it falls back to the listening port.
func browserPort(r *http.Request) string {
	if _, port, err := net.SplitHostPort(r.Host); err == nil && port != "" {
		return port
	}

	return listenPort(r)
}

func hostAllowed(r *http.Request) bool {
	port := listenPort(r)
	if port == "" {
		return false
	}

	// The container health probe calls /up as 127.0.0.1:7878 whatever APP_URL
	// says; a loopback peer without a proxy header is exempt (3.7 item 6).
	if r.URL.Path == healthPath && peerAllowed(r) && !hasRelay(r) {
		return true
	}

	switch strings.ToLower(r.Host) {
	case "127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port:
		return true
	}

	return false
}

// SignInCallbackPath is the one page a person may reach from another site: the redirect address of the
// sign-in to a social network (SDD-020 section 6). The network sends the browser there, so the request is
// a cross-site navigation by nature.
const SignInCallbackPath = "/social/callback"

// ProviderSignInCallbackPath is the same page for the sign-in to an AI provider (SDD-016 section 12): the
// provider sends the browser there with a state and a code. It is exactly one more path with exactly the
// same conditions, never a prefix or a pattern (SDD-007 section 3.6 item 4, T-75, T-76).
const ProviderSignInCallbackPath = "/auth/callback"

// isSignInCallbackPath says whether path is one of the two pages a provider or a network sends the browser to.
func isSignInCallbackPath(path string) bool {
	return path == SignInCallbackPath || path == ProviderSignInCallbackPath
}

// signInRedirect says whether r is the top-level navigation of the browser coming back from a consent
// screen: a GET of the sign-in page, as a navigation that loads a document. The page holds no data and
// changes nothing; it hands the state and the code to POST /api/v1/social/complete, which is a
// same-origin request with every check of this guard, and the state is single-use and bound to the
// session that began it (SDD-007 section 3.6 item 4, T-59).
func signInRedirect(r *http.Request) bool {
	return r.Method == http.MethodGet && isSignInCallbackPath(r.URL.Path) &&
		r.Header.Get("Sec-Fetch-Mode") == "navigate" && r.Header.Get("Sec-Fetch-Dest") == "document"
}

// googleRedirect is signInRedirect for the second and last cross-site page, which exists in network mode
// only: where Google sends the browser back after "Sign in with Google" (SDD-007 section 3.6 item 4 and
// 4.7, T-82). Same shape, no wider: a GET navigation that loads a document, to that one path.
func googleRedirect(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == auth.GoogleCallbackPath &&
		r.Header.Get("Sec-Fetch-Mode") == "navigate" && r.Header.Get("Sec-Fetch-Dest") == "document"
}

func fetchSiteAllowed(r *http.Request) bool { return fetchSiteAllowedIn(r, false) }

// fetchSiteAllowedIn is step 4 of SDD-007 section 3.6; network is true in network mode, which has the
// callback page of the Google sign-in as a second exception.
func fetchSiteAllowedIn(r *http.Request, network bool) bool {
	site, present := r.Header["Sec-Fetch-Site"]
	if !present {
		return true
	}

	if len(site) != 1 {
		return false
	}

	switch site[0] {
	case "same-origin", "none":
		return true
	case "same-site", "cross-site":
		return signInRedirect(r) || (network && googleRedirect(r))
	}

	return false
}

func originAllowed(r *http.Request) bool {
	origin, present := r.Header["Origin"]
	if !present {
		return true
	}

	port := listenPort(r)
	if len(origin) != 1 || port == "" {
		return false
	}

	switch origin[0] {
	case "http://127.0.0.1:" + port, "http://localhost:" + port, "http://[::1]:" + port:
		return true
	}

	return false
}
